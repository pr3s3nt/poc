package environment

import (
	"errors"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WorkloadExecution is the versioned platform extension carried by desired and
// applied workload snapshots. It never contains secret bytes or host paths.
type WorkloadExecution struct {
	Version  int            `json:"version"`
	Kind     string         `json:"kind"`
	Template TemplatePin    `json:"template"`
	Schedule *Schedule      `json:"schedule,omitempty"`
	Mounts   []StorageMount `json:"mounts,omitempty"`
	Headless bool           `json:"headless,omitempty"`
}

type TemplatePin struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type Schedule struct {
	Expression                 string `json:"expression"`
	TimeZone                   string `json:"timeZone"`
	Suspend                    bool   `json:"suspend"`
	ConcurrencyPolicy          string `json:"concurrencyPolicy"`
	StartingDeadlineSeconds    *int64 `json:"startingDeadlineSeconds,omitempty"`
	ActiveDeadlineSeconds      *int64 `json:"activeDeadlineSeconds,omitempty"`
	BackoffLimit               *int32 `json:"backoffLimit,omitempty"`
	SuccessfulJobsHistoryLimit *int32 `json:"successfulJobsHistoryLimit,omitempty"`
	FailedJobsHistoryLimit     *int32 `json:"failedJobsHistoryLimit,omitempty"`
}

// Resource is a desired resource alias, not a PVC name. Planning resolves it
// against the workload's dependency graph on the same target and namespace.
type StorageMount struct {
	Resource  string `json:"resource"`
	Container string `json:"container"`
	Path      string `json:"path"`
	ReadOnly  bool   `json:"readOnly"`
}

// ErrExecutionUnavailable prevents legacy rendering from silently ignoring
// execution intent until the renderer capability contract is implemented.
var ErrExecutionUnavailable = errors.New("environment: workload execution capability is unavailable")

var ErrExecution = errors.New("environment: invalid workload execution")
var templateID = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
var versionID = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var sha256ID = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Validate checks the execution policy without I/O. Cron syntax is validated by
// the numeric five-field grammar below before admission, plus the timezone.
func (e *WorkloadExecution) Validate(containers map[string]Container, replicas int, hasPorts bool, resources map[string]bool) error {
	if e == nil {
		return nil
	}
	if e.Version != 1 || !templateID.MatchString(e.Template.ID) || !versionID.MatchString(e.Template.Version) || !sha256ID.MatchString(e.Template.SHA256) {
		return ErrExecution
	}
	switch e.Kind {
	case "Deployment":
		if e.Schedule != nil {
			return ErrExecution
		}
	case "StatefulSet":
		if e.Schedule != nil || replicas != 1 {
			return ErrExecution
		}
	case "CronJob":
		if e.Schedule == nil || hasPorts || replicas > 1 {
			return ErrExecution
		}
		if err := e.Schedule.Validate(); err != nil {
			return err
		}
	default:
		return ErrExecution
	}
	if e.Headless && !hasPorts {
		return ErrExecution
	}
	seen := map[string][]string{}
	for _, mount := range e.Mounts {
		if _, ok := containers[mount.Container]; !ok || !resources[mount.Resource] {
			return ErrExecution
		}
		if !strings.HasPrefix(mount.Path, "/") || mount.Path == "/" || path.Clean(mount.Path) != mount.Path || strings.ContainsAny(mount.Path, "\x00\r\n") {
			return ErrExecution
		}
		for _, prefix := range []string{"/vault", "/var/run/secrets"} {
			if mount.Path == prefix || strings.HasPrefix(mount.Path, prefix+"/") || strings.HasPrefix(prefix, mount.Path+"/") {
				return ErrExecution
			}
		}
		for _, previous := range seen[mount.Container] {
			if previous == mount.Path || strings.HasPrefix(previous, mount.Path+"/") || strings.HasPrefix(mount.Path, previous+"/") {
				return ErrExecution
			}
		}
		seen[mount.Container] = append(seen[mount.Container], mount.Path)
	}
	return nil
}

func (s Schedule) Validate() error {
	if !validCron(s.Expression) || len(s.Expression) > 256 || s.TimeZone == "" || strings.ContainsAny(s.Expression, "\r\n") {
		return ErrExecution
	}
	// Local depends on the host; published policies must identify a real timezone.
	if s.TimeZone == "Local" {
		return ErrExecution
	}
	if _, err := time.LoadLocation(s.TimeZone); err != nil {
		return ErrExecution
	}
	switch s.ConcurrencyPolicy {
	case "Allow", "Forbid", "Replace":
	default:
		return ErrExecution
	}
	if s.StartingDeadlineSeconds != nil && *s.StartingDeadlineSeconds < 1 || s.ActiveDeadlineSeconds != nil && *s.ActiveDeadlineSeconds < 1 || s.BackoffLimit != nil && *s.BackoffLimit < 0 || s.SuccessfulJobsHistoryLimit != nil && *s.SuccessfulJobsHistoryLimit < 0 || s.FailedJobsHistoryLimit != nil && *s.FailedJobsHistoryLimit < 0 {
		return ErrExecution
	}
	return nil
}

// Clone keeps desired and applied documents independent, including optional
// numeric policy fields and nested slices.
func (e *WorkloadExecution) Clone() *WorkloadExecution {
	if e == nil {
		return nil
	}
	out := *e
	out.Mounts = append([]StorageMount(nil), e.Mounts...)
	if e.Schedule != nil {
		schedule := *e.Schedule
		copyInt64 := func(p *int64) *int64 {
			if p == nil {
				return nil
			}
			v := *p
			return &v
		}
		copyInt32 := func(p *int32) *int32 {
			if p == nil {
				return nil
			}
			v := *p
			return &v
		}
		schedule.StartingDeadlineSeconds = copyInt64(schedule.StartingDeadlineSeconds)
		schedule.ActiveDeadlineSeconds = copyInt64(schedule.ActiveDeadlineSeconds)
		schedule.BackoffLimit = copyInt32(schedule.BackoffLimit)
		schedule.SuccessfulJobsHistoryLimit = copyInt32(schedule.SuccessfulJobsHistoryLimit)
		schedule.FailedJobsHistoryLimit = copyInt32(schedule.FailedJobsHistoryLimit)
		out.Schedule = &schedule
	}
	return &out
}

// validCron deliberately accepts the portable numeric five-field subset. Names,
// aliases and embedded timezone directives are refused, never reinterpreted.
func validCron(expression string) bool {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return false
	}
	bounds := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	for i, field := range fields {
		min, max := bounds[i][0], bounds[i][1]
		number := func(raw string) (int, bool) {
			if raw == "" {
				return 0, false
			}
			for _, c := range raw {
				if c < '0' || c > '9' {
					return 0, false
				}
			}
			v, err := strconv.Atoi(raw)
			return v, err == nil && v >= min && v <= max
		}
		for _, term := range strings.Split(field, ",") {
			parts := strings.Split(term, "/")
			if len(parts) > 2 {
				return false
			}
			if len(parts) == 2 {
				step, err := strconv.Atoi(parts[1])
				if err != nil || step < 1 || step > max-min+1 {
					return false
				}
			}
			if parts[0] == "*" {
				continue
			}
			span := strings.Split(parts[0], "-")
			if len(span) > 2 {
				return false
			}
			first, ok := number(span[0])
			if !ok {
				return false
			}
			if len(span) == 2 {
				last, ok := number(span[1])
				if !ok || last < first {
					return false
				}
			}
		}
	}
	return true
}
