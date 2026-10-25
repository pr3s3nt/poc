// Package terraformexec defines the outbound ports of the Terraform v2
// executor: the process runner that drives one private Terraform CLI session
// (init, plan, apply, outputs, state pull), the provider binder that turns
// pinned provider bindings into a private subprocess environment, and the
// private recovery store. Adapters live under internal/adapters/terraform.
package terraformexec

import (
	"context"
	"errors"
	"fmt"
	"time"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/execution"
)

// Class is the safe failure classification of one CLI step. The executor
// decides retry and recovery from it; it never reads raw CLI output.
type Class string

// Failure classes.
const (
	// ClassTransient is a throttling, timeout or connection failure of the
	// provider API that may succeed when repeated.
	ClassTransient Class = "TRANSIENT"
	// ClassAuth is a credential or permission failure; never retried.
	ClassAuth Class = "AUTH"
	// ClassInput is a configuration, variable or contract failure.
	ClassInput Class = "INPUT"
	// ClassBackend is a failure of the state backend request or lock.
	ClassBackend Class = "BACKEND"
	// ClassTimeout means the step exceeded its bounded deadline and the
	// process group was stopped.
	ClassTimeout Class = "TIMEOUT"
	// ClassCancelled means the caller cancelled and the group was stopped.
	ClassCancelled Class = "CANCELLED"
	// ClassFailed is any other clean failure.
	ClassFailed Class = "FAILED"
)

// Outcome says whether the result of a step is known.
type Outcome string

const (
	// OutcomeConfirmed means Terraform exited by itself with a status and no
	// process of the group remains. It does NOT prove what the provider did: a
	// clean nonzero exit after a mutating call may still have changed remote
	// objects whose state was never acknowledged. Retrying a mutating step
	// therefore needs proof from acknowledged state, not this value alone.
	OutcomeConfirmed Outcome = "CONFIRMED"
	// OutcomeUnknown means the step was interrupted by a signal, deadline,
	// cancellation or crash, or the group could not be confirmed stopped:
	// provider objects and state may be in any state.
	OutcomeUnknown Outcome = "UNKNOWN"
)

// StepError is the only error a Session returns for a failed CLI step. Its
// message is a fixed safe sentence; raw output stays private.
type StepError struct {
	Step    string
	Class   Class
	Outcome Outcome
	// Stopped reports that the process group was confirmed stopped.
	Stopped bool
}

func (e *StepError) Error() string {
	return "terraform " + e.Step + " failed (" + string(e.Class) + ", outcome " + string(e.Outcome) + ")"
}

// ErrWorkspace means the private workspace could not be prepared or re-verified.
var ErrWorkspace = errors.New("terraformexec: the private Terraform workspace is not valid")

// Backend locates the internal state backend of one invocation. Token is
// delivered to the subprocess only through its private environment.
type Backend struct {
	Address string
	Token   string
}

// SessionRequest opens one private invocation.
type SessionRequest struct {
	// Artifact is the verified private copy of binary, module tree and
	// transitive call tree. The caller owns and closes it.
	Artifact *tfmodule.VerifiedArtifact
	// Backend selects the HTTP state backend; nil runs against LocalState.
	Backend *Backend
	// LocalState is the state a local-only (backend-less) session reads.
	LocalState []byte
	// Variables are written to a private tfvars file (may be secret).
	Variables map[string]any
	// Env is the private provider environment; the runner never inherits the
	// host environment.
	Env []string
	// Deadline bounds every step of the session.
	Deadline time.Time
}

// ResourceChange is one planned change.
type ResourceChange struct {
	Address  string
	Type     string
	Provider string
	Actions  []string
}

// Plan is the parsed review of one saved plan. It holds no values.
type Plan struct {
	Changes []ResourceChange
	// OutputsChanged reports that the plan changes root outputs; applying it
	// records them in state even when no resource changes.
	OutputsChanged bool
	// Drift lists resource addresses whose remote object disappeared,
	// observed by a refresh-only plan.
	Drift []string
}

// Destructive reports whether any change deletes or replaces a resource.
func (p Plan) Destructive() bool {
	for _, c := range p.Changes {
		for _, a := range c.Actions {
			if a == "delete" {
				return true
			}
		}
	}
	return false
}

// OnlyCreates reports whether every change creates, reads or does nothing.
func (p Plan) OnlyCreates() bool {
	for _, c := range p.Changes {
		for _, a := range c.Actions {
			switch a {
			case "create", "no-op", "read":
			default:
				return false
			}
		}
	}
	return true
}

// OnlyDeletes reports whether every change deletes, reads or does nothing.
func (p Plan) OnlyDeletes() bool {
	for _, c := range p.Changes {
		for _, a := range c.Actions {
			switch a {
			case "delete", "no-op", "read":
			default:
				return false
			}
		}
	}
	return true
}

// Mutating reports whether the plan changes anything, including root
// outputs (an output-only module still has state to write).
func (p Plan) Mutating() bool {
	if p.OutputsChanged {
		return true
	}
	for _, c := range p.Changes {
		for _, a := range c.Actions {
			if a != "no-op" && a != "read" {
				return true
			}
		}
	}
	return false
}

// Output is one Terraform output with its runtime type and sensitivity.
type Output struct {
	Value any
	// Type is the Terraform runtime type expression ("string", "number",
	// "bool", ["list","string"], ["object",{...}], ...).
	Type      any
	Sensitive bool
}

// PlanMode selects what a plan computes.
type PlanMode int

// Plan modes.
const (
	// PlanApply plans the configuration against refreshed state.
	PlanApply PlanMode = iota
	// PlanDestroy plans destruction of everything in state.
	PlanDestroy
	// PlanRefreshOnly refreshes state against the provider and reports drift.
	PlanRefreshOnly
)

// Session is one private Terraform invocation. Methods run bounded steps in
// the process group of the session. A step failure is a *StepError.
type Session interface {
	Init(ctx context.Context) error
	// Plan writes and reviews a saved plan (refresh included).
	Plan(ctx context.Context, mode PlanMode) (Plan, error)
	// Apply applies exactly the last saved plan.
	Apply(ctx context.Context) error
	Outputs(ctx context.Context) (map[string]Output, error)
	// PullState reads the backend (or local) state document.
	PullState(ctx context.Context) ([]byte, error)
	// StateResources lists the managed resources of a state document with
	// their provider and physical ID.
	StateResources(ctx context.Context) ([]StateResource, error)
	// RecoveryState returns the state Terraform kept locally when the backend
	// refused to store it ("errored.tfstate"), if any.
	RecoveryState() ([]byte, bool)
	// Close removes the session's private files, except a recovery state
	// still needed by the caller (see KeepRecovery).
	Close() error
}

// StateResource is one managed resource recorded in state.
type StateResource struct {
	Address  string
	Type     string
	Provider string
	ID       string
	// UID is the provider-assigned object UID recorded in state (for example
	// metadata.uid of a Kubernetes object); empty when the type has none.
	UID string
}

// Runner opens sessions.
type Runner interface {
	Open(ctx context.Context, req SessionRequest) (Session, error)
}

// BindRequest asks the binder to prepare provider credentials.
type BindRequest struct {
	OrganizationKey string
	Pins            []tfmodule.ProviderPin
	// Targets holds the completed opaque target of every provider resource
	// node a resource binding names, keyed by that node's descriptor.
	Targets map[string]execution.Target
	// Environment is the Environment Connection and region the plan pinned.
	EnvironmentConnection string
	Region                string
}

// ProviderEnv is the private subprocess environment of resolved bindings and
// the cleanup of the private files that back it.
type ProviderEnv struct {
	Env []string
	// AccountID and Region are the verified identity of an AWS binding.
	AccountID string
	Region    string
	Close     func() error
	// Retain records an opaque handle -> private directory mapping so the
	// credential files can be located after the executor kept them.
	Retain func(handle string) error
}

// ErrProviderBinding means a binding could not be resolved: missing, stale,
// foreign or unavailable credential. It never carries credential material.
var ErrProviderBinding = errors.New("terraformexec: a provider binding cannot be resolved for this Environment")

// ProviderBinder resolves pinned bindings into a private environment. It
// never falls back to host credentials, the host credential chain or the
// default kube context, and never inherits the host environment.
type ProviderBinder interface {
	Bind(ctx context.Context, req BindRequest) (ProviderEnv, error)
}

// RecoveryStore keeps the private encrypted recovery artifact of a state
// that the backend could not store. It returns an opaque handle.
type RecoveryStore interface {
	Save(ctx context.Context, instanceID, attemptID string, state []byte) (handle string, err error)
}

// ReadinessRequest asks whether the provider objects of one applied instance
// are ready for consumers.
type ReadinessRequest struct {
	OrganizationKey string
	ResourceType    string
	Module          string
	Pins            []tfmodule.ProviderPin
	Targets         map[string]execution.Target
	// Resources are the managed objects recorded in the acknowledged state.
	Resources []StateResource
	// Outputs are the validated runtime outputs, secrets included; adapters
	// must never log or return them.
	Outputs map[string]any
	// Env is the private provider environment of the invocation.
	Env      []string
	Deadline time.Time
}

// ErrNotReady means a readiness query ran to completion and the objects are
// not ready. It is a deterministic outcome: nothing is unknown. Any other
// error is an unknown readiness result.
var ErrNotReady = errors.New("terraformexec: the provisioned resource is not ready")

// ReadinessChecker completes UC-08 BR-10 after successful Terraform execution,
// state acknowledgement and output validation. Provider completion covers all
// managed resource types; applicable supplemental Kubernetes/AWS probes must
// also pass within the deadline. ErrNotReady contains safe IDs only.
type ReadinessChecker interface {
	Check(ctx context.Context, req ReadinessRequest) error
}

// AWSConnection is the resolved, scoped identity of an AWS Connection: an
// stored key or temporary role credential with pinned account/role identity. It
// is a secret and never leaves the process except in a private subprocess
// environment.
type AWSConnection struct {
	// Role credentials are ephemeral; Refresh is invocation-local and never serialized.
	RoleARN         string
	RoleID          string
	Expiration      time.Time
	Refresh         func(context.Context) (AWSConnection, error)
	ConnectionKey   string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	AccountID       string
	Region          string
	// AllowedRegions are the regions the Connection was verified for. Region
	// is the Connection default; an invocation may bind any allowed region.
	// Empty means only Region.
	AllowedRegions []string
}

// Credential values must not be exposed by diagnostic formatting.
func (AWSConnection) String() string               { return "AWSConnection{redacted}" }
func (AWSConnection) GoString() string             { return "AWSConnection{redacted}" }
func (c AWSConnection) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(c.String())) }

// AllowsRegion reports whether an invocation may bind region with this
// Connection: the default Region or any verified allowed region.
func (c AWSConnection) AllowsRegion(region string) bool {
	if region == "" {
		return false
	}
	if len(c.AllowedRegions) == 0 {
		return region == c.Region
	}
	for _, allowed := range c.AllowedRegions {
		if allowed == region {
			return true
		}
	}
	return false
}

// AWSIdentitySource resolves the credential of one READY AWS Connection of the
// Organization from the Connection credential store. It fails closed for a
// missing, foreign, non-READY or non-access-key Connection.
type AWSIdentitySource interface {
	ResolveAWS(ctx context.Context, organizationKey, connectionKey string) (AWSConnection, error)
}

// KubernetesIdentitySource resolves the normalized selected-context kubeconfig
// and context name of one READY KUBECONFIG Connection of the Organization.
type KubernetesIdentitySource interface {
	ResolveKubernetes(ctx context.Context, organizationKey, connectionKey string) (kubeconfig []byte, kubeContext string, err error)
}

// ClusterEndpoint is the API endpoint and CA of an EKS cluster.
type ClusterEndpoint struct {
	Endpoint string
	// CAPEM is the cluster certificate authority in PEM form.
	CAPEM  string
	Status string
}

// AWSAPI is the small set of signed AWS calls the binder needs.
type AWSAPI interface {
	// CallerAccount returns the account the access key belongs to
	// (STS GetCallerIdentity).
	CallerAccount(ctx context.Context, c AWSConnection) (string, error)
	// DescribeCluster returns the endpoint and CA of an EKS cluster.
	DescribeCluster(ctx context.Context, c AWSConnection, cluster string) (ClusterEndpoint, error)
	// ClusterToken returns a short-lived EKS authentication token.
	ClusterToken(c AWSConnection, cluster string) (string, error)
}

// StatusQuery names one read-only provider status query.
type StatusQuery string

// Status queries the readiness check can ask the AWS API.
const (
	StatusEC2Vpc                      StatusQuery = "ec2-vpc"
	StatusRDSCluster                  StatusQuery = "rds-cluster"
	StatusRDSInstance                 StatusQuery = "rds-instance"
	StatusElastiCacheReplicationGroup StatusQuery = "elasticache-replication-group"
	StatusElastiCacheCluster          StatusQuery = "elasticache-cluster"
	// StatusEKSNodegroup takes the ID "<cluster>/<nodegroup>".
	StatusEKSNodegroup StatusQuery = "eks-nodegroup"
	// StatusEKSCluster takes the cluster name.
	StatusEKSCluster StatusQuery = "eks-cluster"
)

// AWSStatusAPI reads the lifecycle status of one AWS object by ID with an
// explicit scoped key. A missing object is an error, never a ready status.
type AWSStatusAPI interface {
	Status(ctx context.Context, c AWSConnection, query StatusQuery, id string) (string, error)
}

// NamespaceOwner is one ownerReference of a Kubernetes object, with the UID
// of the owner it names: a recreated or spoofed same-name owner has another UID.
type NamespaceOwner struct {
	Kind string
	Name string
	UID  string
}

// NamespaceObject is the safe summary of one object found in a namespace: its
// identity, UID, owners and the provenance facts the ownership policy needs.
// It never carries values or label/annotation values.
type NamespaceObject struct {
	Kind   string
	Name   string
	UID    string
	Owners []NamespaceOwner
	// DataKeys are the sorted data key names of a ConfigMap or Secret.
	DataKeys []string
	// SecretRefs and ImagePullSecrets count the secrets a ServiceAccount
	// references; AutomountSet reports an explicit automount override.
	SecretRefs       int
	ImagePullSecrets int
	AutomountSet     bool
	// Managers are the distinct field managers recorded in managedFields: the
	// controller that generated an object, or whoever created or changed it.
	Managers []string
	// LabelKeys and AnnotationKeys are the sorted metadata key names.
	LabelKeys      []string
	AnnotationKeys []string
}

// NamespaceInspection is the real content of one namespace: its UID and every
// object of every listable namespaced API resource.
type NamespaceInspection struct {
	UID     string
	Objects []NamespaceObject
	Absent  bool
}

// ErrNamespaceAbsent is returned only for an authoritative Kubernetes
// NotFound response to the exact namespace GET.
var ErrNamespaceAbsent = errors.New("terraformexec: namespace is absent")

// NamespaceInspector lists the real contents of one namespace through the
// scoped provider environment, immediately before a destroy that would delete
// the namespace. An error means the contents are unknown and the destroy must
// be refused.
type NamespaceInspector interface {
	Inspect(ctx context.Context, env []string, namespace string) (NamespaceInspection, error)
}

// NamespaceAbsenceInspector verifies the exact historical state identities
// are NotFound when a namespace recorded in Terraform state is already gone.
type NamespaceAbsenceInspector interface {
	VerifyAbsent(ctx context.Context, env []string, namespace string, resources []StateResource) error
}

// VerificationStore keeps the private encrypted pre-destroy state of one
// destroy cycle, written BEFORE any destroy effect and never overwritten, so a
// later retry can still query the ORIGINAL physical objects after the backend
// state was emptied. An empty terminal state or a zero exit proves nothing
// about the provider; only a query of these objects does.
type VerificationStore interface {
	// SaveVerification stores state under handle for the instance. An
	// existing artifact is kept untouched (idempotent).
	SaveVerification(ctx context.Context, handle, instanceID string, state []byte) error
	// LoadVerification returns the stored state; found=false when none exists.
	// An unreadable or undecryptable artifact is an error, never found=false.
	LoadVerification(ctx context.Context, handle, instanceID string) (state []byte, found bool, err error)
}
