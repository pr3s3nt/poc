// Package execution defines the outbound ports used to provision resources and
// deploy workloads. Adapters live under internal/adapters.
package execution

import (
	"context"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/domain/tfresource"
)

// Target is the resolved deployment target of a workload or Kubernetes resource.
//
// Organization and Connection are the opaque execution identity of a
// credential-backed (KUBECONFIG) Connection. Adapters resolve its credential
// through KubeconfigSource at each operation; no credential bytes or temporary
// paths are ever stored in a Target. Otherwise only an explicit Kubeconfig file
// path (for example the one a Terraform EKS run generated) can reach a cluster;
// a Context alone never selects a host default.
type Target struct {
	Kind         string `json:"kind"`
	Context      string `json:"context,omitempty"`
	Kubeconfig   string `json:"kubeconfig,omitempty"`
	ClusterName  string `json:"clusterName,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
	Organization string `json:"organization,omitempty"`
	Connection   string `json:"connection,omitempty"`
	// Provider names a Terraform-created target whose credential is derived
	// from a scoped Connection instead of a kubeconfig: "eks" means Connection
	// is the AWS Connection and ClusterName/Region identify the cluster. The
	// target stays opaque: no endpoint, token, kubeconfig or temp path.
	Provider string            `json:"provider,omitempty"`
	Region   string            `json:"region,omitempty"`
	Extra    map[string]string `json:"extra,omitempty"`
}

// CredentialBacked reports whether the target must be reached with a
// Connection credential instead of the backend host's kube configuration.
func (t Target) CredentialBacked() bool { return t.Connection != "" }

// Explicit reports whether the target names a credential: a scoped Connection
// or an explicit kubeconfig path. A context name alone is not explicit.
func (t Target) Explicit() bool {
	return t.CredentialBacked() || t.Kubeconfig != ""
}

// KubeconfigSource resolves short-lived kubeconfig data for an explicit
// credential-backed target. Implementations may support a scoped KUBECONFIG
// Connection or a completed opaque EKS target; they fail closed for mismatched
// scope and never fall back to host credentials.
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
	// Artifact is the registry-verified Terraform module, set only for an
	// executor that implements VerifiedModuleExecutor.
	Artifact *tfmodule.VerifiedArtifact
	// Source is the pinned module of a Terraform Definition. An executor that
	// implements SelfVerifyingExecutor verifies and copies it itself, right
	// before its first effect, and receives no Artifact.
	Source *tfmodule.SourceRef
	// Identity is the canonical Terraform instance identity tuple of the node.
	Identity tfresource.Identity
	// OutputMapping maps module output -> Resource Type output.
	OutputMapping map[string]string
	// Type is the Resource Type contract the outputs must meet.
	Type resource.Type
	// ProviderPins are the plan-pinned provider bindings of this node;
	// ProviderTargets the completed opaque target of every cluster resource a
	// binding names, keyed by that node's descriptor; ProviderInstanceIDs the
	// Terraform instance IDs of the providers this node depends on.
	ProviderPins        []tfmodule.ProviderPin
	ProviderTargets     map[string]Target
	ProviderInstanceIDs []string
	// Inputs are the resolved driver variables merged with the resource params.
	Inputs map[string]any
	// Generation is the target generation of the Environment run. Executors
	// that keep local state (Terraform workspaces) isolate generations >= 1.
	Generation int64
	PriorState map[string]any
	Connection application.Connection
	Target     Target
}

// ProvisionResult carries the outputs and executor state of one resource node.
type ProvisionResult struct {
	Outputs map[string]any
	State   map[string]any
	Target  *Target
	// Sensitive marks module outputs that are sensitive at runtime; a secret
	// classification is never downgraded by mapping.
	Sensitive map[string]bool
	// InstanceID is the durable Terraform instance of the node, if any.
	InstanceID string
}

// ResourceExecutor provisions or reconciles one resource node.
type ResourceExecutor interface {
	Provision(ctx context.Context, req ProvisionRequest) (ProvisionResult, error)
}

// VerifiedModuleExecutor is implemented by executors that run Terraform
// module code. Provisioning re-verifies the pinned bundle through the module
// registry immediately before every call and passes the verified artifact;
// verification failure stops the node before any provider call. Executors
// that run no module code (fakes) do not implement it.
type VerifiedModuleExecutor interface {
	RequiresVerifiedModule() bool
}

// SelfVerifyingExecutor is implemented by an executor that verifies the
// pinned module and creates its own private invocation copies right before its
// first effect (Terraform v2). Provisioning passes it the pin, not an artifact.
type SelfVerifyingExecutor interface {
	VerifiesModules() bool
}

// ModuleVerifyingRegistry is optionally implemented by an ExecutorRegistry
// that can tell, without resolving an executor, whether the executor of a
// driver verifies modules itself (SelfVerifyingExecutor).
type ModuleVerifyingRegistry interface {
	VerifiesModules(driver resource.DriverType) bool
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
	Selection        resource.RenderingSelection
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

// ConfigSecretRemover removes the synchronization objects a ConfigSecretSynchronizer
// created for ONE Application Environment (proven by the exact Vault value
// path of each object, never by name pattern) and returns the kinds and names
// it removed. It is used only by the acceptance safety net.
type ConfigSecretRemover interface {
	RemoveSynchronized(ctx context.Context, target Target, applicationKey, environmentKey string) ([]string, error)
}

type ConfigBundle struct {
	StoreKey, Address, Mount, AuthMount, Path, Role, ServiceAccount, SecretName string
	CAPEM                                                                       string
	Keys                                                                        map[string]map[string]string
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

// RenderPreflight validates pinned renderer availability before infrastructure side effects.
type RenderPreflight interface {
	ValidateSelection(context.Context, resource.RenderingSelection) error
}
