---
id: VERIFY-20261118-KUBERNETES-INSTALLATIONS
artifact: verification-evidence
status: current
last_reviewed: 2026-11-18
---

# Independent Kubernetes installation — issue #91

Branch `features/kubernetes-manifests`, based on main `1463d8cb`, includes the
unmerged #89 first-run implementation. Design `da7666b1`, initial package
`e530f350`; subsequent operator/E2E refinements do not change backend/frontend
product trees. Neither feature is merged. Staging/production were not accessed.

## Actual kind installation

A separately owned namespace on kind-k8s-4f uses three retained PVCs, PostgreSQL
and Vault with verified TLS, independent generated credentials and manual operator
unseal. Existing Traefik Gateway/controller, main Console and shared RBAC were
preserved. A task hostname's HTTPRoute serves the entire Console origin through
frontend. Exact Tailscale hostname/port was opened in Playwright with real API data;
private handoff contains its URL, restart commands, identities and file locations.
No IP, credential, kubeconfig or recovery material is included in this public record.

Observed using `e2e/scenarios/kubernetes-installation/`:

- Fresh operator-token Organization/Admin creation and actual user-source catalog
  import: 10 Resource Types, 8 Definitions, pinned source
  `8aed1e05170a003719919baf63917a9a15b88feb`. Rerun reuses all 18 documents.
- Real kind credential uploaded through the Console, verified READY and stored
  by backend in scoped Vault KV over TLS. No root token or Shamir share in any
  Kubernetes Secret; recovery response remains operator-held outside the cluster.
- Image-reference update from initial to r2 tags, with quiescence, validated
  PostgreSQL/backend/Vault backups, explicit unseal and owner setup before runtime.
  Tags contain the same reviewed product bytes; this is an image-rollout rehearsal,
  not evidence of a new schema-version migration or a different product release.
- Unchanged install, Vault Pod recreation/operator unseal and backend restart retain
  installation/Organization/Admin/catalog/source/account IDs, three PVC UIDs,
  Secret UIDs/data and a backend recovery marker. Browser/API repeat succeeds.

- The final streamed backup was restored in separate temporary emptyDir Pods:
  PostgreSQL matches the complete allowlisted pre-backup domain snapshot, backend
  archive retains the marker, and Vault unseals with external shares and retains
  the exact scoped credential hash. All restore and backup helper Pods were removed
  with UID/ownership checks; full-suite/image-loader temporary namespaces are absent.
  Retained installation, all three PVCs and access forwards stay running for review.

Bringup failures were resolved before the successful final rehearsal: restore/upgrade
could open port-forward before Vault's TLS listener and lose the tunnel on its first
connection; both paths now wait for the listener. The first credential-material
assertion expected CLI-shaped init JSON; it now handles the raw API keys as well.
Restore now compares the full saved domain snapshot rather than a fixed connection
count. Token-policy inspection uses transient operator authorization because the
canonical scoped policy intentionally omits lookup-self; credential reads still use
only the scoped token. No scope/policy was broadened to satisfy the test.
A completed backup followed by early tunnel failure left writers stopped as intended;
matching release/namespace/backup/database identity and a fresh owner Job were
verified before explicitly resuming the same runtime for the corrected rehearsal.

## Focused checks and full-suite limits

Installer tests cover all three overlays, least-consumer mounts, ownership/UID,
Secret byte reuse, private directory constraints, HTTPS/digests, real private
preparation idempotence/target isolation and mismatching TLS keys: 6 passed.
Node syntax, changed shell syntax, documentation checker and diff whitespace pass.
PlantUML is unavailable locally; checker reports that syntax check skipped, and no
PlantUML file changed.

Full product suites ran in disposable kind Pods at `e530f350` against main
`1463d8cb`; namespace cleanup verified. Product source trees are unchanged by the
later operator/E2E changes.

| Check | Candidate | Main |
|---|---:|---:|
| Go passing top-level tests | 1610 | 1603 |
| Go failing top-level tests | 104 | 103 |
| Go skipped top-level tests | 8 | 8 |
| Go build | Pass | Pass |
| Frontend test/typecheck/lint/build | 117 files, 1104 tests pass; all checks pass | Not repeated in this run |

The 103 shared failures and eight identical skips have the exact inventory and
limitations recorded in [first-run verification](2026-11-18-first-run-setup.md#exact-baseline-failure-inventory).
All eight added setup tests pass. One extra observed failure was
`orchestrator/internal/adapters/gitpublication/TestCancelledGitReadDoesNotStartAnotherProcess`:
its fixture counter was empty at cancellation. The entire package tree is identical
on candidate/main (`56fd8a5f0694bd07e04d9ddc8e3b29d137d8d9d3`); a focused local
50-repeat rerun passed. This supports an intermittent unchanged test, not a clean
full backend result. The failed full-run result remains reported; no assertion was
removed or failure relabelled as passing. No workload deployment or production
security/HA acceptance is inferred from Console installation.
