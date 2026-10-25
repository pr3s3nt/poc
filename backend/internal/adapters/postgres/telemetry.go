package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/ports/persistence"
)

const telemetryScopeJoins = ` JOIN telemetry_sources s ON s.source_key=x.source_key JOIN environments e ON e.id=s.environment_id JOIN applications a ON a.id=e.application_id JOIN organizations o ON o.id=a.organization_id `
const telemetryScopeWhere = ` o.organization_key=$1 AND a.application_key=$2 AND e.environment_key=$3 AND s.target_generation=$4 AND ($5='' OR s.workload_id=$5) `

func telemetryArgs(q telemetry.Query) []any {
	return []any{q.Scope.OrganizationKey, q.Scope.ApplicationKey, q.Scope.EnvironmentKey, q.Scope.Generation, q.Workload}
}

const telemetryStateSelect = `SELECT s.source_key,o.organization_key,a.application_key,e.environment_key,s.target_generation,s.workload_id,s.target_hash,s.checkpoints,s.lease_owner,s.fence,s.lease_until,s.logs_code,s.metrics_code,s.observed_at,s.last_log_at,s.last_metric_at,s.limited
FROM telemetry_sources s JOIN environments e ON e.id=s.environment_id JOIN applications a ON a.id=e.application_id JOIN organizations o ON o.id=a.organization_id `

func scanTelemetryState(row interface{ Scan(...any) error }) (telemetry.State, error) {
	var state telemetry.State
	var checkpoints []byte
	err := row.Scan(&state.Source.Key, &state.Source.OrganizationKey, &state.Source.ApplicationKey, &state.Source.EnvironmentKey, &state.Source.Generation, &state.Source.WorkloadID, &state.Source.TargetHash, &checkpoints, &state.Owner, &state.Fence, &state.LeaseUntil, &state.LogsCode, &state.MetricsCode, &state.ObservedAt, &state.LastLogAt, &state.LastMetricAt, &state.Limited)
	if err != nil {
		return state, translate(err)
	}
	if json.Unmarshal(checkpoints, &state.Checkpoints) != nil || state.Checkpoints == nil || len(state.Checkpoints) > 512 || !state.Source.Valid() {
		return state, errors.New("postgres: invalid telemetry source")
	}
	state.LogsCode = telemetry.SafeCode(state.LogsCode)
	state.MetricsCode = telemetry.SafeCode(state.MetricsCode)
	return state, nil
}

func (s *Store) LeaseTelemetry(ctx context.Context, source telemetry.Source, owner string, now time.Time) (state telemetry.State, acquired bool, err error) {
	if !source.Valid() || owner == "" {
		return state, false, persistence.ErrNotFound
	}
	err = s.Transact(ctx, func(ctx context.Context) error {
		_, err := s.q(ctx).Exec(ctx, `INSERT INTO telemetry_sources(source_key,environment_id,target_generation,workload_id,target_hash,lease_until,logs_code,metrics_code)
SELECT $1,e.id,$5,$6,$7,$8,'WAITING','WAITING' FROM environments e JOIN applications a ON a.id=e.application_id JOIN organizations o ON o.id=a.organization_id
WHERE o.organization_key=$2 AND a.application_key=$3 AND e.environment_key=$4 AND e.target_generation=$5 ON CONFLICT(source_key) DO NOTHING`, source.Key, source.OrganizationKey, source.ApplicationKey, source.EnvironmentKey, source.Generation, source.WorkloadID, source.TargetHash, now)
		if err != nil {
			return fmt.Errorf("postgres: telemetry source (%s)", sqlState(err))
		}
		state, err = scanTelemetryState(s.q(ctx).QueryRow(ctx, telemetryStateSelect+`WHERE s.source_key=$1 AND e.target_generation=s.target_generation FOR UPDATE OF s`, source.Key))
		if err != nil {
			return err
		}
		if state.Source != source {
			return persistence.ErrImmutable
		}
		if state.Owner != "" && state.LeaseUntil.After(now) {
			return nil
		}
		state.Owner = owner
		state.Fence++
		state.LeaseUntil = now.Add(telemetry.LeaseDuration)
		_, err = s.q(ctx).Exec(ctx, `UPDATE telemetry_sources SET lease_owner=$2,fence=$3,lease_until=$4 WHERE source_key=$1`, source.Key, owner, state.Fence, state.LeaseUntil)
		acquired = err == nil
		return translate(err)
	})
	return
}

func (s *Store) telemetryExisting(ctx context.Context, table string, ids []string) (map[string]bool, error) {
	switch table {
	case "telemetry_logs", "telemetry_samples", "telemetry_events":
	default:
		return nil, errors.New("postgres: invalid telemetry table")
	}
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q(ctx).Query(ctx, `SELECT id FROM `+table+` WHERE id=ANY($1::text[])`, ids)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (s *Store) CommitTelemetry(ctx context.Context, b telemetry.Batch) error {
	if err := b.Validate(); err != nil {
		return err
	}
	return s.Transact(ctx, func(ctx context.Context) error {
		old, err := scanTelemetryState(s.q(ctx).QueryRow(ctx, telemetryStateSelect+`WHERE s.source_key=$1 FOR UPDATE OF s`, b.State.Source.Key))
		if err != nil {
			return err
		}
		if old.Source != b.State.Source || old.Owner != b.State.Owner || old.Fence != b.State.Fence || !old.LeaseUntil.After(b.At) {
			return persistence.ErrOperationLost
		}
		var logCount, logBytes, sampleCount, eventCount int64
		if err := s.q(ctx).QueryRow(ctx, `SELECT (SELECT count(*) FROM telemetry_logs WHERE source_key=$1),(SELECT COALESCE(sum(octet_length(message)),0) FROM telemetry_logs WHERE source_key=$1),(SELECT count(*) FROM telemetry_samples WHERE source_key=$1),(SELECT count(*) FROM telemetry_events WHERE source_key=$1)`, old.Source.Key).Scan(&logCount, &logBytes, &sampleCount, &eventCount); err != nil {
			return translate(err)
		}
		ids := []string{}
		for _, l := range b.Logs {
			ids = append(ids, l.ID)
		}
		existing, err := s.telemetryExisting(ctx, "telemetry_logs", ids)
		if err != nil {
			return err
		}
		logs := []map[string]any{}
		newLogBytes := int64(0)
		for _, l := range b.Logs {
			if existing[l.ID] {
				continue
			}
			existing[l.ID] = true
			newLogBytes += int64(len(l.Message))
			logs = append(logs, map[string]any{"id": l.ID, "timestamp": l.Timestamp, "pod_uid": l.PodUID, "pod_name": l.PodName, "container_name": l.Container, "restart_index": l.RestartIndex, "level": l.Level, "message": l.Message})
		}
		ids = nil
		for _, v := range b.Samples {
			ids = append(ids, v.ID)
		}
		existing, err = s.telemetryExisting(ctx, "telemetry_samples", ids)
		if err != nil {
			return err
		}
		samples := []map[string]any{}
		for _, v := range b.Samples {
			if existing[v.ID] {
				continue
			}
			existing[v.ID] = true
			samples = append(samples, map[string]any{"id": v.ID, "observed_at": v.ObservedAt, "metrics_at": v.MetricsAt, "cpu_milli": v.CPU, "memory_bytes": v.Memory, "cpu_limit_milli": v.CPULimit, "memory_limit_bytes": v.MemoryLimit, "ready_pods": v.ReadyPods, "total_pods": v.TotalPods, "restarts": v.Restarts})
		}
		ids = nil
		for _, v := range b.Events {
			ids = append(ids, v.ID)
		}
		existing, err = s.telemetryExisting(ctx, "telemetry_events", ids)
		if err != nil {
			return err
		}
		events := []map[string]any{}
		for _, v := range b.Events {
			if existing[v.ID] {
				continue
			}
			existing[v.ID] = true
			events = append(events, map[string]any{"id": v.ID, "timestamp": v.Timestamp, "pod_uid": v.PodUID, "pod_name": v.PodName, "container_name": v.Container, "kind": v.Kind, "reason": v.Reason, "occurrences": v.Occurrences})
		}
		state := b.State
		state.Limited = false
		state.LogsCode = telemetry.SafeCode(state.LogsCode)
		state.MetricsCode = telemetry.SafeCode(state.MetricsCode)
		if logCount+int64(len(logs)) > telemetry.MaxLogRows || logBytes+newLogBytes > telemetry.MaxLogBytes {
			state.LogsCode = telemetry.StorageLimit
			state.Checkpoints = old.Checkpoints
			state.LastLogAt = old.LastLogAt
			state.Limited = true
		} else if len(logs) > 0 {
			payload, _ := json.Marshal(logs)
			_, err = s.q(ctx).Exec(ctx, `INSERT INTO telemetry_logs(id,source_key,timestamp,pod_uid,pod_name,container_name,restart_index,level,message)
SELECT r.id,$1,r.timestamp,r.pod_uid,r.pod_name,r.container_name,r.restart_index,r.level,r.message FROM jsonb_to_recordset($2::jsonb) AS r(id text,timestamp timestamptz,pod_uid text,pod_name text,container_name text,restart_index bigint,level text,message text) ON CONFLICT(id) DO NOTHING`, old.Source.Key, payload)
			if err != nil {
				return fmt.Errorf("postgres: telemetry logs (%s)", sqlState(err))
			}
		}
		if sampleCount+int64(len(samples)) > telemetry.MaxSamples {
			state.MetricsCode = telemetry.StorageLimit
			state.LastMetricAt = old.LastMetricAt
			state.Limited = true
		} else if len(samples) > 0 {
			payload, _ := json.Marshal(samples)
			_, err = s.q(ctx).Exec(ctx, `INSERT INTO telemetry_samples(id,source_key,observed_at,metrics_at,cpu_milli,memory_bytes,cpu_limit_milli,memory_limit_bytes,ready_pods,total_pods,restarts)
SELECT r.id,$1,r.observed_at,r.metrics_at,r.cpu_milli,r.memory_bytes,r.cpu_limit_milli,r.memory_limit_bytes,r.ready_pods,r.total_pods,r.restarts FROM jsonb_to_recordset($2::jsonb) AS r(id text,observed_at timestamptz,metrics_at timestamptz,cpu_milli double precision,memory_bytes double precision,cpu_limit_milli double precision,memory_limit_bytes double precision,ready_pods bigint,total_pods bigint,restarts bigint) ON CONFLICT(id) DO NOTHING`, old.Source.Key, payload)
			if err != nil {
				return fmt.Errorf("postgres: telemetry samples (%s)", sqlState(err))
			}
		}
		if eventCount+int64(len(events)) > telemetry.MaxEvents {
			state.Limited = true
		} else if len(events) > 0 {
			payload, _ := json.Marshal(events)
			_, err = s.q(ctx).Exec(ctx, `INSERT INTO telemetry_events(id,source_key,timestamp,pod_uid,pod_name,container_name,kind,reason,occurrences)
SELECT r.id,$1,r.timestamp,r.pod_uid,r.pod_name,r.container_name,r.kind,r.reason,r.occurrences FROM jsonb_to_recordset($2::jsonb) AS r(id text,timestamp timestamptz,pod_uid text,pod_name text,container_name text,kind text,reason text,occurrences bigint) ON CONFLICT(id) DO NOTHING`, old.Source.Key, payload)
			if err != nil {
				return fmt.Errorf("postgres: telemetry events (%s)", sqlState(err))
			}
		}
		checkpoints, err := json.Marshal(state.Checkpoints)
		if err != nil {
			return errors.New("postgres: invalid telemetry checkpoints")
		}
		_, err = s.q(ctx).Exec(ctx, `UPDATE telemetry_sources SET checkpoints=$2,logs_code=$3,metrics_code=$4,observed_at=$5,last_log_at=$6,last_metric_at=$7,limited=$8,lease_owner='',lease_until=$9 WHERE source_key=$1`, old.Source.Key, checkpoints, state.LogsCode, state.MetricsCode, state.ObservedAt, state.LastLogAt, state.LastMetricAt, state.Limited, b.At)
		return translate(err)
	})
}

func (s *Store) ReadTelemetry(ctx context.Context, q telemetry.Query) (out telemetry.Data, err error) {
	out = telemetry.EmptyData()
	err = s.ReadSnapshot(ctx, func(ctx context.Context, _ persistence.Store) error {
		var exists bool
		if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environments e JOIN applications a ON a.id=e.application_id JOIN organizations o ON o.id=a.organization_id WHERE o.organization_key=$1 AND a.application_key=$2 AND e.environment_key=$3)`, q.Scope.OrganizationKey, q.Scope.ApplicationKey, q.Scope.EnvironmentKey).Scan(&exists); err != nil {
			return translate(err)
		}
		if !exists {
			return persistence.ErrNotFound
		}
		args := telemetryArgs(q)
		rows, err := s.q(ctx).Query(ctx, telemetryStateSelect+`WHERE `+telemetryScopeWhere+` ORDER BY s.workload_id,s.source_key`, args...)
		if err != nil {
			return translate(err)
		}
		for rows.Next() {
			state, err := scanTelemetryState(rows)
			if err != nil {
				rows.Close()
				return err
			}
			out.Sources = append(out.Sources, state)
		}
		rows.Close()
		if rows.Err() != nil {
			return rows.Err()
		}
		if len(out.Sources) > 128 {
			return errors.New("postgres: telemetry query needs a workload filter")
		}
		if q.Kind != "metrics" {
			if err = s.readTelemetryLogs(ctx, q, &out); err != nil {
				return err
			}
		}
		if q.Kind != "logs" {
			if err = s.readTelemetryMetrics(ctx, q, &out); err != nil {
				return err
			}
			if err = s.readTelemetryEvents(ctx, q, &out); err != nil {
				return err
			}
		}
		return nil
	})
	return
}

func (s *Store) readTelemetryLogs(ctx context.Context, q telemetry.Query, out *telemetry.Data) error {
	args := append(telemetryArgs(q), q.From, q.To, q.Level, q.Search)
	where := telemetryScopeWhere + ` AND x.timestamp >= $6 AND x.timestamp < $7 AND ($8='' OR $8='ALL' OR x.level=$8) AND ($9='' OR strpos(lower(x.message),lower($9))>0) `
	from := ` FROM telemetry_logs x ` + telemetryScopeJoins + ` WHERE ` + where
	if err := s.q(ctx).QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE x.level='ERROR')`+from, args...).Scan(&out.LogCount, &out.ErrorCount); err != nil {
		return translate(err)
	}
	beforeTime, beforeID := q.To, ""
	if q.Before != nil {
		beforeTime = q.Before.Time
		beforeID = q.Before.ID
	}
	rows, err := s.q(ctx).Query(ctx, `SELECT x.id,x.source_key,s.workload_id,x.timestamp,x.pod_uid,x.pod_name,x.container_name,x.restart_index,x.level,x.message`+from+` AND (NOT $10::boolean OR (x.timestamp,x.id)<($11::timestamptz,$12::text)) ORDER BY x.timestamp DESC,x.id DESC LIMIT $13`, append(args, q.Before != nil, beforeTime, beforeID, q.Limit+1)...)
	if err != nil {
		return translate(err)
	}
	for rows.Next() {
		var l telemetry.Log
		if err = rows.Scan(&l.ID, &l.SourceKey, &l.WorkloadID, &l.Timestamp, &l.PodUID, &l.PodName, &l.Container, &l.RestartIndex, &l.Level, &l.Message); err != nil {
			rows.Close()
			return err
		}
		out.Logs = append(out.Logs, l)
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}
	if len(out.Logs) > q.Limit {
		out.Logs = out.Logs[:q.Limit]
		out.NextCursor = telemetry.EncodeCursor(out.Logs[len(out.Logs)-1])
	}
	if q.Kind == "overview" {
		width := q.To.Sub(q.From).Seconds() / 16
		for i := 0; i < 16; i++ {
			out.Histogram = append(out.Histogram, telemetry.Bucket{At: q.From.Add(time.Duration(float64(i) * width * float64(time.Second)))})
		}
		rows, err = s.q(ctx).Query(ctx, `SELECT least(15,floor(extract(epoch FROM (x.timestamp-$6::timestamptz))/$10::double precision)::integer),count(*)`+from+` AND x.level='ERROR' GROUP BY 1`, append(args, width)...)
		if err != nil {
			return translate(err)
		}
		for rows.Next() {
			var bucket int
			var count int64
			if err = rows.Scan(&bucket, &count); err != nil {
				rows.Close()
				return err
			}
			if bucket >= 0 && bucket < 16 {
				out.Histogram[bucket].Count = count
			}
		}
		rows.Close()
		return rows.Err()
	}
	return nil
}

func scanTelemetrySample(row interface{ Scan(...any) error }) (telemetry.Sample, error) {
	var v telemetry.Sample
	err := row.Scan(&v.ID, &v.SourceKey, &v.WorkloadID, &v.ObservedAt, &v.MetricsAt, &v.CPU, &v.Memory, &v.CPULimit, &v.MemoryLimit, &v.ReadyPods, &v.TotalPods, &v.Restarts)
	return v, err
}

const sampleColumns = `x.id,x.source_key,s.workload_id,x.observed_at,x.metrics_at,x.cpu_milli,x.memory_bytes,x.cpu_limit_milli,x.memory_limit_bytes,x.ready_pods,x.total_pods,x.restarts`

func (s *Store) readTelemetryMetrics(ctx context.Context, q telemetry.Query, out *telemetry.Data) error {
	args := append(telemetryArgs(q), q.From, q.To, max(q.Step, telemetry.Interval).Seconds())
	from := ` FROM telemetry_samples x ` + telemetryScopeJoins + ` WHERE ` + telemetryScopeWhere
	rows, err := s.q(ctx).Query(ctx, `SELECT s.workload_id,to_timestamp(floor(extract(epoch FROM x.observed_at)/$8::double precision)*$8::double precision),max(x.metrics_at),CASE WHEN count(x.cpu_milli)=count(*) THEN avg(x.cpu_milli) ELSE NULL END,CASE WHEN count(x.memory_bytes)=count(*) THEN avg(x.memory_bytes) ELSE NULL END,max(x.ready_pods),max(x.total_pods),max(x.restarts)`+from+` AND x.observed_at >= $6 AND x.observed_at < $7 GROUP BY 1,2 ORDER BY 1,2`, args...)
	if err != nil {
		return translate(err)
	}
	for rows.Next() {
		var v telemetry.Sample
		if err = rows.Scan(&v.WorkloadID, &v.ObservedAt, &v.MetricsAt, &v.CPU, &v.Memory, &v.ReadyPods, &v.TotalPods, &v.Restarts); err != nil {
			rows.Close()
			return err
		}
		out.Points = append(out.Points, v)
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}
	rows, err = s.q(ctx).Query(ctx, `SELECT DISTINCT ON(s.workload_id) `+sampleColumns+from+` ORDER BY s.workload_id,x.observed_at DESC,x.id DESC`, telemetryArgs(q)...)
	if err != nil {
		return translate(err)
	}
	for rows.Next() {
		v, err := scanTelemetrySample(rows)
		if err != nil {
			rows.Close()
			return err
		}
		out.Latest = append(out.Latest, v)
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}
	peakFrom := ` FROM telemetry_samples x ` + telemetryScopeJoins + ` CROSS JOIN LATERAL (VALUES ('cpu',x.cpu_milli,x.cpu_limit_milli),('memory',x.memory_bytes,x.memory_limit_bytes)) p(metric,value,lim) WHERE ` + telemetryScopeWhere + ` AND x.observed_at >= $6 AND x.observed_at < $7 AND p.value IS NOT NULL AND p.lim > 0 `
	rows, err = s.q(ctx).Query(ctx, `SELECT DISTINCT ON(s.workload_id,p.metric) s.workload_id,p.metric,x.observed_at,p.value,p.lim,p.value/p.lim*100`+peakFrom+` ORDER BY s.workload_id,p.metric,p.value/p.lim DESC,x.observed_at DESC`, args[:7]...)
	if err != nil {
		return translate(err)
	}
	for rows.Next() {
		var p telemetry.Peak
		if err = rows.Scan(&p.WorkloadID, &p.Metric, &p.Timestamp, &p.Value, &p.Limit, &p.Percent); err != nil {
			rows.Close()
			return err
		}
		out.Peaks = append(out.Peaks, p)
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}
	usageFrom := ` FROM telemetry_samples x ` + telemetryScopeJoins + ` CROSS JOIN LATERAL (VALUES ('cpu',x.cpu_milli),('memory',x.memory_bytes)) p(metric,value) WHERE ` + telemetryScopeWhere + ` AND x.observed_at >= $6 AND x.observed_at < $7 AND p.value IS NOT NULL `
	rows, err = s.q(ctx).Query(ctx, `SELECT DISTINCT ON(s.workload_id,p.metric) s.workload_id,p.metric,x.observed_at,p.value`+usageFrom+` ORDER BY s.workload_id,p.metric,p.value DESC,x.observed_at DESC`, args[:7]...)
	if err != nil {
		return translate(err)
	}
	for rows.Next() {
		var p telemetry.UsagePeak
		if err = rows.Scan(&p.WorkloadID, &p.Metric, &p.Timestamp, &p.Value); err != nil {
			rows.Close()
			return err
		}
		out.UsagePeaks = append(out.UsagePeaks, p)
	}
	rows.Close()
	return rows.Err()
}

func (s *Store) readTelemetryEvents(ctx context.Context, q telemetry.Query, out *telemetry.Data) error {
	args := append(telemetryArgs(q), q.From, q.To)
	from := ` FROM telemetry_events x ` + telemetryScopeJoins + ` WHERE ` + telemetryScopeWhere + ` AND x.timestamp >= $6 AND x.timestamp < $7 `
	if err := s.q(ctx).QueryRow(ctx, `SELECT count(*)`+from, args...).Scan(&out.EventCount); err != nil {
		return translate(err)
	}
	rows, err := s.q(ctx).Query(ctx, `SELECT x.id,x.source_key,s.workload_id,x.timestamp,x.pod_uid,x.pod_name,x.container_name,x.kind,x.reason,x.occurrences`+from+` ORDER BY x.timestamp DESC,x.id DESC LIMIT 200`, args...)
	if err != nil {
		return translate(err)
	}
	defer rows.Close()
	for rows.Next() {
		var e telemetry.Event
		if err = rows.Scan(&e.ID, &e.SourceKey, &e.WorkloadID, &e.Timestamp, &e.PodUID, &e.PodName, &e.Container, &e.Kind, &e.Reason, &e.Occurrences); err != nil {
			return err
		}
		out.Events = append(out.Events, e)
	}
	return rows.Err()
}

func (s *Store) PruneTelemetry(ctx context.Context, now time.Time) error {
	return s.Transact(ctx, func(ctx context.Context) error {
		for _, v := range []struct {
			table, column string
			cut           time.Time
		}{{"telemetry_logs", "timestamp", now.Add(-telemetry.LogRetention)}, {"telemetry_samples", "observed_at", now.Add(-telemetry.MetricRetention)}, {"telemetry_events", "timestamp", now.Add(-telemetry.MetricRetention)}} {
			if _, err := s.q(ctx).Exec(ctx, `DELETE FROM `+v.table+` WHERE `+v.column+` < $1`, v.cut); err != nil {
				return fmt.Errorf("postgres: expire telemetry (%s)", sqlState(err))
			}
		}
		return nil
	})
}

var _ persistence.TelemetryRepository = (*Store)(nil)
