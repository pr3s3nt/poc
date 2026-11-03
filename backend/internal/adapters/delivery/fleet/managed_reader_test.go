package fleet

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	k8s "orchestrator/internal/adapters/kubernetes"
	port "orchestrator/internal/ports/delivery"
	"os"
	"path/filepath"
	"testing"
)

func TestManagedReaderRefusesForeignSecretAndReusesIdenticalOwnedSecret(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "kubectl")
	snapshot := filepath.Join(dir, "secret.json")
	mutation := filepath.Join(dir, "mutation")
	kubeconfig := filepath.Join(dir, "kubeconfig")
	t.Setenv("ISSUE31_READER_SNAPSHOT", snapshot)
	t.Setenv("ISSUE31_READER_MUTATION", mutation)
	body := "#!/bin/sh\ncase \" $* \" in\n*' get secret '*) cat \"$ISSUE31_READER_SNAPSHOT\";;\n*) cat > \"$ISSUE31_READER_MUTATION\";;\nesac\n"
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kubeconfig, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	intent := packageIntent()
	cfg := Config{ManagedReadSecret: true, ReadSecret: managedReaderName(intent.Binding), Workspace: "fleet-local"}
	manager := &Manager{ReaderCredentials: func(context.Context, port.BindingVersion) ([]byte, error) {
		return []byte(`{"username":"writer","password":"private-test-value"}`), nil
	}, Guard: func(context.Context, port.Intent) error { return nil }}
	secret := map[string]any{"type": "kubernetes.io/basic-auth", "metadata": map[string]any{"labels": map[string]any{"orchestrator.io/delivery-binding": "foreign"}}}
	raw, _ := json.Marshal(secret)
	if err := os.WriteFile(snapshot, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cli := k8s.NewCLI(script, "", kubeconfig)
	if err := manager.ensureReader(context.Background(), intent, cli, cfg); !errors.Is(err, ErrOwnership) {
		t.Fatal("foreign Secret was not refused", err)
	}
	if _, err := os.Stat(mutation); !os.IsNotExist(err) {
		t.Fatal("foreign Secret was mutated")
	}
	secret["metadata"] = map[string]any{"labels": map[string]any{"orchestrator.io/organization-id": "org", "orchestrator.io/delivery-binding": "binding", "orchestrator.io/binding-version": "1"}}
	secret["data"] = map[string]any{"username": base64.StdEncoding.EncodeToString([]byte("writer")), "password": base64.StdEncoding.EncodeToString([]byte("private-test-value"))}
	raw, _ = json.Marshal(secret)
	if err := os.WriteFile(snapshot, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.ensureReader(context.Background(), intent, cli, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mutation); !os.IsNotExist(err) {
		t.Fatal("identical owned Secret was rewritten")
	}
}

func TestManagedReaderIsDerivedPerImmutableBindingVersion(t *testing.T) {
	p := &Provider{}
	b := port.BindingVersion{ID: "binding", Version: 1, OrganizationID: "org", Config: map[string]string{}}
	first, err := p.InitializeConfiguration(b)
	if err != nil || first["readSecretMode"] != "MANAGED" || first["readSecret"] == "" {
		t.Fatal("missing private reader identity", err)
	}
	if len(b.Config) != 0 {
		t.Fatal("caller config mutated")
	}
	second := b
	second.Version = 2
	second.Config = first
	next, err := p.InitializeConfiguration(second)
	if err != nil || next["readSecret"] == first["readSecret"] {
		t.Fatal("new binding version reused reader identity", err)
	}
	other := b
	other.OrganizationID = "other-org"
	crossOrg, _ := p.InitializeConfiguration(other)
	if crossOrg["readSecret"] == first["readSecret"] {
		t.Fatal("reader identity crossed Organization boundary")
	}
}

func TestExistingReaderIsNeverAdoptedByManagedSetup(t *testing.T) {
	p := &Provider{}
	b := port.BindingVersion{ID: "binding", Version: 1, OrganizationID: "org", Config: map[string]string{"readSecret": "preexisting-reader"}}
	config, err := p.InitializeConfiguration(b)
	if err != nil || config["readSecret"] != "preexisting-reader" || config["readSecretMode"] != "" {
		t.Fatal("preexisting reader was adopted", err)
	}
}
