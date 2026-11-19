---
id: VERIFY-20261119-APPLICATION-CATALOG-DELETION
artifact: verification-record
status: current
last_reviewed: 2026-11-19
---

# Application and catalog deletion on main, issue #80

At the user's request, feature head `d7fa3ef0` was merged and pushed as
`77fee0d1` before the main runtime test. The backend and frontend source trees
are identical to the full-test candidate. The kind rollout built and imported both
images, took private platform/Vault and PostgreSQL backups, and restored the
database into an isolated Pod to verify the retained identities. The owner setup
Job completed. Both main Deployments became Ready, and the existing installation
and data identities were preserved.

## Retained kind verification

- Browser sign-in through the main Tailscale Console reached real Application and
  catalog APIs. The separate Force button deleted a new ACTIVE Application that
  had never deployed.
- A second ACTIVE fixture deployed two workloads through the verified Fleet
  provider. A migration Job completed against PostgreSQL, the Deployment was
  present, and the database PVC was bound to a real PV. Force withdrew the Fleet
  GitRepo and removed the workloads and database. The namespace ownership guard
  preserved the role and role binding installed separately by the Dev/Test
  bootstrap. After those two fixture objects were removed with UID and
  resourceVersion preconditions, a second Force from DELETING completed. Live
  Kubernetes reads verified namespace, GitRepo and PV absence. The remote Git
  repository is retained by the delivery contract.
- A third fixture failed its first deployment after a migration Job reached
  `BackoffLimitExceeded`. The first Force attempt removed the GitRepo and
  PostgreSQL/PVC, but left a kept Helm Job and three failed Pods. The failed
  publication stored an exact receipt and had no live-object UIDs in its
  applied inventory.
- Fix `9f03b41a` verifies remaining objects from the original release receipt
  after Fleet withdrawal. It checks each recovered object's exact revision and
  scoped Helm ownership and deletes with UID and resourceVersion preconditions.
  It can recheck the old acknowledged removal under a new Environment fence.
  Regression tests cover receipt discovery and exclusion of a different
  revision, handoff records, and already verified removals.
- The corrected backend was built and deployed, reusing the identical frontend
  source. A second private backup/restore rehearsal and owner Job preserved
  the retained installation. Force on the **same** DELETING fixture removed
  the Job and all three Pods. The separate bootstrap Role and RoleBinding were
  still protected and then retired with their captured UIDs. Another Force
  completed in both Environments. Native reads verified namespace, GitRepo and
  PV absence.
- Console catalog deletion returned HTTP 409 and displayed the Definition
  reference in the Type deletion dialog. Deleting the Definition and then
  the custom Type returned HTTP 204, and both rows disappeared. PostgreSQL
  integration tests also checked built-in Type, role restrictions and
  concurrent registration.
- The original `migrate71` was already a DELETED tombstone with a SUCCEEDED
  deletion record dated before this rollout. It was absent from the Console, and a live Fleet GitRepo
  still matched its recorded repository allocation. The existing tombstone and
  controller were left in place. The original-fixture criterion stays open.

## Additional checks

The isolated feature stack verified normal and Force deletion and recovery after
an intentional backend interruption during Vault purge. Recovery completed the
same operation and all 75 checked Vault references were absent. A kind storage
probe checked UID-based matching and physical PV absence. Main focused tests
passed for application/catalog deletion on PostgreSQL, cleanup and delivery
adapters, and 28 frontend tests.

Full checks on kind used correction `9f03b41a` and the stored result from the
same runner on baseline `04feb705`. The corrected frontend passed typecheck,
lint, build and 1,105 tests. Go builds passed for both commits. The correction
had 1,620 passing Go tests versus 1,611 on the baseline. The exact sets of 103
failures, eight skips and 20 failed packages matched. This leaves the existing
Go failures open. Both new regression tests passed in the full run. The runner
verified deletion of its disposable namespace.

## Handoff

The merged feature branch was retired locally and remotely. The Docker feature
stack was stopped and its volumes retained. The main Console stays online on the
existing kind installation with its PostgreSQL, Vault and persistent storage.
See the [Tailscale runbook](../operations/tailscale-handoff.md) for the access
path. The verified URL and private test/backup receipts are kept outside Git.
