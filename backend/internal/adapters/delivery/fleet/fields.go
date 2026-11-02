package fleet

import port "orchestrator/internal/ports/delivery"

// Only platform policy is authored. Concrete app URLs/branches are allocated
// internally and never appear in Application settings.
func (p *Provider) ConfigurationFields() []port.ConfigurationField {
	fields := []port.ConfigurationField{}
	add := func(key, kind, value string, required, advanced bool) {
		fields = append(fields, port.ConfigurationField{Key: key, Kind: kind, Default: value, Required: required, Advanced: advanced, LabelKey: "cd.fields." + key})
	}
	add("repositoryOwner", "text", "", true, false)
	add("targetAccountKey", "account", "", true, true)
	add("workspace", "text", "fleet-local", true, true)
	add("targetCluster", "text", "local", true, true)
	add("targetClusterUID", "text", "", true, true)
	fields[len(fields)-1].ReadOnly = true
	add("controllerNamespace", "text", "cattle-fleet-system", true, true)
	add("controllerName", "text", "fleet-controller", true, true)
	add("agentNamespace", "text", "cattle-fleet-local-system", true, true)
	add("serviceAccount", "text", "orchestrator-delivery", true, true)
	add("releaseNamespace", "text", "fleet-releases", true, true)
	add("readSecret", "text", "orchestrator-git-reader", true, true)
	add("caBundle", "textarea", "", false, true)
	add("repositoryAPI", "url", "https://api.github.com", true, true)
	add("repositoryHost", "url", "https://github.com", true, true)
	add("repositoryPrefix", "text", "orch-app-", true, true)
	add("root", "text", "environments", true, true)
	return fields
}
