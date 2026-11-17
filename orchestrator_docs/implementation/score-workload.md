---
id: IMP-SCORE-WORKLOAD
artifact: implementation-handoff
status: current
last_reviewed: 2026-11-17
---

# Score authoring — issue #83

Accepted and integrated into main at `8651a603`, with retained kind Console
rollout verified. See [main integration](../verification/2026-11-17-score-main.md).
Required behavior is owned by [SC-01–06](../architecture/contracts/score-workload.md); this record maps
that contract to implementation and its verification limits.

## Implementation

- `backend/internal/planning/workloadspec/score_adapter.go` validates against a
  locally embedded, commit-pinned upstream Score v1b1 JSON schema. Converts the
  supported Score and Humanitec-format extension subset into Generic Workload.
  Export validates normalization equivalence and preserves native advanced fields
  explicitly in `spec.orchestrator`. No source-file reads or schema network fetch.
- `backend/internal/application/workloadconfig/score.go` uses the existing
  five-field matcher and selected Definition revisions read-only. Profile lookup
  is restricted to the winner's source, freezes ID/version/SHA256 and uses the
  existing capability, input and template-owned-default checks.
- `backend/internal/delivery/http/workloads.go` exposes scoped parse/export APIs.
  Known schema/unsupported/profile error codes never return raw schema values.
  `Save` admits direct Score input through the same scoped profile resolution.
- `frontend/src/features/workloads/WorkloadEditorPage.tsx` restores import,
  adds optional extension upload and current-edit export. Superseded parse replies
  are ignored; failed replacement cannot save an earlier import. The parsed
  internal document is in a collapsed disclosure; shell/form patterns are kept.
- Existing normalized draft/Deployment Set storage and legacy readers remain.
  There is no SQL DDL, bulk rewrite, reset, Terraform driver, graph, matching
  algorithm, placement, CD/Fleet or rendering-engine change.

## Supported boundary

This is a validated Score input implementation with an explicitly bounded feature
subset, not complete Humanitec compatibility. Supported mappings and profile
resolution are in the contract. Score files/volumes, custom metadata/resource
metadata, unsupported probe fields and unsupported extension fields are rejected,
not ignored. Native advanced authoring remains available through the form and
namespaced export extension. No Job workload profile is introduced.

## Tests and evidence

- `score_adapter_test.go`: equivalent planner fragments, schema/unsupported guards,
  CronJob conversion, zero/false, advanced export round trip and extension isolation.
- `workloadconfig/score_test.go`: selected-source isolation, version/pin stability,
  defaults snapshot and no-write admission.
- `http/workload_score_test.go`: scoped export, advanced preservation, rejected
  unsupported/multiple-document imports and no draft writes.
- `ScoreDocuments.test.tsx`: two-file import, stale reply rejection, invalid
  replacement, export and bidirectional tab synchronization, invalid input,
  stale replies and preservation of pins/defaults/refs. Existing authoring suites retained.
- [Browser and kind scenario](../../e2e/scenarios/score-authoring/README.md).
- [Verification](../verification/2026-11-16-score-authoring.md) records full
  candidate/main comparison, real runtime outcomes and cleanup status.

The full backend baseline is not green. Its failures must not be described as
passing merely because candidate and main have identical failure names.
