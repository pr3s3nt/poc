---
id: RUNBOOK-FLEET-GITREPO-KIND
artifact: operations-runbook
status: current
last_reviewed: 2026-09-28
---

# Fleet GitRepo delivery on kind

This optional UC-06/07 mode uses Fleet on `kind-idp-internal`. It does not
build images: UC-16 Score images must already exist in Harbor. UC-08 still
creates namespaces and resource dependencies through its Kubernetes executor.

## Bootstrap state

- Private manifest repository: `pr3s3nt/poc-fleet-gitops`, branch `main`.
- Local writable clone: `/home/thanhnt1/idp_lab/read_code/poc-fleet-gitops`.
- Fleet GitRepo: `fleet-local/poc-workloads`, declared in
  [`deploy/kind/fleet-poc-gitrepo.yaml`](../../deploy/kind/fleet-poc-gitrepo.yaml).
- Fleet's read-only SSH deploy key is outside the repository under the owner's
  `.local/share/poc-fleet/`; its Kubernetes SSH auth Secret is
  `fleet-local/fleet-poc-gitops-ro`.
- A Harbor read-only project robot account and Docker config are in owner-only
  files under `.local/share/poc-harbor/`. The test robot has a 365-day lifetime;
  rotate it before expiry. Never commit or print it.

Verify without reading credentials:

```bash
kubectl config current-context
kubectl -n fleet-local get gitrepo poc-workloads
kubectl -n fleet-local get secret fleet-poc-gitops-ro -o name
git -C /home/thanhnt1/idp_lab/read_code/poc-fleet-gitops status --short --branch
```

Fleet's `status.commit` should advance to repository HEAD after a push. For a
workload, the GitOps adapter waits for that commit and for the exact Deployment
revision label to roll out. On failure, inspect GitRepo, Bundle and
BundleDeployment status before retrying Preview → Deploy.

Start Orchestrator with `-adapters kubernetes -workload-delivery fleet-gitrepo`
plus matching `-kube-context`, `-gitops-repo-dir`, `-gitops-branch`,
`-fleet-gitrepo-name`, `-harbor-registry` and `-harbor-dockerconfig-file` flags.
The host Git client must push non-interactively; the local clone uses the
existing `gh` credential helper. The Docker config must contain an `auths`
entry for exactly the registry host in the Score image reference. Orchestrator
creates a namespace-local `harbor-pull` Secret before writing manifests to
Git. Registry credentials never enter the GitOps repository.

The current Harbor chart exposes only a ClusterIP. A BusyBox sample image was
mirrored to `library/busybox:1.37-poc` for verification; no source build was
performed. The kind node reaches Harbor at `10.96.91.170:80`; HTTP registry
access was configured on that node from
[`deploy/kind/harbor-containerd-hosts.toml`](../../deploy/kind/harbor-containerd-hosts.toml)
under `/etc/containerd/certs.d/10.96.91.170:80/hosts.toml`. This is kind-only
and not a production TLS pattern. The Service IP and node config must be
rechecked if Harbor or the cluster is recreated. Pod reachability alone does
not prove node image-pull reachability.

## Recovery and cleanup

The GitOps repository is externally durable; the Orchestrator JSON snapshot
does not own it. If Git push succeeded but Fleet failed, fix the Fleet/Harbor
issue and re-run Preview → Deploy. Never switch the same workload back to
direct kubectl management without reconciling ownership. Test cleanup removes
only the run-scoped bundle directory and namespace; it must not delete the
shared Fleet or Harbor installations, PVCs or credential files.

## References

- [ADR-007](../architecture/decisions/ADR-007-fleet-gitrepo-workload-delivery.md)
- [Fleet GitRepo reference](https://fleet.rancher.io/reference/ref-gitrepo)
