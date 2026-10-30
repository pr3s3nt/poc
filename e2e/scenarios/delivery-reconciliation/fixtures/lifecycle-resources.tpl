{{- if eq .Input.identity.workload "backend" -}}
{{- $env := list -}}
{{- range $m := .Manifests -}}{{- if eq $m.kind "Deployment" -}}
 {{- range $c := $m.spec.template.spec.containers -}}{{- range $e := $c.env -}}
  {{- if hasKey (dict "PGHOST" true "PGPORT" true "PGDATABASE" true "PGUSER" true "PGPASSWORD" true) $e.name -}}{{- $env = append $env $e -}}{{- end -}}
 {{- end -}}{{- end -}}
{{- end -}}{{- end -}}
{{- $image := "postgres:16.14-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777" -}}
- apiVersion: batch/v1
  kind: Job
  metadata:
    name: backend-migration
  spec:
    backoffLimit: 2
    template:
      spec:
        restartPolicy: Never
        containers:
          - name: migrate
            image: {{ $image | toJson }}
            imagePullPolicy: IfNotPresent
            command: ["psql", "-v", "ON_ERROR_STOP=1", "-c", "CREATE TABLE IF NOT EXISTS fleet_migration_receipts (version integer PRIMARY KEY); INSERT INTO fleet_migration_receipts VALUES (1) ON CONFLICT DO NOTHING;"]
            env: {{ toJson $env }}
- apiVersion: batch/v1
  kind: CronJob
  metadata:
    name: backend-maintenance
  spec:
    schedule: "* * * * *"
    concurrencyPolicy: Forbid
    successfulJobsHistoryLimit: 2
    failedJobsHistoryLimit: 1
    jobTemplate:
      spec:
        template:
          spec:
            restartPolicy: Never
            containers:
              - name: maintenance
                image: {{ $image | toJson }}
                imagePullPolicy: IfNotPresent
                command: ["psql", "-v", "ON_ERROR_STOP=1", "-c", "SELECT version FROM fleet_migration_receipts WHERE version=1;"]
                env: {{ toJson $env }}
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: backend-retired-marker
  data:
    release: first
{{- else if eq .Input.identity.workload "frontend" }}
- apiVersion: autoscaling/v2
  kind: HorizontalPodAutoscaler
  metadata:
    name: frontend
  spec:
    scaleTargetRef:
      apiVersion: apps/v1
      kind: Deployment
      name: frontend
    minReplicas: 2
    maxReplicas: 3
    metrics:
      - type: Resource
        resource:
          name: cpu
          target:
            type: Utilization
            averageUtilization: 80
{{- else }}
[]
{{- end }}
