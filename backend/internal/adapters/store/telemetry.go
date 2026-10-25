package store

import (
	"context"
	"encoding/json"
	"time"

	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/ports/persistence"
)

func copyTelemetry(in telemetry.State) telemetry.State {
	b, _ := json.Marshal(in)
	var out telemetry.State
	_ = json.Unmarshal(b, &out)
	return out
}

func (s *Store) telemetryScope(scope telemetry.Scope) bool {
	a, ok := s.state.Applications[scope.ApplicationKey]
	if !ok || a.OrganizationKey != scope.OrganizationKey {
		return false
	}
	_, ok = s.state.Environments[envKey(scope.ApplicationKey, scope.EnvironmentKey)]
	return ok
}

func (s *Store) LeaseTelemetry(ctx context.Context, source telemetry.Source, owner string, now time.Time) (out telemetry.State, acquired bool, err error) {
	err = s.Transact(ctx, func(ctx context.Context) error {
		if !source.Valid() || owner == "" || !s.telemetryScope(source.Scope) {
			return persistence.ErrNotFound
		}
		env := s.state.Environments[envKey(source.ApplicationKey, source.EnvironmentKey)]
		if env.TargetGeneration != source.Generation {
			return persistence.ErrNotFound
		}
		state, ok := s.state.TelemetrySources[source.Key]
		if ok && state.Source != source {
			return persistence.ErrImmutable
		}
		if ok && state.Owner != "" && state.LeaseUntil.After(now) {
			out = copyTelemetry(state)
			return nil
		}
		if !ok {
			state = telemetry.State{Source: source, Checkpoints: map[string]telemetry.Cursor{}, LogsCode: telemetry.Waiting, MetricsCode: telemetry.Waiting}
		}
		state.Owner = owner
		state.Fence++
		state.LeaseUntil = now.Add(telemetry.LeaseDuration)
		s.state.TelemetrySources[source.Key] = copyTelemetry(state)
		out = copyTelemetry(state)
		acquired = true
		return nil
	})
	return
}

func (s *Store) CommitTelemetry(ctx context.Context, b telemetry.Batch) error {
	if err := b.Validate(); err != nil {
		return err
	}
	return s.Transact(ctx, func(ctx context.Context) error {
		old, ok := s.state.TelemetrySources[b.State.Source.Key]
		if !ok || old.Source != b.State.Source || old.Owner != b.State.Owner || old.Fence != b.State.Fence || !old.LeaseUntil.After(b.At) {
			return persistence.ErrOperationLost
		}
		logCount, logBytes, sampleCount, eventCount := 0, 0, 0, 0
		for _, l := range s.state.TelemetryLogs {
			if l.SourceKey == old.Source.Key {
				logCount++
				logBytes += len(l.Message)
			}
		}
		for _, v := range s.state.TelemetrySamples {
			if v.SourceKey == old.Source.Key {
				sampleCount++
			}
		}
		for _, v := range s.state.TelemetryEvents {
			if v.SourceKey == old.Source.Key {
				eventCount++
			}
		}
		newLogs := map[string]telemetry.Log{}
		for _, l := range b.Logs {
			if _, exists := s.state.TelemetryLogs[l.ID]; !exists {
				l.SourceKey = old.Source.Key
				newLogs[l.ID] = l
			}
		}
		newSamples := map[string]telemetry.Sample{}
		for _, v := range b.Samples {
			if _, exists := s.state.TelemetrySamples[v.ID]; !exists {
				v.SourceKey = old.Source.Key
				newSamples[v.ID] = v
			}
		}
		newEvents := map[string]telemetry.Event{}
		for _, v := range b.Events {
			if _, exists := s.state.TelemetryEvents[v.ID]; !exists {
				v.SourceKey = old.Source.Key
				newEvents[v.ID] = v
			}
		}
		bytes := 0
		for _, l := range newLogs {
			bytes += len(l.Message)
		}
		state := copyTelemetry(b.State)
		state.Limited = false
		state.LogsCode = telemetry.SafeCode(state.LogsCode)
		state.MetricsCode = telemetry.SafeCode(state.MetricsCode)
		if logCount+len(newLogs) > telemetry.MaxLogRows || logBytes+bytes > telemetry.MaxLogBytes {
			state.Checkpoints = old.Checkpoints
			state.LastLogAt = old.LastLogAt
			state.LogsCode = telemetry.StorageLimit
			state.Limited = true
		} else {
			for id, l := range newLogs {
				s.state.TelemetryLogs[id] = l
			}
		}
		if sampleCount+len(newSamples) > telemetry.MaxSamples {
			state.LastMetricAt = old.LastMetricAt
			state.MetricsCode = telemetry.StorageLimit
			state.Limited = true
		} else {
			for id, v := range newSamples {
				s.state.TelemetrySamples[id] = v
			}
		}
		if eventCount+len(newEvents) > telemetry.MaxEvents {
			state.Limited = true
		} else {
			for id, v := range newEvents {
				s.state.TelemetryEvents[id] = v
			}
		}
		// Release ownership after commit; a new lease receives a new fence.
		state.Owner = ""
		state.LeaseUntil = b.At
		s.state.TelemetrySources[old.Source.Key] = state
		return nil
	})
}

func (s *Store) ReadTelemetry(ctx context.Context, q telemetry.Query) (telemetry.Data, error) {
	defer s.rlock(ctx)()
	if !s.telemetryScope(q.Scope) {
		return telemetry.Data{}, persistence.ErrNotFound
	}
	states := []telemetry.State{}
	allowed := map[string]bool{}
	for key, state := range s.state.TelemetrySources {
		if state.Source.Scope == q.Scope && (q.Workload == "" || state.Source.WorkloadID == q.Workload) {
			allowed[key] = true
			states = append(states, copyTelemetry(state))
		}
	}
	logs := []telemetry.Log{}
	samples := []telemetry.Sample{}
	events := []telemetry.Event{}
	for _, v := range s.state.TelemetryLogs {
		if allowed[v.SourceKey] {
			logs = append(logs, v)
		}
	}
	for _, v := range s.state.TelemetrySamples {
		if allowed[v.SourceKey] {
			samples = append(samples, v)
		}
	}
	for _, v := range s.state.TelemetryEvents {
		if allowed[v.SourceKey] {
			events = append(events, v)
		}
	}
	if q.Kind == "metrics" {
		logs = nil
	}
	if q.Kind == "logs" {
		samples = nil
		events = nil
	}
	return telemetry.Summarize(logs, samples, events, states, q), nil
}

func (s *Store) PruneTelemetry(ctx context.Context, now time.Time) error {
	return s.Transact(ctx, func(ctx context.Context) error {
		for id, v := range s.state.TelemetryLogs {
			if v.Timestamp.Before(now.Add(-telemetry.LogRetention)) {
				delete(s.state.TelemetryLogs, id)
			}
		}
		for id, v := range s.state.TelemetrySamples {
			if v.ObservedAt.Before(now.Add(-telemetry.MetricRetention)) {
				delete(s.state.TelemetrySamples, id)
			}
		}
		for id, v := range s.state.TelemetryEvents {
			if v.Timestamp.Before(now.Add(-telemetry.MetricRetention)) {
				delete(s.state.TelemetryEvents, id)
			}
		}
		return nil
	})
}

var _ persistence.TelemetryRepository = (*Store)(nil)
