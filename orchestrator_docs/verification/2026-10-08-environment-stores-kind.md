---
id: VER-2026-10-08-ENVIRONMENT-STORES-KIND
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-08
---

# Environment stores and transitions — real kind verification

The first [ADR-012](../architecture/decisions/ADR-012-environment-stores-and-transitions.md)
delivery passed the complete human Playwright flow on existing `kind-idp-internal`.
Claude cloud authored code; Codex reviewed successive commits, ran independent
gates and executed the local real-cluster recorder. Cloud execution was not used
as a substitute for access to this cluster. Coordination/status checks ran at
approximately four-minute intervals.

## Recording and provenance

- Run: `envstores-20261008045702-29522`, code `9105e608475e4cc4802574b6a48f429849806837`.
- [Reviewed MP4](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/environment-stores-kind-envstores-20261008045702-29522.mp4): H.264, 1440×900, 667.333333 seconds, 6,073,769 bytes, 33 phase marks.
- SHA-256: `425b238288a4c199a97bb642b2930ff414cccf8ccfc005376386a412755277aa`.
- Runner exit status `0`; full decode, nonblank settled frames and final `cleanup-done` mark passed. Codex visually reviewed masked-token registration, stale-preview rejection, restored job over HTTP and completed cleanup.
- Safe logs, marks, frames, API/persisted projections and cleanup proof stay outside Git under `/tmp/poc-environment-stores-review/live/envstores-20261008045702-29522/`. Raw private credentials and runtime state were deleted, not uploaded.

## Observed behavior

Platform Engineer registered Alpha and Beta Vault KV v2 stores with scoped tokens.
They use separate `kubernetes` and `k8s-b` auth mounts; a third run-owned Vault
stores platform credentials. Tokens were masked during input, cleared after
submission, absent from response/page output and checked against safe artifacts.

Developer created an unconfigured Application and chose deployment Connection and
Secret Store separately in each Environment's Settings. Staging deployed two
workloads through the uploaded source Connection while the backend's host-context
credential was deliberately invalid. The acceptance app submitted one job;
source PostgreSQL held its exact payload and count `1`.

Switching Staging to Beta copied/verified its secret and rolled out VSO through
Preview/Deploy. Both store-specific VaultConnections and auth mounts remained
distinct; the running application still passed environment/secret/database checks.
A second tab edited configuration while transition Preview was open. Starting
the old Preview returned `409` before any destination namespace existed.

Previewing again and confirming downtime executed `MIGRATE_POSTGRES` into target
generation `1`. Destination PostgreSQL retained count `1` and the exact job payload;
the destination served the restored job and checks through Traefik. Source workload
replicas were `0`, source data remained present and Ingress belonged only to the
destination. Refresh and backend restart retained the committed Connection/store
and generation. Source deletion occurred only after the explicit UI cleanup action.

Final API/state observers found one `SUCCEEDED` transition with destination
authority, source `CLEANED`, no operation holding the Environment, generation-0
instances `REMOVED` and generation-1 instances `READY` on the selected destination.
Artifact scans passed for 12 files against six sensitive values; no values were
printed or uploaded.

## Review and validation

Review fixed recovery polling, Settings idle-to-busy refresh, conflicting config
and draft submissions, early cleanup identity capture, owned backend/browser
process groups and bounded calls. Existing acceptance helpers select a store
explicitly; current Connection runners follow editable Settings rather than the
superseded permanent lock.

Earlier attempts exposed missing Definition profile, a password-input HTML scan
false positive, JSON-key-order comparison and transition-run cleanup identity.
They were corrected before this successful run; failed recordings were not uploaded.

Codex independently passed `go test -race ./...` with isolated Docker PostgreSQL
16, `go vet ./...`, `go build ./...`, frontend typecheck/lint/153 tests/build,
changed shell/mjs syntax, documentation checks and whitespace checks. The final
post-recording runner change labels the global reviewer ClusterRoleBinding and
checks label/role/subject before deletion; its scope is setup/cleanup, not the
recorded product behavior. Its independent real-kind setup/cleanup probe at
`d65be51`, run `envstores-20261008051320-7474`, exited `0`: scoped store verification
was READY and the labelled reviewer binding, namespace, all three Vaults and
private files were cleaned. This probe is separate from the human-flow evidence.

## Cleanup and limits

The wrapper and independent Kubernetes/Docker queries found both application
namespaces absent, reviewer resources absent, all three run-owned Vaults absent
and private credential files removed. Owned backend/browser groups stopped;
current context remained `kind-idp-internal`. Existing operator workloads and
services were preserved. Loaded acceptance images retain the existing local
lifecycle limit.

Both logical Connections use the same physical cluster and credential. This
proves generation isolation and controlled same-cluster Ingress cutover, not
distinct-cluster DNS automation or AWS/Aurora migration. Only Staging was deployed;
Production selection was verified without deployment. The restored job remains
`PENDING`; worker processing is outside this flow. Recovery/fencing failure paths
were tested locally rather than through this happy-path video. Other changed
legacy recorders were syntax-checked, not all replayed live. The pre-existing
AWS integration-tag test has an `EKSDescriptor` argument mismatch; no AWS run is
authorized or claimed.
