package vault

import (
	"context"
	"strings"
	"testing"
)

func TestOutputSecretsAreDurableCreateOnlyAndScoped(t *testing.T) {
	fake := newFakeVault(t, "scoped-token-0001")
	store, err := NewOutputSecrets(fake.server.URL, "scoped-token-0001", "kv", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ref, err := store.Put(ctx, "postgres.default#shared.db/password", "s3cret-value-1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ref, "s3cret") || !strings.HasPrefix(ref, "secret://vault/kv/orchestrator/outputs/") {
		t.Fatalf("ref %q", ref)
	}
	other, _ := store.Put(ctx, "postgres.default#shared.db/password", "s3cret-value-1")
	if other == ref {
		t.Fatal("two Puts must create two immutable objects")
	}
	// A new store instance (process restart) resolves the same reference.
	reopened, _ := NewOutputSecrets(fake.server.URL, "scoped-token-0001", "kv", nil)
	if got, ok := reopened.Get(ctx, ref); !ok || got != "s3cret-value-1" {
		t.Fatalf("restart lost the secret: %q %v", got, ok)
	}
	for _, bad := range []string{
		"", "secret://other", "secret://vault/kv/orchestrator/connections/org/conn/credentials/x",
		"secret://vault/kv/orchestrator/outputs/../connections/a/b", "secret://vault/other/orchestrator/outputs/0123456789abcdef/abcdefgh",
		"secret://vault/kv/orchestrator/outputs/0123456789abcdef/../../x", ref + "/extra",
	} {
		if _, ok := reopened.Get(ctx, bad); ok {
			t.Errorf("reference %q resolved", bad)
		}
	}
	if _, ok := reopened.Get(ctx, "secret://vault/kv/orchestrator/outputs/0123456789abcdef/abcdefgh"); ok {
		t.Fatal("a missing object resolved")
	}
	wrong, _ := NewOutputSecrets(fake.server.URL, "another-token-0002", "kv", nil)
	if _, ok := wrong.Get(ctx, ref); ok {
		t.Fatal("a token without access resolved the secret")
	}
	if _, err := wrong.Put(ctx, "x", "y"); err == nil {
		t.Fatal("a token without access wrote a secret")
	}
	if _, err := store.Put(ctx, "name", ""); err == nil {
		t.Fatal("an empty value was stored")
	}
	if !store.Durable() {
		t.Fatal("durable")
	}
}

func TestOutputSecretsFailClosedOnAFailedWrite(t *testing.T) {
	fake := newFakeVault(t, "scoped-token-0001")
	fake.failWrite = true
	store, _ := NewOutputSecrets(fake.server.URL, "scoped-token-0001", "kv", nil)
	if ref, err := store.Put(context.Background(), "n", "v"); err == nil || ref != "" {
		t.Fatalf("a failed write returned a reference: %q %v", ref, err)
	}
	for _, bad := range []struct{ addr, token, mount string }{{"ftp://x", "t", "kv"}, {"http://u:p@x", "t", "kv"}, {"http://x", " ", "kv"}, {"http://x", "t", "../kv"}} {
		if _, err := NewOutputSecrets(bad.addr, bad.token, bad.mount, nil); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}
