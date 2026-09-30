package planning

import "fmt"

// Stage names the planning step that rejected a request. Callers that expose
// failures publicly use Stage and Reason to choose a fixed message: a raw
// message of any stage can quote catalog, Definition, Terraform module or
// other persisted module content (UC-05 API mapping), so it must not be
// echoed publicly.
type Stage string

// Planning stages.
const (
	StageScore     Stage = "score"
	StageBefore    Stage = "before-state"
	StageCandidate Stage = "candidate"
	StageRoutes    Stage = "public-routes"
	StageCatalog   Stage = "catalog"
	StageGraph     Stage = "graph"
	StageReference Stage = "reference"
	StageContract  Stage = "contract"
	StageSchedule  Stage = "schedule"
)

// Before-state reasons.
const (
	ReasonWorkloadExists     = "workload-exists"
	ReasonWorkloadNotCurrent = "workload-not-current"
	ReasonBeforeMismatch     = "before-mismatch"
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
	return &StageError{Stage: stage, Err: err}
}

func beforeErr(reason, format string, args ...any) error {
	return &StageError{Stage: StageBefore, Reason: reason, Err: fmt.Errorf(format, args...)}
}
