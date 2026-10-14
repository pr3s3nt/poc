package v2run

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Armed is what a launcher learns about a running supervisor.
type Armed struct {
	PID, PGID, SID int
}

// StartDetached launches `binary args...` as the supervisor in its OWN session
// and process group (setsid), with a minimal environment, and waits until its
// status file reports ARMED. It refuses to return unless the supervisor's
// process group and session differ from the caller's, so a runner or browser
// group kill cannot reach it. Its stdout/stderr are discarded; the supervisor
// writes only its private status and logs.
func StartDetached(binary string, args []string, statusPath string, wait time.Duration) (Armed, error) {
	_ = os.Remove(statusPath)
	cmd := exec.Command(binary, args...)
	cmd.Env = baseEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return Armed{}, fmt.Errorf("start the cleanup supervisor: %w", err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			return Armed{}, errors.New("the cleanup supervisor exited before it was armed")
		default:
		}
		raw, err := os.ReadFile(statusPath)
		if err == nil {
			var st Status
			if json.Unmarshal(raw, &st) == nil && st.State == StateArmed && st.PID == cmd.Process.Pid {
				ownGroup, _ := syscall.Getpgid(0)
				if st.PGID == ownGroup || st.SID == sessionID() || st.PGID != cmd.Process.Pid {
					_ = syscall.Kill(cmd.Process.Pid, syscall.SIGTERM)
					return Armed{}, errors.New("the cleanup supervisor is not isolated in its own session and process group")
				}
				return Armed{PID: st.PID, PGID: st.PGID, SID: st.SID}, nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(cmd.Process.Pid, syscall.SIGTERM)
	return Armed{}, errors.New("the cleanup supervisor was not armed in time")
}

// ReadStatus loads a supervisor status file.
func ReadStatus(path string) (Status, error) {
	var st Status
	raw, err := os.ReadFile(path)
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal(raw, &st)
}

// RequestFinish asks the supervisor to run the verified cleanup now (the
// runner finished or failed cleanly).
func RequestFinish(finishPath string) error {
	return WriteFilePrivate(finishPath, []byte("finish\n"))
}

// WaitTerminal waits for a CLEAN or NEEDS_ATTENTION status.
func WaitTerminal(statusPath string, timeout time.Duration) (Status, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if st, err := ReadStatus(statusPath); err == nil && (st.State == StateClean || st.State == StateAttention) {
			return st, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return Status{}, errors.New("the cleanup supervisor did not reach a terminal state in time")
}
