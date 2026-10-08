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

// WorkloadScaler quiesces and restores the writers of one Environment
// generation (ADR-012 target transitions).
type WorkloadScaler interface {
	// Replicas reports the desired replica count of a workload and whether it exists.
	Replicas(ctx context.Context, target Target, workloadID string) (int, bool, error)
	// Scale sets the desired replicas and, when scaling to zero, waits until no
	// Pod of the workload remains.
	Scale(ctx context.Context, target Target, workloadID string, replicas int) error
}

// PostgresResource identifies one Kubernetes-managed PostgreSQL resource. It
// carries no credential: the adapter reaches the server through the Pod's
// local socket and declares unsupported anything that requires a password.
type PostgresResource struct {
	Target    Target
	Namespace string
	Name      string // StatefulSet and Service name; the Pod is "<name>-0"
	Database  string
	Username  string
}

// PostgresInventory is the schema/table/row-count fingerprint of a database.
type PostgresInventory struct {
	ServerVersionNum int
	// Tables maps "schema.table" to its row count.
	Tables map[string]int64
}

// PostgresArchive is a private on-Pod backup handle; it holds no bytes.
type PostgresArchive struct {
	Pod    string
	Dir    string
	SHA256 string
	Bytes  int64
}

// PostgresTransfer backs up, streams and restores PostgreSQL 16 (ADR-012).
type PostgresTransfer interface {
	// Inspect proves the server is reachable and reports version and row counts.
	Inspect(ctx context.Context, res PostgresResource) (PostgresInventory, error)
	// Backup writes a custom-format archive to the private (0700/0600) path
	// named by want.Dir on the source Pod and returns its handle. The caller
	// chooses and records the path first, so a crash leaves a known handle. On
	// failure the adapter removes whatever it created under that path.
	Backup(ctx context.Context, res PostgresResource, want PostgresArchive) (PostgresArchive, error)
	// Restore streams the archive into the destination database. The bytes
	// pass through the orchestrator process only as a pipe and are verified
	// against the archive hash; nothing is stored in the orchestrator.
	Restore(ctx context.Context, src PostgresResource, archive PostgresArchive, dst PostgresResource) error
	// RemoveArchive deletes the private archive and its directory.
	RemoveArchive(ctx context.Context, src PostgresResource, archive PostgresArchive) error
}

// RouteInspector reports whether the Environment-owned public Ingress exists
// at a target, so cutover can prove which side serves the host.
type RouteInspector interface {
	HasPublicRoute(ctx context.Context, target Target, applicationID, environmentID string) (bool, error)
}

// NamespaceCleaner deletes one retained generation namespace after verifying
// that it carries the orchestrator ownership labels of the Environment.
type NamespaceCleaner interface {
	DeleteOwnedNamespace(ctx context.Context, target Target, namespace, applicationID, environmentID string) error
}

// TargetProbe proves a destination is reachable with its Connection credential
// and may create the objects a deployment needs, before anything is stopped.
type TargetProbe interface {
	Probe(ctx context.Context, target Target) error
	// ClusterIdentity returns a stable physical identity of the cluster behind
	// a target (not a Connection key or kube context string), so two logical
	// Connections can be proven to reach the same cluster and ingress controller.
	ClusterIdentity(ctx context.Context, target Target) (string, error)
}
