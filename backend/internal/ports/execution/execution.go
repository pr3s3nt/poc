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
type Target struct {
	Kind        string            `json:"kind"`
	Context     string            `json:"context,omitempty"`
	Kubeconfig  string            `json:"kubeconfig,omitempty"`
	ClusterName string            `json:"clusterName,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
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
	WorkloadID      string
	Module          environment.Module
	Namespace       string
	Labels          map[string]string
	PlainEnv        map[string]map[string]string
	SecretEnv       map[string]map[string]string
	DeploymentID    string
	ImagePullSecret string
	Vault           *VaultInjection
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
