// A small editable starting point; the verified catalog source supports the full execution contract.
export const deploymentTemplate = `{{- $containers := list -}}
{{- range $name := keys .Workload.containers -}}
{{- $declared := get $.Workload.containers $name -}}
{{- $c := dict "name" $name "image" $declared.image -}}
{{- range $key := list "command" "args" "workingDir" "ports" -}}
{{- if hasKey $declared $key }}{{ $_ := set $c $key (get $declared $key) }}{{ end -}}
{{- end -}}
{{- $resources := get $declared "resources" | default (dict) -}}
{{- $requests := deepCopy (get $resources "limits" | default (dict)) -}}
{{- $requests = merge $requests (get $resources "requests" | default (dict)) -}}
{{- if gt (len $resources) 0 }}{{ $_ := set $c "resources" (merge $resources (dict "requests" $requests)) }}{{ end -}}
{{- range $probe := list "startupProbe" "readinessProbe" "livenessProbe" -}}
{{- if hasKey $declared $probe -}}
{{- $p := get $declared $probe -}}
{{- $_ := set $c $probe (dict "httpGet" (dict "path" $p.path "port" $p.port)) -}}
{{- end -}}{{- end -}}
{{- $env := list -}}
{{- range $b := get $.Input.bindings $name | default (list) -}}
{{- $entry := dict "name" $b.name -}}
{{- if eq $b.kind "secret" -}}
{{- $_ := set $entry "valueFrom" (dict "secretKeyRef" (dict "name" $b.secretName "key" $b.secretKey)) -}}
{{- else -}}{{- $_ := set $entry "value" $b.value -}}{{- end -}}
{{- $env = append $env $entry -}}
{{- end -}}
{{- $_ := set $c "env" $env -}}
{{- $containers = append $containers $c -}}
{{- end -}}
{{- $labels := merge .Input.identity.labels .Input.identity.selector -}}
{{- $metadata := dict "labels" $labels -}}
{{- if hasKey .Input.delivery "podAnnotations" }}{{ $_ := set $metadata "annotations" .Input.delivery.podAnnotations }}{{ end -}}
{{- $pod := dict "containers" $containers -}}
{{- range $key := list "imagePullSecrets" "serviceAccountName" -}}
{{- if hasKey $.Input.delivery $key }}{{ $_ := set $pod $key (get $.Input.delivery $key) }}{{ end -}}
{{- end -}}
{{- $replicas := 1 -}}{{- if hasKey .Workload "replicas" }}{{ $replicas = .Workload.replicas }}{{ end -}}
{{- $spec := dict "replicas" $replicas "selector" (dict "matchLabels" .Input.identity.selector) "template" (dict "metadata" $metadata "spec" $pod) -}}
{{ toJson (list (dict "apiVersion" "apps/v1" "kind" "Deployment" "metadata" (dict "name" .Input.identity.workload "namespace" .Input.target.namespace "labels" $labels) "spec" $spec)) }}`;

export const serviceTemplate = `{{- $service := get .Workload "service" | default (dict) -}}
{{- $ports := list -}}
{{- range $name := keys (get $service "ports" | default (dict)) -}}
{{- $p := deepCopy (get $service.ports $name) -}}
{{- $_ := set $p "name" $name -}}
{{- $ports = append $ports $p -}}
{{- end -}}
{{- if gt (len $ports) 0 -}}
{{ toJson (list (dict "apiVersion" "v1" "kind" "Service" "metadata" (dict "name" .Input.identity.workload "namespace" .Input.target.namespace "labels" .Input.identity.labels) "spec" (dict "ports" $ports "selector" .Input.identity.selector))) }}
{{- else -}}[]{{- end -}}`;
