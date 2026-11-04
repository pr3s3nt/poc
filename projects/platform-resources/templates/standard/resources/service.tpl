{{- $service := get .Normalized "service" | default (dict) -}}
{{- $ports := list -}}
{{- range $name := keys (get $service "ports" | default (dict)) -}}
{{- $p := deepCopy (get $service.ports $name) -}}
{{- $_ := set $p "name" $name -}}
{{- $ports = append $ports $p -}}
{{- end -}}
{{- if gt (len $ports) 0 -}}
{{- $service := dict "apiVersion" "v1" "kind" "Service" "metadata" (dict "name" .Input.identity.workload "namespace" .Input.target.namespace) "spec" (dict "ports" $ports) -}}
{{ toJson (list $service) }}
{{- else -}}[]{{- end -}}
