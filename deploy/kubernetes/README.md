# Orchestrator Kubernetes installation

Issue #91. [Architecture](../../orchestrator_docs/architecture/deployment/kubernetes.md)
is the approved design; [first-run](../../orchestrator_docs/operations/first-run-setup.md)
owns Console initialization. Each cluster is a separate installation: no shared
PostgreSQL/Vault/state key/setup token, no fixed Organization/Admin/catalog.

## Requirements

- Node 22+, kubectl with Kustomize, OpenSSL, tar; `npm ci --prefix scripts/kubernetes`.
- Existing Traefik Gateway/Gateway API and DNS/TLS for the Console hostname. The
  Gateway must permit this namespace. This package never installs or widens them.
- StorageClass supporting three RWO PVCs; reviewed backend/frontend/PostgreSQL/Vault
  images and registry access. Initial backend/frontend/PostgreSQL/Vault are single
  replicas; this is not an HA package. PostgreSQL/Vault require TLS, not dev mode.
- Explicit operator Kubernetes context, with permissions confined to the installation
  resources and read access to its existing Gateway. No runtime ServiceAccount token,
  ClusterRole or shared RBAC is created. VSO/Fleet/workload permissions are separate
  prerequisites for subsequent application deployment.
- Owner-controlled private directory outside every Git tree, mode 0700. Keep
  credentials, CA/private keys, Vault initialization response and evidence there.

## Three overlays, one base

`base/` holds the common Services/Deployments, separate PostgreSQL/Vault/backend
PVCs, owner Job and HTTPRoute. `overlays/{kind,staging,production}` customize target
settings with Kustomize. `deploy/local/kustomization.yaml` references canonical
existing PostgreSQL role/policy files; `scripts/` holds Kubernetes-only helpers.
There are no duplicated credential policies or changes to the older kind/main
installation. Rootless Docker/WSL, local image loading and test domains are not
requirements of the staging package.

Preview ordinary Kustomize YAML without touching a cluster:

```sh
kubectl kustomize deploy/kubernetes/overlays/staging
```

The checked-in examples deliberately contain CHANGE_ME placeholders. Copy the
selected `installation.example.json` outside Git and fill context, namespace,
hostname/scheme, Gateway namespace/name/listener, StorageClass/sizes, image references
and optional existing imagePullSecret names. Staging/production require HTTPS and
image digests for all four images; unresolved configuration refuses deployment.
Backend/frontend should use the exact same release digests promoted from staging.
Local kind may use reviewed locally loaded tags and its existing HTTP listener.
Do not point the kind overlay at the main Console's namespace.

Supply absolute paths to `tls.postgres` and `tls.vault` CA/cert/private-key files.
Certificates must validate for service DNS names postgres/vault and the appropriate
namespace DNS names; trust/chain and hostname checks must pass. Private keys must
be regular owner-only files and match their corresponding certificates. Test CA generation is allowed only with explicit
`--test-certificates` on the kind environment; never promote its short-lived test
certificates to staging/production. Use your reviewed PKI there.

## New installation

The operator works from repository root. Every cluster command requires an explicit
kubeconfig and a context that exactly matches the JSON input. The examples below
are command shapes, not authorization to access staging/production from an agent.

```sh
npm ci --prefix scripts/kubernetes
node scripts/kubernetes/run.mjs prepare --config /private/installation/config.json \
  --private /private/installation
node scripts/kubernetes/run.mjs render --config /private/installation/config.json \
  --private /private/installation --output /private/installation/rendered.yaml
node scripts/kubernetes/run.mjs dependencies --config /private/installation/config.json \
  --private /private/installation --kubeconfig /private/kubeconfig --context YOUR_CONTEXT
node scripts/kubernetes/run.mjs vault-init --config /private/installation/config.json \
  --private /private/installation --kubeconfig /private/kubeconfig --context YOUR_CONTEXT
node scripts/kubernetes/run.mjs install --config /private/installation/config.json \
  --private /private/installation --kubeconfig /private/kubeconfig --context YOUR_CONTEXT
```

For the task-owned kind trial only, prepare may add `--test-certificates`. Install
never initializes/unseals Vault in a background Pod. `dependencies` waits for
PostgreSQL and proves Vault process/TLS reachability; Vault can still be sealed.
`vault-init` is the explicit operator action: it creates five Shamir shares with
threshold three, saves the complete response privately outside the cluster, unseals
and configures KV v2 plus the three canonical scoped policies/tokens. No root token
or unseal share is stored in a Secret, PVC, manifest, process argument or runtime
mount. The operator must archive/distribute shares using their custody procedure;
keeping the combined response privately is not split-person custody by itself.

Scope tokens are periodic and renewed by a sidecar that mounts only the three scoped
token files and public Vault CA. Backend mounts Console/State runtime database URLs,
state key, setup token, scoped tokens and public trust; it cannot mount database
owner/superuser credentials. The database Job alone gets owner authorization.
Root initContainers only normalize private-file ownership into emptyDir before
non-root consumers start; credentials are never made group/world readable.

The namespace is labelled with a fresh private installation ID and its UID saved.
Existing mismatching namespaces/resources/Secret bytes fail rather than being
adopted or overwritten. Prepare/create requests are not blindly replayed after an
uncertain outcome. Interrupted private initialization needs operator recovery;
never delete matching PVCs or regenerate credentials to make it pass.

After install, open the actual hostname `/ui/`, enter the private `setup-token`,
choose your Organization/Admin, sign in and configure the platform/Git source.
The token is not a Git token. Organization creation permanently closes claim.
No fixed seed account, workload instance, Application or deployment is created.
A successful rollout or HTTPRoute condition does not replace browser/API checks.

## Restart / unseal

PostgreSQL and backend data survive Pod recreation. Vault deliberately starts sealed
after its Pod restarts. An operator must supply matching private shares:

```sh
node scripts/kubernetes/run.mjs unseal --config /private/installation/config.json \
  --private /private/installation --kubeconfig /private/kubeconfig --context YOUR_CONTEXT
node scripts/kubernetes/run.mjs check --config /private/installation/config.json \
  --private /private/installation --kubeconfig /private/kubeconfig --context YOUR_CONTEXT
```

By default unseal reads `vault-init.json` in the private directory; `--shares-file`
can instead reference an owner-only JSON file with `keys: [share1, share2, share3]`.
Root authorization is not required to unseal. Scoped token expiry/revocation is an
explicit operator recovery/rotation action, never silent regeneration by install.
Private state and Kubernetes Secrets must remain together with their matching data.
Check requires all four single replicas running and validates ownership, Secret data, database installation identity, TLS service
readiness and Accepted/ResolvedRefs HTTPRoute; it never creates missing credentials.

## Upgrade and backup

Review target image digests/configuration first and have the matching private
installation inputs. Resolve active/interrupted operations before upgrade. The
runner checks them before and after writer quiescence, scales backend/frontend
to zero, takes a custom-format database dump and verifies its archive listing.
It stops Vault to archive its file backend consistently and archives the backend
PVC separately on the original consumer node, using owned temporary Pods with
read-only mounts. It retains matching private inputs/certificates outside Git.
Backup Pods are deleted using fresh UID/resourceVersion preconditions. Archive bytes
stream to exclusive private files rather than accumulating in memory. Database
listing drains the remaining input after reading its table of contents, so kubectl
can close the stream even for a large dump; listing failures remain failures. Changes to
PVC class or requested size require a separate reviewed storage procedure; this
installer refuses them before stopping writers.

```sh
node scripts/kubernetes/run.mjs upgrade --config /private/installation/config.json \
  --private /private/installation --kubeconfig /private/kubeconfig --context YOUR_CONTEXT \
  --backup-dir /private/backups/UNIQUE_RELEASE
```

Upgrade explicitly resumes/unseals Vault using operator-held shares, executes a
new owner Job and waits for success before applying the new backend/frontend.
Jobs remain as upgrade receipts. No released schema/migration or marker is edited.
PVCs/Secret values/state key/installation identity are preserved. A different
existing release requires upgrade, not install. An unchanged install reuses the
existing installation and performs checks only. Missing/different data identity
requires restore/recovery, never reset.

`backup` uses the same private archive procedure and deliberately leaves writers
and Vault stopped. Resume only as a reviewed upgrade/recovery action; do not use
it casually as a read-only diagnostic. A failed backup/setup/upgrade leaves writers
stopped; inspect and recover instead of starting a potentially incompatible binary.
Archive listing is not proof of a successful restore. Before retained staging/
production upgrades, rehearse restoration into an isolated installation, verify
its schema/installation/domain identities, then restore only with the matching
old image, Vault data, state key and credentials if rollback is necessary.
[ADR-027 procedure](../../orchestrator_docs/operations/postgres-upgrades.md) remains
canonical. There is no automatic database downgrade or destructive cleanup command.

## Diagnostics and handoff

```sh
node scripts/kubernetes/run.mjs diagnostics --config /private/installation/config.json \
  --private /private/installation --kubeconfig /private/kubeconfig --context YOUR_CONTEXT
```

Produces private allowlisted resource state/conditions/image information. It excludes
Secret values, environment values, raw logs, kubeconfig and source file bodies.
Share only the reviewed output. Inspect raw logs locally if necessary and redact
before sharing; do not send private state/Vault init/backup files in issues.

Operator must additionally verify the exact Console hostname in a browser, setup,
Git init, reapplication/restart and retained state. Agent acceptance is kind only;
staging/production samples are handoff configuration, not an assertion that either
cluster has been deployed or tested. Local accounts currently follow the existing
cookie behavior; deployment packaging does not add an SSO/security redesign.
Configure approved OIDC/HTTPS operations separately before a production security
acceptance, preserving the Organization identity when switching off first-run mode.

Data is retained until explicit owner cleanup/acceptance. Namespace deletion removes
namespace objects/PVC claims and may delete volumes depending on StorageClass policy;
never use delete/reset as a schema upgrade or retry mechanism.
