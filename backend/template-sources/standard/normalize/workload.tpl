{{- /*
  Normalization: neutral render input -> normalized workload intent (JSON).
  Owns container/probe conversion, resource request fallbacks and packaging
  of the platform-resolved bindings as the "bindings" Workload resource.
*/ -}}
{{- $spec := .Input.workload.spec -}}
{{- $d := .Defaults -}}
{{- $containers := dict -}}
{{- range $name := keys $spec.containers -}}
  {{- $c := get $spec.containers $name -}}
  {{- $out := dict "image" $c.image -}}
{{- if hasKey $c "ports" }}{{ $_ := set $out "ports" $c.ports }}{{ end -}}
  {{- range $field := list "command" "args" -}}
    {{- if hasKey $c $field }}{{ $_ := set $out $field (get $c $field) }}{{ end -}}
  {{- end -}}
  {{- $declared := get $c "resources" | default (dict) -}}
  {{- $declaredRequests := get $declared "requests" | default (dict) -}}
  {{- $declaredLimits := get $declared "limits" | default (dict) -}}
  {{- $requests := dict -}}
  {{- range $field := list "cpu" "memory" -}}
    {{- /* explicit request, then the same-field limit, then the bundle default */ -}}
    {{- $value := get $declaredRequests $field | default (get $declaredLimits $field) | default (get $d.resources.requests $field) -}}
    {{- if $value }}{{ $_ := set $requests $field $value }}{{ end -}}
  {{- end -}}
  {{- $resources := dict "requests" $requests -}}
  {{- if gt (len $declaredLimits) 0 }}{{ $_ := set $resources "limits" $declaredLimits }}{{ end -}}
  {{- $_ := set $out "resources" $resources -}}
  {{- range $probe := list "readinessProbe" "livenessProbe" "startupProbe" -}}
    {{- if hasKey $c $probe }}{{ $_ := set $out $probe (get (get $.Input.probes $name) $probe) }}{{ end -}}
  {{- end -}}
  {{- $env := list -}}
  {{- range $b := get $.Input.bindings $name | default (list) -}}
    {{- $entry := dict "name" $b.name -}}
    {{- if eq $b.kind "secret" -}}
      {{- $_ := set $entry "valueFrom" (dict "secretKeyRef" (dict "name" $b.secretName "key" $b.secretKey)) -}}
    {{- else -}}
      {{- $_ := set $entry "value" $b.value -}}
    {{- end -}}
    {{- $env = append $env $entry -}}
  {{- end -}}
  {{- if gt (len $env) 0 }}{{ $_ := set $out "env" $env }}{{ end -}}
  {{- $_ := set $containers $name $out -}}
{{- end -}}
{{- $doc := dict "apiVersion" "orchestrator.dev/workload/v1" "metadata" (dict "name" .Input.identity.workload) "containers" $containers -}}
{{- if hasKey $spec "service" -}}
  {{- $ports := dict -}}
  {{- range $portName, $p := get $spec.service "ports" | default (dict) -}}
    {{- $_ := set $ports $portName (dict "port" $p.port "targetPort" (get $p "targetPort" | default $p.port) "protocol" (get $p "protocol" | default "TCP")) -}}
  {{- end -}}
  {{- if gt (len $ports) 0 }}{{ $_ := set $doc "service" (dict "ports" $ports) }}{{ end -}}
{{- end -}}
{{- toJson $doc -}}
