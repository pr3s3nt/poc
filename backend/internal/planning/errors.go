package planning

import (
	"errors"
	"fmt"
)

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
	ReasonUnconfigured = "environment-unconfigured"
	// Renderer selection reasons (UC-05 BR-09): no workload rendering
	// Definition matches, several tie, or the selected one is unusable.
	ReasonRendererNoMatch     = "renderer-no-match"
	ReasonRendererAmbiguous   = "renderer-ambiguous"
	ReasonRendererUnavailable = "renderer-unavailable"
	// Placement reasons (ADR-016): a typed target is missing, ambiguous,
	// contradictory or unsupported before any consumer runs.
	// A resource (for example the cluster) matches no Definition, or several equally.
	ReasonResourceNoMatch   = "resource-no-match"
	ReasonResourceAmbiguous = "resource-ambiguous"
	ReasonPlacement         = "placement"
	// ReasonResourceConflict: two authored sources give one scoped resource different params.
	ReasonResourceConflict = "resource-conflict"
	ReasonStorageImmutable = "storage-immutable"
	ReasonMultiCluster     = "multi-cluster-unsupported"
	// ReasonTargetChanged: an applied workload would move to another target.
	ReasonTargetChanged = "target-changed"
	// ReasonRetiredTarget: the Environment still carries a retired internal deployment Connection.
	ReasonRetiredTarget = "retired-target"
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
		ReasonUnconfigured:        "a workload or resource placement cannot be resolved from Resource Definitions; ask a platform engineer to check the Definitions",
		ReasonRendererNoMatch:     "no workload rendering template (score-k8s Definition) matches this workload for the Application and Environment; ask a platform engineer to register one",
		ReasonRendererAmbiguous:   "several workload rendering templates (score-k8s Definitions) match this workload equally; ask a platform engineer to leave exactly one",
		ReasonStorageImmutable:    "storage parameters cannot change without an explicit migration or verified expansion capability",
		ReasonRendererUnavailable: "the workload rendering template needs a score-k8s bundle that is not installed; ask a platform engineer to check the platform release",
		ReasonResourceNoMatch:     "a resource of this Score, or a resource it needs such as the Kubernetes cluster, matches no Resource Definition for this Application and Environment; ask a platform engineer to register one",
		ReasonResourceAmbiguous:   "several Resource Definitions match a resource of this Score equally, so none was chosen; ask a platform engineer to leave exactly one most specific Definition",
		ReasonResourceConflict:    "two Score or Definition sources declare the same resource with different params; make them identical or use different resource ids",
		ReasonPlacement:           "the workload or resource target cannot be resolved from Resource Definitions: a namespace or cluster reference is missing, contradictory or backed by an unavailable Driver Account; ask a platform engineer to check the Definitions",
		ReasonRetiredTarget:       "this Environment carries a retired internal deployment connection from an older baseline; internal placement now comes from Resource Definitions, so the baseline must be recreated",
		ReasonMultiCluster:        "this Environment resolves more than one Kubernetes cluster; multiple clusters in one Environment are not supported yet",
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

// IsRendererReason reports whether reason is a renderer selection failure.
func IsRendererReason(reason string) bool {
	return reason == ReasonRendererNoMatch || reason == ReasonRendererAmbiguous || reason == ReasonRendererUnavailable
}

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
