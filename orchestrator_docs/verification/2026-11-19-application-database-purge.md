---
id: VERIFY-20261119-APPLICATION-DATABASE-PURGE
artifact: verification-record
status: current
last_reviewed: 2026-11-20
---

# Application database purge, issue #94

Candidate `025be729` on `features/application-db-purge`.
The [operator command](../operations/application-database-purge.md) was run
against a disposable PostgreSQL 16 database initialized by the regular owner
setup. The built binary previewed a DELETING app, accepted its exact name,
returned `purged:true`, and left the second app present.

The PostgreSQL integration test included a held Environment operation,
encrypted Terraform state, capability and lock records, a failed receipted
Fleet delivery and a Catalog resource scoped to a prior target generation.
App-exclusive references were removed and ordinary Catalog deletion succeeded
for their Definition and Type. Another app's reference to a shared Active
Resource and its Definition survived. The preview exposed safe namespace and
Fleet names, without fields containing the planted private values. It refused
the runtime login, connected Console/State writers, another Organization, an
incorrect confirmation and an already-purged key.

A seeded cross-app foreign key failed late in the purge. A subsequent preview
proved the app, receipts and resource metadata remained and the baseline
catalog fingerprint still matched. After removing the injected conflict from
the fixture, the same command completed. Opening the normal Console login
after purge revalidated its original grants and structural fingerprint.

The existing Application deletion/Force and Catalog deletion API cases passed
using required PostgreSQL. Go build, docs validation and the immutable
migration guard passed. All databases used here were disposable fixtures.

Full verification ran the candidate Go suite on kind with required PostgreSQL
and Terraform. Go build passed and 1,622 tests passed. The exact sets of
103 failures, eight skips and 20 failed packages matched baseline `04feb705`.
The new purge regression passed. The runner verified that its disposable
namespace was gone. The identical frontend source tree reused its checked
results (1,111 tests, typecheck, lint and build).

## Web Console and schema 12

Candidate `f7e798d3` retains the owner CLI and adds the separate Settings
action. Playwright opened the actual Tailscale route of an isolated Compose
stack with PostgreSQL. Admin purged an ACTIVE demo app, and Platform Engineer
purged a stalled DELETING demo app. Developer lacked access, exact name checks
held, and another app stayed present. A Catalog Definition that was blocked
before purge became deletable afterward, followed by its Type. The retained
app sample and feature stack stayed available after the verified restart.

The screen reuses the Application Settings danger-zone and the deletion
dialog's confirmation field. The dialog shows the permanent scope, the
selected app and its known identities, with inventory expanded on demand.
This was reviewed against the existing Settings page and deletion controls.

Required-PostgreSQL tests passed for normal deletion, Force deletion, Catalog
deletion and both web authorization roles. The v11 upgrade preserved the
installation, app and encrypted Terraform state identities, defaulted to
disabled, and rolled back an unexpected grant. A rerun on kind passed all
eight historical state-upgrade variants after restoring old trigger bodies
from the frozen fixture references. The correction changes test setup only.
Frontend typecheck, lint, build and all 1,112 tests passed on kind.

The full kind suite used `0cde4c6c` and `main` at `7d52e090`. The Go
build passed on both. With the historical fixture rerun for `f7e798d3`,
1,624 top-level Go tests passed. The exact sets of 103 failures, eight
skipped tests and 20 failed packages matched `main`. Both temporary
kind namespaces were removed and their absence was confirmed. The isolated
Web Console stack remains available for acceptance.
