---
id: CONTRACT-DEPLOYMENT-REVIEW
artifact: shared-contract
status: current
last_reviewed: 2026-10-23
---

# Deployment review artifacts and progress

## Ownership

[ADR-019](../decisions/ADR-019-visual-deployment-review.md) extends Preview and
observation without changing planner matching or execution admission. Required
user behavior belongs to UC-05 `VIS-01..08`, UC-09 `OBS-01..07` and UC-06/08
`REV-*`. This contract owns the common projection and storage semantics.

## Review assembly and identity

1. Read one scoped, consistent planning snapshot including baseline deployment
   identities and their safe review artifacts. Plan using the existing pipeline.
2. Copy required immutable inputs; close the database read transaction before
   any renderer subprocess. Read verified local bundle content only by full pin.
3. Assemble `review` using that captured snapshot, exact plan and pinned renderer.
   No reload of mutable catalog/config/active state between nodes. Revalidate
   content digests against captured pins; missing content does not trigger fetch.
4. Pending Preview returns one deduplicated graph for the final Candidate Set,
   plus node artifacts and existing per-workload changes/token. Shared nodes use
   canonical descriptor/scope/generation, never display name. Conflicting node
   pins cannot be silently merged. Standalone Score Preview uses the same view
   contract but never receives deploy authority.
5. Existing stale-token checks remain authoritative. Rendering/review projection
   must not alter the internal Delta/Candidate or execution plan hash. Version the
   projection separately. Snapshot/bundle/Definition changes invalidate Preview
   under the existing admission rules; do not add a "deploy anyway" bypass.

## Additive public review shape (version 1)

`review` has `version: 1`, `planHash`, `nodes`, `edges`, `batches`, `artifacts`.
Empty collections are arrays; unavailable data carries an explicit reason.

| Member | Required meaning |
|---|---|
| node | `id` canonical descriptor, `kind` workload/resource, display name, type/class, Definition, ownership, change intent, safe module/renderer pin, placement and dependency provenance |
| placement | account key/region, cluster descriptor and namespace source when known; typed deferred provenance otherwise; no auth/endpoint bytes |
| edge | consumer, provider, reason and optional source path from actual planner DAG |
| artifact | stable `id`, `nodeId`, kind `terraform-source`/`terraform-inputs`/`kubernetes-manifest`/`binding`, safe relative display path, language, before/after sides |
| side | availability `AVAILABLE`, `PARTIAL`, `ABSENT`, `UNAVAILABLE`; safe text if present; provenance; unknown/redacted paths; allowlisted reason if unavailable |
| provenance | plan hash, Definition version, full module/revision/digest or renderer bundle pin, baseline deployment ID where applicable; no raw input/state/credential digest |

`ABSENT` means proven creation/removal in this plan, never missing history.
`UNAVAILABLE` reasons include `BASELINE_MISSING`, `CONTENT_UNAVAILABLE`,
`UNSUPPORTED_PREVIEW`, `LIMIT_EXCEEDED`, `REDACTION_REQUIRED` and
`RENDER_UNAVAILABLE`. Mixed artifact availability never erases a valid graph.
Node metadata remains usable when one file cannot be shown. Known unsupported
preview does not invent a renderer or falsely block otherwise valid existing
Deploy; normal planning/preflight errors still block under their own contracts.

Bounds per response: at most 128 artifacts, 256 KiB UTF-8 per side, 2 MiB total
text; render deadline at most 10 seconds per workload and 30 seconds per request.
Exceeded content is explicitly unavailable, never silently truncated into a
valid-looking diff. Limits apply before expensive diff work and response writes.
Stable sorting by node/path; no arbitrary file path from client input. HTML is
never interpreted. Diff work must also be bounded; UI may decline a very large
diff while retaining bounded after-text.

## Terraform source and inputs

Read only source files in the verified bundle's transitive allowlisted tree:
`.tf` and `.tf.json`, including validated local child modules. Verify pinned
content digest before serving; reject escaping paths, symlinks and oversized or
non-text entries. Never read execution workspace, `.terraform`, tfvars from disk,
state, plan binaries, lock/backend auth files, arbitrary URLs or latest Git HEAD.
No Terraform init/plan/show/provider calls are permitted for source inspection.

The view is explicitly **source at pinned revision, with redactions**. Structural
HCL identifiers/expressions and independently classified non-secret values can
be shown. Parse HCL/JSON structurally; mask sensitive defaults, credential-bearing
fields, unclassified literal values and comments that cannot be proven safe.
Do not rely solely on key-name regex, entropy or `sensitive=true`; opaque scripts,
templates or expressions that cannot be projected safely are unavailable. Mark
redacted spans and keep the original immutable pin separately; never label
sanitized text byte-for-byte original. Fixture tests must include literal secrets
in defaults, nested structures, comments and output expressions.

The separate input view uses Type/module metadata to expose safe values or
symbolic source mappings. Secret values, raw credentials, state references and
configuration values lacking non-secret classification remain masked. This is
not runnable tfvars. Editing/copying the projection cannot mutate a Definition.
A new use of an unchanged module is not a provider-create plan: source equality
and resource intent are displayed separately.

## Kubernetes preview rendering

Use the selected installed bundle and the same normalization, platform policy,
identity/probes/resources rules and output validation as execution. Add a preview
capability, not a second hardcoded YAML template or unselected renderer fallback.
Only an offline, output-only local renderer invocation is allowed: controlled
workspace, bounded subprocess, no inherited cloud/cluster credentials, network,
fetch or command provisioners. Cleanup the disposable workspace on all outcomes.

Bind only captured safe literal inputs and references. Unavailable dependency
outputs remain typed symbolic provenance. Use deterministic preview sentinels
internally only if their substitution cannot affect object structure; replace
sentinels in the public view with clear unknown markers and paths. If an unknown
would select resource shape/count/identity, return `UNSUPPORTED_PREVIEW` for that
artifact rather than invent values. Partial YAML is labeled non-executable.
Never resolve secret bytes to produce Preview. Secret objects/data/stringData,
unclassified container env literals, sensitive annotations and private backend
references are removed/masked structurally; preserve safe Secret name/key refs
only where existing visibility contracts permit them. No hash of hidden values.

Minimum supported preview coverage is Deployment/Service output of installed
workload bundles on Internal and EKS placements, with ordinary resource-output
and configuration-secret placeholders. Other objects are shown only if produced
and safely projected by their owner; no fabricated Ingress, PVC or database YAML.
Cloud resource modules expose Terraform source rather than invented K8s manifests.

## Baseline, diff and execution capture

For a workload, baseline is the last successfully applied workload snapshot in
the same Organization/Application/Environment and target generation, not the
latest failed deployment or current mutable Definition. Resource source baseline
uses its successful resource association and pinned plan/bundle. Missing baseline
is `UNAVAILABLE`; do not call a provider or regenerate historical manifests from
current bundles. Creation/removal uses an explicitly absent side. Changed target
or incompatible generation is not compared as an ordinary in-place update.

Persist sanitized desired review artifacts inside the admitted immutable plan's
`document.review` before leaving PLANNING. After execution render/validation and
before apply, store `manifest_review` with the owning deployment workload row;
label it rendered intent until that exact workload reaches READY. Promote only
that READY association as a successful applied baseline. Failure/partial batch
must not replace another workload's successful baseline. Do not modify manifest
bytes used for Apply, workload digest, ownership or secret delivery to obtain the
safe view. Keep new snapshots immutable after terminal status. Schema ownership
is [database schema](../database/schema.md).

Browser diff uses the two safe texts from the response: removed red/`−`, added
green/`+`, context unchanged, old/new line numbers. Missing/redacted/unknown text
must not be described as a confirmed provider change or secret-value equality.
A redacted path is "value hidden; change unknown", not "unchanged". Copy copies
only safe after-text, without deleted lines/gutters. Source and manifest tabs
remain linked to selected node. Provide expand/collapse and after-only view.

## Correlation while deploying

Existing pending `runId` is deterministic planning context and may repeat after
failure; it must NOT be the execution correlation identity. For each explicit
Deploy click the Console generates a fresh UUID `reviewRequestId` and sends it
alongside the existing Preview token. This optional request field is a correlation
key, not authorization, not an idempotent replay command and not a planner input.
Old CLI/API callers may omit it and retain their existing behavior.

On admission UC-06/07 validates UUID syntax, scopes it to the authenticated
Environment and atomically binds it to the claimed Environment operation and
validated token snapshot. Enforce unique `(environment_id, review_request_id)`
across admitted operations. A reused ID is a safe conflict, never another execute
or a silent replay. Persist the same ID on each Deployment created by that
operation before its external execution. Correlation persistence cannot weaken
operation CAS, ownership/fencing or stale Preview checks.

`GET .../deployments?reviewRequestId={uuid}` filters the existing list within
session Organization/Application/Environment. Query each returned exact
Deployment ID. Do not associate by newest timestamp, workload, image or the
possibly repeated deterministic planning runId. Before admission an empty list
is "waiting for execution record", not success. Keep the existing pending POST
lifecycle/report and Environment operation guards. If the POST response is lost,
read scoped operation/deployment state by the same request ID; do not replay POST.
Existing operation list projection may expose only this safe correlation ID and
status, not arbitrary operation detail. A fresh user retry requires new Preview
and new request ID.

A multi-workload graph retains per-deployment status/provenance; nodes not yet
admitted stay pending. Shared node observations must match identity and selected
plan pins; contradictory observations remain unavailable instead of taking the
most optimistic state. Existing operation/report governs batch completion and
route sync, not merely the number of deployments observed so far. Route-only
sync preserves its existing behavior and may have no Deployment records.

## Progress projection (not persisted lifecycle enums)

UC-09 returns `nodeProgress`: node ID, display state, source status/stage/attempt,
started/updated/finished timestamps when available, blocking provider IDs/reasons,
safe error code and `observedAt`. Read one consistent persisted snapshot. Do not
query provider/secret store/CLI. Display states are derived, not replacement
resource/deployment state machines.

| Display state | Evidence / rule |
|---|---|
| PENDING | Known planned node with no start; no failed dependency |
| WAITING | Not started and known required provider not yet ready; list provider IDs |
| PROVISIONING | Matching running resource attempt before readiness, or workload APPLYING; show exact safe stage |
| CHECKING | Persisted resource READINESS stage; workload remains generic applying until readiness-specific evidence exists |
| READY | Matching node/workload readiness completed; Terraform APPLIED/OUTPUTS alone is insufficient |
| RETAINED | Explicit verified retain/preexisting binding evidence for this plan; must not imply fresh provider refresh |
| FAILED | Matching failed/interrupted attempt/node; retain safe code and completed peers |
| BLOCKED | Proven failed provider blocks a not-started consumer, transitively; preserve existing blocked view |
| UNKNOWN | Missing/unsupported/inconsistent historical facts; never infer READY from absence |

Explicit failed/running observations take precedence over derived waiting/retain.
Plan intent and execution status remain independent. Update/retry observations
are scoped to exact attempt; a prior successful attempt cannot mask current failure.
No simulated percentage/ETA. Count ready/retained versus total known nodes and
label as a count, not work completion percentage. Elapsed uses persisted start
and terminal timestamps; missing timestamps display unavailable. Browser time may
animate elapsed for a known active attempt but never advance its state.

Poll read-only detail about every 2 seconds while running, single flight, cancel
on scope/run change/unmount and stop on terminal response. On read errors keep
last data with stale/error indication; retry GET only. Display independent running
nodes concurrently only when actual records show concurrency. Do not alter the
scheduler to mimic the prototype. No pause/reset/failure selector in product UI.
