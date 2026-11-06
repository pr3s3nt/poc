package templatedriver

import (
	"context"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/execution"
	"reflect"
)

func containsIntent(actual any, want any) bool {
	w, ok := want.(map[string]any)
	if !ok {
		return reflect.DeepEqual(actual, want)
	}
	a := object(actual)
	if a == nil {
		return false
	}
	for k, v := range w {
		if !containsIntent(a[k], v) {
			return false
		}
	}
	return true
}
func guardRuntimePolicy(req execution.RenderRequest, kind string, d map[string]any, objects []execution.Manifest, selector map[string]string) error {
	e := req.Module.Spec.Execution
	spec := object(d["spec"])
	podSpec := spec
	if kind == "CronJob" {
		podSpec = object(object(spec["jobTemplate"])["spec"])
	}
	pod := object(object(podSpec["template"])["spec"])
	for name, c := range req.Module.Spec.Containers {
		if c.SecurityContext != nil {
			want, _ := toTree(c.SecurityContext)
			found := false
			for _, raw := range podContainers(pod) {
				a := object(raw)
				if a["name"] == name && containsIntent(a["securityContext"], want) {
					found = true
				}
			}
			if !found {
				return violated("effective container security must be kept")
			}
		}
	}
	if e == nil {
		return nil
	}
	if e.PodSecurityContext != nil {
		want, _ := toTree(e.PodSecurityContext)
		if !containsIntent(pod["securityContext"], want) {
			return violated("effective Pod security must be kept")
		}
	}
	if e.AutomountServiceAccountToken != nil && pod["automountServiceAccountToken"] != *e.AutomountServiceAccountToken {
		return violated("token mount intent must be kept")
	}
	if e.ServiceAccountName != "" && pod["serviceAccountName"] != e.ServiceAccountName {
		return violated("ServiceAccount intent must be kept")
	}
	if e.UpdateStrategy != nil {
		key := "strategy"
		if kind == "StatefulSet" {
			key = "updateStrategy"
		}
		want, _ := toTree(e.UpdateStrategy)
		if !reflect.DeepEqual(spec[key], want) {
			return violated("controller strategy must be kept without incompatible fields")
		}
	}
	if e.RevisionHistoryLimit != nil && (spec["revisionHistoryLimit"] == nil || toNumber(spec["revisionHistoryLimit"]) != float64(*e.RevisionHistoryLimit)) {
		return violated("history limit must be kept")
	}
	pdbCount := 0
	for _, m := range objects {
		if m.Kind == "PersistentVolumeClaim" {
			for _, claim := range e.VolumeClaimTemplates {
				if m.Name == claim.Name+"-"+req.WorkloadID+"-0" {
					return violated("StatefulSet claims cannot also be independent PVC resources")
				}
			}
		}
		if m.Kind != "PodDisruptionBudget" {
			continue
		}
		pdbCount++
		p := e.PodDisruptionBudget
		if p == nil || !p.Enabled || m.Name != req.WorkloadID {
			return violated("PDB must be explicitly enabled")
		}
		s := object(m.Object["spec"])
		if !sameStringMap(object(s["selector"])["matchLabels"], selector) {
			return violated("PDB selector must match workload")
		}
		want, _ := toTree(p)
		delete(want, "enabled")
		for _, key := range []string{"minAvailable", "maxUnavailable", "unhealthyPodEvictionPolicy"} {
			if !reflect.DeepEqual(s[key], want[key]) {
				return violated("PDB policy must be kept")
			}
		}
	}
	if e.PodDisruptionBudget != nil && e.PodDisruptionBudget.Enabled && pdbCount != 1 {
		return violated("enabled PDB is missing")
	}
	claims := list(spec["volumeClaimTemplates"])
	if len(claims) != len(e.VolumeClaimTemplates) {
		return violated("claim templates must match intent")
	}
	for _, c := range e.VolumeClaimTemplates {
		found := false
		for _, raw := range claims {
			a := object(raw)
			if object(a["metadata"])["name"] != c.Name {
				continue
			}
			s := object(a["spec"])
			for _, key := range []string{"selector", "volumeName", "dataSource", "dataSourceRef"} {
				if _, present := s[key]; present {
					return violated("claim templates cannot adopt or restore other storage")
				}
			}
			if c.VolumeMode != "" && s["volumeMode"] != c.VolumeMode {
				continue
			}
			modes, _ := toTree(map[string]any{"accessModes": c.AccessModes})
			if !reflect.DeepEqual(s["accessModes"], modes["accessModes"]) || object(object(s["resources"])["requests"])["storage"] != c.Size {
				continue
			}
			if c.StorageClassName != nil && s["storageClassName"] != *c.StorageClassName {
				continue
			}
			found = true
		}
		if !found {
			return violated("claim template configuration must be kept")
		}
	}
	if len(claims) > 0 {
		ret := object(spec["persistentVolumeClaimRetentionPolicy"])
		if ret["whenDeleted"] != "Retain" || ret["whenScaled"] != "Retain" {
			return violated("claims must be retained")
		}
	}
	for _, mount := range e.ClaimMounts {
		found := false
		for _, raw := range podContainers(pod) {
			c := object(raw)
			if c["name"] != mount.Container {
				continue
			}
			for _, v := range list(c["volumeMounts"]) {
				m := object(v)
				ro, _ := m["readOnly"].(bool)
				if m["name"] == mount.Claim && m["mountPath"] == mount.Path && stringOrEmpty(m["subPath"]) == mount.SubPath && ro == mount.ReadOnly {
					found = true
				}
			}
		}
		if !found {
			return violated("claim mount must be kept")
		}
	}
	return nil
}

// RuntimeProfile returns only immutable nonsecret metadata from the selected cache.
func (r *Renderer) RuntimeProfile(ctx context.Context, selection resource.RenderingSelection, e *environment.WorkloadExecution) (*environment.RuntimeDefaults, error) {
	if !e.HasTemplate() {
		return nil, nil
	}
	bundle, err := r.validateGit(ctx, selection)
	if err != nil {
		return nil, err
	}
	for _, t := range bundle.Contract.Manifest.WorkloadTemplates {
		if t.ID == e.Template.ID && t.Version == e.Template.Version && t.SHA256() == e.Template.SHA256 {
			profile := t.RuntimeDefaults
			if e.TemplateDefaults != nil {
				profile = profile.ForEnvironment(e.TemplateDefaults.Environment)
			}
			return profile, nil
		}
	}
	return nil, environment.ErrExecutionUnavailable
}
