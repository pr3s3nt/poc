// Package execution defines the outbound ports used to provision resources and
// deploy workloads. Adapters live under internal/adapters.
package execution

import (
	"context"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
)

// Target is the resolved deployment target of a workload or Kubernetes resource.
//
// Organization and Connection are the opaque execution identity of a
// credential-backed (KUBECONFIG) Connection. Adapters resolve its credential
// through KubeconfigSource at each operation; no credential bytes or temporary
// paths are ever stored in a Target. Legacy host-context targets leave both
// empty and use Context/Kubeconfig as before.
type Target struct {
	Kind         string            `json:"kind"`
	Context      string            `json:"context,omitempty"`
	Kubeconfig   string            `json:"kubeconfig,omitempty"`
	ClusterName  string            `json:"clusterName,omitempty"`
	Namespace    string            `json:"namespace,omitempty"`
	Organization string            `json:"organization,omitempty"`
	Connection   string            `json:"connection,omitempty"`
	Extra        map[string]string `json:"extra,omitempty"`
}

// CredentialBacked reports whether the target must be reached with a
// Connection credential instead of the backend host's kube configuration.
func (t Target) CredentialBacked() bool { return t.Connection != "" }

// Explicit reports whether the target names a cluster rather than relying on
// the host's default kube context.
func (t Target) Explicit() bool {
	return t.CredentialBacked() || t.Context != "" || t.Kubeconfig != ""
}

// KubeconfigSource resolves the normalized selected-context kubeconfig of a
// credential-backed target. It fails closed for a missing, foreign, non-READY
// or non-KUBECONFIG Connection and never falls back to host credentials.
type KubeconfigSource interface {
	ResolveKubeconfig(ctx context.Context, target Target) ([]byte, error)
}

// PublicRoute is the complete HTTP route set of one Environment.
type PublicRoute struct {
	ApplicationID, EnvironmentID, Host string
	Paths                              []PublicPath
}

type PublicPath struct{ Path, WorkloadID, PortName string }

type PublicRouteManager interface {
	Reconcile(ctx context.Context, target Target, route PublicRoute) error
}

// ProvisionRequest is one resource node handed to an executor (UC-08 MS-05).
type ProvisionRequest struct {
	DeploymentID    string
	RunID           string
	OrganizationKey string
	ApplicationKey  string
	EnvironmentKey  string
	Descriptor      string
	ResourceType    string
	Class           string
	Scope           resource.Scope
	DefinitionKey   string
	DriverType      resource.DriverType
	// Module names the Terraform module of the matched Definition source.
	Module string
	// Inputs are the resolved driver variables merged with the resource params.
	Inputs     map[string]any
	PriorState map[string]any
	Connection application.Connection
	Target     Target
}

// ProvisionResult carries the outputs and executor state of one resource node.
type ProvisionResult struct {
	Outputs map[string]any
	State   map[string]any
	Target  *Target
}

// ResourceExecutor provisions or reconciles one resource node.
type ResourceExecutor interface {
	Provision(ctx context.Context, req ProvisionRequest) (ProvisionResult, error)
}

// ExecutorRegistry maps a Driver Type to its executor adapter (UC-08 MS-04).
type ExecutorRegistry interface {
	Resolve(driver resource.DriverType) (ResourceExecutor, error)
}

// Manifest is one rendered Kubernetes object.
type Manifest struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Name       string         `json:"name"`
	Namespace  string         `json:"namespace,omitempty"`
	Secret     bool           `json:"-"`
	Object     map[string]any `json:"-"`
}

// RenderRequest describes a workload whose bindings are already resolved.
type RenderRequest struct {
	WorkloadID       string
	Module           environment.Module
	Namespace        string
	Labels           map[string]string
	PlainEnv         map[string]map[string]string
	SecretEnv        map[string]map[string]string
	DeploymentID     string
	ImagePullSecret  string
	Vault            *VaultInjection
	ConfigSecretName string
	ConfigSecretKeys map[string]map[string]string
}

// ConfigSecretSynchronizer reconciles VSO objects and verifies the Secret of
// the exact workload revision before a Deployment is applied.
type ConfigSecretSynchronizer interface {
	Sync(ctx context.Context, target Target, bundle ConfigBundle) error
}

type ConfigBundle struct {
	Address, Mount, Path, Role, ServiceAccount, SecretName string
	Keys                                                   map[string]map[string]string
}

// VaultInjection supplies only opaque immutable value references to the
// Kubernetes renderer. Secret bytes never enter manifests or annotations.
type VaultInjection struct {
	Address        string
	Role           string
	ServiceAccount string
	Bindings       map[string]map[string]string
}

// WorkloadRenderer turns a workload module into Kubernetes manifests (UC-06 MS-11).
type WorkloadRenderer interface {
	Render(ctx context.Context, req RenderRequest) ([]Manifest, error)
}

// WorkloadRef identifies an applied object whose readiness must be observed.
type WorkloadRef struct {
	Kind      string
	Name      string
	Namespace string
}

// WorkloadDeployer applies manifests and observes readiness.
type WorkloadDeployer interface {
	Apply(ctx context.Context, target Target, manifests []Manifest) error
	WaitReady(ctx context.Context, target Target, refs []WorkloadRef) error
	Remove(ctx context.Context, target Target, workloadID string) error
}

// SecretStore keeps secret values outside the orchestrator database.
type SecretStore interface {
	Put(ctx context.Context, name string, value string) (string, error)
	Get(ctx context.Context, ref string) (string, bool)
}
