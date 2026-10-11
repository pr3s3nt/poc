---
id: VERIFY-2026-10-12-T20-CONSOLE-ERRORS
artifact: verification-record
status: evidence
last_reviewed: 2026-10-12
---

# T20 — Public error codes and Vietnamese Console guidance

## Scope

VI-03 and VI-04/05 error coverage on branch `refactor_ui`. Initially clean
working tree; no user-owned changes. Codex updated the public HTTP error
contract, shared UI presentation and affected UC-00/01/02/03/04/05/07/09/12/16
UI mappings/states before Claude code/test implementation in task tmux
`codex-T20-20261011`. No database schema, matcher ranking, fixtures or retained
K8S-4F changes; no external mutation.

HTTP handlers share a typed/sentinel classifier and fixed safe error envelope.
Status/code/allowlisted field, registered Type and planner stage/reason metadata replace raw
exception/provider/decoder strings. Unknown errors fail closed; submitted
values/unknown keys cannot enter error responses. Client ignores wire `error`,
validates code/field/params and chooses hard-coded Vietnamese guidance.
BUSY and stale are distinguished; edits remain, mutation retry is explicit.
Failed authoritative reload blocks another stale write without falsely claiming
reload success; scope switch resets the gate. Selected renderer behavior and
internal planning classification/conformance remain unchanged.

## Validation

Logs outside Git in `/tmp/poc-T20-handoff-20261011`:

- `v-be-test.log`: full Go suite including conformance, exit 0; `v-be-build.log`:
  build exit 0. PostgreSQL/kind opt-ins unset for local checks.
- `v-fe.log`: typecheck/lint/425 tests across 22 files/build, exit 0; existing
  nonfatal Vite large-chunk notice.
- `bash-n.log`: changed local refactor runner and kind-video assertion script
  syntax pass; kind script not executed against a cluster.
- Codex independent uncached Go HTTP/catalog/connection/workloadconfig/Score
  tests pass; frontend client/Settings/EnvironmentConnection/environment tests
  66 pass in `codex-frontend-review.log`; final rerun after metadata correction
  has 92 tests across five files in `codex-final-tests.log`, exit 0.
  Uncached metadata/error HTTP/workloadconfig/e2e checks pass in
  `codex-param-backend.log`. Client mutation tests exercise PUT/POST/DELETE.
- Documentation checker and whitespace pass after canonical/current-state/
  code-map/traceability updates. Changed e2e source syntax is checked with Node.

## Browser evidence and visual review

All evidence is local fake adapters/disposable state; not proof of live
providers. Product actions use headed UI with human cursor/click and captions
below browser. BUSY/unknown code/copy failure and source-store read projection
use labelled page-route mocks. Auth, duplicate app/type, invalid ID/variable,
stale from two browser tabs and kubeconfig rejection use the local backend.
Canary strings are generated synthetic test data, not real credentials.

- `/tmp/poc-T20-evidence-1`: fail at ambiguous kubeconfig locator after earlier
  assertions passed; video/marks/assertions/logs retained.
- `/tmp/poc-T20-evidence-2`: scenario PASS. Captioned video 1440×996, 192.6 s,
  2889 frames; Codex full decode pass and review montage at
  `/tmp/poc-T20-handoff-20261011/codex-video2-review.jpg`. Captions/cursor visible,
  but several alerts are above viewport. This prompted a scenario correction;
  this run is not final visual proof of readable error alerts.

- `/tmp/poc-T20-evidence-3`: PASS before final presentation corrections.
- Final `/tmp/poc-T20-evidence-4`, run ID `refactor-20261011170433-26252`:
  PASS; raw/captioned MP4, captions.srt/ass, marks.json, assertions.txt,
  run.json/result.txt and logs retained. Captioned video 1440×996, 205.6 s,
  3084 frames. Codex full ffmpeg decode exit 0; review montage
  `/tmp/poc-T20-handoff-20261011/codex-video4-review.jpg` shows auth/duplicate/
  invalid-variable/stale/BUSY/unknown/copy-failure/previous-store/invalid-ID/
  duplicate-type/kubeconfig rejection. Actual alerts are visible during pauses;
  captions below UI, cursor visible, no real credential material.
- T01 regression `/tmp/poc-T20-evidence-T01-regress`: PASS before final caption/
  locator review; final `/tmp/poc-T20-evidence-T01-regress-2` PASS, run ID
  `refactor-20261011170831-20336`; Codex full video decode exit 0.

Scenario uses UI for sign-in/create/bind/register/config edits. A second browser
page writes first to produce an actual local stale conflict. Seven real local
error responses exclude synthetic canary content, and backend log canary check
passes. Copy setup patches the application read to show `kho-a` selected;
selection of `kho-b` and failing PUT are UI steps, then reloaded projection still
shows `kho-a`. This demonstrates UI error handling, not a real secret-store copy.
BUSY and unknown-code steps are also page-route mocks, explicitly labelled.
No automatic mutation replay is observed/asserted.

## Limits and ownership

SQL opt-in, cluster/cloud/provider verification and retained Vault/DB/K8S-4F
mutation were not run: external mutation is outside T20 scope. No fixture or
matching-policy changes. Historical safe failure summaries keep existing
projection semantics; T20 changes public HTTP envelope/presentation, not runtime
recovery or renderer selection. Existing nonfatal build chunk warning remains.
All failure/pass artifacts are retained outside Git; none uploaded.
Runner traps stopped owned processes; user-owned orchestrator processes were
left untouched. Task session `codex-T20-20261011` closed after final review; no
user-owned session closed. Working tree had no pre-existing user changes.


Final canonical review also checked UC-16 BR-14 actionable Resource Type
identification. Generic-only INVALID_SCORE guidance was rejected; typed ParamError now carries
the registered catalog Type only. Unknown Type gets fixed field `resources`;
malformed metadata is omitted, never echoing unknown keys or param values.
Final V-BE/build/conformance and V-FE 425-test pass include this correction.
Browser final4 predates this isolated metadata fix; it contains no resource-param
step. Unit/HTTP/frontend tests validate the changed behavior without an
unnecessary browser replay.
