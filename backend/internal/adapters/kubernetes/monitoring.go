package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"orchestrator/internal/domain/monitoring"
	"orchestrator/internal/ports/execution"
	obs "orchestrator/internal/ports/observability"
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
