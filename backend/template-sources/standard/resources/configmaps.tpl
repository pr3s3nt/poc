{{- $objects := list -}}
{{- range $name := keys (get .Input "configMaps" | default (dict)) -}}
{{- $c := get $.Input.configMaps $name -}}
{{- $objects = append $objects (dict "apiVersion" "v1" "kind" "ConfigMap" "metadata" (dict "name" $c.name "namespace" $.Input.target.namespace) "data" $c.data) -}}
{{- end -}}
{{ toJson $objects }}
