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
	BackendURL string
	Jobs       []job
	Error      string
}

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Acceptance application</title></head>
<body>
<h1>Acceptance application</h1>
<p id="backend">backend: {{.BackendURL}}</p>
{{if .Error}}<p id="error">backend error: {{.Error}}</p>{{end}}
<form method="post" action="/submit">
  <input type="text" name="payload" value="hello-orchestrator" />
  <button type="submit">Submit job</button>
</form>
<table id="jobs">
<tr><th>id</th><th>payload</th><th>status</th><th>result</th><th>processed by</th></tr>
{{range .Jobs}}<tr class="job" data-id="{{.ID}}"><td>{{.ID}}</td><td>{{.Payload}}</td><td>{{.Status}}</td><td>{{.Result}}</td><td>{{.ProcessedBy}}</td></tr>
{{end}}
</table>
</body>
</html>`))

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
		data := pageData{BackendURL: backendURL}
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
