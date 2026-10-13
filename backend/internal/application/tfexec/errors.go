// Package tfexec is the Terraform v2 executor application service (UC-08,
// ADR-014, OC-10/18/19): CREATE and non-destructive UPDATE of registered
// module instances with durable state, write-ahead attempts and stages,
// bounded retry, output and readiness contracts, and the authorized DESTROY
// engine with reverse-dependency ordering and physical absence proof.
//
// The service depends on ports only. Every error message is a fixed safe
// sentence: no CLI output, plan, state, credential or secret reaches an error,
// a stage or an audit field.
package tfexec

import "errors"

// Sentinel errors. Wrapping errors add only safe, fixed context.
var (
	// ErrNeedsAttention means the outcome of an external step is unknown or
	// the state could not be stored: the Environment claim must stay held, no
	// duplicate CREATE may run and an operator must act (D12/D13 are deferred).
	ErrNeedsAttention = errors.New("tfexec: the outcome is unknown; the resource needs operator attention and the Environment stays held")
	// ErrInstanceHeld means an earlier attempt left the instance in an
	// unresolved state; this execution refused to start.
	ErrInstanceHeld = errors.New("tfexec: an earlier attempt of this resource is unresolved; it must be resolved before another execution")
	// ErrReplacementRequired means the pinned class, module or target differs
	// from the existing instance; replacement is a distinct approved flow.
	ErrReplacementRequired = errors.New("tfexec: the module or target differs from the existing resource; replacement needs its own approved operation")
	// ErrDestructivePlan means the plan deletes or replaces existing
	// objects; it was rejected before apply.
	ErrDestructivePlan = errors.New("tfexec: the plan would delete or replace existing objects and was rejected before apply")
	// ErrOutputContract means runtime outputs do not match the module or the
	// Resource Type contract; nothing was published.
	ErrOutputContract = errors.New("tfexec: the runtime outputs do not match the module and Resource Type contracts")
	// ErrNoClaim means the call carries no Environment claim owner.
	ErrNoClaim = errors.New("tfexec: an Environment operation claim is required before any Terraform execution")
	// ErrInput means a required input is missing, mistyped or conflicts with
	// an executor-owned value.
	ErrInput = errors.New("tfexec: the module inputs do not satisfy the pinned module contract")
	// ErrExecution is a confirmed, retained failure of one Terraform step.
	ErrExecution = errors.New("tfexec: the Terraform execution failed")
	// ErrBinding means a provider binding differs from its plan pin or cannot
	// be resolved; nothing ran.
	ErrBinding = errors.New("tfexec: a provider binding differs from the plan or cannot be resolved")
	// ErrCleanupRejected means a cleanup selection failed its ownership,
	// consumer or dependency check; nothing was destroyed.
	ErrCleanupRejected = errors.New("tfexec: the cleanup selection failed its ownership or dependency check")
)

// Safe attempt stage and error codes (schema: ^[A-Z][A-Z0-9_]*$).
const (
	StageIntent     = "INTENT"
	StageInit       = "INIT"
	StagePlanning   = "PLANNING"
	StagePlanned    = "PLANNED"
	StageApplying   = "APPLYING"
	StageApplied    = "APPLIED"
	StageOutputs    = "OUTPUTS"
	StageReadiness  = "READINESS"
	StageReady      = "READY"
	StageNoChange   = "NO_CHANGE"
	StageChecking   = "CHECKING"
	StageDestroying = "DESTROYING"
	StageVerifying  = "VERIFYING"
	StageDestroyed  = "DESTROYED"
	StageRecovery   = "RECOVERY"
)

const (
	codeDestructive   = "DESTRUCTIVE_PLAN"
	codeOutputs       = "OUTPUT_CONTRACT"
	codeNotReady      = "NOT_READY"
	codeReadinessUnav = "READINESS_UNAVAILABLE"
	codeUnknown       = "OUTCOME_UNKNOWN"
	codeStateNotAck   = "STATE_NOT_ACKNOWLEDGED"
	codeBackend       = "STATE_BACKEND"
	codeTransient     = "TRANSIENT"
	codeAuth          = "AUTH"
	codeInput         = "INPUT"
	codeTimeout       = "TIMEOUT"
	codeCancelled     = "CANCELLED"
	codeFailed        = "FAILED"
	codeRemains       = "PHYSICAL_RESOURCE_REMAINS"
	codeSecret        = "SECRET_UNAVAILABLE"
)
