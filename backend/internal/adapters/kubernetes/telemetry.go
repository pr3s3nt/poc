package kubernetes

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/ports/execution"
	obs "orchestrator/internal/ports/observability"
)

var ErrTelemetryOutputLimit = errors.New("kubernetes: telemetry response exceeds its limit")

type cappedOutput struct {
	buffer   bytes.Buffer
	max      int
	exceeded bool
	cancel   context.CancelFunc
}

func (w *cappedOutput) Write(b []byte) (int, error) {
	if len(b) > w.max-w.buffer.Len() {
		w.exceeded = true
		w.cancel()
		return 0, ErrTelemetryOutputLimit
	}
	return w.buffer.Write(b)
}

// RunBounded is used only by telemetry reads. It caps both output streams,
// uses the same explicit credential boundary, and never reports remote text.
func (c CLI) RunBounded(ctx context.Context, limit int, args ...string) ([]byte, error) {
	if c.Kubeconfig == "" {
		return nil, ErrHostKubeconfig
	}
	if info, err := os.Stat(c.Kubeconfig); err != nil || info.IsDir() {
		return nil, ErrHostKubeconfig
	}
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(run, c.Path, append(c.baseArgs(), args...)...)
	out := &cappedOutput{max: limit, cancel: cancel}
	stderr := &cappedOutput{max: 4096, cancel: cancel}
	cmd.Stdout = out
	cmd.Stderr = stderr
	err := cmd.Run()
	if out.exceeded || stderr.exceeded {
		return nil, ErrTelemetryOutputLimit
	}
	if err != nil {
		return nil, &CommandError{Operation: []string{"telemetry-read"}, Category: classify(stderr.buffer.String()), Process: err}
	}
	return out.buffer.Bytes(), nil
}

type TelemetryReader struct {
	KubectlPath string
	Credentials execution.KubeconfigSource
	Now         func() time.Time
}

type telemetryMeta struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	UID       string            `json:"uid"`
	Creation  time.Time         `json:"creationTimestamp"`
	Labels    map[string]string `json:"labels"`
	Owners    []jobOwner        `json:"ownerReferences"`
}
type termination struct {
	Reason   string    `json:"reason"`
	Finished time.Time `json:"finishedAt"`
}
type containerState struct {
	Waiting *struct {
		Reason string `json:"reason"`
	} `json:"waiting"`
	Terminated *termination `json:"terminated"`
}
type containerStatus struct {
	Name     string         `json:"name"`
	Ready    bool           `json:"ready"`
	Restarts int64          `json:"restartCount"`
	State    containerState `json:"state"`
	Last     containerState `json:"lastState"`
}
type telemetryPod struct {
	Metadata telemetryMeta `json:"metadata"`
	Spec     struct {
		Containers []struct {
			Name      string `json:"name"`
			Resources struct {
				Limits map[string]string `json:"limits"`
			} `json:"resources"`
		} `json:"containers"`
	} `json:"spec"`
	Status struct {
		Phase             string            `json:"phase"`
		ContainerStatuses []containerStatus `json:"containerStatuses"`
	} `json:"status"`
}
type metricPod struct {
	Metadata   telemetryMeta `json:"metadata"`
	Timestamp  time.Time     `json:"timestamp"`
	Window     string        `json:"window"`
	Containers []struct {
		Name  string            `json:"name"`
		Usage map[string]string `json:"usage"`
	} `json:"containers"`
}

func telemetryCode(err error) telemetry.Code {
	switch {
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrUnauthorized), errors.Is(err, ErrCredentialTarget):
		return telemetry.Forbidden
	case errors.Is(err, ErrObjectNotFound):
		return telemetry.Unavailable
	case errors.Is(err, ErrTelemetryOutputLimit):
		return telemetry.Truncated
	default:
		return telemetry.Failed
	}
}
func mergeCode(old, next telemetry.Code) telemetry.Code {
	if old == telemetry.OK || old == telemetry.Waiting {
		return next
	}
	return old
}
func reasonCode(reason string) string {
	switch reason {
	case "OOMKilled", "Error", "Completed", "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError", "Failed":
		return reason
	}
	return "Unknown"
}

// quantity decodes the resource quantities emitted by pods/Metrics Server.
// big.Rat avoids floating-point parsing surprises and overflow before conversion.
var telemetryPodName = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)

var quantityShape = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?|\.[0-9]+)([a-zA-Z]*)$`)

func quantity(raw string, cpu bool) (float64, bool) {
	m := quantityShape.FindStringSubmatch(raw)
	if m == nil {
		return 0, false
	}
	v, ok := new(big.Rat).SetString(m[1])
	if !ok {
		return 0, false
	}
	multipliers := map[string]*big.Rat{"": big.NewRat(1, 1)}
	if cpu {
		multipliers = map[string]*big.Rat{"": big.NewRat(1000, 1), "m": big.NewRat(1, 1), "u": big.NewRat(1, 1000), "n": big.NewRat(1, 1000000)}
	} else {
		for i, suffix := range []string{"Ki", "Mi", "Gi", "Ti", "Pi", "Ei"} {
			multipliers[suffix] = new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), uint(10*(i+1))))
		}
		for i, suffix := range []string{"k", "M", "G", "T", "P", "E"} {
			multipliers[suffix] = new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(1000), big.NewInt(int64(i+1)), nil))
		}
	}
	mul, ok := multipliers[m[2]]
	if !ok {
		return 0, false
	}
	n, _ := new(big.Rat).Mul(v, mul).Float64()
	return n, n >= 0 && n < 1e20
}

func (r *TelemetryReader) Collect(ctx context.Context, source obs.Source, prior telemetry.State) (telemetry.Batch, error) {
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	b := telemetry.Batch{At: now, State: prior, Logs: []telemetry.Log{}, Samples: []telemetry.Sample{}, Events: []telemetry.Event{}}
	b.State.ObservedAt = &now
	b.State.LogsCode = telemetry.OK
	b.State.MetricsCode = telemetry.OK
	b.State.Checkpoints = map[string]telemetry.Cursor{}
	// This adapter never accepts explicit host paths or an unscoped target.
	if !source.Source.Valid() || !source.Target.CredentialBacked() || source.Target.Organization != source.OrganizationKey || !dnsLabel.MatchString(source.Target.Namespace) {
		return b, ErrCredentialTarget
	}
	cli, cleanup, err := OpenCLI(ctx, r.KubectlPath, r.Credentials, source.Target)
	if err != nil {
		b.State.LogsCode, b.State.MetricsCode = telemetryCode(err), telemetryCode(err)
		return b, nil
	}
	defer cleanup()
	selector := "app.kubernetes.io/managed-by=orchestrator,app.kubernetes.io/name=" + source.WorkloadID + ",orchestrator.io/application=" + source.ApplicationKey + ",orchestrator.io/environment=" + source.EnvironmentKey
	query := url.Values{"labelSelector": {selector}, "limit": {"32"}}
	path := "/api/v1/namespaces/" + source.Target.Namespace + "/pods?" + query.Encode()
	payload, err := cli.RunBounded(ctx, 2<<20, "get", "--raw", path, "--request-timeout=15s")
	if err != nil {
		b.State.LogsCode, b.State.MetricsCode = telemetryCode(err), telemetryCode(err)
		return b, nil
	}
	var list struct {
		Items    []telemetryPod `json:"items"`
		Metadata struct {
			Continue string `json:"continue"`
		} `json:"metadata"`
	}
	if json.Unmarshal(payload, &list) != nil {
		return b, errors.New("kubernetes: malformed pod telemetry")
	}
	if len(list.Items) > 8 {
		list.Items = list.Items[:8]
		b.State.LogsCode = telemetry.Truncated
		b.State.MetricsCode = telemetry.Truncated
	}
	if list.Metadata.Continue != "" {
		b.State.LogsCode = telemetry.Truncated
		b.State.MetricsCode = telemetry.Truncated
	}
	jobUIDs := map[string]bool{}
	if source.Controller != nil {
		history := r.InspectJobs(ctx, source, *source.Controller)
		if history.Code != "OK" || history.Truncated {
			b.State.LogsCode, b.State.MetricsCode = telemetry.SafeCode(telemetry.Code(history.Code)), telemetry.Waiting
			if history.Truncated {
				b.State.LogsCode = telemetry.Truncated
			}
			return b, nil
		}
		for _, job := range history.Jobs {
			jobUIDs[job.UID] = true
		}
	}
	pods := []telemetryPod{}
	for _, p := range list.Items {
		labels := p.Metadata.Labels
		if p.Metadata.Namespace != source.Target.Namespace || p.Metadata.UID == "" || !telemetryPodName.MatchString(p.Metadata.Name) || labels["app.kubernetes.io/name"] != source.WorkloadID || labels["app.kubernetes.io/managed-by"] != "orchestrator" || labels["orchestrator.io/application"] != source.ApplicationKey || labels["orchestrator.io/environment"] != source.EnvironmentKey {
			continue
		}
		if source.Controller != nil {
			owned := false
			for _, owner := range p.Metadata.Owners {
				owned = owned || owner.Controller && owner.Kind == "Job" && jobUIDs[owner.UID]
			}
			if !owned {
				continue
			}
		}
		pods = append(pods, p)
	}
	if len(pods) == 0 {
		b.State.LogsCode = telemetry.Waiting
		b.State.MetricsCode = telemetry.Waiting
		return b, nil
	}
	metricMap := map[string]metricPod{}
	metrics, metricsErr := cli.RunBounded(ctx, 2<<20, "get", "--raw", "/apis/metrics.k8s.io/v1beta1/namespaces/"+source.Target.Namespace+"/pods?"+query.Encode(), "--request-timeout=15s")
	if metricsErr != nil {
		b.State.MetricsCode = mergeCode(b.State.MetricsCode, telemetryCode(metricsErr))
	} else {
		var data struct {
			Items []metricPod `json:"items"`
		}
		if json.Unmarshal(metrics, &data) != nil {
			b.State.MetricsCode = telemetry.Failed
		} else {
			for _, p := range data.Items {
				if p.Metadata.Namespace == source.Target.Namespace {
					metricMap[p.Metadata.Name] = p
				}
			}
		}
	}
	sample := telemetry.Sample{SourceKey: source.Key, WorkloadID: source.WorkloadID, ObservedAt: now.Truncate(telemetry.Interval), TotalPods: int64(len(pods))}
	var cpu, mem, cpuLimit, memLimit float64
	complete := true
	completeCPU, completeMemory := true, true
	var metricsAt time.Time
	for _, p := range pods {
		statuses := map[string]containerStatus{}
		podReady := len(p.Spec.Containers) > 0
		for _, status := range p.Status.ContainerStatuses {
			statuses[status.Name] = status
			sample.Restarts += status.Restarts
			if !status.Ready {
				podReady = false
			}
		}
		if len(p.Status.ContainerStatuses) != len(p.Spec.Containers) {
			podReady = false
		}
		if podReady && p.Status.Phase == "Running" {
			sample.ReadyPods++
		}
		mp, hasMetrics := metricMap[p.Metadata.Name]
		if !hasMetrics || mp.Timestamp.IsZero() || mp.Timestamp.Before(now.Add(-2*time.Minute)) || mp.Timestamp.After(now.Add(time.Minute)) || mp.Timestamp.Before(p.Metadata.Creation) || (mp.Metadata.UID != "" && mp.Metadata.UID != p.Metadata.UID) {
			complete = false
			b.State.MetricsCode = mergeCode(b.State.MetricsCode, telemetry.Stale)
			hasMetrics = false
		}
		if hasMetrics && (metricsAt.IsZero() || mp.Timestamp.Before(metricsAt)) {
			metricsAt = mp.Timestamp
		}
		containers := map[string]map[string]string{}
		for _, c := range mp.Containers {
			containers[c.Name] = c.Usage
		}
		if len(p.Spec.Containers) > 8 {
			b.State.LogsCode = mergeCode(b.State.LogsCode, telemetry.Truncated)
			complete = false
		}
		for index, c := range p.Spec.Containers {
			if index >= 8 {
				break
			}
			if !dnsLabel.MatchString(c.Name) {
				complete = false
				continue
			}
			cv, cok := quantity(containers[c.Name]["cpu"], true)
			mv, mok := quantity(containers[c.Name]["memory"], false)
			if !hasMetrics || !cok || !mok {
				complete = false
			} else {
				cpu += cv
				mem += mv
			}
			cl, cok := quantity(c.Resources.Limits["cpu"], true)
			ml, mok := quantity(c.Resources.Limits["memory"], false)
			if !cok || cl <= 0 {
				completeCPU = false
			} else {
				cpuLimit += cl
			}
			if !mok || ml <= 0 {
				completeMemory = false
			} else {
				memLimit += ml
			}
			status, exists := statuses[c.Name]
			if !exists {
				b.State.LogsCode = mergeCode(b.State.LogsCode, telemetry.Waiting)
				continue
			}
			if status.Restarts > 0 && status.Last.Terminated != nil {
				term := status.Last.Terminated
				stamp := term.Finished
				if stamp.IsZero() {
					stamp = now
				}
				if !stamp.Before(now.Add(-telemetry.MetricRetention)) && stamp.Before(now.Add(time.Minute)) {
					b.Events = append(b.Events, telemetry.Event{ID: telemetry.Hash(source.Key, p.Metadata.UID, c.Name, "restart", strconv.FormatInt(status.Restarts, 10)), WorkloadID: source.WorkloadID, Timestamp: stamp, PodUID: p.Metadata.UID, PodName: p.Metadata.Name, Container: c.Name, Kind: "RESTART", Reason: reasonCode(term.Reason), Occurrences: 1})
				}
			}
			if term := status.State.Terminated; term != nil && term.Reason != "Completed" && !term.Finished.IsZero() && !term.Finished.Before(now.Add(-telemetry.MetricRetention)) && term.Finished.Before(now.Add(time.Minute)) {
				b.Events = append(b.Events, telemetry.Event{ID: telemetry.Hash(source.Key, p.Metadata.UID, c.Name, "failed", strconv.FormatInt(status.Restarts, 10)), WorkloadID: source.WorkloadID, Timestamp: term.Finished, PodUID: p.Metadata.UID, PodName: p.Metadata.Name, Container: c.Name, Kind: "FAILED", Reason: reasonCode(term.Reason), Occurrences: 1})
			}
			if status.State.Waiting != nil {
				reason := reasonCode(status.State.Waiting.Reason)
				if reason != "Unknown" {
					b.Events = append(b.Events, telemetry.Event{ID: telemetry.Hash(source.Key, p.Metadata.UID, c.Name, reason, strconv.FormatInt(status.Restarts, 10)), WorkloadID: source.WorkloadID, Timestamp: now, PodUID: p.Metadata.UID, PodName: p.Metadata.Name, Container: c.Name, Kind: "WAITING", Reason: reason, Occurrences: 1})
				}
			}
			for _, previous := range []bool{false, true} {
				restart := status.Restarts
				if previous {
					if restart == 0 {
						continue
					}
					restart--
				}
				stream := telemetry.Hash(p.Metadata.UID, c.Name, strconv.FormatInt(restart, 10))
				checkpoint := prior.Checkpoints[stream]
				if previous && checkpoint.Previous {
					b.State.Checkpoints[stream] = checkpoint
					continue
				}
				if len(b.Logs) >= 8000 {
					b.State.LogsCode = mergeCode(b.State.LogsCode, telemetry.Truncated)
					continue
				}
				since := checkpoint.Timestamp.Add(-time.Second)
				if checkpoint.Timestamp.IsZero() || since.Before(now.Add(-telemetry.LogRetention)) {
					since = now.Add(-telemetry.LogRetention)
				}
				args := []string{"logs", p.Metadata.Name, "-n", source.Target.Namespace, "-c", c.Name, "--timestamps=true", "--limit-bytes=262144", "--tail=2000", "--since-time=" + since.Format(time.RFC3339Nano), "--request-timeout=15s"}
				if previous {
					args = append(args, "--previous=true")
				}
				data, logErr := cli.RunBounded(ctx, 256<<10, args...)
				if logErr != nil {
					b.State.Checkpoints[stream] = checkpoint
					b.State.LogsCode = mergeCode(b.State.LogsCode, telemetryCode(logErr))
					continue
				}
				rows, last, truncated := parseTelemetryLogs(source, p, c.Name, restart, stream, data, now)
				if len(b.Logs)+len(rows) > 8000 {
					rows = rows[:8000-len(b.Logs)]
					last = time.Time{}
					for _, row := range rows {
						if row.Timestamp.After(last) {
							last = row.Timestamp
						}
					}
					truncated = true
				}
				b.Logs = append(b.Logs, rows...)
				if truncated {
					b.State.LogsCode = mergeCode(b.State.LogsCode, telemetry.Truncated)
				}
				if last.After(checkpoint.Timestamp) {
					checkpoint.Timestamp = last
				}
				checkpoint.Previous = previous && !truncated
				b.State.Checkpoints[stream] = checkpoint
			}
		}
	}
	if complete && b.State.MetricsCode == telemetry.OK && !metricsAt.IsZero() {
		sample.CPU = &cpu
		sample.Memory = &mem
		sample.MetricsAt = &metricsAt
		sample.ObservedAt = metricsAt
		if completeCPU {
			sample.CPULimit = &cpuLimit
		}
		if completeMemory {
			sample.MemoryLimit = &memLimit
		}
		b.State.LastMetricAt = &metricsAt
	} else if b.State.MetricsCode == telemetry.OK {
		b.State.MetricsCode = telemetry.Unavailable
	}
	sample.ID = telemetry.Hash(source.Key, "sample", sample.ObservedAt.Format(time.RFC3339Nano))
	b.Samples = append(b.Samples, sample)
	for _, row := range b.Logs {
		if b.State.LastLogAt == nil || row.Timestamp.After(*b.State.LastLogAt) {
			stamp := row.Timestamp
			b.State.LastLogAt = &stamp
		}
	}
	if prior.ObservedAt != nil && prior.ObservedAt.Before(now.Add(-2*telemetry.Interval)) && b.State.LogsCode == telemetry.OK {
		b.State.LogsCode = telemetry.Gap
	}
	return b, nil
}

func parseTelemetryLogs(source obs.Source, pod telemetryPod, container string, restart int64, stream string, data []byte, now time.Time) ([]telemetry.Log, time.Time, bool) {
	out := []telemetry.Log{}
	last := time.Time{}
	truncated := len(data) >= 256<<10
	// A byte-limited response may end in a partial record. Never persist it as
	// another version of the same line when the next read obtains it in full.
	if truncated && len(data) > 0 && data[len(data)-1] != '\n' {
		if end := bytes.LastIndexByte(data, '\n'); end >= 0 {
			data = data[:end+1]
		} else {
			data = nil
		}
	}
	ordinals := map[string]int{}
	scan := bufio.NewScanner(bytes.NewReader(data))
	scan.Buffer(make([]byte, 4096), 256<<10)
	for scan.Scan() {
		raw := scan.Text()
		stampText, message, ok := strings.Cut(raw, " ")
		if !ok {
			truncated = true
			continue
		}
		stamp, err := time.Parse(time.RFC3339Nano, stampText)
		if err != nil || stamp.Before(now.Add(-telemetry.LogRetention)) || stamp.After(now.Add(time.Minute)) {
			truncated = true
			continue
		}
		identityMessage := message
		message = strings.ReplaceAll(strings.ToValidUTF8(message, "�"), "\x00", "\\0")
		if len(message) > telemetry.MaxMessage {
			message = message[:telemetry.MaxMessage-len(" … [truncated]")]
			for !utf8.ValidString(message) {
				message = message[:len(message)-1]
			}
			message += " … [truncated]"
			truncated = true
		}
		ordinalKey := stampText + "\x00" + identityMessage
		ordinals[ordinalKey]++
		id := telemetry.Hash(source.Key, stream, stampText, identityMessage, strconv.Itoa(ordinals[ordinalKey]))
		out = append(out, telemetry.Log{ID: id, WorkloadID: source.WorkloadID, Timestamp: stamp, PodUID: pod.Metadata.UID, PodName: pod.Metadata.Name, Container: container, RestartIndex: restart, Level: telemetry.Level(message), Message: message})
		if stamp.After(last) {
			last = stamp
		}
	}
	if len(out) >= 2000 {
		truncated = true
	}
	if scan.Err() != nil {
		truncated = true
	}
	return out, last, truncated
}

var _ obs.Reader = (*TelemetryReader)(nil)
