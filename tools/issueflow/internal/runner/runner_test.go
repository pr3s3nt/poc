package runner

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTimeoutKillsHungCommandAndItsChildren(t *testing.T) {
	r := Timeout{R: OS{}, ByName: map[string]time.Duration{"bash": 300 * time.Millisecond}}
	start := time.Now()
	// the child sleeps and inherits stdout; a naive Wait would block for 30s
	res, err := r.Run(context.Background(), "", nil, "bash", "-c", "sleep 30 & sleep 30; echo done")
	if err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("timeout must be an error, not an exit status: res=%+v err=%v", res, err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("took %v", time.Since(start))
	}
}

func TestCancelStopsCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := OS{}.Run(ctx, "", nil, "sleep", "30")
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("cancel ignored: %v after %v", err, time.Since(start))
	}
}

func TestPlainExitStatusIsNotAnError(t *testing.T) {
	res, err := OS{}.Run(context.Background(), "", nil, "bash", "-c", "echo out; echo err >&2; exit 3")
	if err != nil || res.Exit != 3 || strings.TrimSpace(res.Stdout) != "out" || strings.TrimSpace(res.Stderr) != "err" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestEnvIsAppendedAndOverridesEmpty(t *testing.T) {
	os.Setenv("ISSUEFLOW_X", "outer")
	defer os.Unsetenv("ISSUEFLOW_X")
	res, _ := OS{}.Run(context.Background(), "", []string{"ISSUEFLOW_X="}, "bash", "-c", `echo "[$ISSUEFLOW_X]"`)
	if strings.TrimSpace(res.Stdout) != "[]" {
		t.Fatalf("opt-in env not cleared: %q", res.Stdout)
	}
}

func TestRouterHonoursContext(t *testing.T) {
	r := NewRouter(nil)
	block := make(chan struct{})
	defer close(block)
	r.Handle("hang", func(c Call) (Result, error, bool) { <-block; return Result{}, nil, true })
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := r.Run(ctx, "", nil, "hang"); err == nil {
		t.Fatal("hung handler not interrupted")
	}
}
