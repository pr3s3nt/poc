---
id: VERIFY-2026-10-11-T18-RENDER-BUNDLES
artifact: verification-record
status: current
last_reviewed: 2026-10-11
---

# T18 — Installed bundle list and Console selector

## Scope and outcome

Branch `refactor_ui`; RD-08/UI-02 integration. Canonical UC-03 BR-15/18/19,
OC-04b, rendering contract, realization/sequence/VOPC/shared class diagram and
UI screens/states/API mapping were updated before Claude implementation in the
task-owned tmux session. ADR-010 and planner ranking/criteria remain unchanged.

GET `/api/v1/render-bundles` requires PE/Admin and session Organization, reads
only the immutable installed registry, and returns sorted ID/version/AVAILABLE
metadata; invalid entries are omitted and empty registry is `[]`. Registration
and discovery share the same availability predicate. No path/digest/binary/
source/credential field is returned. Registration independently revalidates.

Console replaces bundle text input with an API selector. Single bundle is
selected only for an unselected draft; multiple require explicit selection.
Loading/error/empty disable submit with Vietnamese guidance/retry. Refresh and
late responses preserve draft and selection; unavailable selected options stay
visible and cannot submit. POST shape remains workload/score-k8s/internal-k8s
with render_bundle and existing criteria. T03 and live helper locators adapted.

## Local checks

- V-D: documentation checker and git diff --check pass; changed PlantUML
  diagrams pass and tracked PNGs regenerated.
- V-FE: typecheck, lint, 386 tests across 20 files, build pass.
- V-BE: full go test ./... and go build ./... pass with PostgreSQL/kind opt-ins
  unset; full suite includes conformance pass. No fixtures changed.
- Codex independently ran uncached `go test ./internal/delivery/http -run
  TestRenderBundles -count=1` with external opt-ins unset: pass.
- Codex independently reran the final RenderingTemplatesPage.test.tsx: all
  19 tests pass.
- `bash -n backend/test/integration/refactor-ui-local.sh`, `node --check` on
  refactor-local.mjs and template-engine-kind-human.mjs: pass.

Logs: `/tmp/poc-T18-handoff-20261011/`; `report.md` records exit 0 for all
V-FE/V-BE gates and headed T18/T03/T01. External SQL integration was not enabled.
Vite reports the existing large-chunk warning; build exits 0.

## Headed browser evidence

T18 run `/tmp/poc-T18-evidence-T18-221418`: `result.txt` PASS;
`assertions.txt`, `marks.json`, `captions.srt`, `captions.ass`, `playwright.log`,
`T18-raw.mp4` and `T18-captioned.mp4` retained outside Git.

Product setup used UI: Developer creates an application and workload, PE opens
Mẫu dựng ứng dụng and registers a template scoped to the application/environment,
then Developer opens pending Preview. API reads only support assertions.
The backend is the real local product with fake infrastructure/delivery and a
score-k8s test stub. List with one installed bundle, unavailable POST 400,
registration and matching Preview use that backend. GET empty/error/multiple
bundle responses are explicit page.route projection mocks, including a bundle
not present in the backend registry. No real renderer binary was installed or
executed for T18; Preview proves selection and zero stub generate calls.

Assertions cover anonymous 401, Developer 403, safe metadata, zero/loading/error
submit blocks, n requires selection, unavailable POST retains ID/bundle/criteria
without insert, stale selected option survives refresh and disables submit,
valid registration payload and single-bundle next draft, and selected template
`tpl-t18` in Preview. Unit tests additionally cover selection preservation
through delayed refresh/error/empty and no automatic mutation retry.

Codex full decoded captioned video with ffmpeg: exit 0. Sampled empty, stale
selection and selected Preview result frames: UI visible, cursor present,
Vietnamese caption strip beneath browser, no credential exposure. Raw video
metadata: 1440×900, 15 fps, 168.6 seconds; caption strip enlarges final height.
Review frames in `/tmp/poc-T18-handoff-20261011/review-*.png`.

## Regression evidence

- T03 `/tmp/poc-T18-evidence-T03-221735`: PASS; bundle input scenario adapted
  to selector and one explicit extra-bundle projection mock. Real local POST
  rejects unavailable bundle; native/selected Preview and intentional selected
  render failure keep existing no-fallback behavior with fake adapters/stub.
- T01 `/tmp/poc-T18-evidence-T01-222223`: PASS; sign-in/navigation smoke.
- Each directory retains raw/captioned videos, captions, marks, assertions and
  result. Codex full decode of both captioned regression videos exits 0.

## Limits and ownership

No DB/cluster/cloud/Vault mutation, retained K8S-4F changes, SQL opt-in or live
helper replay. No live renderer/provisioning proof. Evidence stays outside Git;
no upload. Working tree initially clean, so no pre-existing user changes were
staged. Task-owned tmux session is closed after final commit/push.
