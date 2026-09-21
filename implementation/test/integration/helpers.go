//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// kubeTarget selects how kubectl reaches a cluster: a context for kind, a
// generated kubeconfig for EKS.
type kubeTarget struct {
	Context    string
	Kubeconfig string
}

func (k kubeTarget) args(extra ...string) []string {
	var args []string
	if k.Kubeconfig != "" {
		args = append(args, "--kubeconfig", k.Kubeconfig)
	}
	if k.Context != "" {
		args = append(args, "--context", k.Context)
	}
	return append(args, extra...)
}

func kubectl(t *testing.T, target kubeTarget, args ...string) string {
	t.Helper()
	cmd := exec.Command("kubectl", target.args(args...)...)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("kubectl %s: %v: %s", strings.Join(args, " "), err, errOut.String())
	}
	return out.String()
}

func requireEnv(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Skipf("%s is not set; run the matching script in test/integration", key)
	}
	return value
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal(message)
}

// runJobFlow proves frontend -> backend -> PostgreSQL -> worker -> backend -> frontend
// through a port-forward, so no public load balancer is needed.
func runJobFlow(ctx context.Context, t *testing.T, target kubeTarget, namespace, label string) string {
	t.Helper()
	port := freePort(t)
	forward := exec.CommandContext(ctx, "kubectl",
		target.args("port-forward", "-n", namespace, "svc/frontend", fmt.Sprintf("%d:8080", port))...)
	if err := forward.Start(); err != nil {
		t.Fatalf("port-forward: %v", err)
	}
	defer func() {
		_ = forward.Process.Kill()
		_ = forward.Wait()
	}()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 15 * time.Second}
	waitFor(t, 90*time.Second, func() bool {
		resp, err := client.Get(base + "/healthz")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, "the acceptance frontend port-forward is not serving")

	payload := fmt.Sprintf(`{"payload":"%s-%d"}`, label, time.Now().UnixNano())
	resp, err := client.Post(base+"/api/jobs", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("submit job through the frontend: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("submit returned %d: %s", resp.StatusCode, body)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode job: %v", err)
	}

	var result, processedBy string
	waitFor(t, 180*time.Second, func() bool {
		jobResp, err := client.Get(fmt.Sprintf("%s/api/jobs/%d", base, created.ID))
		if err != nil {
			return false
		}
		defer jobResp.Body.Close()
		var job struct {
			Status      string `json:"status"`
			Result      string `json:"result"`
			ProcessedBy string `json:"processedBy"`
		}
		if err := json.NewDecoder(jobResp.Body).Decode(&job); err != nil {
			return false
		}
		if job.Status == "DONE" && job.ProcessedBy != "" {
			result, processedBy = job.Result, job.ProcessedBy
			return true
		}
		return false
	}, "the worker did not process the job")

	if !strings.HasPrefix(result, "processed:") {
		t.Fatalf("unexpected worker result %q", result)
	}
	t.Logf("job %d processed by %s", created.ID, processedBy)

	pageResp, err := client.Get(base + "/")
	if err != nil {
		t.Fatalf("frontend page: %v", err)
	}
	defer pageResp.Body.Close()
	page, _ := io.ReadAll(pageResp.Body)
	if !strings.Contains(string(page), result) {
		t.Fatalf("the acceptance frontend does not display the worker result %q", result)
	}
	return result
}
