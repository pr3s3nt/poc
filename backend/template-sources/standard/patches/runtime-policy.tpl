{{- $in := .Input -}}
{{- $e := get $in.workload.spec "workloadExecution" | default (dict) -}}
{{- range $i,$m := .Manifests -}}
{{- if or (eq $m.kind "Deployment") (eq $m.kind "StatefulSet") (eq $m.kind "CronJob") -}}
{{- $path := printf "%d.spec.template.spec" $i -}}
{{- $pod := dict -}}
{{- if eq $m.kind "CronJob" -}}
{{- $path = printf "%d.spec.jobTemplate.spec.template.spec" $i -}}
{{- $pod = $m.spec.jobTemplate.spec.template.spec -}}
{{- else -}}{{- $pod = $m.spec.template.spec -}}{{- end -}}
{{- range $key := list "securityContext" "automountServiceAccountToken" "serviceAccountName" -}}
{{- $intentKey := $key -}}{{- if eq $key "securityContext" }}{{ $intentKey = "podSecurityContext" }}{{ end -}}
{{- if hasKey $e $intentKey }}
- op: set
  path: "{{ $path }}.{{ $key }}"
  value: {{ toJson (get $e $intentKey) }}
{{- end -}}{{- end -}}
{{- range $field := list "containers" "initContainers" -}}
{{- range $ci,$c := get $pod $field | default (list) -}}
{{- $declared := get $in.workload.spec.containers $c.name | default (dict) -}}
{{- $security := merge $.Defaults.containerSecurityContext (get $c "securityContext" | default (dict)) (get $declared "securityContext" | default (dict)) -}}
{{- if gt (len $security) 0 }}
- op: set
  path: "{{ $path }}.{{ $field }}.{{ $ci }}.securityContext"
  value: {{ toJson $security }}
{{- end -}}
{{- $mounts := get $c "volumeMounts" | default (list) -}}
{{- range $mount := get $e "claimMounts" | default (list) -}}
{{- if eq $mount.container $c.name -}}
{{- $v := dict "name" $mount.claim "mountPath" $mount.path "readOnly" $mount.readOnly -}}
{{- if hasKey $mount "subPath" }}{{ $_ := set $v "subPath" $mount.subPath }}{{ end -}}
{{- $mounts = append $mounts $v -}}
{{- end -}}{{- end -}}
{{- if gt (len (get $e "claimMounts" | default (list))) 0 }}
- op: set
  path: "{{ $path }}.{{ $field }}.{{ $ci }}.volumeMounts"
  value: {{ toJson $mounts }}
{{- end -}}{{- end -}}{{- end -}}
{{- if ne $m.kind "CronJob" -}}
{{- if hasKey $e "revisionHistoryLimit" }}
- op: set
  path: "{{ $i }}.spec.revisionHistoryLimit"
  value: {{ toJson $e.revisionHistoryLimit }}
{{- end -}}
{{- if hasKey $e "updateStrategy" -}}
{{- $key := "strategy" -}}{{- if eq $m.kind "StatefulSet" }}{{ $key = "updateStrategy" }}{{ end }}
- op: set
  path: "{{ $i }}.spec.{{ $key }}"
  value: {{ toJson $e.updateStrategy }}
{{- end -}}{{- end -}}
{{- if and (eq $m.kind "StatefulSet") (gt (len (get $e "volumeClaimTemplates" | default (list))) 0) -}}
{{- $claims := list -}}
{{- range $claim := $e.volumeClaimTemplates -}}
{{- $spec := dict "accessModes" $claim.accessModes "resources" (dict "requests" (dict "storage" $claim.size)) -}}
{{- range $key := list "storageClassName" "volumeMode" -}}{{- if hasKey $claim $key }}{{ $_ := set $spec $key (get $claim $key) }}{{ end -}}{{- end -}}
{{- $claims = append $claims (dict "metadata" (dict "name" $claim.name "labels" $in.identity.labels) "spec" $spec) -}}
{{- end }}
- op: set
  path: "{{ $i }}.spec.volumeClaimTemplates"
  value: {{ toJson $claims }}
- op: set
  path: "{{ $i }}.spec.persistentVolumeClaimRetentionPolicy"
  value: {whenDeleted: Retain, whenScaled: Retain}
{{- end -}}{{- end -}}{{- end -}}
