---
id: RUNBOOK-VAULT-KIND
artifact: operations-runbook
status: current
last_reviewed: 2026-09-27
---

# UC-12 Vault on kind

The `kind-idp-internal` cluster has two Vault releases in namespace `vault`:
the pre-existing `vault` release (development/in-memory, owns the Injector)
and `vault-uc12` (standalone/file storage on a 1 GiB PVC). Do not replace the
former or install a second Injector webhook. The declarative Helm values for
the latter are [tracked here](../../deploy/kind/vault-uc12-values.yaml).

## Verify without reading credentials

```bash
kubectl config current-context
helm status vault-uc12 -n vault
kubectl -n vault get pod vault-uc12-0
kubectl -n vault get pvc data-vault-uc12-0
kubectl get mutatingwebhookconfigurations
kubectl -n vault exec vault-uc12-0 -- vault status
```

`vault status` may exit nonzero while Vault is sealed or uninitialized. A
running Pod and bound PVC do not mean the Vault API is usable. The server was
initialized and unsealed on 2026-09-26; the initial unseal key and root token
were written to an owner-only file outside the repository. The Pod then became
Ready. After a server restart, check status and unseal again if necessary.

## Initialize and unseal — manual security handoff

For this kind-only Vault, the operator-approved initial credentials are kept
in a mode-`0600` file under an owner-only directory outside the repository.
Do not commit the file, put it into a Kubernetes Secret, or paste its contents
into an issue/chat. Back it up in a secure location controlled by the owner;
the local file is not a substitute for a backup. Unseal is needed after server
restarts in this standalone configuration. Use Vault's own unseal documentation
for the exact procedure, keeping the key out of terminal history and process
arguments.

KV v2 at `kv/`, Kubernetes auth at `auth/kubernetes`, a scoped `orch-backend`
policy/token and the `vault-uc12-auth-delegator` ClusterRoleBinding were
configured on 2026-09-27. The backend token is held in an owner-only file
outside the repository and supplied with `-vault-token-file`; it is not a root
token. The adapter creates a narrowly scoped per-workload read policy and
Kubernetes auth role. The Pod annotations direct the existing Injector to
`vault-uc12.vault.svc:8200`, rendering a shell-sourceable file at
`/vault/secrets/app-env`. The workload startup script must source
`$ORCHESTRATOR_CONFIG_FILE` before launching its process. Values are not copied
to Kubernetes Secrets or Deployment annotations.

After a Vault restart, unseal it before starting a deploy. The current scoped
backend token has a finite TTL (created with 720h); rotate it before expiry
using the owner's secure credential workflow. Do not use the root token for
product integration. To re-run the scoped kind check, first verify context
`kind-idp-internal`, then run `bash backend/test/integration/uc12-kind-verify.sh`
from the repository root. The script creates a unique Application/namespace,
checks Preview → Deploy and secret rotation, and removes only a namespace with
its exact run label. The Application record remains in its temporary local
snapshot; it is not persisted in the product checkout.

The PVC uses kind's local-path StorageClass. It survives Pod restarts, but not
necessarily deletion of the kind cluster or the PVC; it is not a backup or HA
solution. Never uninstall `vault-uc12` or delete `data-vault-uc12-0` as a
cleanup step for a test.

## References

- [ADR-006](../architecture/decisions/ADR-006-application-configuration-provider.md)
- [Vault initialization](https://developer.hashicorp.com/vault/docs/commands/operator/init)
- [Vault Agent Injector](https://developer.hashicorp.com/vault/docs/deploy/kubernetes/injector)
