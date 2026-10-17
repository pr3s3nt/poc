package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"orchestrator/internal/domain/tfmodule"
	"reflect"
	"strings"
	"testing"

	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

func previewCandidate(key string, criteria ...resource.Criterion) catalog.DefinitionDraft {
	return catalog.DefinitionDraft{Key: key, Name: key, ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform, Criteria: criteria}
}

// awsDraft is an unsaved AWS Terraform candidate bound to a Driver Account and a
// literal region, as the Console authors it.
func awsDraft(key, account, region string, criteria ...resource.Criterion) catalog.DefinitionDraft {
	variables := map[string]any{}
	if region != "" {
		variables["region"] = region
	}
	return catalog.DefinitionDraft{Key: key, Name: "AWS " + key, ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform, DriverAccount: account,
		DriverInputs: map[string]any{"values": map[string]any{"variables": variables, "provider_bindings": map[string]any{"aws": map[string]any{"target": "account"}}}},
		Criteria:     criteria}
}

func previewQuery(opts seed.Options, def catalog.DefinitionDraft, resID, class string) catalog.MatchingPreviewQuery {
	return catalog.MatchingPreviewQuery{Definition: def, Context: catalog.MatchingContext{
		ApplicationID: opts.ApplicationKey, EnvironmentID: opts.EnvironmentKey, ResourceID: resID, Class: class}}
}

func runPreview(t *testing.T, svc *catalog.Service, opts seed.Options, q catalog.MatchingPreviewQuery) catalog.MatchingPreview {
	t.Helper()
	got, err := svc.PreviewMatching(context.Background(), opts.OrganizationKey, q)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	return got
}

func candidateByKey(p catalog.MatchingPreview, key string) (catalog.MatchingCandidate, bool) {
	for _, c := range p.Candidates {
		if c.Key == key {
			return c, true
		}
	}
	return catalog.MatchingCandidate{}, false
}

// UC-03 V02-02: ranking uses only the five criteria. The AWS Definition is not
// "ineligible"; it simply does not match this Application's context.
func TestPreviewMatching_TiesAreDecidedByScoreAndNoProfileOrAccountFilter(t *testing.T) {
	_, svc, opts := seededCatalog(t, nil)
	got := runPreview(t, svc, opts, previewQuery(opts, previewCandidate("pg-app", resource.Criterion{ApplicationID: opts.ApplicationKey}), "db", "default"))
	if got.Status != catalog.PreviewAmbiguous || !reflect.DeepEqual(got.Ties, []string{"pg-app", "postgres-internal-statefulset"}) || got.Winner != nil {
		t.Fatalf("got %+v", got)
	}
	aws, ok := candidateByKey(got, "postgres-aws-aurora")
	if !ok || aws.Matched || aws.Score != nil || aws.Name == "" {
		t.Fatalf("aws candidate must be listed but not matched: %+v", aws)
	}
	// A wildcard (score 0) loses to the seeded explicit app_id criterion.
	wild := runPreview(t, svc, opts, previewQuery(opts, previewCandidate("pg-wild", resource.Criterion{}), "db", "default"))
	if wild.Status != catalog.PreviewMatched || wild.Winner.Key != "postgres-internal-statefulset" {
		t.Fatalf("wildcard: %+v", wild)
	}
	if got.Snapshot.Marker == "" || got.Snapshot.EnvironmentVersion == 0 {
		t.Fatalf("snapshot %+v", got.Snapshot)
	}
}

func TestPreviewMatching_HigherScoreEliminatesLowerTie(t *testing.T) {
	_, svc, opts := seededCatalog(t, nil)
	def := previewCandidate("pg-class", resource.Criterion{Class: "fast"}, resource.Criterion{ResourceID: "db"})
	got := runPreview(t, svc, opts, previewQuery(opts, def, "db", "fast"))
	if got.Status != catalog.PreviewMatched || got.Winner == nil || got.Winner.Key != "pg-class" || got.Winner.Score != 16 || len(got.Ties) != 0 {
		t.Fatalf("got %+v", got)
	}
	c, _ := candidateByKey(got, "pg-class")
	if !c.Matched || c.Score == nil || *c.Score != 16 || c.BestCriterion == nil || c.BestCriterion.Class != "fast" {
		t.Fatalf("candidate %+v", c)
	}
	shared, _ := candidateByKey(got, "postgres-internal-statefulset")
	if !shared.Matched || *shared.Score != 2 {
		t.Fatalf("shared %+v", shared)
	}
}

func TestPreviewMatching_ExactTieAtHighestScoreOnly(t *testing.T) {
	st, svc, opts := seededCatalog(t, nil)
	ctx := context.Background()
	other := previewCandidate("pg-other", resource.Criterion{ResourceID: "db"}).Definition()
	other.Version = 1
	if err := st.CreateResourceDefinition(ctx, opts.OrganizationKey, other); err != nil {
		t.Fatal(err)
	}
	got := runPreview(t, svc, opts, previewQuery(opts, previewCandidate("pg-new", resource.Criterion{ApplicationID: opts.ApplicationKey, EnvironmentID: opts.EnvironmentKey}, resource.Criterion{ResourceID: "db"}), "db", "default"))
	// pg-new best = app+env = 6, but res_id db = 8 ties with pg-other at 8.
	if got.Status != catalog.PreviewAmbiguous || !reflect.DeepEqual(got.Ties, []string{"pg-new", "pg-other"}) {
		t.Fatalf("got %+v", got)
	}
	got = runPreview(t, svc, opts, previewQuery(opts, previewCandidate("pg-new", resource.Criterion{ResourceID: "db"}, resource.Criterion{ApplicationID: opts.ApplicationKey}), "db", "default"))
	if got.Status != catalog.PreviewAmbiguous || !reflect.DeepEqual(got.Ties, []string{"pg-new", "pg-other"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestPreviewMatching_NonOverlapAndNoMatch(t *testing.T) {
	_, svc, opts := seededCatalog(t, nil)
	// The shared Definition has an empty criterion, so a non-overlapping candidate
	// loses to nothing: the shared one wins alone.
	got := runPreview(t, svc, opts, previewQuery(opts, previewCandidate("pg-staging", resource.Criterion{EnvironmentID: "other-env"}), "db", "default"))
	if got.Status != catalog.PreviewMatched || got.Winner.Key != "postgres-internal-statefulset" {
		t.Fatalf("got %+v", got)
	}
	c, _ := candidateByKey(got, "pg-staging")
	if c.Matched {
		t.Fatalf("candidate %+v", c)
	}
	// A different type has no matching Definition.
	def := catalog.DefinitionDraft{Key: "ns-only", Name: "ns-only", ResourceTypeKey: "k8s-namespace", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{ResourceID: "nope"}}}
	q := previewQuery(opts, def, "x", "default")
	got = runPreview(t, svc, opts, q)
	if got.Status != catalog.PreviewMatched || got.Winner.Key != "namespace-kubernetes" {
		t.Fatalf("got %+v", got)
	}
	nc, _ := candidateByKey(got, "ns-only")
	if nc.Matched {
		t.Fatalf("non-overlapping candidate matched: %+v", nc)
	}
}

func TestPreviewMatching_IncompleteContextHasNoWinnerOrTies(t *testing.T) {
	_, svc, opts := seededCatalog(t, nil)
	for name, ctx := range map[string][2]string{"both": {"", ""}, "resource": {"", "default"}, "class": {"db", ""}} {
		got := runPreview(t, svc, opts, previewQuery(opts, previewCandidate("pg-wild", resource.Criterion{}), ctx[0], ctx[1]))
		if got.Status != catalog.PreviewIncomplete || got.Winner != nil || len(got.Ties) != 0 || len(got.MissingFields) == 0 {
			t.Fatalf("%s: %+v", name, got)
		}
		for _, c := range got.Candidates {
			if c.Matched {
				t.Fatalf("%s: incomplete context must not match: %+v", name, c)
			}
		}
	}
}

// The unique winner shows its Name, selected Driver Account (key, name, type,
// version) and literal region; nothing secret is part of the view.
func TestPreviewMatching_WinnerShowsSafeAccountAndRegion(t *testing.T) {
	_, svc, opts := seededCatalog(t, nil)
	def := awsDraft("pg-aws", opts.CloudConnectionKey, "ap-southeast-1", resource.Criterion{ResourceID: "db"})
	got := runPreview(t, svc, opts, previewQuery(opts, def, "db", "default"))
	if got.Status != catalog.PreviewMatched || got.Winner == nil || got.Winner.Key != "pg-aws" || got.Winner.Name != "AWS pg-aws" ||
		got.Winner.Region != "ap-southeast-1" || got.Winner.Account == nil || *got.Winner.Account != (catalog.MatchingAccount{Key: opts.CloudConnectionKey, Name: opts.CloudConnectionKey, Type: "AWS", Version: 1}) {
		t.Fatalf("got %+v", got)
	}
	raw, _ := json.Marshal(got)
	for _, leak := range []string{"secret://", "accessKey", "secretRef", "accountId", "000000000000"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("preview leaks %q: %s", leak, raw)
		}
	}
}

// An unusable winner account is reported as a conflict and never replaced by
// another candidate; a losing candidate's conflict does not matter.
func TestPreviewMatching_AccountConflictKeepsWinnerWithoutFallback(t *testing.T) {
	st, svc, opts := seededCatalog(t, nil)
	ctx := context.Background()
	if err := st.SaveConnection(ctx, application.Connection{Name: "pending-aws", AuthenticationType: application.AuthAWSAccessKey, Key: "pending-aws", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionAWS, Status: application.ConnectionVerifying, Version: 1}); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct{ account, region, reason string }{
		"no account":      {"", "us-east-1", catalog.ConflictAccountMissing},
		"unknown account": {"gone", "us-east-1", catalog.ConflictAccountNotFound},
		"internal kind":   {opts.ConnectionKey, "us-east-1", catalog.ConflictAccountKindMismatch},
		"not ready":       {"pending-aws", "us-east-1", catalog.ConflictAccountNotReady},
		"no region":       {opts.CloudConnectionKey, "", catalog.ConflictRegionInvalid},
		"bad region":      {opts.CloudConnectionKey, "mars-1", catalog.ConflictRegionInvalid},
	}
	for name, tc := range cases {
		def := awsDraft("pg-conflict", tc.account, tc.region, resource.Criterion{ResourceID: "db"})
		got := runPreview(t, svc, opts, previewQuery(opts, def, "db", "default"))
		if got.Status != catalog.PreviewConnectionConflict || got.Winner == nil || got.Winner.Key != "pg-conflict" ||
			got.ConnectionConflict == nil || got.ConnectionConflict.Reason != tc.reason || got.ConnectionConflict.DefinitionKey != "pg-conflict" {
			t.Fatalf("%s: %+v", name, got)
		}
	}
	loser := awsDraft("pg-loser", "gone", "us-east-1", resource.Criterion{ResourceID: "other"})
	got := runPreview(t, svc, opts, previewQuery(opts, loser, "db", "default"))
	if got.Status != catalog.PreviewMatched || got.ConnectionConflict != nil || got.Winner.Key != "postgres-internal-statefulset" {
		t.Fatalf("loser conflict blocked the winner: %+v", got)
	}
}

// A Kubernetes binding shows where its cluster comes from; the legacy
// Environment binding is labelled, never an account.
func TestPreviewMatching_KubernetesWinnerShowsInheritedClusterSource(t *testing.T) {
	_, svc, opts := seededCatalog(t, nil)
	for name, tc := range map[string]struct {
		binding map[string]any
		want    catalog.MatchingClusterSource
	}{
		"resource":           {map[string]any{"target": "resource", "resource_ref": "k8s-cluster.eks#environments.@app.@env"}, catalog.MatchingClusterSource{Kind: catalog.ClusterSourceResource, Reference: "k8s-cluster.eks#environments.@app.@env"}},
		"legacy environment": {map[string]any{"target": "environment"}, catalog.MatchingClusterSource{Kind: catalog.ClusterSourceLegacyEnvironment}},
	} {
		def := catalog.DefinitionDraft{Key: "pg-k8s", Name: "PG k8s", ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform,
			DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{}, "provider_bindings": map[string]any{"kubernetes": tc.binding}}},
			Criteria:     []resource.Criterion{{ResourceID: "db"}}}
		got := runPreview(t, svc, opts, previewQuery(opts, def, "db", "default"))
		if got.Status != catalog.PreviewMatched || got.Winner.Account != nil || got.Winner.Region != "" || got.Winner.ClusterSource == nil || *got.Winner.ClusterSource != tc.want {
			t.Fatalf("%s: %+v", name, got)
		}
	}
}

func TestPreviewMatching_SameKeyReplacementIsInMemoryOnly(t *testing.T) {
	st, svc, opts := seededCatalog(t, nil)
	ctx := context.Background()
	before, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	envBefore, _ := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	def := previewCandidate("postgres-internal-statefulset", resource.Criterion{Class: "never"})
	got := runPreview(t, svc, opts, previewQuery(opts, def, "db", "default"))
	if got.Status != catalog.PreviewNoMatch {
		t.Fatalf("replaced criteria should stop matching: %+v", got)
	}
	n := 0
	for _, c := range got.Candidates {
		if c.Key == "postgres-internal-statefulset" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("same key listed %d times", n)
	}
	after, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	envAfter, _ := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(envBefore, envAfter) {
		t.Fatal("preview mutated the store")
	}
	// A new key is not inserted either.
	runPreview(t, svc, opts, previewQuery(opts, previewCandidate("brand-new", resource.Criterion{}), "db", "default"))
	after, _ = st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	if len(after) != len(before) {
		t.Fatal("candidate was inserted")
	}
}

func TestPreviewMatching_MarkerIsDeterministicAndCatalogSensitive(t *testing.T) {
	st, svc, opts := seededCatalog(t, nil)
	a := runPreview(t, svc, opts, previewQuery(opts, previewCandidate("a", resource.Criterion{}), "db", "default"))
	b := runPreview(t, svc, opts, previewQuery(opts, previewCandidate("b", resource.Criterion{Class: "x"}), "other", "x"))
	if a.Snapshot != b.Snapshot {
		t.Fatalf("candidate/context must not change marker: %+v %+v", a.Snapshot, b.Snapshot)
	}
	later := previewCandidate("later", resource.Criterion{ResourceID: "z"}).Definition()
	later.Version = 1
	if err := st.CreateResourceDefinition(context.Background(), opts.OrganizationKey, later); err != nil {
		t.Fatal(err)
	}
	c := runPreview(t, svc, opts, previewQuery(opts, previewCandidate("a", resource.Criterion{}), "db", "default"))
	if c.Snapshot.Marker == a.Snapshot.Marker {
		t.Fatal("catalog change must change marker")
	}
	// An account change (here a new version) changes the marker as well.
	account, _ := st.GetConnection(context.Background(), opts.OrganizationKey, opts.CloudConnectionKey)
	account.Version++
	if err := st.SaveConnection(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	d := runPreview(t, svc, opts, previewQuery(opts, previewCandidate("a", resource.Criterion{}), "db", "default"))
	if d.Snapshot.Marker == c.Snapshot.Marker {
		t.Fatal("account change must change marker")
	}
}

func TestPreviewMatching_ScopingAndTarget(t *testing.T) {
	_, svc, opts := seededCatalog(t, nil)
	ctx := context.Background()
	q := previewQuery(opts, previewCandidate("pg", resource.Criterion{}), "db", "default")
	if _, err := svc.PreviewMatching(ctx, "other-org", q); !errors.Is(err, catalog.ErrPreviewNotFound) {
		t.Fatalf("other org: %v", err)
	}
	q.Context.EnvironmentID = "missing"
	if _, err := svc.PreviewMatching(ctx, opts.OrganizationKey, q); !errors.Is(err, catalog.ErrPreviewNotFound) {
		t.Fatalf("missing env: %v", err)
	}
	q.Context.ApplicationID = "missing"
	if _, err := svc.PreviewMatching(ctx, opts.OrganizationKey, q); !errors.Is(err, catalog.ErrPreviewNotFound) {
		t.Fatalf("missing app: %v", err)
	}
	q.Context = catalog.MatchingContext{}
	if _, err := svc.PreviewMatching(ctx, opts.OrganizationKey, q); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("empty scope: %v", err)
	}
}

// UC-03 V02-05: ordinary matching needs no configured Environment target, and
// the server (not the client) supplies env_type.
func TestPreviewMatching_WorksForUnconfiguredEnvironmentWithServerEnvType(t *testing.T) {
	st, svc, opts := seededCatalog(t, nil)
	ctx := context.Background()
	env, err := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	env.Key, env.ID, env.ConnectionKey, env.Profile, env.Type = "unbound", "unbound-id", "", "", "qa"
	if err := st.SaveEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	q := previewQuery(opts, previewCandidate("pg-qa", resource.Criterion{EnvironmentType: "qa"}), "db", "default")
	q.Context.EnvironmentID = "unbound"
	got, err := svc.PreviewMatching(ctx, opts.OrganizationKey, q)
	if err != nil {
		t.Fatalf("unbound env: %v", err)
	}
	c, _ := candidateByKey(got, "pg-qa")
	if got.Status == catalog.PreviewIncomplete || !c.Matched || *c.Score != 1 {
		t.Fatalf("env_type from the server snapshot did not apply: %+v", got)
	}
	// No system node is created for an unconfigured Environment.
	cluster := catalog.DefinitionDraft{Key: "cluster-x", Name: "x", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{Class: "internal"}}}
	q = previewQuery(opts, cluster, "connections."+opts.ConnectionKey, "internal")
	q.Context.EnvironmentID = "unbound"
	got, err = svc.PreviewMatching(ctx, opts.OrganizationKey, q)
	if err != nil || got.Status == catalog.PreviewSystemBinding {
		t.Fatalf("preview must not invent the legacy system node: %+v %v", got, err)
	}
}

func TestPreviewMatching_ReservedAndPublicExistingClusterRejected(t *testing.T) {
	_, svc, opts := seededCatalog(t, nil)
	ctx := context.Background()
	cases := map[string]catalog.DefinitionDraft{
		"builtin key":      {Key: "builtin-existing-cluster", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{}}},
		"existing-cluster": {Key: "any-key", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster, Criteria: []resource.Criterion{{}}},
		"unknown type":     {Key: "x", ResourceTypeKey: "nope", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{}}},
		"no criteria":      {Key: "x", ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform},
		"control name":     {Key: "x", Name: "a\nb", ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{}}},
		"bad id":           {Key: "Bad_Key", ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{}}},
		"too many":         {Key: "x", ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform, Criteria: make([]resource.Criterion, 65)},
		"long criterion":   {Key: "x", ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{Class: strings.Repeat("a", 300)}}},
	}
	for name, def := range cases {
		_, err := svc.PreviewMatching(ctx, opts.OrganizationKey, previewQuery(opts, def, "db", "default"))
		if !errors.Is(err, catalog.ErrInvalid) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestPreviewMatching_SystemBindingOnlyForExactImplicitClusterNode(t *testing.T) {
	_, svc, opts := seededCatalog(t, nil)
	cluster := catalog.DefinitionDraft{Key: "cluster-x", Name: "cluster-x", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{Class: "internal"}}}
	exact := "connections." + opts.ConnectionKey
	got := runPreview(t, svc, opts, previewQuery(opts, cluster, exact, "internal"))
	if got.Status != catalog.PreviewSystemBinding || got.Winner != nil || len(got.Ties) != 0 {
		t.Fatalf("exact: %+v", got)
	}
	for name, ctx := range map[string][2]string{"other id": {"connections.other", "internal"}, "other class": {exact, "default"}} {
		got = runPreview(t, svc, opts, previewQuery(opts, cluster, ctx[0], ctx[1]))
		if got.Status == catalog.PreviewSystemBinding {
			t.Fatalf("%s must not be a system binding: %+v", name, got)
		}
	}
}

func TestPreviewMatching_WorkloadOnlyRanksScoreK8sAndNeverFallsBackToNative(t *testing.T) {
	_, svc, opts := seededCatalog(t, nil)
	k8s := catalog.DefinitionDraft{Key: "wl-k8s", Name: "wl-k8s", ResourceTypeKey: "workload", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{}}}
	got := runPreview(t, svc, opts, previewQuery(opts, k8s, "modules.web", "default"))
	if got.Status != catalog.PreviewNoMatch || got.Winner != nil {
		t.Fatalf("non-score-k8s must not rank: %+v", got)
	}
	if _, ok := candidateByKey(got, "wl-k8s"); ok {
		t.Fatal("non-score-k8s workload definition must be excluded from ranking")
	}
	score := catalog.DefinitionDraft{Key: "wl-score", Name: "wl-score", ResourceTypeKey: "workload", DriverType: resource.DriverScoreK8s, Criteria: []resource.Criterion{{}}}
	got = runPreview(t, svc, opts, previewQuery(opts, score, "modules.web", "default"))
	if got.Status != catalog.PreviewMatched || got.Winner.Key != "wl-score" {
		t.Fatalf("score-k8s: %+v", got)
	}
}

// The Environment connection's state never gates ordinary matching: only the
// scoped catalog, the account metadata and the context do (UC-03 V02-05).
func TestPreviewMatching_IgnoresEnvironmentConnectionState(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(*environment.Environment){
		"kind inconsistent with profile": func(e *environment.Environment) { e.ConnectionKey = "aws-account" },
		"invalid scope":                  func(e *environment.Environment) { e.InfrastructureScope = "BOGUS" },
		"failed runtime":                 func(e *environment.Environment) { e.RuntimeStatus = application.RuntimeStatus("FAILED") },
		"aws without region": func(e *environment.Environment) {
			e.Profile, e.Region, e.ConnectionKey = application.ProfileAWSEKS, "", "aws-account"
		},
	} {
		st, _, opts := seededCatalog(t, nil)
		svc := catalog.NewService(envMutatingStore{st, mutate})
		got, err := svc.PreviewMatching(ctx, opts.OrganizationKey, previewQuery(opts, previewCandidate("pg", resource.Criterion{}), "db", "default"))
		if err != nil || got.Status == "" {
			t.Fatalf("%s: %+v %v", name, got, err)
		}
	}
	st, svc, opts := seededCatalog(t, nil)
	c, _ := st.GetConnection(ctx, opts.OrganizationKey, opts.ConnectionKey)
	c.Status = application.ConnectionVerifying
	_ = st.SaveConnection(ctx, c)
	if _, err := svc.PreviewMatching(ctx, opts.OrganizationKey, previewQuery(opts, previewCandidate("pg", resource.Criterion{}), "db", "default")); err != nil {
		t.Fatalf("connection not ready blocked ordinary matching: %v", err)
	}
}

// reversingStore returns catalog lists in reverse order inside ReadSnapshot.
type reversingStore struct{ persistence.Store }

type reversingView struct{ persistence.Store }

func (s reversingStore) ReadSnapshot(ctx context.Context, fn func(context.Context, persistence.Store) error) error {
	return s.Store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error { return fn(ctx, reversingView{view}) })
}
func (v reversingView) ListResourceDefinitions(ctx context.Context, org string) ([]resource.Definition, error) {
	list, err := v.Store.ListResourceDefinitions(ctx, org)
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	return list, err
}
func (v reversingView) ListResourceTypes(ctx context.Context, org string) ([]resource.Type, error) {
	list, err := v.Store.ListResourceTypes(ctx, org)
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	return list, err
}

func TestPreviewMatching_MarkerIgnoresRepositoryListOrder(t *testing.T) {
	st, svc, opts := seededCatalog(t, nil)
	q := previewQuery(opts, previewCandidate("a", resource.Criterion{}), "db", "default")
	normal := runPreview(t, svc, opts, q)
	reversed := runPreview(t, catalog.NewService(reversingStore{st}), opts, q)
	if normal.Snapshot != reversed.Snapshot || !reflect.DeepEqual(normal.Candidates, reversed.Candidates) {
		t.Fatalf("order changed result: %+v vs %+v", normal.Snapshot, reversed.Snapshot)
	}
}

func TestPreviewMatching_ValidationErrorNeverEchoesSubmittedValues(t *testing.T) {
	_, svc, opts := seededCatalog(t, nil)
	const secret = "SENTINEL-s3cret"
	bad := []catalog.DefinitionDraft{
		{Key: secret + "_BAD", ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{}}},
		{Key: "ok", ResourceTypeKey: secret, DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{}}},
		{Key: "ok", ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform, DriverAccount: secret + strings.Repeat("x", 300), Criteria: []resource.Criterion{{}}},
		{Key: "ok", ResourceTypeKey: "postgres", DriverType: resource.DriverType(secret), Criteria: []resource.Criterion{{}}},
		{Key: "ok", ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{Class: secret + strings.Repeat("x", 300)}}},
	}
	for i, def := range bad {
		_, err := svc.PreviewMatching(context.Background(), opts.OrganizationKey, previewQuery(opts, def, "db", "default"))
		var invalid *catalog.PreviewInvalidError
		if !errors.As(err, &invalid) || !errors.Is(err, catalog.ErrInvalid) || strings.Contains(err.Error(), secret) || invalid.Field == "" {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}

func TestPreviewMatching_SystemBindingDoesNotShowAuthoredMatches(t *testing.T) {
	_, svc, opts := seededCatalog(t, nil)
	cluster := catalog.DefinitionDraft{Key: "cluster-x", Name: "cluster-x", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{Class: "internal"}}}
	got := runPreview(t, svc, opts, previewQuery(opts, cluster, "connections."+opts.ConnectionKey, "internal"))
	for _, c := range got.Candidates {
		if c.Matched || c.Score != nil || c.BestCriterion != nil {
			t.Fatalf("system binding must not present authored matches: %+v", c)
		}
	}
}

func rebind(t *testing.T, st persistence.Store, opts seed.Options, mutate func(*persistence.EnvironmentBinding)) {
	t.Helper()
	ctx := context.Background()
	env, err := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	b := persistence.EnvironmentBinding{ApplicationKey: env.ApplicationKey, EnvironmentKey: env.Key, ConnectionKey: env.ConnectionKey, Profile: env.Profile,
		Region: env.Region, RuntimeStatus: env.RuntimeStatus, Scope: env.Scope(), Generation: env.TargetGeneration, ExpectedVersion: env.Version}
	mutate(&b)
	if _, err := st.BindEnvironment(ctx, b); err != nil {
		t.Fatal(err)
	}
}

type envMutatingStore struct {
	persistence.Store
	mutate func(*environment.Environment)
}
type envMutatingView struct {
	persistence.Store
	mutate func(*environment.Environment)
}

func (s envMutatingStore) ReadSnapshot(ctx context.Context, fn func(context.Context, persistence.Store) error) error {
	return s.Store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		return fn(ctx, envMutatingView{view, s.mutate})
	})
}
func (v envMutatingView) GetEnvironment(ctx context.Context, app, env string) (environment.Environment, error) {
	e, err := v.Store.GetEnvironment(ctx, app, env)
	v.mutate(&e)
	return e, err
}

// readSpy allows only the five snapshot reads PreviewMatching needs. The
// embedded Store is nil, so any other call (Transact, Create*, Save*, other
// reads) panics and fails the test.
type readSpy struct {
	persistence.Store
	real      persistence.Store
	snapshots int
	reads     []string
}
type readSpyView struct {
	persistence.Store
	spy *readSpy
	in  persistence.Store
}

func (s *readSpy) ReadSnapshot(ctx context.Context, fn func(context.Context, persistence.Store) error) error {
	s.snapshots++
	return s.real.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		return fn(ctx, readSpyView{spy: s, in: view})
	})
}
func (v readSpyView) GetApplication(ctx context.Context, key string) (application.Application, error) {
	v.spy.reads = append(v.spy.reads, "GetApplication")
	return v.in.GetApplication(ctx, key)
}
func (v readSpyView) GetEnvironment(ctx context.Context, a, e string) (environment.Environment, error) {
	v.spy.reads = append(v.spy.reads, "GetEnvironment")
	return v.in.GetEnvironment(ctx, a, e)
}
func (v readSpyView) ListConnections(ctx context.Context, o string) ([]application.Connection, error) {
	v.spy.reads = append(v.spy.reads, "ListConnections")
	return v.in.ListConnections(ctx, o)
}
func (v readSpyView) ListResourceTypes(ctx context.Context, o string) ([]resource.Type, error) {
	v.spy.reads = append(v.spy.reads, "ListResourceTypes")
	return v.in.ListResourceTypes(ctx, o)
}
func (v readSpyView) ListResourceDefinitions(ctx context.Context, o string) ([]resource.Definition, error) {
	v.spy.reads = append(v.spy.reads, "ListResourceDefinitions")
	return v.in.ListResourceDefinitions(ctx, o)
}

// noInspectorUse fails the test if the module registry (fetch, metadata or
// execution verification) is ever used.
type noInspectorUse struct{ t *testing.T }

func (f noInspectorUse) Verify(context.Context, tfmodule.SourceRef) (tfmodule.Bundle, error) {
	f.t.Fatal("module registry must not be called by matching preview")
	return tfmodule.Bundle{}, nil
}
func (f noInspectorUse) Bundle(context.Context, tfmodule.SourceRef) (tfmodule.Bundle, error) {
	f.t.Fatal("module registry used")
	return tfmodule.Bundle{}, nil
}
func (f noInspectorUse) List(context.Context) ([]tfmodule.Bundle, error) {
	f.t.Fatal("module registry used")
	return nil, nil
}
func (f noInspectorUse) VerifyHistoricalForExecution(context.Context, tfmodule.Bundle) (tfmodule.VerifiedArtifact, error) {
	f.t.Fatal("historical verification must not run in catalog tests")
	return tfmodule.VerifiedArtifact{}, nil
}

func (f noInspectorUse) VerifyForExecution(context.Context, tfmodule.SourceRef) (tfmodule.VerifiedArtifact, error) {
	f.t.Fatal("module registry used")
	return tfmodule.VerifiedArtifact{}, nil
}

func TestPreviewMatching_UsesExactlyOneSnapshotAndNoWrites(t *testing.T) {
	st, _, opts := seededCatalog(t, nil)
	allowed := map[string]bool{"GetApplication": true, "GetEnvironment": true, "ListConnections": true, "ListResourceTypes": true, "ListResourceDefinitions": true}
	scenarios := map[string]struct {
		q       catalog.MatchingPreviewQuery
		org     string
		wantErr bool
	}{
		"success":    {q: previewQuery(opts, previewCandidate("pg", resource.Criterion{}), "db", "default"), org: opts.OrganizationKey},
		"incomplete": {q: previewQuery(opts, previewCandidate("pg", resource.Criterion{}), "", ""), org: opts.OrganizationKey},
		"not found":  {q: previewQuery(opts, previewCandidate("pg", resource.Criterion{}), "db", "default"), org: "other-org", wantErr: true},
		"invalid":    {q: previewQuery(opts, catalog.DefinitionDraft{Key: "builtin-existing-cluster", Name: "builtin-existing-cluster", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{}}}, "db", "default"), org: opts.OrganizationKey, wantErr: true},
		"system":     {q: previewQuery(opts, catalog.DefinitionDraft{Key: "c", Name: "c", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{Class: "internal"}}}, "connections."+opts.ConnectionKey, "internal"), org: opts.OrganizationKey},
	}
	for name, sc := range scenarios {
		spy := &readSpy{real: st}
		svc := catalog.NewService(spy, noInspectorUse{t})
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s: call outside the allowed snapshot reads: %v", name, r)
				}
			}()
			_, err := svc.PreviewMatching(context.Background(), sc.org, sc.q)
			if (err != nil) != sc.wantErr {
				t.Fatalf("%s: err=%v", name, err)
			}
		}()
		for _, read := range spy.reads {
			if !allowed[read] {
				t.Fatalf("%s: unexpected read %s", name, read)
			}
		}
		// Every scenario here passes context validation, so exactly one snapshot is used.
		if spy.snapshots != 1 {
			t.Fatalf("%s: snapshots=%d reads=%v", name, spy.snapshots, spy.reads)
		}
	}
}

func TestPreviewMatching_AllFiveCriteriaScoreThirtyOne(t *testing.T) {
	_, svc, opts := seededCatalog(t, nil)
	full := resource.Criterion{EnvironmentType: opts.EnvironmentType, ApplicationID: opts.ApplicationKey, EnvironmentID: opts.EnvironmentKey, ResourceID: "db", Class: "fast"}
	got := runPreview(t, svc, opts, previewQuery(opts, previewCandidate("pg-all", full, resource.Criterion{Class: "fast"}), "db", "fast"))
	if got.Status != catalog.PreviewMatched || got.Winner.Score != 31 {
		t.Fatalf("got %+v", got)
	}
	c, _ := candidateByKey(got, "pg-all")
	if *c.Score != 31 || *c.BestCriterion != full {
		t.Fatalf("candidate %+v", c)
	}
}

func TestPreviewMatching_WhitespaceOnlyContext(t *testing.T) {
	_, svc, opts := seededCatalog(t, nil)
	ctx := context.Background()
	for name, c := range map[string][2]string{"resource": {"  ", "default"}, "class": {"db", "   "}, "both": {" ", " "}} {
		got := runPreview(t, svc, opts, previewQuery(opts, previewCandidate("pg-wild", resource.Criterion{}), c[0], c[1]))
		if got.Status != catalog.PreviewIncomplete || got.Winner != nil || len(got.Ties) != 0 || len(got.MissingFields) == 0 {
			t.Fatalf("%s: %+v", name, got)
		}
	}
	for name, mutate := range map[string]func(*catalog.MatchingContext){
		"app": func(c *catalog.MatchingContext) { c.ApplicationID = "  " },
		"env": func(c *catalog.MatchingContext) { c.EnvironmentID = "   " },
	} {
		q := previewQuery(opts, previewCandidate("pg", resource.Criterion{}), "db", "default")
		mutate(&q.Context)
		if _, err := svc.PreviewMatching(ctx, opts.OrganizationKey, q); !errors.Is(err, catalog.ErrInvalid) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// Nonblank technical IDs are never trimmed: a padded ID does not match its unpadded criterion.
	got := runPreview(t, svc, opts, previewQuery(opts, previewCandidate("pg-db", resource.Criterion{ResourceID: "db"}), " db", "default"))
	c, _ := candidateByKey(got, "pg-db")
	if c.Matched {
		t.Fatalf("padded resourceId must not be normalized: %+v", c)
	}
}

// The legacy system binding is claimed only when its backing Driver Account is
// a READY Kubernetes account; otherwise ordinary matching continues.
func TestPreviewMatching_SystemBindingNeedsReadyKubernetesBackingAccount(t *testing.T) {
	cluster := catalog.DefinitionDraft{Key: "cluster-x", Name: "cluster-x", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{Class: "internal"}}}
	for name, mutate := range map[string]func(*application.Connection){
		"not ready": func(c *application.Connection) { c.Status = application.ConnectionVerifying },
		"wrong kind": func(c *application.Connection) {
			c.Kind, c.AuthenticationType = application.ConnectionAWS, application.AuthAWSAccessKey
		},
	} {
		t.Run(name, func(t *testing.T) {
			st, svc, opts := seededCatalog(t, nil)
			ctx := context.Background()
			list, err := st.ListConnections(ctx, opts.OrganizationKey)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range list {
				if c.Key == opts.ConnectionKey {
					mutate(&c)
					if err := st.SaveConnection(ctx, c); err != nil {
						t.Fatal(err)
					}
				}
			}
			got := runPreview(t, svc, opts, previewQuery(opts, cluster, "connections."+opts.ConnectionKey, "internal"))
			if got.Status == catalog.PreviewSystemBinding {
				t.Fatalf("%s: unusable backing account claimed as system binding: %+v", name, got)
			}
		})
	}
}

// Preview of an existing key keeps the stored version in the winner, and a
// long multibyte Name that registration accepts is accepted by the preview.
func TestPreviewMatching_ExistingVersionAndMultibyteName(t *testing.T) {
	st, svc, opts := seededCatalog(t, nil)
	ctx := context.Background()
	stored, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	var existing resource.Definition
	for _, d := range stored {
		if d.Key == "postgres-internal-statefulset" {
			existing = d
		}
	}
	if existing.Version == 0 {
		t.Fatal("seed must carry a stored version")
	}
	got := runPreview(t, svc, opts, previewQuery(opts, previewCandidate(existing.Key, resource.Criterion{}), "db", "default"))
	if got.Winner == nil || got.Winner.Key != existing.Key || got.Winner.Version != existing.Version {
		t.Fatalf("winner lost the stored version %d: %+v", existing.Version, got.Winner)
	}
	long := previewCandidate("long-name", resource.Criterion{})
	long.Name = strings.Repeat("Ă", resource.MaxDefinitionNameLength)
	if _, err := svc.PreviewMatching(ctx, opts.OrganizationKey, previewQuery(opts, long, "db", "default")); err != nil {
		t.Fatalf("a valid 253-character Name must preview: %v", err)
	}
	long.Name += "x"
	if _, err := svc.PreviewMatching(ctx, opts.OrganizationKey, previewQuery(opts, long, "db", "default")); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("an overlong Name: %v", err)
	}
}
