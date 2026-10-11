---
id: VERIFY-2026-10-11-T19-MATCHING-PREVIEW
artifact: verification-record
status: current
last_reviewed: 2026-10-11
---

# T19 — Exact single-context matching preview

## Outcome and scope

Branch `refactor_ui`; RD-06 backend and both Console catalog forms. Codex updated
UC-03 BR-20–22/specification/realization/sequence/VOPC/UI, UC-05 boundary,
OC-04c, shared class diagram and planner reference before Claude code/test work
in task-owned tmux `codex-T19-20261011`. No match.go or reference fixture changes.

POST `/api/v1/resource-definitions/matching-preview` authenticates PE/Admin in
session Organization, decodes a bounded strict candidate/context and computes
from one scoped read-only snapshot of app/env/READY target/types/definitions.
Candidate replaces only the same key in memory. Domain BestCriterion and
ConnectionMismatch keep weights/profile/winning ties and conflict guards.
Response has eligibility, best criterion/score, winner/ties/conflict, incomplete,
exact implicit-node system binding/native renderer and sorted content marker
with Environment version. No writes, system admission, driver inspection,
rendering, secret resolution or external provider call; snapshot SQL reads are
permitted. Registration/full graph Preview independently validate again.

Shared panel in resource/template forms submits unsaved candidate and explicit
sample context only on action. Input/metadata/scope/session changes invalidate
pending/result; transport errors remain separate, retry explicit. INCOMPLETE
shows unassessed candidates rather than claiming no match; SYSTEM_BINDING does
not imply authored criteria control the implicit node. Values/payload unchanged.

## Checks

All listed checks exit 0; commands/statuses retained at
`/tmp/poc-T19-handoff/checks.txt`.

- V-BE: full uncached `go test -count=1 ./...` and `go build ./...`; PostgreSQL
  and kind opt-ins unset. Full conformance suite passes, fixtures untouched.
- V-FE final: typecheck, lint, 399 tests across 21 files, build. Existing Vite
  large-chunk warning remains nonfatal.
- Shell runner `bash -n`, e2e `node --check`, gofmt check and whitespace pass.
- V-D documentation checker, PlantUML syntax checks and regenerated PNGs pass.
- Codex independently ran uncached catalog/HTTP matching tests and final
  whitespace tests with opt-ins unset: pass. Independently ran panel tests
  before the final wording fix: 12 pass; final extra wording assertion is in
  the full 399-test pass verified from logs and source.
- V-UI T19 and T02B/T03 headed regressions pass.

No PostgreSQL opt-in, kind, AWS, retained Vault/DB/cluster run or live renderer
proof. T19 needs no persistent schema change; no external mutation authorized.

## Browser evidence and review

Final T19: `/tmp/poc-T19-final2-T19-20261011230907`; `result.txt` PASS.
Raw/captioned MP4, captions.srt/ass, marks.json, assertions.txt and Playwright
logs retained outside Git. Run ID is in run.json. Captioned video is 1440×996,
180.6 seconds, 2709 frames. Full ffmpeg decode exit 0; reviewed wildcard,
class-specific, exact tie, non-overlap, incomplete and transport failure frames.
Final incomplete/tie frames in `/tmp/poc-T19-handoff/final-*.png`; earlier
multi-phase review sheet also retained there. Cursor/click and UI are visible;
Vietnamese captions below browser do not cover controls or expose credentials.

Setup uses UI to create app and bind staging to fake internal-cluster Connection;
setup is labelled separately. Product steps use UI to enter candidate, choose
context, check matching, edit criteria, explicitly register the first Definition,
then check an unsaved second Definition. API reads serve assertions only.
Backend is the real local product with fake adapters/disposable JSON state.
Matching statuses come from that backend. Only the transport-error step uses
page.route mock 500; assertions/caption disclose it. No preview persists the
unsaved candidate; one explicit UI registration is separately asserted.

Coverage: wildcard/shared tie and AWS profile exclusion; class score 16 wins
above shared score 0; exact highest-score tie; non-overlap; missing resource ID
returns INCOMPLETE without winner/ties; safe transport failure and explicit retry.
Unit/service tests additionally cover all-five score 31, snapshot/no-write spy,
scoping/READY target/kind consistency, marker ordering/content changes, winner
Connection conflicts including VPC/EKS versus Aurora external accounts, reserved
keys, exact system node, native renderer, late response and safe error envelopes.

Regression evidence:

- T02B `/tmp/poc-T19-regress-T02B-20261011230013`: PASS, criteria modes/copy/
  advanced preservation and frontend assistive preview unchanged.
- T03 `/tmp/poc-T19-regress-T03-20261011230304`: PASS, native/selected renderer
  Preview and selected render failure without native fallback; fake deliverer/
  score-k8s failure stub. No live rendering/provisioning claim.
- Both captioned regression videos full-decode exit 0.

Earlier T19 evidence remains: `/tmp/poc-T19-evidence-20261011224641`,
`/tmp/poc-T19-evidence-20261011224819-b` failed due to scenario locator/mode-click
assumptions; `...20261011225021-c` and `...20261011225515-final` passed before
later whitespace/wording corrections. Nothing deleted or uploaded. All paths
are listed in `/tmp/poc-T19-handoff/evidence-path.txt`.

## Ownership and limitations

Working tree initially clean; no pre-existing user-owned changes staged.
Runner cleanup traps stop only owned processes and retain evidence. Task tmux
is closed at final handoff. K8S-4F/retained apps untouched. Exact preview validates
matching structure, not full driver inputs/output contracts or whole graph;
marker is snapshot evidence, not Save/Deploy authorization or freshness token.
