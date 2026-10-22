---
id: ADR-019
artifact: architecture-decision
status: current
last_reviewed: 2026-10-23
---

# ADR-019 — Visual deployment review and persisted progress

## Decision and scope

User accepted the desktop prototype on 2026-10-23 and requested canonical design
then implementation through a dedicated Codex `gpt-6-sol` / `medium` tmux session.
Extend UC-05/06/07/08/09; do not add a new use case. Internal and cloud are
variants of the same graph, not execution modes selected by the Developer.
The [review contract](../contracts/deployment-review.md) owns shared DTOs,
artifact boundaries, correlation and progress mapping.

Keep the planner pure. After closing the read snapshot, an application-level
ReviewAssembler may read already verified, pinned module content and invoke an
offline preview capability of the selected installed renderer. No Terraform,
provider, secret store, module fetch, apply or runtime query occurs in Preview.
This narrowly supersedes UC-05's former ban on all renderer invocation; ordinary
execution rendering still requires completed outputs and remains unchanged.

Review code is a sanitized projection with provenance and availability, not an
executable download or a promise of final provider changes. Unknown values stay
symbolic; unavailable baseline is never an empty existing document. Capture safe
review snapshots during admitted execution so subsequent diffs use the exact
successful baseline; never reconstruct historical code from current Definitions.

UC-09 maps persisted node/attempt/workload facts to display states. Browser timers
may update elapsed time, never synthesize provisioning/readiness. A fresh scoped
query supplies the states; failed refresh preserves last observation as stale.
No new executor concurrency, lifecycle state or retry policy is introduced.

## Rationale and consequences

The graph answers what changes and why; node-linked source/diff answers how.
Pinned provenance and honest partial artifacts avoid a separate fake preview
renderer. Immutable safe snapshots make history useful without exposing state,
credentials or secret values. Rendering outside transactions preserves ADR-002.
The source projection needs strict limits, structured redaction and cache-pin
verification; unavailable content must be explicit rather than guessed.

The prototype's fixtures, scenario selector, fake clock, pause/reset and simulated
failure controls are review aids only. Production uses real matched graph,
accounts/placement, existing deploy admission and persisted execution status.
Approval of real Terraform plans, estimates, drift, new providers, multi-cluster
execution, new mobile design and cloud deployment verification are out of scope.

## Delivery

This is accepted design, not an implementation claim. See
[implementation handoff](../../implementation/visual-deployment-review.md) and
[IMP-029](../../implementation/deviations.md#imp-029--visual-deployment-review-designed-not-yet-implemented).
