// Development-only notification receiver; never use as a production receiver.
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

func main() {
	var mu sync.Mutex
	receipts := []json.RawMessage{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /alerts", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		b, e := io.ReadAll(r.Body)
		if e != nil {
			http.Error(w, "invalid", 400)
			return
		}
		var payload struct {
			Alerts []struct {
				Labels map[string]string `json:"labels"`
			} `json:"alerts"`
		}
		if json.Unmarshal(b, &payload) != nil || len(payload.Alerts) == 0 {
			http.Error(w, "invalid", 400)
			return
		}
		for _, a := range payload.Alerts {
			if a.Labels["orchestrator_test"] != "true" {
				http.Error(w, "test alerts only", 403)
				return
			}
		}
		mu.Lock()
		defer mu.Unlock()
		if len(receipts) >= 1000 {
			http.Error(w, "capacity", 503)
			return
		}
		receipt, _ := json.Marshal(map[string]any{"receivedAt": time.Now().UTC(), "notification": json.RawMessage(b)})
		receipts = append(receipts, receipt)
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /notifications", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(receipts)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	server := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, MaxHeaderBytes: 8192}
	log.Fatal(server.ListenAndServe())
}
