{{- /*
  Deployment policy: identity labels/selector, replicas fallback, Pod
  metadata, delivery references, probe timing, pull policy and container
  ports. Returns score-k8s set/delete operations.
*/ -}}
{{- $in := .Input -}}
{{- $d := .Defaults -}}
{{- $inputs := .Inputs -}}
{{- $policy := $d.labels -}}
{{- if hasKey $inputs "team" }}{{ $policy = merge $policy (dict "example.com/team" $inputs.team) }}{{ end -}}
{{- $labels := merge $policy $in.identity.labels -}}
{{- $replicas := get $in.workload.spec "replicas" | default (get $inputs "defaultReplicas") | default 1 -}}
{{- $first := index (keys $in.workload.spec.containers) 0 -}}
{{- $ports := list -}}
{{- if hasKey $in.workload.spec "service" -}}
  {{- $declared := get $in.workload.spec.service "ports" | default (dict) -}}
  {{- range $portName := keys $declared -}}
    {{- $p := get $declared $portName -}}
    {{- $ports = append $ports (dict "name" $portName "containerPort" (get $p "targetPort" | default $p.port)) -}}
  {{- end -}}
{{- end -}}
{{- $podSecurity := $d.podSecurityContext -}}
{{- if $inputs.runAsNonRoot }}{{ $podSecurity = merge $podSecurity (dict "runAsNonRoot" true) }}{{ end -}}
{{- range $i, $m := .Manifests }}{{ if eq $m.kind "Deployment" }}
- op: set
  path: "{{ $i }}.metadata.labels"
  value: {{ toJson $labels }}
- op: delete
  path: "{{ $i }}.metadata.annotations"
- op: set
  path: "{{ $i }}.spec.replicas"
  value: {{ toJson $replicas }}
- op: set
  path: "{{ $i }}.spec.selector"
  value: {{ toJson (dict "matchLabels" $in.identity.selector) }}
{{- $generated := get $m.spec.template.metadata "annotations" | default (dict) }}
{{- $_ := unset $generated "k8s.score.dev/workload-name" }}
{{- $annotations := merge $generated $d.podAnnotations (get $in.delivery "podAnnotations" | default (dict)) }}
{{- $podMetadata := dict "labels" (merge $labels $in.identity.selector) }}
{{- if gt (len $annotations) 0 }}{{ $_ := set $podMetadata "annotations" $annotations }}{{ end }}
- op: set
  path: "{{ $i }}.spec.template.metadata"
  value: {{ toJson $podMetadata }}
{{- range $field := list "imagePullSecrets" "serviceAccountName" }}{{ if hasKey $in.delivery $field }}
- op: set
  path: "{{ $i }}.spec.template.spec.{{ $field }}"
  value: {{ toJson (get $in.delivery $field) }}
{{- end }}{{ end }}
{{- if gt (len $podSecurity) 0 }}
- op: set
  path: "{{ $i }}.spec.template.spec.securityContext"
  value: {{ toJson $podSecurity }}
{{- end }}
{{- range $ci, $c := $m.spec.template.spec.containers }}
{{- $declared := get $in.workload.spec.containers $c.name }}
{{- if hasKey $declared "workingDir" }}
- op: set
  path: "{{ $i }}.spec.template.spec.containers.{{ $ci }}.workingDir"
  value: {{ toJson $declared.workingDir }}
{{- end }}
{{- if hasKey $declared "startupProbe" }}
- op: set
  path: "{{ $i }}.spec.template.spec.containers.{{ $ci }}.startupProbe"
  value: {{ toJson (merge (dict "httpGet" (dict "path" $declared.startupProbe.path "port" $declared.startupProbe.port)) $d.probe) }}
{{- end }}
- op: set
  path: "{{ $i }}.spec.template.spec.containers.{{ $ci }}.imagePullPolicy"
  value: {{ toJson $d.imagePullPolicy }}
{{- if gt (len $d.containerSecurityContext) 0 }}
- op: set
  path: "{{ $i }}.spec.template.spec.containers.{{ $ci }}.securityContext"
  value: {{ toJson $d.containerSecurityContext }}
{{- end }}
{{- if and (eq $c.name $first) (gt (len $ports) 0) }}
- op: set
  path: "{{ $i }}.spec.template.spec.containers.{{ $ci }}.ports"
  value: {{ toJson $ports }}
{{- end }}
{{- range $probe := list "readinessProbe" "livenessProbe" }}{{ if hasKey $c $probe }}
- op: set
  path: "{{ $i }}.spec.template.spec.containers.{{ $ci }}.{{ $probe }}"
  value: {{ toJson (merge (dict "httpGet" (get (get $c $probe) "httpGet")) $d.probe) }}
{{- end }}{{ end }}
{{- range $ei, $e := get $c "env" | default (list) }}{{ if and (not (hasKey $e "value")) (not (hasKey $e "valueFrom")) }}
- op: set
  path: "{{ $i }}.spec.template.spec.containers.{{ $ci }}.env.{{ $ei }}.value"
  value: ""
{{- end }}{{ end }}
{{- end }}
{{- end }}{{ end }}
