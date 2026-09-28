---
id: VERIFY-2026-09-28-FLEET-GITREPO-KIND
artifact: verification-record
status: current
last_reviewed: 2026-09-28
---

# Fleet GitRepo + Harbor kind verification — 2026-09-28

Final run `fleet-20260928031442-24179` used
`backend/test/integration/fleet-gitrepo-kind-verify.sh` against
`kind-idp-internal`. Temporary evidence was produced outside the repository;
no credential or registry auth value is copied here.

Observed:

- A private GitOps repository and Fleet read-only deploy key were created.
  Fleet `fleet-local/poc-workloads` observed the initial commit. A run-scoped
  ConfigMap bundle confirmed Git path matching, reconciliation and pruning;
  its namespace was deleted after a label check.
- A BusyBox image was mirrored into internal Harbor without building source.
  The kind node pulled it from the in-cluster Harbor Service IP over the
  configured kind-only HTTP registry endpoint. The project-scoped, read-only
  pull robot also passed a separate registry login check with a temporary
  Docker config that was removed afterward.
- An authenticated developer created an Application and staged one Score
  workload. Preview → Deploy wrote non-secret manifests to the GitOps repo;
  Fleet applied them. The Deployment reached Ready with the Harbor image and
  `harbor-pull` imagePullSecret; the Secret type was
  `kubernetes.io/dockerconfigjson`.
- UC-16 Delete → Preview → Deploy removed the GitOps bundle; Fleet removed the
  Deployment. The run-scoped namespace was then deleted. The GitOps repo was
  left with only its bootstrap README; Fleet and Harbor installations were
  retained.

This run does not validate private Harbor projects, AWS delivery, multi-node
kind or production TLS/credential rotation. The sample Harbor image and
read-only robot account remain as local test infrastructure.
