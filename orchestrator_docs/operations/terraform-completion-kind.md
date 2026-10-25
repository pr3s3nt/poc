---
id: RUNBOOK-TERRAFORM-COMPLETION-KIND
artifact: operations-runbook
status: current
last_reviewed: 2026-10-26
---

# Generic Terraform resource completion on real kind

User authorization 2026-10-26: verify complex successful user flows through
headed Playwright with real backend, Terraform, PostgreSQL, Vault and kind.
No fake adapters, mock services or intercepted success responses. No AWS calls.
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
   really create ConfigMap and ServiceAccount objects, neither of which has a
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
