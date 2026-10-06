---
id: VERIFY-20261006-UC04-CONNECTIONS-UI
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-06
---

# UC-04 desktop layout correction

User accepted correction of the Connections form's detached radio controls and
poorly placed actions. [UI design](../usecase/UC-04/ui/screens.md) was updated
before Claude coded in tmux; Codex reviewed the resulting desktop screenshots
and recording. The approved specification remains unchanged. User clarified
that desktop usability is the acceptance target, without further visual polish
or mobile optimization work.

Registration now uses a readable 720px column, adjacent Upload/Paste choices,
inline checkbox, styled native file chooser, nearby inspect action, grouped
context/destination summary and a footer action. The list uses Name, Type,
Destination and Status columns, secondary keys and READY badges. Kubernetes
fields are not shown for AWS entries. Existing functional/security states remain.

Codex found a clipped keyboard focus outline on the segmented choices during
review. Claude corrected it to an inset outline, then captured actual Tab/arrow
keyboard focus and reran frontend gates. Codex visually confirmed both desktop
focus screenshots. Responsive checks were already produced by Claude; no
additional mobile work was requested after the user's clarification.

## Reviewed recording

[Desktop Connections demo](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/uc04-connections-ui-20261006-075001-26956.mp4)

- Run: `uc04-kubeconfig-kind-video-20261006075001-26956`; exit 0.
- Connection: `kind-internal-126956`, READY; real read-only kind verification.
- MP4: 1440x900, 88.066667 seconds, eleven settled frame markers.
- SHA-256: `84bdfbf521b2a071b6b8dd7266d5a26dd81dbe6718bf82acbecf634a8ef38528`.
- Codex reviewed registration/list/error/context/success/cleared-input frames
  and independently decoded the full MP4. Credential values are absent from
  responses/logs/state; scoped Vault has one normalized immutable object,
  version 1, with overwrite denied (403).
- The recording predates the final focus-only CSS correction; final keyboard
  focus evidence is in the desktop screenshots, with no subsequent layout change.

Workload executors are fake, as in the original
[upload verification](2026-10-06-uc04-kubeconfig-upload.md). This recording proves
registration, not live deployment. Run-owned Vault/private files/state/processes
were cleaned; existing platform services were preserved. No cluster mutation,
AWS access, code commit or push was performed. The video was uploaded as a new
asset and downloaded for byte comparison; previous videos remain available.

## Validation and artifacts

Claude passed frontend typecheck/lint/tests/build (14 files, 111 tests), the
updated local UC-02/03/04 recording (19 markers), and the screenshot helper's
overflow/control-width/credential/focus assertions. Codex reran frontend gates,
documentation checker and diff check. Go tests were skipped for this correction
because backend code was unchanged.

Source changes: ConnectionsPage, its two existing test files, scoped CSS and
the two existing recording scripts' list selectors. The new
`frontend/test/e2e/uc04-connections-screenshots.mjs` uses synthetic route fixtures
for layout evidence; the published recording uses the real backend.
The narrow shell overflow rule is shared; desktop layout is unaffected.

Local evidence: `/tmp/uc04-coordination-20261006/ui-evidence/` (recording),
`ui-screens-4/` (final images), `ui-report.md` and `ui-review2-report.md`.
Evidence/private data is outside tracked source. All earlier dirty/untracked
delivery work, including the approved spec, remains preserved and uncommitted.
