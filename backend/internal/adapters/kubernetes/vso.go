package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/ports/execution"
)

// VSOSynchronizer prepares only non-secret VSO objects and waits for their
// revision-specific Kubernetes Secret. It never reads or logs Secret values.
type VSOSynchronizer struct {
	KubectlPath string
	Timeout     time.Duration
	Credentials execution.KubeconfigSource
}

func (s *VSOSynchronizer) Sync(ctx context.Context, target execution.Target, bundle execution.ConfigBundle) error {
	if !target.Explicit() {
		return fmt.Errorf("vso: explicit Kubernetes target required")
	}
	if target.Namespace == "" || bundle.StoreKey == "" || bundle.AuthMount == "" || bundle.Address == "" || bundle.Mount == "" || bundle.Path == "" || bundle.Role == "" || bundle.ServiceAccount == "" || bundle.SecretName == "" || len(bundle.Keys) == 0 {
		return fmt.Errorf("vso: incomplete workload bundle")
	}
	if !strings.HasPrefix(bundle.SecretName, "orch-") {
		return fmt.Errorf("vso: invalid destination Secret name")
	}
	cli, cleanup, err := OpenCLI(ctx, s.KubectlPath, s.Credentials, target)
	if err != nil {
		return err
	}
	defer cleanup()
	ns := target.Namespace
	auth := bundle.SecretName + "-auth"
	// The VaultConnection name includes the store identity, so a second store
	// in the same namespace never overwrites an object that Secrets synced for
	// older deployments still use.
	connection := VaultConnectionName(bundle.StoreKey)
	connectionSpec := map[string]any{"address": bundle.Address, "skipTLSVerify": false}
	objects := []map[string]any{
		{"apiVersion": "v1", "kind": "ServiceAccount", "metadata": map[string]any{"name": bundle.ServiceAccount, "namespace": ns}},
	}
	if bundle.CAPEM != "" {
		// A CA certificate is public data; it lets VSO verify a private-CA store.
		caName := connection + "-ca"
		objects = append(objects, map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": caName, "namespace": ns}, "type": "Opaque", "stringData": map[string]any{"ca.crt": bundle.CAPEM}})
		connectionSpec["caCertSecretRef"] = caName
	}
	objects = append(objects, []map[string]any{
		{"apiVersion": "secrets.hashicorp.com/v1beta1", "kind": "VaultConnection", "metadata": map[string]any{"name": connection, "namespace": ns}, "spec": connectionSpec},
		{"apiVersion": "secrets.hashicorp.com/v1beta1", "kind": "VaultAuth", "metadata": map[string]any{"name": auth, "namespace": ns}, "spec": map[string]any{"vaultConnectionRef": connection, "method": "kubernetes", "mount": bundle.AuthMount, "kubernetes": map[string]any{"role": bundle.Role, "serviceAccount": bundle.ServiceAccount}}},
		{"apiVersion": "secrets.hashicorp.com/v1beta1", "kind": "VaultStaticSecret", "metadata": map[string]any{"name": bundle.SecretName, "namespace": ns}, "spec": map[string]any{"vaultAuthRef": auth, "type": "kv-v2", "mount": bundle.Mount, "path": bundle.Path, "refreshAfter": "1m", "hmacSecretData": true, "destination": map[string]any{"create": true, "name": bundle.SecretName}}},
	}...)
	if err := cli.Apply(ctx, objects); err != nil {
		return fmt.Errorf("vso: apply synchronization objects: %w", err)
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		object, exists, err := cli.Get(deadline, ns, "secret", bundle.SecretName)
		if err == nil && exists && hasBundleKeys(object, bundle.Keys) {
			return nil
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("vso: destination Secret %s was not synchronized: %w", bundle.SecretName, deadline.Err())
		case <-time.After(time.Second):
		}
	}
}

// VaultConnectionName is the per-store VaultConnection object name.
func VaultConnectionName(storeKey string) string {
	sum := sha256.Sum256([]byte(storeKey))
	return "orch-vault-" + hex.EncodeToString(sum[:4])
}

func hasBundleKeys(secret map[string]any, keys map[string]map[string]string) bool {
	data, ok := secret["data"].(map[string]any)
	if !ok {
		return false
	}
	for _, entries := range keys {
		for _, key := range entries {
			if _, exists := data[key]; !exists {
				return false
			}
		}
	}
	return true
}

var _ execution.ConfigSecretSynchronizer = (*VSOSynchronizer)(nil)
