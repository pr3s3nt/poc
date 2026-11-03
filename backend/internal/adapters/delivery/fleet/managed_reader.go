package fleet

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	k8s "orchestrator/internal/adapters/kubernetes"
	port "orchestrator/internal/ports/delivery"
)

func managedReaderName(b port.BindingVersion) string {
	sum := sha256.Sum256([]byte(b.OrganizationID + "\x00" + b.ID + "\x00" + strconv.FormatInt(b.Version, 10)))
	return "orch-git-reader-" + hex.EncodeToString(sum[:])[:20]
}

func (p *Provider) InitializeConfiguration(b port.BindingVersion) (map[string]string, error) {
	config := map[string]string{}
	for k, v := range b.Config {
		config[k] = v
	}
	if config["readSecret"] == "" || config["readSecretMode"] == "MANAGED" {
		config["readSecretMode"] = "MANAGED"
		config["readSecret"] = managedReaderName(b)
	}
	return config, nil
}

func (m *Manager) ensureReader(ctx context.Context, i port.Intent, cli k8s.CLI, cfg Config) error {
	if !cfg.ManagedReadSecret {
		return nil
	}
	if m.ReaderCredentials == nil || cfg.ReadSecret != managedReaderName(i.Binding) {
		return ErrConfig
	}
	raw, err := m.ReaderCredentials(ctx, i.Binding)
	if err != nil {
		return ErrVerification
	}
	var credential struct{ Username, Password string }
	if json.Unmarshal(raw, &credential) != nil || credential.Username == "" || credential.Password == "" || strings.ContainsAny(credential.Username+credential.Password, "\x00\r\n") {
		return ErrVerification
	}
	labels := map[string]any{"orchestrator.io/organization-id": i.Binding.OrganizationID, "orchestrator.io/delivery-binding": i.Binding.ID, "orchestrator.io/binding-version": strconv.FormatInt(i.Binding.Version, 10)}
	current, exists, err := cli.Get(ctx, cfg.Workspace, "secret", cfg.ReadSecret)
	if err != nil {
		return ErrController
	}
	metadata := map[string]any{"name": cfg.ReadSecret, "namespace": cfg.Workspace, "labels": labels}
	verb := "create"
	if exists {
		previous, _ := current["metadata"].(map[string]any)
		actual, _ := previous["labels"].(map[string]any)
		for key, value := range labels {
			if actual[key] != value {
				return ErrOwnership
			}
		}
		if current["type"] != "kubernetes.io/basic-auth" {
			return ErrOwnership
		}
		data, _ := current["data"].(map[string]any)
		if data["username"] == base64.StdEncoding.EncodeToString([]byte(credential.Username)) && data["password"] == base64.StdEncoding.EncodeToString([]byte(credential.Password)) {
			return nil
		}
		metadata["resourceVersion"] = previous["resourceVersion"]
		verb = "replace"
	}
	body, err := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "Secret", "type": "kubernetes.io/basic-auth", "metadata": metadata, "data": map[string][]byte{"username": []byte(credential.Username), "password": []byte(credential.Password)}})
	if err != nil {
		return ErrConfig
	}
	if m.Guard == nil || m.Guard(ctx, i) != nil {
		return ErrOwnership
	}
	if _, err := cli.Run(ctx, body, verb, "-f", "-"); err != nil {
		return ErrController
	}
	return nil
}
