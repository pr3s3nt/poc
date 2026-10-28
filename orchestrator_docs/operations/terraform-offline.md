---
id: RUNBOOK-TERRAFORM-OFFLINE
artifact: operations-runbook
status: current
last_reviewed: 2026-10-28
---

# Offline Terraform provider installation

The staging Deployment is managed through its staging-owned configuration;
its base manifest is not present in this checkout. The former repository-specific
patch has been retired. Keep the cluster's image, secrets, service account,
security context, recovery storage and existing volumes in that configuration.

Build the backend from `backend/Dockerfile` on a connected build host and import
the resulting image into the registry reachable by staging. Providers are
checksum-verified against the embedded release during build and installed into
an immutable mirror; cache PVCs and Internet egress are not required for packaged
Terraform modules. Do not mount an empty volume over the image mirror or policy.

Validate the image before publishing (from repository root):

```bash
docker build --target offline-provider-check -t orchestrator-offline-check backend
docker run --rm --network none orchestrator-offline-check
docker build -t <internal-registry>/orchestrator-backend:<new-tag> backend
```

The check uses the production Terraform runner, all seven packaged modules and
release provider locks, as the non-root image user, with no network. It runs only
`init`, never plan/apply, and checks that the lock file remains unchanged. This
does not claim a successful application deployment or access to staging.

When a rollout is authorized, select the tested image through the normal image
delivery process and staging-owned deployment configuration. The image defaults
`ORCHESTRATOR_TERRAFORM_CLI_CONFIG_FILE` to
`/etc/orchestrator/terraform-offline.tfrc`. If the Deployment runs the binary
directly or overrides arguments, retain that environment variable or pass
`-terraform-cli-config-file /etc/orchestrator/terraform-offline.tfrc`.

The provider mirror does not require a module/plugin cache PVC. A private durable
verified module cache remains useful for Git modules across restarts, but does not solve
provider installation. Git module/render bundle sources must be reachable from
staging separately. Keep old providers and historical Terraform binaries/releases
available for cleanup before replacing a release; the shipped mirror contains
only the current embedded release's provider versions.

After rollout, create a fresh Preview and deploy `acceptance-1`. Keep the namespace
Definition's `name` input. On failure, inspect attempt stage and safe error code;
do not dump credentials or enable raw Terraform logging in shared logs.

Canonical contract: [Terraform modules](../architecture/contracts/terraform-modules.md#provider-installation-trong-môi-trường-offline).
