{{- /* Route and Service share workload ownership, namespace and Fleet inventory. */ -}}
{{- $enabled := get .Inputs "httpRouteEnabled" | default false -}}
{{- $annotations := get .Input.workload "annotations" | default (dict) -}}
{{- $hostname := get $annotations "routing.orchestrator.dev/hostname" | default "" -}}
{{- if and $enabled $hostname -}}
{{- $service := get .Normalized "service" | default (dict) -}}
{{- $portName := get $annotations "routing.orchestrator.dev/service-port" | default "http" -}}
{{- $port := required "HTTPRoute requires the selected declared Service port" (get (get $service "ports" | default (dict)) $portName) -}}
{{- $originalService := get .Input.workload.spec "service" | default (dict) -}}
{{- if gt (len (get $originalService "publicRoutes" | default (list))) 0 -}}
{{- fail "HTTPRoute template cannot be combined with platform publicRoutes on the same workload" -}}
{{- end -}}
{{- $parent := dict "group" "gateway.networking.k8s.io" "kind" "Gateway" "name" (required "HTTPRoute requires an existing Gateway name" (get .Inputs "httpRouteGatewayName")) "namespace" (required "HTTPRoute requires an existing Gateway namespace" (get .Inputs "httpRouteGatewayNamespace")) -}}
{{- $section := get .Inputs "httpRouteGatewaySectionName" | default "" -}}
{{- if $section }}{{ $_ := set $parent "sectionName" $section }}{{ end -}}
{{- $rule := dict "matches" (list (dict "path" (dict "type" "PathPrefix" "value" (get $annotations "routing.orchestrator.dev/path" | default "/")))) "backendRefs" (list (dict "group" "" "kind" "Service" "name" .Input.identity.workload "port" $port.port)) -}}
{{- $route := dict "apiVersion" "gateway.networking.k8s.io/v1" "kind" "HTTPRoute" "metadata" (dict "name" .Input.identity.workload "namespace" .Input.target.namespace) "spec" (dict "parentRefs" (list $parent) "hostnames" (list $hostname) "rules" (list $rule)) -}}
{{ toJson (list $route) }}
{{- else -}}[]{{- end -}}
