package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"orchestrator/internal/domain/monitoring"
	"orchestrator/internal/ports/execution"
	obs "orchestrator/internal/ports/observability"
	"sort"
)

func (r *TelemetryReader) MonitoringTarget(ctx context.Context, source obs.Source) (monitoring.Target, error) {
	var out monitoring.Target
	if !source.Source.Valid() || source.Target.Organization != source.OrganizationKey || !source.Target.CredentialBacked() {
		return out, ErrCredentialTarget
	}
	cli, cleanup, e := OpenCLI(ctx, r.KubectlPath, r.Credentials, source.Target)
	if e != nil {
		return out, e
	}
	defer cleanup()
	raw, e := cli.RunBounded(ctx, 1<<20, "get", "--raw", "/api/v1/namespaces/kube-system", "--request-timeout=8s")
	if e != nil {
		return out, e
	}
	var ns struct {
		Metadata telemetryMeta `json:"metadata"`
	}
	if json.Unmarshal(raw, &ns) != nil || ns.Metadata.UID == "" {
		return out, ErrCredentialTarget
	}
	// Controller names come from persisted deployment inventory; observe the live UID after ownership checks.
	if source.MonitoringController == nil || (source.MonitoringController.Kind != "Deployment" && source.MonitoringController.Kind != "StatefulSet") {
		return out, ErrCredentialTarget
	}
	kind := "deployments"
	if source.MonitoringController.Kind == "StatefulSet" {
		kind = "statefulsets"
	}
	path := "/apis/apps/v1/namespaces/" + url.PathEscape(source.Target.Namespace) + "/" + kind + "/" + url.PathEscape(source.MonitoringController.Name)
	raw, e = cli.RunBounded(ctx, 1<<20, "get", "--raw", path, "--request-timeout=8s")
	if e != nil {
		return out, e
	}
	var object struct {
		Metadata telemetryMeta `json:"metadata"`
	}
	if json.Unmarshal(raw, &object) != nil || object.Metadata.UID == "" || object.Metadata.Labels["orchestrator.io/application"] != source.ApplicationKey || object.Metadata.Labels["orchestrator.io/environment"] != source.EnvironmentKey || object.Metadata.Labels["app.kubernetes.io/name"] != source.WorkloadID || (object.Metadata.Labels["app.kubernetes.io/managed-by"] != "orchestrator" && object.Metadata.Labels["orchestrator.io/managed-by"] != "orchestrator") {
		return out, ErrCredentialTarget
	}
	owner := object.Metadata.Labels["orchestrator.io/deployment-id"]
	validOwner := owner != "" && owner == source.DeploymentID
	for _, expected := range source.MonitoringController.Owners {
		validOwner = validOwner || (expected != "" && owner == expected)
	}
	if !validOwner {
		return out, ErrCredentialTarget
	}
	out = monitoring.Target{Scope: monitoring.Scope{Organization: source.OrganizationKey, Application: source.ApplicationKey, Environment: source.EnvironmentKey, Generation: source.Generation, ClusterUID: ns.Metadata.UID, Namespace: source.Target.Namespace}, Workload: source.WorkloadID, Kind: source.MonitoringController.Kind, Name: source.MonitoringController.Name, UID: object.Metadata.UID}
	selector := "app.kubernetes.io/name=" + source.WorkloadID + ",orchestrator.io/application=" + source.ApplicationKey + ",orchestrator.io/environment=" + source.EnvironmentKey
	query := url.Values{"labelSelector": {selector}, "limit": {"64"}}
	owners := map[string]bool{object.Metadata.UID: true}
	if out.Kind == "Deployment" {
		rsRaw, err := cli.RunBounded(ctx, 2<<20, "get", "--raw", "/apis/apps/v1/namespaces/"+url.PathEscape(source.Target.Namespace)+"/replicasets?"+query.Encode(), "--request-timeout=8s")
		if err != nil {
			return out, err
		}
		var sets struct {
			Metadata struct {
				Continue string `json:"continue"`
			} `json:"metadata"`
			Items []struct {
				Metadata telemetryMeta `json:"metadata"`
			} `json:"items"`
		}
		if json.Unmarshal(rsRaw, &sets) != nil || sets.Metadata.Continue != "" || len(sets.Items) > 64 {
			return out, ErrCredentialTarget
		}
		owners = map[string]bool{}
		for _, rs := range sets.Items {
			for _, owner := range rs.Metadata.Owners {
				if owner.Controller && owner.Kind == "Deployment" && owner.UID == object.Metadata.UID {
					owners[rs.Metadata.UID] = true
				}
			}
		}
	}
	podsRaw, err := cli.RunBounded(ctx, 2<<20, "get", "--raw", "/api/v1/namespaces/"+url.PathEscape(source.Target.Namespace)+"/pods?"+query.Encode(), "--request-timeout=8s")
	if err != nil {
		return out, err
	}
	var pods struct {
		Metadata struct {
			Continue string `json:"continue"`
		} `json:"metadata"`
		Items []telemetryPod `json:"items"`
	}
	if json.Unmarshal(podsRaw, &pods) != nil || pods.Metadata.Continue != "" || len(pods.Items) > 64 {
		return out, ErrCredentialTarget
	}
	cpu, memory := 0.0, 0.0
	completeCPU, completeMemory := true, true
	names := map[string]bool{}
	out.PodUIDs = []string{}
	for _, pod := range pods.Items {
		owned := false
		for _, owner := range pod.Metadata.Owners {
			if owner.Controller && owners[owner.UID] {
				owned = true
			}
		}
		if !owned || pod.Metadata.UID == "" {
			continue
		}
		out.PodUIDs = append(out.PodUIDs, pod.Metadata.UID)
		for _, container := range pod.Spec.Containers {
			names[container.Name] = true
			v, ok := quantity(container.Resources.Limits["cpu"], true)
			completeCPU = completeCPU && ok
			cpu += v
			v, ok = quantity(container.Resources.Limits["memory"], false)
			completeMemory = completeMemory && ok
			memory += v
		}
	}
	sort.Strings(out.PodUIDs)
	for name := range names {
		out.Containers = append(out.Containers, name)
	}
	sort.Strings(out.Containers)
	if len(out.PodUIDs) > 0 && completeCPU {
		out.CPULimitMilli = &cpu
	}
	if len(out.PodUIDs) > 0 && completeMemory {
		out.MemoryLimitBytes = &memory
	}
	return out, out.Validate()
}

func (r *TelemetryReader) MonitoringCluster(ctx context.Context, target execution.Target) (string, error) {
	if target.Organization == "" || !target.CredentialBacked() {
		return "", ErrCredentialTarget
	}
	cli, cleanup, e := OpenCLI(ctx, r.KubectlPath, r.Credentials, target)
	if e != nil {
		return "", e
	}
	defer cleanup()
	raw, e := cli.RunBounded(ctx, 1<<20, "get", "--raw", "/api/v1/namespaces/kube-system", "--request-timeout=8s")
	if e != nil {
		return "", e
	}
	var ns struct {
		Metadata telemetryMeta `json:"metadata"`
	}
	if json.Unmarshal(raw, &ns) != nil || ns.Metadata.UID == "" {
		return "", ErrCredentialTarget
	}
	return ns.Metadata.UID, nil
}

func (r *TelemetryReader) MonitoringConfiguration(ctx context.Context, source obs.Source, desired []execution.Manifest) (string, error) {
	t, e := r.MonitoringTarget(ctx, source)
	if e != nil {
		return "UNAVAILABLE", e
	}
	cli, cleanup, e := OpenCLI(ctx, r.KubectlPath, r.Credentials, source.Target)
	if e != nil {
		return "UNAVAILABLE", e
	}
	defer cleanup()
	expected := map[string]execution.Manifest{}
	for _, m := range desired {
		expected[m.Kind] = m
	}
	state := "APPLIED"
	for _, kind := range []string{"PrometheusRule", "PodMonitor"} {
		resource := "prometheusrules"
		name := monitoring.RuleName(t)
		if kind == "PodMonitor" {
			resource = "podmonitors"
			name += "-scrape"
		}
		path := "/apis/monitoring.coreos.com/v1/namespaces/" + url.PathEscape(t.Scope.Namespace) + "/" + resource + "/" + url.PathEscape(name)
		raw, e := cli.RunBounded(ctx, 1<<20, "get", "--raw", path, "--request-timeout=8s")
		wanted, ok := expected[kind]
		if errors.Is(e, ErrObjectNotFound) {
			if ok {
				state = "PENDING"
			}
			continue
		}
		if e != nil {
			return "UNAVAILABLE", e
		}
		var object map[string]any
		if json.Unmarshal(raw, &object) != nil {
			return "UNAVAILABLE", ErrCredentialTarget
		}
		md, _ := object["metadata"].(map[string]any)
		labels, _ := md["labels"].(map[string]any)
		if (labels["app.kubernetes.io/managed-by"] != "orchestrator" && labels["orchestrator.io/managed-by"] != "orchestrator") || labels["orchestrator.io/application"] != t.Scope.Application || labels["orchestrator.io/environment"] != t.Scope.Environment || labels["app.kubernetes.io/name"] != t.Workload {
			return "DRIFT", nil
		}
		if !ok || !containsConfiguration(object["spec"], wanted.Object["spec"]) {
			return "DRIFT", nil
		}
	}
	return state, nil
}

// Operator defaults may add fields; every desired value must still match.
func containsConfiguration(actual, expected any) bool {
	switch want := expected.(type) {
	case map[string]any:
		got, ok := actual.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range want {
			if !containsConfiguration(got[k], v) {
				return false
			}
		}
		return true
	case []any:
		got, ok := actual.([]any)
		if !ok || len(got) != len(want) {
			return false
		}
		for i, v := range want {
			if !containsConfiguration(got[i], v) {
				return false
			}
		}
		return true
	default:
		a, _ := json.Marshal(actual)
		b, _ := json.Marshal(expected)
		return string(a) == string(b)
	}
}
