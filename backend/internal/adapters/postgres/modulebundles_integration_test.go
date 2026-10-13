package postgres

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/platform/ids"
)

// Registry persistence on real PostgreSQL (option A logins): insert-only
// bundles, reopen, the revision-path pin under concurrency, immutability
// and constraints. These tests fail under REQUIRE_POSTGRES=1 without a
// test server.

func testBundle(digest string) tfmodule.Bundle {
	return tfmodule.Bundle{
		ID:               ids.New(),
		Ref:              tfmodule.SourceRef{SourceID: "acme", ModuleID: "redis", Revision: strings.Repeat("a", 40), Subdirectory: "modules/redis", ContentSHA256: digest},
		TerraformVersion: "1.9.8", TerraformBinarySHA256: strings.Repeat("b", 64), ProviderLockDigest: strings.Repeat("c", 64), Status: tfmodule.StatusVerified,
		Contract: tfmodule.Contract{
			SchemaVersion: tfmodule.ContractSchemaVersion, SourceKind: tfmodule.SourceKindGit, ResourceTypes: []string{"redis"},
			Inputs:      map[string]tfmodule.Input{"size": {Type: "number", HasDefault: true, Default: float64(1)}},
			Outputs:     map[string]tfmodule.Output{"host": {Type: "string"}, "password": {Type: "string", Sensitive: true}},
			Transitive:  []tfmodule.TransitiveModule{{SourceID: "shared", Revision: strings.Repeat("e", 40), Path: "net", ContentSHA256: strings.Repeat("f", 64)}},
			ModuleCalls: []tfmodule.ModuleCall{{Key: "net", Source: "git::https://git.example.com/shared.git//net?ref=" + strings.Repeat("e", 40), Transitive: 0}},
		},
		Providers: map[string]tfmodule.ProviderRequirement{"aws": {Source: "registry.terraform.io/hashicorp/aws", Version: "5.80.0", Constraints: ">= 5.0", Binding: tfmodule.BindingAWS, Hashes: []string{"h1:x", "zh:y"}}},
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

func TestModuleBundleInsertReopenAndRead(t *testing.T) {
	ctx := context.Background()
	st, console := openFresh(t)
	b := testBundle(strings.Repeat("d", 64))
	stored, err := st.InsertModuleBundle(ctx, b)
	if err != nil || stored.ID != b.ID || !stored.SameVerifiedContent(b) {
		t.Fatalf("insert: %+v %v", stored, err)
	}
	st.Close()

	reopened, err := Open(ctx, console)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetModuleBundle(ctx, b.Ref)
	if err != nil || got.ID != b.ID || !got.SameVerifiedContent(b) || !got.CreatedAt.Equal(b.CreatedAt) {
		t.Fatalf("reopened bundle: %+v %v", got, err)
	}
	list, err := reopened.ListModuleBundles(ctx)
	if err != nil || len(list) != 1 || list[0].ID != b.ID {
		t.Fatalf("list: %+v %v", list, err)
	}
	other := b.Ref
	other.Revision = strings.Repeat("9", 40)
	if _, err := reopened.GetModuleBundle(ctx, other); err == nil {
		t.Fatal("an unknown pin was found")
	}

	// Re-inserting the same pin never changes the stored row: the second
	// caller receives the stored bundle and detects the difference.
	conflicting := testBundle(strings.Repeat("d", 64))
	conflicting.Contract.Inputs["size"] = tfmodule.Input{Type: "number", HasDefault: true, Default: float64(9)}
	again, err := reopened.InsertModuleBundle(ctx, conflicting)
	if err != nil || again.ID != b.ID || again.SameVerifiedContent(conflicting) {
		t.Fatalf("same pin, other contract: %+v %v", again, err)
	}
	drift := testBundle(strings.Repeat("7", 64))
	kept, err := reopened.InsertModuleBundle(ctx, drift)
	if err != nil || kept.ID != b.ID || kept.Ref.ContentSHA256 != b.Ref.ContentSHA256 {
		t.Fatalf("same revision, other digest: %+v %v", kept, err)
	}
	if n := countBundles(t, reopened); n != 1 {
		t.Fatalf("rows = %d", n)
	}
}

func countBundles(t *testing.T, st *Store) int {
	t.Helper()
	list, err := st.ListModuleBundles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return len(list)
}

// Two transactions register different digests for one revision path at the
// same time: the second insert waits for the first, then reads the
// committed bundle (and the caller rejects the drift); after a rollback the
// waiting insert stores its own bundle. Never two digests for one pin.
func TestModuleBundleConcurrentDifferentDigests(t *testing.T) {
	for _, commitFirst := range []bool{true, false} {
		name := map[bool]string{true: "first commits", false: "first rolls back"}[commitFirst]
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			first, console := openFresh(t)
			second, err := Open(ctx, console)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()
			a, b := testBundle(strings.Repeat("1", 64)), testBundle(strings.Repeat("2", 64))
			inserted := make(chan struct{})
			release := make(chan struct{})
			firstDone := make(chan error, 1)
			go func() {
				firstDone <- first.Transact(ctx, func(ctx context.Context) error {
					if _, err := first.InsertModuleBundle(ctx, a); err != nil {
						return err
					}
					close(inserted)
					<-release
					if !commitFirst {
						return errors.New("rollback")
					}
					return nil
				})
			}()
			<-inserted
			var got tfmodule.Bundle
			var secondErr error
			var wg sync.WaitGroup
			secondDone := make(chan struct{})
			wg.Add(1)
			go func() {
				defer wg.Done()
				secondErr = second.Transact(ctx, func(ctx context.Context) error {
					var err error
					got, err = second.InsertModuleBundle(ctx, b)
					return err
				})
				close(secondDone)
			}()
			select {
			case <-secondDone:
				t.Fatal("the second insert did not wait for the first transaction")
			case <-time.After(300 * time.Millisecond):
			}
			close(release)
			wg.Wait()
			firstErr := <-firstDone
			if commitFirst {
				if firstErr != nil || secondErr != nil || got.ID != a.ID || got.Ref.ContentSHA256 != a.Ref.ContentSHA256 || got.SameVerifiedContent(b) {
					t.Fatalf("first=%v second=%v got=%+v", firstErr, secondErr, got.Ref)
				}
			} else if secondErr != nil || got.ID != b.ID {
				t.Fatalf("after rollback: second=%v got=%+v", secondErr, got.Ref)
			}
			if n := countBundles(t, first); n != 1 {
				t.Fatalf("rows = %d", n)
			}
		})
	}
}

func TestModuleBundlesAreImmutableConstrainedAndGranted(t *testing.T) {
	ctx := context.Background()
	db := freshDatabase(t)
	st, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	b := testBundle(strings.Repeat("d", 64))
	if _, err := st.InsertModuleBundle(ctx, b); err != nil {
		t.Fatal(err)
	}
	expect := func(url, sql, code string) {
		t.Helper()
		conn := connectAs(t, url)
		_, err := conn.Exec(ctx, sql)
		if got := codeOf(t, err); got != code {
			t.Fatalf("%s: want %s, got %s (%v)", sql, code, got, err)
		}
	}
	// The owner holds every table privilege; the trigger still refuses.
	expect(db.Owner, `UPDATE terraform_module_bundles SET status='VERIFIED'`, "23514")
	expect(db.Owner, `UPDATE terraform_module_bundles SET content_sha256=repeat('e',64)`, "23514")
	expect(db.Owner, `DELETE FROM terraform_module_bundles`, "23514")
	// Console: SELECT/INSERT only; State: no bundle privilege.
	expect(db.Console, `UPDATE terraform_module_bundles SET status='VERIFIED'`, "42501")
	expect(db.Console, `DELETE FROM terraform_module_bundles`, "42501")
	expect(db.State, `SELECT id FROM terraform_module_bundles`, "42501")
	insert := func(source, module, revision, subdir, digest, version, status string) string {
		return `INSERT INTO terraform_module_bundles(id,source_id,module_id,revision,subdirectory,content_sha256,terraform_version,terraform_binary_sha256,provider_lock_digest,contract,provider_requirements,status,created_at,updated_at) VALUES(gen_random_uuid(),'` +
			source + `','` + module + `','` + revision + `','` + subdir + `','` + digest + `','` + version + `',repeat('b',64),repeat('c',64),'{}','{}','` + status + `',now(),now())`
	}
	full, digest := strings.Repeat("a", 40), strings.Repeat("d", 64)
	expect(db.State, insert("acme", "other", full, "x", digest, "1.9.8", "VERIFIED"), "42501")
	expect(db.Console, insert("acme", "redis", full, "modules/redis", strings.Repeat("e", 64), "1.9.8", "VERIFIED"), "23505")
	for name, sql := range map[string]string{
		"revoked status":   insert("acme", "m1", full, "x", digest, "1.9.8", "REVOKED"),
		"short digest":     insert("acme", "m2", full, "x", "abc", "1.9.8", "VERIFIED"),
		"upper digest":     insert("acme", "m3", full, "x", strings.Repeat("D", 64), "1.9.8", "VERIFIED"),
		"traversal subdir": insert("acme", "m4", full, "../x", digest, "1.9.8", "VERIFIED"),
		"absolute subdir":  insert("acme", "m5", full, "/etc", digest, "1.9.8", "VERIFIED"),
		"dot segment":      insert("acme", "m6", full, "a/./b", digest, "1.9.8", "VERIFIED"),
		"revision space":   insert("acme", "m7", "main branch", "x", digest, "1.9.8", "VERIFIED"),
		"floating version": insert("acme", "m8", full, "x", digest, "~> 1.9", "VERIFIED"),
		"source id":        insert("Acme Repo", "m9", full, "x", digest, "1.9.8", "VERIFIED"),
	} {
		t.Run(name, func(t *testing.T) { expect(db.Console, sql, "23514") })
	}
	if got, err := st.GetModuleBundle(ctx, b.Ref); err != nil || !got.SameVerifiedContent(b) {
		t.Fatalf("stored bundle changed: %v", err)
	}
}
