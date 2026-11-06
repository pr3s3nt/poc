{{- $in := .Input -}}
{{- range $i,$m := .Manifests -}}
{{- if or (eq $m.kind "Deployment") (eq $m.kind "StatefulSet") (eq $m.kind "CronJob") -}}
{{- $pod := dict -}}
{{- $path := printf "%d.spec.template.spec" $i -}}
{{- if eq $m.kind "CronJob" -}}
{{- $pod = $m.spec.jobTemplate.spec.template -}}
{{- $path = printf "%d.spec.jobTemplate.spec.template.spec" $i -}}
{{- else -}}{{- $pod = $m.spec.template -}}{{- end -}}
{{- $apps := list -}}
{{- $ordered := dict -}}
{{- range $container := $pod.spec.containers -}}
{{- $declared := get $in.workload.spec.containers $container.name | default (dict) -}}
{{- $role := get $declared "role" | default "app" -}}
{{- if or (eq $role "init") (eq $role "sidecar") -}}
{{- if eq $role "sidecar" }}{{ $_ := set $container "restartPolicy" "Always" }}{{ end -}}
{{- $_ := set $ordered (printf "%03.0f" $declared.initOrder) $container -}}
{{- else -}}{{- $apps = append $apps $container -}}{{- end -}}
{{- end -}}
{{- $init := list -}}
{{- range $order := keys $ordered -}}{{- $init = append $init (get $ordered $order) -}}{{- end -}}
{{- if gt (len $init) 0 }}
- op: set
  path: "{{ $path }}.containers"
  value: {{ toJson $apps }}
- op: set
  path: "{{ $path }}.initContainers"
  value: {{ toJson $init }}
{{ end -}}
{{- end -}}{{- end -}}
