// Package telemetry models received application telemetry, not deployment status.
package telemetry

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	LogRetention    = 7 * 24 * time.Hour
	MetricRetention = 15 * 24 * time.Hour
	Interval        = 30 * time.Second
	LeaseDuration   = time.Minute
	ReadTimeout     = 20 * time.Second
	MaxMessage      = 16 << 10
	MaxLogRows      = 100000
	MaxLogBytes     = 64 << 20
	MaxSamples      = 50000
	MaxEvents       = 10000
	MaxPage         = 200
)

type Code string

const (
	Waiting      Code = "WAITING"
	OK           Code = "OK"
	Unavailable  Code = "UNAVAILABLE"
	Forbidden    Code = "FORBIDDEN"
	Failed       Code = "FAILED"
	Truncated    Code = "TRUNCATED"
	Stale        Code = "STALE"
	Gap          Code = "GAP"
	StorageLimit Code = "STORAGE_LIMIT"
)

func SafeCode(c Code) Code {
	switch c {
	case Waiting, OK, Unavailable, Forbidden, Failed, Truncated, Stale, Gap, StorageLimit:
		return c
	}
	return Failed
}

type Scope struct {
	OrganizationKey string `json:"organizationKey"`
	ApplicationKey  string `json:"applicationKey"`
	EnvironmentKey  string `json:"environmentKey"`
	Generation      int64  `json:"generation"`
}

type Source struct {
	Scope
	Key        string `json:"key"`
	WorkloadID string `json:"workloadId"`
	TargetHash string `json:"targetHash"`
}

// Cursor identifies a single pod/container/restart stream. Previous is marked
// complete only after successfully inspecting the terminated container.
type Cursor struct {
	Timestamp time.Time `json:"timestamp"`
	Previous  bool      `json:"previous,omitempty"`
}

type State struct {
	Source       Source            `json:"source"`
	Checkpoints  map[string]Cursor `json:"checkpoints"`
	Owner        string            `json:"owner"`
	Fence        int64             `json:"fence"`
	LeaseUntil   time.Time         `json:"leaseUntil"`
	LogsCode     Code              `json:"logsCode"`
	MetricsCode  Code              `json:"metricsCode"`
	ObservedAt   *time.Time        `json:"observedAt"`
	LastLogAt    *time.Time        `json:"lastLogAt"`
	LastMetricAt *time.Time        `json:"lastMetricAt"`
	Limited      bool              `json:"storageLimited"`
}

type Log struct {
	ID           string    `json:"id"`
	SourceKey    string    `json:"sourceKey"`
	WorkloadID   string    `json:"workloadId"`
	Timestamp    time.Time `json:"timestamp"`
	PodUID       string    `json:"podUid"`
	PodName      string    `json:"podName"`
	Container    string    `json:"container"`
	RestartIndex int64     `json:"restartIndex"`
	Level        string    `json:"level"`
	Message      string    `json:"message"`
}

type Sample struct {
	ID          string     `json:"id"`
	SourceKey   string     `json:"sourceKey"`
	WorkloadID  string     `json:"workloadId"`
	ObservedAt  time.Time  `json:"timestamp"`
	MetricsAt   *time.Time `json:"metricsAt"`
	CPU         *float64   `json:"cpuMilli"`
	Memory      *float64   `json:"memoryBytes"`
	CPULimit    *float64   `json:"cpuLimitMilli"`
	MemoryLimit *float64   `json:"memoryLimitBytes"`
	ReadyPods   int64      `json:"readyPods"`
	TotalPods   int64      `json:"totalPods"`
	Restarts    int64      `json:"restarts"`
}

type Event struct {
	ID          string    `json:"id"`
	SourceKey   string    `json:"sourceKey"`
	WorkloadID  string    `json:"workloadId"`
	Timestamp   time.Time `json:"timestamp"`
	PodUID      string    `json:"podUid"`
	PodName     string    `json:"podName"`
	Container   string    `json:"container"`
	Kind        string    `json:"kind"`
	Reason      string    `json:"reason"`
	Occurrences int64     `json:"occurrences"`
}

type Batch struct {
	At      time.Time
	State   State
	Logs    []Log
	Samples []Sample
	Events  []Event
}

type Query struct {
	Scope                   Scope
	Kind                    string
	From, To                time.Time
	Workload, Search, Level string
	Limit                   int
	Before                  *Position
	Step                    time.Duration
}

type Position struct {
	Time time.Time `json:"t"`
	ID   string    `json:"i"`
}

func EncodeCursor(l Log) string {
	b, _ := json.Marshal(Position{l.Timestamp, l.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}
func DecodeCursor(raw string) (*Position, error) {
	if len(raw) > 512 {
		return nil, errors.New("telemetry: invalid cursor")
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.New("telemetry: invalid cursor")
	}
	var p Position
	if json.Unmarshal(b, &p) != nil || p.Time.IsZero() || len(p.ID) != 64 {
		return nil, errors.New("telemetry: invalid cursor")
	}
	if _, err = hex.DecodeString(p.ID); err != nil {
		return nil, errors.New("telemetry: invalid cursor")
	}
	return &p, nil
}

func Hash(parts ...string) string {
	b, _ := json.Marshal(parts)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func Level(message string) string {
	var document map[string]any
	level := ""
	if json.Unmarshal([]byte(message), &document) == nil {
		for _, key := range []string{"level", "severity", "log.level"} {
			if v, ok := document[key].(string); ok {
				level = v
				break
			}
		}
	}
	if level == "" {
		fields := strings.Fields(message)
		if len(fields) > 0 {
			level = strings.Trim(fields[0], "[]:")
		}
	}
	switch strings.ToUpper(level) {
	case "ERROR", "ERR", "FATAL", "PANIC":
		return "ERROR"
	case "WARN", "WARNING":
		return "WARN"
	case "INFO", "INFORMATION":
		return "INFO"
	case "DEBUG", "TRACE":
		return "DEBUG"
	}
	return "UNKNOWN"
}

func ValidNumber(n *float64) bool {
	return n == nil || (!math.IsNaN(*n) && !math.IsInf(*n, 0) && *n >= 0)
}

var keyShape = regexp.MustCompile(`^[0-9a-f]{64}$`)
var nameShape = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)

func (s Source) Valid() bool {
	return keyShape.MatchString(s.Key) && keyShape.MatchString(s.TargetHash) && s.OrganizationKey != "" && s.ApplicationKey != "" && s.EnvironmentKey != "" && s.Generation >= 0 && nameShape.MatchString(s.WorkloadID)
}

func (b Batch) Validate() error {
	s := b.State.Source
	if !s.Valid() || b.State.Owner == "" || b.State.Fence < 1 || b.At.IsZero() || len(b.State.Checkpoints) > 512 || len(b.Logs) > 8000 || len(b.Samples) > 4 || len(b.Events) > 256 {
		return errors.New("telemetry: invalid batch")
	}
	for key, c := range b.State.Checkpoints {
		if len(key) > 512 || c.Timestamp.After(b.At.Add(time.Minute)) {
			return errors.New("telemetry: invalid checkpoint")
		}
	}
	for _, l := range b.Logs {
		if !keyShape.MatchString(l.ID) || l.WorkloadID != s.WorkloadID || len(l.Message) > MaxMessage || l.Timestamp.IsZero() || l.Timestamp.Before(b.At.Add(-LogRetention)) || l.Timestamp.After(b.At.Add(time.Minute)) || !nameShape.MatchString(l.PodName) || l.PodUID == "" || l.RestartIndex < 0 {
			return errors.New("telemetry: invalid log")
		}
		switch l.Level {
		case "ERROR", "WARN", "INFO", "DEBUG", "UNKNOWN":
		default:
			return errors.New("telemetry: invalid level")
		}
	}
	for _, v := range b.Samples {
		if !keyShape.MatchString(v.ID) || v.WorkloadID != s.WorkloadID || v.ObservedAt.IsZero() || v.ObservedAt.Before(b.At.Add(-MetricRetention)) || v.ObservedAt.After(b.At.Add(time.Minute)) || !ValidNumber(v.CPU) || !ValidNumber(v.Memory) || !ValidNumber(v.CPULimit) || !ValidNumber(v.MemoryLimit) || v.ReadyPods < 0 || v.TotalPods < v.ReadyPods || v.Restarts < 0 {
			return errors.New("telemetry: invalid sample")
		}
	}
	for _, e := range b.Events {
		if !keyShape.MatchString(e.ID) || e.WorkloadID != s.WorkloadID || e.Timestamp.IsZero() || e.Timestamp.Before(b.At.Add(-MetricRetention)) || e.Timestamp.After(b.At.Add(time.Minute)) || e.Occurrences < 1 {
			return errors.New("telemetry: invalid event")
		}
		switch e.Kind {
		case "RESTART", "WAITING", "FAILED":
		default:
			return errors.New("telemetry: invalid event kind")
		}
		switch e.Reason {
		case "OOMKilled", "Error", "Completed", "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError", "Failed", "Unknown":
		default:
			return errors.New("telemetry: invalid event reason")
		}
	}
	return nil
}

type Bucket struct {
	At    time.Time `json:"timestamp"`
	Count int64     `json:"count"`
}
type Peak struct {
	WorkloadID string    `json:"workloadId"`
	Metric     string    `json:"metric"`
	Timestamp  time.Time `json:"timestamp"`
	Value      float64   `json:"value"`
	Limit      float64   `json:"limit"`
	Percent    float64   `json:"percent"`
}

type UsagePeak struct {
	WorkloadID string    `json:"workloadId"`
	Metric     string    `json:"metric"`
	Timestamp  time.Time `json:"timestamp"`
	Value      float64   `json:"value"`
}

type Data struct {
	Logs       []Log       `json:"logs"`
	LogCount   int64       `json:"logCount"`
	ErrorCount int64       `json:"errorCount"`
	Histogram  []Bucket    `json:"histogram"`
	NextCursor string      `json:"nextCursor,omitempty"`
	Points     []Sample    `json:"points"`
	Latest     []Sample    `json:"latest"`
	Peaks      []Peak      `json:"peaks"`
	UsagePeaks []UsagePeak `json:"usagePeaks"`
	Events     []Event     `json:"events"`
	EventCount int64       `json:"eventCount"`
	Sources    []State     `json:"-"`
}

func EmptyData() Data {
	return Data{Logs: []Log{}, Histogram: []Bucket{}, Points: []Sample{}, Latest: []Sample{}, Peaks: []Peak{}, UsagePeaks: []UsagePeak{}, Events: []Event{}, Sources: []State{}}
}

func InWindow(stamp time.Time, q Query) bool { return !stamp.Before(q.From) && stamp.Before(q.To) }
func MatchLog(l Log, q Query) bool {
	return (q.Workload == "" || l.WorkloadID == q.Workload) && (q.Level == "" || q.Level == "ALL" || q.Level == l.Level) && strings.Contains(strings.ToLower(l.Message), strings.ToLower(q.Search)) && InWindow(l.Timestamp, q)
}
func Older(l Log, p *Position) bool {
	return p == nil || l.Timestamp.Before(p.Time) || (l.Timestamp.Equal(p.Time) && l.ID < p.ID)
}

// Summarize is also used by the memory adapter. SQL adapters may aggregate at
// the database, but must keep these exact cursor, null and peak semantics.
func Summarize(logs []Log, samples []Sample, events []Event, states []State, q Query) Data {
	if q.Limit <= 0 || q.Limit > MaxPage {
		q.Limit = MaxPage
	}
	out := EmptyData()
	out.Sources = states
	for _, l := range logs {
		if MatchLog(l, q) {
			out.LogCount++
			if l.Level == "ERROR" {
				out.ErrorCount++
			}
			if Older(l, q.Before) {
				out.Logs = append(out.Logs, l)
			}
		}
	}
	sort.Slice(out.Logs, func(i, j int) bool {
		a, b := out.Logs[i], out.Logs[j]
		if a.Timestamp.Equal(b.Timestamp) {
			return a.ID > b.ID
		}
		return a.Timestamp.After(b.Timestamp)
	})
	if len(out.Logs) > q.Limit {
		out.Logs = out.Logs[:q.Limit]
		out.NextCursor = EncodeCursor(out.Logs[len(out.Logs)-1])
	}
	width := q.To.Sub(q.From) / 16
	if width < time.Second {
		width = time.Second
	}
	for i := 0; i < 16; i++ {
		at := q.From.Add(time.Duration(i) * width)
		b := Bucket{At: at}
		for _, l := range logs {
			if l.Level == "ERROR" && MatchLog(l, q) && !l.Timestamp.Before(at) && l.Timestamp.Before(at.Add(width)) {
				b.Count++
			}
		}
		out.Histogram = append(out.Histogram, b)
	}
	latest := map[string]Sample{}
	peaks := map[string]Peak{}
	usagePeaks := map[string]UsagePeak{}
	buckets := map[string][]Sample{}
	for _, sample := range samples {
		if q.Workload != "" && sample.WorkloadID != q.Workload {
			continue
		}
		if old, ok := latest[sample.WorkloadID]; !ok || sample.ObservedAt.After(old.ObservedAt) {
			latest[sample.WorkloadID] = sample
		}
		if !InWindow(sample.ObservedAt, q) {
			continue
		}
		step := q.Step
		if step < Interval {
			step = Interval
		}
		at := BucketTime(sample.ObservedAt, step)
		key := sample.WorkloadID + at.Format(time.RFC3339Nano)
		buckets[key] = append(buckets[key], sample)
		for _, m := range []struct {
			key  string
			v, l *float64
		}{{"cpu", sample.CPU, sample.CPULimit}, {"memory", sample.Memory, sample.MemoryLimit}} {
			usageKey := sample.WorkloadID + "/" + m.key
			if m.v != nil {
				if old, ok := usagePeaks[usageKey]; !ok || *m.v > old.Value {
					usagePeaks[usageKey] = UsagePeak{sample.WorkloadID, m.key, sample.ObservedAt, *m.v}
				}
			}
			if m.v == nil || m.l == nil || *m.l <= 0 {
				continue
			}
			p := Peak{sample.WorkloadID, m.key, sample.ObservedAt, *m.v, *m.l, *m.v / *m.l * 100}
			key := sample.WorkloadID + "/" + m.key
			if old, ok := peaks[key]; !ok || p.Percent > old.Percent {
				peaks[key] = p
			}
		}
	}
	for _, s := range latest {
		out.Latest = append(out.Latest, s)
	}
	for _, p := range peaks {
		out.Peaks = append(out.Peaks, p)
	}
	for _, p := range usagePeaks {
		out.UsagePeaks = append(out.UsagePeaks, p)
	}
	for _, list := range buckets {
		s := list[0]
		s.CPU = nil
		s.Memory = nil
		s.CPULimit = nil
		s.MemoryLimit = nil
		s.ObservedAt = BucketTime(s.ObservedAt, max(q.Step, Interval))
		var cpu, mem float64
		var nc, nm int
		for _, v := range list {
			if v.CPU != nil {
				cpu += *v.CPU
				nc++
			}
			if v.Memory != nil {
				mem += *v.Memory
				nm++
			}
		}
		if nc == len(list) {
			v := cpu / float64(nc)
			s.CPU = &v
		}
		if nm == len(list) {
			v := mem / float64(nm)
			s.Memory = &v
		}
		out.Points = append(out.Points, s)
	}
	for _, e := range events {
		if (q.Workload == "" || q.Workload == e.WorkloadID) && InWindow(e.Timestamp, q) {
			out.EventCount++
			out.Events = append(out.Events, e)
		}
	}
	sort.Slice(out.Events, func(i, j int) bool { return out.Events[i].Timestamp.After(out.Events[j].Timestamp) })
	if len(out.Events) > 200 {
		out.Events = out.Events[:200]
	}
	sort.Slice(out.Points, func(i, j int) bool {
		a, b := out.Points[i], out.Points[j]
		if a.WorkloadID != b.WorkloadID {
			return a.WorkloadID < b.WorkloadID
		}
		return a.ObservedAt.Before(b.ObservedAt)
	})
	sort.Slice(out.Latest, func(i, j int) bool { return out.Latest[i].WorkloadID < out.Latest[j].WorkloadID })
	sort.Slice(out.Peaks, func(i, j int) bool {
		return out.Peaks[i].WorkloadID+out.Peaks[i].Metric < out.Peaks[j].WorkloadID+out.Peaks[j].Metric
	})
	return out
}

// BucketTime shares Unix UTC alignment with SQL and the Console charts.
func BucketTime(at time.Time, step time.Duration) time.Time {
	return time.Unix(0, at.UnixNano()/int64(step)*int64(step)).UTC()
}
