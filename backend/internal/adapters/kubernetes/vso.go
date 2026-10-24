package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	configport "orchestrator/internal/ports/configuration"
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

// RemoveSynchronized deletes the VSO objects of one Application Environment:
// a VaultStaticSecret is the run's only when its Vault path lies below the
// Application's own value prefix; its destination Secret, VaultAuth,
// ServiceAccount, and a VaultConnection no other VaultAuth still uses are
// removed by the exact names those objects reference. A cluster without the
// VSO CRDs has nothing to remove.
func (s *VSOSynchronizer) RemoveSynchronized(ctx context.Context, target execution.Target, applicationKey, environmentKey string) ([]string, error) {
	return s.removeSynchronized(ctx, target, applicationKey, environmentKey, "")
}

// RemoveWorkloadSynchronized deletes the VSO objects of ONE removed workload
// (UC-07 VSO-01): the VaultStaticSecrets below the Application Environment's
// own value prefix whose VaultAuth binds this workload's delivery
// ServiceAccount, their destination Secrets and VaultAuths, the
// ServiceAccount once no remaining VaultAuth uses it, and a VaultConnection no
// remaining VaultAuth references. Other workloads' objects are untouched.
func (s *VSOSynchronizer) RemoveWorkloadSynchronized(ctx context.Context, target execution.Target, applicationKey, environmentKey, workloadID string) ([]string, error) {
	if workloadID == "" {
		return nil, fmt.Errorf("vso: workload identity required")
	}
	return s.removeSynchronized(ctx, target, applicationKey, environmentKey, configport.WorkloadServiceAccount(applicationKey, environmentKey, workloadID))
}

// removeSynchronized removes the Application Environment's VSO objects;
// a non-empty serviceAccount narrows it to the VaultAuths bound to it.
func (s *VSOSynchronizer) removeSynchronized(ctx context.Context, target execution.Target, applicationKey, environmentKey, serviceAccount string) ([]string, error) {
	if !target.Explicit() || target.Namespace == "" {
		return nil, fmt.Errorf("vso: explicit Kubernetes target required")
	}
	cli, cleanup, err := OpenCLI(ctx, s.KubectlPath, s.Credentials, target)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	ns := target.Namespace
	marker := "/apps/" + applicationKey + "/envs/" + environmentKey + "/values/"
	list := func(kind string) ([]map[string]any, error) {
		out, err := cli.Run(ctx, nil, "get", kind, "-n", ns, "-o", "json")
		if err != nil {
			if strings.Contains(err.Error(), "doesn't have a resource type") || strings.Contains(fmt.Sprint(err), "NotFound") {
				return nil, nil
			}
			return nil, err
		}
		var doc struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(out, &doc); err != nil {
			return nil, fmt.Errorf("vso: decode %s", kind)
		}
		return doc.Items, nil
	}
	text := func(obj map[string]any, path ...string) string {
		var cur any = obj
		for _, p := range path {
			m, ok := cur.(map[string]any)
			if !ok {
				return ""
			}
			cur = m[p]
		}
		v, _ := cur.(string)
		return v
	}
	statics, err := list("vaultstaticsecrets.secrets.hashicorp.com")
	if err != nil {
		return nil, err
	}
	auths, err := list("vaultauths.secrets.hashicorp.com")
	if err != nil {
		return nil, err
	}
	authByName := map[string]map[string]any{}
	for _, a := range auths {
		authByName[text(a, "metadata", "name")] = a
	}
	var removed []string
	drop := func(kind, name string) error {
		if name == "" {
			return nil
		}
		if err := cli.Delete(ctx, ns, kind, name); err != nil {
			return err
		}
		removed = append(removed, kind+"/"+name)
		return nil
	}
	ownedAuth := map[string]bool{}
	serviceAccounts := map[string]bool{}
	for _, v := range statics {
		if !strings.Contains(text(v, "spec", "path"), marker) {
			continue
		}
		if serviceAccount != "" && text(authByName[text(v, "spec", "vaultAuthRef")], "spec", "kubernetes", "serviceAccount") != serviceAccount {
			continue
		}
		name := text(v, "metadata", "name")
		authName := text(v, "spec", "vaultAuthRef")
		if err := drop("vaultstaticsecrets.secrets.hashicorp.com", name); err != nil {
			return removed, err
		}
		if dest := text(v, "spec", "destination", "name"); dest != "" {
			if err := drop("secret", dest); err != nil {
				return removed, err
			}
		}
		ownedAuth[authName] = true
	}
	usedConnections := map[string]bool{}
	usedAccounts := map[string]bool{}
	for name, a := range authByName {
		if !ownedAuth[name] {
			usedConnections[text(a, "spec", "vaultConnectionRef")] = true
			usedAccounts[text(a, "spec", "kubernetes", "serviceAccount")] = true
		}
	}
	for name := range ownedAuth {
		a, ok := authByName[name]
		if !ok {
			continue
		}
		if sa := text(a, "spec", "kubernetes", "serviceAccount"); sa != "" {
			serviceAccounts[sa] = true
		}
		if err := drop("vaultauths.secrets.hashicorp.com", name); err != nil {
			return removed, err
		}
		if conn := text(a, "spec", "vaultConnectionRef"); conn != "" && !usedConnections[conn] {
			usedConnections[conn] = true // drop once
			if obj, found, err := cli.Get(ctx, ns, "vaultconnections.secrets.hashicorp.com", conn); err == nil && found {
				if ca := text(obj, "spec", "caCertSecretRef"); ca != "" {
					if err := drop("secret", ca); err != nil {
						return removed, err
					}
				}
			}
			if err := drop("vaultconnections.secrets.hashicorp.com", conn); err != nil {
				return removed, err
			}
		}
	}
	for sa := range serviceAccounts {
		if usedAccounts[sa] {
			continue
		}
		if err := drop("serviceaccount", sa); err != nil {
			return removed, err
		}
	}
	sort.Strings(removed)
	return removed, nil
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

var (
	_ execution.ConfigSecretSynchronizer = (*VSOSynchronizer)(nil)
	_ execution.WorkloadSecretRemover    = (*VSOSynchronizer)(nil)
)
