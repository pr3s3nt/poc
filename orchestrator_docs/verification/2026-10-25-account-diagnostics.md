---
id: VERIFY-ACCOUNT-DIAGNOSTICS-20261025
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-25
---

# Infrastructure Account diagnostics: independent pre-merge acceptance

[PR #17](https://github.com/pr3s3nt/poc/pull/17) implements
[UC-04 DIAG-01..14](../usecase/UC-04/specification.md#account-diagnostics--diag-0114).
Codex independently reviewed and tested head
`21d18fbb7b515d2a0506d1bd5e5d5b549678d11c` before merging.
Claude wrote code/tests in the requested `clauded` tmux session; its identity and
ownership are recorded in the [delivery handoff](../implementation/account-diagnostics.md).

## Real browser acceptance

Command: `bash backend/test/integration/diagnostics-e2e.sh --evidence NEW_PATH`,
from a clean reviewer checkout at the exact PR head. Result: **37/37 assertions
PASS**, no browser page errors, `CLEANUP-PROVEN`. This run used a newly created
kind cluster, real PostgreSQL/Vault/backend and the built nginx Console, with no
intercepted product API or fake diagnostic provider. Platform catalog and a
never-deployed workload draft were fixture setup through the product API,
separately logged; diagnostic actions used the actual Console.

| Acceptance | Actual proof |
|---|---|
| DG-E01/02 | Sign-in and real kubeconfig onboarding; General without Application; TLS/API/server version/node count/StorageClass facts matched native kubectl; connection version/status unchanged and latest summary refreshed |
| DG-E03/04 | Saved, never-deployed draft resolved; exact required class missing produced FAIL and dependent database BLOCKED; concrete remedy shown; native class creation and UI rerun passed; a real unavailable provisioner was also detected |
| DG-E05 | PVC used the exact requested class, was observed natively, and its probe pod became ready; report cleanup VERIFIED plus native namespace/PV absence |
| DG-E06 | Stop after native namespace existed; cancellation, purpose/account isolation, old failure history and versioned JSON export retained truthful scope/counts |
| DG-E07 | Browser reload, actual backend restart and history persistence; SIGKILL during execution became INTERRUPTED with preserved findings and recovered verified cleanup |
| DG-E08 | Developer HTTP403; no account version mutation; secrets/HTTPS missing prerequisites stayed FAIL/BLOCKED with visible limits, no fabricated runtime proof |

Evidence remains outside Git at
`/home/thanhnt1/projects/diagnose/evidence/diagnose-coordinator-20261025/premerge-21d18fb`:
`owned.json`, `run.json`, `assertions.json`, `assertions.txt`, screenshots,
fixture/API logs, `result.txt`, `cleanup.txt`. Run ID/cluster/Compose project:
`cdiag-261025044519-28753`. Its containers, volumes, network, images and cluster
were removed and absence verified; private runtime credentials were then removed.

The UI was visually compared with the existing Infrastructure Accounts screen
and approved integrated mock: existing shell, management forms and tab styles
remain; findings prioritize actionable results, with concrete remedies and safe
technical details. Duplicate snapshot explanations were removed.

## Additional independent evidence and checks

- Earlier built-source checks on `6f931c2` independently exercised actual
  desired-state missing-class repair, exact-class Bound PVC/readypod, UID-guarded
  class cleanup, cancellation and SIGKILL recovery. Evidence is under
  `review/browser-6f931c2` in the same evidence root.
- DG-E09: real AWS onboarding and read-only General in `ap-southeast-1` produced
  eight PASS and quota UNKNOWN. Switching to `us-east-1` preserved the old report,
  account version and verification-region configuration. Calls used saved scoped
  Vault credentials; no paid cloud writes occurred. These live checks used
  `6f931c2`; later product changes exposed safe context pins only. AWS adapter
  tests separately verify SigV4 signing, selected families and typed failures.
- Full independent frontend suite: **75 files / 909 tests PASS** on `b550e00`;
  typecheck/lint/build passed. A prior 5000ms UI-test timeout was fixed with a
  bounded test timeout; assertions were preserved.
- Independent Go checks included the full suite on `9b54d33` with required real
  PostgreSQL, later focused/race suites, build, and the explicit
  `TestSavedUndeployedDraftIsPlannedIntoExactRequirementsBeforeAnyProbe` test.
- Final PR head [Product CI](https://github.com/pr3s3nt/poc/actions/runs/40151718877)
  passed full backend/PostgreSQL tests, build/vet/race checks and frontend checks;
  [Documentation](https://github.com/pr3s3nt/poc/actions/runs/40151718806) passed.
  Earlier failures are preserved in local logs: idle snapshot rewrite and Node
  fixture lifecycle were fixed; the 150ms cluster-category race became an
  observation-then-cancel test with separate deadline coverage. An earlier push
  run failed; the later PR run on the same head passed all jobs.

## Evidence boundaries

Conditions are observations at report time; primitive pod/PVC execution does not
run an Application image or prove SQL/cache behavior. Workload-side Vault login
is unobservable. AWS policy simulation is labelled as policy evidence; quota
comparison and paid AWS execution remain UNKNOWN. No retained database was reset
or migrated. Existing user runtimes, clusters and tmux sessions were preserved.
