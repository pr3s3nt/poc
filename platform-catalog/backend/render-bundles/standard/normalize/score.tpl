{{- /*
  Normalization: neutral render input -> one Score document (JSON).
  Owns container/probe conversion, resource request fallbacks and packaging
  of the platform-resolved bindings as the "bindings" Score resource.
*/ -}}
{{- $spec := .Input.workload.spec -}}
{{- $d := .Defaults -}}
{{- $containers := dict -}}
{{- $params := dict -}}
{{- range $name := keys $spec.containers -}}
  {{- $c := get $spec.containers $name -}}
  {{- $out := dict "image" $c.image -}}
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
  {{- range $probe := list "readinessProbe" "livenessProbe" -}}
    {{- if hasKey $c $probe -}}
      {{- $p := get $c $probe -}}
      {{- $_ := set $out $probe (dict "httpGet" (dict "path" $p.path "port" $p.port)) -}}
    {{- end -}}
  {{- end -}}
  {{- $variables := dict -}}
  {{- $bound := dict -}}
  {{- range $b := get $.Input.bindings $name | default (list) -}}
    {{- $_ := set $variables $b.name (printf "${resources.bindings.%s.%s}" $name $b.name) -}}
    {{- if eq $b.kind "secret" -}}
      {{- $_ := set $bound $b.name (dict "secret" true "secretName" $b.secretName "secretKey" $b.secretKey) -}}
    {{- else -}}
      {{- $_ := set $bound $b.name (dict "secret" false "value" (escapeScore $b.value)) -}}
    {{- end -}}
  {{- end -}}
  {{- if gt (len $variables) 0 }}{{ $_ := set $out "variables" $variables }}{{ $_ := set $params $name $bound }}{{ end -}}
  {{- $_ := set $containers $name $out -}}
{{- end -}}
{{- $doc := dict "apiVersion" "score.dev/v1b1" "metadata" (dict "name" .Input.identity.workload) "containers" $containers -}}
{{- if hasKey $spec "service" -}}
  {{- $ports := dict -}}
  {{- range $portName, $p := get $spec.service "ports" | default (dict) -}}
    {{- $_ := set $ports $portName (dict "port" $p.port "targetPort" (get $p "targetPort" | default $p.port) "protocol" (get $p "protocol" | default "TCP")) -}}
  {{- end -}}
  {{- if gt (len $ports) 0 }}{{ $_ := set $doc "service" (dict "ports" $ports) }}{{ end -}}
{{- end -}}
{{- if gt (len $params) 0 -}}
  {{- $_ := set $doc "resources" (dict "bindings" (dict "type" "orchestrator-bindings" "params" $params)) -}}
{{- end -}}
{{- toJson $doc -}}
