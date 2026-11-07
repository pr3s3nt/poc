package gitpublication

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGitReadsRetryTransientFailuresButNeverReplayPush(t *testing.T) {
	for _, tc := range []struct {
		name, mode, verb string
		calls            string
		success          bool
	}{
		{"third-read-succeeds", "transient", "fetch", "3", true},
		{"read-exhausted", "exhausted", "fetch", "3", false},
		{"push-outcome-unknown", "exhausted", "push", "1", false},
		{"authentication-permanent", "auth", "fetch", "1", false},
		{"certificate-permanent", "tls", "fetch", "1", false},
		{"checkout-not-read-only", "exhausted", "checkout", "1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			counter := filepath.Join(dir, "calls")
			os.WriteFile(counter, []byte("0\n"), 0600)
			script := `#!/bin/sh
n=$(cat "$FAKE_GIT_COUNT")
n=$((n+1))
printf '%s\n' "$n" > "$FAKE_GIT_COUNT"
if [ "$FAKE_GIT_MODE" = tls ]; then
 printf "unable to access https://private.example/token-MUST-NOT-LOG: SSL certificate problem\n" >&2
 exit 1
fi
if [ "$FAKE_GIT_MODE" = auth ]; then
 printf 'Authentication failed for https://private.example/token-MUST-NOT-LOG\n' >&2
 exit 1
fi
if [ "$FAKE_GIT_MODE" = transient ] && [ "$n" -eq 3 ]; then
 printf 'read-result\n'
 exit 0
fi
printf 'RPC failed; HTTP/2 stream was not closed for token-MUST-NOT-LOG\n' >&2
exit 1
`
			os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0700)
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			writer := Writer{Dir: dir, Env: []string{"FAKE_GIT_COUNT=" + counter, "FAKE_GIT_MODE=" + tc.mode}}
			var diagnostic bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&diagnostic)
			defer log.SetOutput(previous)
			result, err := writer.git(context.Background(), tc.verb, "origin", "main")
			if tc.success {
				if err != nil || string(result) != "read-result\n" {
					t.Fatalf("read failed: %q %v", result, err)
				}
			} else if !errors.Is(err, ErrPublication) {
				t.Fatalf("unconfirmed command succeeded: %v", err)
			}
			actual, _ := os.ReadFile(counter)
			if strings.TrimSpace(string(actual)) != tc.calls {
				t.Fatalf("command invocations=%s expected=%s", actual, tc.calls)
			}
			if strings.Contains(diagnostic.String(), "token-MUST-NOT-LOG") || strings.Contains(diagnostic.String(), "private.example") {
				t.Fatal("private Git stderr leaked into diagnostics")
			}
		})
	}
}

func TestCancelledGitReadDoesNotStartAnotherProcess(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "calls")
	script := `#!/bin/sh
printf '1\n' > "$FAKE_GIT_COUNT"
printf 'RPC failed; HTTP/2 stream closed\n' >&2
sleep 0.2
exit 1
`
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := (&Writer{Dir: dir, Env: []string{"FAKE_GIT_COUNT=" + counter}}).git(ctx, "fetch", "origin", "main")
		done <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(counter); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Git process did not start")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrPublication) {
			t.Fatalf("cancelled read: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled Git process was not reaped")
	}
	calls, _ := os.ReadFile(counter)
	if strings.TrimSpace(string(calls)) != "1" {
		t.Fatalf("cancelled command was retried: %s", calls)
	}
}
