package runner

import (
	"encoding/json"
	"fmt"
	"sort"

	"orchestrator/internal/ports/terraformexec"
)

type planDocument struct {
	ResourceChanges []planChange `json:"resource_changes"`
	ResourceDrift   []planChange `json:"resource_drift"`
	OutputChanges   map[string]struct {
		Actions []string `json:"actions"`
	} `json:"output_changes"`
}

type planChange struct {
	Address      string `json:"address"`
	Mode         string `json:"mode"`
	Type         string `json:"type"`
	ProviderName string `json:"provider_name"`
	Change       struct {
		Actions []string `json:"actions"`
	} `json:"change"`
}

// parsePlan reads `terraform show -json <plan>`. Only addresses, types,
// providers and actions are kept; values never leave the document.
func parsePlan(data []byte) (terraformexec.Plan, error) {
	var doc planDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return terraformexec.Plan{}, fmt.Errorf("plan review is not valid JSON")
	}
	out := terraformexec.Plan{}
	for _, c := range doc.ResourceChanges {
		if c.Mode != "" && c.Mode != "managed" {
			continue
		}
		out.Changes = append(out.Changes, terraformexec.ResourceChange{Address: c.Address, Type: c.Type, Provider: c.ProviderName, Actions: append([]string(nil), c.Change.Actions...)})
	}
	for _, c := range doc.ResourceDrift {
		for _, a := range c.Change.Actions {
			if a == "delete" {
				out.Drift = append(out.Drift, c.Address)
				break
			}
		}
	}
	for _, change := range doc.OutputChanges {
		for _, a := range change.Actions {
			if a != "no-op" {
				out.OutputsChanged = true
			}
		}
	}
	sort.Strings(out.Drift)
	return out, nil
}

type stateDocument struct {
	Values struct {
		RootModule stateModule `json:"root_module"`
	} `json:"values"`
}

type stateModule struct {
	Resources    []stateResource `json:"resources"`
	ChildModules []stateModule   `json:"child_modules"`
}

type stateResource struct {
	Address      string         `json:"address"`
	Mode         string         `json:"mode"`
	Type         string         `json:"type"`
	ProviderName string         `json:"provider_name"`
	Values       map[string]any `json:"values"`
}

// parseStateResources reads `terraform show -json` of a state.
func parseStateResources(data []byte) ([]terraformexec.StateResource, error) {
	var doc stateDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("state listing is not valid JSON")
	}
	var out []terraformexec.StateResource
	var walk func(m stateModule)
	walk = func(m stateModule) {
		for _, r := range m.Resources {
			if r.Mode != "managed" {
				continue
			}
			id, _ := r.Values["id"].(string)
			out = append(out, terraformexec.StateResource{Address: r.Address, Type: r.Type, Provider: r.ProviderName, ID: id, UID: uidOf(r.Values)})
		}
		for _, c := range m.ChildModules {
			walk(c)
		}
	}
	walk(doc.Values.RootModule)
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out, nil
}

type outputDocument map[string]struct {
	Sensitive bool `json:"sensitive"`
	Type      any  `json:"type"`
	Value     any  `json:"value"`
}

func parseOutputs(data []byte) (map[string]terraformexec.Output, error) {
	var doc outputDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("outputs are not valid JSON")
	}
	out := make(map[string]terraformexec.Output, len(doc))
	for name, o := range doc {
		out[name] = terraformexec.Output{Value: o.Value, Type: o.Type, Sensitive: o.Sensitive}
	}
	return out, nil
}

// uidOf reads metadata.uid of a Kubernetes-style resource value: metadata is a
// one-element list in the Terraform provider schema (or an object).
func uidOf(values map[string]any) string {
	switch meta := values["metadata"].(type) {
	case []any:
		if len(meta) == 1 {
			if m, ok := meta[0].(map[string]any); ok {
				uid, _ := m["uid"].(string)
				return uid
			}
		}
	case map[string]any:
		uid, _ := meta["uid"].(string)
		return uid
	}
	return ""
}
