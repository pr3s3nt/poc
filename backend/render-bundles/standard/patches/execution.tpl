{{- $in := .Input -}}
{{- $e := get $in.workload.spec "workloadExecution" | default (dict) -}}
{{- if $e -}}
{{- range $i, $m := .Manifests -}}
{{- if eq $m.kind "Deployment" -}}
{{- $pod := $m.spec.template -}}
{{- $volumes := get $pod.spec "volumes" | default (list) -}}
{{- range $mi, $mount := get $e "mounts" | default (list) -}}
{{- $binding := get $in.storage $mount.resource -}}
{{- $name := printf "persistent-%d" $mi -}}
{{- $volumes = append $volumes (dict "name" $name "persistentVolumeClaim" (dict "claimName" $binding.claimName "readOnly" $mount.readOnly)) -}}
{{- range $ci, $container := $pod.spec.containers -}}
{{- if eq $container.name $mount.container -}}
{{- $mounts := get $container "volumeMounts" | default (list) -}}
{{- $_ := set $container "volumeMounts" (append $mounts (dict "name" $name "mountPath" $mount.path "readOnly" $mount.readOnly)) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- range $fi,$file:=get $in "secretFiles" | default (list) -}}
{{- $name:=printf "secret-file-%d" $fi -}}
{{- $volumes = append $volumes (dict "name" $name "secret" (dict "secretName" $file.secretName "defaultMode" $file.mode "items" (list (dict "key" $file.secretKey "path" "value")))) -}}
{{- range $ci,$container:=$pod.spec.containers -}}
{{- if eq $container.name $file.container -}}
{{- $mounts:=get $container "volumeMounts" | default (list) -}}
{{- $_:=set $container "volumeMounts" (append $mounts (dict "name" $name "mountPath" $file.path "subPath" "value" "readOnly" true)) -}}
{{- end -}}{{- end -}}{{- end -}}
{{- if gt (len $volumes) 0 }}{{ $_ := set $pod.spec "volumes" $volumes }}{{ end -}}
{{- if eq $e.kind "StatefulSet" }}
- op: set
  path: "{{ $i }}.kind"
  value: StatefulSet
- op: set
  path: "{{ $i }}.spec"
  value: {{ toJson (dict "serviceName" (printf "%s-identity" $in.identity.workload) "replicas" 1 "selector" $m.spec.selector "template" $pod) }}
{{- else if eq $e.kind "CronJob" -}}
{{- $_ := set $pod.spec "restartPolicy" "Never" -}}
{{- $job := dict "template" $pod -}}
{{- $schedule := $e.schedule -}}
{{- range $key := list "activeDeadlineSeconds" "backoffLimit" }}{{ if hasKey $schedule $key }}{{ $_ := set $job $key (get $schedule $key) }}{{ end }}{{ end -}}
{{- $spec := dict "schedule" $schedule.expression "timeZone" $schedule.timeZone "suspend" $schedule.suspend "concurrencyPolicy" $schedule.concurrencyPolicy "jobTemplate" (dict "spec" $job) -}}
{{- range $key := list "startingDeadlineSeconds" "successfulJobsHistoryLimit" "failedJobsHistoryLimit" }}{{ if hasKey $schedule $key }}{{ $_ := set $spec $key (get $schedule $key) }}{{ end }}{{ end }}
- op: set
  path: "{{ $i }}.apiVersion"
  value: batch/v1
- op: set
  path: "{{ $i }}.kind"
  value: CronJob
- op: set
  path: "{{ $i }}.spec"
  value: {{ toJson $spec }}
{{- else }}
- op: set
  path: "{{ $i }}.spec.template"
  value: {{ toJson $pod }}
{{- end }}
{{- else if and (eq $m.kind "Service") (eq $m.metadata.name $in.identity.workload) (get $e "headless") }}
- op: set
  path: "{{ $i }}.spec.clusterIP"
  value: None
{{- end }}
{{- end }}
{{- else }}[]{{- end }}
