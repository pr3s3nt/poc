package prometheus

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEndpointSafety(t *testing.T) {
	for _, raw := range []string{"file:///tmp/x", "http://user:pass@localhost", "http://169.254.169.254", "http://[::ffff:169.254.169.254]", "http://metadata.google.internal", "http://host/?token=x", "http://host/#x", "http://0.0.0.0", "http://224.0.0.1", "http://host:0"} {
		if _, e := Endpoint(raw); e == nil {
			t.Errorf("accepted forbidden endpoint %s", raw)
		}
	}
	for _, raw := range []string{"https://monitoring.internal/prometheus", "http://127.0.0.1:9090", "http://10.0.0.2:9090"} {
		if _, e := Endpoint(raw); e != nil {
			t.Errorf("valid endpoint rejected: %v", e)
		}
	}
}
func TestBoundedQueries(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer private" {
			t.Error("credential not forwarded")
		}
		if r.URL.Query().Get("query") != `up{namespace="owned"}` {
			t.Error("query encoding changed")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"pod":"owned-pod"},"values":[[100,"0"],[130,"NaN"],[160,"1.5"]]}]}}`))
	}))
	defer server.Close()
	c, e := New(Config{Endpoint: server.URL, BearerToken: "private"})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	data, e := c.Range(context.Background(), `up{namespace="owned"}`, time.Unix(100, 0), time.Unix(160, 0), 30*time.Second)
	if e != nil || len(data) != 1 {
		t.Fatalf("query failed %v", e)
	}
	if data[0].Values[0].Value == nil || *data[0].Values[0].Value != 0 || data[0].Values[1].Value != nil || *data[0].Values[2].Value != 1.5 {
		t.Fatal("zero/gaps lost")
	}
	_, e = c.Range(context.Background(), "up", time.Unix(0, 0), time.Unix(7200, 0), 30*time.Second)
	if !errors.Is(e, ErrBounds) || calls != 1 {
		t.Fatal("oversized range reached provider")
	}
}
func TestProviderFailuresAreSafe(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"upstream", `private credential=secret`, 500},
		{"partial", `{"status":"success","warnings":["partial"],"data":{"resultType":"vector","result":[]}}`, 200},
		{"wrong type", `{"status":"success","data":{"resultType":"matrix","result":[]}}`, 200},
		{"missing sample", `{"status":"success","data":{"resultType":"vector","result":[{"metric":{}}]}}`, 200},
		{"oversize", strings.Repeat("x", MaxResponse+1), 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer s.Close()
			c, e := New(Config{Endpoint: s.URL})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			_, e = c.Instant(context.Background(), "up", time.Now())
			if e == nil || strings.Contains(e.Error(), "secret") {
				t.Fatal("unsafe success or error")
			}
		})
	}
}
func TestRedirectDoesNotLeakCredentials(t *testing.T) {
	reached := false
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer dest.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, dest.URL, 302) }))
	defer s.Close()
	c, e := New(Config{Endpoint: s.URL, BearerToken: "private"})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.Instant(context.Background(), "up", time.Now())
	if e == nil || reached {
		t.Fatal("redirect followed")
	}
}

func TestRangeKeepsMissingEvaluationIntervalsNull(t *testing.T) {
	from := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	a, b := 1.0, 2.0
	data := fillRangeGaps([]Series{{Values: []Sample{{Timestamp: from, Value: &a}, {Timestamp: from.Add(time.Minute), Value: &b}}}}, from, from.Add(time.Minute), 30*time.Second)
	if len(data) != 1 || len(data[0].Values) != 3 || data[0].Values[1].Value != nil || data[0].Values[2].Value == nil {
		t.Fatal("missing interval became a connected/zero observation")
	}
}
func TestAlertmanagerStateIsScopedAndNotDeliveryProof(t *testing.T) {
	foreign := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/alerts" || r.URL.Query().Get("filter") != `orchestrator_app="owned"` {
			t.Error("missing AM scope filter")
		}
		app := "owned"
		if foreign {
			app = "foreign"
		}
		fmt.Fprintf(w, `[{"labels":{"orchestrator_app":%q,"alertname":"Test"},"status":{"state":"suppressed","silencedBy":[],"inhibitedBy":["test"]},"receivers":[{"name":"owned-test"}]}]`, app)
	}))
	defer server.Close()
	client, e := New(Config{Endpoint: server.URL})
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	states, e := client.AlertmanagerAlerts(context.Background(), map[string]string{"orchestrator_app": "owned"})
	if e != nil || len(states) != 1 || states[0].State != "suppressed" {
		t.Fatal("AM state", e)
	}
	foreign = true
	if _, e = client.AlertmanagerAlerts(context.Background(), map[string]string{"orchestrator_app": "owned"}); e == nil {
		t.Fatal("foreign AM response accepted")
	}
}
