package loki

import (
	"context"
	"encoding/json"
	"net/url"
	"orchestrator/internal/domain/monitoring"
	"strconv"
	"strings"
	"testing"
	"time"
)

type transportFixture struct {
	t     *testing.T
	scope string
	query string
}

func (f *transportFixture) ReadJSON(_ context.Context, path string, q url.Values, out any) error {
	f.query = q.Get("query")
	if path != "/loki/api/v1/query_range" || q.Get("direction") != "backward" || q.Get("limit") != "2000" {
		f.t.Fatal("unbounded/wrong Loki request")
	}
	labels := map[string]string{"cluster_uid": "cluster", "organization": "org", "application": f.scope, "environment": "staging", "generation": "1", "namespace": "owned", "workload": "api", "container": "server", "pod": "api-old"}
	at := time.Now().Add(-time.Minute).UnixNano()
	raw, _ := json.Marshal(map[string]any{"status": "success", "data": map[string]any{"resultType": "streams", "result": []any{map[string]any{"stream": labels, "values": []any{[]any{strconv.FormatInt(at, 10), `{"level":"error","message":"old pod"}`, map[string]string{"pod_uid": "old-uid"}}, []any{strconv.FormatInt(at-1, 10), "plain text"}}}}}})
	return json.Unmarshal(raw, out)
}
func TestRetainedLokiLogsUseTrustedScopeAndKeepUnknownLevel(t *testing.T) {
	f := &transportFixture{t: t, scope: "app"}
	q := Query{Scope: monitoring.Scope{Organization: "org", Application: "app", Environment: "staging", Generation: 1, ClusterUID: "cluster"}, Namespaces: []string{"owned"}, Workload: "api", Search: `a"} |= "foreign`, From: time.Now().Add(-time.Hour), To: time.Now(), Limit: 1}
	page, e := Read(context.Background(), f, q)
	if e != nil || len(page.Logs) != 1 || page.Logs[0].PodUID != "old-uid" || page.Logs[0].Level != "ERROR" || page.NextCursor == "" {
		t.Fatalf("retained log page: %+v %v", page, e)
	}
	for _, want := range []string{`cluster_uid="cluster"`, `organization="org"`, `application="app"`, `generation="1"`, `namespace=~"owned"`} {
		if !strings.Contains(f.query, want) {
			t.Fatal("missing trusted selector", want)
		}
	}
	q.Limit = 2
	page, e = Read(context.Background(), f, q)
	if e != nil || page.Logs[1].Level != "UNKNOWN" {
		t.Fatal("invented level", e)
	}
	f.scope = "foreign"
	if _, e = Read(context.Background(), f, q); e == nil {
		t.Fatal("accepted cross-scope provider response")
	}
	q.Namespaces = nil
	if _, e = Read(context.Background(), f, q); e == nil {
		t.Fatal("unscoped namespace query")
	}
}
