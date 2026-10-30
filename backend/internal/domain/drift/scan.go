package drift

import (
	"encoding/hex"
	"errors"
	"time"
)

var ErrScan = errors.New("drift: invalid scan request or transition")

type ScanStatus string

const (
	Queued      ScanStatus = "QUEUED"
	Running     ScanStatus = "RUNNING"
	Succeeded   ScanStatus = "SUCCEEDED"
	Failed      ScanStatus = "FAILED"
	Interrupted ScanStatus = "INTERRUPTED"
	Skipped     ScanStatus = "SKIPPED"
)

type Result string

const (
	InSync              Result = "IN_SYNC"
	Drifted             Result = "DRIFTED"
	Unknown             Result = "UNKNOWN"
	BaselineUnavailable Result = "BASELINE_UNAVAILABLE"
	NotApplicable       Result = "NOT_APPLICABLE"
)

type InstancePin struct {
	InstanceID      string `json:"instanceId"`
	InstanceVersion int64  `json:"instanceVersion"`
	BaselineID      string `json:"baselineId,omitempty"`
}
type InstanceResult struct {
	InstanceID string `json:"instanceId"`
	Descriptor string `json:"descriptor"`
	Result     Result `json:"result"`
	Report     Report `json:"report"`
	ReasonCode string `json:"reasonCode,omitempty"`
}
type Scan struct {
	ID                 string           `json:"id"`
	OrganizationID     string           `json:"-"`
	EnvironmentID      string           `json:"environmentId"`
	ApplicationKey     string           `json:"-"`
	EnvironmentKey     string           `json:"-"`
	OperationID        string           `json:"operationId,omitempty"`
	RequestID          string           `json:"requestId"`
	RequestDigest      string           `json:"-"`
	EnvironmentVersion int64            `json:"environmentVersion"`
	Status             ScanStatus       `json:"status"`
	Result             Result           `json:"result"`
	Scope              []InstancePin    `json:"scope"`
	Findings           []InstanceResult `json:"findings"`
	Owner              string           `json:"-"`
	Fence              int64            `json:"-"`
	Version            int64            `json:"-"`
	StartedAt          *time.Time       `json:"startedAt,omitempty"`
	FinishedAt         *time.Time       `json:"finishedAt,omitempty"`
	CreatedAt          time.Time        `json:"createdAt"`
	ReasonCode         string           `json:"reasonCode,omitempty"`
}

func (r Report) Result() Result {
	if !r.Complete {
		return Unknown
	}
	if len(r.External) > 0 || len(r.Proposed) > 0 || len(r.OutputNames) > 0 || r.OutputsUnknown {
		return Drifted
	}
	return InSync
}
func (s ScanStatus) Terminal() bool {
	return s == Succeeded || s == Failed || s == Interrupted || s == Skipped
}

func (s Scan) ValidateQueued() error {
	if s.ID == "" || s.OrganizationID == "" || s.EnvironmentID == "" || s.ApplicationKey == "" || s.EnvironmentKey == "" || s.RequestID == "" || s.EnvironmentVersion < 1 || s.Status != Queued || s.Result != Unknown || s.CreatedAt.IsZero() || s.OperationID != "" || s.Owner != "" || s.Fence != 0 {
		return ErrScan
	}
	decoded, err := hex.DecodeString(s.RequestDigest)
	if err != nil || len(decoded) != 32 {
		return ErrScan
	}
	seen := map[string]bool{}
	for _, p := range s.Scope {
		if p.InstanceID == "" || p.InstanceVersion < 1 || seen[p.InstanceID] {
			return ErrScan
		}
		seen[p.InstanceID] = true
	}
	return nil
}

func (s Scan) ValidateTerminal() error {
	if !s.Status.Terminal() || s.FinishedAt == nil || s.FinishedAt.Before(s.CreatedAt) {
		return ErrScan
	}
	switch s.Result {
	case InSync, Drifted, Unknown, BaselineUnavailable, NotApplicable:
	default:
		return ErrScan
	}
	if s.Result == InSync {
		if s.Status != Succeeded || len(s.Findings) == 0 {
			return ErrScan
		}
		for _, f := range s.Findings {
			if f.Result != InSync || f.Report.Result() != InSync {
				return ErrScan
			}
		}
	}
	seen := map[string]bool{}
	scope := map[string]bool{}
	for _, p := range s.Scope {
		scope[p.InstanceID] = true
	}
	for _, f := range s.Findings {
		if !scope[f.InstanceID] || seen[f.InstanceID] {
			return ErrScan
		}
		seen[f.InstanceID] = true
	}
	if s.Result == InSync && len(seen) != len(scope) {
		return ErrScan
	}
	return nil
}
