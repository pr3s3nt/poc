package postgres

import (
	"context"
	"errors"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/monitoring"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"testing"
	"time"
)

func TestMonitoringRepositoryRevisionAndReferences(t *testing.T) {
	ctx := context.Background()
	db := rolesOnly(t)
	if _, e := Setup(ctx, db.Owner, setupRoles(db)); e != nil {
		t.Fatal(e)
	}
	st, e := Open(ctx, db.Console)
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	for _, f := range []func() error{
		func() error {
			return st.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: "observe", Name: "Observe"})
		},
		func() error {
			return st.SaveApplication(ctx, application.Application{ID: ids.New(), Key: "shop", OrganizationKey: "observe", Name: "Shop"})
		},
		func() error {
			return st.SaveEnvironment(ctx, environment.Environment{ID: ids.New(), ApplicationKey: "shop", Key: "staging", Name: "Staging", NamespaceIdentity: "shop-stage"})
		},
	} {
		if e = f(); e != nil {
			t.Fatal(e)
		}
	}
	c := monitoring.Connection{ID: ids.New(), Key: "metrics", Organization: "observe", Name: "Metrics", Revision: 1, ClusterUID: "cluster", Rules: monitoring.DevRules(), CredentialRefs: map[string]string{"loki": "opaque-private-ref"}}
	if e = st.Transact(ctx, func(ctx context.Context) error { return st.PutMonitoringConnection(ctx, c, 0) }); e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	c.VerifiedAt = &now
	c.Verification = monitoring.Verification{Revision: 1, Connectivity: map[string]bool{"loki": true}}
	if e = st.VerifyMonitoringConnection(ctx, c, 1); e != nil {
		t.Fatal(e)
	}
	loaded, e := st.GetMonitoringConnection(ctx, "observe", c.ID)
	if e != nil || loaded.Revision != 1 || loaded.VerifiedAt == nil || !loaded.Verification.Connectivity["loki"] {
		t.Fatalf("verification/config revision: %+v %v", loaded, e)
	}
	env, e := st.GetEnvironment(ctx, "shop", "staging")
	if e != nil {
		t.Fatal(e)
	}
	b := monitoring.Binding{Organization: "observe", Application: "shop", Environment: "staging", ConnectionID: c.ID, ConnectionRevision: 1, ClusterUID: "cluster", Generation: env.TargetGeneration, Revision: 1, LogsSource: "loki"}
	if e = st.PutMonitoringBinding(ctx, b, 0); e != nil {
		t.Fatal(e)
	}
	if e = st.DeleteMonitoringConnection(ctx, "observe", c.ID, 1); !errors.Is(e, monitoring.ErrInUse) {
		t.Fatal("referenced deletion", e)
	}
	if e = st.DeleteMonitoringBinding(ctx, "observe", "shop", "staging", 2); !errors.Is(e, persistence.ErrVersionConflict) {
		t.Fatal("stale delete", e)
	}
	if e = st.DeleteMonitoringBinding(ctx, "foreign", "shop", "staging", 1); !errors.Is(e, persistence.ErrNotFound) {
		t.Fatal("foreign delete", e)
	}
	if e = st.DeleteMonitoringBinding(ctx, "observe", "shop", "staging", 1); e != nil {
		t.Fatal(e)
	}
	c.Revision = 2
	c.VerifiedAt = nil
	c.Verification = monitoring.Verification{Revision: 2}
	if e = st.PutMonitoringConnection(ctx, c, 1); e != nil {
		t.Fatal(e)
	}
	if e = st.VerifyMonitoringConnection(ctx, loaded, 1); !errors.Is(e, persistence.ErrVersionConflict) {
		t.Fatal("stale verification", e)
	}
	if e = st.DeleteMonitoringConnection(ctx, "observe", c.ID, 2); e != nil {
		t.Fatal(e)
	}
}
