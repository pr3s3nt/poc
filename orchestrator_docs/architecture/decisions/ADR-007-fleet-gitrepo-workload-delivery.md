---
id: ADR-007
artifact: architecture-decision
status: current
last_reviewed: 2026-09-28
---

# ADR-007 — Fleet GitRepo workload delivery on internal kind

Status: Accepted
Date: 2026-09-28

## Context

Developers already enter an image reference in UC-16; images come from the
internal Harbor registry, not from an Orchestrator build. Fleet 0.16 is already
installed on `kind-idp-internal`. The existing UC-06 path renders and applies
workload manifests directly with kubectl, while UC-08 still provisions
namespaces and resource dependencies directly.

## Decision

1. A configured `fleet-gitrepo` workload-delivery mode replaces only the
   workload apply/remove/readiness adapter for the internal kind target. The
   planner, UC-08 resource provisioning, Vault access preparation and
   Preview-token semantics remain unchanged. The direct Kubernetes adapter
   stays available for existing verification and AWS until separately migrated.
2. Orchestrator writes one directory per Application/Environment/workload in a
   dedicated private GitOps repository. Each directory contains `fleet.yaml`
   targeting the Environment namespace and individual non-secret Kubernetes
   manifests. It commits and pushes a revision, then waits for Fleet GitRepo
   observation and the corresponding workload revision to become ready before
   marking the deployment successful. Delete removes only that workload's
   directory; Fleet performs the removal.
3. A single Fleet `GitRepo` in `fleet-local` watches the manifest paths on the
   selected branch and targets only the local cluster. Fleet receives a
   read-only deploy key; Orchestrator's Git writer credential stays outside
   product state and Git. Neither credential is copied into a manifest.
4. Image bytes are pulled by kind from Harbor. A namespace-local
   `imagePullSecret` is created out of band from an owner-only Docker config
   file before deployment; only its name may appear in Git. Raw secrets,
   including resource-output Kubernetes Secrets, must never be committed.
5. Git push is an external side effect: a failed Fleet reconciliation keeps
   the pending desired change and reports deployment failure. Re-preview/retry
   is safe and writes the same desired manifests. A GitOps directory is the
   ownership boundary; the direct workload deployer must not also manage it.

## Consequences

- The GitOps repository is an external desired-state store, separate from
  Orchestrator's immutable Deployment Set and JSON snapshot. Git commits are
  observable deployment evidence, not a replacement for UC-09 history.
- Namespace and infrastructure objects remain UC-08-owned. Secret data is
  never stored in Git, so out-of-band Kubernetes Secret lifecycle needs a
  separate operational path.
- This mode is initially scoped to `kind-idp-internal`; Harbor's node-level
  reachability and private pull credentials must pass preflight. Production
  Git write identity, branch protection and multi-cluster promotion remain
  future work.

## References

- [Fleet GitRepo resource](https://fleet.rancher.io/reference/ref-gitrepo)
- [Fleet repository paths](https://fleet.rancher.io/explanations/gitrepo-content)
- [Fleet private repository authentication](https://fleet.rancher.io/how-tos-for-users/gitrepo-add)
