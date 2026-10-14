package v2run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Layers of the actual reverse dependency cleanup, in the order the
// supervisor runs them: delivery (workload objects) -> app resources
// (Redis/PostgreSQL/ElastiCache/Aurora) -> namespace -> EKS -> VPC -> the
// run-owned auxiliary assets (ECR, IAM, network leftovers).
const (
	LayerDelivery     = "delivery"
	LayerAppResources = "app-resources"
	LayerNamespace    = "namespace"
	LayerEKS          = "eks"
	LayerVPC          = "vpc"
	LayerAux          = "aux"
)

var layerOrder = map[string]int{LayerDelivery: 0, LayerAppResources: 1, LayerNamespace: 2, LayerEKS: 3, LayerVPC: 4, LayerAux: 5}

// Guard exit codes. 0 means the asset is verified run-owned; ExitNotOwned
// means it is verified NOT run-owned (preexisting, retained or updated by
// someone else) and must never be deleted; anything else is unknown ownership.
const ExitNotOwned = 10

// Step outcomes.
const (
	StepDone         = "DONE"
	StepNotOwned     = "SKIPPED_NOT_OWNED"
	StepUnknown      = "UNKNOWN_OWNERSHIP"
	StepFailed       = "FAILED"
	StepNotVerified  = "NOT_VERIFIED_ABSENT"
	StepBlocked      = "BLOCKED_BY_EARLIER_LAYER"
	StepBudgetExpiry = "CLEANUP_BUDGET_EXHAUSTED"
)

// Supervisor states.
const (
	StateArmed       = "ARMED"
	StateRunning     = "CLEANING"
	StateClean       = "CLEAN"
	StateAttention   = "NEEDS_ATTENTION"
	ExitCleanCode    = 0
	ExitAttention    = 3
	ExitBudgetCode   = 4
	ExitBadSpecCode  = 2
	maxStepAttempts  = 3
	defaultPoll      = 500 * time.Millisecond
	maxExecutionTime = 180 * time.Minute
	maxCleanupBudget = 120 * time.Minute
)

// Duration is a JSON duration string such as "90s".
type Duration struct{ time.Duration }

// MarshalJSON writes the duration as a string.
func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// UnmarshalJSON parses a duration string.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a string like \"90s\"")
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// Watch names a process whose death triggers cleanup. StartTicks pins the
// process identity so a reused PID is never mistaken for the runner.
type Watch struct {
	Name       string `json:"name"`
	PID        int    `json:"pid"`
	StartTicks uint64 `json:"startTicks"`
}

// Step is one cleanup action in the actual reverse graph.
type Step struct {
	Name  string `json:"name"`
	Layer string `json:"layer"`
	// Argv is executed directly (no shell). Env values are explicit and
	// non-secret (handle file paths); the supervisor never inherits the
	// runner's environment beyond PATH, HOME, TMPDIR and proxy settings.
	Argv []string          `json:"argv"`
	Env  map[string]string `json:"env,omitempty"`
	// Guard proves ownership before the effect (see ExitNotOwned).
	Guard []string `json:"guard,omitempty"`
	// Verify proves absence after the effect: exit 0 only when verified.
	Verify      []string `json:"verify,omitempty"`
	Timeout     Duration `json:"timeout"`
	MaxAttempts int      `json:"maxAttempts,omitempty"`
}

// Spec is the supervisor input (private, 0600).
type Spec struct {
	RunID      string `json:"runId"`
	StatusPath string `json:"statusPath"`
	// FinishPath is created by the runner when it finished (or failed
	// cleanly): the supervisor then runs the same verified cleanup.
	FinishPath string `json:"finishPath"`
	LogDir     string `json:"logDir"`
	// ExpiresAt is the finite execution expiry: reaching it triggers cleanup.
	ExpiresAt     time.Time `json:"expiresAt"`
	CleanupBudget Duration  `json:"cleanupBudget"`
	Poll          Duration  `json:"poll,omitempty"`
	Watch         []Watch   `json:"watch"`
	Steps         []Step    `json:"steps"`
}

// StepResult is the safe (no output) result of one step.
type StepResult struct {
	Name     string `json:"name"`
	Layer    string `json:"layer"`
	Outcome  string `json:"outcome"`
	Attempts int    `json:"attempts"`
}

// Status is the progressively written supervisor status (public-safe).
type Status struct {
	RunID      string       `json:"runId"`
	State      string       `json:"state"`
	Reason     string       `json:"reason,omitempty"`
	PID        int          `json:"pid"`
	PGID       int          `json:"pgid"`
	SID        int          `json:"sid"`
	StartedAt  time.Time    `json:"startedAt"`
	FinishedAt *time.Time   `json:"finishedAt,omitempty"`
	Steps      []StepResult `json:"steps"`
	// Remaining lists the steps that did not reach a verified-absent or
	// verified-not-owned outcome: the operator continues from these.
	Remaining []string `json:"remaining,omitempty"`
	// KeepEvidence is true whenever state/recovery evidence must be kept.
	KeepEvidence bool `json:"keepEvidence"`
}

// Validate checks the spec before anything is armed.
func (s Spec) Validate(now time.Time) error {
	switch {
	case s.RunID == "":
		return errors.New("supervisor: runId is required")
	case s.StatusPath == "" || s.FinishPath == "" || s.LogDir == "":
		return errors.New("supervisor: statusPath, finishPath and logDir are required")
	case s.ExpiresAt.IsZero() || !s.ExpiresAt.After(now):
		return errors.New("supervisor: expiresAt must be a finite future time")
	case s.ExpiresAt.After(now.Add(maxExecutionTime + time.Minute)):
		return fmt.Errorf("supervisor: expiresAt is beyond the %s execution limit", maxExecutionTime)
	case s.CleanupBudget.Duration <= 0 || s.CleanupBudget.Duration > maxCleanupBudget:
		return fmt.Errorf("supervisor: cleanupBudget must be positive and at most %s", maxCleanupBudget)
	case len(s.Watch) == 0:
		return errors.New("supervisor: at least the runner must be watched")
	case len(s.Steps) == 0:
		return errors.New("supervisor: no cleanup steps")
	}
	for _, p := range []string{s.StatusPath, s.FinishPath, s.LogDir} {
		if !filepath.IsAbs(p) {
			return errors.New("supervisor: paths must be absolute")
		}
	}
	last := -1
	for _, step := range s.Steps {
		rank, ok := layerOrder[step.Layer]
		if !ok {
			return fmt.Errorf("supervisor: step %q has unknown layer %q", step.Name, step.Layer)
		}
		if rank < last {
			return fmt.Errorf("supervisor: step %q breaks the reverse dependency order (delivery -> app resources -> namespace -> EKS -> VPC -> aux)", step.Name)
		}
		last = rank
		if step.Name == "" || len(step.Argv) == 0 || len(step.Verify) == 0 || len(step.Guard) == 0 {
			return fmt.Errorf("supervisor: step %q needs a name, argv, an ownership guard and an absence verification", step.Name)
		}
		if step.Timeout.Duration <= 0 {
			return fmt.Errorf("supervisor: step %q needs a finite timeout", step.Name)
		}
		if step.MaxAttempts > maxStepAttempts {
			return fmt.Errorf("supervisor: step %q exceeds %d bounded attempts", step.Name, maxStepAttempts)
		}
	}
	return nil
}

// LoadSpec reads a spec file; it must be a private regular file.
func LoadSpec(path string) (Spec, error) {
	var s Spec
	if err := requirePrivateFile(path); err != nil {
		return s, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return s, fmt.Errorf("supervisor: invalid spec: %w", err)
	}
	return s, nil
}

func requirePrivateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s must be a regular file with mode 0600", filepath.Base(path))
	}
	return nil
}

// ProcessIdentity returns the start ticks of pid, or an error when it does not exist.
func ProcessIdentity(pid int) (uint64, error) {
	_, ticks, err := procStat(pid)
	return ticks, err
}

func procStat(pid int) (state byte, ticks uint64, err error) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, 0, err
	}
	text := string(raw)
	end := strings.LastIndexByte(text, ')')
	if end < 0 || end+2 >= len(text) {
		return 0, 0, errors.New("unparsable /proc stat")
	}
	fields := strings.Fields(text[end+2:])
	if len(fields) < 20 {
		return 0, 0, errors.New("short /proc stat")
	}
	ticks, err = strconv.ParseUint(fields[19], 10, 64)
	return fields[0][0], ticks, err
}

// Alive reports whether the watched process still exists with the same identity
// and is not a zombie.
func (w Watch) Alive() bool {
	state, ticks, err := procStat(w.PID)
	if err != nil || state == 'Z' || state == 'X' {
		return false
	}
	return w.StartTicks == 0 || ticks == w.StartTicks
}

// Supervisor runs the verified cleanup when its trigger fires.
type Supervisor struct {
	Spec Spec
	// Now is injectable for tests.
	Now func() time.Time
	// Signals receives the termination signals; nil uses the process signals.
	Signals <-chan os.Signal
	Log     io.Writer
}

func (s *Supervisor) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Run arms the supervisor, waits for a trigger and runs the cleanup. It
// returns the process exit code.
func (s *Supervisor) Run(ctx context.Context) int {
	if err := s.Spec.Validate(s.now()); err != nil {
		fmt.Fprintln(s.logw(), err)
		return ExitBadSpecCode
	}
	if err := os.MkdirAll(s.Spec.LogDir, 0o700); err != nil {
		fmt.Fprintln(s.logw(), "supervisor: cannot create the private log directory")
		return ExitBadSpecCode
	}
	signals := s.Signals
	if signals == nil {
		ch := make(chan os.Signal, 4)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGQUIT)
		defer signal.Stop(ch)
		signals = ch
	}
	status := Status{RunID: s.Spec.RunID, State: StateArmed, PID: os.Getpid(), StartedAt: s.now().UTC(), Steps: []StepResult{}}
	status.PGID, _ = syscall.Getpgid(0)
	status.SID = sessionID()
	if err := writeJSONPrivate(s.Spec.StatusPath, status); err != nil {
		fmt.Fprintln(s.logw(), "supervisor: cannot write the status file")
		return ExitBadSpecCode
	}
	reason := s.waitTrigger(ctx, signals)
	status.State, status.Reason = StateRunning, reason
	_ = writeJSONPrivate(s.Spec.StatusPath, status)
	return s.cleanup(&status, signals)
}

func (s *Supervisor) logw() io.Writer {
	if s.Log != nil {
		return s.Log
	}
	return io.Discard
}

func sessionID() int {
	raw, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0
	}
	text := string(raw)
	end := strings.LastIndexByte(text, ')')
	fields := strings.Fields(text[end+2:])
	if len(fields) < 4 {
		return 0
	}
	sid, _ := strconv.Atoi(fields[3])
	return sid
}

// waitTrigger blocks until one of: the finish file, a termination signal, a
// watched process death or the expiry.
func (s *Supervisor) waitTrigger(ctx context.Context, signals <-chan os.Signal) string {
	poll := s.Spec.Poll.Duration
	if poll <= 0 {
		poll = defaultPoll
	}
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		select {
		case sig := <-signals:
			return "signal:" + sig.String()
		case <-ctx.Done():
			return "context-cancelled"
		case <-tick.C:
		}
		if _, err := os.Lstat(s.Spec.FinishPath); err == nil {
			return "runner-finished"
		}
		for _, w := range s.Spec.Watch {
			if !w.Alive() {
				return "watched-process-exited:" + w.Name
			}
		}
		if !s.now().Before(s.Spec.ExpiresAt) {
			return "expiry"
		}
	}
}

func baseEnv() []string {
	env := []string{}
	for _, name := range []string{"PATH", "HOME", "TMPDIR", "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "https_proxy", "http_proxy", "no_proxy", "SSL_CERT_FILE", "LANG", "LC_ALL"} {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return env
}

func (s *Supervisor) exec(ctx context.Context, step Step, argv []string, logName string) (int, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = baseEnv()
	for k, v := range step.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	logFile, err := os.OpenFile(filepath.Join(s.Spec.LogDir, logName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return -1, err
	}
	defer logFile.Close()
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return -1, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(step.Timeout.Duration)
	defer timer.Stop()
	select {
	case err := <-done:
		return exitCode(err), nil
	case <-timer.C:
	case <-ctx.Done():
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	<-done
	return -1, errors.New("timed out")
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// cleanup runs the steps in reverse dependency order. A failed or
// unverifiable layer blocks every later dependent layer: a namespace is never
// deleted to skip the lifecycle of children that could not be removed.
func (s *Supervisor) cleanup(status *Status, signals <-chan os.Signal) int {
	budget := s.Spec.CleanupBudget.Duration
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	// Further termination signals do not abort a running cleanup.
	go func() {
		for {
			select {
			case <-signals:
			case <-ctx.Done():
				return
			}
		}
	}()
	blockedAt := -1
	exit := ExitCleanCode
	for _, step := range s.Spec.Steps {
		rank := layerOrder[step.Layer]
		result := StepResult{Name: step.Name, Layer: step.Layer}
		switch {
		case ctx.Err() != nil:
			result.Outcome, exit = StepBudgetExpiry, ExitBudgetCode
		case blockedAt >= 0 && rank > blockedAt:
			result.Outcome = StepBlocked
		default:
			result.Outcome, result.Attempts = s.runStep(ctx, step)
			if ctx.Err() != nil && result.Outcome != StepDone && result.Outcome != StepNotOwned {
				result.Outcome, exit = StepBudgetExpiry, ExitBudgetCode
			}
		}
		if result.Outcome != StepDone && result.Outcome != StepNotOwned {
			if blockedAt < 0 || rank < blockedAt {
				blockedAt = rank
			}
			status.Remaining = append(status.Remaining, step.Name)
			if exit == ExitCleanCode {
				exit = ExitAttention
			}
		}
		status.Steps = append(status.Steps, result)
		_ = writeJSONPrivate(s.Spec.StatusPath, *status)
	}
	finished := s.now().UTC()
	status.FinishedAt = &finished
	if len(status.Remaining) == 0 {
		status.State = StateClean
	} else {
		status.State, status.KeepEvidence = StateAttention, true
	}
	_ = writeJSONPrivate(s.Spec.StatusPath, *status)
	return exit
}

func (s *Supervisor) runStep(ctx context.Context, step Step) (string, int) {
	code, err := s.exec(ctx, step, step.Guard, step.Name+".guard.log")
	switch {
	case err == nil && code == ExitNotOwned:
		return StepNotOwned, 0
	case err != nil || code != 0:
		return StepUnknown, 0 // never act on unknown ownership
	}
	attempts := step.MaxAttempts
	if attempts <= 0 {
		attempts = 2
	}
	var last string
	for i := 1; i <= attempts; i++ {
		if ctx.Err() != nil {
			return last, i - 1
		}
		code, err = s.exec(ctx, step, step.Argv, step.Name+".log")
		if err != nil || code != 0 {
			last = StepFailed
			// The effect may have partly happened: verification decides.
		}
		vcode, verr := s.exec(ctx, step, step.Verify, step.Name+".verify.log")
		if verr == nil && vcode == 0 {
			return StepDone, i
		}
		if last == "" {
			last = StepNotVerified
		}
	}
	return last, attempts
}
