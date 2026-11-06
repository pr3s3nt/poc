---
id: VERIFY-WORKLOAD-SIMPLE-DEFAULT-HINTS-20261106
artifact: verification-record
status: current
last_reviewed: 2026-11-06
---

# Compact workload form and muted default hints

Scope: user clarification on `features/bo_sung_default`, starting `8245a763` with a
clean tree. Only name/image/environment variables should be prominent. Less-used
fields move under Show advanced; inherited defaults show as muted hints that do
not require deleting before entering an override. Do not show per-field source
labels. Canonical presentation is in [UC-16 screens](../usecase/UC-16/ui/screens.md#wd--runtime-groups-and-inherited-values)
and [WD-09](../usecase/UC-16/workload-defaults-specification.md).

## Implemented behavior

Each container keeps its name/image/environment outside its own Show advanced.
Compute, volumes, ports, health, startup and security remain scoped groups inside.
Workload Show advanced contains Service/dependencies/scaling/storage/Pod security
and infrequent configuration. Applicable CronJob schedule stays outside. Keyboard
accessible shared Buttons disclose the advanced editors and preserve mounted raw
edits after first opening. Validation errors open the corresponding disclosures.

CPU/memory, image, numeric policies, rollout values, probe timing and timezone use
resolved values as placeholders in otherwise empty inputs. Optional selects use a
muted inherited default option. Typing replaces the hint immediately and stores
only the authored override; clearing or Reset returns to inheritance. Explicit false
and zero remain distinct from absence. No new policy values are guessed or copied
into desired config. FieldSource labels and source qualifiers in runtime summaries
were removed; backend provenance and Preview contracts remain unchanged. Platform-
owned identity help and resource-source selection remain because they affect rights
and binding decisions, rather than explaining where a default originated.

UI references: existing real Console form, AppShell, shared UI design, Buttons,
Advanced disclosures, styles and UC-16 group components. Shell/navigation/theme
remain. The visual review removes source-label repetition and hides secondary groups
from the initial screen; no introductory instruction blocks were added.

## Validation

- Existing 134 workload tests passed after updating disclosure interactions and
  replacing source-label assertions with effective-value assertions. An additional
  test proves the compact initial state, 100m placeholder with empty actual value,
  direct typing of 250m without a Clear step, persistence through hide/show and
  saving only the changed CPU override. Focused final defaults/lifecycle checks:
  20 tests passed.
- Typecheck, lint and production build pass. Existing PlatformSettingsPage effect/
  ref warning and chunk-size warning remain.
- Full frontend suite completed with 969/972 tests passing (100 files: 97 pass,
  3 fail). The two baseline E2E source-assertion failures remain. A Resource
  Definitions deep-link timing failure in App.test.tsx passed focused (all three
  selects-nothing deep-link cases). All 135 workload tests passed. These full-suite
  failures are recorded as failures, not converted to a green suite.
- Documentation checker, diff check and `node --check` for the adjusted existing
  workload-defaults scenario pass. The runner now opens workload advanced before
  editing PDB; no new runner or live Kubernetes scenario was added.
- No backend/API/SQL/renderer policy changes. Go/PostgreSQL/Kubernetes checks were
  not repeated for this UI-only follow-up.

## Real Console

[Verified draft editor](http://desktop-bdjp2rq-1.tail342836.ts.net:18339/ui/applications/b1b983d7-cea1-4e4d-9a9e-63638919db9c/environments/staging/workloads/web)
uses the actual isolated backend via Tailscale Serve 18339. Private evidence is
under `/home/thanhnt1/.local/state/issue37`: `simple-browser.json`,
`simple-basic-vi.png`, `simple-basic-en.png`, `simple-defaults-vi.png`,
`simple-cron-vi.png`; source logs are `/tmp/issue37-simple-*.log`.

At `2026-11-06T08:09:23Z`, browser checks passed for basic-only initial view, Enter-key disclosure, muted hints
100m/128Mi/512Mi/replica 1/history 3, boolean default, direct typing and clear,
absence of source labels, hide/show edit preservation and visible CronJob schedule
when workload advanced is closed. VI/EN are checked with real API responses and no
interception. After restoring temporary edits, product Save is compared against
the original desired document and matched exactly; browser errors were empty. No Preview/Deploy or cluster mutation is performed.
The same authoring Application remains; the dedicated frontend image was rebuilt
and restarted. Keep Compose `issue37-defaults` and Serve 18339 running; data/restart
instructions remain in [the original handoff](2026-11-06-workload-defaults.md).
