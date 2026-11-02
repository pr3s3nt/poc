---
id: VERIFY-2026-11-02-CATALOG-ONLY-LIVE
artifact: verification-record
status: evidence
last_reviewed: 2026-11-02
---

# Catalog-only configuration — live Console and Kubernetes

Scope: [CR-06/07](../usecase/UC-03/catalog-releases.md), read-only release details,
Git-owned workload configuration and selected-release-only Preview/Deploy.
[Runner and prerequisites](../../e2e/scenarios/catalog-only/README.md).

## Actual environment and actions

Console `http://localhost:3001`, Compose project `thieu-staging`, real Kubernetes
adapters, existing PostgreSQL/Vault, account `k8s-4f` READY and context
`kind-k8s-4f`. The browser used the actual backend and HTTPS GitHub transport;
no route interception or fabricated API/provider responses counted as acceptance.
Existing applications, volumes and credentials were retained.

Source `catalog-only-20261102` tracks `main` in
`https://github.com/pr3s3nt/poc.git`, root `catalog.yaml`, tags `v0.20261102.*`,
with the symbolic `internal` account slot bound to the real `k8s-4f` account.
Two immutable releases were published:

| Release | Commit | Workload policy |
|---|---|---|
| `v0.20261102.1` | `ca447d6b3326c49e116ae955caf7718ce547deb9` | Default one replica |
| `v0.20261102.2` | `c78cdbaa1455e6a171f07e2019a9598c34a2ed36` | Definition version 2, `defaultReplicas: 2` |

All business mutations used Console controls: source registration/refresh, schema
registration, Application creation, Environment selection, Score file import,
draft save, Preview and Deploy. API reads and Kubernetes reads supplemented the
UI assertions. A temporary local service port-forward supported a real browser
HTTP check; it is not an ingress/public-domain acceptance test.

Owned Application: `1e0135af-d25e-4dd3-abd2-b49239e1cd09`.
Namespace: `app-1e0135af-d25e-4dd3-abd2-b49239e1cd09-staging`.
The real `nginx:1.27-alpine` Deployment `web` was retained for review.

## Observations

- Release discovery verified five Terraform components, a render bundle and seven
  Definition snapshots. The UI displays their paths, hashes, criteria, inputs and
  placement read-only, with Git-edit guidance and no global editor navigation.
- The first release was selected and deployed successfully through UI. Kubernetes
  reported one Ready Pod; the service returned HTTP 200 with the nginx page.
- The second release was refreshed, selected, Previewed and deployed through UI.
  Preview reported workload UPDATE; applied advanced only on success.
- Kubernetes then reported two Ready Pods. Logical resource ID
  `02225108-e302-44ca-b52a-740258542182` remained present. Namespace UID
  `6f10cddb-765a-4700-bfd3-f98bd2824845` and Deployment UID
  `db5d529c-9b50-401b-92b4-8775ccf5b154` were unchanged. Browser HTTP remained 200.
- Production's Catalog state remained unselected and unchanged. Every application
  present before the scenario remained present; unrelated workloads were not deployed.
- Backend/frontend containers were forcibly recreated with the same volumes.
  The module cache retained 17 entries. The actual UI still showed applied and
  desired `v0.20261102.2`; Preview returned zero changes and no Catalog pending
  flag. The read-only policy view still showed two replicas.

## Failures found and corrected

Initial discovery safely refused publication because the retained Console lacked
MySQL's Resource Type schema. That prerequisite was registered through the real
Resource Types form; existing schemas were preserved.

The first upgrade attempt exposed an actual cache problem: the old Compose setup
used a temporary Terraform module cache, lost on backend recreation. Reusing an
unchanged historical module pin then failed verification. Default Compose/image
configuration now uses a durable module cache. During preparation, a missing cache
can be rebuilt only from the already verified full Git commit and must reproduce
its complete stored identity. Existing corrupt cache entries, metadata reads and
execution do not receive a repair/fetch bypass. A real HTTPS Git regression test
checks restoration after branch movement and continued rejection of corrupt cache.

The corrected live upgrade and post-recreation UI checks passed. Failed screenshots
were retained alongside successful evidence. Temporary diagnostic logging was
removed before the successful final runtime build.

## Evidence and limits

Nonsecret screenshots, result JSON, Kubernetes UID/replica reads and container
identity observations are outside Git under:
`/home/thanhnt1/.local/state/orch-acceptance/catalog-only-20261102-runtime/`.
`live-v1/` and `live-v2/` contain the browser and Kubernetes observations;
`compose.catalog.yml` and `start.sh` retain the runtime images/cache configuration.
No password, token, kubeconfig or Terraform state was copied into evidence/Git.

The product UI reuses AppShell, Catalog content panels, Button and existing
Definition review styles. The expanded configuration was visually checked on the
actual Console and adjusted to stack content and wrap long JSON references.

Go tests/build, frontend typecheck/lint/build, all 775 frontend tests (82 files,
two workers), 22 bootstrap tests, Compose validation and documentation checks
passed. The unchanged Git settings lint warning and bundle-size build warning
remain. An initial concurrent frontend run hit the existing observability timing
test; the complete two-worker suite passed without modifying that test.

This run verifies a real stateless workload and Catalog selection/upgrade/cache
persistence. It does not claim PostgreSQL/Redis/PVC data migration, AWS deployment,
public routing or every workload kind. The source and owned app remain available
for review; resource cleanup must use the existing ownership-aware Console flow.
