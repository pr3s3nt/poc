---
id: OPS-TERRAFORM-MODULE-SOURCES
artifact: operational-runbook
status: current
last_reviewed: 2026-10-21
---

# Terraform modules from GitHub

The seven reference modules are published at
[pr3s3nt/terraform-modules](https://github.com/pr3s3nt/terraform-modules).
Release `v1.0.0` resolves to commit
`6b991b16c39ac8485c74fa6756cf3223a129c8c9`; module contents match the
product's packaged release `v4-2026.10.15` unchanged.

## Register a Definition

1. Open Resource Definitions → Tạo cấu hình and select the Resource Type.
   Git is the initial source per [UC-03 GIT-03](../usecase/UC-03/specification.md).
2. Enter Git HTTPS URL `https://github.com/pr3s3nt/terraform-modules.git`.
3. Choose a directory from the table below and enter tag `v1.0.0` or the full
   commit above. Click Kiểm tra module; the server pins commit and content digest.
4. Configure typed variables, mappings and provider bindings, then save.
   AWS modules require a READY Driver Account and explicit region. Kubernetes
   consumers bind to their matched cluster resource. Apply presets/overrides
   explicitly; Git inspection does not create resources.

| Directory | Resource Type |
|---|---|
| `modules/vpc` | `vpc` |
| `modules/eks` | `k8s-cluster` |
| `modules/aurora` | `postgres` |
| `modules/elasticache` | `redis` |
| `modules/k8s-namespace` | `k8s-namespace` |
| `modules/redis-k8s` | `redis` |
| `modules/postgres-k8s` | `postgres` |

Editing an existing Definition preserves its stored pin. Packaged snapshots
remain available for original plans/state/cleanup. A new Git-sourced Definition
does not migrate an existing deployed resource. Release-equivalent identity
and executor outputs follow the [module contract](../architecture/contracts/terraform-modules.md).

## Verify the public source without deployment

From `frontend/`, run `npm run build`. From `backend/`, run:

```bash
PUBLISHED_MODULE_COMMIT=6b991b16c39ac8485c74fa6756cf3223a129c8c9 \
  go run ./test/integration/gitmodulebrowser \
  -scenario ../frontend/test/e2e/published-modules-local.mjs
```

This uses an isolated in-memory catalog and browser session, reads the public
GitHub source, inspects all seven modules and checks the exact release digests,
Git pins and EKS executor-output mapping. No provider apply or cloud/database
mutation runs. General Git create/edit/CAS/reset coverage uses the same harness
with its default scenario and temporary HTTPS Git fixture.

## Publish later versions

Use a new tag for changed module contents; never move `v1.0.0`. Confirm the
platform provider release and module/executor contract before authoring a new
Definition. A changed module does not inherit the release-equivalent module
identity solely because its path/name matches; arbitrary custom EKS/namespace
lifecycle extensions require their own design. Existing pins keep their source,
commit, digest and historical lifecycle artifacts.
