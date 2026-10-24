---
id: VERIFY-20261024-DEVEX-ENV-DESIGN
artifact: verification-record
status: evidence
last_reviewed: 2026-10-24
---

# DEVEX-ENV documentation and existing UI reference

## Scope and source

Documentation only: approved unified environment-variable table, inline source
creation, revised ordinary-value policy and coding-agent handoff. Product feature
implementation and its E01–E12 acceptance remain pending.

The branch started at `384dee6`. At the user's explicit request, in-progress docs
were stashed, main fast-forwarded to `8f4d66f`, then features/refactor_dev_ex
fast-forwarded to main and the docs reapplied without conflicts. ADR-020/i18n is
included in the updated design; no previous edits were discarded.

The existing Docker project `orchestrator-dev-ex` at localhost:3002 was used for
reference. Only its frontend was rebuilt from `8f4d66f` after the update (Docker
build included frontend typecheck/build and container healthcheck). Backend code
was unchanged by that upstream commit. No app deploy, cluster/cloud write,
database reset or retained-stack restart was performed.

## Browser observation

Opened the actual product WorkloadEditorPage in Playwright/Chromium, signed in
as the seeded developer, at 1512×980 Vietnamese and 1280×900 English. App,
workload and configuration GETs for `docs-ui-reference` were intercepted with
explicit fixture data; no persistent app/config/workload mutation was submitted.
This was necessary because the fresh stack's seeded acceptance app has a legacy
Environment unsupported by this editor. Fixture key LOG_LEVEL is non-secret.
The source page uses the actual built product components/styles, not a mock UI.

Observed separate Application Variable/Secret checklists and Other sources,
resource editor lower on the page, existing container/advanced sections, sticky
Save/Cancel and sidebar locale selector. These are reference for visual reuse;
they do not prove the new unified table or any deployment works.

- [Vietnamese existing editor, 1512px](artifacts/devex-env-20261024/editor-before-vi.png)
- [English existing editor, 1280px](artifacts/devex-env-20261024/editor-before-en.png)

Reviewed AppShell, WorkloadEditorPage, SettingsPage, Button, shared tokens/styles
and ADR-020 catalogs. The design keeps these components and visual conventions;
long repeated explanations are replaced by source-specific controls and one
commit-boundary note in each inline creation dialog. No new prototype or product
code was added.

## Validation

PASS: `REQUIRE_PLANTUML=1 python3 scripts/check_docs.py` checked 389 Markdown
files and 337 artifact IDs, including PlantUML syntax and local links.
PASS: `git diff --check`. Three changed PlantUML diagrams were regenerated as PNG. New application/backend/frontend feature tests
are not run in this documentation task because no feature code changed. Future
coding acceptance is explicitly listed in the implementation handoff.
