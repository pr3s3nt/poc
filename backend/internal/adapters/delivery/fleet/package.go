package fleet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"sort"
	"strings"

	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	gitport "orchestrator/internal/ports/gitpublication"
)

// Package is a deterministic generated unit, never a user-authored chart. The
// submitted revision pins both these files and the provider lifecycle metadata.
type Package struct {
	UnitName    string
	ReleaseName string
	Publication gitport.Intent
	Objects     []execution.ObjectRef
}

func PackageUnit(intent port.Intent, manifests []execution.Manifest) (Package, error) {
	if err := intent.Validate(); err != nil {
		return Package{}, err
	}
	cfg, err := ParseConfig(intent.Binding.Config)
	if err != nil {
		return Package{}, err
	}
	if intent.Target.Namespace == "" || intent.Target.Namespace == cfg.ReleaseNamespace || !label.MatchString(intent.Target.Namespace) {
		return Package{}, ErrConfig
	}
	if err := validateReplicaOwnership(manifests); err != nil {
		return Package{}, err
	}
	identity, err := json.Marshal(intent.Revision.Scope)
	if err != nil {
		return Package{}, ErrConfig
	}
	sum := sha256.Sum256(identity)
	name := "orch-" + hex.EncodeToString(sum[:])[:24]
	files := map[string][]byte{}
	objects := []execution.ObjectRef{}
	seen := map[string]bool{}
	for _, m := range manifests {
		if m.Auxiliary {
			continue
		}
		if m.Secret || m.Kind == "Secret" || strings.HasPrefix(m.APIVersion, "secrets.hashicorp.com/") || m.Namespace != intent.Target.Namespace || m.Object == nil || !label.MatchString(strings.ToLower(m.Kind)) || !validObjectName(m.Name) {
			return Package{}, ErrConfig
		}
		if intent.Revision.Phase == domain.Job && m.Kind != "Job" {
			return Package{}, ErrConfig
		}
		if intent.Revision.Phase != domain.Job && m.Kind == "Job" {
			return Package{}, ErrConfig
		}
		if m.Kind == "Job" {
			spec, _ := m.Object["spec"].(map[string]any)
			if _, ttl := spec["ttlSecondsAfterFinished"]; ttl {
				// A controller could recreate a TTL-deleted Job before our first
				// completion observation. No TTL support is advertised for this pin.
				return Package{}, domain.ErrUnsupported
			}
		}
		if intent.Revision.Selection.SelfHeal && (m.Kind == "PersistentVolumeClaim" || !knownContinuous(m.Kind)) {
			return Package{}, domain.ErrUnsupported
		}
		raw, err := json.Marshal(m.Object)
		if err != nil {
			return Package{}, ErrConfig
		}
		var obj map[string]any
		if json.Unmarshal(raw, &obj) != nil {
			return Package{}, ErrConfig
		}
		metadata, ok := obj["metadata"].(map[string]any)
		if !ok || metadata["name"] != m.Name || metadata["namespace"] != m.Namespace || obj["kind"] != m.Kind || obj["apiVersion"] != m.APIVersion {
			return Package{}, ErrConfig
		}
		annotations, _ := metadata["annotations"].(map[string]any)
		if annotations == nil {
			annotations = map[string]any{}
		}
		if _, exists := annotations["helm.sh/hook"]; exists {
			return Package{}, domain.ErrUnsupported
		}
		annotations["helm.sh/resource-policy"] = "keep"
		annotations["meta.helm.sh/release-name"] = name
		annotations["meta.helm.sh/release-namespace"] = cfg.ReleaseNamespace
		annotations["orchestrator.io/delivery-revision"] = intent.Revision.ID
		metadata["annotations"] = annotations
		labels, _ := metadata["labels"].(map[string]any)
		if labels == nil {
			return Package{}, ErrConfig
		}
		// Helm owns its conventional manager label; preserve the independent platform
		// ownership identity for guarded cleanup/transfer.
		if labels["app.kubernetes.io/managed-by"] != "orchestrator" {
			return Package{}, ErrConfig
		}
		labels["orchestrator.io/managed-by"] = "orchestrator"
		labels["app.kubernetes.io/managed-by"] = "Helm"
		metadata["labels"] = labels
		objectKey := m.APIVersion + "/" + m.Kind + "/" + m.Name
		if seen[objectKey] {
			return Package{}, ErrConfig
		}
		seen[objectKey] = true
		body, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			return Package{}, ErrConfig
		}
		// Include the API group in the filename to avoid collisions across CRDs.
		keySum := sha256.Sum256([]byte(objectKey))
		filename := strings.ToLower(m.Kind) + "-" + hex.EncodeToString(keySum[:])[:20] + ".json"
		files[filename] = append(body, '\n')
		objects = append(objects, execution.ObjectRef{APIVersion: m.APIVersion, Kind: m.Kind, Name: m.Name})
	}
	if len(objects) == 0 {
		return Package{}, ErrConfig
	}
	fleet := map[string]any{"defaultNamespace": cfg.ReleaseNamespace, "deleteNamespace": false, "keepResources": false, "correctDrift": map[string]any{"enabled": intent.Revision.Selection.SelfHeal, "force": false}, "serviceAccount": cfg.ServiceAccount, "helm": map[string]any{"releaseName": name, "takeOwnership": false, "force": false, "atomic": false, "waitForJobs": intent.Revision.Phase == domain.Job, "timeoutSeconds": 300}}
	raw, err := json.MarshalIndent(fleet, "", "  ")
	if err != nil {
		return Package{}, ErrConfig
	}
	files["fleet.yaml"] = append(raw, '\n')
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key() < objects[j].Key() })
	destination := sha256.Sum256([]byte(cfg.RepositoryURL + "\x00" + cfg.Branch))
	return Package{UnitName: name, ReleaseName: name, Objects: objects, Publication: gitport.Intent{ID: intent.ID, DestinationKey: hex.EncodeToString(destination[:]), Branch: cfg.Branch, Path: path.Join(cfg.Root, name), Files: files, TreeDigest: gitport.Digest(files)}}, nil
}
func validObjectName(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	for _, part := range strings.Split(s, ".") {
		if !label.MatchString(part) {
			return false
		}
	}
	return true
}
func knownContinuous(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "Service", "ConfigMap", "HorizontalPodAutoscaler", "PodDisruptionBudget", "NetworkPolicy", "ServiceAccount", "Role", "RoleBinding", "Ingress", "HTTPRoute", "CronJob":
		return true
	}
	return false
}

// Fleet 0.15 delegates an HPA's bounded replica changes. Admission must also
// exclude a second configured replica owner; ignoring only the observed diff
// would still allow the next Helm upgrade to overwrite the HPA's decision.
func validateReplicaOwnership(manifests []execution.Manifest) error {
	for _, hpa := range manifests {
		if hpa.Auxiliary || hpa.Kind != "HorizontalPodAutoscaler" {
			continue
		}
		spec, _ := hpa.Object["spec"].(map[string]any)
		ref, _ := spec["scaleTargetRef"].(map[string]any)
		if ref["apiVersion"] != "apps/v1" || (ref["kind"] != "Deployment" && ref["kind"] != "StatefulSet") {
			return domain.ErrUnsupported
		}
		found := false
		for _, target := range manifests {
			if target.Auxiliary || target.APIVersion != ref["apiVersion"] || target.Kind != ref["kind"] || target.Name != ref["name"] || target.Namespace != hpa.Namespace {
				continue
			}
			found = true
			targetSpec, ok := target.Object["spec"].(map[string]any)
			if !ok {
				return ErrConfig
			}
			if _, explicit := targetSpec["replicas"]; explicit {
				return domain.ErrUnsupported
			}
		}
		if !found {
			return domain.ErrUnsupported
		}
	}
	return nil
}
