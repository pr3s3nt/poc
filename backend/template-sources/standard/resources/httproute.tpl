{{- $endpoint := .Input.endpoint -}}
{{- if $endpoint -}}
{{- $backends := list -}}
{{- range $m := .Manifests -}}
  {{- if and (eq $m.apiVersion "v1") (eq $m.kind "Service") -}}
    {{- range $port := $m.spec.ports -}}
      {{- if eq $port.name $endpoint.port -}}
        {{- $backends = append $backends (dict "group" "" "kind" "Service" "name" $m.metadata.name "port" $port.port) -}}
      {{- end -}}
    {{- end -}}
  {{- end -}}
{{- end -}}
{{- if ne (len $backends) 1 -}}{{ fail "endpoint requires exactly one rendered Service port" }}{{- end -}}
{{- $parent := dict "group" "gateway.networking.k8s.io" "kind" "Gateway" "name" $endpoint.gatewayName "namespace" $endpoint.gatewayNamespace -}}
{{- if (get $endpoint "gatewaySectionName") -}}{{- $_ := set $parent "sectionName" $endpoint.gatewaySectionName -}}{{- end -}}
{{- $rule := dict "matches" (list (dict "path" (dict "type" "PathPrefix" "value" $endpoint.path))) "backendRefs" $backends -}}
{{- $metadata := dict "name" $endpoint.name "namespace" .Input.target.namespace "labels" .Input.identity.labels -}}
{{ toJson (list (dict "apiVersion" "gateway.networking.k8s.io/v1" "kind" "HTTPRoute" "metadata" $metadata "spec" (dict "hostnames" (list $endpoint.hostname) "parentRefs" (list $parent) "rules" (list $rule)))) }}
{{- else -}}[]{{- end -}}
