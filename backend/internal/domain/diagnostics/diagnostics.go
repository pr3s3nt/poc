// Package diagnostics models Infrastructure Account diagnostic reports
// (ADR-022, UC-04 DIAG-01..14). A report is an immutable observation of a
// pinned scope at a time; it never changes Connection readiness.
package diagnostics

import (
	"errors"
	"regexp"
	"time"
)

// Purpose selects what a run is for.
type Purpose string

// Mode selects whether probes may create run-owned objects.
type Mode string

// Status is the run lifecycle.
type Status string

// Verdict is the outcome of one check.
type Verdict string

// CleanupStatus is the state of run-owned probe resources.
type CleanupStatus string

const (
	PurposeGeneral Purpose = "GENERAL"
	PurposeUseCase Purpose = "USE_CASE"

	ModeConditions Mode = "CONDITIONS"
	ModeExecution  Mode = "EXECUTION"

	StatusQueued      Status = "QUEUED"
	StatusRunning     Status = "RUNNING"
	StatusCancelling  Status = "CANCELLING"
	StatusCompleted   Status = "COMPLETED"
	StatusCancelled   Status = "CANCELLED"
	StatusInterrupted Status = "INTERRUPTED"

	VerdictPass    Verdict = "PASS"
	VerdictFail    Verdict = "FAIL"
	VerdictBlocked Verdict = "BLOCKED"
	VerdictUnknown Verdict = "UNKNOWN"
	VerdictNA      Verdict = "NA"

	CleanupNotRequired CleanupStatus = "NOT_REQUIRED"
	CleanupPending     CleanupStatus = "PENDING"
	CleanupVerified    CleanupStatus = "VERIFIED"
	CleanupUnknown     CleanupStatus = "UNKNOWN"
)

// UseCases are the supported selectable capabilities, in display order.
var UseCases = []string{"preview", "deploy", "database", "cache", "secrets", "https", "lifecycle"}

// Active reports whether the status holds the account's single active slot.
func (s Status) Active() bool {
	return s == StatusQueued || s == StatusRunning || s == StatusCancelling
}

// Terminal reports whether the run can no longer change.
func (s Status) Terminal() bool {
	return s == StatusCompleted || s == StatusCancelled || s == StatusInterrupted
}

// Valid reports whether v is one of the five verdicts.
func (v Verdict) Valid() bool {
	switch v {
	case VerdictPass, VerdictFail, VerdictBlocked, VerdictUnknown, VerdictNA:
		return true
	}
	return false
}

// Sentinel errors. None carries a rejected value.
var (
	ErrInvalidScope  = errors.New("diagnostics: the diagnostic scope is invalid")
	ErrInvalidReport = errors.New("diagnostics: the report is invalid")
	// ErrBusy means the account already has an active run.
	ErrBusy = errors.New("diagnostics: the account has an active diagnostic run")
	// ErrLeaseLost means the caller no longer owns the run.
	ErrLeaseLost = errors.New("diagnostics: the run ownership was lost")
)

// FieldError names the request field that failed validation.
type FieldError struct{ Field string }

func (e *FieldError) Error() string { return "diagnostics: invalid field " + e.Field }
func (e *FieldError) Unwrap() error { return ErrInvalidScope }

var (
	regionPattern = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]$`)
	keyPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
)

// ContextRef identifies the optional Application/Environment of a scope.
type ContextRef struct {
	ApplicationKey      string `json:"applicationKey"`
	ApplicationName     string `json:"applicationName,omitempty"`
	ApplicationVersion  int64  `json:"applicationVersion,omitempty"`
	EnvironmentKey      string `json:"environmentKey,omitempty"`
	EnvironmentVersion  int64  `json:"environmentVersion,omitempty"`
	EnvironmentType     string `json:"environmentType,omitempty"`
	CurrentDeploymentID string `json:"currentDeploymentSetId,omitempty"`
}

// Scope is the immutable, validated request snapshot of a run. Account identity
// fields are copied from the saved account, never accepted from the client.
type Scope struct {
	Purpose        Purpose     `json:"purpose"`
	Mode           Mode        `json:"mode"`
	Region         string      `json:"region,omitempty"`
	UseCases       []string    `json:"useCases,omitempty"`
	Context        *ContextRef `json:"context,omitempty"`
	AccountKey     string      `json:"accountKey"`
	AccountName    string      `json:"accountName"`
	AccountType    string      `json:"accountType"`
	AccountVersion int64       `json:"accountVersion"`
	// Identity is a safe account target description: cluster/endpoint for
	// Internal accounts, the AWS account ID for AWS accounts.
	Identity map[string]string `json:"identity,omitempty"`
}

// Request is the client-supplied part of a scope.
type Request struct {
	Version     int64
	Purpose     Purpose
	Mode        Mode
	Region      string
	UseCases    []string
	Application string
	Environment string
}

// Normalize validates the request combination for an account of accountType
// ("AWS" or "INTERNAL") and returns the canonical use-case list.
func (r Request) Normalize(accountType string) (Request, error) {
	if r.Version < 1 {
		return r, &FieldError{Field: "version"}
	}
	switch r.Purpose {
	case PurposeGeneral, PurposeUseCase:
	default:
		return r, &FieldError{Field: "purpose"}
	}
	switch r.Mode {
	case ModeConditions, ModeExecution:
	default:
		return r, &FieldError{Field: "mode"}
	}
	if accountType == "AWS" {
		if !regionPattern.MatchString(r.Region) {
			return r, &FieldError{Field: "region"}
		}
	} else if r.Region != "" {
		return r, &FieldError{Field: "region"}
	}
	if r.Application != "" && !keyPattern.MatchString(r.Application) {
		return r, &FieldError{Field: "application"}
	}
	if r.Environment != "" && !keyPattern.MatchString(r.Environment) {
		return r, &FieldError{Field: "environment"}
	}
	if r.Environment != "" && r.Application == "" {
		return r, &FieldError{Field: "environment"}
	}
	if r.Purpose == PurposeGeneral {
		if r.Mode != ModeConditions {
			return r, &FieldError{Field: "mode"}
		}
		if len(r.UseCases) > 0 {
			return r, &FieldError{Field: "useCases"}
		}
		if r.Application != "" || r.Environment != "" {
			return r, &FieldError{Field: "application"}
		}
		return r, nil
	}
	if len(r.UseCases) == 0 || len(r.UseCases) > len(UseCases) {
		return r, &FieldError{Field: "useCases"}
	}
	seen := map[string]bool{}
	for _, key := range r.UseCases {
		if !supported(key) || seen[key] {
			return r, &FieldError{Field: "useCases"}
		}
		seen[key] = true
	}
	ordered := make([]string, 0, len(r.UseCases))
	for _, key := range UseCases {
		if seen[key] {
			ordered = append(ordered, key)
		}
	}
	r.UseCases = ordered
	return r, nil
}

func supported(key string) bool {
	for _, k := range UseCases {
		if k == key {
			return true
		}
	}
	return false
}

// Fact limits keep every persisted document bounded.
const (
	MaxFacts      = 24
	MaxFindings   = 128
	MaxDependsOn  = 16
	MaxProbeItems = 32
)

var (
	codePattern   = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
	reasonPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
)

// Finding is one check outcome. Every field is safe to persist and return.
type Finding struct {
	// ID is the stable check code and the key other findings depend on.
	ID      string  `json:"id"`
	Code    string  `json:"code"`
	Group   string  `json:"group"`
	Verdict Verdict `json:"verdict"`
	// Reason is a fixed safe code explaining the verdict (for example
	// TLS_UNTRUSTED_CA); the catalogs localize it.
	Reason string `json:"reason"`
	// Facts are typed observations; only keys of the fact registry with a
	// value of the key's kind are ever stored (see facts.go).
	Facts map[string]string `json:"facts,omitempty"`
	// Remediation is a catalog key; empty when none applies.
	Remediation      string    `json:"remediation,omitempty"`
	AffectedUseCases []string  `json:"affectedUseCases,omitempty"`
	DependsOn        []string  `json:"dependsOn,omitempty"`
	ObservedAt       time.Time `json:"observedAt"`
	DurationMillis   int64     `json:"durationMs,omitempty"`
}

// ProbeResource is a run-owned object recorded before and after creation, so
// cleanup can validate ownership and UID. It never holds credentials.
type ProbeResource struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
	// State is INTENDED, CREATED, DELETED, PRESERVED or UNKNOWN.
	State string `json:"state"`
}

// Report is the durable diagnostic aggregate.
type Report struct {
	ID              string          `json:"id"`
	OrganizationKey string          `json:"organizationKey"`
	AccountKey      string          `json:"accountKey"`
	AccountID       string          `json:"accountId"`
	CreatedBy       string          `json:"createdBy"`
	AccountVersion  int64           `json:"accountVersion"`
	Purpose         Purpose         `json:"purpose"`
	Mode            Mode            `json:"mode"`
	Scope           Scope           `json:"scope"`
	Status          Status          `json:"status"`
	Revision        int64           `json:"revision"`
	Owner           string          `json:"owner,omitempty"`
	LeaseExpiresAt  *time.Time      `json:"leaseExpiresAt,omitempty"`
	HeartbeatAt     *time.Time      `json:"heartbeatAt,omitempty"`
	Findings        []Finding       `json:"findings"`
	CompletedChecks int             `json:"completedChecks"`
	TotalChecks     int             `json:"totalChecks"`
	ProbeResources  []ProbeResource `json:"probeResources,omitempty"`
	CleanupStatus   CleanupStatus   `json:"cleanupStatus"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	StartedAt       *time.Time      `json:"startedAt,omitempty"`
	FinishedAt      *time.Time      `json:"finishedAt,omitempty"`
}

// Validate enforces every persisted invariant: scope/report agreement, the
// verdict and fact allowlists, unique findings with earlier dependencies,
// progress equal to observed findings and consistent cleanup facts.
func (r Report) Validate() error {
	if err := r.Scope.validate(r); err != nil {
		return err
	}
	switch r.Status {
	case StatusQueued, StatusRunning, StatusCancelling, StatusCompleted, StatusCancelled, StatusInterrupted:
	default:
		return ErrInvalidReport
	}
	switch r.CleanupStatus {
	case CleanupNotRequired, CleanupPending, CleanupVerified, CleanupUnknown:
	default:
		return ErrInvalidReport
	}
	if r.AccountVersion < 1 || r.Revision < 1 || r.CompletedChecks < 0 || r.TotalChecks < r.CompletedChecks || r.TotalChecks > MaxFindings ||
		len(r.Findings) > MaxFindings || len(r.ProbeResources) > MaxProbeItems || r.CompletedChecks != len(r.Findings) {
		return ErrInvalidReport
	}
	seen := map[string]int{}
	for i, f := range r.Findings {
		if err := f.Validate(); err != nil {
			return err
		}
		if _, dup := seen[f.ID]; dup {
			return ErrInvalidReport
		}
		for _, d := range f.DependsOn {
			if at, ok := seen[d]; !ok || at >= i {
				return ErrInvalidReport
			}
		}
		seen[f.ID] = i
	}
	keys := map[string]bool{}
	for _, p := range r.ProbeResources {
		if err := p.Validate(); err != nil || keys[resourceKey(p)] {
			return ErrInvalidReport
		}
		keys[resourceKey(p)] = true
	}
	switch {
	case r.Mode == ModeConditions && (len(r.ProbeResources) > 0 || r.CleanupStatus != CleanupNotRequired):
		return ErrInvalidReport
	case r.CleanupStatus == CleanupNotRequired && len(r.ProbeResources) > 0:
		return ErrInvalidReport
	case r.CleanupStatus == CleanupVerified && (len(r.ProbeResources) == 0 || !AllSettled(r.ProbeResources)):
		return ErrInvalidReport
	}
	return nil
}

func (s Scope) validate(r Report) error {
	if s.Purpose != r.Purpose || s.Mode != r.Mode || s.AccountKey != r.AccountKey || s.AccountVersion != r.AccountVersion ||
		s.AccountName == "" || len(s.AccountName) > 400 || (s.AccountType != "AWS" && s.AccountType != "INTERNAL") || !validIdentity(s.Identity) {
		return ErrInvalidReport
	}
	if s.Region != "" && !regionPattern.MatchString(s.Region) {
		return ErrInvalidReport
	}
	if (s.AccountType == "AWS") != (s.Region != "") {
		return ErrInvalidReport
	}
	switch s.Purpose {
	case PurposeGeneral:
		if s.Mode != ModeConditions || s.Context != nil || len(s.UseCases) > 0 {
			return ErrInvalidReport
		}
	case PurposeUseCase:
		if s.Mode != ModeConditions && s.Mode != ModeExecution {
			return ErrInvalidReport
		}
		if len(s.UseCases) == 0 || len(s.UseCases) > len(UseCases) {
			return ErrInvalidReport
		}
		seen := map[string]bool{}
		for _, u := range s.UseCases {
			if !supported(u) || seen[u] {
				return ErrInvalidReport
			}
			seen[u] = true
		}
	default:
		return ErrInvalidReport
	}
	if c := s.Context; c != nil {
		if !keyPattern.MatchString(c.ApplicationKey) || (c.EnvironmentKey != "" && !keyPattern.MatchString(c.EnvironmentKey)) || len(c.ApplicationName) > 400 ||
			len(c.EnvironmentType) > 64 || len(c.CurrentDeploymentID) > 64 {
			return ErrInvalidReport
		}
	}
	return nil
}

// Counts are the verdict totals of a report.
type Counts struct {
	Pass    int `json:"pass"`
	Fail    int `json:"fail"`
	Blocked int `json:"blocked"`
	Unknown int `json:"unknown"`
	NA      int `json:"na"`
	// Pending are planned checks without a finding yet.
	Pending int `json:"pending"`
}

// Counts derives verdict totals from the findings and planned total.
func (r Report) Counts() Counts {
	var c Counts
	for _, f := range r.Findings {
		switch f.Verdict {
		case VerdictPass:
			c.Pass++
		case VerdictFail:
			c.Fail++
		case VerdictBlocked:
			c.Blocked++
		case VerdictUnknown:
			c.Unknown++
		case VerdictNA:
			c.NA++
		}
	}
	if pending := r.TotalChecks - len(r.Findings); pending > 0 {
		c.Pending = pending
	}
	return c
}

// Ready reports whether the report may be shown as fully satisfied: a
// completed run, every planned check observed, nothing failed, blocked or
// unknown, and no unresolved cleanup. It is never a promise about the future.
func (r Report) Ready() bool {
	if r.Status != StatusCompleted || r.CleanupStatus == CleanupPending || r.CleanupStatus == CleanupUnknown {
		return false
	}
	c := r.Counts()
	return c.Fail == 0 && c.Blocked == 0 && c.Unknown == 0 && c.Pending == 0 && c.Pass > 0
}

// Summary is the safe latest-report projection shown on the account list.
type Summary struct {
	ReportID      string        `json:"reportId"`
	Scope         Scope         `json:"scope"`
	Status        Status        `json:"status"`
	Counts        Counts        `json:"counts"`
	CleanupStatus CleanupStatus `json:"cleanupStatus"`
	CreatedAt     time.Time     `json:"createdAt"`
	FinishedAt    *time.Time    `json:"finishedAt,omitempty"`
	Ready         bool          `json:"ready"`
}

// SummaryOf projects a report.
func SummaryOf(r Report) Summary {
	return Summary{ReportID: r.ID, Scope: r.Scope, Status: r.Status, Counts: r.Counts(), CleanupStatus: r.CleanupStatus, CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt, Ready: r.Ready()}
}
