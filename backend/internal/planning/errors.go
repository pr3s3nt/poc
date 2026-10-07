package planning

import (
	"errors"
	"fmt"
)

// ErrEnvironmentUnconfigured reports an Environment without an execution
// Connection. Preview and deploy fail with it before any side effect.
var ErrEnvironmentUnconfigured = errors.New("planning: environment has no execution connection")

// Stage names the planning step that rejected a request. Callers that expose
// failures publicly use Stage and Reason to choose a fixed message: a raw
// message of any stage can quote catalog, Definition, Terraform module or
// other persisted module content (UC-05 API mapping), so it must not be
// echoed publicly.
type Stage string

// Planning stages.
const (
	StageScore       Stage = "score"
	StageBefore      Stage = "before-state"
	StageCandidate   Stage = "candidate"
	StageRoutes      Stage = "public-routes"
	StageServiceRefs Stage = "service-references"
	StageCatalog     Stage = "catalog"
	StageGraph       Stage = "graph"
	StageReference   Stage = "reference"
	StageContract    Stage = "contract"
	StageSchedule    Stage = "schedule"
)

// Before-state reasons.
const (
	ReasonWorkloadExists     = "workload-exists"
	ReasonWorkloadNotCurrent = "workload-not-current"
	ReasonBeforeMismatch     = "before-mismatch"
)

// Reasons of other stages.
const (
	ReasonLegacyInfrastructure = "legacy-infrastructure-reference"
	ReasonUnconfigured         = "environment-unconfigured"
)

// StageError wraps a planning failure with the stage that produced it. Its
// message is the wrapped message, so existing callers see no difference.
type StageError struct {
	Stage  Stage
	Reason string
	Err    error
}

func (e *StageError) Error() string { return e.Err.Error() }
func (e *StageError) Unwrap() error { return e.Err }

func stageErr(stage Stage, err error) error {
	if err == nil {
		return nil
	}
	var existing *StageError
	if errors.As(err, &existing) {
		return err
	}
	return &StageError{Stage: stage, Err: err}
}

func beforeErr(reason, format string, args ...any) error {
	return &StageError{Stage: StageBefore, Reason: reason, Err: fmt.Errorf(format, args...)}
}

// Fixed public sentences. Planner messages are never shown to users or
// persisted as public failure reasons: any stage can quote catalog,
// Definition, Terraform module or other persisted module content.
var (
	beforeMessages = map[string]string{
		ReasonWorkloadExists:     "the workload already exists in the current Deployment Set; use update with scoreBefore",
		ReasonWorkloadNotCurrent: "the workload is not part of the current Deployment Set; use deploy to add it",
		ReasonBeforeMismatch:     "scoreBefore does not match the current Deployment Set for this workload; refresh it from the currently deployed Score",
	}
	reasonMessages = map[string]string{
		ReasonLegacyInfrastructure: "a Resource Definition references the application-scoped VPC/EKS; ask a platform engineer to use the @infra token (for example vpc.default#@infra)",
		ReasonUnconfigured:         "the Environment has no execution connection; set one in Environment Settings",
	}
	stageMessages = map[Stage]string{
		StageScore:       "the Score does not satisfy the registered Resource Type contracts (unregistered type, invalid params or unknown resource output)",
		StageBefore:      "scoreBefore does not match the current Deployment Set for this workload",
		StageCandidate:   "the Candidate Deployment Set cannot be built from this Score, for example because a shared resource conflicts with the current one",
		StageRoutes:      "public routes of the resulting Environment are invalid: a duplicate path or an undeclared Service port",
		StageServiceRefs: "a workload in the resulting Environment references a Service or Service port that would not exist; update or delete its consumers in the same change",
		StageCatalog:     "the registered Resource Definitions cannot be used for planning; ask a platform engineer to check them",
		StageGraph:       "the Resource Graph cannot be built or matched for this Score; check resource types, classes and ids against the registered Resource Definitions",
		StageReference:   "a resource reference cannot be resolved against the matched Resource Definitions",
		StageContract:    "a matched Resource Definition has an invalid Terraform contract; ask a platform engineer to check it",
		StageSchedule:    "the Resource Graph has a dependency cycle",
	}
)

// IsPublicMessage reports whether text is one of the fixed public sentences.
func IsPublicMessage(text string) bool {
	if text == "planning failed for this Score" {
		return true
	}
	for _, message := range beforeMessages {
		if message == text {
			return true
		}
	}
	for _, message := range reasonMessages {
		if message == text {
			return true
		}
	}
	for _, message := range stageMessages {
		if message == text {
			return true
		}
	}
	return false
}

// PublicMessage returns the fixed public sentence for a staged planning
// failure; ok is false for errors without a stage.
func PublicMessage(err error) (string, bool) {
	var stage *StageError
	if !errors.As(err, &stage) {
		return "", false
	}
	if message, ok := beforeMessages[stage.Reason]; ok {
		return message, true
	}
	if message, ok := reasonMessages[stage.Reason]; ok {
		return message, true
	}
	if message, ok := stageMessages[stage.Stage]; ok {
		return message, true
	}
	return "planning failed for this Score", true
}
