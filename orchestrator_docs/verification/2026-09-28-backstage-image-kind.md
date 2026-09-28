---
id: VERIFICATION-BACKSTAGE-IMAGE-KIND-20260928
artifact: verification-record
status: current
last_reviewed: 2026-09-28
---

# Backstage image smoke test on kind — 2026-09-28

- Cluster/context: `kind-idp-internal`; node `idp-internal-control-plane`,
  linux/amd64, Ready.
- Image: `ghcr.io/backstage/backstage:1.53.1`, pulled from the Backstage GitHub
  package with digest `sha256:bfb38d3452712c4e5b9e6630667932816425b3ac8de37b4b0daf86a09b4740f0`.
  The node's amd64 manifest is
  `sha256:ae6ed7409d1a5ff351d7ea6f548bd1de542ee831a9639db67a38f6399b64afb6`.
- `kind load docker-image` failed on a missing arm64 content digest in the
  multi-platform archive. An amd64-only Docker archive was imported into the
  node's `k8s.io` containerd namespace; `ctr images list` confirmed the tag.
- A Pod in namespace `backstage-image-20260928` ran the image with
  `imagePullPolicy=Never` and `node --version`; it succeeded and printed
  `v24.18.0`. The test namespace was deleted afterward.
- This checks image availability and execution only. It does not prove that
  Backstage starts with PostgreSQL, serves its UI/API, or deploys through
  Orchestrator. Those remain to be verified separately.
- Temporary GHCR Docker login was removed after the pull; no registry
  credential or image layer was committed to this repository.
