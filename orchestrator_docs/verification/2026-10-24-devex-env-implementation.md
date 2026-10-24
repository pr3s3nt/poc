---
id: VERIFY-20261024-DEVEX-ENV-IMPLEMENTATION
artifact: verification-record
status: evidence
last_reviewed: 2026-10-24
---

# DEVEX-ENV implementation (issue #13)

## Scope and source

Branch `features/refactor_dev_ex`, base `995afd6` (design `7845786`, main `8f4d66f` with ADR-020 catalogs).
Implemented: create-only `POST .../configuration/keys`; ordinary-value lifecycle with
private `literalProofs` stored in the Deployment Set `document` jsonb (no DDL change, no
reset); one **Biến môi trường** table per container with five sources; inline Tạo
biến/bí mật (with initial Secret Store selection) and Thêm tài nguyên dialogs; vi/en
catalogs. Public Score, Delta and `document_hash` are unchanged.

## What is real and what is fake

Real: Go services and HTTP handlers, Web Console bundle, a disposable PostgreSQL 17
container (`postgres:17-alpine`, loopback port, removed after the run) for the
persistence/lifecycle/restart tests, installed `score-k8s` renderer in the browser
fixture. FAKE: workload and resource executors (nothing provisioned or deployed), the
Secret Store (synthetic READY record `vault-qa`, in-memory provider). No cluster, cloud,
credential or retained data was touched. Browser data is fixture data.

## Checks run

| Check | Result |
|---|---|
| `cd backend && go build ./... && go vet ./...` | PASS |
| `cd backend && REQUIRE_POSTGRES=1 go test ./... -count=1` (real PostgreSQL, zero skip) | PASS except `examples/acceptance-app/backend`, which needs `redis-server` (not installed here; unrelated to this change, no code of it was touched). Without `REQUIRE_POSTGRES` that package skips as before. |
| `cd frontend && npm ci && npm run typecheck && npm run build` | PASS |
| `cd frontend && npm test` | PASS (874 tests). Under full-suite load 5 unrelated `features/platform/*` files can hit the 5 s default timeout; the same 22 tests pass alone with `--testTimeout=60000`. |
| `cd frontend && npm run lint` | Baseline failure outside scope: 46 errors in `frontend/design/**` and `frontend/test/e2e/*.mjs` (no-undef for Node/browser globals), identical on the untouched tree; `npx eslint src` is clean. |
| `python3 scripts/check_docs.py`, `git diff --check` | PASS (390 Markdown files, 338 artifact IDs; no PlantUML changed) |

## Acceptance map (tests by name)

- **E01** `keeps one table per container…`, `saves each source as its Score projection…`, `keeps the name and drops the old source payload…`, `removes only the mapping of a row…` (`EnvironmentVariablesTable.test.tsx`).
- **E02** `TestAcceptedLiteralsRoundTripAndDirectDeployStaysHidden` (empty, spaces, escaped `$${}` through Save→Preview→Deploy→List→second edit), `TestAcceptedLiteralsSurviveRestartOnPostgres` (close and rebuild on the same database), `TestSnapshotKeepsDeploymentSetLiteralProofs`, form escape tests, `bindings.test.ts` (escape/unescape inverse, expander parity), `TestWorkloadDraftRejectsInvalidBindingsAndStaleVersion` (NUL, mixed, malformed, unknown reference rejected), mixed-reference import stays on the advanced Score path.
- **E03** `never selects a key on its own…`, `allows one name in two containers but blocks Save for a duplicate…`, `restores sources, aliases, repeated keys and containers on Edit…`.
- **E04** `TestCreateKeyNeverOverwrites`, `TestCreateKeyRejectsStaleBusyInvalidAndMissingStore`, `TestConcurrentCreateKeyHasOneWinner` (8 goroutines, one winner, no orphan provider value), `TestCreateConfigurationKeyHandler` (strict body, 201, DUPLICATE/STALE/422, anonymous 401, other org 404), dialog tests: success, masked secret, duplicate with explicit `Dùng key hiện có`, unknown outcome needs review before another create, stale/busy keep input without retry, keys survive Cancel.
- **E05** store selected and saved in the dialog before the secret is created; no READY store message while other rows stay editable; browser run saves `vault-qa` in the dialog.
- **E06** add-dependency dialog edits the draft only (no HTTP call), selects the alias, leaves Output to choose, reused in a second container; Cancel adds nothing; reserved/taken alias rejected.
- **E07/E08** `keeps a missing key, alias, output and Service port on its row…`, `offers only other workloads…` (self, pending delete, portless excluded).
- **E09** catalog loading/error is not empty; late catalog, late create reply, late parse and late Save of another scope are dropped; dialog closes on scope change.
- **E10** `TestLiteralProofIsPerWorkload`, `TestProveLiteralsCarriesOnlyIdenticalValues`, `TestRenameKeepsProvenLiteralsAndRejectsUnprovenOnes` (unproven literal fails closed with REFERENCE_REWRITE_UNSAFE, state unchanged), `TestLiteralProofsRoundTripRestartAndLegacyRows` (a set without the member reads as no proof). A direct Score deploy that changes a value invalidates the proof; a workload deployed directly never gains one.
- **E11** existing pending/preview/no-op/rename/route/class/redaction suites pass unchanged; the old literal-rejection assertion was superseded, negative cases kept.
- **E12** `renders the table, source choices and dialog names from the English catalog`, catalog parity test, `traps focus, closes on Escape and returns focus…`, and the browser run below.

## Browser acceptance (fake execution)

`frontend/test/e2e/devex-env-local.mjs` against `backend/test/fixtures/deployment-review`
(`-ready-secret-store vault-qa`): Vietnamese 1512×980 and 1280×900 (also sidebar
collapsed), English at both sizes. 28 assertions PASS (list in
[assertions.txt](artifacts/devex-env-20261024/assertions.txt)): inline dependency, variable
and secret creation (201, masked input, no secret value in page, Score, Preview or reopened
editor), nine rows saved and deployed, reopened with exact values, duplicate create left the
key unchanged, keys stay after Cancel, no horizontal overflow, Escape returns focus.

Reference UI: AppShell, WorkloadEditorPage, SettingsPage, DeleteResourceDialog and shared
styles, compared with the screenshots below in the same browser. Concision check: the table has one note for literals and one boundary
sentence per dialog; per-row paragraphs were not added.

- [empty table](artifacts/devex-env-20261024/01-vi-empty-table-1512x980.png)
- [add resource dialog](artifacts/devex-env-20261024/02-vi-add-resource-dialog.png)
- [create variable dialog](artifacts/devex-env-20261024/03-vi-create-variable-dialog.png)
- [create secret dialog after store saved](artifacts/devex-env-20261024/04-vi-create-secret-dialog-store-saved.png)
- [table 1512×980](artifacts/devex-env-20261024/05-vi-full-table-1512x980.png), [1280×900](artifacts/devex-env-20261024/06-vi-full-table-1280x900.png), [sidebar collapsed](artifacts/devex-env-20261024/06b-vi-full-table-1280x900-sidebar-collapsed.png)
- [reopened after deploy](artifacts/devex-env-20261024/07-vi-reopened-after-deploy.png), [duplicate key](artifacts/devex-env-20261024/08-vi-duplicate-key-no-overwrite.png)
- English: [dialog 1512](artifacts/devex-env-20261024/09-en-create-variable-dialog-1512x980.png), [dialog 1280](artifacts/devex-env-20261024/09-en-create-variable-dialog-1280x900.png), [table 1512](artifacts/devex-env-20261024/10-en-table-1512x980.png), [table 1280](artifacts/devex-env-20261024/10-en-table-1280x900.png)

Full-page screenshots show the fixed sidebar and footer where the page scrolls; that is a
capture artifact, not a layout fault.

## Limitations

- Product restart was verified on the real PostgreSQL store at service level and on the
  snapshot-file store; the browser fixture is in-memory, so the browser run has no restart.
- Execution is fake; no live cluster, AWS or Vault acceptance was required or run.
- A literal accepted by Save is stored with the configuration and is readable; it is not a
  place for secrets (the form says so). Existing literals without a proof stay unreadable and
  such a workload opens only through the advanced Score path.
- A dependency's output is never chosen for the user; Preview/Deploy still validate matching.
