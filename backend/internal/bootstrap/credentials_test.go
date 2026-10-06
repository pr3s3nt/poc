package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/seed"
)

func TestConnectionCredentialStoreSelection(t *testing.T) {
	ctx := context.Background()
	build := func(opts Options) (*App, error) {
		opts.Seed = seed.Defaults()
		return Build(ctx, opts)
	}
	app, err := build(Options{Adapters: AdapterFake})
	if err != nil || app.ConnectionCredentials != nil {
		t.Fatalf("default must have no credential store: %v %v", err, app.ConnectionCredentials)
	}
	app, err = build(Options{Adapters: AdapterFake, ConnectionCredentialStore: "memory"})
	if err != nil || app.ConnectionCredentials == nil || app.ConnectionCredentials.Durable() {
		t.Fatalf("explicit memory store: %v", err)
	}
	statePath := filepath.Join(t.TempDir(), "state.json")
	for name, opts := range map[string]Options{
		"memory with kubernetes adapters": {Adapters: AdapterKubernetes, ConnectionCredentialStore: "memory"},
		"memory with persistent state":    {Adapters: AdapterFake, StatePath: statePath, ConnectionCredentialStore: "memory"},
		"vault without token file":        {Adapters: AdapterFake, ConnectionCredentialStore: "vault", ConnectionVaultAddress: "http://127.0.0.1:8200"},
		"vault with missing token file":   {Adapters: AdapterFake, ConnectionCredentialStore: "vault", ConnectionVaultAddress: "http://127.0.0.1:8200", ConnectionVaultTokenFile: filepath.Join(t.TempDir(), "absent")},
		"unknown store":                   {Adapters: AdapterFake, ConnectionCredentialStore: "disk"},
	} {
		if _, err := build(opts); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("never-printed-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app, err = build(Options{Adapters: AdapterFake, ConnectionCredentialStore: "vault", ConnectionVaultAddress: "http://127.0.0.1:1", ConnectionVaultTokenFile: tokenFile})
	if err != nil || app.ConnectionCredentials == nil || !app.ConnectionCredentials.Durable() {
		t.Fatalf("vault store: %v", err)
	}
	if _, err := build(Options{Adapters: AdapterFake, ConnectionCredentialStore: "vault", ConnectionVaultAddress: "http://127.0.0.1:8200", ConnectionVaultTokenFile: filepath.Join(t.TempDir(), "absent")}); err == nil || strings.Contains(err.Error(), "absent") {
		t.Fatalf("token file error must not echo the path: %v", err)
	}
}
