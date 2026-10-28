package kubernetes

import (
	"context"
	"fmt"

	"orchestrator/internal/ports/execution"
)

// EnvBinding is one platform-approved container environment entry: a plain
// value or a Secret name/key reference. Secret bytes are never part of it.
type EnvBinding struct {
	Name      string
	Value     string
	Secret    bool
	SecretRef string
	SecretKey string
}

// Delivery is the shared private secret bridge of one workload render
// (ADR-021 RB-C04 step 1): identity labels and selector, the approved
// environment bindings per container, Vault Agent metadata and the
// platform-owned auxiliary objects (the workload env Secret and the Agent
// ServiceAccount). It never renders a Deployment or Service.
type FileBinding struct {
	Container  string `json:"container"`
	SecretName string `json:"secretName"`
	SecretKey  string `json:"secretKey"`
	Path       string `json:"path"`
	Mode       int    `json:"mode"`
}

type Delivery struct {
	Files []FileBinding

	Labels          map[string]string
	Selector        map[string]string
	Env             map[string][]EnvBinding
	PodAnnotations  map[string]string
	ServiceAccount  string
	ImagePullSecret string
	Auxiliaries     []execution.Manifest
}

// PrepareDelivery builds the secret bridge with the same rules as the native
// renderer: plain values, then SecretEnv through the workload Secret, then
// VSO references, then the Agent configuration file, each sorted by name.
func PrepareDelivery(_ context.Context, req execution.RenderRequest) (Delivery, error) {
	if req.Namespace == "" {
		return Delivery{}, fmt.Errorf("kubernetes: render %q without a namespace", req.WorkloadID)
	}
	if req.Vault != nil && req.ConfigSecretName != "" {
		return Delivery{}, fmt.Errorf("kubernetes: Vault Agent and VSO delivery are mutually exclusive")
	}
	d := Delivery{
		Labels:          map[string]string{"app.kubernetes.io/name": req.WorkloadID, "app.kubernetes.io/managed-by": "orchestrator"},
		Selector:        map[string]string{"app.kubernetes.io/name": req.WorkloadID},
		Env:             map[string][]EnvBinding{},
		ImagePullSecret: req.ImagePullSecret,
	}
	for k, v := range req.Labels {
		d.Labels[k] = v
	}
	labels := map[string]any{}
	for k, v := range d.Labels {
		labels[k] = v
	}
	if req.Vault != nil {
		if req.Vault.Role == "" || req.Vault.Address == "" || req.Vault.ServiceAccount == "" {
			return Delivery{}, fmt.Errorf("kubernetes: incomplete Vault injection")
		}
		template, firstPath, err := vaultTemplate(req.Vault.Bindings)
		if err != nil {
			return Delivery{}, err
		}
		d.PodAnnotations = map[string]string{
			"vault.hashicorp.com/agent-inject":                  "true",
			"vault.hashicorp.com/agent-pre-populate-only":       "true",
			"vault.hashicorp.com/role":                          req.Vault.Role,
			"vault.hashicorp.com/service":                       req.Vault.Address,
			"vault.hashicorp.com/agent-inject-secret-app-env":   firstPath,
			"vault.hashicorp.com/agent-inject-template-app-env": template,
			"vault.hashicorp.com/agent-inject-perms-app-env":    "0400",
		}
		d.ServiceAccount = req.Vault.ServiceAccount
	}
	secretName := req.WorkloadID + "-env"
	secretData := map[string]any{}
	for _, containerName := range sortedKeys(req.SecretEnv) {
		for _, key := range sortedStringKeys(req.SecretEnv[containerName]) {
			secretData[secretKey(containerName, key)] = req.SecretEnv[containerName][key]
		}
	}
	if len(secretData) > 0 {
		d.Auxiliaries = append(d.Auxiliaries, execution.Manifest{
			APIVersion: "v1", Kind: "Secret", Name: secretName, Namespace: req.Namespace, Secret: true,
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata":   map[string]any{"name": secretName, "namespace": req.Namespace, "labels": labels},
				"type":       "Opaque",
				"stringData": secretData,
			},
		})
	}
	fileKeys := map[string]map[string]bool{}
	if e := req.Module.Spec.Execution; e != nil {
		for i, file := range e.SecretFiles {
			if req.Vault != nil {
				return Delivery{}, fmt.Errorf("kubernetes: secret files require revisioned Secret delivery")
			}
			key := fmt.Sprintf("orchestrator-file-%d", i)
			secretKey := req.ConfigSecretKeys[file.Container][key]
			if req.ConfigSecretName == "" || secretKey == "" {
				return Delivery{}, fmt.Errorf("kubernetes: secret file reference is unavailable")
			}
			if fileKeys[file.Container] == nil {
				fileKeys[file.Container] = map[string]bool{}
			}
			fileKeys[file.Container][key] = true
			d.Files = append(d.Files, FileBinding{Container: file.Container, SecretName: req.ConfigSecretName, SecretKey: secretKey, Path: file.Path, Mode: file.Mode})
		}
	}
	for name := range req.Module.Spec.Containers {
		env := []EnvBinding{}
		for _, key := range sortedStringKeys(req.PlainEnv[name]) {
			env = append(env, EnvBinding{Name: key, Value: req.PlainEnv[name][key]})
		}
		for _, key := range sortedStringKeys(req.SecretEnv[name]) {
			env = append(env, EnvBinding{Name: key, Secret: true, SecretRef: secretName, SecretKey: secretKey(name, key)})
		}
		for _, key := range sortedStringKeys(req.ConfigSecretKeys[name]) {
			if fileKeys[name][key] {
				continue
			}
			if req.ConfigSecretName == "" || req.ConfigSecretKeys[name][key] == "" {
				return Delivery{}, fmt.Errorf("kubernetes: incomplete VSO Secret reference")
			}
			env = append(env, EnvBinding{Name: key, Secret: true, SecretRef: req.ConfigSecretName, SecretKey: req.ConfigSecretKeys[name][key]})
		}
		if req.Vault != nil {
			env = append(env, EnvBinding{Name: "ORCHESTRATOR_CONFIG_FILE", Value: "/vault/secrets/app-env"})
		}
		d.Env[name] = env
	}
	if req.Vault != nil {
		d.Auxiliaries = append(d.Auxiliaries, execution.Manifest{APIVersion: "v1", Kind: "ServiceAccount", Name: req.Vault.ServiceAccount, Namespace: req.Namespace, Object: map[string]any{
			"apiVersion": "v1", "kind": "ServiceAccount", "metadata": map[string]any{"name": req.Vault.ServiceAccount, "namespace": req.Namespace, "labels": labels},
		}})
	}
	return d, nil
}

// envEntries is the Kubernetes form of approved bindings.
func envEntries(bindings []EnvBinding) []any {
	env := make([]any, 0, len(bindings))
	for _, b := range bindings {
		if b.Secret {
			env = append(env, map[string]any{"name": b.Name, "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": b.SecretRef, "key": b.SecretKey}}})
			continue
		}
		env = append(env, map[string]any{"name": b.Name, "value": b.Value})
	}
	return env
}
