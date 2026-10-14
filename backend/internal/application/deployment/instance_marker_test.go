package deployment

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/domain/resource"
)

type recordingMarker struct {
	calls [][4]string
	gens  []int64
	err   error
}

func (m *recordingMarker) MarkUnreferenced(_ context.Context, generation int64, descriptor, scopeType, scopeID string) error {
	m.calls = append(m.calls, [4]string{descriptor, scopeType, scopeID, ""})
	m.gens = append(m.gens, generation)
	return m.err
}

func TestUnreferencedMarkIsMirroredBestEffort(t *testing.T) {
	d, _ := resource.ParseDescriptor("postgres.default#shared.db")
	list := []resource.ActiveResource{{Descriptor: d, Scope: resource.Scope{Type: resource.ScopeShared, ID: "app.staging"}}}
	// Without a marker (no Terraform instances) nothing happens.
	(&Service{}).markInstancesUnreferenced(context.Background(), 3, list)
	m := &recordingMarker{err: errors.New("state login down")}
	s := &Service{instanceMarker: m}
	// A failing mirror never fails the committed deployment; it is retried by neither
	// path, so the instance stays ineligible for deletion (fail closed).
	s.markInstancesUnreferenced(context.Background(), 3, list)
	if len(m.calls) != 1 || m.calls[0][0] != "postgres.default#shared.db" || m.calls[0][1] != "SHARED" || m.gens[0] != 3 {
		t.Fatalf("calls %+v gens %v", m.calls, m.gens)
	}
}
