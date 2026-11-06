package observability

import (
	"context"
	"errors"
	"orchestrator/internal/adapters/loki"
	deployapp "orchestrator/internal/application/deployment"
	"orchestrator/internal/domain/monitoring"
	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"sort"
	"time"
)

func (m *Monitoring) QueryLogs(ctx context.Context, scope telemetry.Scope, p Params, source string) (View, error) {
	var out View
	if source != "" && source != "native" && source != "loki" {
		return out, &FieldError{"source"}
	}
	if m.Native == nil {
		return out, ErrUnavailable
	}
	// Reuse native authorization, range and cursor validation, then discard its data
	// when an external source is selected. No external error returns native rows.
	out, e := m.Native.Query(ctx, scope, "logs", p)
	if e != nil {
		return out, e
	}
	out.Source = "native"
	if source == "native" {
		return out, nil
	}
	if m.Repo == nil {
		if source == "loki" {
			return View{}, ErrUnavailable
		}
		return out, nil
	}
	b, e := m.Repo.GetMonitoringBinding(ctx, scope.OrganizationKey, scope.ApplicationKey, scope.EnvironmentKey)
	if errors.Is(e, persistence.ErrNotFound) && source == "" {
		return out, nil
	}
	if e != nil {
		return View{}, e
	}
	if source == "" && b.LogsSource != "loki" {
		return out, nil
	}
	env, e := m.Store.GetEnvironment(ctx, scope.ApplicationKey, scope.EnvironmentKey)
	if e != nil {
		return View{}, e
	}
	c, e := m.Repo.GetMonitoringConnection(ctx, scope.OrganizationKey, b.ConnectionID)
	if e != nil {
		return View{}, e
	}
	if b.Generation != env.TargetGeneration || c.Revision != b.ConnectionRevision {
		return View{}, persistence.ErrVersionConflict
	}
	if !sourceVerified(c, "loki") {
		return View{}, ErrUnavailable
	}
	if p.Container != "" && !workloadShape.MatchString(p.Container) {
		return View{}, &FieldError{"container"}
	}
	instances, e := m.Store.ListWorkloadInstancesFor(ctx, scope.ApplicationKey+"/"+scope.EnvironmentKey, env.TargetGeneration)
	if e != nil {
		return View{}, e
	}
	namespaces := map[string]bool{}
	for _, i := range instances {
		var t execution.Target
		if deployapp.RestoreTarget(&t, i.TargetRef, scope.OrganizationKey) == nil && t.CredentialBacked() && workloadShape.MatchString(t.Namespace) {
			namespaces[t.Namespace] = true
		}
	}
	ns := []string{}
	for n := range namespaces {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	out.Source = "loki"
	out.Data = telemetry.EmptyData()
	out.Sources = []SourceHealth{}
	out.State = "WAITING"
	out.HasHistory = false
	out.CountLowerBound = true
	if len(ns) == 0 {
		out.State = "NOT_DEPLOYED"
		return out, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cl, e := m.client(ctx, c, "loki")
	if e != nil {
		return View{}, e
	}
	defer cl.Close()
	var before *telemetry.Position
	if p.Cursor != "" {
		before, e = telemetry.DecodeCursor(p.Cursor)
		if e != nil {
			return View{}, &FieldError{"cursor"}
		}
	}
	limit := p.Limit
	if limit == 0 {
		limit = 100
	}
	page, e := loki.Read(ctx, cl, loki.Query{Scope: monitoring.Scope{Organization: scope.OrganizationKey, Application: scope.ApplicationKey, Environment: scope.EnvironmentKey, Generation: env.TargetGeneration, ClusterUID: b.ClusterUID}, Namespaces: ns, Workload: p.Workload, Container: p.Container, Search: p.Search, Level: p.Level, From: out.From, To: out.To, Limit: limit, Before: before})
	if e != nil {
		return View{}, e
	}
	out.Logs = page.Logs
	out.NextCursor = page.NextCursor
	out.LogCount = int64(len(page.Logs))
	out.HasHistory = len(page.Logs) > 0
	if out.HasHistory {
		out.State = "RECEIVING"
	}
	if page.Truncated {
		out.State = "PARTIAL"
	}
	for _, l := range page.Logs {
		if l.Level == "ERROR" {
			out.ErrorCount++
		}
		out.Histogram = append(out.Histogram, telemetry.Bucket{At: l.Timestamp, Count: 1})
	}
	return out, nil
}
