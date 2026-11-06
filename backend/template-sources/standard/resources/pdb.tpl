{{- $e := get .Input.workload.spec "workloadExecution" | default (dict) -}}
{{- $p := get $e "podDisruptionBudget" | default (dict) -}}
{{- if get $p "enabled" -}}
{{- $spec := dict "selector" (dict "matchLabels" .Input.identity.selector) -}}
{{- range $key := list "minAvailable" "maxUnavailable" "unhealthyPodEvictionPolicy" -}}
{{- if hasKey $p $key }}{{ $_ := set $spec $key (get $p $key) }}{{ end -}}
{{- end -}}
{{ toJson (list (dict "apiVersion" "policy/v1" "kind" "PodDisruptionBudget" "metadata" (dict "name" .Input.identity.workload "namespace" .Input.target.namespace "labels" .Input.identity.labels) "spec" $spec)) }}
{{- else -}}[]{{- end -}}
