// Command deployctl drives deployments through the orchestrator HTTP API, which
// is the same request the Web Console Deploy page sends. The verification
// scripts use it so the cloud path is exercised end to end instead of calling
// the application service directly.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type scoreSamples struct {
	Order   []string                  `json:"order"`
	Samples map[string]map[string]any `json:"samples"`
}

// deployRequest is the exact body the Web Console Deploy page sends. The run id
// is deliberately absent: the orchestrator process owns it.
type deployRequest struct {
	ApplicationKey string         `json:"applicationKey"`
	EnvironmentKey string         `json:"environmentKey"`
	WorkloadID     string         `json:"workloadId"`
	Actor          string         `json:"actor"`
	Score          map[string]any `json:"score"`
}

type deployResponse struct {
	DeploymentID string `json:"deploymentId"`
	Status       string `json:"status"`
	PlanHash     string `json:"planHash"`
	WorkloadID   string `json:"workloadId"`
	Error        string `json:"error"`
}

func main() {
	api := flag.String("api", "", "orchestrator base URL, for example http://127.0.0.1:8080")
	applicationKey := flag.String("application", "", "application key")
	environmentKey := flag.String("environment", "dev", "environment key")
	workloads := flag.String("workloads", "backend,worker,frontend", "comma separated deployment order")
	actor := flag.String("actor", "deployctl", "actor recorded on the deployment")
	timeout := flag.Duration("timeout", 90*time.Minute, "per-deployment timeout")
	out := flag.String("deployment-id-file", "", "optional file receiving the last deployment id")
	flag.Parse()

	if *api == "" || *applicationKey == "" {
		fmt.Fprintln(os.Stderr, "deployctl: -api and -application are required")
		os.Exit(2)
	}
	base := strings.TrimRight(*api, "/")
	client := &http.Client{Timeout: *timeout}

	samples, err := fetchSamples(client, base)
	if err != nil {
		fmt.Fprintf(os.Stderr, "deployctl: %v\n", err)
		os.Exit(1)
	}

	last := ""
	for _, workload := range strings.Split(*workloads, ",") {
		workload = strings.TrimSpace(workload)
		if workload == "" {
			continue
		}
		score, ok := samples.Samples[workload]
		if !ok {
			fmt.Fprintf(os.Stderr, "deployctl: the API has no Score sample for %q\n", workload)
			os.Exit(1)
		}
		started := time.Now()
		result, err := deploy(client, base, deployRequest{
			ApplicationKey: *applicationKey,
			EnvironmentKey: *environmentKey,
			WorkloadID:     workload,
			Actor:          *actor,
			Score:          score,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "deployctl: deploy %s: %v\n", workload, err)
			os.Exit(1)
		}
		fmt.Printf("%s: %s (%s) in %s\n", workload, result.Status, result.DeploymentID,
			time.Since(started).Round(time.Second))
		if result.Status != "SUCCEEDED" {
			fmt.Fprintf(os.Stderr, "deployctl: %s ended in %s\n", workload, result.Status)
			os.Exit(1)
		}
		last = result.DeploymentID
	}

	if *out != "" && last != "" {
		if err := os.WriteFile(*out, []byte(last), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "deployctl: write deployment id: %v\n", err)
			os.Exit(1)
		}
	}
}

func fetchSamples(client *http.Client, base string) (*scoreSamples, error) {
	resp, err := client.Get(base + "/api/v1/score-samples")
	if err != nil {
		return nil, fmt.Errorf("fetch score samples: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("score samples returned %d", resp.StatusCode)
	}
	var samples scoreSamples
	if err := json.NewDecoder(resp.Body).Decode(&samples); err != nil {
		return nil, fmt.Errorf("decode score samples: %w", err)
	}
	return &samples, nil
}

func deploy(client *http.Client, base string, req deployRequest) (*deployResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	resp, err := client.Post(base+"/api/v1/deployments", "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result deployResponse
	_ = json.Unmarshal(body, &result)
	if resp.StatusCode != http.StatusCreated {
		if result.Error != "" {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, result.Error)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return &result, nil
}
