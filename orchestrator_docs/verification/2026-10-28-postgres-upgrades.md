---
id: VER-2026-10-28-POSTGRES-UPGRADES
artifact: verification-record
status: evidence
last_reviewed: 2026-10-28
---

# Issue #23 PostgreSQL upgrades

Branch `features/migration` was created from the latest fetched `origin/main`
`1b22e6e` with a clean working tree. No preexisting user edits were present.

Read-only queries on the existing staging and preview PostgreSQL containers
identified baseline SQL digests matching tracked commits `1516dec` and `2a111db`.
No retained database, Vault, workload or cluster was changed. All mutation tests
used the separately labelled disposable PostgreSQL 16 container for this task.

## Verified migration behavior

Real PostgreSQL tests cover both frozen source schemas with retained application,
account/session and immutable render-bundle rows. Owner setup preserves their
identities and data, fills only the newly designed Application metadata/git kind,
and reopens with both isolated runtime roles. Registration, deployment workload
snapshot/history writes and restart reads work after upgrade.

The existing full Terraform state fixture was arranged into each released source
layout and structurally verified against the frozen source. Per-table fingerprints
before/after upgrade match, excluding only the explicit new Application/render
fields. This covers the populated module, instance, claim, state blob, capability
and lock rows. Runtime history writes and Console state ciphertext reads remain
denied by PostgreSQL.

`TestUpgradeHTTPLoginPreviewDeploy` also uses the product HTTP handlers on the
upgraded real DB: the retained account signs in and reads its retained Application;
the normal seeded fake-execution catalog saves a draft, previews, deploys one
workload successfully, reads configuration and reads the workload after restart.
This passes race detection; execution is explicitly fake, not live Kubernetes.

Additional tests pass for concurrent/repeated setup, rollback after final grant
verification fails, unknown/damaged/forged source rejection, history tampering,
active/frozen structural equivalence and comment-only baseline changes.

A custom-format pre-upgrade backup was restored to a separate disposable DB;
source marker and row fingerprints match and setup succeeds on the restored DB.
No private archive content or connection credential was written into Git.

## Checks

- Full backend tests used `REQUIRE_POSTGRES=1` and the isolated server. All packages
  except `internal/application/tfexec` passed on the initial run; that package hit
  the default cumulative 10-minute timeout while copying a Terraform fixture
  binary, without a reported assertion failure. Its retry uses the existing CI
  timeout of 30 minutes and mandatory Terraform/PostgreSQL. That complete retry
  ran all tests but failed one safety-net case because `/tmp` ran out of space
  copying Terraform binaries. Rerunning only
  `TestSafetyNetDestroysAWSResourcesInDependencyAndLayerOrder` with `TMPDIR` on
  the disk-backed `/var/tmp` passed (44.452s), with unchanged assertions/code.
  Thus every backend test passed across the full runs and focused retries; the
  initial full command did not exit successfully and is not reported as such.
- Focused migration tests pass with race detection and a real PostgreSQL server,
  including actual `pg_dump`/`pg_restore` inside the disposable test container.
- `go build ./...` and focused Go vet pass.
- Seven Python guard/workflow tests pass: immutable artifacts, schema-version
  requirements, comment-only SQL, deployment ordering and build/backup/setup
  failure gates. Deployment tests mock Docker and do not mutate an installation.
- `bash -n deploy/local/upgrade-database.sh`, Compose config with an empty env
  file, `python3 scripts/check_docs.py`, migration guard and `git diff --check`
  pass. ERD PNG was regenerated with PlantUML.

Frontend checks were not run because no Web Console code changed. No live
Kubernetes/cloud deploy or retained-DB upgrade was performed; the functional
migration gate is the isolated PostgreSQL suite. Actual installation upgrade uses
the [runbook](../operations/postgres-upgrades.md), including backup and quiescing
all old writers. Future historical baselines require separately reviewed routes.

## Cleanup and handoff

After checking the task label and exact ID, the disposable PostgreSQL container
`328120d9940d` was stopped and removed together with its anonymous volume.
Container/volume listings confirmed both absent. Existing services/sessions and
retained databases were preserved. No user-owned edits remained from task start.
