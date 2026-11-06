---
id: VERIFY-WORKLOAD-RESOLUTION-ERRORS-20261106
artifact: verification-record
status: current
last_reviewed: 2026-11-06
---

# Effective configuration error diagnosis and runtime validation feedback

User reported a generic resolution error on the retained Git-source workload
editor. Starting clean commit `d832e2c9`, branch `features/bo_sung_default`.
Scope: diagnose and repair the error handling while preserving the user's draft.

## Findings and correction

The saved document returned HTTP 200 during diagnosis, including five consecutive
real browser reloads. It retained the expected pinned backend 1.1.0 profile and
staging CPU 100m. Git source metadata was not missing. The user's exact unsaved
field edits were unavailable, so the original failure trigger is not proven.

Independent pure API probes found a concrete error: enabling PDB with the template's
one replica returned HTTP 500 INTERNAL_ERROR. The merged domain validator correctly
rejected that combination, but HTTP error classification omitted ErrExecution.
Resolved compute/runtime validation now returns 400 INVALID_WORKLOAD, with fixed
safe response text. Validation guards remain; invalid PDB settings are not accepted.

Console now displays safe code-based validation feedback instead of treating every
rejection as unavailable defaults. When the failed input contains an enabled PDB
and fewer than two known effective replicas, its existing prerequisite help is
shown. It reads replica overrides or the pinned template/environment metadata,
never a guessed replica count. Network/server failures still retain the generic
retry feedback. Corrections automatically rerun the resolver and preserve edits.
No raw backend error sentence or submitted secret is displayed.

Sources: [UC-16 screens](../usecase/UC-16/ui/screens.md),
[API mapping](../usecase/UC-16/ui/api-mapping.md),
[template defaults contract](../architecture/contracts/workload-defaults.md).
Existing Console shell, controls/styles and scoped groups are retained. The
existing help supplies the actionable prerequisite without adding source labels.

## Verification

Affected HTTP/workload configuration Go packages and Go build pass. Error contract
regression checks wrapped ErrExecution returns 400 and excludes canary text from
responses/logs. UI regression exercises invalid PDB, preserved image/edits, corrected
two-replica retry and sparse Save. Typecheck/lint/build and the workload UI suite
pass: 138/138 workload tests (12 files), and final focused defaults checks 10/10.
Existing PlatformSettingsPage lint and chunk-size build warnings remain. Full
suites were not repeated for this focused error-path correction; prior full-suite
failures remain recorded in the preceding verification records.

Private evidence: `/home/thanhnt1/.local/state/issue37/resolution-diagnosis.json`,
`resolution-probes.json`, `resolution-diagnosis.png`. Source logs:
`/tmp/issue37-resolver-errors-*.log`. These API probes are pure, without Save,
Preview/Deploy or cluster writes. The original desired document is retained.

## Published live result

[Verified Console editor](http://desktop-bdjp2rq-1.tail342836.ts.net:18339/ui/applications/5c7bc93a-edd3-4ee2-a618-f3a4a60d9853/environments/staging/workloads/web)
was checked through the exact Tailscale Serve origin with real authenticated APIs,
without interception. At `2026-11-06T09:55:53Z`, initial load returned 200; enabling
PDB with inherited one replica returned 400 INVALID_WORKLOAD and showed the existing
prerequisite help in both Vietnamese and English; changing replicas to two returned
200 and cleared the error. An excessive memory request relative to the inherited
limit also returned 400. Final page reload was error-free with the 100m hint.
The desired document and draft version matched the original exactly. No Save,
Preview/Deploy or Kubernetes mutation was performed. Browser errors were empty.
Private evidence: `resolution-fixed-browser.json`, `resolution-fixed-invalid-vi.png`,
`resolution-fixed-invalid-en.png`, `resolution-fixed-handoff.png`.

The backend/frontend images were updated. Because renderer identity covers the
backend binary, the unchanged Git source was inspected again against the new
runtime: source ID `d47c1758-d027-4558-a032-32b2c4ab8e33`, kind Git, same commit
`b6ab5bd4ce18ce6fb881345a98daff682282a7d9` and source content digest as before.
The issue-owned workload Definition publishes version 2; staging and production
explicitly select revision
`747a0d89d535980d170c31d756966ebb0bf592a96a1d6d0378ef2c64eca093cc`,
selection revision 2. Read-back and the same live editor passed at
`2026-11-06T09:56:44Z`. No shared Definition or template values were changed.
Private registration/selection receipts are in `resolver-errors-git-*.log` and
`git-source-selected.json`. The first attempted Definition update included
read-only response fields and was rejected with 400 before any write; the narrowed
request then passed. The new proposed revision was explicitly selected rather
than assuming the previous active revision changed.

Compose `issue37-defaults`, isolated data/cache volumes, kind and Serve 18339 stay
running. Restart instructions remain in the
[retained stack handoff](2026-11-06-template-only-defaults.md#console-handoff).
Access was verified from this host, not every tailnet device/ACL. Documentation
checker and diff check pass; no schema migration is involved.
