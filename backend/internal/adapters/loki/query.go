// Package loki implements bounded, server-scoped retained log reads.
package loki

import (
	"context"
	"encoding/json"
	"net/url"
	"orchestrator/internal/adapters/prometheus"
	"orchestrator/internal/domain/monitoring"
	"orchestrator/internal/domain/telemetry"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Transport interface {
	ReadJSON(context.Context, string, url.Values, any) error
}
type Query struct {
	Scope                              monitoring.Scope
	Namespaces                         []string
	Workload, Container, Search, Level string
	From, To                           time.Time
	Limit                              int
	Before                             *telemetry.Position
}
type Page struct {
	Logs       []telemetry.Log
	NextCursor string
	Truncated  bool
}

func Selector(q Query) string {
	pairs := []string{"cluster_uid=" + strconv.Quote(q.Scope.ClusterUID), "organization=" + strconv.Quote(q.Scope.Organization), "application=" + strconv.Quote(q.Scope.Application), "environment=" + strconv.Quote(q.Scope.Environment), "generation=" + strconv.Quote(strconv.FormatInt(q.Scope.Generation, 10))}
	ns := append([]string{}, q.Namespaces...)
	sort.Strings(ns)
	for i := range ns {
		ns[i] = regexp.QuoteMeta(ns[i])
	}
	// Namespace names are validated DNS names by the application inventory.
	pairs = append(pairs, "namespace=~"+strconv.Quote(strings.Join(ns, "|")))
	if q.Workload != "" {
		pairs = append(pairs, "workload="+strconv.Quote(q.Workload))
	}
	if q.Container != "" {
		pairs = append(pairs, "container="+strconv.Quote(q.Container))
	}
	if q.Level != "" && q.Level != "ALL" {
		pairs = append(pairs, "level="+strconv.Quote(q.Level))
	}
	selector := "{" + strings.Join(pairs, ",") + "}"
	if q.Search != "" {
		selector += " |~ " + strconv.Quote("(?i)"+regexp.QuoteMeta(q.Search))
	}
	return selector
}
func Read(ctx context.Context, c Transport, q Query) (Page, error) {
	out := Page{Logs: []telemetry.Log{}}
	if len(q.Namespaces) == 0 || len(q.Namespaces) > 32 || q.Limit < 1 || q.Limit > 200 || !q.From.Before(q.To) || q.To.Sub(q.From) > 7*24*time.Hour {
		return out, prometheus.ErrBounds
	}
	end := q.To
	if q.Before != nil {
		end = q.Before.Time.Add(time.Nanosecond)
	}
	var envelope struct {
		Status   string   `json:"status"`
		Warnings []string `json:"warnings"`
		Data     struct {
			Type   string `json:"resultType"`
			Result []struct {
				Stream map[string]string   `json:"stream"`
				Values [][]json.RawMessage `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	err := c.ReadJSON(ctx, "/loki/api/v1/query_range", url.Values{"query": {Selector(q)}, "start": {strconv.FormatInt(q.From.UnixNano(), 10)}, "end": {strconv.FormatInt(end.UnixNano(), 10)}, "limit": {"2000"}, "direction": {"backward"}}, &envelope)
	if err != nil {
		return out, err
	}
	if envelope.Status != "success" || len(envelope.Warnings) > 0 || envelope.Data.Type != "streams" || len(envelope.Data.Result) > 2000 {
		return out, prometheus.ErrResponse
	}
	rawCount := 0
	seen := map[string]bool{}
	namespaces := map[string]bool{}
	for _, ns := range q.Namespaces {
		namespaces[ns] = true
	}
	for _, stream := range envelope.Data.Result {
		l := stream.Stream
		if len(l) > 40 || l["cluster_uid"] != q.Scope.ClusterUID || l["organization"] != q.Scope.Organization || l["application"] != q.Scope.Application || l["environment"] != q.Scope.Environment || l["generation"] != strconv.FormatInt(q.Scope.Generation, 10) || !namespaces[l["namespace"]] || l["workload"] == "" || (q.Workload != "" && l["workload"] != q.Workload) || (q.Container != "" && l["container"] != q.Container) {
			return out, prometheus.ErrResponse
		}
		for _, pair := range stream.Values {
			rawCount++
			if rawCount > 2000 || len(pair) < 2 || len(pair) > 3 {
				return out, prometheus.ErrResponse
			}
			var stamp, line string
			if json.Unmarshal(pair[0], &stamp) != nil || json.Unmarshal(pair[1], &line) != nil || len(line) > telemetry.MaxMessage {
				return out, prometheus.ErrResponse
			}
			n, err := strconv.ParseInt(stamp, 10, 64)
			if err != nil {
				return out, prometheus.ErrResponse
			}
			at := time.Unix(0, n).UTC()
			if at.Before(q.From) || !at.Before(end) {
				return out, prometheus.ErrResponse
			}
			uid := l["pod_uid"]
			if len(pair) == 3 {
				var metadata map[string]string
				if json.Unmarshal(pair[2], &metadata) != nil {
					return out, prometheus.ErrResponse
				}
				uid = metadata["pod_uid"]
			}
			level := telemetry.Level(line)
			if q.Level != "" && q.Level != "ALL" && l["level"] != q.Level {
				return out, prometheus.ErrResponse
			}
			log := telemetry.Log{ID: telemetry.Hash(stamp, l["namespace"], l["pod"], uid, l["container"], line), WorkloadID: l["workload"], Timestamp: at, PodUID: uid, PodName: l["pod"], Container: l["container"], Level: level, Message: line}
			if telemetry.Older(log, q.Before) && !seen[log.ID] {
				out.Logs = append(out.Logs, log)
				seen[log.ID] = true
			}
		}
	}
	sort.Slice(out.Logs, func(i, j int) bool {
		if out.Logs[i].Timestamp.Equal(out.Logs[j].Timestamp) {
			return out.Logs[i].ID > out.Logs[j].ID
		}
		return out.Logs[i].Timestamp.After(out.Logs[j].Timestamp)
	})
	if len(out.Logs) > q.Limit {
		out.Logs = out.Logs[:q.Limit]
		out.NextCursor = telemetry.EncodeCursor(out.Logs[len(out.Logs)-1])
	} else if rawCount == 2000 {
		out.Truncated = true
	}
	return out, nil
}
