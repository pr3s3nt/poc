---
id: VERIFY-20261021-DEFINITION-V3-VISUAL
artifact: verification
status: evidence
last_reviewed: 2026-10-21
related: [UC-03]
---

# Issue #8 — v3 visual correction

Base `1b4a486` on `iam_role`. User review identified that functional acceptance
had been reported before the presentation matched the fixed v3 reference in
`32db49f5cb1df6a855e55241e26962e1cb0adec2`. IMP-023 records that gap and its
resolution. No prototype, backend, API, source pin or secret binding was changed.

## Corrected presentation

Catalog and focused Definition editor are separate views. Editor has breadcrumb,
display-name/driver/version header, two tabs, numbered Module and Account/parameters
cards, actual metadata overview at desktop widths, aligned Type-to-module mapping
rows and a fixed Save bar. Technical explanations/provenance use disclosures;
required/type/secret classification, actual defaults and all controls remain.
Create/Edit/Back/resume are presentation changes over mounted draft/CAS/metadata
state. Type CTA enters creation directly. Explicit cancel/new behavior and
unknown-save/stale-version holds are retained. Resume preserves the exact draft
and expected version. No Save/inspect/matching happens automatically.

## Validation

- Affected component/editor selection: **215 distinct tests PASS** across12affected files and focused reruns, including
  Definition form/CAS/unknown outcome, typed conversions/mapping, Git metadata,
  criteria/matching invalidation, ExistingClusterSection and Rendering Templates.
- App T04 routing selection: **6 PASS** for CTA and deep links; the other 27 App
  cases were not selected, not counted. No test was marked skip or assertion
  weakened. Creation tests enter through the real catalog CTA. Fixture ID/name/region and long typed placeholder entry use paste to avoid repeated rendering within the unchanged
  five-second deadlines; explicit keystroke/late-response tests remain intact.
  Earlier runs hit authoring timeouts; fixture entry is batched as paste user
  actions and focused reruns use one worker without concurrent TypeScript builds. All assertions and timeouts retained.
- Frontend typecheck/production build and changed-file ESLint with zero warnings
  PASS. Existing bundle size advisory remains. Full-directory lint has the known
  unrelated prototype/IAM-script/user-owned demo failures from the prior record;
  no unrelated lint cleanup was attempted.
- Docs checker, Node syntax checks for the two browser scripts and diff whitespace
  check PASS. No PlantUML changed; operations/participants/contracts unchanged.

## Browser and visual review

`cd backend && go run ./test/integration/gitmodulebrowser -scenario
../frontend/test/e2e/resource-definition-v3-local.mjs` PASS with the real product
HTTP/auth/catalog/registry, a temporary HTTPS Git fixture and in-memory catalog.
No fake inspect/matching/write responses, Terraform/provider/database/cloud calls.

Existing business assertions remain: unequal mapping names, params/both flags,
Form/JSON including invalid text, AND/OR/copy/matching, create/reload, edit/CAS409/
explicit reload, keyboard/mobile and shared renderer smoke. Additional layout
assertions check catalog hidden during editing, overview to the right of the
module card, fixed bar reaching viewport bottom, and draft/expected-version
preservation through catalog/resume followed by the real concurrent CAS conflict.
Screenshots use `/tmp/issue8-v3-visual-browser/`.

Read-only live review also opens `pg-kind` through the Tailscale Vite link and
captures configuration/advanced/matching at 1440×1100 plus mobile 390px. It does
not Save, inspect modules or invoke matching against that retained environment.
The preserved v3 HTML was opened in Chromium and captured at the same viewport.
Before/after/reference images are under `/tmp/issue8-visual-review/`, with final
views named `final-configuration.png`, `final-advanced.png`, `final-matching.png`
and `final-mobile.png`. No live catalog or deployed resource was mutated.

## Review and limits

Open `/ui/platform/resource-definitions`, then Sửa on an existing Definition or
Tạo cấu hình. The list/cluster area is hidden while editing. Breadcrumb returns
to catalog; Tiếp tục bản nháp resumes an edit. Two tabs, advanced mappings and
fixed Save actions are available there. The existing Tailscale/Vite service stays
running; no user service, tmux or cluster session was stopped.

No fullsuite, database persistence or deploy/cloud acceptance outside issue #8.
User-owned untracked `frontend/test/e2e/deploy-kind-demo.mjs` remains untouched.
