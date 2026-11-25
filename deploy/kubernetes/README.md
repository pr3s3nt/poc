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
  replicas; this is not an HA package. Vault always uses persistent server mode.
- Explicit operator Kubernetes context with access to the installation resources
  and read access to its existing Gateway. The staging sample enables
  `vaultKubernetesAuth`, which additionally requires permission to create an
  installation-owned ClusterRole and ClusterRoleBinding for TokenReview.
  VSO/Fleet/workload permissions are separate prerequisites.
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

Preview the overlay before installer substitutions without touching a cluster:

```sh
kubectl kustomize deploy/kubernetes/overlays/staging
```

The checked-in examples deliberately contain CHANGE_ME placeholders. Copy the
selected `installation.example.json` outside Git and fill context, namespace,
hostname/scheme, Gateway namespace/name/listener, StorageClass/sizes, image references
and optional existing imagePullSecret names. Staging and production require HTTPS.
In staging, each of the four images can use an explicit tag, such as
`postgres:16-alpine`, or a SHA-256 digest. You can mix tags and digests in one
configuration. Production requires digests for all four images. Use the digests
of the backend and frontend images verified on staging when promoting to production.
Unresolved configuration refuses deployment.
Local kind may use reviewed locally loaded tags and its existing HTTP listener.
Do not point the kind overlay at the main Console's namespace.

For a new staging installation on a trusted cluster network, omit `tls` as in the
staging sample. The installer uses PostgreSQL with `sslmode=disable` and Vault
over HTTP on internal Services. Database credentials and Vault traffic are
unencrypted on the cluster network in this mode. The Console still uses HTTPS
at the Gateway. Use the installer's `render` command to see the selected transport.

To enable internal TLS on staging, copy the `tls` object from the production
sample. Production requires this configuration. Supply absolute paths to
`tls.postgres` and `tls.vault` CA/cert/private-key files.
Certificates must validate for service DNS names postgres/vault and the appropriate
namespace DNS names; trust/chain and hostname checks must pass. Private keys must
be regular owner-only files and match their corresponding certificates. Test CA generation is allowed only with explicit
`--test-certificates` on the kind environment; never promote its short-lived test
certificates to staging/production. Use your reviewed PKI there. Keep the internal
TLS mode selected by `prepare` for the lifetime of the installation.

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
PostgreSQL and proves Vault reachability. Vault can still be sealed.
`vault-init` is the explicit operator action: it creates five Shamir shares with
threshold three, saves the complete response privately outside the cluster, unseals
and configures KV v2 plus the three canonical scoped policies/tokens. No root token
or unseal share is stored in a Secret, PVC, manifest, process argument or runtime
mount. The operator must archive/distribute shares using their custody procedure;
keeping the combined response privately is not split-person custody by itself.

With `vaultKubernetesAuth: true`, the installer projects a rotating reviewer
ServiceAccount token into the Vault container. `vault-init` enables the
`kubernetes` auth method for the same cluster's API server. The application
token can then read `auth/kubernetes/config`. Use this setting when Vault and
the workloads it authenticates run on the same cluster. Configure the Secret
Store with auth mount `kubernetes`.

Scope tokens are periodic and renewed by a sidecar that mounts only the three scoped
token files and, with TLS enabled, the public Vault CA. Backend mounts Console/State
runtime database URLs, state key, setup token, scoped tokens and TLS trust when
configured. It cannot mount database
owner/superuser credentials. The database Job alone gets owner authorization.
Root initContainers prepare private files in emptyDir for non-root consumers.
The backend init container also restores private permissions on its retained
workspace before the backend opens its caches.

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

## Optional Keycloak SSO

Complete [first-run setup](../../orchestrator_docs/operations/first-run-setup.md)
in local mode before enabling SSO. Follow the
[Keycloak SSO runbook](../../orchestrator_docs/operations/keycloak-sso.md) to
register a confidential client, PKCE S256, the client role mapper, and the exact
callback `https://CONSOLE_HOST/api/v1/auth/oidc/callback`. Configure GitHub or
another upstream identity provider in the Keycloak realm.

Set `"oidc": true` in the private installation config and save `oidc.json` in
the same `--private` directory as the installation state, with mode 0600:

```json
{
  "issuer": "https://KEYCLOAK_HOST/realms/REALM",
  "clientId": "orchestrator",
  "clientSecret": "KEYCLOAK_CLIENT_SECRET",
  "publicUrl": "https://CONSOLE_HOST",
  "organizationKey": "EXISTING_ORGANIZATION_KEY",
  "localTestAdminUsername": "EXISTING_LOCAL_ADMIN"
}
```

Use the exact issuer from the realm discovery document. When an isolated cluster
needs a dedicated issuer relay, the optional `oidcNetwork` object sets
`hostname`, `address` (relay IP) and `ca` (absolute path to a PEM certificate).
Configure the relay and trust before enabling OIDC. Its hostname must be the
issuer hostname, and the issuer URL and TLS validation stay in effect.  The Console origin must
match the installation scheme and hostname. Use HTTPS with a certificate trusted
by the backend. Only an isolated kind installation may opt in to HTTP with
`"allowHttp": true`. Omit `localTestAdminUsername` if no local Admin should
remain eligible in OIDC mode.

Run `render` to inspect the nonsecret backend configuration, then run `upgrade`
with the existing installation inputs. The installer checks the Organization
and fetches issuer discovery from the running backend before stopping writers.
It creates an installation-owned OIDC Secret and rejects mismatching bytes on
reuse. The backend init container prepares a private copy for its runtime UID.
Verify redirect, sign-in, role, and local Admin access through the Console.

To return to local login, remove `oidc` from the private config and run the
backed-up upgrade again. If the backend cannot start with OIDC, remove the
option before retrying. The installer retains the OIDC Secret for reuse.

## Vault Web UI

The staging sample includes `"vaultUI": {}` for private forwarding. For staging
with internal HTTP transport, set a separate hostname to add a Gateway route:

```json
"vaultUI": {"hostname": "vault.staging.example.com"}
```

Use DNS and a Gateway certificate that cover the Vault hostname. The same
HTTPS listener must allow both hostnames and the installation namespace.
Open `/ui/` on the Vault hostname. The route serves the UI and API with Vault's
own authentication and access policies.

Use `"vaultUI": {}` to enable the UI for a private forward. Forward port 8200
from Service `vault` and open `/ui/` with the Vault Service's configured protocol
and trust. Removing a managed route requires explicit operator cleanup.

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
Check requires all four single replicas running and validates ownership, Secret
data, database installation identity, dependency readiness and HTTPRoute
Accepted/ResolvedRefs conditions. It never creates missing credentials.

## Retained state key for a restore rehearsal

When restoring a retained database and Terraform state into a newly prepared
installation, save the matching key as `retained-state-key` in the private
directory and set `stateKeyId` to its original identifier before `prepare`.
Keep the identifier and key together. Omit that setting for a fresh installation
to generate an independent key. This setting does not restore the database,
Vault, or workspace, and cannot rotate the key on an existing installation.

For a logical PostgreSQL restore, preserve the database owner and database-level
ACL as well as schema and table grants. A custom-format dump restored into an
existing database does not automatically restore its database-level ACL. Compare
source and target before running the owner Job. Rehearse with the matching
encryption key and verify the restored encrypted state can be read.

## Upgrade and backup

Review target image references and configuration first and have the matching
private installation inputs. If using staging tags, choose a new version tag for
each changed image. Repointing a tag in the registry can change the image pulled
on Pod restart without changing the release the installer sees.
Resolve active/interrupted operations before upgrade. The
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

For an existing installation, add `vaultKubernetesAuth: true` and the optional
`vaultUI` settings to its existing configuration, verify that Vault is unsealed,
then run `upgrade` with matching private inputs and a new backup directory.
Keep the current images and storage settings if only enabling these features.
An existing Vault auth configuration with a different reviewer or API host
requires operator review. Decommissioning an installation should remove its
dedicated reviewer ClusterRoleBinding and ClusterRole after checking ownership.

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
