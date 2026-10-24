package accountdiag

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"orchestrator/internal/adapters/credentialmemory"
	k8s "orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/connection"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/ids"
	probes "orchestrator/internal/ports/diagnostics"
)

// TestRealKindProbes runs the real Kubernetes provider against a run-owned kind
// cluster named by DIAGNOSTICS_KIND_KUBECONFIG / DIAGNOSTICS_KIND_CONTEXT. It
// is skipped without them; it never touches a cluster it was not given.
func TestRealKindProbes(t *testing.T) {
	path, kubeContext := os.Getenv("DIAGNOSTICS_KIND_KUBECONFIG"), os.Getenv("DIAGNOSTICS_KIND_CONTEXT")
	if path == "" || kubeContext == "" {
		t.Skip("set DIAGNOSTICS_KIND_KUBECONFIG and DIAGNOSTICS_KIND_CONTEXT to run against an owned kind cluster")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st, creds := store.New(), credentialmemory.New()
	_ = st.SaveOrganization(ctx, application.Organization{Key: "acme", Name: "acme"})
	user := ids.New()
	_ = st.SaveUserAccount(ctx, identity.UserAccount{ID: user, OrganizationKey: "acme", Username: "pe", PasswordHash: "x", Role: identity.RolePlatformEngineer, Status: identity.AccountActive})
	conns := connection.NewService(st)
	conns.SetKubeconfigRegistration(k8s.ConnectionVerifier{}, creds)
	account, err := conns.RegisterKubeconfig(ctx, "acme", connection.RegisterKubeconfigCommand{Name: "Kind", Kubeconfig: string(raw), Context: kubeContext})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := New(st, conns, st, map[string]probes.Provider{"INTERNAL": k8s.Diagnostics{Credentials: creds}}, Config{})
	defer svc.Close()
	run := func(req domain.Request) domain.Report {
		req.Version = account.Version
		rep, err := svc.Start(ctx, StartCommand{Organization: "acme", UserID: user, AccountKey: account.Key, Request: req})
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(4 * time.Minute)
		for time.Now().Before(deadline) {
			cur, _ := svc.Get(ctx, "acme", account.Key, rep.ID)
			if cur.Status.Terminal() {
				out, _ := json.MarshalIndent(cur.Findings, "", " ")
				t.Logf("%s %s cleanup=%s resources=%+v\n%s", req.Purpose, cur.Status, cur.CleanupStatus, cur.ProbeResources, out)
				return cur
			}
			time.Sleep(time.Second)
		}
		t.Fatal("run did not finish")
		return domain.Report{}
	}
	general := run(domain.Request{Purpose: domain.PurposeGeneral, Mode: domain.ModeConditions})
	if general.Status != domain.StatusCompleted {
		t.Fatalf("general: %s", general.Status)
	}
	exec := run(domain.Request{Purpose: domain.PurposeUseCase, Mode: domain.ModeExecution, UseCases: []string{"deploy"}})
	if exec.CleanupStatus != domain.CleanupVerified {
		t.Fatalf("deploy execution cleanup: %s %+v", exec.CleanupStatus, exec.ProbeResources)
	}
	db := run(domain.Request{Purpose: domain.PurposeUseCase, Mode: domain.ModeExecution, UseCases: []string{"database"}})
	for _, f := range db.Findings {
		if f.ID == "k8s.exec.volume" && f.Verdict == domain.VerdictPass {
			t.Fatal("database execution passed without resolved storage requirements")
		}
	}
}
