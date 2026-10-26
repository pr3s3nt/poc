---
id: RUNBOOK-TERRAFORM-COMPLETION-KIND
artifact: operations-runbook
status: current
last_reviewed: 2026-10-26
---

# Generic Terraform resource completion on real kind

User authorization 2026-10-26: verify complex successful user flows through
headed Playwright with real backend, Terraform, PostgreSQL, Vault and kind.
No fake adapters, mock services or intercepted success responses. The kind
command below makes no AWS calls. Additional authorization on 2026-10-26
requests full AWS EKS/Aurora/ElastiCache happy-path acceptance and merge to
`main` only after successful validation and verified cleanup.
Canonical behavior: [UC-08 BR-10](../usecase/UC-08/specification.md),
[completion contract](../architecture/contracts/terraform-modules.md#resource-completion-and-supplemental-readiness).

Run the existing owned-cluster/full-application acceptance with completion
coverage enabled:

```bash
bash backend/test/integration/workload-manifests-kind.sh \
  --evidence /absolute/new/evidence/directory \
  --run-id uniqueid \
  --completion-ref <full-published-commit-of-this-repository>
```

The existing runner owns a new kind cluster, isolated Compose project and
volumes, workload Vault, run-tagged images and private credential files. Its
EXIT trap removes only those resources and verifies absence. Retained user
installations are untouched. The source commit must be published on the task
branch so the product fetches the fixture through its ordinary HTTPS Git
transport; no local downloader or artifact injection substitutes for it.

## Successful business scenarios

1. PE registers the Internal account, Type/Definitions and Vault store through
   the Console, then inspects the real Git module directory
   `backend/test/acceptance/terraform-completion` and registers its Definition.
2. Developer imports frontend/backend/worker. Backend and worker share one
   `app-config` resource; frontend uses a private instance. Both instances
   really create ConfigMap and ServiceAccount objects, plus a ConfigMap through
   `kubernetes_manifest` (its provider schema has no `id` attribute). None has a
   supplemental readiness probe. Their outputs configure the workloads.
3. Preview does not mutate the cluster. Deploy produces both READY instances,
   real PostgreSQL/Redis and the application. The existing PostgreSQL module
   also verifies mixed completion: StatefulSet probe plus Service/Secret/PVC.
4. Browser opens the application through its HTTPRoute domain, verifies its
   configuration/database/cache checks and submits a job to DONE.
5. A valid private `revision` change is saved for frontend together with the
   ordinary backend annotation update. Deploy updates the private resource
   outputs in place; both shared consumers keep their resource at revision v1.
   Object UIDs, Terraform instance/state identities and PostgreSQL/Redis
   StatefulSets remain stable. Domain and job flow pass again. A normal Console
   reload retains visible results.
6. Developer removes workloads through the Console; PE deletes unreferenced
   resources through the product. Verify every owned Terraform instance is
   DESTROYED and the namespace is absent, then runner teardown verifies the
   cluster/containers/volumes/run image tags are absent.

Existing workload-manifest assertions remain enabled. `TC-*` assertions add
completion/output/reuse coverage. Unit tests cover invalid evidence, deadlines
and supplemental failures; the live run only uses successful user scenarios.
Screenshots and sanitized assertions remain outside Git; state ciphertext,
credential files and secret values never enter public evidence. An actual
execution result belongs in a dated verification record.

## Additional real AWS acceptance

Use `backend/test/integration/terraform-v2-acceptance.sh --config <private-config>
--browser-only` with scenario `A02`, following [AWS verification](aws.md) and
the [AWS acceptance safety contract](terraform-v2-acceptance.md). The application
runs on a newly authored EKS graph with managed Aurora and ElastiCache, rather
than kind. Reuse the source-bound completed V04 predecessor receipt and retain
the current completion-specific kind proof alongside it; do not manufacture a
replacement admission receipt. Recheck account/region, /32 executor access,
current prices, exact planned counts, provider pins and finite deadlines.

The current AWS browser authors the production Git render bundle at commit
`9d5b31be06f3b0b7d4051296e98f4176514ee5fc` through inspection and the template
form (ADR-021), and uses the current Definition tabs. All existing main-flow
assertions, real jobs/cache/SQL checks, valid update, restart/reuse and guarded
reverse cleanup stay enabled. Mixed AWS modules exercise provider completion
for IAM, subnets, routing, security groups and service configuration alongside
the supplemental VPC/EKS/RDS/ElastiCache probes. No intentional failure paths
are introduced. Keep the independent supervisor and original durable encrypted
state/Vault handles until every native service absence check succeeds.
