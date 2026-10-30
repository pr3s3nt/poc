{{- if eq .Input.identity.workload "frontend" }}
{{- range $i, $m := .Manifests }}{{ if eq $m.kind "Deployment" }}
- op: delete
  path: "{{ $i }}.spec.replicas"
{{- end }}{{ end }}
{{- else }}
[]
{{- end }}
