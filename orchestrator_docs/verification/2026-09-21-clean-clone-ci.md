---
id: VERIFY-2026-09-21-CLEAN-CLONE-CI
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-21
---

# 2026-09-21 — Clean-clone documentation and product CI

## Reproduced issue

Three local empty directories were not tracked by Git:

- `backend/internal/application/admin/`;
- `backend/internal/application/preview/`;
- `backend/internal/adapters/aws/`.

After removing them to model a clean clone, the documentation checker reported
six missing links in the implementation map and UC-01/02/04/05 context files.
The repository also had no CI jobs for Go or frontend validation.

## Fix

- Replaced all six links with current executable entry points under `seed`,
  `store`, `planning`, `kubernetes`, `terraform` and `bootstrap`.
- Removed unimplemented placeholder packages from the package-layout tree and
  documented that packages are added only with executable code/tests.
- Strengthened the documentation checker: a directory link must contain at
  least one tracked or untracked non-ignored file, so an empty local directory
  cannot hide a clean-clone failure.
- Added `.github/workflows/ci.yml` with backend test/build/vet/race jobs and
  frontend install/typecheck/lint/test/build jobs.
- Removed the unrelated empty root file `typescript` via Trash.

## Result

- Documentation validation passed without any local empty package directory.
- `go test -count=1 ./...`, `go build ./...`, `go vet ./...` and critical race
  tests passed.
- Frontend typecheck, lint, 23 tests and production build passed; two live
  external tests remained intentionally skipped.
- Both GitHub workflow YAML files parsed successfully.
