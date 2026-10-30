package postgres

import (
	"context"
	"testing"
	"time"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func TestDriftRetentionPreservesActiveWorkAndImmutableBaselines(t *testing.T) {
	f := newStateFixture(t)
	ctx := context.Background()
	owner := persistence.Owner{OperationID: f.op.ID, Owner: f.op.Owner, Fence: f.op.Fence}
	old, recent, baseline := ids.New(), ids.New(), ids.New()
	for _, a := range []persistence.CipherArtifact{
		{ID: old, OrganizationID: f.inst.Identity.OrganizationID, OwnerID: f.inst.ID, Purpose: "TERRAFORM_REVIEW", KeyID: "key", Nonce: make([]byte, 12), Ciphertext: []byte("opaque-expired-saved-plan"), AADVersion: 1, CreatedAt: time.Now().Add(-25 * time.Hour)},
		{ID: recent, OrganizationID: f.inst.Identity.OrganizationID, OwnerID: f.inst.ID, Purpose: "TERRAFORM_REVIEW", KeyID: "key", Nonce: make([]byte, 12), Ciphertext: []byte("opaque-current-saved-plan"), AADVersion: 1, CreatedAt: time.Now()},
	} {
		if err := f.state.PutRepairArtifact(persistence.WithOwner(ctx, owner), a); err != nil {
			t.Fatal(err)
		}
	}
	sqlOn(t, f.db.Admin, `INSERT INTO reconciliation_private_artifacts(id,organization_id,owner_id,purpose,key_id,nonce,ciphertext,aad_version,created_at) VALUES('`+baseline+`','`+f.inst.Identity.OrganizationID+`','`+f.inst.ID+`','TERRAFORM_BASELINE','key',decode(repeat('00',12),'hex'),decode(repeat('00',20),'hex'),1,now()-interval '40 days')`)
	sqlOn(t, f.db.Admin, `UPDATE terraform_resource_instances SET status='READY' WHERE id='`+f.inst.ID+`'`)
	scans, plans, err := PruneDriftHistory(ctx, f.db.Owner)
	if err != nil || scans != 0 || plans != 0 {
		t.Fatal("active operation/lock was not retained", scans, plans, err)
	}
	if _, _, err := PruneDriftHistory(ctx, f.db.State); err == nil {
		t.Fatal("runtime login gained owner maintenance")
	}
	if err := f.state.DeleteLock(persistence.WithOwner(ctx, owner), f.inst.ID, "lock-1", owner.OperationID, owner.Fence); err != nil {
		t.Fatal(err)
	}
	if err := f.console.ReleaseOperation(ctx, owner, environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}

	sqlOn(t, f.db.Admin, `UPDATE terraform_resource_instances SET status='NEEDS_ATTENTION' WHERE id='`+f.inst.ID+`'`)
	_, plans, err = PruneDriftHistory(ctx, f.db.Owner)
	if err != nil || plans != 0 {
		t.Fatal("unresolved instance lost private recovery plan", plans, err)
	}
	sqlOn(t, f.db.Admin, `UPDATE terraform_resource_instances SET status='READY' WHERE id='`+f.inst.ID+`'`)
	env, err := f.console.GetEnvironment(ctx, "app", "staging")
	if err != nil {
		t.Fatal(err)
	}
	oldScan, recentScan, auditScan := ids.New(), ids.New(), ids.New()
	for n, id := range []string{oldScan, recentScan, auditScan} {
		age := "31 days"
		if n == 1 {
			age = "1 day"
		}
		sqlOn(t, f.db.Admin, `INSERT INTO drift_scans(id,organization_id,environment_id,request_id,request_digest,environment_version,status,result,scope,findings,version,created_at,finished_at) VALUES('`+id+`','`+f.inst.Identity.OrganizationID+`','`+env.ID+`','`+ids.New()+`',repeat('a',64),1,'SKIPPED','UNKNOWN','[]','[]',1,now()-interval '`+age+`',now()-interval '`+age+`')`)
	}

	actor := identity.UserAccount{ID: ids.New(), OrganizationKey: "acme", Username: "retention-reviewer", PasswordHash: "opaque-test-hash", Role: identity.RolePlatformEngineer, Status: identity.AccountActive}
	if err := f.console.SaveUserAccount(ctx, actor); err != nil {
		t.Fatal(err)
	}
	reviewOp, err := f.console.ClaimEnvironment(ctx, persistence.OperationClaim{ID: ids.New(), ApplicationKey: "app", EnvironmentKey: "staging", Kind: environment.OpDriftRepair, Owner: "retention-review", Deadline: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	reviewID := ids.New()
	sqlOn(t, f.db.Admin, `INSERT INTO drift_repair_reviews(id,organization_id,environment_id,scan_id,actor_id,request_id,request_digest,operation_id,status,safe_review,private_refs,token_hash,token_cipher,environment_version,version,expires_at,created_at) VALUES('`+reviewID+`','`+f.inst.Identity.OrganizationID+`','`+env.ID+`','`+auditScan+`','`+actor.ID+`','`+ids.New()+`',repeat('b',64),'`+reviewOp.ID+`','BLOCKED','{"id":"`+reviewID+`","status":"BLOCKED"}','{}',repeat('c',64),'{}',1,1,now()-interval '31 days',now()-interval '31 days')`)
	if err := f.console.ReleaseOperation(ctx, persistence.Owner{OperationID: reviewOp.ID, Owner: reviewOp.Owner, Fence: reviewOp.Fence}, environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	scans, plans, err = PruneDriftHistory(ctx, f.db.Owner)
	if err != nil || scans != 1 || plans != 1 {
		t.Fatal("settled expiry did not remove exact eligible rows", scans, plans, err)
	}
	db := connectAs(t, f.db.Owner)
	for id, want := range map[string]int{old: 0, recent: 1, baseline: 1} {
		var count int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM reconciliation_private_artifacts WHERE id=$1`, id).Scan(&count); err != nil || count != want {
			t.Fatal("artifact retention boundary", count, want, err)
		}
	}

	var auditRetained int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM drift_scans WHERE id=$1`, auditScan).Scan(&auditRetained); err != nil || auditRetained != 1 {
		t.Fatal("review-referenced safe scan removed", err)
	}
	var retained int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM drift_scans WHERE id=$1`, recentScan).Scan(&retained); err != nil || retained != 1 {
		t.Fatal("recent scan removed", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM reconciliation_private_artifacts WHERE id=$1`, baseline); err == nil {
		t.Fatal("owner retention weakened immutable execution baseline")
	}
	scans, plans, err = PruneDriftHistory(ctx, f.db.Owner)
	if err != nil || scans != 0 || plans != 0 {
		t.Fatal("retention not idempotent", scans, plans, err)
	}
}
