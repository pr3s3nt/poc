// Package progress contains credential-free execution facts, independent of phases.
package progress

import (
	"context"
	"encoding/json"
	"regexp"
	"time"
)

type Pod struct {
	Phase    string `json:"phase,omitempty"`
	Workload string `json:"workload,omitempty"`
	Name     string `json:"name"`
	Image    string `json:"image,omitempty"`
	Reason   string `json:"reason"`
	Restarts int    `json:"restarts"`
}
type Event struct {
	Workload string    `json:"workload,omitempty"`
	Reason   string    `json:"reason"`
	Object   string    `json:"object"`
	At       time.Time `json:"at"`
	Count    int       `json:"count,omitempty"`
}
type Log struct {
	At     time.Time `json:"at"`
	Stage  string    `json:"stage"`
	Status string    `json:"status"`
}
type Step struct {
	Logs  []Log  `json:"logs,omitempty"`
	Stage string `json:"stage,omitempty"`

	ResourceKind string     `json:"resourceKind,omitempty"`
	ResourceName string     `json:"resourceName,omitempty"`
	Namespace    string     `json:"namespace,omitempty"`
	ID           string     `json:"id"`
	Parent       string     `json:"parent,omitempty"`
	Workload     string     `json:"workload,omitempty"`
	DeploymentID string     `json:"deploymentId,omitempty"`
	Kind         string     `json:"kind"`
	Status       string     `json:"status"`
	Reason       string     `json:"reason,omitempty"`
	StartedAt    time.Time  `json:"startedAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
	Deadline     *time.Time `json:"deadline,omitempty"`
	Pods         []Pod      `json:"pods,omitempty"`
	Events       []Event    `json:"events,omitempty"`
	Commit       string     `json:"commit,omitempty"`
}
type Record struct {
	Version        int        `json:"version"`
	Steps          []Step     `json:"steps"`
	Checks         []Step     `json:"checks,omitempty"`
	PriorHeartbeat *time.Time `json:"priorHeartbeat,omitempty"`
}

var identity = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/#@+-]{0,255}$`)
var image = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/:@-]{0,511}$`)

func Identity(v string) string {
	if identity.MatchString(v) {
		return v
	}
	return ""
}
func Reason(v string) string {
	switch v {
	case "ABSENT", "PUBLISHED", "DELIVERY_CONFIRMED", "WAITING_DEPENDENCY", "ImagePullBackOff", "ErrImagePull", "InvalidImageName", "CrashLoopBackOff", "CreateContainerConfigError", "CreateContainerError", "FailedScheduling", "Pending", "OOMKilled", "Error", "Completed", "Running", "ContainerCreating", "PodInitializing", "Failed", "BackOff", "FailedMount", "FailedAttachVolume", "Unhealthy", "FailedToRetrieveImagePullSecret", "Scheduled", "Pulled", "Created", "Started", "SuccessfulCreate", "NotReady", "OWNERSHIP", "CONTROLLER", "CONFIG", "TARGET", "UNKNOWN", "GIT", "GIT_CONFLICT", "GIT_INVALID", "TERRAFORM", "JOB_FAILED", "SECRET_NOT_READY", "WAITING_FLEET", "WAITING_READINESS", "RECOVERY_REQUIRED", "PRIOR_STOPPED":
		return v
	}
	return "UNKNOWN"
}
func (s Step) Safe() Step {
	s.ResourceKind = Identity(s.ResourceKind)
	s.ResourceName = Identity(s.ResourceName)
	s.Namespace = Identity(s.Namespace)
	s.ID = Identity(s.ID)
	s.Parent = Identity(s.Parent)
	s.Workload = Identity(s.Workload)
	s.DeploymentID = Identity(s.DeploymentID)
	s.Kind = Identity(s.Kind)
	switch s.Stage {
	case "INTENT", "INIT", "PLANNING", "PLANNED", "APPLYING", "APPLIED", "OUTPUTS", "READINESS", "READY", "NO_CHANGE", "CHECKING", "DESTROYING", "VERIFYING", "DESTROYED", "RECOVERY", "":
	default:
		s.Stage = "UNKNOWN"
	}
	logs := []Log{}
	for _, l := range s.Logs {
		switch l.Stage {
		case "INTENT", "INIT", "PLANNING", "PLANNED", "APPLYING", "APPLIED", "OUTPUTS", "READINESS", "READY", "NO_CHANGE", "CHECKING", "DESTROYING", "VERIFYING", "DESTROYED", "RECOVERY":
		default:
			l.Stage = "UNKNOWN"
		}
		switch l.Status {
		case "RUNNING", "SUCCEEDED", "FAILED", "INTERRUPTED":
		default:
			l.Status = "UNKNOWN"
		}
		logs = append(logs, l)
		if len(logs) >= 100 {
			break
		}
	}
	s.Logs = logs

	switch s.Status {
	case "PENDING", "RUNNING", "SUCCEEDED", "FAILED", "INTERRUPTED":
	default:
		s.Status = "PENDING"
	}
	if s.Reason != "" {
		s.Reason = Reason(s.Reason)
	}
	s.Commit = Identity(s.Commit)
	var pods []Pod
	for _, p := range s.Pods {
		switch p.Phase {
		case "Pending", "Running", "Succeeded", "Failed", "Unknown", "":
		default:
			p.Phase = "Unknown"
		}
		p.Workload = Identity(p.Workload)
		p.Name = Identity(p.Name)
		if !image.MatchString(p.Image) {
			p.Image = ""
		}
		p.Reason = Reason(p.Reason)
		if p.Restarts < 0 {
			p.Restarts = 0
		}
		if p.Name != "" {
			pods = append(pods, p)
		}
		if len(pods) >= 100 {
			break
		}
	}
	s.Pods = pods
	var events []Event
	for _, e := range s.Events {
		e.Workload = Identity(e.Workload)
		e.Reason = Reason(e.Reason)
		e.Object = Identity(e.Object)
		if e.Object != "" {
			events = append(events, e)
		}
		if len(events) >= 50 {
			break
		}
	}
	s.Events = events
	return s
}
func Read(value any) Record {
	r := Record{Version: 1, Steps: []Step{}}
	b, err := json.Marshal(value)
	if err == nil && value != nil {
		_ = json.Unmarshal(b, &r)
	}
	r.Version = 1
	if len(r.Steps) > 1000 {
		r.Steps = r.Steps[:1000]
	}
	if len(r.Checks) > 1000 {
		r.Checks = r.Checks[:1000]
	}
	for i := range r.Steps {
		r.Steps[i] = r.Steps[i].Safe()
	}
	for i := range r.Checks {
		r.Checks[i] = r.Checks[i].Safe()
	}
	return r
}

type reporterKey struct{}
type Reporter func(Step, bool)

func WithReporter(ctx context.Context, f Reporter) context.Context {
	return context.WithValue(ctx, reporterKey{}, f)
}
func Report(ctx context.Context, s Step) {
	if f, ok := ctx.Value(reporterKey{}).(Reporter); ok {
		f(s.Safe(), false)
	}
}
func Check(ctx context.Context, s Step) {
	if f, ok := ctx.Value(reporterKey{}).(Reporter); ok {
		f(s.Safe(), true)
	}
}
