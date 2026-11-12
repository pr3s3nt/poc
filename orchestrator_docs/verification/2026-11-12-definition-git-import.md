---
id: VER-2026-11-12-DEFINITION-GIT-IMPORT
artifact: verification-record
status: current
last_reviewed: 2026-11-12
---

# Portable Git Definition files — issue #79

## Scope and revisions

Product code: `7b45f4a6` on `features/definition-git-import`, after design
`14c98e0d`. Release repository sample/docs: `5c55e1b` on the same feature branch
in `pr3s3nt/orchestrator-workload-templates`. No merge, release tag, retained
Console rollout or application deployment was performed. Later changes only
correct fixture normalization and record this verification.

[UC-03 GFILE-01..05](../usecase/UC-03/specification.md#nguồn-git-trong-file-definition--issue-79)
owns behavior. Terraform files author `driverInputs.values.source: {url, ref,
path}`; explicit preparation inspects/validates and returns a canonical immutable
pin. Registration consumes that pin. Git exports use saved origin/exact commit/
path. Other drivers and existing pin/form registration remain available.

## Focused checks

- Catalog tests pass: preparation does not write Definitions or mutate caller
  maps; scoped Type is checked before Git; ambiguous/mixed source fields and
  invalid mappings reject. A simulated branch change after preparation cannot
  alter the registered pin. Portable export/re-import returns REUSED; conflict
  leaves history unchanged. Exports preserve a clicked older version after a
  concurrent save and enforce Organization/revision scope.
- HTTP tests pass: prepare/import/export role and Organization gates, strict
  DTO/size/trailing documents, canonical creation, public-only export, selected
  version/revision, invalid/ambiguous version query and foreign revision errors.
- Frontend focused tests pass: parser/authoring projection, check/register UI,
  uploaded/edited drafts, late checks and scope changes, download of the server
  projection for the exact list version/history revision and safe failures.
  The existing ResourceDefinitionsPage and DefinitionVersions tests also pass.
- Typecheck, lint and production build pass. Lint has only the preexisting
  PlatformSettingsPage ref-cleanup warning; build retains its existing chunk-size
  warning. Node syntax, documentation checker and diff check pass. PlantUML
  sequence syntax passes using the existing PlantUML Docker image; the local
  documentation checker reports that a host PlantUML binary is absent.

Commands used for the new backend boundaries:

```sh
cd backend
go test ./internal/application/catalog ./internal/delivery/http \
  -run 'TestGitDocument|TestDefinitionDocument|TestDocumentImportStrictAndScoped|TestImportDefinitionDoesNotCreateRevisionOnReuseOrConflict' -count=1
```

The earlier broader catalog package run still reported the known workload-v03
capability/registration failures, while HTTP passed. This is not a claim that
all backend packages are green. No unrelated tests were changed to hide them.

## Real browser and Git

The isolated Docker feature stack runs the final product code, PostgreSQL and
Vault. Browser opened the exact Tailscale feature hostname and used real APIs
and Git inspection, without request mocks. The URL and private evidence remain
in the local handoff, not in this public record.

`e2e/scenarios/definition-git-import/run.mjs` passed:

1. Registered the approved MongoDB Type through the file tab on a fresh stack.
2. Uploaded a Definition with Git URL/ref/path only; invalid output mapping
   stayed editable and blocked registration.
3. Checked real `terraform/mongodb` from the release repository. Full commit
   review was visible and checking did not create a Definition.
4. Editing invalidated the reviewed result. Rechecking and registering created
   the canonical pinned Definition.
5. Downloaded the displayed current version and exact history revision; both
   contained portable source with the reviewed exact commit, no internal IDs/hash.
6. Uploaded exported YAML, checked and received REUSED. A changed name returned
   CONFLICT, with revision history byte-equivalent throughout both operations.

The final rerun initially stopped because the fixture compared omitted false
flags against explicit false. The runner now normalizes those JSON defaults
before comparing the same complete input/output schema; it still refuses any
actual contract mismatch. The rerun passed on retained task-owned catalog data.
The earlier catalog-document Type/endpoint import/export scenario also passed
after adding explicit Definition checking.

No MongoDB server was provisioned: an existing real image digest was validated
as module input without pulling it. This verifies catalog authoring/registration,
not MongoDB availability, application connectivity or Terraform deployment.

## UI review and cleanup

Reference: live existing Definition creation screen plus main's import panel,
AppShell/styles/Button and SavedSourceSummary. The older live reference stack
did not yet contain the file tab. Final screenshot review preserves the shell,
adjacent tabs, controls, colors and source summary. Only file input, short
status/source/commit and Check/Register actions are visible; format help stays
collapsed. No default explanation banner or internal pin fields were added.

The user subsequently restricted validation to tests related to this change.
The in-progress full Go/frontend checks were stopped. Their partial logs are
not full-suite evidence. The dedicated kind test namespace was removed after
checking its ownership label and recorded UID, using UID/resourceVersion delete
preconditions, and namespace absence was verified. No full-suite gate remains
for this task under that instruction.

The isolated feature stack and its PostgreSQL/Vault/source-cache volumes remain
running for review. No user-owned checkout changes, other Docker projects or
retained Console resources were changed. Local handoff records the restart
command, private password-file path and verified Tailscale URL. Both feature
branches remain unmerged pending user acceptance.


## Accepted main integration and retained Console rollout

The user authorized main merge. Product merge `9bf70931` and release repository
merge `3e4d54f` were pushed; product source is byte-equivalent to the checked
feature head. Release documentation subsequently records acceptance without
changing the Type/module/sample behavior.

Only affected checks were run again: five selected Go catalog/HTTP tests and
24 frontend parser/import/download/revision tests passed. Docs and diff checks
passed. No full suite was restarted, in accordance with the user's instruction.

The retained Console had no active/recovering work at preflight. Its four visible
Applications were recorded. The upgrade quiesced backend/frontend/Vault writers,
created a private SQL dump and successfully restored it into an isolated
verification database (11 Application rows), then backed up the retained PVC
content including Vault, credential references, state key and source caches.
Owner setup ran before runtime and reported the baseline/grants already current;
no schema migration or reset was needed. Runtime has no owner env/mount.

Tested product images were retagged `main-9bf70931`, imported into the existing
kind node and deployed; backend/frontend/Vault/PostgreSQL are Ready. The
run-owned backup Pod was deleted with recorded UID/resourceVersion guards and
absence verified. Backups/evidence remain private outside Git.

The former single port-forward exited during the frontend replacement, producing
502 on the Tailscale path while in-cluster API returned the expected unauthenticated
401. A task-owned reconnecting loopback forward restored the upstream used by
the existing MagicDNS Serve route, without changing the other Serve ports or
closing user tmux sessions. The exact URL was then opened in a real browser.

Main browser smoke exported an existing Git Terraform Definition at its displayed
version, checked the portable document, reviewed the exact commit and re-imported
it as REUSED. Its revision history was unchanged and all four visible Application
identities match preflight. Git credentials remained configured and the existing
`dev-fleet` binding remained READY and selected as the platform default; no
credential or CD configuration was overwritten. The CD screen was also opened
and checked. The first CD-screen assertion used an exact text selector although
the default badge shares the cell; correcting that private selector made the
smoke pass without product changes.

Feature branches were deleted in both repos. Only the task's Docker feature
stack was stopped, with its volumes/data retained; the feature URL is retired.
The retained main forward remains running. Public comments omit the private
Tailscale URL, password paths and infrastructure evidence. No preexisting
user-owned checkout changes were present or modified. Issue #79 is complete.
