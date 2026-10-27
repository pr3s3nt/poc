# Offline Terraform staging patch

`backend.yaml` is a strategic merge patch for the existing Deployment, not a
complete manifest. It selects the provider mirror shipped in the new backend
image. Keep the staging-owned image, secrets, storage and other deployment
settings.

See the [canonical runbook](../../orchestrator_docs/operations/terraform-offline.md)
for image build, network-isolated init checks, patch application and limitations.
