---
id: VERIFY-WORKLOAD-DEFAULT-SELECTS-20261106
artifact: verification-record
status: current
last_reviewed: 2026-11-06
---

# Distinct select choices for inherited workload values

User reported Service showing Disabled / Enabled / Disabled on the retained
Console draft. Starting commit `76a693b6`, clean branch `features/bo_sung_default`.
Real browser inspection reproduced three visible options: the inherited hint was
incorrectly offered as a selectable item beside the two explicit choices.

## Change and UI review

[UC-16 presentation](../usecase/UC-16/ui/screens.md#wd--runtime-groups-and-inherited-values)
keeps effective defaults muted without source labels. `DefaultSelect` now shares
a disabled hidden placeholder for the closed control, with only distinct actual
values in the opened menu. An explicit choice exposes Restore defaults, which
removes the override. A reopened sparse CronJob normalizes an absent concurrency
value to the empty inherited state rather than showing an override/reset. Applied to Service, Pod/container booleans and seccomp,
fsGroup policy, strategy, PDB enablement/eviction policy and CronJob concurrency.
No default/profile/API/renderer behavior changed.

UI references: existing real workload editor, shared UI design, AppShell, select
styles and Button. Browser visual comparison retains shell, spacing, scoped advanced
and muted hints. Duplicate/source-explanation options are removed; Reset appears
only after an explicit choice. Backend and Kubernetes were not redeployed or mutated.

## Validation

- Workload suite: 136/136 tests pass, 12 files. New regression verifies that Service,
  token, seccomp and rollout offer distinct choices, explicit false/default-matching
  choices survive Save, and Reset restores omitted fields on the next Save. Final
  focused defaults checks pass 9/9, including the additional sparse CronJob case.
- Typecheck, lint and production build pass. Existing PlatformSettingsPage ref/effect
  lint warning and bundle-size build warning remain.
- Full suite rerun with two workers: 970/973 tests pass. The same two baseline
  E2E source-assertion failures and one App deep-link timing failure remain. The
  initial default-worker run had 949/973 passing with 24 failures, including many
  unrelated 30-second timeouts under memory/swap pressure. This is not a green
  full suite. All three App deep-link cases pass when run focused. The final
  sparse-Cron normalization and its new case were validated
  by the final focused run; no second full-suite checkpoint was required.
- Documentation and diff checks pass. No Go/schema/shell changes; their runtime
  suites are outside this UI-only fix. No additional deploy/PVC run was needed.

Logs: `/tmp/issue37-selects-workloads.log`, `...-typecheck.log`, `...-lint.log`,
`...-build.log`, `...-full-ui.log`, `...-full-ui-bounded.log`,
`...-final-focused.log`, `...-final-build.log`, `...-app-focused.log`.

## Real Console and retained data

[Verified Console editor](http://desktop-bdjp2rq-1.tail342836.ts.net:18339/ui/applications/5c7bc93a-edd3-4ee2-a618-f3a4a60d9853/environments/staging/workloads/web)
was checked through the exact origin published by Tailscale Serve at
`2026-11-06T09:23:24Z`, after publishing the rebuilt frontend image.

Authenticated browser/API checks, without mocking: Service offers only Enabled /
Disabled (Bật / Tắt in Vietnamese), token/container security only True / False,
seccomp RuntimeDefault / Localhost, strategy RollingUpdate / Recreate, PDB only
Enabled / Disabled, fsGroup policy distinct choices, and three Cron concurrency
choices. Hidden inherited placeholders remain selected and muted when omitted.
Keyboard selection/Reset works. Temporary local overrides were reset, product
no-op Save succeeded and the saved desired document matched the original exactly.
CronJob was inspected in an unsaved new form; no extra draft was created.
Browser errors were empty. Existing draft, template pin/profile and backend remain.

Private evidence: `/home/thanhnt1/.local/state/issue37/selects-browser.json`,
`selects-before.json`, `selects-before.png`, `selects-after-en.png`,
`selects-after-vi.png`; no session/credentials are committed. Access was checked
from this host through Tailscale, not from every tailnet device/ACL.

Compose `issue37-defaults`, isolated data volumes and Serve 18339 remain active.
Restart instructions and data scope remain in the
[template-only handoff](2026-11-06-template-only-defaults.md#console-handoff).
