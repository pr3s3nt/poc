package artifact

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
)

var ErrInvalid = errors.New("delivery: invalid execution artifact")

// SameExecution compares business content while ignoring platform provenance.
func SameExecution(prior, next []map[string]any) (bool, error) {
	normalize := func(objects []map[string]any) ([]byte, error) {
		for _, obj := range objects {
			md, _ := obj["metadata"].(map[string]any)
			if md == nil {
				return nil, ErrInvalid
			}
			annotations, _ := md["annotations"].(map[string]any)
			for _, key := range []string{"helm.sh/resource-policy", "meta.helm.sh/release-name", "meta.helm.sh/release-namespace", "orchestrator.io/delivery-revision"} {
				delete(annotations, key)
			}
			if len(annotations) == 0 {
				delete(md, "annotations")
			}
			labels, _ := md["labels"].(map[string]any)
			delete(labels, "orchestrator.io/deployment-id")
			delete(labels, "orchestrator.io/managed-by")
			if labels["app.kubernetes.io/managed-by"] == "Helm" {
				labels["app.kubernetes.io/managed-by"] = "orchestrator"
			}
			spec, _ := obj["spec"].(map[string]any)
			template, _ := spec["template"].(map[string]any)
			templateMD, _ := template["metadata"].(map[string]any)
			templateLabels, _ := templateMD["labels"].(map[string]any)
			delete(templateLabels, "orchestrator.io/deployment-id")
		}
		sort.Slice(objects, func(i, j int) bool {
			a, _ := objects[i]["metadata"].(map[string]any)
			b, _ := objects[j]["metadata"].(map[string]any)
			an, _ := a["name"].(string)
			bn, _ := b["name"].(string)
			return an < bn
		})
		return json.Marshal(objects)
	}
	a, err := normalize(prior)
	if err != nil {
		return false, err
	}
	b, err := normalize(next)
	if err != nil {
		return false, err
	}
	return bytes.Equal(a, b), nil
}
