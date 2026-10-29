---
id: VERIFY-RENDER-BUNDLE-SETUP-20261029
artifact: verification-record
status: current
last_reviewed: 2026-10-29
---

# Configurable Git render bundle setup

User requested a portable Playwright registration script and explicitly asked
to execute it on the retained Tailscale preview before manual testing.
Runner: [register-render-bundles](../../e2e/setup/register-render-bundles/README.md).
Requirements: [UC-03 RB](../usecase/UC-03/specification.md#git-rendering-bundle-authoring--adr-021).

## Runtime and observations

- Console: `http://100.77.224.24:18596`, isolated Compose project
  `main-a0fe3a4-preview`; frontend/backend image tags `main-a0fe3a4`.
- Product source/Git bundle pin: `a0fe3a4a1e3c7ed22ad3dc18fa19744de040ce01`.
  Source directory `backend/render-bundles/standard` in the public project repo.
- Before setup: scoped workload-templates GET returned `200 {"templates":[]}`.
- Playwright signed in as Platform Engineer with the shared Console helper,
  inspected the Git bundle and submitted `standard-workload` through the real
  Rendering templates form, using the example JSON configuration.
- Initial post-create verification attempted an unsupported Definition detail
  GET and failed with 404 **after registration succeeded**. Runner was corrected
  to read the authoritative Definitions list; no duplicate create or rollback.
- Rerun passed: existing Definition's source pin, name, placement, render inputs,
  provision and criteria match; no repeated Definition POST. Browser opened the
  existing Application's Add Workload picker and verified all five templates:
  frontend/backend/worker/stateful/scheduled, version 1.0.0.
- Git bundle remains AVAILABLE. The Definition and cache are intentionally
  retained for the user's manual testing. Session signed out/browser closed.

## Checks and limits

`cd e2e && npm test`: 10/10 pass, zero skipped; includes configuration rejection
and idempotent rerun/conflict checks. Runner syntax/help, documentation checker
and diff whitespace checks passed. Build/login evidence from the preceding
preview setup is reused: this change adds only E2E setup scripts/documentation,
not backend/frontend product code. No product rebuild required.

Credentials remain outside Git under the preview's private temporary directory;
none are recorded here. Existing Applications, databases and other stacks were
not changed. No workload draft, Preview, Deploy, Kubernetes/cloud mutation or
data cleanup was run. Registration/picker success does not prove infrastructure
Definitions, Secret Stores, MySQL/Passbolt or application deployment on another
machine. The generic runner supports multiple bundle entries and custom criteria;
this real run used the standard example entry only.
