---
id: VERIFY-20261125-KUBERNETES-ONLY-DEPLOYMENT
artifact: verification-record
status: current
last_reviewed: 2026-11-25
---

# Console migration to the Kubernetes installer

[Issue #101](https://github.com/pr3s3nt/poc/issues/101) moved the retained kind
Console to the [Kubernetes installer](../../deploy/kubernetes/README.md).
The source and target had separate PVCs and namespace identities. The same
backend and frontend image contents were used for the migration.

## Restore and cutover

- A quiesced source backup captured PostgreSQL, Vault file storage, state
  encryption key and backend workspace. The source resumed while restore
  was rehearsed. Backup material remained in a private directory outside Git.
- PostgreSQL installation identity, account IDs, Application IDs and ciphertext
  matched. Both restored Terraform state records decrypted with the original key.
  Vault cluster identity and the contents and history of all ten KV keys matched.
- First owner setup stopped on a database-level ACL mismatch. The source
  was restarted before retrying. Restoring the source database ACL on the
  target resolved the mismatch. No released schema or marker was changed.
- The OIDC rollout exposed the incompatible first-run setup token. The renderer
  was corrected and the backend rolled out with the corrected rendered
  manifest. The installer reported successful upgrade and check.
- Both existing Console routes were switched to the new frontend. Chromium
  verified local Admin and the retained Keycloak Application. A real GitHub
  login created an OIDC session on the target with the retained Organization
  and Platform Engineer role.
- The old Vault Service address forwards requests to the restored Vault. The
  existing Keycloak ServiceAccount authenticated and read its Vault secret
  through that address. The old backend, frontend, PostgreSQL and Vault
  workloads were stopped. Source storage and rollback inputs were retained.

## Installer validation

An isolated kind installation completed prepare, dependency rollout, manual
Vault initialization, install, first-run Organization setup and an OIDC
upgrade from the corrected tree. Validation included local Admin after
upgrade and owned namespace/PV cleanup, with live GitHub login verified
separately on the migrated Console.

The 13 installer tests, Node syntax, documentation and Git whitespace checks
passed. PlantUML was unavailable and no diagrams changed. The broader Product
CI suite and external company environment were outside this verification.
Private evidence and rollback notes were retained outside Git.
