---
id: CONTRACT-DEPLOYMENT-PROGRESS
artifact: shared-contract
status: current
last_reviewed: 2026-11-11
---

# Deploy and Recovery progress — issue #69

UC-09 DP-01–05 own behavior. This contract adds optional `progress` to the
existing scoped deployment detail and operation DTOs; detail also returns
`operationStatus`. No new endpoint, permission, execution decision or observer
loop is introduced. Poll persisted GET views every three seconds while active.
A terminal Deployment may precede claim release: keep polling while its operation
is ACTIVE/RECOVERING; never infer Environment unlock from Deployment status.

`progress` version 1 contains generic `steps`, optional `checks` and the prior
worker's `priorHeartbeat`. A step has stable `id`, optional `parent`, `workload`,
`deploymentId`, `kind`, `status`, `reason`, start/update/finish timestamps and
optional readiness `deadline`. PENDING has no execution duration; entering
RUNNING starts its clock. Terminal timestamps remain fixed on repeated writes.
The structure supports actual phase ordering, including Environment-wide Fleet
bundles, without inventing sequential per-workload publications. Adjacent steps are grouped
by workload while retaining execution order; shared release steps never move
ahead of earlier workload preparation merely because their group is Environment.

Git publication and Fleet apply have separate steps: a receipt does not prove
that Fleet applied the revision. Fleet permission/authentication failures use
fixed safe reason codes; controller message text never reaches the API. A failed
read does not replace the last successful observation.

Evidence may include validated resource kind/name/namespace, Git commit,
current-revision pod name/image/reason/restart count, Kubernetes event
object/reason/time/count and structured Terraform stage logs. No raw CLI or
provider text is stored here. Stage logs contain timestamp plus allowlisted
stage/status, so a CLI value cannot bypass redaction. Events are reconstructed
from safe fields rather than publishing their arbitrary message text. Latest
three events are visible by default, with all retained events in a disclosure.
Bounds are 100 pods, 50 events and 100 structured log entries per step; the
public projection bounds steps/checks at 1000 each.

The operation lease serializes progress, Stage and heartbeat writes through the
existing owner/fence. Stage retains the progress key and preserves the prior
replacement semantics for other detail. Observability writes do not change an
execution result. The existing heartbeat retries the latest in-memory snapshot
if a best-effort progress write fails; lost ownership still rejects all writes.
A stopped worker's last successfully stored observation is explicitly historical.

Deployment history reads the operation ID pinned into its plan, or its exact
review request before a plan exists. It never substitutes the current operation.
Legacy records without correlation have no timeline. Recovery adds checks to the
adopted operation and preserves the Deploy timeline. Everyone with existing
Environment read permission can see the conclusion, required action and checks;
existing Recovery authorization and stopped-worker confirmation remain intact.
