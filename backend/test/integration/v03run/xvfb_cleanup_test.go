package v03run

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCleanupUsesActualPrivateXvfbLaunchRecord(t *testing.T) {
	if _, err := exec.LookPath("Xvfb"); err != nil {
		t.Skip("Xvfb is not installed")
	}
	f := newCleanupFixture(t)
	for _, name := range []string{"baseline.json", "product-started", "pg-manifest.json"} {
		_ = os.Remove(filepath.Join(f.private, name))
	}
	launcher := `source ../video-lib.sh; WORK="$1"; PRIVATE="$2"; PIDS=(); video_start_xvfb 800x600 "$PRIVATE/aux-xvfb.pid"; printf '%s\n%s\n' "$VIDEO_DISPLAY" "${PIDS[-1]}"`
	out, err := exec.Command("bash", "-c", launcher, "x", f.work, f.private).CombinedOutput()
	if err != nil {
		t.Fatalf("actual Xvfb launch failed: %v: %s", err, out)
	}
	lines := strings.Fields(string(out))
	if len(lines) < 2 {
		t.Fatalf("missing display/process output: %q", out)
	}
	display := lines[len(lines)-2]
	pidText := lines[len(lines)-1]
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.private, "aux-xvfb.pid")); err != nil {
		t.Fatal("actual launch did not write its cleanup record", err)
	}
	foreign := exec.Command("sleep", "300")
	foreign.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := foreign.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(-foreign.Process.Pid, syscall.SIGKILL); _ = foreign.Wait() }()
	if err := runCleanup(t, f, ""); err != nil {
		t.Fatalf("cleanup refused actual Xvfb lifecycle: %v", err)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("Xvfb process %d remains", pid)
	}
	displayNumber := strings.TrimPrefix(display, ":")
	for _, path := range []string{"/tmp/.X" + displayNumber + "-lock", "/tmp/.X11-unix/X" + displayNumber} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("X display artifact remains: %s", path)
		}
	}
	if err := syscall.Kill(os.Getpid(), 0); err != nil {
		t.Fatal("cleanup signalled the test runner", err)
	}
	if err := syscall.Kill(foreign.Process.Pid, 0); err != nil {
		t.Fatalf("cleanup affected a foreign process group: %v", err)
	}
}

func TestSharedXvfbLauncherWorksForCallerWithoutPrivateDirectory(t *testing.T) {
	if _, err := exec.LookPath("Xvfb"); err != nil {
		t.Skip("Xvfb is not installed")
	}
	work := t.TempDir()
	launcher := `source ../video-lib.sh; WORK="$1"; PIDS=(); unset PRIVATE; video_start_xvfb 800x600; printf '%s\n%s\n' "$VIDEO_DISPLAY" "${PIDS[-1]}"`
	out, err := exec.Command("bash", "-c", launcher, "x", work).CombinedOutput()
	if err != nil {
		t.Fatalf("shared no-PRIVATE caller failed: %v: %s", err, out)
	}
	lines := strings.Fields(string(out))
	if len(lines) < 2 {
		t.Fatalf("missing display/process output: %q", out)
	}
	display := lines[len(lines)-2]
	pidText := lines[len(lines)-1]
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(pid, syscall.SIGTERM) }()
	if _, err := os.Stat(filepath.Join(work, "aux-xvfb.pid")); err == nil {
		t.Fatal("no-PRIVATE caller unexpectedly wrote an ownership receipt")
	}
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("Xvfb was not left running for caller-owned cleanup: %v", err)
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	displayNumber := strings.TrimPrefix(display, ":")
	for i := 0; i < 30; i++ {
		_, lockErr := os.Stat("/tmp/.X" + displayNumber + "-lock")
		_, socketErr := os.Stat("/tmp/.X11-unix/X" + displayNumber)
		if os.IsNotExist(lockErr) && os.IsNotExist(socketErr) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no-PRIVATE caller's exact display %s did not stop", display)
}
