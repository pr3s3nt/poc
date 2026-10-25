package accountdiag

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/application/pending"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	probes "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/ports/persistence"
)

type fakePlanner struct {
	desired pending.Desired
	err     error
	calls   int
}

func (p *fakePlanner) Desired(context.Context, string, string) (pending.Desired, error) {
	p.calls++
	return p.desired, p.err
}

func contextFixture(t *testing.T, envKey string) *fixture {
	t.Helper()
	f := newFixture(t, Config{BaseDomain: "example.com"})
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(f.st.SaveApplication(ctx, application.Application{ID: "00000000-0000-4000-8000-000000000001", Key: "shop", OrganizationKey: "acme", Name: "Shop", Subdomain: "shop", Version: 1}))
	must(f.st.SaveDeploymentSet(ctx, environment.DeploymentSet{ID: "set-" + envKey, EnvironmentKey: envKey, Document: environment.NewDocument()}))
	must(f.st.SaveEnvironment(ctx, environment.Environment{ID: "00000000-0000-4000-8000-000000000002", Key: envKey, ApplicationID: "00000000-0000-4000-8000-000000000001", ApplicationKey: "shop", Name: envKey, Type: "development", NamespaceIdentity: "shop-" + envKey, CurrentDeploymentSetID: "set-" + envKey, Version: 1}))
	return f
}

func plannedDesired(envKey string) pending.Desired {
	intent := planning.ResourceIntent{Descriptor: "postgres.default#modules.web.externals.db", ResourceType: "postgres", Class: "default", DefinitionKey: "pg", DriverType: "terraform", Intent: planning.IntentUnverified,
		Module: &planning.IntentModule{Module: "postgres-k8s", SourceID: "packaged", Revision: "r1", ContentSHA256: "abc"}, Providers: []planning.IntentProvider{{Name: "kubernetes", Kind: "kubernetes", ConnectionKey: "k8s-a"}}}
	doc := environment.NewDocument()
	doc.Modules["web"] = environment.Module{Spec: environment.ModuleSpec{Service: &environment.Service{PublicRoutes: []environment.PublicPath{{Path: "/", Port: "http"}}}}}
	bundle := tfmodule.Bundle{Ref: tfmodule.SourceRef{ContentSHA256: "abc"}, Contract: tfmodule.Contract{SourceKind: tfmodule.SourceKindPackaged, Inputs: map[string]tfmodule.Input{"storage_class": {Type: "string", HasDefault: true, Default: ""}}}}
	return pending.Desired{
		Application:   application.Application{Key: "shop", OrganizationKey: "acme", Name: "Shop (snapshot)", Subdomain: "snap", Version: 7},
		Environment:   environment.Environment{Key: envKey, ApplicationKey: "shop", Type: "staging-type", NamespaceIdentity: "shop-" + envKey, CurrentDeploymentSetID: "set-" + envKey, Version: 5, DraftVersion: 9},
		ConfigVersion: 4, Final: doc, Workloads: 1, PublicRoutes: 1,
		Definitions: []resource.Definition{{Key: "pg", Name: "pg", ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform, Version: 4}},
		Bundles:     map[string]tfmodule.Bundle{intent.Descriptor: bundle},
		Preview:     pending.Preview{Changes: []pending.Change{{ResourceIntents: []planning.ResourceIntent{intent}, Placements: []planning.Placement{{Node: intent.Descriptor, Account: "k8s-a", Namespace: "shop-" + envKey}}}}},
	}
}

// Requirements, pins and counts all come from the planner's one snapshot, not
// from reads of the outer store (which here still shows older versions), and the
// public host is the canonical one of the diagnosed Environment.
func TestContextIsPinnedFromTheSinglePlanningSnapshot(t *testing.T) {
	f := contextFixture(t, "staging")
	ctx := context.Background()
	f.svc.SetPlanner(&fakePlanner{desired: plannedDesired("staging")})
	ref, need, err := f.svc.resolveContext(ctx, "acme", domain.Request{Application: "shop", Environment: "staging"})
	if err != nil {
		t.Fatal(err)
	}
	if ref.ApplicationVersion != 7 || ref.EnvironmentVersion != 5 || ref.DraftVersion != 9 || ref.ConfigVersion != 4 {
		t.Fatalf("pins were not taken from the planning snapshot: %+v", ref)
	}
	if len(ref.Definitions) != 1 || ref.Definitions[0] != (domain.DefinitionPin{Key: "pg", Version: 4}) {
		t.Fatalf("definition pins: %+v", ref.Definitions)
	}
	if !need.Resolved || need.Workloads != 1 || need.PublicRoutes != 1 || len(need.Resources) != 1 {
		t.Fatalf("requirements: %+v", need)
	}
	r := need.Resources[0]
	if r.Cluster != "k8s-a" || !r.Packaged || !r.Storage.Required || !r.Storage.Resolved || r.Storage.Class != "" {
		t.Fatalf("resource need: %+v", r)
	}
	// The outer reads (Subdomain "", name "Shop", type "development") were only
	// used to validate the selection; every reported fact is the snapshot's.
	if need.PublicHost != "staging.snap.example.com" || ref.ApplicationName != "Shop (snapshot)" || ref.EnvironmentType != "staging-type" || need.EnvironmentType != "staging-type" {
		t.Fatalf("context facts must come from the planning snapshot: host=%q ref=%+v", need.PublicHost, ref)
	}
	prod := contextFixture(t, "production")
	prod.svc.SetPlanner(&fakePlanner{desired: plannedDesired("production")})
	if _, prodNeed, err := prod.svc.resolveContext(ctx, "acme", domain.Request{Application: "shop", Environment: "production"}); err != nil || prodNeed.PublicHost != "snap.example.com" {
		t.Fatalf("production host: %+v %v", prodNeed, err)
	}
}

func TestPlannerFailuresNeverBecomeAnEmptyContext(t *testing.T) {
	f := contextFixture(t, "staging")
	ctx := context.Background()
	want := domain.Request{Application: "shop", Environment: "staging"}
	f.svc.SetPlanner(&fakePlanner{err: errors.New("database down")})
	if _, _, err := f.svc.resolveContext(ctx, "acme", want); !errors.Is(err, ErrContextUnavailable) {
		t.Fatalf("an unreadable desired state must refuse the run: %v", err)
	}
	f.svc.SetPlanner(&fakePlanner{err: persistence.ErrNotFound})
	if _, _, err := f.svc.resolveContext(ctx, "acme", want); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("not found: %v", err)
	}
	// A planner rejection is a FAIL with the planner enums, still pinned.
	rejected := plannedDesired("staging")
	rejected.PlanErr = &planning.StageError{Stage: planning.StageCatalog, Reason: planning.ReasonResourceNoMatch}
	f.svc.SetPlanner(&fakePlanner{desired: rejected})
	ref, need, err := f.svc.resolveContext(ctx, "acme", want)
	if err != nil || need.Resolved || need.PlanStage != "catalog" || need.PlanReason != planning.ReasonResourceNoMatch || ref.DraftVersion != 9 {
		t.Fatalf("rejected plan: %+v %+v %v", need, ref, err)
	}
	finding := f.svc.evalPlatform(ctx, probes.Check{ID: "uc.preview.requirements"}, domain.Scope{}, application.Connection{}, need, "acme", nil)
	if finding.Verdict != domain.VerdictFail || finding.Reason != "PLANNING_REJECTED" || finding.Facts["planReason"] != planning.ReasonResourceNoMatch {
		t.Fatalf("rejection finding: %+v", finding)
	}
	// Without a planner nothing is claimed about requirements.
	f.svc.planner = nil
	_, need, _ = f.svc.resolveContext(ctx, "acme", want)
	if got := f.svc.evalPlatform(ctx, probes.Check{ID: "uc.preview.requirements"}, domain.Scope{}, application.Connection{}, need, "acme", nil); got.Verdict != domain.VerdictUnknown || got.Reason != "PLAN_UNAVAILABLE" {
		t.Fatalf("no planner: %+v", got)
	}
}

func TestCapabilityRequirementsUsePlacementAndTheProviderStorageVerdict(t *testing.T) {
	scope := domain.Scope{AccountType: "INTERNAL", AccountKey: "lab"}
	acct := application.Connection{Key: "lab"}
	res := probes.ResourceNeed{Descriptor: "d", Type: "postgres", Cluster: "lab"}
	need := &probes.Requirements{Resolved: true, Resources: []probes.ResourceNeed{res}}
	storage := func(v domain.Verdict, reason string) map[string]domain.Finding {
		return map[string]domain.Finding{"k8s.storage.requirements": {Verdict: v, Reason: reason, Facts: map[string]string{"storageNeeds": "1"}}}
	}
	if got := evalCapability("database", scope, acct, need, storage(domain.VerdictPass, "STORAGE_REQUIREMENTS_MET")); got.Verdict != domain.VerdictPass || got.Facts["storageNeeds"] != "1" {
		t.Errorf("provider PASS is carried: %+v", got)
	}
	if got := evalCapability("database", scope, acct, need, storage(domain.VerdictFail, "STORAGE_CLASS_MISSING")); got.Verdict != domain.VerdictFail || got.Reason != "STORAGE_CLASS_MISSING" {
		t.Errorf("provider FAIL is carried: %+v", got)
	}
	if got := evalCapability("database", scope, acct, need, nil); got.Verdict != domain.VerdictUnknown || got.Reason != "STORAGE_EVIDENCE_UNAVAILABLE" {
		t.Errorf("missing evidence: %+v", got)
	}
	if got := evalCapability("cache", scope, acct, need, nil); got.Verdict != domain.VerdictNA || got.Reason != "NO_RESOURCE_FOR_USE_CASE" {
		t.Errorf("another capability: %+v", got)
	}
	elsewhere := &probes.Requirements{Resolved: true, Resources: []probes.ResourceNeed{{Type: "postgres", Cluster: "other"}}}
	if got := evalCapability("database", scope, acct, elsewhere, nil); got.Verdict != domain.VerdictNA || got.Reason != "PLACED_ON_OTHER_ACCOUNT" {
		t.Errorf("another account: %+v", got)
	}
	// An AWS-placed resource in another region is a mismatch, never readiness.
	aws := domain.Scope{AccountType: "AWS", AccountKey: "cloud", Region: "us-east-1"}
	r := probes.ResourceNeed{Type: "redis", Providers: []probes.ProviderTarget{{Kind: "aws", Connection: "cloud", Region: "eu-west-1"}}}
	if got := evalCapability("cache", aws, application.Connection{Key: "cloud"}, &probes.Requirements{Resolved: true, Resources: []probes.ResourceNeed{r}}, nil); got.Verdict != domain.VerdictFail || got.Reason != "REGION_MISMATCH" {
		t.Errorf("region mismatch: %+v", got)
	}
}

// A secrets-, HTTPS- or lifecycle-only selection never plans the desired state:
// an unrelated invalid draft cannot fail it, while public routes are still
// counted from the drafts and the applied set.
func TestSelectionWithoutPlanDependentWorkSkipsThePlanCheck(t *testing.T) {
	ids := func(scope domain.Scope) map[string]bool {
		out := map[string]bool{}
		for _, c := range platformChecks(scope, nil) {
			out[c.ID] = true
		}
		return out
	}
	only := ids(domain.Scope{Purpose: domain.PurposeUseCase, UseCases: []string{"secrets", "https", "lifecycle"}})
	if only["uc.preview.requirements"] || !only["uc.secrets.store"] || !only["uc.https.certificate"] {
		t.Errorf("secrets/https/lifecycle plan %v", only)
	}
	for _, use := range []string{"preview", "deploy", "database", "cache"} {
		if !ids(domain.Scope{Purpose: domain.PurposeUseCase, UseCases: []string{use}})["uc.preview.requirements"] {
			t.Errorf("%s needs the plan check", use)
		}
	}
	f := contextFixture(t, "staging")
	rejected := plannedDesired("staging")
	rejected.PlanErr = &planning.StageError{Stage: planning.StageCatalog, Reason: planning.ReasonResourceNoMatch}
	rejected.Final = environment.NewDocument() // a rejected plan has no final document
	f.svc.SetPlanner(&fakePlanner{desired: rejected})
	_, need, err := f.svc.resolveContext(context.Background(), "acme", domain.Request{Application: "shop", Environment: "staging"})
	if err != nil || need.Resolved || need.PublicRoutes != 1 || need.Workloads != 1 {
		t.Fatalf("routes of a rejected plan are still counted from the drafts and applied set: %+v %v", need, err)
	}
}
