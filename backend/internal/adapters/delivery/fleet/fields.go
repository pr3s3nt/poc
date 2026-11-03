package fleet

import port "orchestrator/internal/ports/delivery"

// Only four user-facing fields remain (ADR-034, issue #31): the Kubernetes
// Connection, the Git Connection and the auto-discovered Fleet Workspace and
// Target Cluster. Everything else is derived by the Platform and pinned into
// the immutable binding version at registration.
func (p *Provider) ConfigurationFields() []port.ConfigurationField {
	fields := []port.ConfigurationField{}
	add := func(key, kind, value string, required, advanced bool) {
		fields = append(fields, port.ConfigurationField{Key: key, Kind: kind, Default: value, Required: required, Advanced: advanced, LabelKey: "cd.fields." + key})
	}
	add("targetAccountKey", "account", "", true, false)
	add("gitConnectionKey", "gitConnection", "", false, false)
	add("workspace", "text", "fleet-local", true, false)
	add("targetCluster", "text", "local", true, false)
	return fields
}

// DefaultConfiguration derives the internal topology the simplified form never
// asks about. Discovery-resolved or explicitly submitted values win.
func (p *Provider) DefaultConfiguration() map[string]string {
	return map[string]string{
		"controllerNamespace": "cattle-fleet-system",
		"controllerName":      "fleet-controller",
		"agentNamespace":      "cattle-fleet-local-system",
		"serviceAccount":      "orchestrator-delivery",
		"releaseNamespace":    "fleet-releases",
		"readSecret":          "",
		"repositoryAPI":       "https://api.github.com",
		"repositoryHost":      "https://github.com",
		"repositoryPrefix":    "orch-app-",
		"root":                "environments",
	}
}
