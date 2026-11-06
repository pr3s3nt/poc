package templatedriver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
	"reflect"
	"sort"
)

func configMaps(req execution.RenderRequest) map[string]any {
	out := map[string]any{}
	if e := req.Module.Spec.Execution; e != nil {
		for _, v := range e.Volumes {
			if c := v.ConfigMap; c != nil {
				raw, _ := json.Marshal(v)
				hash := sha256.Sum256(raw)
				mode := c.DefaultMode
				if mode == 0 {
					mode = 0444
				}
				out[v.Name] = map[string]any{"name": req.WorkloadID + "-config-" + hex.EncodeToString(hash[:])[:12], "data": c.Values(), "mode": mode}
			}
		}
	}
	return out
}
func podContainers(pod map[string]any) []any {
	return append(append([]any{}, list(pod["containers"])...), list(pod["initContainers"])...)
}
func guardLifecycle(req execution.RenderRequest, pod map[string]any) error {
	fields := map[string]string{}
	positions := map[string]int{}
	all := map[string]map[string]any{}
	for _, field := range []string{"containers", "initContainers"} {
		for i, raw := range list(pod[field]) {
			c := object(raw)
			name, _ := c["name"].(string)
			if name == "" || all[name] != nil {
				return violated("container names must be unique across roles")
			}
			all[name] = c
			fields[name] = field
			positions[name] = i
		}
	}
	ordered := []string{}
	for name, c := range req.Module.Spec.Containers {
		actual := all[name]
		if actual == nil {
			return violated("every declared container must be kept")
		}
		init := c.Role == "init" || c.Role == "sidecar"
		if (fields[name] == "initContainers") != init {
			return violated("declared container role must be kept")
		}
		if c.Role == "sidecar" {
			if actual["restartPolicy"] != "Always" {
				return violated("native sidecar restart policy must be Always")
			}
		} else if _, ok := actual["restartPolicy"]; ok {
			return violated("regular container cannot gain a sidecar restart policy")
		}
		if init {
			ordered = append(ordered, name)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		return req.Module.Spec.Containers[ordered[i]].InitOrder < req.Module.Spec.Containers[ordered[j]].InitOrder
	})
	for i := 1; i < len(ordered); i++ {
		if positions[ordered[i-1]] >= positions[ordered[i]] {
			return violated("initialization order must be kept")
		}
	}
	return nil
}
func guardLocalMounts(req execution.RenderRequest, pod map[string]any) error {
	e := req.Module.Spec.Execution
	if e == nil {
		return nil
	}
	volumes := map[string]map[string]any{}
	for _, raw := range list(pod["volumes"]) {
		v := object(raw)
		name, _ := v["name"].(string)
		if volumes[name] != nil {
			return violated("volume names must be unique")
		}
		volumes[name] = v
	}
	intent := configMaps(req)
	for _, mount := range e.VolumeMounts {
		found := false
		for _, raw := range podContainers(pod) {
			c := object(raw)
			if c["name"] != mount.Container {
				continue
			}
			for _, rawMount := range list(c["volumeMounts"]) {
				m := object(rawMount)
				sub, _ := m["subPath"].(string)
				readOnly, _ := m["readOnly"].(bool)
				if m["mountPath"] != mount.Path || sub != mount.SubPath || readOnly != mount.ReadOnly {
					continue
				}
				name, _ := m["name"].(string)
				actual := volumes[name]
				if len(actual) != 2 {
					continue
				}
				for _, v := range e.Volumes {
					if v.Name != mount.Volume {
						continue
					}
					if v.HostPath != nil {
						want, _ := toTree(v.HostPath)
						found = reflect.DeepEqual(object(actual["hostPath"]), want)
					}
					if v.EmptyDir != nil {
						want, _ := toTree(v.EmptyDir)
						found = reflect.DeepEqual(object(actual["emptyDir"]), want)
					}
					if v.ConfigMap != nil {
						want := object(intent[v.Name])
						got := object(actual["configMap"])
						found = len(got) == 2 && got["name"] == want["name"] && toNumber(got["defaultMode"]) == toNumber(want["mode"])
					}
				}
			}
		}
		if !found {
			return violated("approved local volume source and mount must be kept")
		}
	}
	return nil
}
func guardConfigMaps(req execution.RenderRequest, objects []execution.Manifest) error {
	for _, raw := range configMaps(req) {
		want := object(raw)
		found := false
		for _, m := range objects {
			if m.Kind == "ConfigMap" && m.APIVersion == "v1" && m.Name == want["name"] {
				data, _ := want["data"].(map[string]string)
				if sameStringMap(m.Object["data"], data) {
					found = true
				}
			}
		}
		if !found {
			return violated("declared ConfigMap data must be kept")
		}
	}
	return nil
}

func probeIntent(p *environment.Probe) map[string]any {
	if p == nil {
		return nil
	}
	out := map[string]any{}
	switch p.Type {
	case "", "http":
		action := map[string]any{"path": p.Path, "port": p.Port}
		if p.Scheme != "" {
			action["scheme"] = p.Scheme
		}
		out["httpGet"] = action
	case "tcp":
		out["tcpSocket"] = map[string]any{"port": p.Port}
	case "exec":
		out["exec"] = map[string]any{"command": p.Command}
	}
	for key, v := range map[string]*int{"initialDelaySeconds": p.InitialDelaySeconds, "timeoutSeconds": p.TimeoutSeconds, "periodSeconds": p.PeriodSeconds, "successThreshold": p.SuccessThreshold, "failureThreshold": p.FailureThreshold} {
		if v != nil {
			out[key] = *v
		}
	}
	return out
}
func probeIntents(req execution.RenderRequest) map[string]any {
	out := map[string]any{}
	for name, c := range req.Module.Spec.Containers {
		probes := map[string]any{}
		for key, p := range map[string]*environment.Probe{"readinessProbe": c.ReadinessProbe, "livenessProbe": c.LivenessProbe, "startupProbe": c.StartupProbe} {
			if p != nil {
				probes[key] = probeIntent(p)
			}
		}
		out[name] = probes
	}
	return out
}
