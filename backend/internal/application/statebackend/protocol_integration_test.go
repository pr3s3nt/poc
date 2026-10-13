package statebackend_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/statecipher"
	"orchestrator/internal/application/statebackend"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

const marker = "PLAINTEXT-SECRET-7f3c9a"

// captureLogs routes the standard logger into a buffer for the test, so the
// test can prove that no state bytes or tokens are logged.
func captureLogs(t *testing.T) *bytes.Buffer {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

// TestStateProtocolPersistsEncryptedStateAcrossRestart covers GET/LOCK/POST/
// UNLOCK semantics, ciphertext at rest, serial/lineage rules, idempotent
// replay, restart/reopen, and no plaintext in logs or other tables.
func TestStateProtocolPersistsEncryptedStateAcrossRestart(t *testing.T) {
	logs := captureLogs(t)
	f := newFixture(t)
	ctx, op := f.claim(f.store, f.appKey, "staging", "proc-a")
	inst := f.instance(ctx, "redis.in-cluster#cache", "run-1")
	if _, err := f.attempt(ctx, inst, "run-1", tfresource.ActionCreate, tfresource.CreatedByRun); err != nil {
		t.Fatal(err)
	}
	tok := f.token(ctx, inst, tfresource.CapabilityCreate)
	key := inst.StateKey

	if code, _ := call(t, f.base, http.MethodGet, key, "", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("no credentials: %d", code)
	}
	if code, _ := call(t, f.base, http.MethodGet, key, "not-a-token", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("unknown token: %d", code)
	}
	if code, _ := call(t, f.base, http.MethodGet, key, tok, "", nil); code != http.StatusNoContent {
		t.Fatalf("GET before any state must be no-state: %d", code)
	}
	if n := f.count(`SELECT count(*) FROM terraform_states`); n != 0 {
		t.Fatalf("GET created state rows: %d", n)
	}
	// POST without holding the lock (also the -lock=false shape) is refused.
	if code, _ := call(t, f.base, http.MethodPost, key, tok, "", stateDoc("lin-1", 1, marker)); code != http.StatusConflict {
		t.Fatalf("unlocked write: %d", code)
	}
	if code, _ := call(t, f.base, "LOCK", key, tok, "", lockBody("lock-1")); code != http.StatusOK {
		t.Fatalf("lock: %d", code)
	}
	if code, _ := call(t, f.base, "LOCK", key, tok, "", lockBody("lock-1")); code != http.StatusOK {
		t.Fatalf("idempotent re-lock by same owner: %d", code)
	}
	code, body := call(t, f.base, "LOCK", key, tok, "", lockBody("lock-other"))
	if code != http.StatusLocked {
		t.Fatalf("second lock id: %d", code)
	}
	var info map[string]string
	if err := json.Unmarshal(body, &info); err != nil || info["ID"] != "lock-1" || strings.Contains(string(body), "host-secret") || strings.Contains(string(body), "/home/secret") {
		t.Fatalf("lock conflict metadata unsafe or wrong: %s", body)
	}
	if code, _ := call(t, f.base, http.MethodPost, key, tok, "wrong-lock", stateDoc("lin-1", 1, marker)); code != http.StatusConflict {
		t.Fatalf("wrong lock id: %d", code)
	}
	first := stateDoc("lin-1", 1, marker)
	if code, _ := call(t, f.base, http.MethodPost, key, tok, "lock-1", first); code != http.StatusOK {
		t.Fatalf("first write: %d", code)
	}
	// Identical replay at the same serial is acknowledged without a new revision.
	if code, _ := call(t, f.base, http.MethodPost, key, tok, "lock-1", first); code != http.StatusOK {
		t.Fatalf("identical replay: %d", code)
	}
	if v := f.count(`SELECT version FROM terraform_states`); v != 1 {
		t.Fatalf("replay advanced version to %d", v)
	}
	for name, doc := range map[string][]byte{
		"different bytes same serial": stateDoc("lin-1", 1, "changed"),
		"lineage change":              stateDoc("lin-2", 2, marker),
		"invalid document":            []byte(`{"serial":2}`),
	} {
		if code, _ := call(t, f.base, http.MethodPost, key, tok, "lock-1", doc); code != http.StatusConflict && code != http.StatusBadRequest {
			t.Errorf("%s accepted: %d", name, code)
		}
	}
	second := stateDoc("lin-1", 5, marker)
	if code, _ := call(t, f.base, http.MethodPost, key, tok, "lock-1", second); code != http.StatusOK {
		t.Fatalf("serial advance: %d", code)
	}
	if code, _ := call(t, f.base, http.MethodPost, key, tok, "lock-1", stateDoc("lin-1", 4, marker)); code != http.StatusConflict {
		t.Fatalf("serial regression: %d", code)
	}
	if code, _ := call(t, f.base, http.MethodDelete, key, tok, "", nil); code != http.StatusMethodNotAllowed {
		t.Fatalf("purge: %d", code)
	}
	if code, _ := call(t, f.base, "UNLOCK", key, tok, "", lockBody("lock-other")); code != http.StatusConflict {
		t.Fatalf("unlock with another lock id: %d", code)
	}
	if code, _ := call(t, f.base, "UNLOCK", key, tok, "", lockBody("lock-1")); code != http.StatusOK {
		t.Fatalf("unlock: %d", code)
	}

	// At rest: ciphertext only, fresh nonce per revision, AAD revision = version,
	// attempt records the serial; nothing else holds the plaintext.
	if n := f.count(`SELECT count(*) FROM terraform_states WHERE position(convert_to($1,'UTF8') in ciphertext)>0 OR position(convert_to('lin-1','UTF8') in ciphertext)>0`, marker); n != 0 {
		t.Fatal("ciphertext contains plaintext")
	}
	if n := f.count(`SELECT count(*) FROM terraform_states WHERE serial=5 AND version=2 AND aad_revision=2 AND key_id='key-1' AND length(nonce)=12`); n != 1 {
		t.Fatal("state row metadata")
	}
	for _, table := range []string{"terraform_resource_instances", "terraform_resource_attempts", "terraform_state_locks", "terraform_executor_capabilities", "environment_operations", "terraform_resource_dependencies"} {
		if n := f.count(`SELECT count(*) FROM `+table+` t WHERE t::text LIKE '%'||$1||'%' OR t::text LIKE '%'||$2||'%'`, marker, tok); n != 0 {
			t.Fatalf("%s holds plaintext state or token", table)
		}
	}
	if n := f.count(`SELECT count(*) FROM terraform_executor_capabilities WHERE token_hash=$1`, statebackend.HashToken(tok)); n != 1 {
		t.Fatal("capability not stored as hash")
	}
	attempts, err := f.state.ListAttempts(context.Background(), inst.ID)
	if err != nil || len(attempts) != 1 || attempts[0].StateSerial == nil || *attempts[0].StateSerial != 5 {
		t.Fatalf("attempt serial: %+v %v", attempts, err)
	}

	// Restart: a new pool and service with the same external key reads the state.
	f.store.Close()
	f.state.Close()
	reopened, err := postgres.OpenState(context.Background(), f.db.State)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	sameKey, _ := statecipher.New("key-1", testKey(1))
	base := f.serve(f.service(reopened, sameKey))
	code, got := call(t, base, http.MethodGet, key, tok, "", nil)
	if code != http.StatusOK || !bytes.Equal(got, second) {
		t.Fatalf("reopen read: %d", code)
	}
	if strings.Contains(logs.String(), marker) || strings.Contains(logs.String(), tok) {
		t.Fatal("logs contain state or token")
	}
	_ = op
}

// TestWrongOrMissingKeyFailsClosed: a different key or key ID never yields an
// empty state and never overwrites ciphertext it cannot read.
func TestWrongOrMissingKeyFailsClosed(t *testing.T) {
	f := newFixture(t)
	ctx, _ := f.claim(f.store, f.appKey, "staging", "proc-a")
	inst := f.instance(ctx, "redis.in-cluster#cache", "run-1")
	if _, err := f.attempt(ctx, inst, "run-1", tfresource.ActionCreate, tfresource.CreatedByRun); err != nil {
		t.Fatal(err)
	}
	tok := f.token(ctx, inst, tfresource.CapabilityCreate)
	call(t, f.base, "LOCK", inst.StateKey, tok, "", lockBody("l"))
	if code, _ := call(t, f.base, http.MethodPost, inst.StateKey, tok, "l", stateDoc("lin", 1, marker)); code != http.StatusOK {
		t.Fatalf("write: %d", code)
	}
	wrongKey, _ := statecipher.New("key-1", testKey(2))
	otherID, _ := statecipher.New("key-2", testKey(1))
	for name, c := range map[string]terraformstate.Cipher{"wrong key": wrongKey, "other key id": otherID} {
		base := f.serve(f.service(f.state, c))
		code, body := call(t, base, http.MethodGet, inst.StateKey, tok, "", nil)
		if code != http.StatusInternalServerError || strings.Contains(string(body), marker) {
			t.Errorf("%s GET: %d", name, code)
		}
		if code, _ := call(t, base, http.MethodPost, inst.StateKey, tok, "l", stateDoc("lin", 9, "overwrite")); code != http.StatusInternalServerError {
			t.Errorf("%s overwrite: %d", name, code)
		}
	}
	if n := f.count(`SELECT count(*) FROM terraform_states WHERE serial=1 AND version=1 AND key_id='key-1'`); n != 1 {
		t.Fatal("state changed under a wrong key")
	}
	if _, err := statebackend.New(f.state, nil); err == nil {
		t.Fatal("service built without a key")
	}
}

// TestScopeRejection: a capability opens only its own state; instances,
// capabilities, locks and attempts cannot cross Environment or Organization.
func TestScopeRejection(t *testing.T) {
	f := newFixture(t)
	stagingCtx, stagingOp := f.claim(f.store, f.appKey, "staging", "proc-a")
	prodCtx, prodOp := f.claim(f.store, f.appKey, "prod", "proc-b")
	otherCtx, _ := f.claim(f.store, f.other, "staging", "proc-c")
	a := f.instance(stagingCtx, "redis.in-cluster#cache", "run-1")
	b := f.instance(prodCtx, "redis.in-cluster#cache", "run-1")
	c := f.instance(otherCtx, "redis.in-cluster#cache", "run-1")
	if a.StateKey == b.StateKey || a.StateKey == c.StateKey {
		t.Fatal("same descriptor aliased state across Environments or Organizations")
	}
	tokA := f.token(stagingCtx, a, tfresource.CapabilityCreate)
	for _, k := range []string{b.StateKey, c.StateKey} {
		if code, _ := call(t, f.base, http.MethodGet, k, tokA, "", nil); code != http.StatusForbidden {
			t.Errorf("foreign state read: %d", code)
		}
		if code, _ := call(t, f.base, "LOCK", k, tokA, "", lockBody("x")); code != http.StatusForbidden {
			t.Errorf("foreign state lock: %d", code)
		}
	}
	// Owner of staging cannot issue for, register edges to, or attempt a prod instance.
	if _, _, err := f.svc.IssueCapability(stagingCtx, b.ID, tfresource.CapabilityCreate, time.Hour); !errors.Is(err, persistence.ErrNotFound) {
		t.Errorf("cross-Environment capability: %v", err)
	}
	if _, err := f.attempt(stagingCtx, b, "run-1", tfresource.ActionCreate, tfresource.CreatedByRun); !errors.Is(err, persistence.ErrNotFound) {
		t.Errorf("cross-Environment attempt: %v", err)
	}
	if _, err := f.state.RecordDependencies(stagingCtx, a.ID, a.Version, []string{b.ID}); !errors.Is(err, tfresource.ErrInvalid) {
		t.Errorf("cross-Environment edge: %v", err)
	}
	// Database guards hold even for a writer that bypasses the repository.
	for name, sql := range map[string]string{
		"lock":       `INSERT INTO terraform_state_locks(resource_instance_id,lock_id,environment_operation_id,owner,fencing_generation,lock_metadata,acquired_at,heartbeat_at,deadline) VALUES($1::uuid,'x',$2::uuid,'o',1,'{}',now(),now(),now())`,
		"capability": `INSERT INTO terraform_executor_capabilities(token_hash,organization_id,resource_instance_id,environment_operation_id,fencing_generation,action,expires_at) SELECT repeat('e',64),organization_id,$1::uuid,$2::uuid,1,'CREATE',now()+interval '1 hour' FROM terraform_resource_instances WHERE id=$1::uuid`,
	} {
		if err := f.execErr(sql, b.ID, stagingOp.ID); err == nil {
			t.Errorf("database accepted cross-Environment %s", name)
		}
	}
	if err := f.execErr(`INSERT INTO terraform_resource_instances(id,organization_id,application_id,environment_id,target_generation,descriptor,scope_type,scope_id,state_key,module_bundle_id,target_ref,physical_intent,created_by_run_id,status,version,created_at,updated_at) SELECT gen_random_uuid(),$1::uuid,application_id,environment_id,0,'redis.in-cluster#x','SHARED','x',repeat('d',64),module_bundle_id,'{}','{}','r','PENDING',1,now(),now() FROM terraform_resource_instances WHERE id=$2::uuid`, c.Identity.OrganizationID, a.ID); err == nil {
		t.Error("database accepted an instance whose Organization differs from its Environment")
	}
	_ = prodOp
}

// TestUniqueIdentityAndGeneration: registration is idempotent on the tuple
// and a different target generation is a different instance and state.
func TestUniqueIdentityAndGeneration(t *testing.T) {
	f := newFixture(t)
	ctx, _ := f.claim(f.store, f.appKey, "staging", "proc-a")
	first := f.instance(ctx, "postgres.in-cluster#db", "run-1")
	again := f.instance(ctx, "postgres.in-cluster#db", "run-2")
	if again.ID != first.ID || again.StateKey != first.StateKey || again.CreatedByRunID != "run-1" {
		t.Fatal("second registration did not return the original identity")
	}
	if _, err := f.state.RegisterInstance(ctx, terraformstate.NewInstance{TargetGeneration: 1, Descriptor: "postgres.in-cluster#db", ScopeType: "SHARED", ScopeID: "postgres.in-cluster#db", ModuleBundleID: f.bundle, TargetRef: map[string]any{}, PhysicalIntent: map[string]any{}, CreatedByRunID: "run-1"}); !errors.Is(err, tfresource.ErrInvalid) {
		t.Fatalf("unallocated generation accepted: %v", err)
	}
	f.exec(`UPDATE environments SET generation_high=1 WHERE id=$1::uuid`, first.Identity.EnvironmentID)
	gen1, err := f.state.RegisterInstance(ctx, terraformstate.NewInstance{TargetGeneration: 1, Descriptor: "postgres.in-cluster#db", ScopeType: "SHARED", ScopeID: "postgres.in-cluster#db", ModuleBundleID: f.bundle, TargetRef: map[string]any{}, PhysicalIntent: map[string]any{}, CreatedByRunID: "run-3"})
	if err != nil || gen1.ID == first.ID || gen1.StateKey == first.StateKey {
		t.Fatalf("generation 1 aliased generation 0: %v", err)
	}
	if err := f.execErr(`INSERT INTO terraform_resource_instances(id,organization_id,application_id,environment_id,target_generation,descriptor,scope_type,scope_id,state_key,module_bundle_id,target_ref,physical_intent,created_by_run_id,status,version,created_at,updated_at) SELECT gen_random_uuid(),organization_id,application_id,environment_id,0,descriptor,scope_type,scope_id,repeat('f',64),module_bundle_id,'{}','{}','r','PENDING',1,now(),now() FROM terraform_resource_instances WHERE id=$1::uuid`, first.ID); err == nil {
		t.Fatal("duplicate identity tuple accepted")
	}
	for name, sql := range map[string]string{
		"identity":        `UPDATE terraform_resource_instances SET descriptor='x.y#z' WHERE id=$1::uuid`,
		"target snapshot": `UPDATE terraform_resource_instances SET target_ref='{"connection":"other"}' WHERE id=$1::uuid`,
		"physical intent": `UPDATE terraform_resource_instances SET physical_intent='{}' WHERE id=$1::uuid`,
		"run provenance":  `UPDATE terraform_resource_instances SET created_by_run_id='x' WHERE id=$1::uuid`,
		"delete":          `DELETE FROM terraform_resource_instances WHERE id=$1::uuid`,
	} {
		if err := f.execErr(sql, first.ID); err == nil {
			t.Errorf("instance %s changed", name)
		}
	}
}

// TestTwoConnectionLockRace: two Store pools and listeners in this process
// race LOCK for one state with different lock IDs; exactly one wins every
// round. Literal two-process evidence is TestTwoProcessLockAndStaleOwner.
func TestTwoConnectionLockRace(t *testing.T) {
	f := newFixture(t)
	second, err := postgres.OpenState(context.Background(), f.db.State)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	baseB := f.serve(f.service(second, f.cipher))
	ctx, _ := f.claim(f.store, f.appKey, "staging", "proc-a")
	inst := f.instance(ctx, "redis.in-cluster#cache", "run-1")
	tokA := f.token(ctx, inst, tfresource.CapabilityCreate)
	tokB := f.token(ctx, inst, tfresource.CapabilityCreate)
	for round := 0; round < 10; round++ {
		var wg sync.WaitGroup
		start := make(chan struct{})
		codes := make([]int, 2)
		for i, target := range []struct{ base, tok, id string }{{f.base, tokA, "lock-a"}, {baseB, tokB, "lock-b"}} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				codes[i], _ = call(t, target.base, "LOCK", inst.StateKey, target.tok, "", lockBody(target.id))
			}()
		}
		close(start)
		wg.Wait()
		wins := 0
		for _, c := range codes {
			if c == http.StatusOK {
				wins++
			} else if c != http.StatusLocked {
				t.Fatalf("round %d: unexpected %d", round, c)
			}
		}
		if wins != 1 || f.count(`SELECT count(*) FROM terraform_state_locks`) != 1 {
			t.Fatalf("round %d: %d winners", round, wins)
		}
		winner := "lock-a"
		tok := tokA
		if codes[1] == http.StatusOK {
			winner, tok = "lock-b", tokB
		}
		if code, _ := call(t, f.base, "UNLOCK", inst.StateKey, tok, "", lockBody(winner)); code != http.StatusOK {
			t.Fatalf("round %d unlock: %d", round, code)
		}
	}
}

// TestStaleFenceCannotWriteOrUnlockAndExpiryNeverUnlocks covers interruption,
// recovery fencing, capability revocation, explicit recovered unlock, and that
// neither capability expiry nor heartbeat staleness releases a lock or claim.
func TestStaleFenceCannotWriteOrUnlockAndExpiryNeverUnlocks(t *testing.T) {
	f := newFixture(t)
	ctx, op := f.claim(f.store, f.appKey, "staging", "proc-a")
	inst := f.instance(ctx, "redis.in-cluster#cache", "run-1")
	if _, err := f.attempt(ctx, inst, "run-1", tfresource.ActionCreate, tfresource.CreatedByRun); err != nil {
		t.Fatal(err)
	}
	// Short capability on a controllable clock.
	now := time.Now().UTC()
	clock := now
	f.svc.SetClock(func() time.Time { return clock })
	tok, _, err := f.svc.IssueCapability(ctx, inst.ID, tfresource.CapabilityCreate, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	call(t, f.base, "LOCK", inst.StateKey, tok, "", lockBody("old-lock"))
	if code, _ := call(t, f.base, http.MethodPost, inst.StateKey, tok, "old-lock", stateDoc("lin", 1, marker)); code != http.StatusOK {
		t.Fatalf("write: %d", code)
	}
	clock = now.Add(2 * time.Minute) // capability and lock deadline passed
	for _, m := range []string{http.MethodGet, "UNLOCK"} {
		if code, _ := call(t, f.base, m, inst.StateKey, tok, "", lockBody("old-lock")); code != http.StatusForbidden {
			t.Errorf("expired capability %s: %d", m, code)
		}
	}
	if f.count(`SELECT count(*) FROM terraform_state_locks WHERE deadline<now()+interval '2 minutes'`) != 1 {
		t.Fatal("expiry released the lock")
	}
	// Heartbeat staleness marks INTERRUPTED but keeps claim and lock.
	if n, err := f.store.MarkInterrupted(context.Background(), time.Now().Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("mark interrupted: %d %v", n, err)
	}
	if _, err := f.store.ClaimEnvironment(context.Background(), persistence.OperationClaim{ApplicationKey: f.appKey, EnvironmentKey: "staging", Owner: "proc-z", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Hour)}); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("interrupted claim was released: %v", err)
	}
	if f.count(`SELECT count(*) FROM terraform_state_locks`) != 1 {
		t.Fatal("interruption released the lock")
	}
	f.svc.SetClock(func() time.Time { return time.Now().UTC() })
	if _, _, err := f.svc.IssueCapability(ctx, inst.ID, tfresource.CapabilityCreate, time.Hour); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("interrupted owner issued a capability: %v", err)
	}

	// Manual recovery after confirmation: fence advances, old capabilities revoked.
	recovered, err := f.store.BeginRecovery(context.Background(), op.ID, "proc-recover", "operator confirmed process group stopped")
	if err != nil || recovered.Fence != op.Fence+1 {
		t.Fatalf("recovery: %v", err)
	}
	if f.count(`SELECT count(*) FROM terraform_executor_capabilities`) != 0 {
		t.Fatal("old capabilities survived recovery")
	}
	if code, _ := call(t, f.base, http.MethodGet, inst.StateKey, tok, "", nil); code != http.StatusUnauthorized {
		t.Errorf("revoked capability: %d", code)
	}
	// A stale-fence capability that bypassed revocation is still refused.
	staleTok := "stale-token-value"
	f.exec(`INSERT INTO terraform_executor_capabilities(token_hash,organization_id,resource_instance_id,environment_operation_id,fencing_generation,action,expires_at) VALUES($1,$2::uuid,$3::uuid,$4::uuid,$5,'CREATE',now()+interval '1 hour')`, statebackend.HashToken(staleTok), inst.Identity.OrganizationID, inst.ID, op.ID, op.Fence)
	for _, m := range []string{http.MethodGet, "LOCK", "UNLOCK"} {
		if code, _ := call(t, f.base, m, inst.StateKey, staleTok, "", lockBody("old-lock")); code != http.StatusForbidden {
			t.Errorf("stale fence %s: %d", m, code)
		}
	}
	if code, _ := call(t, f.base, http.MethodPost, inst.StateKey, staleTok, "old-lock", stateDoc("lin", 2, marker)); code != http.StatusForbidden {
		t.Errorf("stale fence write: %d", code)
	}
	if _, err := f.state.SetInstanceStatus(ctx, inst.ID, inst.Version, tfresource.InstanceReady); !errors.Is(err, persistence.ErrOperationLost) {
		t.Errorf("stale owner metadata write: %v", err)
	}
	if err := f.store.ReleaseOperation(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}, environment.OpFailed, ""); !errors.Is(err, persistence.ErrOperationLost) {
		t.Errorf("stale owner release: %v", err)
	}

	newCtx := ownerCtx(recovered)
	newTok := f.token(newCtx, inst, tfresource.CapabilityCreate)
	if code, _ := call(t, f.base, "LOCK", inst.StateKey, newTok, "", lockBody("new-lock")); code != http.StatusLocked {
		t.Errorf("new owner took a held lock: %d", code)
	}
	if code, _ := call(t, f.base, http.MethodPost, inst.StateKey, newTok, "old-lock", stateDoc("lin", 2, marker)); code != http.StatusConflict {
		t.Errorf("new fence wrote under the old fence's lock: %d", code)
	}
	if code, _ := call(t, f.base, "UNLOCK", inst.StateKey, newTok, "", lockBody("old-lock")); code != http.StatusConflict {
		t.Errorf("unlock across fences: %d", code)
	}
	if err := f.store.ReleaseOperation(context.Background(), persistence.Owner{OperationID: recovered.ID, Owner: recovered.Owner, Fence: recovered.Fence}, environment.OpFailed, ""); !errors.Is(err, persistence.ErrStateLocksHeld) {
		t.Fatalf("claim released while a state lock is held: %v", err)
	}
	if err := f.state.ReleaseRecoveredLock(ctx, inst.ID); !errors.Is(err, persistence.ErrOperationLost) {
		t.Errorf("stale owner released recovered lock: %v", err)
	}
	if err := f.state.ReleaseRecoveredLock(newCtx, inst.ID); err != nil {
		t.Fatalf("recovered unlock: %v", err)
	}
	if code, _ := call(t, f.base, "LOCK", inst.StateKey, newTok, "", lockBody("new-lock")); code != http.StatusOK {
		t.Fatalf("relock: %d", code)
	}
	if code, _ := call(t, f.base, http.MethodPost, inst.StateKey, newTok, "new-lock", stateDoc("lin", 2, marker)); code != http.StatusOK {
		t.Fatalf("recovered write: %d", code)
	}
	if code, _ := call(t, f.base, "UNLOCK", inst.StateKey, newTok, "", lockBody("new-lock")); code != http.StatusOK {
		t.Fatalf("recovered unlock: %d", code)
	}
	if err := f.store.ReleaseOperation(context.Background(), persistence.Owner{OperationID: recovered.ID, Owner: recovered.Owner, Fence: recovered.Fence}, environment.OpRecovered, ""); err != nil {
		t.Fatalf("release: %v", err)
	}
	if code, _ := call(t, f.base, http.MethodGet, inst.StateKey, newTok, "", nil); code != http.StatusUnauthorized {
		t.Errorf("capability outlived its operation: %d", code)
	}
}
