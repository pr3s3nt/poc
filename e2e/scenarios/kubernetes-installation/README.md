# Kubernetes installation acceptance — issue #91

[Deployment design](../../../orchestrator_docs/architecture/deployment/kubernetes.md)
and [operator runbook](../../../deploy/kubernetes/README.md) own the behavior.
Run only on an explicitly owned installation in kind-k8s-4f, never the main Console.
Preconditions: prepared private inputs, manually initialized/unsealed Vault,
completed phased installation, all four images preloaded, task-owned HTTPRoute
hostname reachable through the existing Traefik Gateway and Tailscale forwarding.
Operator must choose initial/update image references and a new private backup path.

Dependencies: Node 22+, kubectl, OpenSSL/tar, Playwright Chromium;
`npm ci --prefix scripts/kubernetes`, `npm ci --prefix frontend`,
`npm ci --prefix scripts/bootstrap`, `npm ci --prefix e2e`.
Shared Console access comes from `e2e/lib/console.mjs`; first-run initialization/Git
assertions reuse `e2e/scenarios/first-run-setup/run.mjs` without a second auth runner.

```sh
INSTALL_CONFIG=/private/installation/config.json INSTALL_PRIVATE=/private/installation \
INSTALL_UPGRADE_CONFIG=/private/installation/update.json INSTALL_BACKUP_DIR=/private/backup-unique \
ORCH_KUBECONFIG=/private/operator-kubeconfig ORCH_CONTEXT=kind-k8s-4f \
ORCH_PLATFORM_URL=http://verified-trial-host ORCH_PLATFORM_USERNAME=setup-admin \
ORCH_PLATFORM_PASSWORD_FILE=/private/admin-password ORCH_EVIDENCE_DIR=/private/evidence \
ORCH_HEADLESS=1 node e2e/scenarios/kubernetes-installation/run.mjs
```

Assertions: real Organization/Admin and 10-Type/8-Definition Git import; a real kind
account uploaded through Console and persisted through backend/Vault TLS; absence
of root/unseal material from Secrets; image-update backup/owner gates; unchanged
install; operator unseal after Vault Pod recreation; backend workspace marker,
database installation/domain/source IDs, PVC and Secret bytes/UID preservation;
exact Tailscale browser/API recheck. No app/workload deployment or shared RBAC.

Side effects: one private kind account in the trial database/Vault, a public marker
in its backend PVC, new owner Job receipts and private archives/evidence. Kubeconfig
is exported privately with the same certificate identity and in-cluster API DNS;
credential bytes never appear on screen, in logs or arguments. Do not provide a
credential you are not authorized to upload into this task-owned installation.
Screenshots occur only after concealed credential submission. No storage state.

The backup archive listing is validated by the installer. Isolated restore rehearsal
is a separate owned Pod check, recorded with the execution; archive listing alone
must not be reported as a restore test. After a successful main scenario, keep the
same private evidence directory (its `before.json` is the expected database identity)
and completed backup directory. Point `INSTALL_CONFIG` to the updated configuration:

```sh
INSTALL_CONFIG=/private/installation/update.json INSTALL_PRIVATE=/private/installation \
INSTALL_BACKUP_DIR=/private/backup-unique ORCH_KUBECONFIG=/private/operator-kubeconfig \
ORCH_CONTEXT=kind-k8s-4f ORCH_EVIDENCE_DIR=/private/evidence \
node e2e/scenarios/kubernetes-installation/restore.mjs
```

`restore.mjs` restores PostgreSQL and Vault into temporary emptyDir Pods, never the
retained PVCs. It checks database identities/catalog, the backend archive marker,
operator unseal with external shares, scoped token policy and exact stored credential
hash. `restore-receipt.json` records each result and verified cleanup; failure logs
remain private. The temporary PostgreSQL password is an isolated fixture, not an
installation credential. Matching Vault shares must still be valid for that backup.
`INSTALL_SKIP_UPGRADE=1` may inspect an
already-checked updated installation; that mode cannot claim an upgrade was run.

Cleanup: keep trial namespace/PVCs for user acceptance. Operator-run cleanup is
explicit and UID/ownership checked; there is no destructive reset command. Temporary
image-loader/backup/restore/full-check resources must be removed with their original
UID and labels; verify absence. Never delete an unrelated installation to retry.
