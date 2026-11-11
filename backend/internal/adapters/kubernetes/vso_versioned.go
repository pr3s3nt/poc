package kubernetes

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"orchestrator/internal/ports/execution"
)

var versionedSecretName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?-config-v[1-9][0-9]*$`)

// renderVersionedBundle renders the fixed-identity declarations of ADR-042:
// the workload's ServiceAccount and VaultAuth keep their names across deploys,
// the VaultConnection is shared per store in the namespace and only the
// VaultStaticSecret (pinned to one KV v2 version) is named after the version.
// Shared objects carry no workload label, so equal declarations from several
// workloads are identical and can be merged into one Environment bundle.
func renderVersionedBundle(target execution.Target, bundle execution.ConfigBundle) ([]execution.Manifest, error) {
	if !versionedSecretName.MatchString(bundle.SecretName) || !strings.HasSuffix(bundle.SecretName, "-config-v"+strconv.Itoa(bundle.Version)) || len(bundle.SecretName) > 253 {
		return nil, fmt.Errorf("vso: invalid destination Secret name")
	}
	if !strings.HasPrefix(bundle.ServiceAccount, "orch-") {
		return nil, fmt.Errorf("vso: invalid delivery ServiceAccount")
	}
	ns := target.Namespace
	auth := bundle.ServiceAccount + "-auth"
	connection := VaultConnectionName(bundle.StoreKey)
	connectionSpec := map[string]any{"address": bundle.Address, "skipTLSVerify": false}
	shared := []map[string]any{}
	if bundle.CAPEM != "" {
		caName := connection + "-ca"
		shared = append(shared, map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": caName, "namespace": ns}, "type": "Opaque", "data": map[string]any{"ca.crt": base64.StdEncoding.EncodeToString([]byte(bundle.CAPEM))}})
		connectionSpec["caCertSecretRef"] = caName
	}
	shared = append(shared, map[string]any{"apiVersion": "secrets.hashicorp.com/v1beta1", "kind": "VaultConnection", "metadata": map[string]any{"name": connection, "namespace": ns}, "spec": connectionSpec})
	owned := []map[string]any{
		{"apiVersion": "v1", "kind": "ServiceAccount", "metadata": map[string]any{"name": bundle.ServiceAccount, "namespace": ns}},
		{"apiVersion": "secrets.hashicorp.com/v1beta1", "kind": "VaultAuth", "metadata": map[string]any{"name": auth, "namespace": ns}, "spec": map[string]any{"vaultConnectionRef": connection, "method": "kubernetes", "mount": bundle.AuthMount, "kubernetes": map[string]any{"role": bundle.Role, "serviceAccount": bundle.ServiceAccount}}},
		{"apiVersion": "secrets.hashicorp.com/v1beta1", "kind": "VaultStaticSecret", "metadata": map[string]any{"name": bundle.SecretName, "namespace": ns}, "spec": map[string]any{"vaultAuthRef": auth, "type": "kv-v2", "mount": bundle.Mount, "path": bundle.Path, "version": bundle.Version, "refreshAfter": "1m", "hmacSecretData": true, "destination": map[string]any{"create": true, "name": bundle.SecretName}}},
	}
	keys := []string{}
	for _, entries := range bundle.Keys {
		for _, key := range entries {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	rawKeys, _ := json.Marshal(keys)
	out := make([]execution.Manifest, 0, len(shared)+len(owned))
	add := func(object map[string]any, workload string) error {
		md := object["metadata"].(map[string]any)
		labels := map[string]any{"app.kubernetes.io/managed-by": "orchestrator", "orchestrator.io/application": target.Extra["application"], "orchestrator.io/environment": target.Extra["environment"]}
		if workload != "" {
			labels["orchestrator.io/workload"] = workload
		}
		md["labels"] = labels
		if object["kind"] == "VaultStaticSecret" {
			md["annotations"] = map[string]any{"orchestrator.io/secret-keys": string(rawKeys)}
		}
		if object["kind"] == "Secret" && !execution.PublicCertificateSecret(object) {
			return fmt.Errorf("vso: invalid public CA certificate")
		}
		out = append(out, execution.Manifest{APIVersion: object["apiVersion"].(string), Kind: object["kind"].(string), Name: md["name"].(string), Namespace: ns, Object: object})
		return nil
	}
	for _, object := range shared {
		if err := add(object, ""); err != nil {
			return nil, err
		}
	}
	for _, object := range owned {
		if err := add(object, target.Extra["workload"]); err != nil {
			return nil, err
		}
	}
	return out, nil
}
