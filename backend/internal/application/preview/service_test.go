package preview_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	tf "orchestrator/internal/adapters/terraform"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/preview"
	"orchestrator/internal/bootstrap"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

// counters are shared by a guard and every snapshot view it hands out.
type counters struct {
	writes, directReads, snapshots atomic.Int32
}

// guard wraps a Store: it counts writes (also through snapshot views),
// counts planning reads made outside ReadSnapshot and can run a hook after
// the snapshot view is taken.
type guard struct {
	persistence.Store
	c             *counters
	inView        bool
	afterSnapshot func()
	// midSnapshot runs once inside the snapshot, after its first read, so a
	// lazily established snapshot (PostgreSQL REPEATABLE READ) is already fixed.
	midSnapshot func()
}

func newGuard(st persistence.Store) *guard { return &guard{Store: st, c: &counters{}} }

// ReadSnapshot wraps the adapter view in a guard that shares the counters,
// so a write attempted through the view is still counted. It never calls
// ReadSnapshot recursively.
func (g *guard) ReadSnapshot(ctx context.Context, fn func(context.Context, persistence.Store) error) error {
	g.c.snapshots.Add(1)
	return g.Store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		if g.afterSnapshot != nil {
			g.afterSnapshot()
		}
		return fn(ctx, &guard{Store: view, c: g.c, inView: true, midSnapshot: g.midSnapshot})
	})
}

func (g *guard) read() {
	if !g.inView {
		g.c.directReads.Add(1)
	}
}
func (g *guard) write() { g.c.writes.Add(1) }

func (g *guard) GetApplication(ctx context.Context, key string) (appdomain.Application, error) {
	g.read()
	app, err := g.Store.GetApplication(ctx, key)
	if g.inView && g.midSnapshot != nil {
		g.midSnapshot()
		g.midSnapshot = nil
	}
	return app, err
}
func (g *guard) GetEnvironment(ctx context.Context, app, env string) (environment.Environment, error) {
	g.read()
	return g.Store.GetEnvironment(ctx, app, env)
}
func (g *guard) GetConnection(ctx context.Context, org, key string) (appdomain.Connection, error) {
	g.read()
	return g.Store.GetConnection(ctx, org, key)
}
func (g *guard) GetDeploymentSet(ctx context.Context, id string) (environment.DeploymentSet, error) {
	g.read()
	return g.Store.GetDeploymentSet(ctx, id)
}
func (g *guard) ListResourceTypes(ctx context.Context, org string) ([]resource.Type, error) {
	g.read()
	return g.Store.ListResourceTypes(ctx, org)
}
func (g *guard) ListResourceDefinitions(ctx context.Context, org string) ([]resource.Definition, error) {
	g.read()
	return g.Store.ListResourceDefinitions(ctx, org)
}
func (g *guard) ListActiveResources(ctx context.Context, org string) ([]resource.ActiveResource, error) {
	g.read()
	return g.Store.ListActiveResources(ctx, org)
}

func (g *guard) SaveOrganization(ctx context.Context, v appdomain.Organization) error {
	g.write()
	return g.Store.SaveOrganization(ctx, v)
}
func (g *guard) SaveApplication(ctx context.Context, v appdomain.Application) error {
	g.write()
	return g.Store.SaveApplication(ctx, v)
}
func (g *guard) SaveConnection(ctx context.Context, v appdomain.Connection) error {
	g.write()
	return g.Store.SaveConnection(ctx, v)
}
func (g *guard) SaveUserAccount(ctx context.Context, v identity.UserAccount) error {
	g.write()
	return g.Store.SaveUserAccount(ctx, v)
}
func (g *guard) SaveSession(ctx context.Context, v identity.Session) error {
	g.write()
	return g.Store.SaveSession(ctx, v)
}
func (g *guard) SaveEnvironment(ctx context.Context, v environment.Environment) error {
	g.write()
	return g.Store.SaveEnvironment(ctx, v)
}
func (g *guard) SaveDeploymentSet(ctx context.Context, v environment.DeploymentSet) error {
	g.write()
	return g.Store.SaveDeploymentSet(ctx, v)
}
func (g *guard) CompareVersionAndSetCurrent(ctx context.Context, app, env string, version int64, setID string) error {
	g.write()
	return g.Store.CompareVersionAndSetCurrent(ctx, app, env, version, setID)
}
func (g *guard) SaveResourceType(ctx context.Context, org string, v resource.Type) error {
	g.write()
	return g.Store.SaveResourceType(ctx, org, v)
}
func (g *guard) SaveResourceDefinition(ctx context.Context, org string, v resource.Definition) error {
	g.write()
	return g.Store.SaveResourceDefinition(ctx, org, v)
}
func (g *guard) SaveDeployment(ctx context.Context, v domain.Deployment) error {
	g.write()
	return g.Store.SaveDeployment(ctx, v)
}
func (g *guard) SavePlan(ctx context.Context, id string, v map[string]any) error {
	g.write()
	return g.Store.SavePlan(ctx, id, v)
}
func (g *guard) SaveDeploymentResource(ctx context.Context, v domain.Resource) error {
	g.write()
	return g.Store.SaveDeploymentResource(ctx, v)
}
func (g *guard) SaveDeltaSnapshot(ctx context.Context, v domain.DeploymentDeltaSnapshot) error {
	g.write()
	return g.Store.SaveDeltaSnapshot(ctx, v)
}
func (g *guard) UpsertActiveResource(ctx context.Context, v resource.ActiveResource) (resource.ActiveResource, error) {
	g.write()
	return g.Store.UpsertActiveResource(ctx, v)
}
func (g *guard) UpsertWorkloadProgress(ctx context.Context, v domain.WorkloadInstance) error {
	g.write()
	return g.Store.UpsertWorkloadProgress(ctx, v)
}
func (g *guard) UpsertWorkloadInstance(ctx context.Context, v domain.WorkloadInstance) error {
	g.write()
	return g.Store.UpsertWorkloadInstance(ctx, v)
}
func (g *guard) CommitConfigurationRevision(ctx context.Context, version int64, v configuration.Revision) error {
	g.write()
	return g.Store.CommitConfigurationRevision(ctx, version, v)
}
func (g *guard) SaveWorkloadDraft(ctx context.Context, version int64, v environment.WorkloadDraft) error {
	g.write()
	return g.Store.SaveWorkloadDraft(ctx, version, v)
}
func (g *guard) DeleteWorkloadDraft(ctx context.Context, app, env, id string, version int64) error {
	g.write()
	return g.Store.DeleteWorkloadDraft(ctx, app, env, id, version)
}
func (g *guard) Transact(ctx context.Context, fn func(context.Context) error) error {
	g.write()
	return g.Store.Transact(ctx, fn)
}

const runID = "run-preview-test"

func newApp(t *testing.T, cloud bool) (*bootstrap.App, seed.Options) {
	t.Helper()
	opts := seed.Defaults()
	opts.RunID = runID
	if cloud {
		opts.Region, opts.AccountID = "us-east-1", "000000000000"
	}
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return app, opts
}

func newPreview(st persistence.Store) *preview.Service {
	return preview.NewService(st, planning.NewService(), tf.NewInspector())
}

func deploy(t *testing.T, app *bootstrap.App, opts seed.Options, id string, before, after map[string]any) *appsvc.DeployResult {
	t.Helper()
	result, err := app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{
		OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey,
		WorkloadID: id, ScoreBefore: before, ScoreAfter: after, Actor: "test", RunID: runID,
	})
	if err != nil {
		t.Fatalf("deploy %s: %v", id, err)
	}
	return result
}

func query(opts seed.Options, id, action string, before, after map[string]any) preview.PreviewDeploymentQuery {
	return preview.PreviewDeploymentQuery{
		OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey,
		WorkloadID: id, Action: action, RunID: runID, ScoreBefore: before, ScoreAfter: after,
	}
}

func scores(opts seed.Options) map[string]map[string]any { return seed.AcceptanceScores(opts) }

func mainContainer(doc map[string]any) map[string]any {
	return doc["containers"].(map[string]any)["main"].(map[string]any)
}

// fingerprint hashes every piece of state a Preview could be tempted to
// change: Environment, current set, Deployments, Active Resources, workload
// instances and drafts.
func fingerprint(t *testing.T, st persistence.Store, opts seed.Options) string {
	t.Helper()
	ctx := context.Background()
	env, err := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	set, err := st.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if err != nil {
		t.Fatal(err)
	}
	deployments, _ := st.ListDeployments(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	active, _ := st.ListActiveResources(ctx, opts.OrganizationKey)
	instances, _ := st.ListWorkloadInstances(ctx, opts.ApplicationKey+"/"+opts.EnvironmentKey)
	drafts, _ := st.ListWorkloadDrafts(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	hash, err := canon.Hash(map[string]any{"env": env, "set": set, "deployments": deployments, "active": active, "instances": instances, "drafts": drafts})
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestPreview_NoRuntimeMutation(t *testing.T) {
	app, opts := newApp(t, false)
	deploy(t, app, opts, "backend", nil, scores(opts)["backend"])
	state := fingerprint(t, app.Store, opts)
	provisions, applied := len(app.FakeExec.Calls), len(app.FakeDeploy.AppliedNames())

	st := newGuard(app.Store)
	svc := newPreview(st)
	after := scores(opts)["backend"]
	mainContainer(after)["variables"].(map[string]any)["PORT"] = "9090"
	valid := []preview.PreviewDeploymentQuery{
		query(opts, "worker", preview.ActionDeploy, nil, scores(opts)["worker"]),
		query(opts, "backend", preview.ActionUpdate, scores(opts)["backend"], after),
		query(opts, "backend", preview.ActionRemove, scores(opts)["backend"], nil),
	}
	invalid := []preview.PreviewDeploymentQuery{
		query(opts, "backend", preview.ActionDeploy, nil, scores(opts)["backend"]), // already exists
		query(opts, "worker", preview.ActionRemove, scores(opts)["worker"], nil),   // not current
		query(opts, "backend", preview.ActionUpdate, nil, scores(opts)["backend"]), // missing before
		query(opts, "frontend", preview.ActionDeploy, nil, scores(opts)["worker"]), // name mismatch
	}
	for _, q := range valid {
		if _, err := svc.PreviewDeployment(context.Background(), q); err != nil {
			t.Fatalf("%s %s: %v", q.Action, q.WorkloadID, err)
		}
	}
	for _, q := range invalid {
		if _, err := svc.PreviewDeployment(context.Background(), q); err == nil {
			t.Fatalf("%s %s: invalid preview accepted", q.Action, q.WorkloadID)
		}
	}
	if n := st.c.writes.Load(); n != 0 {
		t.Fatalf("preview attempted %d writes (including through snapshot views)", n)
	}
	if len(app.FakeExec.Calls) != provisions || len(app.FakeDeploy.AppliedNames()) != applied {
		t.Fatal("preview reached the resource executor or workload deployer")
	}
	if fingerprint(t, app.Store, opts) != state {
		t.Fatal("valid or invalid preview changed persisted state")
	}
}

func TestPreview_ReadsOneConsistentSnapshot(t *testing.T) {
	ctx := context.Background()
	app, opts := newApp(t, false)
	deploy(t, app, opts, "backend", nil, scores(opts)["backend"])
	env, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)

	st := newGuard(app.Store)
	// A concurrent writer commits after the snapshot is taken: the preview
	// must still report the state it planned from, not a mix.
	st.afterSnapshot = func() {
		moved := env
		moved.Version++
		moved.CurrentDeploymentSetID = "concurrent-set"
		if err := app.Store.SaveEnvironment(ctx, moved); err != nil {
			t.Error(err)
		}
	}
	result, err := newPreview(st).PreviewDeployment(ctx, query(opts, "worker", preview.ActionDeploy, nil, scores(opts)["worker"]))
	if err != nil {
		t.Fatal(err)
	}
	if st.c.snapshots.Load() != 1 || st.c.directReads.Load() != 0 {
		t.Fatalf("snapshots=%d directReads=%d, want 1/0", st.c.snapshots.Load(), st.c.directReads.Load())
	}
	if result.BaseVersion != env.Version || result.BaseSetID != env.CurrentDeploymentSetID {
		t.Fatalf("preview mixed snapshot state: version %d set %s", result.BaseVersion, result.BaseSetID)
	}
	if _, ok := result.Plan.CandidateSet.Modules["backend"]; !ok {
		t.Fatal("candidate lost the current module read from the snapshot")
	}
}

func TestLoadPlanningSnapshot_EmptyCurrentSet(t *testing.T) {
	ctx := context.Background()
	app, opts := newApp(t, false)
	env, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	env.CurrentDeploymentSetID = ""
	if err := app.Store.SaveEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	result, err := newPreview(app.Store).PreviewDeployment(ctx, query(opts, "frontend", preview.ActionDeploy, nil, scores(opts)["frontend"]))
	if err != nil {
		t.Fatal(err)
	}
	if result.BaseSetID != "" || len(result.Plan.BaseSet.Modules) != 0 {
		t.Fatalf("empty current set not used: %q", result.BaseSetID)
	}
}

// TestPreview_MatchesDeployPlan proves BR-01: for one snapshot and Run ID,
// Preview and Deploy produce the same plan hash, base and Delta.
func TestPreview_MatchesDeployPlan(t *testing.T) {
	ctx := context.Background()
	app, opts := newApp(t, false)
	svc := newPreview(app.Store)
	check := func(id, action string, before, after map[string]any) {
		t.Helper()
		previewed, err := svc.PreviewDeployment(ctx, query(opts, id, action, before, after))
		if err != nil {
			t.Fatalf("preview %s %s: %v", action, id, err)
		}
		result := deploy(t, app, opts, id, before, after)
		if result.PlanHash != previewed.Plan.PlanHash {
			t.Fatalf("%s %s: deploy hash %s != preview hash %s", action, id, result.PlanHash, previewed.Plan.PlanHash)
		}
		record, err := app.Store.GetDeployment(ctx, result.DeploymentID)
		if err != nil {
			t.Fatal(err)
		}
		if record.BaseEnvironmentVersion != previewed.BaseVersion || record.BaseDeploymentSetID != previewed.BaseSetID {
			t.Fatalf("%s %s: deploy base differs from preview base", action, id)
		}
		snapshot, err := app.Store.GetDeltaSnapshot(ctx, record.DeltaSnapshotID)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := canon.Hash(previewed.Plan.Delta)
		got, _ := canon.Hash(snapshot.Document)
		if want != got {
			t.Fatalf("%s %s: persisted Delta differs from preview Delta", action, id)
		}
	}
	check("backend", preview.ActionDeploy, nil, scores(opts)["backend"])
	check("worker", preview.ActionDeploy, nil, scores(opts)["worker"])
	after := scores(opts)["backend"]
	mainContainer(after)["resources"].(map[string]any)["limits"].(map[string]any)["memory"] = "512Mi"
	check("backend", preview.ActionUpdate, scores(opts)["backend"], after)
	check("worker", preview.ActionRemove, scores(opts)["worker"], nil)
}

func TestPreview_DeltaInvariantAndContainerResources(t *testing.T) {
	ctx := context.Background()
	app, opts := newApp(t, false)
	deploy(t, app, opts, "backend", nil, scores(opts)["backend"])
	after := scores(opts)["backend"]
	mainContainer(after)["resources"].(map[string]any)["requests"].(map[string]any)["cpu"] = "75m"
	result, err := newPreview(app.Store).PreviewDeployment(ctx, query(opts, "backend", preview.ActionUpdate, scores(opts)["backend"], after))
	if err != nil {
		t.Fatal(err)
	}
	if err := planning.VerifyDelta(result.Plan.BaseSet, result.Plan.Delta, result.Plan.CandidateSet); err != nil {
		t.Fatalf("base + Delta != Candidate: %v", err)
	}
	patches := result.Plan.Delta.Modules.Update["backend"]
	if len(patches) != 1 || patches[0].Path != "/spec/containers/main/resources/requests/cpu" {
		t.Fatalf("want one module-relative patch, got %+v", patches)
	}
	res := result.Plan.CandidateSet.Modules["backend"].Spec.Containers["main"].Resources
	if res == nil || res.Requests == nil || res.Requests.CPU != "75m" || res.Limits == nil || res.Limits.Memory != "256Mi" {
		t.Fatalf("container resources not preserved: %+v", res)
	}
}

func TestPreview_NoChangeHasEmptyDelta(t *testing.T) {
	ctx := context.Background()
	app, opts := newApp(t, false)
	deploy(t, app, opts, "backend", nil, scores(opts)["backend"])
	result, err := newPreview(app.Store).PreviewDeployment(ctx, query(opts, "backend", preview.ActionUpdate, scores(opts)["backend"], scores(opts)["backend"]))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Plan.Delta.IsEmpty() || len(result.Plan.Graph.Nodes) == 0 {
		t.Fatalf("no-change preview: delta=%+v nodes=%d", result.Plan.Delta, len(result.Plan.Graph.Nodes))
	}
}

func TestPreview_StrictQuery(t *testing.T) {
	ctx := context.Background()
	app, opts := newApp(t, false)
	deploy(t, app, opts, "backend", nil, scores(opts)["backend"])
	svc := newPreview(app.Store)
	b := scores(opts)["backend"]
	withRun := func(q preview.PreviewDeploymentQuery, run string) preview.PreviewDeploymentQuery {
		q.RunID = run
		return q
	}
	for name, q := range map[string]preview.PreviewDeploymentQuery{
		"missing workload":      query(opts, "", preview.ActionDeploy, nil, b),
		"blank workload":        query(opts, "  \t", preview.ActionDeploy, nil, b),
		"missing run":           withRun(query(opts, "backend", preview.ActionUpdate, b, b), ""),
		"blank run":             withRun(query(opts, "backend", preview.ActionUpdate, b, b), "   "),
		"uppercase action":      query(opts, "worker", "DEPLOY", nil, scores(opts)["worker"]),
		"unknown action":        query(opts, "worker", "apply", nil, scores(opts)["worker"]),
		"deploy with before":    query(opts, "backend", preview.ActionDeploy, b, b),
		"deploy without after":  query(opts, "worker", preview.ActionDeploy, nil, nil),
		"update without before": query(opts, "backend", preview.ActionUpdate, nil, b),
		"update without after":  query(opts, "backend", preview.ActionUpdate, b, nil),
		"remove with after":     query(opts, "backend", preview.ActionRemove, b, b),
		"remove without before": query(opts, "backend", preview.ActionRemove, nil, nil),
		"name mismatch after":   query(opts, "api", preview.ActionDeploy, nil, scores(opts)["worker"]),
		"name mismatch before":  query(opts, "api", preview.ActionRemove, b, nil),
	} {
		if _, err := svc.PreviewDeployment(ctx, q); !errors.Is(err, preview.ErrInvalidRequest) {
			t.Errorf("%s: want ErrInvalidRequest, got %v", name, err)
		}
	}
	badResources := scores(opts)["worker"]
	mainContainer(badResources)["resources"] = map[string]any{"requests": map[string]any{"cpu": 1}}
	for name, tc := range map[string]struct {
		q    preview.PreviewDeploymentQuery
		want string
	}{
		"deploy existing":  {query(opts, "backend", preview.ActionDeploy, nil, b), "already exists in the current Deployment Set"},
		"stale before":     {query(opts, "worker", preview.ActionRemove, scores(opts)["worker"], nil), "is not part of the current Deployment Set"},
		"invalid resource": {query(opts, "worker", preview.ActionDeploy, nil, badResources), `scoreAfter: score:`},
	} {
		_, err := svc.PreviewDeployment(ctx, tc.q)
		if !errors.Is(err, preview.ErrInvalidScore) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want actionable ErrInvalidScore containing %q, got %v", name, tc.want, err)
		}
	}
}

func TestPreview_ScopeAndConnection(t *testing.T) {
	ctx := context.Background()
	app, opts := newApp(t, false)
	svc := newPreview(app.Store)
	q := query(opts, "worker", preview.ActionDeploy, nil, scores(opts)["worker"])
	q.OrganizationKey = "globex"
	if _, err := svc.PreviewDeployment(ctx, q); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign organization: %v", err)
	}
	q = query(opts, "worker", preview.ActionDeploy, nil, scores(opts)["worker"])
	q.EnvironmentKey = "missing"
	if _, err := svc.PreviewDeployment(ctx, q); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("missing environment: %v", err)
	}
	a, _ := app.Store.GetApplication(ctx, opts.ApplicationKey)
	e, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	conn, _ := app.Store.GetConnection(ctx, a.OrganizationKey, e.ConnectionKey)
	conn.Status = appdomain.ConnectionVerifying
	if err := app.Store.SaveConnection(ctx, conn); err != nil {
		t.Fatal(err)
	}
	_, err := svc.PreviewDeployment(ctx, query(opts, "worker", preview.ActionDeploy, nil, scores(opts)["worker"]))
	if !errors.Is(err, preview.ErrNotReady) || strings.Contains(err.Error(), conn.Key) {
		t.Fatalf("connection not ready: %v", err)
	}
}

func nodeByType(plan *planning.Plan, typ string) (planning.Node, bool) {
	for _, n := range plan.Graph.Nodes {
		if n.ResourceType == typ {
			return n, true
		}
	}
	return planning.Node{}, false
}

func batchIndex(batches [][]string, descriptor string) int {
	for i, batch := range batches {
		for _, d := range batch {
			if d == descriptor {
				return i
			}
		}
	}
	return -1
}

func TestPreview_InternalImplicitGraph(t *testing.T) {
	app, opts := newApp(t, false)
	result, err := newPreview(app.Store).PreviewDeployment(context.Background(), query(opts, "backend", preview.ActionDeploy, nil, scores(opts)["backend"]))
	if err != nil {
		t.Fatal(err)
	}
	ns, ok := nodeByType(result.Plan, planning.TypeNamespace)
	if !ok {
		t.Fatal("internal profile must add the implicit namespace")
	}
	pg, ok := nodeByType(result.Plan, "postgres")
	if !ok || result.Plan.Matches[pg.Descriptor].DefinitionKey != "postgres-internal-statefulset" {
		t.Fatalf("postgres match: %+v", result.Plan.Matches[pg.Descriptor])
	}
	if batchIndex(result.Plan.Batches, ns.Descriptor) >= batchIndex(result.Plan.Batches, pg.Descriptor) {
		t.Fatalf("namespace must be provisioned before postgres: %v", result.Plan.Batches)
	}
}

func TestPreview_AWSImplicitGraph(t *testing.T) {
	app, opts := newApp(t, true)
	opts.ApplicationKey = opts.CloudApplicationKey
	result, err := newPreview(app.Store).PreviewDeployment(context.Background(), query(opts, "backend", preview.ActionDeploy, nil, scores(opts)["backend"]))
	if err != nil {
		t.Fatal(err)
	}
	vpc, hasVPC := nodeByType(result.Plan, planning.TypeVPC)
	cluster, hasCluster := nodeByType(result.Plan, planning.TypeCluster)
	ns, hasNS := nodeByType(result.Plan, planning.TypeNamespace)
	if !hasVPC || !hasCluster || cluster.Class != planning.ClassEKS || !hasNS {
		t.Fatalf("aws-eks must add implicit VPC, EKS and namespace: %+v", result.Plan.Graph.Nodes)
	}
	if !(batchIndex(result.Plan.Batches, vpc.Descriptor) < batchIndex(result.Plan.Batches, cluster.Descriptor) &&
		batchIndex(result.Plan.Batches, cluster.Descriptor) < batchIndex(result.Plan.Batches, ns.Descriptor)) {
		t.Fatalf("provider-first order VPC < EKS < namespace broken: %v", result.Plan.Batches)
	}
	if got := result.Plan.Matches[vpc.Descriptor].DefinitionKey; got != "vpc-aws" {
		t.Fatalf("vpc matched %q", got)
	}
	view, err := result.Public()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(view)
	conn, _ := app.Store.GetConnection(context.Background(), opts.OrganizationKey, opts.CloudConnectionKey)
	if conn.SecretRef == "" || strings.Contains(string(raw), conn.SecretRef) {
		t.Fatalf("connection secret reference %q exposed or unset", conn.SecretRef)
	}
}

// decodeNodes returns the JSON object of every public graph node.
func decodeNodes(t *testing.T, view preview.View) []map[string]any {
	t.Helper()
	raw, _ := json.Marshal(view.Graph)
	var graph struct {
		Nodes []map[string]any `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &graph); err != nil {
		t.Fatal(err)
	}
	return graph.Nodes
}

func TestPreview_PublicViewRedactsWithoutChangingPlan(t *testing.T) {
	ctx := context.Background()
	app, opts := newApp(t, false)
	literal := "literal-password-do-not-show"
	backend := scores(opts)["backend"]
	vars := mainContainer(backend)["variables"].(map[string]any)
	vars["API_PASSWORD"] = literal
	vars["MIXED"] = "prefix-${resources.db.host}"
	deploy(t, app, opts, "backend", nil, backend)

	worker := scores(opts)["worker"]
	mainContainer(worker)["variables"].(map[string]any)["OWN_LITERAL"] = "caller-owned-value"
	result, err := newPreview(app.Store).PreviewDeployment(ctx, query(opts, "worker", preview.ActionDeploy, nil, worker))
	if err != nil {
		t.Fatal(err)
	}
	internalHash, _ := canon.Hash(result.Plan.CandidateSet)
	view, err := result.Public()
	if err != nil {
		t.Fatal(err)
	}

	other := view.CandidateSet.Modules["backend"].Spec.Containers["main"].Variables
	for key, want := range map[string]string{
		"API_PASSWORD": preview.RedactedValue,
		"MIXED":        preview.RedactedValue,
		"PORT":         preview.RedactedValue,
		"PGHOST":       "${shared.acceptance-db.host}",
		"PGPASSWORD":   "${shared.acceptance-db.password}",
	} {
		if other[key] != want {
			t.Errorf("backend %s = %q, want %q", key, other[key], want)
		}
	}
	if own := view.CandidateSet.Modules["worker"].Spec.Containers["main"].Variables["OWN_LITERAL"]; own != "caller-owned-value" {
		t.Errorf("caller-owned literal changed to %q", own)
	}
	if view.PlanHash != result.Plan.PlanHash {
		t.Fatal("view plan hash differs from the internal plan")
	}
	if after, _ := canon.Hash(result.Plan.CandidateSet); after != internalHash ||
		result.Plan.CandidateSet.Modules["backend"].Spec.Containers["main"].Variables["API_PASSWORD"] != literal {
		t.Fatal("public projection mutated the internal Candidate Set")
	}
	if err := planning.VerifyDelta(result.Plan.BaseSet, result.Plan.Delta, result.Plan.CandidateSet); err != nil {
		t.Fatalf("invariant after projection: %v", err)
	}
	allowed := map[string]bool{"descriptor": true, "kind": true, "resourceType": true, "class": true, "origins": true, "workloadId": true, "paramKeys": true, "bindings": true}
	for _, node := range decodeNodes(t, view) {
		for key := range node {
			if !allowed[key] {
				t.Fatalf("graph node exposes %q", key)
			}
		}
	}
	raw, _ := json.Marshal(view)
	var top map[string]any
	_ = json.Unmarshal(raw, &top)
	want := []string{"action", "applicationKey", "baseSetId", "baseVersion", "batches", "candidateSet", "classification", "delta", "environmentKey", "graph", "matches", "planHash", "runId", "workloadId"}
	if len(top) != len(want) {
		t.Fatalf("view fields = %d, want exactly %v", len(top), want)
	}
	for _, key := range want {
		if _, ok := top[key]; !ok {
			t.Fatalf("view missing %s", key)
		}
	}
	for _, m := range top["matches"].([]any) {
		if len(m.(map[string]any)) != 4 {
			t.Fatalf("match exposes extra fields: %v", m)
		}
	}
}

// TestPreview_DefinitionSecretsNeverInView seeds nested secret references
// and a secret-looking literal in a matched Definition; none may surface.
func TestPreview_DefinitionSecretsNeverInView(t *testing.T) {
	ctx := context.Background()
	app, opts := newApp(t, false)
	defs, _ := app.Store.ListResourceDefinitions(ctx, opts.OrganizationKey)
	const nested, literal = "vault://sentinel-nested-secret-ref", "sentinel-definition-literal"
	for _, d := range defs {
		if d.Key != "postgres-internal-statefulset" {
			continue
		}
		values := d.DriverValues()
		values["variables"].(map[string]any)["admin_password"] = literal
		values["credentials"] = map[string]any{"password": map[string]any{"secretRef": nested}}
		d.DriverInputs["secrets"] = map[string]any{"admin": map[string]any{"secretRef": nested}}
		if err := app.Store.SaveResourceDefinition(ctx, opts.OrganizationKey, d); err != nil {
			t.Fatal(err)
		}
	}
	result, err := newPreview(app.Store).PreviewDeployment(ctx, query(opts, "backend", preview.ActionDeploy, nil, scores(opts)["backend"]))
	if err != nil {
		t.Fatal(err)
	}
	view, err := result.Public()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(view)
	for _, sentinel := range []string{nested, literal, "secretRef"} {
		if strings.Contains(string(raw), sentinel) {
			t.Fatalf("view exposes Definition content %q", sentinel)
		}
	}
}

type failingInspector struct {
	planning.ModuleInspector
	sentinel string
	calls    *int
}

func (f failingInspector) Inspect(string) (planning.ModuleContract, error) {
	*f.calls++
	return planning.ModuleContract{}, errors.New("terraform: default password = " + f.sentinel)
}

func TestPreview_CatalogErrorsNeverEchoed(t *testing.T) {
	ctx := context.Background()
	const sentinel = "sentinel-inspector-secret"

	t.Run("inspector", func(t *testing.T) {
		app, opts := newApp(t, true)
		opts.ApplicationKey = opts.CloudApplicationKey
		calls := 0
		svc := preview.NewService(app.Store, planning.NewService(), failingInspector{ModuleInspector: tf.NewInspector(), sentinel: sentinel, calls: &calls})
		_, err := svc.PreviewDeployment(ctx, query(opts, "backend", preview.ActionDeploy, nil, scores(opts)["backend"]))
		var public *preview.PublicError
		if !errors.As(err, &public) || !errors.Is(err, preview.ErrPlanningRejected) || strings.Contains(err.Error(), sentinel) {
			t.Fatalf("inspector error not sanitized: %v", err)
		}
		if calls == 0 {
			t.Fatal("inspector was never reached")
		}
	})

	t.Run("malformed Definition reference", func(t *testing.T) {
		app, opts := newApp(t, true)
		opts.ApplicationKey = opts.CloudApplicationKey
		defs, _ := app.Store.ListResourceDefinitions(ctx, opts.OrganizationKey)
		for _, d := range defs {
			if d.Key == "vpc-aws" {
				d.DriverValues()["variables"].(map[string]any)["name"] = "${resources." + sentinel + "}"
				if err := app.Store.SaveResourceDefinition(ctx, opts.OrganizationKey, d); err != nil {
					t.Fatal(err)
				}
			}
		}
		_, err := newPreview(app.Store).PreviewDeployment(ctx, query(opts, "backend", preview.ActionDeploy, nil, scores(opts)["backend"]))
		if !errors.Is(err, preview.ErrPlanningRejected) || strings.Contains(err.Error(), sentinel) {
			t.Fatalf("Definition error not sanitized: %v", err)
		}
		_, raw := app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "backend", ScoreAfter: scores(opts)["backend"], Actor: "test", RunID: runID})
		if raw == nil || !strings.Contains(raw.Error(), sentinel) {
			t.Fatalf("fixture no longer exercises a leaking planner error: %v", raw)
		}
	})

	t.Run("malformed placeholder in another current module", func(t *testing.T) {
		app, opts := newApp(t, false)
		deploy(t, app, opts, "backend", nil, scores(opts)["backend"])
		env, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
		set, _ := app.Store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
		module := set.Document.Modules["backend"]
		container := module.Spec.Containers["main"]
		container.Variables["LEGACY"] = "${MALFORMED_SECRET_SENTINEL-" + sentinel + "}"
		module.Spec.Containers["main"] = container
		set.Document.Modules["backend"] = module
		if err := app.Store.SaveDeploymentSet(ctx, set); err != nil {
			t.Fatal(err)
		}
		_, err := newPreview(app.Store).PreviewDeployment(ctx, query(opts, "worker", preview.ActionDeploy, nil, scores(opts)["worker"]))
		var public *preview.PublicError
		if !errors.As(err, &public) || strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), "MALFORMED_SECRET_SENTINEL") {
			t.Fatalf("persisted module content not sanitized: %v", err)
		}
		// The raw planner error does quote the persisted content, which is
		// why Preview must not echo it.
		_, raw := app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "worker", ScoreAfter: scores(opts)["worker"], Actor: "test", RunID: runID})
		if raw == nil || !strings.Contains(raw.Error(), sentinel) {
			t.Fatalf("fixture no longer exercises a leaking planner error: %v", raw)
		}
	})
}

// failingStore fails one snapshot read with an error quoting a sentinel.
type failingStore struct {
	persistence.Store
	sentinel string
}

func (f failingStore) ReadSnapshot(ctx context.Context, fn func(context.Context, persistence.Store) error) error {
	return f.Store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		return fn(ctx, failingView{Store: view, sentinel: f.sentinel})
	})
}

type failingView struct {
	persistence.Store
	sentinel string
}

func (f failingView) ListActiveResources(context.Context, string) ([]resource.ActiveResource, error) {
	return nil, errors.New("pq: connection string password=" + f.sentinel)
}

func TestPreview_UnknownStoreErrorIsNotPublic(t *testing.T) {
	app, opts := newApp(t, false)
	_, err := newPreview(failingStore{Store: app.Store, sentinel: "sentinel-store"}).PreviewDeployment(context.Background(), query(opts, "worker", preview.ActionDeploy, nil, scores(opts)["worker"]))
	var public *preview.PublicError
	if err == nil || errors.As(err, &public) {
		t.Fatalf("store failure must stay internal, got %v", err)
	}
}

func TestPreview_PublicViewUsesStableEmptyCollections(t *testing.T) {
	ctx := context.Background()
	app, opts := newApp(t, false)
	result, err := newPreview(app.Store).PreviewDeployment(ctx, query(opts, "frontend", preview.ActionDeploy, nil, scores(opts)["frontend"]))
	if err != nil {
		t.Fatal(err)
	}
	view, err := result.Public()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(view)
	var decoded map[string]any
	_ = json.Unmarshal(raw, &decoded)
	class := decoded["classification"].(map[string]any)
	for _, key := range []string{"existing", "new", "unreferenced"} {
		if _, ok := class[key].([]any); !ok {
			t.Errorf("classification.%s is not an array: %v", key, class[key])
		}
	}
	for _, node := range decodeNodes(t, view) {
		if _, ok := node["paramKeys"].([]any); !ok {
			t.Errorf("node paramKeys is not an array: %v", node)
		}
	}
	if decoded["action"] != "deploy" || decoded["runId"] != runID {
		t.Fatalf("echo fields wrong: %v %v", decoded["action"], decoded["runId"])
	}
	position := map[string]int{}
	for i, batch := range view.Batches {
		for _, d := range batch {
			position[d] = i
		}
	}
	for _, e := range view.Graph.Edges {
		c, cok := position[e.Consumer]
		p, pok := position[e.Provider]
		if cok && pok && p >= c {
			t.Fatalf("provider %s not before consumer %s", e.Provider, e.Consumer)
		}
	}
	if !reflect.DeepEqual(view.Batches, result.Plan.Batches) {
		t.Fatal("batches changed in the view")
	}
}
