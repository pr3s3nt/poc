---
id: WORKLOAD-CATALOG-IMPLEMENTATION
artifact: implementation-handoff
status: current
last_reviewed: 2026-10-29
---

# Workload catalog — implementation entry, issue #24

Issue: https://github.com/pr3s3nt/poc/issues/24. Branch:
`features/24-workload-catalog`. User authorized implementation, existing local
kind and merge to main **after** acceptance. No data migration/adoption or changes
to the old Passbolt installation. No subagent writer is used.

## Decisions and canonical sources

Keep the original application home; Add Workload opens the scoped template
picker. MySQL is an independent Terraform resource; backup and cleanup are two
optional independent scheduled workloads. Removing a consumer retains storage;
namespace deletion is refused while retained data remains. Explicit confirmed
resource deletion is separate. Passbolt backups target S3-compatible storage.

Read [specification](../usecase/UC-16/workload-catalog-specification.md),
[ADR-025](../architecture/decisions/ADR-025-workload-catalog.md),
[contract](../architecture/contracts/workload-catalog.md),
[UI](../usecase/UC-16/ui/workload-catalog.md),
[realization](../usecase/UC-16/workload-catalog-realization.md) and
[Passbolt preparation](../operations/passbolt-catalog.md).

## Implemented on the task branch

- Optional typed `workloadExecution` v1 survives Score/module reconstruction,
  draft/applied JSONB and reopening the database. No DDL reset or new registry.
- Immutable Git bundle metadata publishes bounded template schemas/defaults.
  Org-scoped catalog read, Save validation and matched-renderer capability pin
  reject conflicts and legacy silent fallback before provisioning.
- Existing editor/picker handles Deployment, StatefulSet and CronJob, container
  commands/resources/HTTP probes, Service/headless identity, dependency mounts,
  schedule/timezone/policies, UC-12 env and read-only secret files. Imported fields
  beyond the form remain in advanced Score mode.
- Standard Git bundle renders the real primary controller. CronJob uses nested
  Pod templates and truthful scheduled status; explicit suspension and no active
  UID-owned Job are required before removal. Live Job inspection follows the
  inventoried deployment owner and CronJob UID; stored logs filter Pod -> Job UID.
- Source Terraform modules provide stable PVC and reference MySQL resource.
  WaitForFirstConsumer is supported. PVC is not in workload manifest inventory.
  Shared mount dependencies are counted even without an env-output placeholder.
- Removal leaves storage UNREFERENCED; retained storage blocks namespace cascade.
  Known storage parameter changes are rejected by Preview; expansion is not
  advertised without a verified capability. Destructive Terraform guards remain.
- VSO file projection uses approved revision Secret/key/mode and retains old
  revision references for live Jobs. Legacy Agent delivery refuses file intent;
  file-only secret consumers are checked before resource side effects.

## Planned traceability/tests

| Requirement | Source/check | Current evidence |
|---|---|---|
| WC-01..03, WC-CAT | Template registry/schema, API and picker tests | Automated + real original-home picker r1 |
| WC-04..10 | environment/score/workloadconfig/planning/Postgres tests | Round-trip, explicit values and real SQL reopen pass |
| WC-11, WC-LIFE | planner storage/dependency tests, resourcecleanup tests, browser runner | r2 Pod restart/update/reuse/retention/explicit deletion pass |
| WC-12, WC-DEP | renderer/guard/Kubernetes tests, browser runner | r1 scheduled Job complete and Suspend/remove pass |
| WC-13 | Service/route renderer tests and browser Service | r1 ClusterIP + identity Service pass |
| WC-OBS | scoped applied inventory service + UID reader + dialog tests | Automated + r2 real Job dialog and collected logs pass |
| WC-SEC | approved projections/guard/rename/revision tests | Automated; live file-secret/resource prerequisites not yet accepted |
| WC-RES | portable module init/validate, real PVC | Both modules validate; r1 real delayed-bound PVC pass; MySQL runtime pending |
| Passbolt | fresh app/DB, two jobs, S3 backup/cleanup and restore | Pending private image/entrypoint, config and binding inputs |

## Acceptance and remaining work

[Runner](../../e2e/scenarios/workload-catalog/README.md) uses an isolated Console
project `catalog24`, context `kind-idp-internal`, new run-owned application
namespaces. Credentials/evidence are under `/tmp/workload-catalog-24` outside Git.
Both generic runs passed and deleted their own PVC/namespace via the product.
Expanded r2 proves update/reuse and the Job dialog; scoped reads prove collected
logs and persisted cleanup success. This is not Passbolt acceptance. See [verification](../verification/2026-10-29-workload-catalog.md).

Remaining: final validation/docs, Terraform SQL harness and real MySQL/Passbolt/S3/secret delivery acceptance. The MySQL
module accepts existing platform-managed Secret refs; automatic resource-scoped
secret prerequisite delivery is not implemented. Do not claim a resource secret
name is equivalent to creating/synchronizing that Secret. The broader probe
union/volume-mode requirements remain an implementation gap (advanced mode does
not invent unsupported backend behavior). No main merge until required gates pass.

Passbolt inputs requested from user: sanitized manifests/image/entrypoint path,
registry binding, URL/mail/GPG initialization and S3 settings/credential refs.
Do not request or store secret bytes in issue/Git/evidence. Missing inputs block
Passbolt configuration/runtime only; independent generic work continues.
