package artifact

import (
	"encoding/json"
	"sort"
	"strings"

	"orchestrator/internal/ports/execution"
)

// Export removes only manager metadata from an independent copy. In particular
// Pod templates, business labels, spec, data and literal strings are preserved.
func Export(objects []map[string]any) ([]execution.Manifest, error) {
	out := []execution.Manifest{}
	seen := map[string]bool{}
	for _, source := range objects {
		raw, err := json.Marshal(source)
		if err != nil {
			return nil, ErrInvalid
		}
		var obj map[string]any
		if json.Unmarshal(raw, &obj) != nil {
			return nil, ErrInvalid
		}
		md, _ := obj["metadata"].(map[string]any)
		api, _ := obj["apiVersion"].(string)
		kind, _ := obj["kind"].(string)
		name, _ := md["name"].(string)
		ns, _ := md["namespace"].(string)
		if api == "" || kind == "" || name == "" || ns == "" || kind == "Secret" || strings.HasPrefix(api, "secrets.hashicorp.com/") {
			return nil, ErrInvalid
		}
		a, _ := md["annotations"].(map[string]any)
		for _, key := range []string{"helm.sh/resource-policy", "meta.helm.sh/release-name", "meta.helm.sh/release-namespace", "orchestrator.io/delivery-revision"} {
			delete(a, key)
		}
		if len(a) == 0 {
			delete(md, "annotations")
		}
		labels, _ := md["labels"].(map[string]any)
		if labels["app.kubernetes.io/managed-by"] == "Helm" {
			if labels["orchestrator.io/managed-by"] != "orchestrator" {
				return nil, ErrInvalid
			}
			labels["app.kubernetes.io/managed-by"] = "orchestrator"
		}
		if labels["app.kubernetes.io/managed-by"] != "orchestrator" {
			return nil, ErrInvalid
		}
		delete(labels, "orchestrator.io/managed-by")
		ref := execution.ObjectRef{APIVersion: api, Kind: kind, Name: name}
		if seen[ref.Key()] {
			return nil, ErrInvalid
		}
		seen[ref.Key()] = true
		out = append(out, execution.Manifest{APIVersion: api, Kind: kind, Name: name, Namespace: ns, Object: obj})
	}
	if len(out) == 0 {
		return nil, ErrInvalid
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].APIVersion+"/"+out[i].Kind+"/"+out[i].Name < out[j].APIVersion+"/"+out[j].Kind+"/"+out[j].Name
	})
	return out, nil
}
