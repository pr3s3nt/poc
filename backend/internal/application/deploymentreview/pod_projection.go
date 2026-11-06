package deploymentreview

func projectContainers(containers []any) []any {
	list := []any{}
	for _, entry := range containers {
		c, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		clean := map[string]any{}
		for _, field := range []string{"name", "image", "imagePullPolicy", "restartPolicy", "workingDir"} {
			if value, ok := c[field].(string); ok {
				clean[field] = value
			}
		}
		if ports, ok := c["ports"].([]any); ok {
			clean["ports"] = selectObjectList(ports, "name", "containerPort", "protocol")
		}
		if mounts, ok := c["volumeMounts"].([]any); ok {
			clean["volumeMounts"] = selectObjectList(mounts, "name", "mountPath", "readOnly", "subPath")
		}
		if value := safeTree(c["securityContext"], 0); value != nil {
			clean["securityContext"] = value
		}
		if resources, ok := c["resources"].(map[string]any); ok {
			selected := map[string]any{}
			for _, class := range []string{"requests", "limits"} {
				if values, ok := resources[class].(map[string]any); ok {
					selected[class] = selectObject(values, "cpu", "memory")
				}
			}
			clean["resources"] = selected
		}
		for _, field := range []string{"readinessProbe", "livenessProbe", "startupProbe"} {
			if probe, ok := c[field].(map[string]any); ok {
				selected := selectObject(probe, "initialDelaySeconds", "periodSeconds", "timeoutSeconds", "failureThreshold", "successThreshold")
				if get, ok := probe["httpGet"].(map[string]any); ok {
					selected["httpGet"] = selectObject(get, "path", "port", "scheme")
				}
				if tcp, ok := probe["tcpSocket"].(map[string]any); ok {
					selected["tcpSocket"] = selectObject(tcp, "port")
				}
				if _, ok := probe["exec"]; ok {
					selected["exec"] = map[string]any{"command": []any{"<redacted>"}}
				}
				clean[field] = selected
			}
		}
		if env, ok := c["env"].([]any); ok {
			safeEnv := []any{}
			for _, e := range env {
				v, ok := e.(map[string]any)
				if !ok {
					continue
				}
				name, valid := v["name"].(string)
				if !valid || !safeLabel(name) {
					continue
				}
				item := map[string]any{"name": name}
				if ref, ok := v["valueFrom"].(map[string]any); ok {
					if secret, ok := ref["secretKeyRef"].(map[string]any); ok {
						item["valueFrom"] = map[string]any{"secretKeyRef": selectObject(secret, "name", "key", "optional")}
					}
				} else {
					item["value"] = "<redacted>"
				}
				safeEnv = append(safeEnv, item)
			}
			clean["env"] = safeEnv
		}
		list = append(list, clean)
	}
	return list
}

func projectVolumes(volumes []any) []any {
	out := []any{}
	for _, raw := range volumes {
		v, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		safe := selectObject(v, "name")
		for kind, fields := range map[string][]string{"hostPath": {"path", "type"}, "emptyDir": {"medium", "sizeLimit"}, "persistentVolumeClaim": {"claimName", "readOnly"}, "configMap": {"name", "defaultMode", "optional"}, "secret": {"secretName", "defaultMode", "optional"}} {
			if source, ok := v[kind].(map[string]any); ok {
				clean := selectObject(source, fields...)
				if items, ok := source["items"].([]any); ok {
					clean["items"] = selectObjectList(items, "key", "path", "mode")
				}
				safe[kind] = clean
			}
		}
		out = append(out, safe)
	}
	return out
}
