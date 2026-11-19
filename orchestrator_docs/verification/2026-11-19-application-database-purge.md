---
id: VERIFY-20261119-APPLICATION-DATABASE-PURGE
artifact: verification-record
status: current
last_reviewed: 2026-11-19
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
