// Command frontend is the acceptance application user-facing workload. It renders
// the job list and proxies API calls to the backend Service. It is not the
// Orchestrator Web Console.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"orchestrator/examples/acceptance-app/internal/jobstore"
)

type job struct {
	ID          int64  `json:"id"`
	Payload     string `json:"payload"`
	Status      string `json:"status"`
	Result      string `json:"result"`
	ProcessedBy string `json:"processedBy"`
}

type pageData struct {
	Lifecycle  lifecycleView
	BackendURL string
	Jobs       []job
	Error      string
	Checks     []check
}

type check struct {
	Name   string
	Passed bool
}

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Acceptance application</title><style>body{font:16px system-ui;max-width:760px;margin:3rem auto;padding:0 1rem;color:#24292f}table{border-collapse:collapse;width:100%}td,th{padding:.6rem;border-bottom:1px solid #d0d7de;text-align:left}.pass{color:#1a7f37}.fail{color:#cf222e}#scheduled-runs{table-layout:fixed}#scheduled-runs td,code{overflow-wrap:anywhere}</style></head>
<body>
<h1>Acceptance application</h1>
<h2>Deployment checks</h2>
<table id="checks"><tr><th>Check</th><th>Status</th></tr>{{range .Checks}}<tr data-check="{{.Name}}"><td>{{.Name}}</td><td class="{{if .Passed}}pass{{else}}fail{{end}}">{{if .Passed}}PASS{{else}}FAIL{{end}}</td></tr>{{end}}</table>
<p id="backend">backend: {{.BackendURL}}</p>
{{if .Error}}<p id="error">backend error: {{.Error}}</p>{{end}}
<h2>Job flow</h2>
<form method="post" action="/submit">
  <input type="text" name="payload" value="hello-orchestrator" />
  <button type="submit">Submit job</button>
</form>
<table id="jobs">
<tr><th>id</th><th>payload</th><th>status</th><th>result</th><th>processed by</th></tr>
{{range .Jobs}}<tr class="job" data-id="{{.ID}}"><td>{{.ID}}</td><td>{{.Payload}}</td><td>{{.Status}}</td><td>{{.Result}}</td><td>{{.ProcessedBy}}</td></tr>
{{end}}
</table>
<h2>Persistent storage</h2>
{{if .Lifecycle.StorageError}}<p role="alert">Storage unavailable</p>{{else if not .Lifecycle.Storage.Configured}}<p>Storage check is not configured.</p>{{else}}
<form id="storage-form" method="post" action="/storage"><label>Test value <input name="value" maxlength="4096" required /></label><button type="submit">Write to volume</button></form>
{{if .Lifecycle.Storage.Present}}<p>Stored value: <code id="storage-value">{{.Lifecycle.Storage.Value}}</code></p><p>SHA-256: <code id="storage-hash">{{.Lifecycle.Storage.SHA256}}</code></p>{{else}}<p id="storage-empty">No value written yet.</p>{{end}}
<p>Backend process: <code id="storage-process">{{.Lifecycle.Storage.Process}}</code></p>{{end}}
<h2>Scheduled runs</h2>
{{if .Lifecycle.ScheduleError}}<p role="alert">Scheduled run history unavailable</p>{{else if not .Lifecycle.ScheduleConfigured}}<p>Scheduled check is not configured.</p>{{else}}
<p>Run: <code id="schedule-run">{{.Lifecycle.ScheduleRun}}</code></p>
<table id="scheduled-runs"><tr><th>Invocation</th><th>Value SHA-256</th><th>Completed (UTC)</th></tr>{{range .Lifecycle.Runs}}<tr data-invocation="{{.Invocation}}"><td>{{.Invocation}}</td><td>{{.ValueSHA256}}</td><td>{{.CompletedAt.UTC.Format "2006-01-02 15:04:05"}}</td></tr>{{end}}</table>
{{if not .Lifecycle.Runs}}<p>No scheduled receipt yet.</p>{{end}}{{end}}
<p><a href="/">Refresh observations</a></p>
</body>
</html>`))

// newChecks lists the existing deployment-check rows, all failing until the
// backend reports otherwise. "cache" is the Redis dependency.
func newChecks() []check {
	return []check{{Name: "backend connection"}, {Name: "environment"}, {Name: "secret"}, {Name: "database"}, {Name: "cache"}}
}

// applyChecks marks the backend connection as passing and copies the backend's
// boolean results. A check the backend does not report stays FAIL.
func applyChecks(checks []check, reported map[string]bool) {
	checks[0].Passed = true
	for i := 1; i < len(checks); i++ {
		checks[i].Passed = reported[checks[i].Name]
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	backendURL := strings.TrimRight(jobstore.EnvOr("BACKEND_URL", "http://backend:8080"), "/")
	client := &http.Client{Timeout: 10 * time.Second}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","workload":"frontend"}`))
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		data := pageData{BackendURL: backendURL, Checks: newChecks()}
		data.Lifecycle = readLifecycle(r.Context(), client, backendURL)
		checksResp, checksErr := client.Get(backendURL + "/api/checks")
		if checksErr == nil {
			var body struct {
				Checks map[string]bool `json:"checks"`
			}
			decodeErr := json.NewDecoder(checksResp.Body).Decode(&body)
			_ = checksResp.Body.Close()
			if checksResp.StatusCode == http.StatusOK && decodeErr == nil {
				applyChecks(data.Checks, body.Checks)
			}
		}
		resp, err := client.Get(backendURL + "/api/jobs")
		if err != nil {
			data.Error = err.Error()
		} else {
			defer resp.Body.Close()
			var body struct {
				Jobs []job `json:"jobs"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				data.Error = err.Error()
			} else {
				data.Jobs = body.Jobs
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := page.Execute(w, data); err != nil {
			log.Printf("frontend: render: %v", err)
		}
	})
	mux.HandleFunc("POST /storage", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		if r.ParseForm() != nil {
			http.Error(w, "invalid form", 400)
			return
		}
		payload, _ := json.Marshal(map[string]string{"value": r.FormValue("value")})
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, backendURL+"/api/storage", bytes.NewReader(payload))
		if err != nil {
			http.Error(w, "backend unavailable", 502)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "backend unavailable", 502)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			http.Error(w, "could not store test value", 502)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	mux.HandleFunc("POST /submit", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		payload, err := json.Marshal(map[string]string{"payload": r.FormValue("payload")})
		if err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		resp, err := client.Post(backendURL+"/api/jobs", "application/json", bytes.NewReader(payload))
		if err != nil {
			http.Error(w, "backend unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	// The frontend proxies /api to the backend Service so a single port-forward
	// exercises the whole chain.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		target, err := url.Parse(backendURL + r.URL.Path)
		if err != nil {
			http.Error(w, "invalid backend url", http.StatusInternalServerError)
			return
		}
		target.RawQuery = r.URL.RawQuery
		proxied, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), r.Body)
		if err != nil {
			http.Error(w, "invalid proxy request", http.StatusInternalServerError)
			return
		}
		proxied.Header = r.Header.Clone()
		resp, err := client.Do(proxied)
		if err != nil {
			http.Error(w, "backend unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})

	addr := ":" + jobstore.EnvOr("PORT", "8080")
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	log.Printf("frontend: listening on %s, backend %s", addr, backendURL)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("frontend: %v", err)
		os.Exit(1)
	}
}
