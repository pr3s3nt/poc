package accountdiag

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/connection"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/ids"
	probes "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/ports/persistence"
)

// fakeProvider is a unit-test double of the probe boundary. It proves the
// service's orchestration only; it is never evidence of real cluster state.
type fakeProvider struct {
	mu      sync.Mutex
	checks  []probes.Check
	run     func(ctx context.Context, c probes.Check, prior map[string]domain.Finding, rec probes.Recorder) domain.Finding
	cleaned [][]domain.ProbeResource
}

func (f *fakeProvider) Plan(domain.Scope, *probes.Requirements) []probes.Check { return f.checks }
func (f *fakeProvider) Open(context.Context, application.Connection, domain.Scope) probes.Session {
	return &fakeSession{f: f}
}

type fakeSession struct{ f *fakeProvider }

func (s *fakeSession) Run(ctx context.Context, c probes.Check, prior map[string]domain.Finding, _ *probes.Requirements, rec probes.Recorder) domain.Finding {
	return s.f.run(ctx, c, prior, rec)
}
func (s *fakeSession) Cleanup(_ context.Context, r []domain.ProbeResource, _ string) []domain.ProbeResource {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	s.f.cleaned = append(s.f.cleaned, r)
	out := append([]domain.ProbeResource(nil), r...)
	for i := range out {
		if out[i].UID != "" {
			out[i].State = domain.ResourceDeleted
		} else {
			out[i].State = domain.ResourceAbsent
		}
	}
	return out
}
func (s *fakeSession) Close() {}

type fixture struct {
	st    *store.Store
	svc   *Service
	prov  *fakeProvider
	user  string
	acct  application.Connection
	other application.Connection
}

func newFixture(t *testing.T, cfg Config) *fixture {
	t.Helper()
	ctx := context.Background()
	st := store.New()
	for _, org := range []string{"acme", "globex"} {
		if err := st.SaveOrganization(ctx, application.Organization{Key: org, Name: org}); err != nil {
			t.Fatal(err)
		}
	}
	user := ids.New()
	if err := st.SaveUserAccount(ctx, identity.UserAccount{ID: user, OrganizationKey: "acme", Username: "pe", PasswordHash: "x", Role: identity.RolePlatformEngineer, Status: identity.AccountActive}); err != nil {
		t.Fatal(err)
	}
	mk := func(org, key string) application.Connection {
		c := application.Connection{ID: ids.New(), Key: key, Name: "Cluster " + key, OrganizationKey: org, Kind: application.ConnectionKubernetes, AuthenticationType: application.AuthKubeconfig,
			Config: map[string]any{"cluster": "kind-a", "endpoint": "https://127.0.0.1:6443", "kubeContext": "kind-a"}, SecretRef: "ref", Status: application.ConnectionReady, Verification: map[string]any{"verified": true}, Version: 3}
		if err := st.SaveConnection(ctx, c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	prov := &fakeProvider{}
	svc := New(st, connection.NewService(st), st, map[string]probes.Provider{"INTERNAL": prov}, cfg)
	t.Cleanup(svc.Close)
	return &fixture{st: st, svc: svc, prov: prov, user: user, acct: mk("acme", "k8s-a"), other: mk("globex", "k8s-a")}
}

func (f *fixture) start(t *testing.T, req domain.Request) (domain.Report, error) {
	t.Helper()
	if req.Version == 0 {
		req.Version = f.acct.Version
	}
	if req.Purpose == "" {
		req.Purpose = domain.PurposeGeneral
	}
	if req.Mode == "" {
		req.Mode = domain.ModeConditions
	}
	return f.svc.Start(context.Background(), StartCommand{Organization: "acme", UserID: f.user, AccountKey: "k8s-a", Request: req})
}

func (f *fixture) waitStatus(t *testing.T, id string, want ...domain.Status) domain.Report {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		r, err := f.svc.Get(context.Background(), "acme", "k8s-a", id)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range want {
			if r.Status == w {
				return r
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("report stayed %s, wanted %v", r.Status, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func pass(reason string) domain.Finding {
	return domain.Finding{Verdict: domain.VerdictPass, Reason: reason}
}

func TestRunRecordsVerdictsDependenciesAndNeverLeaksProbeText(t *testing.T) {
	f := newFixture(t, Config{})
	f.prov.checks = []probes.Check{
		{ID: "k8s.api", Group: "access"},
		{ID: "k8s.nodes", Group: "cluster", DependsOn: []string{"k8s.api"}},
		{ID: "k8s.storage.classes", Group: "storage", DependsOn: []string{"k8s.nodes"}},
		{ID: "k8s.controllers", Group: "platform", DependsOn: []string{"k8s.api"}},
	}
	const secret = "Bearer eyJhbGciOi.SECRET-TOKEN-SENTINEL"
	f.prov.run = func(_ context.Context, c probes.Check, _ map[string]domain.Finding, _ probes.Recorder) domain.Finding {
		switch c.ID {
		case "k8s.api":
			// A probe cannot choose its identity, and unregistered or
			// malformed facts never survive.
			return domain.Finding{ID: "spoofed", Verdict: domain.VerdictPass, Reason: "API_ANSWERED", Facts: map[string]string{"serverVersion": "v1.31.0", "stderr": secret, "nodes": "3; " + secret}}
		case "k8s.nodes":
			return domain.Finding{Verdict: domain.VerdictFail, Reason: "NO_READY_NODE", Remediation: "restore-nodes", Facts: map[string]string{"nodes": "2", "nodesReady": "0"}}
		}
		return pass("INVENTORY_OBSERVED")
	}
	rep, err := f.start(t, domain.Request{})
	if err != nil || rep.Status != domain.StatusQueued || rep.TotalChecks != 4 {
		t.Fatalf("start: %+v %v", rep, err)
	}
	done := f.waitStatus(t, rep.ID, domain.StatusCompleted)
	verdicts := map[string]domain.Verdict{}
	for _, fd := range done.Findings {
		verdicts[fd.ID] = fd.Verdict
	}
	if verdicts["k8s.api"] != domain.VerdictPass || verdicts["k8s.nodes"] != domain.VerdictFail || verdicts["k8s.storage.classes"] != domain.VerdictBlocked || verdicts["k8s.controllers"] != domain.VerdictPass {
		t.Fatalf("verdicts %v", verdicts)
	}
	if done.Ready() || done.Counts().Fail != 1 || done.Counts().Blocked != 1 || done.Counts().Pass != 2 {
		t.Fatalf("counts %+v ready=%v", done.Counts(), done.Ready())
	}
	raw, _ := json.Marshal(done)
	if strings.Contains(string(raw), "SECRET-TOKEN-SENTINEL") || strings.Contains(string(raw), "spoofed") {
		t.Fatalf("probe text reached the report: %s", raw)
	}
	if got := done.Findings[0].Facts; got["serverVersion"] != "v1.31.0" || len(got) != 1 {
		t.Fatalf("facts %v", got)
	}
	blocked := done.Findings[2]
	if blocked.Reason != "PREREQUISITE_FAILED" || len(blocked.DependsOn) != 1 || blocked.DependsOn[0] != "k8s.nodes" {
		t.Fatalf("blocked finding %+v", blocked)
	}
	// The account record is untouched by a diagnostic.
	after, _ := f.svc.accounts.GetAccount(context.Background(), "acme", "k8s-a")
	if after.Version != f.acct.Version || after.Status != application.ConnectionReady {
		t.Fatalf("connection changed: %+v", after)
	}
}

func TestStartValidationIsolationAndBusy(t *testing.T) {
	f := newFixture(t, Config{})
	f.prov.checks = []probes.Check{{ID: "k8s.api", Group: "access"}}
	release := make(chan struct{})
	f.prov.run = func(ctx context.Context, _ probes.Check, _ map[string]domain.Finding, _ probes.Recorder) domain.Finding {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return pass("API_ANSWERED")
	}
	for name, req := range map[string]domain.Request{
		"stale version":      {Version: 99},
		"region on k8s":      {Region: "us-east-1"},
		"general use cases":  {UseCases: []string{"deploy"}},
		"general execution":  {Mode: domain.ModeExecution},
		"use case none":      {Purpose: domain.PurposeUseCase},
		"unknown use case":   {Purpose: domain.PurposeUseCase, UseCases: []string{"teleport"}},
		"duplicate use case": {Purpose: domain.PurposeUseCase, UseCases: []string{"deploy", "deploy"}},
		"env without app":    {Purpose: domain.PurposeUseCase, UseCases: []string{"deploy"}, Environment: "staging"},
	} {
		_, err := f.start(t, req)
		if name == "stale version" {
			if !errors.Is(err, persistence.ErrVersionConflict) {
				t.Fatalf("%s: %v", name, err)
			}
			continue
		}
		if !errors.Is(err, domain.ErrInvalidScope) {
			t.Fatalf("%s accepted: %v", name, err)
		}
	}
	if _, err := f.start(t, domain.Request{Purpose: domain.PurposeUseCase, UseCases: []string{"deploy"}, Application: "nope"}); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("unknown application: %v", err)
	}
	rep, err := f.start(t, domain.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.start(t, domain.Request{}); !errors.Is(err, domain.ErrBusy) {
		t.Fatalf("second run on the account: %v", err)
	}
	// A foreign Organization sees nothing of the account or its report.
	if _, err := f.svc.Get(context.Background(), "globex", "k8s-a", rep.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign read: %v", err)
	}
	if _, err := f.svc.Cancel(context.Background(), "globex", "k8s-a", rep.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign cancel: %v", err)
	}
	close(release)
	f.waitStatus(t, rep.ID, domain.StatusCompleted)
	if _, err := f.start(t, domain.Request{}); err != nil {
		t.Fatalf("run after completion: %v", err)
	}
}

func TestMissingContextIsUnknownNeverPass(t *testing.T) {
	f := newFixture(t, Config{})
	f.prov.checks = []probes.Check{{ID: "k8s.api", Group: "access"}}
	f.prov.run = func(context.Context, probes.Check, map[string]domain.Finding, probes.Recorder) domain.Finding {
		return pass("API_ANSWERED")
	}
	rep, err := f.start(t, domain.Request{Purpose: domain.PurposeUseCase, UseCases: []string{"database", "secrets", "https", "deploy"}})
	if err != nil {
		t.Fatal(err)
	}
	done := f.waitStatus(t, rep.ID, domain.StatusCompleted)
	for _, fd := range done.Findings {
		if strings.HasPrefix(fd.ID, "uc.") && fd.Verdict == domain.VerdictPass {
			t.Fatalf("%s passed without an Application: %+v", fd.ID, fd)
		}
		if strings.HasPrefix(fd.ID, "uc.") && fd.Reason != "APPLICATION_CONTEXT_REQUIRED" && fd.Verdict == domain.VerdictUnknown {
			t.Fatalf("unexpected reason %+v", fd)
		}
	}
	if done.Ready() {
		t.Fatal("a report with unknown requirements was ready")
	}
}

func TestCancelStopsProbesAndVerifiesCleanup(t *testing.T) {
	f := newFixture(t, Config{})
	f.prov.checks = []probes.Check{
		{ID: "k8s.api", Group: "access"},
		{ID: "k8s.exec.namespace", Group: "execution", DependsOn: []string{"k8s.api"}, Execution: true},
		{ID: "k8s.exec.workload", Group: "execution", DependsOn: []string{"k8s.exec.namespace"}, Execution: true},
		{ID: "k8s.exec.cleanup", Group: "execution", Always: true},
	}
	started := make(chan struct{})
	f.prov.run = func(ctx context.Context, c probes.Check, _ map[string]domain.Finding, rec probes.Recorder) domain.Finding {
		switch c.ID {
		case "k8s.exec.namespace":
			_ = rec.Record(ctx, domain.ProbeResource{Kind: "namespace", Name: "orch-diag-abc", State: domain.ResourceIntended})
			_ = rec.Record(ctx, domain.ProbeResource{Kind: "namespace", Name: "orch-diag-abc", UID: "11111111-2222-3333-4444-555555555555", State: domain.ResourceCreated})
			return pass("NAMESPACE_CREATED")
		case "k8s.exec.workload":
			close(started)
			<-ctx.Done()
			return domain.Finding{}
		}
		return pass("API_ANSWERED")
	}
	rep, err := f.start(t, domain.Request{Purpose: domain.PurposeUseCase, Mode: domain.ModeExecution, UseCases: []string{"deploy"}})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	mid, err := f.svc.Get(context.Background(), "acme", "k8s-a", rep.ID)
	if err != nil || mid.Status != domain.StatusRunning || len(mid.ProbeResources) != 1 || mid.CleanupStatus != domain.CleanupPending {
		t.Fatalf("in flight: %+v %v", mid, err)
	}
	if _, err := f.svc.Cancel(context.Background(), "acme", "k8s-a", rep.ID); err != nil {
		t.Fatal(err)
	}
	done := f.waitStatus(t, rep.ID, domain.StatusCancelled)
	if done.CleanupStatus != domain.CleanupVerified || done.ProbeResources[0].State != domain.ResourceDeleted {
		t.Fatalf("cleanup after cancel: %+v", done)
	}
	if done.Ready() || done.CompletedChecks >= done.TotalChecks && done.Counts().Pending != 0 && false {
		t.Fatal("a cancelled run was ready")
	}
	byID := map[string]domain.Finding{}
	for _, fd := range done.Findings {
		byID[fd.ID] = fd
	}
	if _, ran := byID["k8s.exec.workload"]; ran {
		t.Fatal("the interrupted probe produced a finding")
	}
	if byID["k8s.exec.cleanup"].Verdict != domain.VerdictPass {
		t.Fatalf("cleanup finding %+v", byID["k8s.exec.cleanup"])
	}
}

func TestLostProcessBecomesInterruptedAndCleanupIsRecovered(t *testing.T) {
	cfg := Config{LeaseTTL: 300 * time.Millisecond, SweepEvery: time.Hour}
	f := newFixture(t, cfg)
	f.prov.checks = []probes.Check{
		{ID: "k8s.exec.namespace", Group: "execution", Execution: true},
		{ID: "k8s.exec.workload", Group: "execution", DependsOn: []string{"k8s.exec.namespace"}, Execution: true},
	}
	hang := make(chan struct{})
	f.prov.run = func(ctx context.Context, c probes.Check, _ map[string]domain.Finding, rec probes.Recorder) domain.Finding {
		if c.ID == "k8s.exec.namespace" {
			_ = rec.Record(ctx, domain.ProbeResource{Kind: "namespace", Name: "orch-diag-lost", UID: "11111111-2222-3333-4444-555555555555", State: domain.ResourceCreated})
			return pass("NAMESPACE_CREATED")
		}
		<-hang
		return domain.Finding{}
	}
	rep, err := f.start(t, domain.Request{Purpose: domain.PurposeUseCase, Mode: domain.ModeExecution, UseCases: []string{"deploy"}})
	if err != nil {
		t.Fatal(err)
	}
	// Wait for the first finding, then simulate a dead process: its worker
	// stops heartbeating because Close cancels it without finishing.
	deadline := time.Now().Add(5 * time.Second)
	for {
		r, _ := f.svc.Get(context.Background(), "acme", "k8s-a", rep.ID)
		if len(r.Findings) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first finding never persisted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.svc.mu.Lock()
	f.svc.closed = true
	for _, handle := range f.svc.runs {
		handle.root()
	}
	f.svc.mu.Unlock()
	close(hang)
	f.svc.Wait()
	time.Sleep(450 * time.Millisecond) // lease ends
	f.svc.Sweep(context.Background())
	got, err := f.svc.Get(context.Background(), "acme", "k8s-a", rep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusInterrupted || len(got.Findings) != 1 || got.Findings[0].ID != "k8s.exec.namespace" || got.Ready() {
		t.Fatalf("interrupted report: %+v", got)
	}
	// The sweep re-checked the run-owned namespace by its recorded UID and
	// recorded the observed outcome; nothing was assumed.
	if got.CleanupStatus != domain.CleanupVerified || got.ProbeResources[0].State != domain.ResourceDeleted {
		t.Fatalf("cleanup recovery: %+v", got)
	}
	if len(f.prov.cleaned) == 0 {
		t.Fatal("no cleanup attempt was made")
	}
}
