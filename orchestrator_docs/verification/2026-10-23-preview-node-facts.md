---
id: VERIFY-PREVIEW-NODE-FACTS-20261023
artifact: verification-report
status: current
last_reviewed: 2026-10-23
---

# Preview node facts and connector readability

User requested more space between cards/arrows and useful facts such as database
name/port. They subsequently confirmed sidebar expansion already worked; no
sidebar implementation change was made. Direct browser checks also exercised
reopening while scrolled inside Preview at desktop widths.

UC-05 VIS-09, realization/screens/states and the shared review contract define
the additive safe facts before implementation. No new use case or execution
workflow is introduced.

## Delivered behavior

- Node dimensions are shared with routing (252×152), with 64px horizontal gaps,
  a minimum 108px row corridor that grows with actual connector tracks and a
  minimum display scale of .85. Scrolling retains readable cards rather than
  forcing the whole dense graph into tiny text.
- One connector still retains every original reason for its endpoint pair.
  Unique horizontal tracks and outside lanes replace the common middle bus
  and modulo-three 6px lanes. Attachment columns are reserved per row corridor
  so aligned source/target cards do not merge their vertical stems.
- **Chỉ đường nối của node đang chọn** temporarily isolates incident connectors;
  clearing it restores the complete graph. Nodes, graph data, reasons and
  scheduling remain unchanged. A visible scroll hint accompanies the control.
- Each card has two prioritized facts; Overview has the fuller list. The live
  kind sample shows PostgreSQL database `acceptance`, port `5432`; Redis cache
  ID `shared.acceptance-cache`, port `6379`; the generated namespace name; and
  workload image/port `8080`, with replicas/requests available in Overview.
  Cluster/network use public account/region/class metadata.

## Safety and compatibility

The backend projects only bounded semantic fields from captured validated
Terraform input contracts. Sensitive/executor-owned nil values and unclassified
sources are omitted; unresolved expressions become UNKNOWN without copying the
expression. Ports require a verified pinned root HCL module's literal integer
output mapped to public `port`, within 1..65535 and not secret-classified.
Functions, references, strings and invalid numeric ports never become guessed
values. No provider/Terraform command, state or secret read was added.

Frontend workload facts use only sanitized Deployment artifacts, covering all
containers' port/image fields; env values and arbitrary Terraform source are not
parsed for facts. Older nodes without properties retain public metadata. Facts
describe desired configuration, not runtime readiness. Existing state badges,
admission/ownership, baseline, immutable-plan and progress rules remain.
Properties are additive inside the existing version1 review JSON; no relational
schema or migration changed.

## Actual runtime verification

Both images were built from this checkout. Before replacing only the owned
`preview-kind` backend, all seven Applications' Environment metadata showed no
active operation. After restart the complete metadata response was unchanged.
PostgreSQL/Vault/cluster workloads were preserved. Frontend assets were refreshed
without restarting nginx; the original 18093 memory demo was not reset.

Real Chromium ran standalone update Preview on the existing kind sample with
matching before/after Scores. At both 1512px and 1280px it verified database name/
port and Redis port, selected node/file preservation, menu preference/navigation,
connection-focus/restoration and no page exception. Scoped deployment histories
were identical before/after. No draft/config/catalog write or Deploy ran.

Every SVG path was sampled against all unrelated node rectangles, and all
horizontal/vertical segments were compared across connectors. Final observations
show zero card crossings and zero overlapping connector segments. An intermediate
observer found two shared vertical stems even after horizontal lanes were split;
the per-corridor attachment reservation fixed them, with a dedicated regression.
Failed geometry logs are retained outside Git.

Validation passed: `go test ./...` and `go build ./...`, backend safe-projection
tests, **841 frontend tests / 66 files** (159.64s), 23 focused geometry/facts/
component tests, TypeScript/build, affected lint, Docker images, docs checker and
diff checks. PostgreSQL opt-in suites were not enabled
for this rendering/projection change; SQL/schema/adapters are unchanged, and the
running real PostgreSQL metadata reopen/history check passed. No AWS/new cluster
deployment or cloud acceptance is claimed. Full-directory frontend lint retains
48 preexisting findings, including the user's untracked
`frontend/test/e2e/deploy-acceptance-video.mjs`, which remains untouched/uncommitted.

Private evidence: `/tmp/preview-detail-20261023/` (Go/frontend/build/lint logs,
idle preflight/rollout, browser observations, failed and corrected geometry).
Public reviewed captures:

- [Full graph](artifacts/preview-node-facts-20261023/full-1512.png),
  [selected connections](artifacts/preview-node-facts-20261023/focused-1512.png).
- [1280px full graph](artifacts/preview-node-facts-20261023/full-1280.png),
  [selected connections](artifacts/preview-node-facts-20261023/focused-1280.png).
- [Browser geometry/state observations](artifacts/preview-node-facts-20261023/browser.json),
  [runtime rollout](artifacts/preview-node-facts-20261023/rollout.json).

No credentials, secret values or Terraform state are published.
