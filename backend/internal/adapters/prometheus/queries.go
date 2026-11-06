package prometheus

import (
	"fmt"
	"orchestrator/internal/domain/monitoring"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WorkloadQueries builds only server-defined selectors from verified targets.
// kube-state-metrics identifies Deployment pods through their ReplicaSet owner.
func WorkloadQueries(t monitoring.Target) (map[string]string, error) {
	if t.Validate() != nil {
		return nil, monitoring.ErrInvalid
	}
	return workloadQueries(t, true)
}
func workloadQueries(t monitoring.Target, strictPods bool) (map[string]string, error) {
	if t.ValidateDesired() != nil {
		return nil, monitoring.ErrInvalid
	}
	ns := strconv.Quote(t.Scope.Namespace)
	name := strconv.Quote(t.Name)
	base := `cluster_uid=` + strconv.Quote(t.Scope.ClusterUID) + `,namespace=` + ns
	var pods, desired, ready, stable string
	if t.Kind == "Deployment" {
		// Rename the Deployment owner label before joining ReplicaSet names.
		pods = `max by(namespace,pod,uid) (label_replace(kube_pod_owner{` + base + `,owner_kind="ReplicaSet",owner_is_controller="true"},"replicaset","$1","owner_name","(.*)") * on(namespace,replicaset) group_left() kube_replicaset_owner{` + base + `,owner_kind="Deployment",owner_name=` + name + `,owner_is_controller="true"})`
		desired = `kube_deployment_spec_replicas{` + base + `,deployment=` + name + `}`
		ready = `kube_deployment_status_replicas_available{` + base + `,deployment=` + name + `}`
		stable = `(kube_deployment_status_observed_generation{` + base + `,deployment=` + name + `} >= kube_deployment_metadata_generation{` + base + `,deployment=` + name + `})`
	} else {
		pods = `max by(namespace,pod,uid) (kube_pod_owner{` + base + `,owner_kind="StatefulSet",owner_name=` + name + `,owner_is_controller="true"})`
		desired = `kube_statefulset_replicas{` + base + `,statefulset=` + name + `}`
		ready = `kube_statefulset_status_replicas_ready{` + base + `,statefulset=` + name + `}`
		stable = `(kube_statefulset_status_observed_generation{` + base + `,statefulset=` + name + `} >= kube_statefulset_metadata_generation{` + base + `,statefulset=` + name + `})`
	}
	cadvisorBase := base
	if strictPods {
		uids := []string{}
		cgroupUIDs := []string{}
		for _, uid := range t.PodUIDs {
			if !regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`).MatchString(uid) {
				return nil, monitoring.ErrInvalid
			}
			uids = append(uids, regexp.QuoteMeta(uid))
			cgroupUIDs = append(cgroupUIDs, strings.ReplaceAll(uid, "-", "[-_]"))
		}
		uidRegex, idRegex, containerRegex := "$^", "$^", "$^"
		if len(uids) > 0 {
			uidRegex = strings.Join(uids, "|")
			idRegex = ".*pod(" + strings.Join(cgroupUIDs, "|") + ").*"
		}
		containers := []string{}
		for _, name := range t.Containers {
			containers = append(containers, regexp.QuoteMeta(name))
		}
		if len(containers) > 0 {
			containerRegex = strings.Join(containers, "|")
		}
		pods = "(" + pods + ") * on(namespace,pod,uid) group_left() max by(namespace,pod,uid) (kube_pod_info{" + base + ",uid=~" + strconv.Quote(uidRegex) + "})"
		cadvisorBase += ",id=~" + strconv.Quote(idRegex) + ",container=~" + strconv.Quote(containerRegex)
	}
	// UID joins keep terminated pod generations out of kube-state-metrics facts.
	membership := ` * on(namespace,pod,uid) group_left() (` + pods + `)`
	// cAdvisor has no Kubernetes UID label; app service must verify current pod UID
	// and mark historical pod-name reuse as partial rather than merge generations.
	cadvisorMembership := ` * on(namespace,pod) group_left() (max by(namespace,pod) (` + pods + `))`
	return map[string]string{
		"pods":            pods,
		"desired":         desired,
		"ready":           ready,
		"unavailable":     `((` + desired + ` > 0) and (` + ready + ` < ` + desired + `)) and ` + stable,
		"cpu":             `sum(rate(container_cpu_usage_seconds_total{` + cadvisorBase + `,container!="",container!="POD"}[5m])` + cadvisorMembership + `) * 1000`,
		"sourceTimestamp": `min(timestamp(container_memory_working_set_bytes{` + cadvisorBase + `,container!="",container!="POD"})` + cadvisorMembership + `)`,
		"memory":          `sum(container_memory_working_set_bytes{` + cadvisorBase + `,container!="",container!="POD"}` + cadvisorMembership + `)`,
		"restarts":        `sum(increase(kube_pod_container_status_restarts_total{` + base + `}[5m])` + membership + `)`,
		"oom":             `sum(kube_pod_container_status_last_terminated_reason{` + base + `,reason="OOMKilled"}` + membership + `)`,
	}, nil
}

// RuleDocument returns a native manifest for the existing Git/Fleet delivery path.
// It does not write to either Kubernetes or the Prometheus query API.
func RuleDocument(t monitoring.Target, c monitoring.RulesConfig) (map[string]any, error) {
	if c.Validate() != nil {
		return nil, monitoring.ErrInvalid
	}
	q, e := workloadQueries(t, false)
	if e != nil {
		return nil, e
	}
	labels := map[string]string{"orchestrator_org": t.Scope.Organization, "orchestrator_app": t.Scope.Application, "orchestrator_env": t.Scope.Environment, "orchestrator_generation": strconv.FormatInt(t.Scope.Generation, 10), "orchestrator_cluster_uid": t.Scope.ClusterUID, "orchestrator_workload": t.Workload, "severity": c.Severity}
	for k, v := range c.NotificationLabels {
		labels[k] = v
	}
	name := monitoring.RuleName(t)
	rule := func(alert, expr string, d time.Duration, summary string) map[string]any {
		return map[string]any{"alert": alert, "expr": expr, "for": fmt.Sprintf("%ds", int64(d/time.Second)), "labels": labels, "annotations": map[string]string{"summary": summary, "configuration_source": c.Provenance}}
	}
	restart := strings.Replace(q["restarts"], "[5m]", fmt.Sprintf("[%ds]", int64(c.RestartWindow/time.Second)), 1)
	return map[string]any{"apiVersion": "monitoring.coreos.com/v1", "kind": "PrometheusRule", "metadata": map[string]any{"name": name, "namespace": t.Scope.Namespace, "labels": map[string]string{"release": "orch-observability", "orchestrator.dev/monitoring-owner": name}}, "spec": map[string]any{"groups": []any{map[string]any{"name": name, "rules": []any{
		rule("OrchestratorWorkloadUnavailable", q["unavailable"], c.AvailabilityFor, "Workload has fewer available replicas than desired"),
		rule("OrchestratorWorkloadRestarts", restart+" >= "+strconv.Itoa(c.RestartThreshold), c.RestartFor, "Workload restart increase exceeds configured threshold"),
		rule("OrchestratorWorkloadOOM", q["oom"]+" > 0", c.OOMFor, "OOMKilled last-termination observation; incomplete event history"),
	}}}}}, nil
}
