package fleet

import port "orchestrator/internal/ports/delivery"

func (p *Provider) ConfigurationFields() []port.ConfigurationField {
	fields := []port.ConfigurationField{}
	add := func(key, kind, value string, required, advanced bool) {
		fields = append(fields, port.ConfigurationField{Key: key, Kind: kind, Default: value, Required: required, Advanced: advanced, LabelKey: "cd.fields." + key})
	}
	add("gitRepoMode", "choice", "MANAGED", true, false)
	fields[len(fields)-1].Choices = []port.ConfigurationChoice{{Value: "MANAGED", LabelKey: "cd.managedRepo"}, {Value: "EXISTING", LabelKey: "cd.existingRepo"}}
	add("existingGitRepo", "text", "", true, false)
	fields[len(fields)-1].VisibleWhenKey = "gitRepoMode"
	fields[len(fields)-1].VisibleWhenValue = "EXISTING"
	add("existingGitRepoUID", "text", "", true, true)
	fields[len(fields)-1].VisibleWhenKey = "gitRepoMode"
	fields[len(fields)-1].VisibleWhenValue = "EXISTING"
	fields[len(fields)-1].ReadOnly = true
	add("targetAccountKey", "account", "", true, false)
	add("workspace", "text", "fleet-local", true, false)
	add("targetCluster", "text", "local", true, false)
	add("targetClusterUID", "text", "", true, true)
	fields[len(fields)-1].ReadOnly = true
	add("repositoryURL", "url", "", true, false)
	add("branch", "text", "dev", true, false)
	add("root", "text", "generated", true, false)
	add("controllerNamespace", "text", "cattle-fleet-system", true, true)
	add("controllerName", "text", "fleet-controller", true, true)
	add("agentNamespace", "text", "cattle-fleet-local-system", true, true)
	add("serviceAccount", "text", "", true, true)
	add("releaseNamespace", "text", "fleet-releases", true, true)
	add("readSecret", "text", "", false, true)
	add("caBundle", "textarea", "", false, true)
	return fields
}
