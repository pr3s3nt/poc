package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"orchestrator/internal/application/envops"
	domain "orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/ports/persistence"
)

func scoreOf(name string, variables map[string]string, extra map[string]any) map[string]any {
	vars := map[string]any{}
	for k, v := range variables {
		vars[k] = v
	}
	out := map[string]any{
		"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": name},
		"containers": map[string]any{"web": map[string]any{"image": "img", "variables": vars}},
		"resources":  map[string]any{"env": map[string]any{"type": "environment"}},
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func (f *fixture) types(t *testing.T) map[string]resource.Type {
	t.Helper()
	typ := resource.Type{Key: "postgres", Inputs: []resource.InputField{{Name: "database", Type: "string"}}, Outputs: []resource.OutputField{{Name: "host", Type: "string", Required: true}}}
	if err := f.st.SaveResourceType(context.Background(), org, typ); err != nil {
		t.Fatal(err)
	}
	return map[string]resource.Type{"postgres": typ}
}

// deploy installs the given Scores as the current Deployment Set (no runtime).
func (f *fixture) deploy(t *testing.T, env string, scores map[string]map[string]any) {
	t.Helper()
	types := f.types(t)
	doc := environment.NewDocument()
	for id, raw := range scores {
		parsed, err := score.FromMap(raw)
		if err != nil {
			t.Fatal(err)
		}
		fragment, err := parsed.Fragment(types)
		if err != nil {
			t.Fatal(err)
		}
		doc.Modules[id] = fragment.Module
		for sid, entry := range fragment.Shared {
			doc.Shared[sid] = entry
		}
	}
	f.installDocument(t, env, doc)
}

func (f *fixture) installDocument(t *testing.T, env string, doc environment.Document) {
	t.Helper()
	ctx := context.Background()
	current, err := f.st.GetEnvironment(ctx, appKey, env)
	if err != nil {
		t.Fatal(err)
	}
	set := environment.DeploymentSet{ID: "11111111-1111-1111-1111-111111111111", EnvironmentKey: env, Document: doc}
	if err := f.st.SaveDeploymentSet(ctx, set); err != nil {
		t.Fatal(err)
	}
	current.CurrentDeploymentSetID = set.ID
	if err := f.st.SaveEnvironment(ctx, current); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) draft(t *testing.T, env, id string, state environment.DraftState, raw map[string]any) {
	t.Helper()
	ctx := context.Background()
	current, _ := f.st.GetEnvironment(ctx, appKey, env)
	if err := f.st.SaveWorkloadDraft(ctx, current.DraftVersion, environment.WorkloadDraft{ApplicationKey: appKey, EnvironmentKey: env, WorkloadID: id, State: state, Score: raw}); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) guards(t *testing.T, env string) (View, ReferenceGuards) {
	t.Helper()
	view, err := f.svc.List(context.Background(), appKey, env)
	if err != nil {
		t.Fatal(err)
	}
	return view, ReferenceGuards{UpdateReferences: true, EnvironmentVersion: view.EnvironmentVersion, DraftVersion: view.DraftVersion}
}

type state struct {
	env    environment.Environment
	scope  domain.Scope
	drafts []environment.WorkloadDraft
}

func (f *fixture) state(t *testing.T, env string) state {
	t.Helper()
	ctx := context.Background()
	e, _ := f.st.GetEnvironment(ctx, appKey, env)
	scope, _ := f.st.GetConfigurationScope(ctx, appKey, env)
	drafts, _ := f.st.ListWorkloadDrafts(ctx, appKey, env)
	return state{e, scope, drafts}
}

func draftByID(drafts []environment.WorkloadDraft) map[string]environment.WorkloadDraft {
	out := map[string]environment.WorkloadDraft{}
	for _, d := range drafts {
		out[d.WorkloadID] = d
	}
	return out
}

func variablesOf(t *testing.T, raw map[string]any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(raw)
	var doc struct {
		Containers map[string]struct {
			Variables map[string]any `json:"variables"`
		} `json:"containers"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Containers["web"].Variables
}

func seedKeys(t *testing.T, f *fixture, env string) View {
	t.Helper()
	ctx := context.Background()
	f.selectStore(t, env, "vault-a")
	view, err := f.svc.Put(ctx, appKey, env, "DB_URL", domain.Variable, "postgres://x", 0)
	if err != nil {
		t.Fatal(err)
	}
	if view, err = f.svc.Put(ctx, appKey, env, "TOKEN", domain.Secret, "s3cret-value", view.Version); err != nil {
		t.Fatal(err)
	}
	if view, err = f.svc.Put(ctx, appKey, env, "OTHER", domain.Variable, "o", view.Version); err != nil {
		t.Fatal(err)
	}
	return view
}

func TestRenameWithReferencesRewritesDesiredWorkloadsAtomically(t *testing.T) {
	for _, tc := range []struct{ name, old, new string }{{"variable", "DB_URL", "DATABASE_URL"}, {"secret", "TOKEN", "API_TOKEN"}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := newFixture(t)
			seedKeys(t, f, "staging")
			f.deploy(t, "staging", map[string]map[string]any{
				"api":   scoreOf("api", map[string]string{"A": "${resources.env." + tc.old + "}", "KEEP": "${resources.env.OTHER}"}, nil),
				"web":   scoreOf("web", map[string]string{"A": "${resources.env." + tc.old + "}"}, nil),
				"gone":  scoreOf("gone", map[string]string{"A": "${resources.env." + tc.old + "}"}, nil),
				"other": scoreOf("other", map[string]string{"A": "${resources.env.OTHER}"}, nil),
			})
			// web has a pending edit (supersedes deployed), gone a pending delete.
			f.draft(t, "staging", "web", environment.DraftUpsert, scoreOf("web", map[string]string{"RENAMED_VAR": "${resources.env." + tc.old + "}", "N": "${resources.env.OTHER}"}, map[string]any{"replicas": 3}))
			f.draft(t, "staging", "gone", environment.DraftDelete, nil)
			before := f.state(t, "staging")
			view, guards := f.guards(t, "staging")
			spy := &spyRegistry{Registry: f.registry}
			svc := NewService(f.st, spy, envops.NewManager(f.st))

			got, err := svc.RenameKey(ctx, appKey, "staging", tc.old, tc.new, view.Version, guards)
			if err != nil {
				t.Fatal(err)
			}
			if spy.count() != 0 {
				t.Fatalf("rename called a provider %d time(s)", spy.count())
			}
			after := f.state(t, "staging")
			if after.scope.Version != before.scope.Version+1 || got.Version != after.scope.Version {
				t.Fatalf("config version %d -> %d", before.scope.Version, after.scope.Version)
			}
			if after.env.DraftVersion != before.env.DraftVersion+1 || after.env.Version != before.env.Version || after.env.CurrentDeploymentSetID != before.env.CurrentDeploymentSetID || after.env.ActiveOperationID != "" {
				t.Fatalf("environment %+v -> %+v", before.env, after.env)
			}
			if got.EnvironmentVersion != after.env.Version || got.DraftVersion != after.env.DraftVersion {
				t.Fatalf("view guards %+v", got)
			}
			drafts := draftByID(after.drafts)
			api := variablesOf(t, drafts["api"].Score)
			if api["A"] != "${resources.env."+tc.new+"}" || api["KEEP"] != "${resources.env.OTHER}" {
				t.Fatalf("api rewrite: %v", api)
			}
			web := variablesOf(t, drafts["web"].Score)
			if web["RENAMED_VAR"] != "${resources.env."+tc.new+"}" || web["N"] != "${resources.env.OTHER}" || drafts["web"].Score["replicas"] != float64(3) {
				t.Fatalf("web rewrite: %v %v", web, drafts["web"].Score)
			}
			if !reflect.DeepEqual(drafts["gone"], draftByID(before.drafts)["gone"]) || drafts["gone"].State != environment.DraftDelete {
				t.Fatalf("pending delete changed: %+v", drafts["gone"])
			}
			if _, created := drafts["other"]; created {
				t.Fatal("unrelated workload got a draft")
			}
			rev, _ := f.st.GetConfigurationRevision(ctx, got.RevisionID)
			old, _ := f.st.GetConfigurationRevision(ctx, view.RevisionID)
			if rev.Entries[tc.new] != old.Entries[tc.old] || len(rev.Entries) != len(old.Entries) {
				t.Fatalf("opaque entry not carried: %+v vs %+v", rev.Entries, old.Entries)
			}
			if _, ok := rev.Entries[tc.old]; ok {
				t.Fatal("old key kept")
			}
			encoded, _ := json.Marshal(after.drafts)
			if strings.Contains(string(encoded), "s3cret-value") || strings.Contains(string(encoded), "postgres://x") {
				t.Fatal("value copied into a draft")
			}
		})
	}
}

func TestRenameWithoutOptInKeepsReferencesAndDrafts(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	view := seedKeys(t, f, "staging")
	f.deploy(t, "staging", map[string]map[string]any{"api": scoreOf("api", map[string]string{"A": "${resources.env.DB_URL}"}, nil)})
	before := f.state(t, "staging")
	if _, err := f.svc.Rename(ctx, appKey, "staging", "DB_URL", "NEW", view.Version); err != nil {
		t.Fatal(err)
	}
	after := f.state(t, "staging")
	if after.env.DraftVersion != before.env.DraftVersion || len(after.drafts) != 0 || after.scope.Version != before.scope.Version+1 {
		t.Fatalf("rename-only changed drafts: %+v", after)
	}
	// Explicit false guards behave the same.
	view, _ = f.guards(t, "staging")
	if _, err := f.svc.RenameKey(ctx, appKey, "staging", "NEW", "NEWER", view.Version, ReferenceGuards{}); err != nil {
		t.Fatal(err)
	}
	if len(f.state(t, "staging").drafts) != 0 {
		t.Fatal("false opt-in wrote drafts")
	}
}

func TestRenameWithReferencesGuardsLeaveEverythingUnchanged(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	seedKeys(t, f, "staging")
	f.deploy(t, "staging", map[string]map[string]any{"api": scoreOf("api", map[string]string{"A": "${resources.env.DB_URL}"}, nil)})
	view, guards := f.guards(t, "staging")
	before := f.state(t, "staging")
	cases := map[string]struct {
		name   string
		new    string
		config int64
		g      ReferenceGuards
		want   error
	}{
		"config":      {"DB_URL", "N", view.Version - 1, guards, persistence.ErrVersionConflict},
		"environment": {"DB_URL", "N", view.Version, ReferenceGuards{UpdateReferences: true, EnvironmentVersion: guards.EnvironmentVersion + 1, DraftVersion: guards.DraftVersion}, persistence.ErrVersionConflict},
		"draft":       {"DB_URL", "N", view.Version, ReferenceGuards{UpdateReferences: true, EnvironmentVersion: guards.EnvironmentVersion, DraftVersion: guards.DraftVersion + 1}, persistence.ErrVersionConflict},
		"zero":        {"DB_URL", "N", view.Version, ReferenceGuards{UpdateReferences: true}, persistence.ErrVersionConflict},
		"duplicate":   {"DB_URL", "OTHER", view.Version, guards, ErrConflict},
		"missing":     {"NOPE", "N", view.Version, guards, persistence.ErrNotFound},
	}
	for name, tc := range cases {
		if _, err := f.svc.RenameKey(ctx, appKey, "staging", tc.name, tc.new, tc.config, tc.g); !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", name, err, tc.want)
		}
		if after := f.state(t, "staging"); !reflect.DeepEqual(before, after) {
			t.Fatalf("%s changed state", name)
		}
	}
	if _, err := f.svc.RenameKey(ctx, appKey, "staging", "DB_URL", "DB_URL", view.Version, guards); !errors.Is(err, ErrInvalid) {
		t.Fatalf("same name: %v", err)
	}
	if _, err := f.st.ClaimEnvironment(ctx, persistence.OperationClaim{ApplicationKey: appKey, EnvironmentKey: "staging", Owner: "other", Kind: environment.OpDeploy}); err != nil {
		t.Fatal(err)
	}
	busyBefore := f.state(t, "staging")
	if _, err := f.svc.RenameKey(ctx, appKey, "staging", "DB_URL", "N", view.Version, guards); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("busy: %v", err)
	}
	if !reflect.DeepEqual(busyBefore, f.state(t, "staging")) {
		t.Fatal("busy rename changed state")
	}
}

func TestRenameWithReferencesFailsClosedOnUnsafeShapes(t *testing.T) {
	ctx := context.Background()
	extType := map[string]any{"type": "postgres", "params": map[string]any{"database": "d"}}
	cases := map[string]func(t *testing.T, f *fixture){
		"legacy literal binding": func(t *testing.T, f *fixture) {
			doc := environment.NewDocument()
			doc.Modules["api"] = environment.Module{Profile: environment.ModuleProfile, Spec: environment.ModuleSpec{Containers: map[string]environment.Container{
				"web": {ID: "web", Image: "i", Variables: map[string]string{"A": "${context.uc12.DB_URL}", "SECRET": "literal-secret"}}}}}
			f.installDocument(t, "staging", doc)
		},
		"external named like the virtual alias": func(t *testing.T, f *fixture) {
			doc := environment.NewDocument()
			doc.Modules["api"] = environment.Module{Profile: environment.ModuleProfile,
				Spec:      environment.ModuleSpec{Containers: map[string]environment.Container{"web": {ID: "web", Image: "i", Variables: map[string]string{"A": "${context.uc12.DB_URL}"}}}},
				Externals: map[string]environment.ResourceEntry{"env": {Type: "postgres", Class: "default", Params: map[string]any{"database": "d"}}}}
			f.installDocument(t, "staging", doc)
		},
		"unreferenced shared resource has no known owner": func(t *testing.T, f *fixture) {
			f.deploy(t, "staging", map[string]map[string]any{
				"api": scoreOf("api", map[string]string{"A": "${resources.env.DB_URL}"}, map[string]any{"resources": map[string]any{
					"env": map[string]any{"type": "environment"}, "db": map[string]any{"type": "postgres", "id": "orphan", "params": map[string]any{"database": "d"}}}}),
			})
		},
		"invalid draft": func(t *testing.T, f *fixture) {
			f.draft(t, "staging", "api", environment.DraftUpsert, map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "api"},
				"containers": map[string]any{"web": map[string]any{"image": "i", "variables": map[string]any{"A": "${resources.env.DB_URL}"}, "bogus": 1}},
				"resources":  map[string]any{"env": map[string]any{"type": "environment"}}})
		},
	}
	_ = extType
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			seedKeys(t, f, "staging")
			setup(t, f)
			view, guards := f.guards(t, "staging")
			before := f.state(t, "staging")
			_, err := f.svc.RenameKey(ctx, appKey, "staging", "DB_URL", "NEW", view.Version, guards)
			if !errors.Is(err, ErrReferenceRewriteUnsafe) {
				t.Fatalf("err = %v, want unsafe", err)
			}
			if strings.Contains(err.Error(), "literal-secret") || strings.Contains(err.Error(), "DB_URL") {
				t.Fatalf("error leaks content: %v", err)
			}
			if !reflect.DeepEqual(before, f.state(t, "staging")) {
				t.Fatal("unsafe rename partially changed state")
			}
		})
	}
}

func TestRenameWithReferencesPreservesAliasesManyToOneAndOtherFields(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	seedKeys(t, f, "staging")
	extra := map[string]any{
		"replicas": 2,
		"service":  map[string]any{"ports": map[string]any{"http": map[string]any{"port": 80}}, "publicRoutes": []any{map[string]any{"path": "/", "port": "http"}}},
		"resources": map[string]any{
			"env": map[string]any{"type": "environment"},
			"db1": map[string]any{"type": "postgres", "id": "shared-db", "params": map[string]any{"database": "${resources.env.DB_URL}"}},
			"db2": map[string]any{"type": "postgres", "id": "shared-db", "params": map[string]any{"database": "${resources.env.DB_URL}"}},
			"own": map[string]any{"type": "postgres", "params": map[string]any{"database": "keep"}},
		},
	}
	draft := scoreOf("api", map[string]string{"A": "${resources.env.DB_URL}", "H1": "${resources.db1.host}", "H2": "${resources.db2.host}", "O": "${resources.own.host}", "LIT": "$${resources.env.DB_URL}"}, extra)
	f.types(t)
	f.draft(t, "staging", "api", environment.DraftUpsert, draft)
	view, guards := f.guards(t, "staging")
	if _, err := f.svc.RenameKey(ctx, appKey, "staging", "DB_URL", "NEW", view.Version, guards); err != nil {
		t.Fatal(err)
	}
	got := draftByID(f.state(t, "staging").drafts)["api"].Score
	want := scoreOf("api", map[string]string{"A": "${resources.env.NEW}", "H1": "${resources.db1.host}", "H2": "${resources.db2.host}", "O": "${resources.own.host}", "LIT": "$${resources.env.DB_URL}"}, extra)
	want["resources"].(map[string]any)["db1"].(map[string]any)["params"] = map[string]any{"database": "${resources.env.NEW}"}
	want["resources"].(map[string]any)["db2"].(map[string]any)["params"] = map[string]any{"database": "${resources.env.NEW}"}
	if !jsonEqual(got, want) {
		gb, _ := json.Marshal(got)
		wb, _ := json.Marshal(want)
		t.Fatalf("got  %s\nwant %s", gb, wb)
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	var left, right any
	_ = json.Unmarshal(x, &left)
	_ = json.Unmarshal(y, &right)
	return reflect.DeepEqual(left, right)
}

func TestRenameDeployedModuleKeepsSharedExternalsServiceRefsAndParams(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	seedKeys(t, f, "staging")
	api := scoreOf("api", map[string]string{"A": "${resources.env.DB_URL}", "H": "${resources.db.host}", "O": "${resources.own.host}", "SVC": "${resources.peer.url}"}, map[string]any{
		"service": map[string]any{"ports": map[string]any{"http": map[string]any{"port": 80}}},
		"resources": map[string]any{
			"env":  map[string]any{"type": "environment"},
			"db":   map[string]any{"type": "postgres", "id": "shared-db", "params": map[string]any{"database": "${resources.env.DB_URL}"}},
			"own":  map[string]any{"type": "postgres", "params": map[string]any{"database": "keep"}},
			"peer": map[string]any{"type": "service", "params": map[string]any{"workload": "peer", "port": "http"}},
		}})
	peer := scoreOf("peer", map[string]string{}, map[string]any{"service": map[string]any{"ports": map[string]any{"http": map[string]any{"port": 81}}}})
	f.deploy(t, "staging", map[string]map[string]any{"api": api, "peer": peer})
	view, guards := f.guards(t, "staging")
	if view.Keys[0].Name != "DB_URL" || len(view.Keys[0].UsedBy) != 1 || view.Keys[0].UsedBy[0] != "api" {
		t.Fatalf("usedBy from actual context.uc12 refs: %+v", view.Keys)
	}
	if _, err := f.svc.RenameKey(ctx, appKey, "staging", "DB_URL", "NEW", view.Version, guards); err != nil {
		t.Fatal(err)
	}
	after := f.state(t, "staging")
	drafts := draftByID(after.drafts)
	if len(drafts) != 1 || drafts["api"].State != environment.DraftUpsert {
		t.Fatalf("drafts: %+v", after.drafts)
	}
	// The rewritten draft converts to the deployed module with only the key renamed.
	parsed, err := score.FromMap(drafts["api"].Score)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := parsed.Fragment(f.types(t))
	if err != nil {
		t.Fatal(err)
	}
	set, _ := f.st.GetDeploymentSet(ctx, after.env.CurrentDeploymentSetID)
	old := set.Document.Modules["api"]
	if fragment.Module.Spec.Containers["web"].Variables["A"] != "${context.uc12.NEW}" ||
		fragment.Module.Spec.Containers["web"].Variables["SVC"] != old.Spec.Containers["web"].Variables["SVC"] ||
		fragment.Module.Spec.Containers["web"].Variables["H"] != old.Spec.Containers["web"].Variables["H"] ||
		!reflect.DeepEqual(fragment.Module.Externals, old.Externals) ||
		fragment.Shared["shared-db"].Params["database"] != "${context.uc12.NEW}" {
		t.Fatalf("fragment %+v", fragment)
	}
	// Runtime desired state is untouched until an explicit Deploy.
	if old.Spec.Containers["web"].Variables["A"] != "${context.uc12.DB_URL}" {
		t.Fatal("current Deployment Set was rewritten")
	}
}

func TestConcurrentRenameWithReferencesHasOneWinner(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	seedKeys(t, f, "staging")
	f.deploy(t, "staging", map[string]map[string]any{"api": scoreOf("api", map[string]string{"A": "${resources.env.DB_URL}"}, nil)})
	view, guards := f.guards(t, "staging")
	var wg sync.WaitGroup
	results := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = f.svc.RenameKey(ctx, appKey, "staging", "DB_URL", "NEW"+string(rune('A'+i)), view.Version, guards)
		}()
	}
	wg.Wait()
	wins := 0
	for _, err := range results {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, persistence.ErrVersionConflict) && !errors.Is(err, ErrNotFound()):
			t.Fatalf("loser err = %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("winners = %d (%v)", wins, results)
	}
	after := f.state(t, "staging")
	if len(after.drafts) != 1 || after.env.DraftVersion != 1 {
		t.Fatalf("state after race: %+v", after)
	}
}

func ErrNotFound() error { return persistence.ErrNotFound }
