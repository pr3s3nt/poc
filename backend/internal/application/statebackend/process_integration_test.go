package statebackend_test

import (
	"context"
	"encoding/hex"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/statecipher"
	"orchestrator/internal/application/statebackend"
	deliveryhttp "orchestrator/internal/delivery/http"
	"orchestrator/internal/domain/tfresource"
)

// Environment of a helper backend process. Secrets travel only through the
// private child environment and owner-only files, never argv or stdout.
const (
	helperMode     = "ORCHESTRATOR_STATE_HELPER_MODE"
	helperDir      = "ORCHESTRATOR_STATE_HELPER_DIR"
	helperToken    = "ORCHESTRATOR_STATE_HELPER_TOKEN"
	helperStateKey = "ORCHESTRATOR_STATE_HELPER_STATE_KEY"
	helperLockID   = "ORCHESTRATOR_STATE_HELPER_LOCK_ID"
	helperName     = "ORCHESTRATOR_STATE_HELPER_NAME"
)

// TestStateBackendHelperProcess is the body of a child backend process. It is
// a no-op (and passes) unless started by TestTwoProcessLockAndStaleOwner.
func TestStateBackendHelperProcess(t *testing.T) {
	mode := os.Getenv(helperMode)
	if mode == "" {
		return
	}
	dir := os.Getenv(helperDir)
	name := os.Getenv(helperName)
	result := func(v string) {
		if err := os.WriteFile(filepath.Join(dir, name+".result"), []byte(v), 0o600); err != nil {
			t.Fatal("cannot record result")
		}
	}
	url, err := os.ReadFile(filepath.Join(dir, "db-url"))
	if err != nil {
		t.Fatal("cannot read database url file")
	}
	st, err := postgres.OpenState(context.Background(), strings.TrimSpace(string(url)))
	if err != nil {
		t.Fatal("cannot open database")
	}
	defer st.Close()
	cipher, err := statecipher.LoadKeyFile("key-1", filepath.Join(dir, "state-key"))
	if err != nil {
		t.Fatal("cannot load state key")
	}
	svc, err := statebackend.New(st, cipher)
	if err != nil {
		t.Fatal(err)
	}
	l, err := deliveryhttp.ServeTerraformStateLoopback(deliveryhttp.NewTerraformStateHandler(svc))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close(context.Background())
	token, key, lockID := os.Getenv(helperToken), os.Getenv(helperStateKey), os.Getenv(helperLockID)
	switch mode {
	case "lock":
		if err := os.WriteFile(filepath.Join(dir, name+".ready"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(30 * time.Second)
		for {
			if _, err := os.Stat(filepath.Join(dir, "go")); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("no start signal")
			}
			time.Sleep(time.Millisecond)
		}
		code, _ := call(t, l.Address, "LOCK", key, token, "", lockBody(lockID))
		result(strconv.Itoa(code))
	case "stale":
		post, _ := call(t, l.Address, http.MethodPost, key, token, lockID, stateDoc("lin", 9, marker))
		unlock, _ := call(t, l.Address, "UNLOCK", key, token, "", lockBody(lockID))
		result(strconv.Itoa(post) + " " + strconv.Itoa(unlock))
	}
}

func startHelper(t *testing.T, dir, mode, name, token, stateKey, lockID string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestStateBackendHelperProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), helperMode+"="+mode, helperDir+"="+dir, helperName+"="+name, helperToken+"="+token, helperStateKey+"="+stateKey, helperLockID+"="+lockID)
	cmd.Stdout, cmd.Stderr = nil, nil // discarded: nothing from the child is shown
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd
}

func waitFile(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			return string(b)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", filepath.Base(path))
	return ""
}

// TestTwoProcessLockAndStaleOwner: two separate OS processes, each its own
// backend with its own pool and loopback listener, race LOCK on one state
// after a shared start signal: exactly one wins. After recovery advanced the
// fence, a third process holding the old owner's capability can neither
// write nor unlock, and the lock survives.
func TestTwoProcessLockAndStaleOwner(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	// Each child backend connects only with the State login.
	if err := os.WriteFile(filepath.Join(dir, "db-url"), []byte(f.db.State), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state-key"), []byte(hex.EncodeToString(testKey(1))), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, op := f.claim(f.store, f.appKey, "staging", "proc-a")
	inst := f.instance(ctx, "redis.in-cluster#cache", "run-1")
	if _, err := f.attempt(ctx, inst, "run-1", tfresource.ActionCreate, tfresource.CreatedByRun); err != nil {
		t.Fatal(err)
	}
	tokA := f.token(ctx, inst, tfresource.CapabilityCreate)
	tokB := f.token(ctx, inst, tfresource.CapabilityCreate)
	a := startHelper(t, dir, "lock", "a", tokA, inst.StateKey, "lock-a")
	b := startHelper(t, dir, "lock", "b", tokB, inst.StateKey, "lock-b")
	waitFile(t, filepath.Join(dir, "a.ready"))
	waitFile(t, filepath.Join(dir, "b.ready"))
	if err := os.WriteFile(filepath.Join(dir, "go"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.Wait(); err != nil {
		t.Fatalf("process a failed: %v", err)
	}
	if err := b.Wait(); err != nil {
		t.Fatalf("process b failed: %v", err)
	}
	codes := []string{waitFile(t, filepath.Join(dir, "a.result")), waitFile(t, filepath.Join(dir, "b.result"))}
	winner, winTok := "", ""
	for i, c := range codes {
		switch c {
		case "200":
			if winner != "" {
				t.Fatal("both processes acquired the lock")
			}
			winner, winTok = []string{"lock-a", "lock-b"}[i], []string{tokA, tokB}[i]
		case "423":
		default:
			t.Fatalf("unexpected LOCK status %s", c)
		}
	}
	if winner == "" || f.count(`SELECT count(*) FROM terraform_state_locks WHERE lock_id=$1`, winner) != 1 {
		t.Fatal("no single lock winner persisted")
	}

	// Old owner stalls; recovery after confirmation advances the fence.
	if _, err := f.store.MarkInterrupted(context.Background(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.BeginRecovery(context.Background(), op.ID, "proc-recover", "operator confirmed stop"); err != nil {
		t.Fatal(err)
	}
	staleTok := "stale-process-token"
	f.exec(`INSERT INTO terraform_executor_capabilities(token_hash,organization_id,resource_instance_id,environment_operation_id,fencing_generation,action,expires_at) VALUES($1,$2::uuid,$3::uuid,$4::uuid,$5,'CREATE',now()+interval '1 hour')`, statebackend.HashToken(staleTok), inst.Identity.OrganizationID, inst.ID, op.ID, op.Fence)
	for name, tok := range map[string]string{"revoked": winTok, "stale": staleTok} {
		cmd := startHelper(t, dir, "stale", name, tok, inst.StateKey, winner)
		if err := cmd.Wait(); err != nil {
			t.Fatalf("%s process failed: %v", name, err)
		}
		got := waitFile(t, filepath.Join(dir, name+".result"))
		want := map[string]string{"revoked": "401 401", "stale": "403 403"}[name]
		if got != want {
			t.Errorf("%s owner write/unlock = %s, want %s", name, got, want)
		}
	}
	if f.count(`SELECT count(*) FROM terraform_state_locks WHERE lock_id=$1`, winner) != 1 || f.count(`SELECT count(*) FROM terraform_states`) != 0 {
		t.Fatal("stale process changed the lock or state")
	}
}
