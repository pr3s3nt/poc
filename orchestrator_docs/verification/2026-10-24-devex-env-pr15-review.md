---
id: VERIFY-20261024-DEVEX-ENV-PR15-REVIEW
artifact: verification-record
status: evidence
last_reviewed: 2026-10-24
---

# PR #15 review corrections

Review of [PR #15](https://github.com/pr3s3nt/poc/pull/15), implementation head
`e5e1fc6`, against [DEVEX-ENV](../implementation/workload-environment-editor.md).
This supplements the [initial implementation evidence](2026-10-24-devex-env-implementation.md).

## Findings corrected

- A create reply from a previous Environment could close the current dialog.
  Unmounted dialogs now discard replies; scope checks precede state mutations.
- Duplicate resource aliases in the main editor could overwrite dictionary entries.
  Save rejects invalid, duplicate and colliding virtual-resource aliases.
- Changing a resource Type could silently preserve an output with the same name.
  The row keeps the old choice visible and requires explicit output selection again.
- Non-URL Service outputs were classified as URL bindings; they now retain the
  advanced Score path. Invalid environment-variable names fail before submission.
- Empty legacy literals matched an empty regex search result and bypassed the
  provenance check. Empty and whitespace literals now obey the same proof requirement.
- Busy dialogs could lose keyboard focus; Tab stays inside during mutation.
- Long key, dependency and workload lists now have a name filter. Filtering keeps
  the selected value and never changes the binding until an option is selected.
- Product CI lacked Terraform and disposable PostgreSQL integration setup. It now
  installs the pinned Terraform 1.9.8 and Redis, requires PostgreSQL/Terraform tests,
  and allows 30 minutes for the sequential Terraform integration package.
- ESLint receives the actual browser/Node globals for affected prototype and E2E
  scripts. Removed unused prototype declarations; no lint rules are disabled.

## Verification environment

Real product UI and HTTP handlers; local PostgreSQL 16 in the disposable container
`pr15-review-postgres`; Terraform 1.9.8; Redis installed for the acceptance-app tests.
The browser fixture uses fake execution adapters and a synthetic Secret Store.
No cloud or Kubernetes deployment was performed. Existing application data and
running user services were not used for these checks.

UI reference: the product workload editor and shared UI components/styles from
[shared UI design](../architecture/ui/README.md). Browser captures before and after
corrections cover Vietnamese/English at 1512×980 and 1280×900. The table and dialog
reuse the existing shell; search controls appear only for lists longer than ten
items. No additional explanatory paragraphs were added to the workflow.

## Validation status

Initial full frontend
execution oversubscribed workers on this host and hit existing 5-second test
limits; the repeat uses four workers without increasing test timeouts. The first
backend suite passed all packages except the sequential Terraform integration
package, which exceeded Go's default ten-minute package timeout; that package is
rerun with a 30-minute timeout, preserving all tests and assertions.

| Check | Result |
|---|---|
| Frontend typecheck, lint, production build | PASS |
| `npm test -- --maxWorkers=4` | PASS: 881 tests in 71 files; no timeout increase |
| Backend affected-package tests, build, vet | PASS |
| `go test -race ./internal/planning ./test/e2e ./test/conformance` with PostgreSQL required | PASS |
| Browser acceptance | PASS: 30 assertions, both languages and desktop sizes, including long-catalog search |
| Documentation checker, `git diff --check` | PASS |

Browser evidence is local at `/tmp/pr15-final-v2/` (screenshots, assertions and
result); these are synthetic fixture data, not production deployment evidence.
The final merge gate is the complete backend/frontend/documentation check suite
on the final PR head, visible in [PR checks](https://github.com/pr3s3nt/poc/pull/15/checks).
