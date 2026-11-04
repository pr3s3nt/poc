{{- /* Service policy: identity labels and selector, ClusterIP. Absent Service: no-op. */ -}}
{{- $in := .Input -}}
{{- $policy := .Defaults.labels -}}
{{- if hasKey .Inputs "team" }}{{ $policy = merge $policy (dict "example.com/team" .Inputs.team) }}{{ end -}}
{{- $labels := merge $policy $in.identity.labels -}}
{{- range $i, $m := .Manifests }}{{ if eq $m.kind "Service" }}
- op: set
  path: "{{ $i }}.metadata.labels"
  value: {{ toJson $labels }}
- op: delete
  path: "{{ $i }}.metadata.annotations"
- op: set
  path: "{{ $i }}.spec.type"
  value: ClusterIP
- op: set
  path: "{{ $i }}.spec.selector"
  value: {{ toJson $in.identity.selector }}
{{- end }}{{ end }}
