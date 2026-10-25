# Observability acceptance workload

A small Go HTTP application for real Kubernetes logging/monitoring verification.
It uses ordinary JSON stdout/stderr, with no collector, sidecar or telemetry SDK.
It is not the Orchestrator Web Console.

The page lets the tester emit INFO/WARN/ERROR records, emit a 120-line batch,
start/stop real CPU and RAM load, and restart the process. Load automatically
ends after two minutes. The acceptance image sets `GOMEMLIMIT=240MiB`; the Score
uses a 250m CPU / 256Mi memory limit. The load keeps 208Mi of touched memory and
runs one CPU worker so Metrics Server can observe an actual high sample. Stop
releases/scavenges that memory; no OOM or failed deployment is required.

`REVISION` identifies a normal workload update; every process boot also logs
its own identifier. Kubernetes timestamps, Pod UID and container restart index
remain collector facts. `/healthz` and `/status` support independent observations;
activity in the real scenario is triggered by the page's buttons.

Run through [the real-cluster acceptance runner](../../test/integration/observability-kind.sh).
The runner builds this binary/image, loads it only into its new owned kind
cluster, and removes its app/resources through the Console before platform
teardown. Product logs and metrics are neither stubbed nor seeded.
