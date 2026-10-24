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
	"orchestrator/internal/planning"
	probes "orchestrator/internal/ports/diagnostics"
)

type fakePlanner struct {
	preview pending.Preview
	err     error
}

func (p fakePlanner) Preview(context.Context, string, string) (pending.Preview, error) {
	return p.preview, p.err
}

// A new Application with only saved workload drafts (nothing deployed) must be
// diagnosable: requirements come from the planning pipeline over drafts and the
// applied set together, not from a current Deployment Set alone.
func TestDraftOnlyApplicationIsPlannedBeforeDeployment(t *testing.T) {
	f := newFixture(t, Config{BaseDomain: "example.com"})
	ctx := context.Background()
	if err := f.st.SaveOrganization(ctx, application.Organization{Key: "acme", Name: "acme"}); err != nil {
		t.Fatal(err)
	}
	if err := f.st.SaveApplication(ctx, application.Application{ID: "00000000-0000-4000-8000-000000000001", Key: "shop", OrganizationKey: "acme", Name: "Shop", Subdomain: "shop", Version: 3}); err != nil {
		t.Fatal(err)
	}
	set := environment.DeploymentSet{ID: "set-1", EnvironmentKey: "staging", Document: environment.NewDocument()}
	if err := f.st.SaveDeploymentSet(ctx, set); err != nil {
		t.Fatal(err)
	}
	if err := f.st.SaveEnvironment(ctx, environment.Environment{ID: "00000000-0000-4000-8000-000000000002", Key: "staging", ApplicationID: "00000000-0000-4000-8000-000000000001", ApplicationKey: "shop", Name: "staging", Type: "development", NamespaceIdentity: "shop-staging", CurrentDeploymentSetID: set.ID, Version: 2, DraftVersion: 5}); err != nil {
		t.Fatal(err)
	}
	draft := map[string]any{
		"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "web"},
		"containers": map[string]any{"web": map[string]any{"image": "nginx:1"}},
		"service":    map[string]any{"ports": map[string]any{"http": map[string]any{"port": 80}}, "publicRoutes": []any{map[string]any{"path": "/", "port": "http"}}},
	}
	if err := f.st.SaveWorkloadDraft(ctx, 5, environment.WorkloadDraft{ApplicationKey: "shop", EnvironmentKey: "staging", WorkloadID: "web", State: environment.DraftUpsert, Score: draft}); err != nil {
		t.Fatal(err)
	}
	intent := planning.ResourceIntent{Descriptor: "postgres.default#modules.web.externals.db", ResourceType: "postgres", Class: "default", DefinitionKey: "pg", DriverType: "kubernetes", Intent: planning.IntentCreate}
	f.svc.SetPlanner(fakePlanner{preview: pending.Preview{Changes: []pending.Change{{WorkloadID: "web", ResourceIntents: []planning.ResourceIntent{intent}, Placements: []planning.Placement{{Node: intent.Descriptor, Account: "k8s-a", Cluster: "internal.k8s-a", Namespace: "shop-staging"}}}}}})
	if err := f.st.SaveResourceType(ctx, "acme", resource.Type{Key: "postgres", Outputs: []resource.OutputField{{Name: "host", Type: "string"}}}); err != nil {
		t.Fatal(err)
	}
	if err := f.st.SaveResourceDefinition(ctx, "acme", resource.Definition{Key: "pg", Name: "pg", ResourceTypeKey: "postgres", DriverType: resource.DriverKubernetes, Criteria: []resource.Criterion{{}}, Version: 4}); err != nil {
		t.Fatal(err)
	}

	ref, need, err := f.svc.resolveContext(ctx, "acme", domain.Request{Application: "shop", Environment: "staging"})
	if err != nil {
		t.Fatal(err)
	}
	if !need.Resolved || need.Workloads != 1 || need.PublicRoutes != 1 || need.PendingChanges != 1 || need.PublicHost != "shop.example.com" {
		t.Fatalf("drafts were not planned: %+v", need)
	}
	if len(need.Resources) != 1 || need.Resources[0].Cluster != "k8s-a" || !need.Resources[0].Storage.Required || !need.Resources[0].Storage.Resolved || need.Resources[0].Storage.Class != "" {
		t.Fatalf("resource need: %+v", need.Resources)
	}
	if ref.DraftVersion != 6 || len(ref.Definitions) != 1 || ref.Definitions[0].Key != "pg" || ref.Definitions[0].Version != 4 {
		t.Fatalf("pins: %+v", ref)
	}
	// The saved drafts and applied set were not changed by planning.
	if got, _ := f.st.GetEnvironment(ctx, "shop", "staging"); got.Version != 2 || got.DraftVersion != 6 {
		t.Fatalf("environment changed: %+v", got)
	}

	// A planner rejection is a FAIL with the planner enums, not an empty context.
	f.svc.SetPlanner(fakePlanner{err: &planning.StageError{Stage: planning.StageCatalog, Reason: planning.ReasonResourceNoMatch}})
	_, need, err = f.svc.resolveContext(ctx, "acme", domain.Request{Application: "shop", Environment: "staging"})
	if err != nil || need.Resolved || need.PlanStage != "catalog" || need.PlanReason != planning.ReasonResourceNoMatch {
		t.Fatalf("rejected plan: %+v %v", need, err)
	}
	finding := f.svc.evalPlatform(ctx, probes.Check{ID: "uc.preview.requirements"}, domain.Scope{}, application.Connection{}, need, "acme", nil)
	if finding.Verdict != domain.VerdictFail || finding.Reason != "PLANNING_REJECTED" || finding.Facts["planReason"] != planning.ReasonResourceNoMatch {
		t.Fatalf("rejection finding: %+v", finding)
	}
	f.svc.SetPlanner(fakePlanner{err: errors.New("database down")})
	_, need, _ = f.svc.resolveContext(ctx, "acme", domain.Request{Application: "shop", Environment: "staging"})
	if got := f.svc.evalPlatform(ctx, probes.Check{ID: "uc.preview.requirements"}, domain.Scope{}, application.Connection{}, need, "acme", nil); got.Verdict != domain.VerdictUnknown || got.Reason != "PLAN_UNAVAILABLE" {
		t.Fatalf("an unreadable plan is unknown: %+v", got)
	}
}

func TestStorageRequirementsAreComparedWithTheRealInventory(t *testing.T) {
	scope := domain.Scope{AccountType: "INTERNAL", AccountKey: "lab"}
	acct := application.Connection{Key: "lab"}
	res := func(s probes.StorageNeed) probes.ResourceNeed {
		return probes.ResourceNeed{Descriptor: "d", Type: "postgres", Cluster: "lab", Storage: s}
	}
	inventory := func(defaults string, classes string) map[string]domain.Finding {
		return map[string]domain.Finding{"k8s.storage.classes": {Verdict: domain.VerdictPass, Facts: map[string]string{"defaultClasses": defaults, "classes": classes}}}
	}
	cases := []struct {
		name    string
		need    probes.ResourceNeed
		prior   map[string]domain.Finding
		verdict domain.Verdict
		reason  string
	}{
		{"default present", res(probes.StorageNeed{Required: true, Resolved: true}), inventory("1", "standard"), domain.VerdictPass, "STORAGE_CLASSES_PRESENT"},
		{"no default class", res(probes.StorageNeed{Required: true, Resolved: true}), inventory("0", "standard"), domain.VerdictFail, "NO_DEFAULT_STORAGE_CLASS"},
		{"no class at all", res(probes.StorageNeed{Required: true, Resolved: true}), inventory("0", ""), domain.VerdictFail, "NO_DEFAULT_STORAGE_CLASS"},
		{"two defaults", res(probes.StorageNeed{Required: true, Resolved: true}), inventory("2", "a,b"), domain.VerdictFail, "MULTIPLE_DEFAULT_STORAGE_CLASSES"},
		{"explicit missing", res(probes.StorageNeed{Required: true, Resolved: true, Class: "gp2"}), inventory("1", "standard"), domain.VerdictFail, "STORAGE_CLASS_MISSING"},
		{"explicit present", res(probes.StorageNeed{Required: true, Resolved: true, Class: "gp2"}), inventory("0", "gp2,standard"), domain.VerdictPass, "STORAGE_CLASSES_PRESENT"},
		{"expression is not default", res(probes.StorageNeed{Required: true}), inventory("1", "standard"), domain.VerdictUnknown, "STORAGE_CLASS_UNRESOLVED"},
		{"undeclared module", res(probes.StorageNeed{}), inventory("1", "standard"), domain.VerdictUnknown, "STORAGE_REQUIREMENT_UNDECLARED"},
		{"inventory unavailable", res(probes.StorageNeed{Required: true, Resolved: true}), map[string]domain.Finding{"k8s.storage.classes": {Verdict: domain.VerdictUnknown}}, domain.VerdictUnknown, "STORAGE_EVIDENCE_UNAVAILABLE"},
	}
	for _, c := range cases {
		got := evalCapability("database", scope, acct, &probes.Requirements{Resolved: true, Resources: []probes.ResourceNeed{c.need}}, c.prior)
		if got.Verdict != c.verdict || got.Reason != c.reason {
			t.Errorf("%s: %+v", c.name, got)
		}
	}
	// An AWS-placed resource on another region is a mismatch, never readiness.
	aws := domain.Scope{AccountType: "AWS", AccountKey: "cloud", Region: "us-east-1"}
	r := probes.ResourceNeed{Type: "redis", Providers: []probes.ProviderTarget{{Kind: "aws", Connection: "cloud", Region: "eu-west-1"}}}
	if got := evalCapability("cache", aws, application.Connection{Key: "cloud"}, &probes.Requirements{Resolved: true, Resources: []probes.ResourceNeed{r}}, nil); got.Verdict != domain.VerdictFail || got.Reason != "REGION_MISMATCH" {
		t.Errorf("region mismatch: %+v", got)
	}
	other := probes.ResourceNeed{Type: "redis", Providers: []probes.ProviderTarget{{Kind: "aws", Connection: "somewhere-else", Region: "us-east-1"}}}
	if got := evalCapability("cache", aws, application.Connection{Key: "cloud"}, &probes.Requirements{Resolved: true, Resources: []probes.ResourceNeed{other}}, nil); got.Verdict != domain.VerdictNA {
		t.Errorf("resource of another account: %+v", got)
	}
}
