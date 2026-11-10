{{- /*
  Optional migration Job (ADR-042): a workload template that sets the
  migrateArgs input gets a one-shot Job running its first app container with
  those arguments. Delivery runs it before the Environment app bundle and gives
  it a content-derived name, so an unchanged Job never runs again.
*/ -}}
{{- $e := get .Input.workload.spec "workloadExecution" | default (dict) -}}
{{- $inputs := get $e "inputs" | default (dict) -}}
{{- $args := get $inputs "migrateArgs" | default (list) -}}
{{- if gt (len $args) 0 -}}
{{- $first := "" -}}
{{- range $name := keys .Input.workload.spec.containers -}}
{{- $declared := get $.Input.workload.spec.containers $name -}}
{{- if and (eq $first "") (or (not (get $declared "role")) (eq $declared.role "app")) }}{{ $first = $name }}{{ end -}}
{{- end -}}
{{- $c := deepCopy (get .Normalized.containers $first) -}}
{{- $_ := set $c "name" $first -}}
{{- $_ := set $c "args" $args -}}
{{- range $field := list "ports" "readinessProbe" "livenessProbe" "startupProbe" "role" -}}{{- $_ := unset $c $field -}}{{- end -}}
{{- $pod := dict "spec" (dict "restartPolicy" "Never" "containers" (list $c)) -}}
{{- $job := dict "apiVersion" "batch/v1" "kind" "Job" "metadata" (dict "name" (printf "%s-migrate" .Input.identity.workload) "namespace" .Input.target.namespace) "spec" (dict "backoffLimit" 2 "template" $pod) -}}
{{ toJson (list $job) }}
{{- else -}}[]{{- end -}}
