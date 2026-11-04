{{- $containers := list -}}
{{- range $name := keys .Normalized.containers -}}
{{- $c := deepCopy (get $.Normalized.containers $name) -}}
{{- $_ := set $c "name" $name -}}
{{- $containers = append $containers $c -}}
{{- end -}}
{{- $pod := dict "metadata" (dict) "spec" (dict "containers" $containers) -}}
{{- $deployment := dict "apiVersion" "apps/v1" "kind" "Deployment" "metadata" (dict "name" .Input.identity.workload "namespace" .Input.target.namespace) "spec" (dict "template" $pod) -}}
{{ toJson (list $deployment) }}
