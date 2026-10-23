# Orchestrator on an existing Kubernetes cluster

This bundle runs the Console frontend, backend, PostgreSQL 16 and persistent
Vault in the dedicated `orchestrator-demo` namespace. It is a development/PoC
deployment, equivalent to the [Compose setup](../../orchestrator_docs/operations/docker-local.md).
It does not install a cluster, ingress controller, Gateway or DNS records.

## Test on existing kind

Run from the repository root. All cluster commands select the context explicitly.
Prerequisites: Docker, kind, kubectl, Python 3 and a default StorageClass.

```bash
docker build -t orchestrator-backend:deploy-k8s backend
docker build -t orchestrator-frontend:deploy-k8s frontend
docker pull postgres:16-alpine
docker pull hashicorp/vault:1.21
kind load docker-image --name idp-internal \
  orchestrator-backend:deploy-k8s orchestrator-frontend:deploy-k8s \
  postgres:16-alpine hashicorp/vault:1.21
python3 deploy/k8s/init-secrets.py kind-idp-internal
kubectl --context kind-idp-internal apply --dry-run=server -k deploy/k8s
kubectl --context kind-idp-internal apply -k deploy/k8s
kubectl --context kind-idp-internal -n orchestrator-demo rollout status deployment/postgres --timeout=300s
kubectl --context kind-idp-internal -n orchestrator-demo rollout status deployment/vault --timeout=300s
kubectl --context kind-idp-internal -n orchestrator-demo rollout status deployment/backend --timeout=300s
kubectl --context kind-idp-internal -n orchestrator-demo rollout status deployment/frontend --timeout=300s
kubectl --context kind-idp-internal -n orchestrator-demo port-forward service/frontend 3301:8080
```

Open <http://localhost:3301/ui/>. The seeded development accounts are
`platform-engineer` and `developer`, password `test-password`. Frontend proxies
`/api/` to the backend Service. PostgreSQL and Vault have ClusterIP Services only.

If `kind load` fails with a missing content digest due to multi-platform Docker
image metadata, this verified fallback imports only the node's platform. For the
existing single-node amd64 cluster:

```bash
set -o pipefail
docker image save --platform linux/amd64 \
  orchestrator-backend:deploy-k8s orchestrator-frontend:deploy-k8s \
  postgres:16-alpine hashicorp/vault:1.21 |
  docker exec -i idp-internal-control-plane \
    ctr --namespace=k8s.io images import --platform linux/amd64 --snapshotter=overlayfs -
```

## Credentials, initialization and persistence

`init-secrets.py` creates four Kubernetes Secrets once without writing values to
disk/stdout. It refuses an unrelated namespace, incomplete credentials, or PVCs
without matching credentials. It preserves a complete existing credential set.
Do not delete/recreate Secrets against retained data. Back up the Secrets,
PostgreSQL PVC, Vault PVC and Terraform recovery PVC together.

PostgreSQL's first-boot script creates separate owner, Console and State logins.
An init container runs the product's explicit `-setup-database` step using only
the owner credential. The backend's main container sees only runtime credentials;
an init container copies these into memory-backed files with UID 65532/mode 0400,
as required by the product's credential checks.

Vault uses file storage, one unseal key and automatic local unseal. Root/unseal
material stays in its own PVC and is not mounted into backend. Three scoped
periodic tokens are renewed by Vault's supervisor. Policies and the PostgreSQL
role-init script in `bootstrap.yaml` mirror `deploy/local/`; keep these aligned
when changing the Compose policies. No Vault PVC is shared between Pods.

All stateful Deployments use one replica and `Recreate` for RWO volumes. Backend
recovery uses a PVC; module/plugin caches remain container-local. A normal
rollout retains the database, Vault and recovery data. Applying the same manifests
does not rerun completed init containers; restart the affected Deployment after
changing ConfigMap/Secret inputs.

## Attach to the destination Traefik Gateway

1. Push frontend/backend images to a reachable registry. Change the two image
   references in `backend.yaml` (including `database-setup`) and `frontend.yaml`;
   use immutable tags/digests and add `imagePullSecrets` if required.
2. Review PVC sizes and select the destination StorageClass if no suitable default
   exists. This bundle uses the fixed namespace `orchestrator-demo`; if renaming
   it, also change the namespace in `init-secrets.py`, `kustomization.yaml` and
   `ORCHESTRATOR_PLATFORM_VAULT_WORKLOAD_ADDR` in `backend.yaml` before first boot.
3. Provision Secrets in that namespace and apply the bundle to the intended context.
4. Copy `httproute.example.yaml` outside the base, set the real hostname, Gateway
   name and Gateway namespace, and apply it with `-n orchestrator-demo`. The
   `sectionName` is `websecure`. The destination listener must allow routes from
   this namespace and its existing certificate must cover the hostname.

```bash
kubectl --context YOUR_CONTEXT -n orchestrator-demo apply --dry-run=server -f /path/to/httproute.yaml
kubectl --context YOUR_CONTEXT -n orchestrator-demo apply -f /path/to/httproute.yaml
kubectl --context YOUR_CONTEXT -n orchestrator-demo get httproute orchestrator -o yaml
```

Check `Accepted=True` and `ResolvedRefs=True` for the intended parent, then test
`https://YOUR_HOST/ui/` and sign-in through `/api/`. TLS terminates at the existing
Gateway. No DNS changes are needed. The example route follows the
[Gateway API HTTP routing guide](https://gateway-api.sigs.k8s.io/guides/http-routing/).
It is intentionally excluded from the base so kind testing does not require
installing Gateway API CRDs or changing the shared Traefik deployment.

This PoC retains the app's seeded test accounts and its existing development
Dockerfiles. Public exposure, authentication hardening and production Vault
unseal/backup design require a separate deployment review. Workload delivery
through the Console additionally requires a registered cluster Connection,
Definitions and workload Vault auth/VSO configuration; booting the Console does
not configure those integrations or grant the backend cluster-wide RBAC.

## Rollback and cleanup

Roll back a frontend/backend image by restoring the previous references and
reapplying, then verify rollout and login. Keep PVCs/Secrets unchanged. Database
schema upgrades are not performed by this bundle; incompatible retained state
requires the product's separately planned restore/reset procedure.

For troubleshooting, inspect Pods/events and component logs in this namespace.
For teardown, `kubectl delete -k deploy/k8s` also deletes the PVCs and may destroy
their data. To preserve data, delete only the four Deployments and four Services.
Never delete shared namespaces/controllers. Full namespace deletion is only for
an explicitly disposable test installation after checking its ownership label.
