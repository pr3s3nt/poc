package runner_test

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/terraform/runner"
	"orchestrator/internal/adapters/terraform/runner/runnertest"
	"orchestrator/internal/ports/terraformexec"
)

func deadline() time.Time { return time.Now().Add(2 * time.Minute) }

func fixtureModule() runnertest.Module {
	return runnertest.Module{
		Files: map[string]string{"main.tf": `
variable "x" { type = string }
variable "note" { type = string }
variable "stable" { type = string }
variable "secret" {
  type      = string
  default   = "s3cret-value"
  sensitive = true
}
module "c" {
  source = "../common"
  x      = var.x
}
resource "terraform_data" "t" {
  input            = var.note
  triggers_replace = var.stable
}
output "o" { value = module.c.y }
output "n" { value = 7 }
output "flag" { value = true }
output "tags" { value = tomap({ a = "b" }) }
output "pw" {
  value     = var.secret
  sensitive = true
}
`},
		Children: map[string]map[string]string{"common": {"main.tf": "variable \"x\" { type = string }\noutput \"y\" { value = \"${var.x}-y\" }\n"}},
		Calls:    map[string]string{"c": "common"},
	}
}

func open(t *testing.T, r *runner.Runner, state []byte, vars map[string]any) terraformexec.Session {
	t.Helper()
	art := runnertest.Artifact(t, fixtureModule(), runnertest.HostTerraform(t))
	t.Cleanup(func() { art.Close() })
	s, err := r.Open(context.Background(), terraformexec.SessionRequest{Artifact: art, LocalState: state, Variables: vars, Deadline: deadline()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func vars(x, note, stable string) map[string]any {
	return map[string]any{"x": x, "note": note, "stable": stable}
}

// ACTUAL Terraform CLI (built-in terraform_data only; no provider download,
// no cloud or cluster): init -get=false over a pre-installed call tree and a
// readonly lock, create, in-place update, replace detection, outputs with
// runtime types and sensitivity, and state listing.
func TestActualCLICreateUpdateReplaceAndOutputs(t *testing.T) {
	r := runner.New(runner.Options{})
	ctx := context.Background()
	s := open(t, r, []byte{}, vars("1", "a", "k"))
	if err := s.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	plan, err := s.Plan(ctx, terraformexec.PlanApply)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Changes) != 1 || !plan.OnlyCreates() || plan.Destructive() || plan.Changes[0].Address != "terraform_data.t" {
		t.Fatalf("create plan %+v", plan)
	}
	if err := s.Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	outputs, err := s.Outputs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if outputs["o"].Value != "1-y" || outputs["o"].Type != "string" || outputs["n"].Value != float64(7) || outputs["n"].Type != "number" ||
		outputs["flag"].Type != "bool" || outputs["pw"].Value != "s3cret-value" || !outputs["pw"].Sensitive || outputs["o"].Sensitive {
		t.Fatalf("outputs %+v", outputs)
	}
	if kind, ok := outputs["tags"].Type.([]any); !ok || kind[0] != "map" {
		t.Fatalf("map output type %+v", outputs["tags"].Type)
	}
	resources, err := s.StateResources(ctx)
	if err != nil || len(resources) != 1 || resources[0].Address != "terraform_data.t" || resources[0].ID == "" || !strings.Contains(resources[0].Provider, "terraform") {
		t.Fatalf("state resources %+v %v", resources, err)
	}
	state, err := s.PullState(ctx)
	if err != nil || len(state) == 0 {
		t.Fatalf("pull state: %v", err)
	}

	// In-place update: not destructive.
	u := open(t, r, state, vars("1", "b", "k"))
	if err := u.Init(ctx); err != nil {
		t.Fatal(err)
	}
	plan, err = u.Plan(ctx, terraformexec.PlanApply)
	if err != nil || !plan.Mutating() || plan.Destructive() || plan.Changes[0].Actions[0] != "update" {
		t.Fatalf("update plan %+v %v", plan, err)
	}
	// Replacement is destructive.
	rep := open(t, r, state, vars("1", "a", "k2"))
	if err := rep.Init(ctx); err != nil {
		t.Fatal(err)
	}
	plan, err = rep.Plan(ctx, terraformexec.PlanApply)
	if err != nil || !plan.Destructive() || plan.OnlyCreates() {
		t.Fatalf("replace plan %+v %v", plan, err)
	}
	// No change: not mutating.
	same := open(t, r, state, vars("1", "a", "k"))
	if err := same.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if plan, err = same.Plan(ctx, terraformexec.PlanApply); err != nil || plan.Mutating() {
		t.Fatalf("no-op plan %+v %v", plan, err)
	}
	// Destroy plan only deletes.
	d := open(t, r, state, vars("1", "a", "k"))
	if err := d.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if plan, err = d.Plan(ctx, terraformexec.PlanDestroy); err != nil || !plan.OnlyDeletes() || !plan.Destructive() {
		t.Fatalf("destroy plan %+v %v", plan, err)
	}
}

// The workspace is re-verified before every step: a module file changed after
// the artifact was verified stops provider execution.
func TestWorkspaceTamperingStopsBeforeExecution(t *testing.T) {
	r := runner.New(runner.Options{})
	art := runnertest.Artifact(t, fixtureModule(), runnertest.HostTerraform(t))
	t.Cleanup(func() { art.Close() })
	s, err := r.Open(context.Background(), terraformexec.SessionRequest{Artifact: art, LocalState: []byte{}, Variables: vars("1", "a", "k"), Deadline: deadline()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := os.WriteFile(filepath.Join(art.ModuleDir, "extra.tf"), []byte(`output "x" { value = 1 }`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Init(context.Background()); !errors.Is(err, terraformexec.ErrWorkspace) {
		t.Fatalf("tampered module: %v", err)
	}
}

func TestChildModuleAndBinaryTamperingStop(t *testing.T) {
	r := runner.New(runner.Options{})
	art := runnertest.Artifact(t, fixtureModule(), runnertest.HostTerraform(t))
	t.Cleanup(func() { art.Close() })
	s, err := r.Open(context.Background(), terraformexec.SessionRequest{Artifact: art, LocalState: []byte{}, Variables: vars("1", "a", "k"), Deadline: deadline()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	child := filepath.Join(art.Modules[0].Dir, "main.tf")
	if err := os.WriteFile(child, []byte(`output "y" { value = "evil" }`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Init(context.Background()); !errors.Is(err, terraformexec.ErrWorkspace) {
		t.Fatalf("tampered child module: %v", err)
	}
	art2 := runnertest.Artifact(t, fixtureModule(), runnertest.HostTerraform(t))
	t.Cleanup(func() { art2.Close() })
	s2, err := r.Open(context.Background(), terraformexec.SessionRequest{Artifact: art2, LocalState: []byte{}, Variables: vars("1", "a", "k"), Deadline: deadline()})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if err := os.Chmod(art2.TerraformBinary, 0o700); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(art2.TerraformBinary, os.O_APPEND|os.O_WRONLY, 0)
	_, _ = f.WriteString("tamper")
	f.Close()
	if err := s2.Init(context.Background()); !errors.Is(err, terraformexec.ErrWorkspace) {
		t.Fatalf("tampered binary: %v", err)
	}
}

func TestReservedAndAmbientFilesAreRefused(t *testing.T) {
	r := runner.New(runner.Options{})
	for _, name := range []string{"terraform.tfvars", "x.auto.tfvars", "zz_orchestrator_backend.tf"} {
		m := fixtureModule()
		m.Files[name] = "x = 1\n"
		art := runnertest.Artifact(t, m, runnertest.HostTerraform(t))
		_, err := r.Open(context.Background(), terraformexec.SessionRequest{Artifact: art, LocalState: []byte{}, Deadline: deadline()})
		if !errors.Is(err, terraformexec.ErrWorkspace) {
			t.Errorf("%s accepted: %v", name, err)
		}
		art.Close()
	}
}

// FAKE CLI (shell script stand-in; no Terraform): cancellation stops the whole
// process group including a child that outlives the leader, and the outcome is
// reported unknown with the group confirmed stopped.
func TestFakeCLICancellationStopsProcessGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "child.pid")
	cli := runnertest.Script(t, "/bin/sleep 300 &\necho $! > "+marker+"\nwait\n")
	art := runnertest.Artifact(t, fixtureModule(), cli)
	t.Cleanup(func() { art.Close() })
	r := runner.New(runner.Options{Grace: 200 * time.Millisecond, StopConfirm: 5 * time.Second})
	s, err := r.Open(context.Background(), terraformexec.SessionRequest{Artifact: art, LocalState: []byte{}, Variables: vars("1", "a", "k"), Deadline: deadline()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for i := 0; i < 200; i++ {
			if _, err := os.Stat(marker); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()
	err = s.Init(ctx)
	var step *terraformexec.StepError
	if !errors.As(err, &step) || step.Class != terraformexec.ClassCancelled || step.Outcome != terraformexec.OutcomeUnknown || !step.Stopped {
		t.Fatalf("cancelled step: %v", err)
	}
	pid, _ := os.ReadFile(marker)
	if out, _ := exec.Command("sh", "-c", "kill -0 "+strings.TrimSpace(string(pid))+" 2>/dev/null && ps -o stat= -p "+strings.TrimSpace(string(pid))).Output(); strings.TrimSpace(string(out)) != "" && !strings.HasPrefix(strings.TrimSpace(string(out)), "Z") {
		t.Fatalf("child process %s survived cancellation (%s)", pid, out)
	}
	// A failed session never runs another step.
	if err := s.Init(context.Background()); !errors.Is(err, terraformexec.ErrWorkspace) {
		t.Fatalf("session reused after a failed step: %v", err)
	}
}

// FAKE CLI: deadline expiry is a timeout with unknown outcome.
func TestFakeCLIDeadlineIsTimeoutUnknown(t *testing.T) {
	cli := runnertest.Script(t, "/bin/sleep 300\n")
	art := runnertest.Artifact(t, fixtureModule(), cli)
	t.Cleanup(func() { art.Close() })
	r := runner.New(runner.Options{Grace: 100 * time.Millisecond, StopConfirm: 5 * time.Second})
	s, err := r.Open(context.Background(), terraformexec.SessionRequest{Artifact: art, LocalState: []byte{}, Deadline: time.Now().Add(300 * time.Millisecond)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var step *terraformexec.StepError
	if err := s.Init(context.Background()); !errors.As(err, &step) || step.Class != terraformexec.ClassTimeout || step.Outcome != terraformexec.OutcomeUnknown || !step.Stopped {
		t.Fatalf("deadline step: %v", err)
	}
}

// FAKE CLI: a clean non-zero exit is a confirmed outcome classified from
// private output; the error text never carries the CLI output or a secret.
func TestFakeCLIFailureClassesAreSafe(t *testing.T) {
	cases := map[string]terraformexec.Class{
		"Error: Throttling: Rate exceeded SECRET-TOKEN-123":                  terraformexec.ClassTransient,
		"Error: AccessDenied SECRET-TOKEN-123":                               terraformexec.ClassAuth,
		"Error: Missing required argument SECRET-TOKEN-123":                  terraformexec.ClassInput,
		"Error: Error acquiring the state lock SECRET-TOKEN-123":             terraformexec.ClassBackend,
		"Error: something odd SECRET-TOKEN-123":                              terraformexec.ClassFailed,
		"Error: Failed to persist state to backend SECRET-TOKEN-123 timeout": terraformexec.ClassBackend,
	}
	for message, want := range cases {
		cli := runnertest.Script(t, "echo '"+message+"' >&2\nexit 1\n")
		art := runnertest.Artifact(t, fixtureModule(), cli)
		r := runner.New(runner.Options{})
		s, err := r.Open(context.Background(), terraformexec.SessionRequest{Artifact: art, LocalState: []byte{}, Deadline: deadline()})
		if err != nil {
			t.Fatal(err)
		}
		err = s.Init(context.Background())
		var step *terraformexec.StepError
		if !errors.As(err, &step) || step.Class != want || step.Outcome != terraformexec.OutcomeConfirmed || !step.Stopped || strings.Contains(err.Error(), "SECRET") {
			t.Errorf("%q: class %v err %v", message, want, err)
		}
		s.Close()
		art.Close()
	}
}

// The subprocess never inherits the host environment: only the allowlist
// and the explicit provider environment are present.
func TestFakeCLIEnvironmentIsScrubbed(t *testing.T) {
	out := filepath.Join(t.TempDir(), "env.txt")
	cli := runnertest.Script(t, "/usr/bin/env > "+out+"\nexit 1\n")
	art := runnertest.Artifact(t, fixtureModule(), cli)
	t.Cleanup(func() { art.Close() })
	t.Setenv("AWS_ACCESS_KEY_ID", "HOSTKEY")
	t.Setenv("KUBECONFIG", "/host/kubeconfig")
	t.Setenv("HTTPS_PROXY", "http://proxy.example")
	r := runner.New(runner.Options{PassThroughEnv: []string{"HTTPS_PROXY"}})
	s, err := r.Open(context.Background(), terraformexec.SessionRequest{Artifact: art, LocalState: []byte{}, Env: []string{"AWS_ACCESS_KEY_ID=SCOPEDKEY"}, Deadline: deadline()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_ = s.Init(context.Background())
	data, _ := os.ReadFile(out)
	text := string(data)
	if strings.Contains(text, "HOSTKEY") || strings.Contains(text, "/host/kubeconfig") || !strings.Contains(text, "SCOPEDKEY") || !strings.Contains(text, "HTTPS_PROXY=http://proxy.example") {
		t.Fatalf("environment not scrubbed:\n%s", text)
	}
	if !strings.Contains(text, "TF_DATA_DIR=") || !strings.Contains(text, "TF_IN_AUTOMATION=1") {
		t.Fatalf("missing runner variables:\n%s", text)
	}
	var providerTmp string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "TMPDIR=") {
			providerTmp = strings.TrimPrefix(line, "TMPDIR=")
		}
	}
	info, err := os.Stat(providerTmp)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("provider temporary directory is not private: %v", err)
	}
	listener, err := net.Listen("unix", filepath.Join(providerTmp, "provider-handshake.sock"))
	if err != nil {
		t.Fatalf("provider socket cannot bind in temporary directory: %v", err)
	}
	_ = listener.Close()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(providerTmp); !os.IsNotExist(err) {
		t.Fatal("confirmed Close retained provider temporary directory")
	}
}

// FAKE CLI: a process that escaped the group (setsid) keeps the output pipes
// open after the leader exited. The step returns promptly (bounded), reports an
// unknown outcome with the stop unconfirmed, and retains the private workspace.
func TestFakeCLIEscapedPipeHolderIsBoundedAndUnconfirmed(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "escaped.pid")
	cli := runnertest.Script(t, "/usr/bin/setsid /bin/sleep 300 &\necho $! > "+marker+"\nexit 0\n")
	t.Cleanup(func() {
		if pid, err := os.ReadFile(marker); err == nil {
			_ = exec.Command("kill", strings.TrimSpace(string(pid))).Run()
		}
	})
	art := runnertest.Artifact(t, fixtureModule(), cli)
	t.Cleanup(func() { art.Close() })
	r := runner.New(runner.Options{Grace: 100 * time.Millisecond, StopConfirm: 400 * time.Millisecond})
	s, err := r.Open(context.Background(), terraformexec.SessionRequest{Artifact: art, LocalState: []byte{}, Variables: vars("1", "a", "k"), Deadline: deadline()})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	var step *terraformexec.StepError
	err = s.Init(context.Background())
	if !errors.As(err, &step) || step.Outcome != terraformexec.OutcomeUnknown || step.Stopped {
		t.Fatalf("escaped pipe holder: %v", err)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatalf("the step did not return in bounded time: %v", time.Since(started))
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// The workspace is retained: an unconfirmed process may still use it.
	if entries, _ := os.ReadDir(art.Root); len(entries) == 0 {
		t.Fatal("artifact vanished")
	}
	works, _ := filepath.Glob(filepath.Join(art.Root, "work-*"))
	if len(works) == 0 {
		t.Fatal("the private workspace was removed while a process may still hold it")
	}
}
