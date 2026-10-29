---
id: VERIFY-INLINE-VOLUME-MOUNT-20261029
artifact: verification-record
status: current
last_reviewed: 2026-10-29
---

# Direct volume-mount entry on Backend

The user still could not click Add volume mount after platform setup and
confirmed the button was disabled. On the retained Tailscale Console
`http://100.77.224.24:18596`, the storage Type and Definition were present,
but a fresh Backend local form contained no storage dependency. The previous
button required users to add that dependency in a separate section below.

## Change and UI reference

[WC-04/05](../usecase/UC-16/workload-catalog-specification.md) and the
[UI design](../usecase/UC-16/ui/workload-catalog.md) now describe direct storage
entry. Add volume mount opens the existing AddResourceDialog when the form has
no storage dependency. Only Types with public string claimName/namespace outputs
and compatible scoped shared resources are offered. Confirmation adds a local
dependency and mount; Cancel changes nothing. An unavailable storage catalog
has a short explanation. Existing storage dependencies keep the direct add-row
behavior. API shape and provisioning boundaries are unchanged.

Visual references read: AppShell, tokens/styles, Button, Dialog, DependencyFields,
AddResourceDialog and WorkloadEditorPage. The actual existing Backend page was
opened and captured before implementation. The resulting VI/EN dialog and form
were inspected through browser screenshots: shared shell, spacing and controls
are retained. Content review kept one unavailable-storage message and reused
the resource form; no extra instructional panel was added.

## Validation and runtime

- Frontend typecheck, lint, all 947 tests across 83 files and production build
  passed. Four new regression tests exercise cancel/focus, saved dependency and
  mount, scoped resource reuse, alias collision and invalid storage capability.
- E2E helper tests: 21/21 passed. Setup runner now starts the mount check at the
  actual Add volume mount button rather than preparing the dependency first.
- Docker frontend image `orchestrator-frontend:mount-entry-20261029-r1` built
  successfully. Only `main-a0fe3a4-preview-frontend-1` was recreated; backend,
  PostgreSQL, Vault, registered metadata and other stacks were preserved.
- A headed paced invocation of the setup runner reused the existing account,
  six Types, five infrastructure Definitions and standard rendering Definition;
  all five catalog templates and direct Backend mount entry assertions passed.
- Separate real browser checks on the Tailscale URL passed in VI and EN:
  fresh Backend button enabled → storage dialog → Escape preserves form and
  restores focus → confirm adds `/data`. No browser page errors occurred.
- Before/after workload API reads matched: browser checks did not save a draft.
  Private screenshots/logs remain outside Git under the preview `/tmp` directory.
- Documentation checker and diff whitespace checks passed.

No Go/backend logic changed; no Go suite or cluster/cloud deployment was needed.
No PVC/workload was provisioned, no user draft was overwritten and no existing
resource was deleted. The frontend is retained on the same Tailscale URL for
manual testing; a reload is required to load the updated application assets.
