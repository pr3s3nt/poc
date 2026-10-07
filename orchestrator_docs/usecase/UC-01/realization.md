---
id: UC-01-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-07
---

# UC-01 — Use Case Realization

## Trách nhiệm và system operations

`ApplicationService.CreateApplication` validates Name/Subdomain and session
Organization, generates identity, saves Application and two UNCONFIGURED
Environments/empty Sets atomically. It does not resolve a Connection.
`ApplicationService.ListApplicationConnections` supplies safe Organization READY
choices for Environment Settings; UC-04 management privileges are unchanged.
`ApplicationService.SetConnection` takes session Organization, app/env,
connection key and expected version. In one transaction it checks ownership,
unset binding/version, validates Connection and derives profile/region/runtime,
then atomically persists immutable binding and increments version. No external
calls and no runtime provisioning. Any existing binding returns conflict.

## Participants

ApplicationController, ApplicationService, EnvironmentConnectionController,
ApplicationService.SetConnection (Environment target responsibility), scoped ExecutionTargetResolver, Application,
Environment, DeploymentSet and persistence UnitOfWork/repositories. Repository
provides compare-and-set binding and prevents normal Save from replacing it.

## Trace and validation

MS-01/02 validate name/subdomain; MS-03/04 generate Application without target;
MS-05/06 create staging/production, empty Sets/namespaces; MS-07/08 persist context.
ES-01/02 load Environment and choices; ES-03/04 validate selected target;
ES-05/06 CAS unset binding, bump version and return locked view.

Test unconfigured creation without default, independent Environment targets,
set once including same-key repeat, foreign/notREADY/blank/region errors, concurrent
version race, save overwrite guards, legacy migration/restart and no external
side effects. Shared target/identity compatibility is owned by
[ADR-011](../../architecture/decisions/ADR-011-environment-execution-binding.md).
