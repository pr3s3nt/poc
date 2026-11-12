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
