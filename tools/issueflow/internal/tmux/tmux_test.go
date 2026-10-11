package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"issueflow/internal/runner"
)

func rt(res runner.Result, err error) (*Real, *runner.Router) {
	r := runner.NewRouter(nil)
	r.Handle("tmux", func(c runner.Call) (runner.Result, error, bool) { return res, err, true })
	return &Real{R: r, Tmp: os.TempDir()}, r
}

func TestHasSessionOnlyDefiniteAnswersAreAbsence(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		res     runner.Result
		err     error
		present bool
		wantErr bool
	}{
		{"present", runner.Result{Exit: 0}, nil, true, false},
		{"no such session", runner.Result{Exit: 1, Stderr: "can't find session: x"}, nil, false, false},
		{"no server", runner.Result{Exit: 1, Stderr: "no server running on /tmp/tmux-1000/default"}, nil, false, false},
		{"socket missing", runner.Result{Exit: 1, Stderr: "error connecting to /tmp/tmux-1000/default (No such file or directory)"}, nil, false, false},
		{"permission denied", runner.Result{Exit: 1, Stderr: "error connecting to /tmp/tmux-1000/default (Permission denied)"}, nil, false, true},
		{"unknown failure", runner.Result{Exit: 1, Stderr: "protocol version mismatch"}, nil, false, true},
		{"runner error", runner.Result{}, fmt.Errorf("exec failed"), false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, _ := rt(c.res, c.err)
			ok, err := r.HasSession(ctx, "s")
			if ok != c.present || (err != nil) != c.wantErr {
				t.Fatalf("ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestKillTreatsOnlyNotFoundAsSuccess(t *testing.T) {
	r, _ := rt(runner.Result{Exit: 1, Stderr: "can't find session: s"}, nil)
	if err := r.Kill(context.Background(), "s"); err != nil {
		t.Fatal(err)
	}
	r, _ = rt(runner.Result{Exit: 1, Stderr: "error connecting to x (Permission denied)"}, nil)
	if err := r.Kill(context.Background(), "s"); err == nil {
		t.Fatal("permission error reported as a successful kill")
	}
}

func TestNewSessionBuildsExactArgsAndPasteIsBracketedWithoutEnter(t *testing.T) {
	r, calls := rt(runner.Result{}, nil)
	r.Socket = "sock"
	ctx := context.Background()
	if err := r.NewSession(ctx, "n", "/w", map[string]string{"B": "2", "A": "1"}, "bash -i"); err != nil {
		t.Fatal(err)
	}
	if got := calls.Calls()[0].String(); got != "tmux -L sock new-session -d -s n -c /w -e A=1 -e B=2 bash -i" {
		t.Fatalf("args: %s", got)
	}
	if err := r.Paste(ctx, "n", "hello\nworld"); err != nil {
		t.Fatal(err)
	}
	var sawPaste, sawEnter bool
	for _, c := range calls.Calls() {
		s := c.String()
		if strings.Contains(s, "paste-buffer -p") {
			sawPaste = true
		}
		if strings.Contains(s, "send-keys") {
			sawEnter = true
		}
	}
	if !sawPaste || sawEnter {
		t.Fatalf("paste must be bracketed and must not press Enter: %v", calls.Calls())
	}
	if err := r.Enter(ctx, "n"); err != nil {
		t.Fatal(err)
	}
}

func TestPanePIDParse(t *testing.T) {
	r, _ := rt(runner.Result{Stdout: "4242\n"}, nil)
	if pid, err := r.PanePID(context.Background(), "s"); err != nil || pid != 4242 {
		t.Fatalf("%d %v", pid, err)
	}
	r, _ = rt(runner.Result{Stdout: "oops"}, nil)
	if _, err := r.PanePID(context.Background(), "s"); err == nil {
		t.Fatal("garbage pane pid accepted")
	}
}

// A real tmux server on an isolated socket runs a scripted fake interactive
// program (no model): it prints a banner, shows a trust dialog and then a
// prompt, and echoes whatever line it receives. This proves pipe-pane logging,
// bracketed paste, the separate Enter, owner recording and a proven kill.
func TestRealTmuxSmokeWithFakeInteractiveProgram(t *testing.T) {
	requireTmux(t)
	dir := t.TempDir()
	sock := fmt.Sprintf("issueflow-test-%d", os.Getpid())
	rn := runner.OS{}
	tm := &Real{R: rn, Tmp: dir, Socket: sock}
	defer exec.Command("tmux", "-L", sock, "kill-server").Run()
	script := filepath.Join(dir, "fake-agent.sh")
	os.WriteFile(script, []byte(`#!/usr/bin/env bash
echo "Welcome to fake-agent"
sleep 0.3
echo "Do you trust the files in this folder?"
echo "  1. Yes, proceed"
echo "  2. No, exit"
read -r answer
if [ "$answer" != "1" ]; then echo "refused"; exit 3; fi
sleep 0.3
echo "? for shortcuts"
read -r line
echo "GOT:$line"
read -r more
echo "GOT2:$more"
sleep 30
`), 0o755)
	ctx := context.Background()
	name := "t-smoke"
	if err := tm.NewSession(ctx, name, dir, map[string]string{"ISSUEFLOW_ROLE": "coder"}, "bash -i"); err != nil {
		t.Fatal(err)
	}
	if err := tm.NewSession(ctx, name, dir, nil, "bash -i"); err == nil {
		t.Fatal("duplicate session name must fail, not be reused")
	}
	logp := filepath.Join(dir, "pane.log")
	if err := tm.PipeLog(ctx, name, logp); err != nil {
		t.Fatal(err)
	}
	if pid, err := tm.PanePID(ctx, name); err != nil || pid <= 1 {
		t.Fatalf("pane pid %d %v", pid, err)
	}
	if err := tm.SendLine(ctx, name, script); err != nil {
		t.Fatal(err)
	}
	waitFor := func(sub string) string {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			b, _ := os.ReadFile(logp)
			if strings.Contains(string(b), sub) {
				return string(b)
			}
			time.Sleep(50 * time.Millisecond)
		}
		b, _ := os.ReadFile(logp)
		t.Fatalf("never saw %q in:\n%s", sub, b)
		return ""
	}
	waitFor("Do you trust")
	// the banner alone must not look ready; answer the dialog explicitly
	if err := tm.SendLine(ctx, name, "1"); err != nil {
		t.Fatal(err)
	}
	waitFor("? for shortcuts")
	if err := tm.Paste(ctx, name, "Read '/tmp/x y/brief.md' now"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if b, _ := os.ReadFile(logp); strings.Contains(string(b), "GOT:") {
		t.Fatal("paste must not submit by itself")
	}
	if err := tm.Enter(ctx, name); err != nil {
		t.Fatal(err)
	}
	waitFor("GOT:Read '/tmp/x y/brief.md' now")
	if ok, err := tm.HasSession(ctx, name); !ok || err != nil {
		t.Fatalf("session should be alive: %v %v", ok, err)
	}
	if err := tm.Kill(ctx, name); err != nil {
		t.Fatal(err)
	}
	if ok, err := tm.HasSession(ctx, name); ok || err != nil {
		t.Fatalf("session should be definitely absent: ok=%v err=%v", ok, err)
	}
}

// The unnumbered trust screen (selected "No, exit" first, "Yes" without a
// number) is answered only with an explicitly configured key sequence, and a
// numbered screen with the number next to Yes. Neither format uses a bare Enter.
func TestRealTmuxKeysSelectExplicitChoice(t *testing.T) {
	requireTmux(t)
	dir := t.TempDir()
	sock := fmt.Sprintf("issueflow-keys-%d", os.Getpid())
	tm := &Real{R: runner.OS{}, Tmp: dir, Socket: sock}
	defer exec.Command("tmux", "-L", sock, "kill-server").Run()
	script := filepath.Join(dir, "unnumbered.sh")
	os.WriteFile(script, []byte(`#!/usr/bin/env bash
echo "> No, exit"
echo "  Yes, I trust this folder"
read -r choice
if [ "$choice" = "y" ]; then echo "TRUSTED"; else echo "DEFAULT-NO:[$choice]"; fi
sleep 20
`), 0o755)
	ctx := context.Background()
	logp := filepath.Join(dir, "p.log")
	if err := tm.NewSession(ctx, "k1", dir, nil, "bash -i"); err != nil {
		t.Fatal(err)
	}
	tm.PipeLog(ctx, "k1", logp)
	tm.SendLine(ctx, "k1", script)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b, _ := os.ReadFile(logp); strings.Contains(string(b), "Yes, I trust this folder") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := tm.Keys(ctx, "k1", "y", "Enter"); err != nil {
		t.Fatal(err)
	}
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(logp)
		if strings.Contains(string(b), "TRUSTED") {
			return
		}
		if strings.Contains(string(b), "DEFAULT-NO") {
			t.Fatal("selection fell through to the default")
		}
		time.Sleep(50 * time.Millisecond)
	}
	b, _ := os.ReadFile(logp)
	t.Fatalf("explicit keys not delivered:\n%s", b)
}

// requireTmux separates "this environment cannot run a tmux server" (a sandbox
// restriction, reported as an explicit SKIP) from a real defect. CI sets
// ISSUEFLOW_REQUIRE_TMUX=1 so that a missing or unusable tmux FAILS there.
func requireTmux(t *testing.T) {
	t.Helper()
	must := os.Getenv("ISSUEFLOW_REQUIRE_TMUX") == "1"
	stop := func(msg string) {
		if must {
			t.Fatalf("tmux is required (ISSUEFLOW_REQUIRE_TMUX=1) but unusable: %s", msg)
		}
		t.Skipf("SANDBOX/ENVIRONMENT: real tmux cannot run here (%s); the fake-tmux tests still run, this one did not", msg)
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		stop("tmux not installed")
	}
	sock := fmt.Sprintf("issueflow-probe-%d", os.Getpid())
	defer exec.Command("tmux", "-L", sock, "kill-server").Run()
	out, err := exec.Command("tmux", "-L", sock, "new-session", "-d", "-s", "probe", "sleep 5").CombinedOutput()
	if err != nil {
		stop(strings.TrimSpace(string(out)))
	}
}
