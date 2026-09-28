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
Kubernetes auth role. The current kind delivery uses Vault Secrets Operator
(VSO) 1.5.1 in `vault-secrets-operator-system`. It authenticates with the
workload-scoped Kubernetes role, synchronizes a pinned Vault bundle to a
namespace-local Secret, and the Pod reads keys through `secretKeyRef`.
Run the backend with `-vault-delivery vso` and an explicit Kubernetes target.
For a fresh kind cluster, install the pinned controller chart only after
verifying the active context and Vault readiness:

```bash
kubectl config current-context
helm repo add hashicorp https://helm.releases.hashicorp.com
helm repo update hashicorp
helm --kube-context kind-idp-internal install vault-secrets-operator \
  hashicorp/vault-secrets-operator --version 1.5.1 \
  --namespace vault-secrets-operator-system --create-namespace --wait
```

The pre-existing Injector is left installed for `-vault-delivery agent` legacy
mode; that mode needs a workload startup script to source
`$ORCHESTRATOR_CONFIG_FILE`. Secret values never enter Deployment annotations
or GitOps manifests, but VSO mode does store them in Kubernetes Secret/etcd.

To inspect VSO health without exposing values:

```bash
kubectl --context kind-idp-internal -n vault-secrets-operator-system get pods
kubectl --context kind-idp-internal get crd vaultstaticsecrets.secrets.hashicorp.com
kubectl --context kind-idp-internal -n <workload-namespace> get vaultconnection,vaultauth,vaultstaticsecret
kubectl --context kind-idp-internal -n <workload-namespace> get secret <orch-secret-name> -o name
```

Do not print Secret data or Vault bundle values. VSO sync failure prevents
workload apply after the sync timeout. Failed/retried deployments and old
revisions currently leave VSO objects, Kubernetes Secrets and Vault bundles;
cleanup must be planned before production. Ensure Kubernetes Secret encryption
at rest and tight RBAC before production use. VSO CRs are applied directly by
the backend before workload delivery, including when Fleet GitRepo mode is used.

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
- [ADR-008](../architecture/decisions/ADR-008-vso-native-secret-delivery.md)
- [Vault initialization](https://developer.hashicorp.com/vault/docs/commands/operator/init)
- [Vault Secrets Operator](https://developer.hashicorp.com/vault/docs/deploy/kubernetes/vso)
