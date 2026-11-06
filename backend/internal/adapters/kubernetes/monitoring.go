package kubernetes

import (
	"context"
	"encoding/json"
	"net/url"
	"orchestrator/internal/domain/monitoring"
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
