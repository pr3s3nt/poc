---
id: SCORE-WORKLOAD-CONTRACT
artifact: operation-contract
status: current
last_reviewed: 2026-11-16
---

# Score workload admission — issue #83

## SC-01 — Boundary

Score `score.dev/v1b1` is the portable authoring input. An optional Humanitec-format
extension `humanitec.org/v1b1` carries `profile` and `spec`. Admission converts both
into the existing Generic Workload document before Save, Preview and Deploy.
Generic Workload remains an internal and backwards-readable format. There is no
Score renderer process, Helm dependency or change to infrastructure algorithms.
Unsupported fields/features fail explicitly; acceptance never silently drops them.
The supported subset is documented here, not a claim of complete Humanitec support.

## SC-02 — Conversion

| Input | Existing normalized field |
|---|---|
| Score metadata.name/annotations | metadata.name/annotations |
| Score containers image/command/args/variables/resources | containers with identical values |
| Score readinessProbe/livenessProbe HTTP or exec | existing Probe action representation |
| Score service.ports | service.ports |
| Score resources type/class/id/params | resources, preserving dependency identity |
| Extension spec.deployment.replicas/strategy/revisionHistoryLimit | replicas / workloadExecution controller settings |
| Extension spec.pod.securityContext/automountServiceAccountToken/serviceAccountName | workloadExecution fields |
| Extension spec.containers.<name>.securityContext | named container securityContext |
| Extension spec.schedules.<name>.schedule and container command/args | workloadExecution.schedule and container overrides |
| Extension spec.cronjob timeZone/suspend/concurrencyPolicy/deadline/history | workloadExecution.schedule fields |
| Extension spec.orchestrator | explicitly namespaced lossless native authoring fields |

Score is validated against the upstream versioned schema before conversion.
Unsupported valid Score features (including unresolved local file sources) fail
rather than triggering host file reads, network access or silent truncation.
The supported `${metadata.name}` placeholder is expanded into variables/params before the
existing reference conversion. Resource output placeholders keep existing guards.
Annotations remain authoring metadata, not arbitrary Kubernetes object metadata.

## SC-03 — Profile and pins

`profile` identifies a template ID, optionally `ID@version`; `humanitec/default-module`
and `humanitec/default-cronjob` are compatibility aliases for Deployment and CronJob.
There is no Job compatibility alias because the product has no general Job workload.
Without an explicit profile, use the default-module behavior. A profile resolves
only against templates exposed by the already selected/matched workload Definition's
source. Default aliases require a unique compatible template; ambiguity is an error,
never arbitrary selection. An unversioned explicit ID resolves to its greatest
numeric semantic version in that source, then freezes ID/version/SHA256 at admission.
Reopening a draft or deployment never upgrades the pin. Existing planner capability,
selected-source and pin checks run before provisioning. Profiles do not change
workload class, Definition matching, resource criteria or placement.

`spec.orchestrator` preserves native authoring configuration not expressible in
this subset: metadata.class, container advanced fields, replicas and
workloadExecution. It cannot replace Score identity/image/variables/resources/service,
authorize extra targets or inject templateDefaults snapshots. Overlapping authored
values between native and Humanitec fields are rejected, not ordered implicitly.
Export includes an exact template pin when present; import verifies that pin.
The `orchestrator/custom` profile preserves unpinned native authoring without
inventing a preset or defaults. Unsupported metadata beyond name/annotations,
metadata placeholders beyond name and resource metadata are rejected explicitly.

## SC-04 — Round trip and API

Scoped POST workloads/parse accepts {content, extensions?}, where both strings are
single YAML/JSON mappings. It returns {workload}, the normalized document. Scoped
POST workloads/export accepts {workload} and returns {score,extensions}, YAML strings.
Export validates both sides and lossless normalization before returning data; no
secret resolution/provider calls. Unsupported export is an error, not partial output.
Safe HTTP 400 codes distinguish SCORE_SCHEMA_INVALID, SCORE_UNSUPPORTED and
SCORE_PROFILE_INVALID; raw schema errors, values and source credentials are not
returned to clients.

Stored drafts and Deployment Set snapshots continue to use the existing internal
model. No SQL migration, bulk rewrite or installation reset is part of this change.

The Console restores Import, accepting Score plus an optional extension. A failed
parse keeps the files/edits available and blocks saving stale imported content.
Export acts on the current authored document, not stale imported input. Existing
advanced native documents remain readable; the unified form remains the default.

## SC-05 — Defaults, safety and regression

Template-owned defaults and snapshots follow ADR-039; no backend default policy is
added. Explicit zero/false and absence remain distinct. Unknown outputs, secrets,
shared resources, ownership, stale Preview and immutable source checks stay intact.
For equivalent supported inputs, admission must produce the same Fragment and
therefore unchanged dependencies, matching, graph, classifications and batches.
Run focused conversion, HTTP, authoring, planner and rendering tests, then isolated
kind full checks and real browser/runtime acceptance. Do not claim runtime support
from parser tests alone.

## Sources

- [Score schema](https://github.com/score-spec/spec/blob/main/score-v1b1.json).
- [Humanitec extensions](https://developer.humanitec.com/platform-orchestrator/docs/platform-orchestrator/score/).
- [Humanitec profiles](https://developer.humanitec.com/platform-orchestrator/docs/integration-and-extensions/workload-profiles/overview/).
- [Generic Workload](generic-workload.md), [defaults](workload-defaults.md).

## Direct document entry — issue #83 refinement, 2026-11-16

Users can type/paste Score and optional extension YAML/JSON directly in the
existing import tab. File selection fills the same editable text. A Check action
uses the existing scoped parse API; edits immediately invalidate earlier results
and block stale Save/export until the current content passes validation. File
imports retain their existing automatic check. Empty extension means no extension.
No backend, graph, matching or provisioning behavior changes.
