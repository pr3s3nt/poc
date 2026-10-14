package resourcecleanup

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfresource"
)

// These are pure unit tests of the selection rules over an in-memory snapshot;
// persistence, HTTP and the engine are covered by the PostgreSQL tests.

func rec(id, descriptor string, scope resource.ScopeType, status tfresource.InstanceStatus) tfresource.ResourceRecord {
	return tfresource.ResourceRecord{
		Instance: tfresource.Instance{ID: id, Status: status, Version: 3, StateKey: strings.Repeat("a", 64),
			Identity: tfresource.Identity{Descriptor: descriptor, Scope: resource.Scope{Type: scope, ID: "s"}}},
		ModuleID: "m", ModuleRevision: "r", Attempts: 1,
	}
}

func snap(records []tfresource.ResourceRecord, edges ...tfresource.Dependency) *snapshot {
	s := &snapshot{scope: Scope{OrganizationKey: "o", ApplicationKey: "a", EnvironmentKey: "staging"}, records: map[string]tfresource.ResourceRecord{},
		active: map[string]resource.ActiveResource{}, types: map[string]resource.Type{}, doc: environment.NewDocument(), edges: edges, env: environment.Environment{ID: "e", Version: 1}}
	for _, r := range records {
		s.records[r.Instance.ID] = r
		s.order = append(s.order, r.Instance.ID)
	}
	return s
}

func edge(consumer, provider string, status tfresource.InstanceStatus) tfresource.Dependency {
	return tfresource.Dependency{ConsumerID: consumer, ProviderID: provider, ConsumerStatus: status, Version: 1}
}

const un = tfresource.InstanceUnreferenced

func TestSelectionRulesCascadeThroughTheDependencyChain(t *testing.T) {
	// root -> mid -> leaf; leaf is only held by mid, mid only by root.
	s := snap([]tfresource.ResourceRecord{
		rec("root", "postgres.default#shared.db", resource.ScopeShared, un),
		rec("mid", "vpc.default#environments.x.vpc", resource.ScopeEnvironment, un),
		rec("leaf", "k8s-namespace.default#environments.x.ns", resource.ScopeEnvironment, un),
	}, edge("root", "mid", un), edge("mid", "leaf", un))
	p, err := buildPlan(s, "root")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.order) != 3 || p.order[0] != "root" {
		t.Fatalf("transitive providers must be candidates: %v", p.order)
	}
	ok := func(sel ...string) error { return p.validate(sel) }
	if err := ok("root"); err != nil {
		t.Fatalf("root only: %v", err)
	}
	if err := ok("root", "mid"); err != nil {
		t.Fatalf("root+mid: %v", err)
	}
	if err := ok("root", "mid", "leaf"); err != nil {
		t.Fatalf("all: %v", err)
	}
	// leaf without mid: mid (its consumer) is outside the set.
	var refused *RefusedError
	if err := ok("root", "leaf"); !errors.As(err, &refused) || refused.Reason != ReasonHasConsumers {
		t.Fatalf("leaf without its consumer: %v", err)
	}
	// Structural errors are invalid, not refusals.
	var field *FieldError
	for name, sel := range map[string][]string{"no root": {"mid"}, "duplicate": {"root", "mid", "mid"}, "unknown": {"root", "zzz"}, "empty id": {"root", ""}} {
		if err := ok(sel...); !errors.As(err, &field) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestOutsideAndRetainedConsumersLockADependency(t *testing.T) {
	cases := map[string]tfresource.InstanceStatus{
		"ready consumer": tfresource.InstanceReady, "unreferenced consumer": un, "failed consumer": tfresource.InstanceFailed,
		"interrupted consumer": tfresource.InstanceInterrupted, "needs attention": tfresource.InstanceNeedsAttention,
	}
	for name, status := range cases {
		s := snap([]tfresource.ResourceRecord{
			rec("root", "postgres.default#shared.db", resource.ScopeShared, un),
			rec("dep", "vpc.default#environments.x.vpc", resource.ScopeEnvironment, un),
			rec("other", "redis.default#shared.cache", resource.ScopeShared, status),
		}, edge("root", "dep", un), edge("other", "dep", status))
		p, _ := buildPlan(s, "root")
		if len(p.items["dep"].reasons) == 0 || p.items["dep"].reasons[0] != ReasonHasConsumers {
			t.Errorf("%s: the dependency must be locked: %+v", name, p.items["dep"].reasons)
		}
		if err := p.validate([]string{"root", "dep"}); err == nil {
			t.Errorf("%s: a locked dependency was accepted", name)
		}
	}
	// A DESTROYED consumer no longer holds.
	s := snap([]tfresource.ResourceRecord{
		rec("root", "postgres.default#shared.db", resource.ScopeShared, un),
		rec("dep", "vpc.default#environments.x.vpc", resource.ScopeEnvironment, un),
		rec("gone", "redis.default#shared.cache", resource.ScopeShared, tfresource.InstanceDestroyed),
	}, edge("root", "dep", un), edge("gone", "dep", tfresource.InstanceDestroyed))
	p, _ := buildPlan(s, "root")
	if err := p.validate([]string{"root", "dep"}); err != nil {
		t.Fatalf("a destroyed consumer must not hold: %v", err)
	}
}

func TestRootNeedsNoConsumerAndPlatformIsNeverACandidate(t *testing.T) {
	s := snap([]tfresource.ResourceRecord{
		rec("root", "postgres.default#shared.db", resource.ScopeShared, un),
		rec("plat", "k8s-cluster.internal#connections.k8s-4f", resource.ScopeApplication, tfresource.InstanceReady),
		rec("app", "k8s-secret.default#applications.x.s", resource.ScopeApplication, un),
		rec("user", "redis.default#shared.cache", resource.ScopeShared, un),
	}, edge("root", "plat", un), edge("root", "app", un), edge("user", "root", un))
	p, _ := buildPlan(s, "root")
	if len(p.order) != 1 {
		t.Fatalf("platform and application-wide resources must not be listed: %v", p.order)
	}
	if p.rootRefusal() != ReasonHasConsumers {
		t.Fatalf("a root with a consumer must be refused, got %q", p.rootRefusal())
	}
	if pr, _ := buildPlan(s, "plat"); pr.rootRefusal() == "" {
		t.Fatal("a platform root must be refused")
	}
}

func TestWorkloadAndPendingConsumersBlock(t *testing.T) {
	s := snap([]tfresource.ResourceRecord{
		rec("shared", "postgres.default#shared.db", resource.ScopeShared, un),
		rec("private", "postgres.default#modules.api.externals.db", resource.ScopeWorkload, un),
		rec("infra", "k8s-namespace.default#environments.x.ns", resource.ScopeEnvironment, un),
	})
	// Nothing consumes anything yet.
	for id, want := range map[string]string{"shared": "", "private": "", "infra": ""} {
		p, _ := buildPlan(s, id)
		if got := p.rootRefusal(); got != want {
			t.Fatalf("%s: %q", id, got)
		}
	}
	// A current module that declares the private resource and references the shared one.
	s.doc.Modules["api"] = environment.Module{Profile: environment.ModuleProfile, Externals: map[string]environment.ResourceEntry{"db": {Type: "postgres"}},
		Spec: environment.ModuleSpec{Containers: map[string]environment.Container{"main": {Image: "x", Variables: map[string]string{"URL": "${shared.db.host}"}}}}}
	s.doc.Shared["db"] = environment.ResourceEntry{Type: "postgres"}
	for _, id := range []string{"shared", "private", "infra"} {
		p, _ := buildPlan(s, id)
		if p.rootRefusal() != ReasonHasConsumers {
			t.Errorf("%s consumed by the current Deployment Set was not refused", id)
		}
	}
	// A pending upsert consumes what it declares (and the Environment infrastructure).
	s.doc = environment.NewDocument()
	s.drafts = []environment.WorkloadDraft{{WorkloadID: "web", State: environment.DraftUpsert, Score: map[string]any{"resources": map[string]any{"db": map[string]any{"type": "postgres", "id": "db"}}}}}
	for _, id := range []string{"shared", "infra"} {
		p, _ := buildPlan(s, id)
		if p.rootRefusal() != ReasonHasConsumers {
			t.Errorf("%s consumed by a pending upsert was not refused", id)
		}
	}
	if p, _ := buildPlan(s, "private"); p.rootRefusal() != "" {
		t.Errorf("a pending workload that does not declare the private resource must not block it")
	}
	// A pending deletion consumes nothing.
	s.drafts = []environment.WorkloadDraft{{WorkloadID: "web", State: environment.DraftDelete}}
	if p, _ := buildPlan(s, "infra"); p.rootRefusal() != "" {
		t.Error("a pending deletion blocked the infrastructure")
	}
}

func TestStatusAndEnvironmentBlockers(t *testing.T) {
	for _, status := range []tfresource.InstanceStatus{tfresource.InstanceReady, tfresource.InstanceProvisioning, tfresource.InstanceFailed, tfresource.InstanceDestroying, tfresource.InstanceDestroyed, tfresource.InstanceNeedsAttention} {
		p, _ := buildPlan(snap([]tfresource.ResourceRecord{rec("root", "postgres.default#shared.db", resource.ScopeShared, status)}), "root")
		if p.rootRefusal() != ReasonNotUnreferenced {
			t.Errorf("%s root accepted", status)
		}
	}
	s := snap([]tfresource.ResourceRecord{rec("root", "postgres.default#shared.db", resource.ScopeShared, un)})
	s.env.ActiveOperationID = "busy"
	if p, _ := buildPlan(s, "root"); p.rootRefusal() != ReasonEnvironmentBusy {
		t.Error("a busy Environment accepted a deletion")
	}
	// An Active Resource that is still READY keeps an UNREFERENCED instance ineligible.
	r := rec("root", "postgres.default#shared.db", resource.ScopeShared, un)
	r.Instance.ActiveResourceID = "act"
	s = snap([]tfresource.ResourceRecord{r})
	s.active["act"] = resource.ActiveResource{ID: "act", Status: resource.StatusReady}
	if p, _ := buildPlan(s, "root"); p.rootRefusal() != ReasonNotUnreferenced {
		t.Error("a READY Active Resource was accepted")
	}
	// No execution history is no proof of ownership.
	r = rec("root", "postgres.default#shared.db", resource.ScopeShared, un)
	r.Attempts = 0
	if p, _ := buildPlan(snap([]tfresource.ResourceRecord{r}), "root"); p.rootRefusal() != ReasonOwnershipUnproven {
		t.Error("an instance without history was accepted")
	}
}

func TestTokenPinsEverythingAndHoldsNoPrivateData(t *testing.T) {
	base := func() *snapshot {
		return snap([]tfresource.ResourceRecord{
			rec("root", "postgres.default#shared.db", resource.ScopeShared, un),
			rec("dep", "vpc.default#environments.x.vpc", resource.ScopeEnvironment, un),
		}, edge("root", "dep", un))
	}
	hash := func(s *snapshot, actor string, targets map[string]string) string {
		p, err := buildPlan(s, "root")
		if err != nil {
			t.Fatal(err)
		}
		h, err := p.tokenHash(actor, targets, 4102444800, "0123456789abcdef")
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	targets := map[string]string{"root": "t1", "dep": "t2"}
	ref := hash(base(), "pe", targets)
	if hash(base(), "pe", targets) != ref {
		t.Fatal("the token is not deterministic")
	}
	changes := map[string]func(s *snapshot){
		"instance version": func(s *snapshot) { r := s.records["dep"]; r.Instance.Version++; s.records["dep"] = r },
		"status": func(s *snapshot) {
			r := s.records["dep"]
			r.Instance.Status = tfresource.InstanceReady
			s.records["dep"] = r
		},
		"environment": func(s *snapshot) { s.env.Version++ },
		"drafts":      func(s *snapshot) { s.env.DraftVersion++ },
		"current set": func(s *snapshot) { s.env.CurrentDeploymentSetID = "set-2" },
		"consumer":    func(s *snapshot) { s.edges = append(s.edges, edge("late", "dep", tfresource.InstanceReady)) },
		"module":      func(s *snapshot) { r := s.records["dep"]; r.ModuleRevision = "other"; s.records["dep"] = r },
		"state key": func(s *snapshot) {
			r := s.records["dep"]
			r.Instance.StateKey = strings.Repeat("b", 64)
			s.records["dep"] = r
		},
	}
	for name, change := range changes {
		s := base()
		change(s)
		if hash(s, "pe", targets) == ref {
			t.Errorf("%s does not change the token", name)
		}
	}
	pl, _ := buildPlan(base(), "root")
	if later, _ := pl.tokenHash("pe", targets, 4102444801, "0123456789abcdef"); later == ref {
		t.Error("the token is not bound to its issue expiry")
	}
	if other, _ := pl.tokenHash("pe", targets, 4102444800, "fedcba9876543210"); other == ref {
		t.Error("two previews of the same state share a token")
	}
	if hash(base(), "other", targets) == ref {
		t.Error("the token is not bound to the actor")
	}
	if hash(base(), "pe", map[string]string{"root": "t1", "dep": "changed"}) == ref {
		t.Error("the token is not bound to the original target")
	}
	if len(ref) != 64 || strings.ContainsAny(ref, "GHIJKLMNOPQRSTUVWXYZ") {
		t.Fatalf("the token digest is not an opaque hex digest: %q", ref)
	}
}

func TestParseTokenRejectsMalformedValues(t *testing.T) {
	good := "rd1.4102444800." + strings.Repeat("b", 16) + "." + strings.Repeat("a", 64)
	if _, nonce, hash, err := parseToken(good); err != nil || hash != strings.Repeat("a", 64) || nonce != strings.Repeat("b", 16) {
		t.Fatalf("good token: %v", err)
	}
	n, h := strings.Repeat("b", 16), strings.Repeat("a", 64)
	for _, bad := range []string{"", "rd1", "rd2.1." + n + "." + h, "rd1.x." + n + "." + h, "rd1.1." + n + "." + strings.Repeat("g", 64), "rd1.1." + n + "." + strings.Repeat("a", 63),
		good + ".x", "rd1.-1." + n + "." + h, "rd1.1." + strings.Repeat("z", 16) + "." + h, "rd1.1." + n[:15] + "." + h, "rd1.4102444800." + h} {
		var field *FieldError
		if _, _, _, err := parseToken(bad); !errors.As(err, &field) || field.Field != "token" {
			t.Errorf("token %q: %v", bad, err)
		}
	}
}

func TestProjectionAllowlistsEveryStoredValue(t *testing.T) {
	s := snap([]tfresource.ResourceRecord{rec("root", "postgres.default#shared.db", resource.ScopeShared, un)})
	rc := tfresource.CleanupRecord{OperationID: "op", Mode: "SOMETHING", ActorRef: "pe", RootID: "root", SelectedIDs: []string{"root"},
		Status: "weird status", Stage: "weird stage password=x", Failure: "dial tcp 10.0.0.1: token=abc",
		Detail: map[string]any{"terminal": "RAW", "results": []any{map[string]any{"id": "root", "destroyed": false, "code": "RAW ERROR", "remaining": []any{"aws_vpc.main=vpc-1", "has space", "/etc/passwd secret"}}}}}
	v := projectCleanup(s, rc, nil)
	out := toString(v)
	for _, bad := range []string{"weird", "password=x", "10.0.0.1", "token=abc", "RAW", "has space", "/etc/passwd"} {
		if strings.Contains(out, bad) {
			t.Errorf("projection forwards %q: %s", bad, out)
		}
	}
	if v.Status != "UNKNOWN" || v.Stage != "UNKNOWN" || v.FailureCode != "CLEANUP_FAILED" || v.AllVerified {
		t.Fatalf("generic markers expected: %+v", v)
	}
	if len(v.Resources) != 1 || v.Resources[0].Verified || len(v.Resources[0].Remaining) != 1 || v.Resources[0].Remaining[0] != "aws_vpc.main=vpc-1" {
		t.Fatalf("resource: %+v", v.Resources)
	}
	// A SUCCEEDED operation without its own proof is not verified, even if the instance is DESTROYED now.
	gone := s.records["root"]
	gone.Instance.Status = tfresource.InstanceDestroyed
	s.records["root"] = gone
	rc.Status, rc.Failure, rc.Detail = "SUCCEEDED", "", nil
	if v := projectCleanup(s, rc, nil); v.AllVerified || v.Resources[0].Verified || !v.Resources[0].RemainingUnknown {
		t.Fatalf("success without persisted absence was believed: %+v", v)
	}
}

func toString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
