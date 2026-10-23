---
id: VERIFY-SIDEBAR-PREVIEW-SPACE-20261023
artifact: verification-report
status: current
last_reviewed: 2026-10-23
---

# Collapsible desktop sidebar and resource colors

User requested a collapsible left menu to give Preview more space and colored
illustrations instead of the mostly monochrome resource cards. The
[shared shell design](../architecture/ui/README.md) and
[UC-05 screen](../usecase/UC-05/ui/screens.md) were updated before implementation.
No new use case, backend operation, API, planner semantics or mobile redesign.

## Result

- The signed-in sidebar collapses from 260px to 76px using an explicit
  keyboard-accessible button. Local storage remembers this display preference;
  a storage error falls back to usable in-memory state.
- The rail preserves named/titled navigation, role gates, active route,
  identity and sign-out. Labels stay available to assistive technology while
  visually clipped. Existing resource-type form labels remain unambiguous.
- Sidebar stays in the desktop viewport while long Preview content scrolls.
  Application/Score Preview/deployment detail pages use available width up to
  1680px. Existing graph ResizeObserver recalculates scale without remounting
  the page or discarding input, selected node or Code file.
- Native SVG menu icons have coordinated colored backgrounds. Graph/inspector
  share resource-type colors: cluster blue, namespace violet, PostgreSQL blue,
  Redis rose, workload teal, VPC amber. Cluster/namespace have distinct glyphs.
  Light card tints/accent edges retain readable text, selected outline and the
  original execution-state text/badges/failure treatment.

## Real browser observations

Chromium used the existing kind sample on port 18094 and its actual current
backend Score in standalone **update** Preview. Before and after Scores match,
so this is a read-only planning/visual test; no drafts or infrastructure changed.

| Desktop viewport | Expanded graph width | Collapsed graph width | Gain |
|---|---:|---:|---:|
| 1512px | 774px | 958px | 184px |
| 1280px | 572px | 756px | 184px |

Both sizes verified the real seven-node graph, at least five distinct computed
icon colors, tinted cards, preserved Redis selection/Code tab/`inputs` file and
unchanged Score editor contents after collapse. Named resource-type navigation
works in rail mode; reloading that page restores the collapsed preference;
expanding restores labels and sign-out. No browser page exception was observed.
Scoped deployment history was identical before/after. Unit tests additionally
cover keyboard toggle, same mounted input identity/value, Developer role gates,
persisted initial preference and blocked storage.

## Validation and preserved work

TypeScript/build and affected ESLint passed. The full frontend suite passed
**835 tests / 65 files** (160.95s). After replacing the remaining decorative menu
glyphs with SVG, the final shell/App regression run passed 37 tests and the
production image/build, affected lint and desktop browser observations were
repeated. Documentation and whitespace checks pass. Full-directory lint retains
the preexisting findings in old prototype/E2E files, including the user-owned
untracked `frontend/test/e2e/deploy-acceptance-video.mjs`; it was not edited/staged.
Go/cluster/cloud checks were not rerun: execution code is unchanged and the only
runtime requests here are authentication, GETs and standalone Preview.

Initial focused tests exposed duplicate resource-type labels when links used
aria-label; links now derive their accessible name from visually clipped text,
and the existing form tests pass. An initial browser observer wrongly used
Deploy Preview for an existing workload; the product correctly rejected it
before mutation. Its retry assumed an exact select label; the observer then
selected the actual update control and supplied before/after Scores. All failed
observer/test logs remain outside Git at `/tmp/preview-sidebar-20261023/`.

The frontend image `orchestrator-frontend:preview-kind` and local bundle were
rebuilt. Fixed assets were copied into the running frontend without restarting
backend/nginx or changing data; the old port 18093 demo shares the rebuilt local
bundle. Existing user sessions/volumes/workloads remain untouched.

## Reviewed captures

- [1512px expanded](artifacts/sidebar-preview-20261023/expanded-1512.png) /
  [collapsed](artifacts/sidebar-preview-20261023/collapsed-1512.png).
- [1280px expanded](artifacts/sidebar-preview-20261023/expanded-1280.png) /
  [collapsed](artifacts/sidebar-preview-20261023/collapsed-1280.png).
- [Browser geometry and state observations](artifacts/sidebar-preview-20261023/browser.json).

Images were visually reviewed against the existing Console typography/panels and
the requested extra Preview space. No credential or Terraform state is included.
