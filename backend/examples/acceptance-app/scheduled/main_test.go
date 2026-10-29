package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"orchestrator/examples/acceptance-app/internal/jobstore"
	"orchestrator/examples/acceptance-app/internal/testfixture"
	"testing"
	"time"
)

func TestReadHashRequiresActualStoredValueAndMatchingDigest(t *testing.T) {
	h := sha256.Sum256([]byte("proof"))
	hash := hex.EncodeToString(h[:])
	for _, tc := range []struct {
		name   string
		status int
		body   any
		ok     bool
	}{
		{"stored", 200, map[string]any{"configured": true, "present": true, "value": "proof", "sha256": hash}, true},
		{"disabled", 200, map[string]any{"configured": false}, false},
		{"empty", 200, map[string]any{"configured": true, "present": false}, false},
		{"mismatch", 200, map[string]any{"configured": true, "present": true, "value": "changed", "sha256": hash}, false},
		{"error", 503, map[string]any{"error": "unavailable"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/storage" {
					t.Error(r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(tc.body)
			}))
			defer s.Close()
			got, e := readHash(context.Background(), &http.Client{Timeout: time.Second}, s.URL)
			if (e == nil) != tc.ok {
				t.Fatal(e)
			}
			if tc.ok && got != hash {
				t.Fatal(got)
			}
		})
	}
}

func TestInvocationCommitsReceiptAndRetryDoesNotDuplicate(t *testing.T) {
	cfg := testfixture.PostgresConfigWithPassword(t, "scheduled-command-test-password")
	ctx := context.Background()
	store, err := jobstore.Open(ctx, cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"PGHOST": cfg.Host, "PGPORT": fmt.Sprint(cfg.Port), "PGDATABASE": cfg.Database, "PGUSER": cfg.User, "PGPASSWORD": cfg.Password, "HOSTNAME": "job-pod-a", "ACCEPTANCE_SCHEDULE_RUN": "test-a"} {
		t.Setenv(k, v)
	}
	h := sha256.Sum256([]byte("proof"))
	hash := hex.EncodeToString(h[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"configured": true, "present": true, "value": "proof", "sha256": hash})
	}))
	defer server.Close()
	t.Setenv("BACKEND_URL", server.URL)
	for i := 0; i < 2; i++ {
		if err := run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.ScheduledRuns(ctx, "test-a")
	if err != nil || len(rows) != 1 || rows[0].ValueSHA256 != hash || rows[0].Invocation != "job-pod-a" {
		t.Fatalf("%+v %v", rows, err)
	}
}
