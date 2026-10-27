---
id: VERIFY-OFFLINE-TERRAFORM-PROVIDERS-20261027
artifact: verification-record
status: current
last_reviewed: 2026-10-27
---

# Offline Terraform provider image verification

Scope: provider installation for the embedded release on Linux amd64, without
staging access. Contract: [provider installation](../architecture/contracts/terraform-modules.md#provider-installation-trong-môi-trường-offline).
Procedure: [offline runbook](../operations/terraform-offline.md).

## Observed results

- Mirror preparation downloaded `hashicorp/aws 6.60.0` and
  `hashicorp/kubernetes 2.38.0` and verified archive SHA-256 against release
  `zh:` hashes. Runtime policy contains only a filesystem mirror.
- `docker build --target offline-provider-check -t
  orchestrator-offline-check:20261027 backend` passed.
- `docker run --rm --network none orchestrator-offline-check:20261027` passed
  **7/7 module subtests**, no skips, in 70.41 seconds: vpc, eks, aurora,
  elasticache, k8s-namespace, redis-k8s and postgres-k8s. The check uses the real
  registry to verify packaged artifacts, binary digest and per-module provider
  locks, then runs production runner `init` as UID/GID 65532. It uses no plugin
  cache and confirms the release lock is unchanged. No plan/apply or provider
  infrastructure API calls are performed.
- Final image `orchestrator-backend:offline-20261027` built successfully;
  image ID `sha256:2eb6ffbf32e7cf09e548e03b710fbb87ef68f826d785dce5f4d018509dc817f5`.
  Image CLI help confirms the default policy path is
  `/etc/orchestrator/terraform-offline.tfrc`.
- `cd backend && go test ./... && go build ./...` passed. The default suite
  did not configure a PostgreSQL test URL, so optional SQL integration tests
  retain their existing skip behavior. This is not a zero-skip full SQL suite.
- A focused rerun of bootstrap, registry, runner and mirror builder used a
  disposable local PostgreSQL 17 container with `REQUIRE_POSTGRES=1` and
  `REQUIRE_TERRAFORM=1`: **69 tests/subtests passed, zero skips**. The container
  was removed by its cleanup trap; no retained database or cluster was used.
- `kubectl patch --local` with `KUBECONFIG=/dev/null` verified the strategic
  merge patch preserves the existing container image and unrelated environment
  variables and adds only the explicit CLI policy selection. No API access.
- Documentation checker and `git diff --check` passed.

## Limits and handoff

No staging rollout, registry image publication, Kubernetes/cloud mutation or
application deployment was performed. Frontend/browser checks are outside this
backend/image change. This result proves offline provider installation, not the
original deployment's unavailable stderr or a successful `acceptance-1` deploy.

The original staging base manifest is absent from this checkout. The new
`deploy/k8s_4f/backend.yaml` is explicitly a patch for the existing Deployment;
the operator must reconcile its names with the staging-owned base and select
the new image through their normal delivery process. Keep the Definition's
`name` input. Internal Git sources and provider APIs still need their own
connectivity; cleanup of historical releases still needs their original
providers, binaries and verified modules.

Initial worktree was clean; no user-owned changes were modified. Local build
and test logs remain outside Git under `/tmp/poc-offline-*`.
