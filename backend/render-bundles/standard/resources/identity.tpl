{{- $e := get .Input.workload.spec "workloadExecution" | default (dict) -}}
{{- if eq (get $e "kind") "StatefulSet" -}}
{{ toJson (list (dict "apiVersion" "v1" "kind" "Service" "metadata" (dict "name" (printf "%s-identity" .Input.identity.workload) "namespace" .Input.target.namespace) "spec" (dict "clusterIP" "None" "selector" .Input.identity.selector "ports" (list)))) }}
{{- else -}}[]{{- end -}}
