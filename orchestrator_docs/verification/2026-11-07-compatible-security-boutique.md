---
id: VERIFY-2026-11-07-COMPATIBLE-SECURITY-BOUTIQUE
artifact: verification
status: current
last_reviewed: 2026-11-07
---

# Compatible security templates and Online Boutique — issue #47

Scope approved by the user: continue `features/organization_seed`, update the
private Git template, configure and deploy the eleven Online Boutique workloads
plus Redis through Console UI on the authorized kind cluster, then verify a
browser checkout. No main merge or new GitHub issue is authorized by this run.

## Template result

Commit `a0e5143b` introduces `privileged: false` as the field-level fallback for
app, init and sidecar containers across Deployment, StatefulSet and CronJob.
Explicit permitted values, including false/zero, keep precedence. Generic
presets are 1.2.0; they do not impose UID/GID, non-root execution, read-only root
filesystem, seccomp, capability dropping or token mounting. The image-specific
nginx-readonly profile and existing product security guards remain intact.
This is a compatible default informed by Baseline, not full PSA enforcement.
See the [contract](../architecture/contracts/workload-defaults.md) and
[WD specification](../usecase/UC-16/workload-defaults-specification.md).

The private source tag is v0.3.0, commit
`1ec2027f1754e4a494b050c329ba4e258b7506e6`. Redis adds a sensitive connection
string output so the form's resource picker can supply Cart without a secret
literal or unsupported mixed-expression input.

## Checks

- PASS: templatedriver, domain/templatesource and domain/environment suites.
  New regression covers explicit security intent and compatible app/init/sidecar
  defaults for all three controllers, with and without a preset.
- PASS: Redis Terraform fmt/init/validate with the configured provider mirror.
- PASS: backend `go build ./...`, documentation checker and diff whitespace check.
  Local PlantUML syntax check is skipped because the executable is unavailable.
- Full Go tests: FAIL in 17 packages. Running the identical command/container
  against archived `origin/main` (`b2d605f9ec22b133dc408edae3aae20b0c07401e`)
  fails the same 79 top-level tests in the same 17 packages. No added failing
  test name or package was observed; the full gate is still red.

Full command: `go test -p 2 -timeout 120s ./...` with Go 1.25/GOMAXPROCS=4 in the
existing tools image, followed by `go build -p 2 ./...`. This is distinct from
successful kind/browser acceptance and does not claim skipped PostgreSQL or
historical integration runners passed. No frontend product code changed here.

Failing packages on both refs: adapters/kubernetes, adapters/templatesources,
adapters/terraform/registry, application/catalog, application/configuration,
application/deployment, application/pending, application/preview,
application/provisioning, bootstrap, test/conformance, test/e2e, and integration
v03run, v04run, v2cleanup, v2ctl, v2run.

## UI and runtime execution

The [scenario](../../e2e/scenarios/online-boutique/README.md) contains the runners,
fixture provenance, exact commands, adaptations and cleanup boundaries.
API reads/response observation provide assertions; business writes use browser
forms, resource pickers, Preview, Deploy and explicit recovery controls.
No Score import, generation command, seed or API mutation creates this app.

A separate feature stack contains the trial database/Vault and the new app.
The Console registered private Git access, the cluster account, workload secret
store, default Fleet binding, custom Redis type and four Definitions. Eleven
workloads were saved through the Add Workload form. Initial Preview showed
exactly eleven DEPLOY changes, no REMOVE.

The first deployment was PARTIAL: ad became ready, Redis provisioned, and Cart
held at prerequisite delivery. Recovery initially returned 409; no ownership or
UID guard was bypassed. Vault Kubernetes auth needed the reachable forwarded API
endpoint with the certificate's Kubernetes DNS name; after technical setup VSO
reported SecretSynced/Healthy/Ready true. The scoped namespace access Role and
RoleBinding use the existing bounded bootstrap policy and product-created
namespace ownership/UID checks; no shared controller or cluster-wide grant changed.

A temporary diagnostic build changed the renderer executable fingerprint and
caused a failed retry before provisioning. That code was removed. The Git source
was inspected again, the workload Definition updated and its new revision
selected through Console Settings before the next Preview/Deploy. This run
retains failed attempts as evidence and does not count setup as acceptance.

Final runtime/browser result is pending; do not claim shop readiness from the
successful template checks, Preview or ready Redis alone. The issue remains
open and main is unmerged. Runtime credentials, logs, screenshots and state are
kept outside Git. Access URLs containing tailnet addresses belong in the private
handoff to the user, not this public record or issue comments.
