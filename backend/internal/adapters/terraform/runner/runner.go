// Package runner is the Terraform CLI process adapter of the v2 executor. It
// drives one private invocation: the verified private binary and module trees
// of the registry artifact, a scrubbed environment that never inherits the
// host, a pre-installed (never downloaded) module call tree, a readonly
// provider lock and the internal HTTP state backend, each CLI step bounded by
// a deadline in its own process group.
package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/terraformexec"
)

// Generated files the runner places in the module directory; the content
// check of the pinned module ignores exactly these.
const (
	lockFileName    = ".terraform.lock.hcl"
	backendFileName = "zz_orchestrator_backend.tf"
	backendConfig   = "terraform {\n  backend \"http\" {}\n}\n"
)

// Options configure every session of one Runner.
type Options struct {
	// PluginCacheDir is an optional platform-owned provider cache. Providers
	// still must match the pinned lock hashes.
	PluginCacheDir string
	// CLIConfigFile is an optional platform-owned Terraform CLI configuration
	// (for example a provider filesystem mirror). It is never taken from a
	// module or from the host environment.
	CLIConfigFile string
	// PassThroughEnv names host variables forwarded verbatim (for example
	// HTTPS_PROXY, NO_PROXY). Everything else is dropped.
	PassThroughEnv []string
	// Grace is how long a cancelled step may shut down before SIGKILL.
	Grace time.Duration
	// StopConfirm bounds the wait for the process group to disappear.
	StopConfirm time.Duration
}

// Runner implements terraformexec.Runner with the real Terraform CLI.
type Runner struct {
	opts Options
}

// New returns a Runner.
func New(opts Options) *Runner {
	if opts.Grace <= 0 {
		opts.Grace = 30 * time.Second
	}
	if opts.StopConfirm <= 0 {
		opts.StopConfirm = 15 * time.Second
	}
	return &Runner{opts: opts}
}

var _ terraformexec.Runner = (*Runner)(nil)

var sessionSeq struct {
	sync.Mutex
	n int
}

// Open prepares a private workspace and returns its session.
func (r *Runner) Open(_ context.Context, req terraformexec.SessionRequest) (terraformexec.Session, error) {
	a := req.Artifact
	if a == nil || a.Root == "" || a.ModuleDir == "" || a.TerraformBinary == "" || len(a.LockFile) == 0 || req.Deadline.IsZero() {
		return nil, terraformexec.ErrWorkspace
	}
	if (req.Backend == nil) == (req.LocalState == nil) {
		return nil, terraformexec.ErrWorkspace
	}
	if req.Backend != nil {
		if u, err := url.Parse(req.Backend.Address); err != nil || u.Scheme == "" || req.Backend.Token == "" {
			return nil, terraformexec.ErrWorkspace
		}
	}
	if err := refuseAmbientConfig(a.ModuleDir); err != nil {
		return nil, err
	}
	sessionSeq.Lock()
	sessionSeq.n++
	n := sessionSeq.n
	sessionSeq.Unlock()
	work := filepath.Join(a.Root, "work-"+strconv.Itoa(n))
	for _, dir := range []string{work, filepath.Join(work, "home"), filepath.Join(work, "tmp"), filepath.Join(work, "data", "modules")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, terraformexec.ErrWorkspace
		}
	}
	// Provider plugins create Unix sockets beneath TMPDIR. Evidence/cache paths
	// can exceed the socket path limit, so use a short invocation-owned directory.
	providerTmp, err := os.MkdirTemp("/tmp", "orch-tf-")
	if err != nil {
		_ = os.RemoveAll(work)
		return nil, terraformexec.ErrWorkspace
	}
	s := &session{runner: r, req: req, art: a, work: work, providerTmp: providerTmp}
	if err := s.prepare(); err != nil {
		_ = os.RemoveAll(providerTmp)
		_ = os.RemoveAll(work)
		return nil, err
	}
	if err := writeFile(filepath.Join(work, "provider-tmp-dir"), []byte(providerTmp+"\n"), 0o600); err != nil {
		_ = os.RemoveAll(providerTmp)
		_ = os.RemoveAll(work)
		return nil, terraformexec.ErrWorkspace
	}
	// Artifact retention clears Cleanup when a process may still use the files.
	// The private pointer above lets recovery locate the retained socket directory.
	cleanup := a.Cleanup
	a.Cleanup = func() error {
		tmpErr := os.RemoveAll(providerTmp)
		if cleanup != nil {
			return errors.Join(tmpErr, cleanup())
		}
		return tmpErr
	}
	return s, nil
}

// refuseAmbientConfig rejects module content Terraform would load on its own
// beside the pinned configuration: variable files and local Terraform data.
func refuseAmbientConfig(moduleDir string) error {
	entries, err := os.ReadDir(moduleDir)
	if err != nil {
		return terraformexec.ErrWorkspace
	}
	for _, e := range entries {
		name := e.Name()
		switch {
		case name == "terraform.tfvars" || name == "terraform.tfvars.json",
			strings.HasSuffix(name, ".auto.tfvars"), strings.HasSuffix(name, ".auto.tfvars.json"),
			strings.HasPrefix(name, ".terraform"), strings.HasPrefix(name, "zz_orchestrator_"):
			return fmt.Errorf("%w: the module directory contains a reserved file", terraformexec.ErrWorkspace)
		}
	}
	return nil
}

type session struct {
	runner      *Runner
	req         terraformexec.SessionRequest
	art         *tfmodule.VerifiedArtifact
	work        string
	providerTmp string
	failed      bool
	closed      bool
	// retain keeps the private workspace when a process could not be
	// confirmed stopped: it may still read the files, and they may be the only
	// copy of state and credentials an operator needs.
	retain bool
}

func (s *session) varsPath() string  { return filepath.Join(s.work, "terraform.tfvars.json") }
func (s *session) planPath() string  { return filepath.Join(s.work, "tfplan") }
func (s *session) localPath() string { return filepath.Join(s.work, "local.tfstate") }

// prepare writes every generated file: provider lock, backend declaration,
// module call manifest, variables and state input.
func (s *session) prepare() error {
	a := s.art
	if err := writeFile(filepath.Join(a.ModuleDir, lockFileName), a.LockFile, 0o400); err != nil {
		return terraformexec.ErrWorkspace
	}
	if s.req.Backend != nil {
		if err := writeFile(filepath.Join(a.ModuleDir, backendFileName), []byte(backendConfig), 0o400); err != nil {
			return terraformexec.ErrWorkspace
		}
	} else if err := writeFile(s.localPath(), s.req.LocalState, 0o600); err != nil {
		return terraformexec.ErrWorkspace
	}
	manifest, err := s.manifest()
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(s.work, "data", "modules", "modules.json"), manifest, 0o600); err != nil {
		return terraformexec.ErrWorkspace
	}
	vars := s.req.Variables
	if vars == nil {
		vars = map[string]any{}
	}
	encoded, err := json.Marshal(vars)
	if err != nil {
		return terraformexec.ErrWorkspace
	}
	if err := writeFile(s.varsPath(), encoded, 0o600); err != nil {
		return terraformexec.ErrWorkspace
	}
	config := filepath.Join(s.work, "empty.tfrc")
	if s.runner.opts.CLIConfigFile == "" {
		if err := writeFile(config, nil, 0o600); err != nil {
			return terraformexec.ErrWorkspace
		}
	}
	return nil
}

func writeFile(path string, content []byte, mode os.FileMode) error {
	return os.WriteFile(path, content, mode)
}

// manifest renders .terraform/modules/modules.json for the verified call
// tree so `init -get=false` installs nothing and downloads nothing.
func (s *session) manifest() ([]byte, error) {
	type record struct {
		Key    string `json:"Key"`
		Source string `json:"Source"`
		Dir    string `json:"Dir"`
	}
	records := []record{{Key: "", Source: "", Dir: "."}}
	for _, m := range s.art.Modules {
		rel, err := filepath.Rel(s.art.ModuleDir, m.Dir)
		if err != nil {
			return nil, terraformexec.ErrWorkspace
		}
		records = append(records, record{Key: m.Key, Source: m.Source, Dir: filepath.ToSlash(rel)})
	}
	return json.Marshal(map[string]any{"Modules": records})
}

// verify re-checks the final workspace content immediately before a step that
// executes the CLI or a provider: the private binary digest, the root module
// tree (ignoring only the generated files), every transitive module tree, and
// the exact bytes of the generated lock and backend files.
func (s *session) verify() error {
	a := s.art
	sum, err := fileDigest(a.TerraformBinary)
	if err != nil || sum != a.Bundle.TerraformBinarySHA256 {
		return fmt.Errorf("%w: the Terraform binary changed", terraformexec.ErrWorkspace)
	}
	root, err := registry.DigestDir(a.ModuleDir, lockFileName, backendFileName)
	if err != nil || root != a.Bundle.Ref.ContentSHA256 {
		return fmt.Errorf("%w: the module content changed", terraformexec.ErrWorkspace)
	}
	for _, call := range a.Bundle.Contract.ModuleCalls {
		if call.Transitive < 0 || call.Transitive >= len(a.Bundle.Contract.Transitive) {
			return terraformexec.ErrWorkspace
		}
	}
	if len(a.Modules) != len(a.Bundle.Contract.ModuleCalls) {
		return terraformexec.ErrWorkspace
	}
	for i, m := range a.Modules {
		want := a.Bundle.Contract.Transitive[a.Bundle.Contract.ModuleCalls[i].Transitive].ContentSHA256
		got, err := registry.DigestDir(m.Dir)
		if err != nil || got != want {
			return fmt.Errorf("%w: a called module changed", terraformexec.ErrWorkspace)
		}
	}
	if got, err := os.ReadFile(filepath.Join(a.ModuleDir, lockFileName)); err != nil || !bytes.Equal(got, a.LockFile) {
		return fmt.Errorf("%w: the provider lock changed", terraformexec.ErrWorkspace)
	}
	if s.req.Backend != nil {
		if got, err := os.ReadFile(filepath.Join(a.ModuleDir, backendFileName)); err != nil || string(got) != backendConfig {
			return fmt.Errorf("%w: the backend declaration changed", terraformexec.ErrWorkspace)
		}
	}
	return nil
}

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// environment builds the complete subprocess environment from scratch.
func (s *session) environment() []string {
	env := []string{
		"PATH=" + filepath.Join(s.work, "nopath"),
		"HOME=" + filepath.Join(s.work, "home"),
		"TMPDIR=" + s.providerTmp,
		"TF_DATA_DIR=" + filepath.Join(s.work, "data"),
		"TF_IN_AUTOMATION=1",
		"TF_INPUT=0",
		"CHECKPOINT_DISABLE=1",
	}
	if s.runner.opts.CLIConfigFile != "" {
		env = append(env, "TF_CLI_CONFIG_FILE="+s.runner.opts.CLIConfigFile)
	} else {
		env = append(env, "TF_CLI_CONFIG_FILE="+filepath.Join(s.work, "empty.tfrc"))
	}
	if s.runner.opts.PluginCacheDir != "" {
		env = append(env, "TF_PLUGIN_CACHE_DIR="+s.runner.opts.PluginCacheDir)
	}
	for _, name := range s.runner.opts.PassThroughEnv {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	if b := s.req.Backend; b != nil {
		env = append(env,
			"TF_HTTP_ADDRESS="+b.Address, "TF_HTTP_LOCK_ADDRESS="+b.Address, "TF_HTTP_UNLOCK_ADDRESS="+b.Address,
			"TF_HTTP_USERNAME=executor", "TF_HTTP_PASSWORD="+b.Token, "TF_HTTP_RETRY_MAX=1")
	}
	return append(env, s.req.Env...)
}

// step runs one CLI command after re-verifying the workspace.
func (s *session) step(ctx context.Context, name string, maxOut int, args ...string) ([]byte, error) {
	if s.closed || s.failed {
		return nil, fmt.Errorf("%w: the session cannot run another step", terraformexec.ErrWorkspace)
	}
	if err := s.verify(); err != nil {
		s.failed = true
		return nil, err
	}
	ctx, cancel := context.WithDeadline(ctx, s.req.Deadline)
	defer cancel()
	cmd := exec.Command(s.art.TerraformBinary, args...)
	cmd.Dir = s.art.ModuleDir
	cmd.Env = s.environment()
	res, err := runProcess(ctx, cmd, maxOut, s.runner.opts.Grace, s.runner.opts.StopConfirm)
	if err == nil {
		if !res.stopped {
			s.failed, s.retain = true, true
			return nil, &terraformexec.StepError{Step: name, Class: terraformexec.ClassFailed, Outcome: terraformexec.OutcomeUnknown}
		}
		if res.truncated {
			s.failed = true
			return nil, &terraformexec.StepError{Step: name, Class: terraformexec.ClassFailed, Outcome: terraformexec.OutcomeUnknown, Stopped: true}
		}
		return res.stdout, nil
	}
	s.failed = true
	if !res.stopped {
		s.retain = true
	}
	step := &terraformexec.StepError{Step: name, Stopped: res.stopped}
	switch {
	case res.timedOut:
		step.Class, step.Outcome = terraformexec.ClassTimeout, terraformexec.OutcomeUnknown
	case res.cancelled:
		step.Class, step.Outcome = terraformexec.ClassCancelled, terraformexec.OutcomeUnknown
	case res.exitedBySignal || !res.stopped:
		step.Class, step.Outcome = terraformexec.ClassFailed, terraformexec.OutcomeUnknown
	case res.exitCode == 0 && res.stdout == nil && res.stderr == nil:
		// The process never started.
		step.Class, step.Outcome = terraformexec.ClassFailed, terraformexec.OutcomeConfirmed
	default:
		step.Class, step.Outcome = classify(string(res.stderr)), terraformexec.OutcomeConfirmed
	}
	return nil, step
}

func (s *session) local() []string {
	if s.req.Backend != nil {
		return nil
	}
	return []string{"-state=" + s.localPath()}
}

func (s *session) Init(ctx context.Context) error {
	args := []string{"init", "-input=false", "-no-color", "-get=false", "-lockfile=readonly"}
	if s.req.Backend == nil {
		args = append(args, "-backend=false")
	}
	_, err := s.step(ctx, "init", 1<<20, args...)
	return err
}

func (s *session) Plan(ctx context.Context, mode terraformexec.PlanMode) (terraformexec.Plan, error) {
	args := []string{"plan", "-input=false", "-no-color", "-var-file=" + s.varsPath(), "-out=" + s.planPath(), "-lock-timeout=0s"}
	switch mode {
	case terraformexec.PlanDestroy:
		args = append(args, "-destroy")
	case terraformexec.PlanRefreshOnly:
		args = append(args, "-refresh-only")
	}
	args = append(args, s.local()...)
	if _, err := s.step(ctx, "plan", 1<<20, args...); err != nil {
		return terraformexec.Plan{}, err
	}
	out, err := s.step(ctx, "show-plan", 64<<20, "show", "-json", "-no-color", s.planPath())
	if err != nil {
		return terraformexec.Plan{}, err
	}
	plan, perr := parsePlan(out)
	if perr != nil {
		s.failed = true
		return terraformexec.Plan{}, &terraformexec.StepError{Step: "show-plan", Class: terraformexec.ClassFailed, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
	}
	return plan, nil
}

func (s *session) Apply(ctx context.Context) error {
	if _, err := os.Stat(s.planPath()); err != nil {
		return fmt.Errorf("%w: there is no saved plan to apply", terraformexec.ErrWorkspace)
	}
	args := []string{"apply", "-input=false", "-no-color", "-auto-approve", "-lock-timeout=0s"}
	args = append(args, s.local()...)
	args = append(args, s.planPath())
	_, err := s.step(ctx, "apply", 1<<20, args...)
	return err
}

func (s *session) Outputs(ctx context.Context) (map[string]terraformexec.Output, error) {
	args := []string{"output", "-json", "-no-color"}
	args = append(args, s.local()...)
	out, err := s.step(ctx, "output", 16<<20, args...)
	if err != nil {
		return nil, err
	}
	outputs, perr := parseOutputs(out)
	if perr != nil {
		s.failed = true
		return nil, &terraformexec.StepError{Step: "output", Class: terraformexec.ClassFailed, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
	}
	return outputs, nil
}

func (s *session) PullState(ctx context.Context) ([]byte, error) {
	if s.req.Backend == nil {
		return os.ReadFile(s.localPath())
	}
	return s.step(ctx, "state-pull", 32<<20, "state", "pull")
}

func (s *session) StateResources(ctx context.Context) ([]terraformexec.StateResource, error) {
	args := []string{"show", "-json", "-no-color"}
	if s.req.Backend == nil {
		args = append(args, s.localPath())
	}
	out, err := s.step(ctx, "show-state", 64<<20, args...)
	if err != nil {
		return nil, err
	}
	resources, perr := parseStateResources(out)
	if perr != nil {
		s.failed = true
		return nil, &terraformexec.StepError{Step: "show-state", Class: terraformexec.ClassFailed, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
	}
	return resources, nil
}

// RecoveryState returns the state Terraform kept in errored.tfstate.
func (s *session) RecoveryState() ([]byte, bool) {
	data, err := os.ReadFile(filepath.Join(s.art.ModuleDir, "errored.tfstate"))
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

// Close removes the session's private files. The artifact itself (and so any
// errored.tfstate in the module directory) is owned and closed by the caller.
func (s *session) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	if s.retain {
		return nil
	}
	return errors.Join(os.RemoveAll(s.providerTmp), os.RemoveAll(s.work))
}

var _ terraformexec.Session = (*session)(nil)
