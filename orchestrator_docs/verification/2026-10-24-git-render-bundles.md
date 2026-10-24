---
id: VER-20261024-GIT-RENDER-BUNDLES
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-24
related: ADR-021, IMP-034, issue #14
---

# Git-owned manifest rendering — real end-user acceptance

## Outcome

Issue #14 implementation passed the required real-adapter Playwright gate
RB-E01..RB-E10 in run `r10a20261024` (47/47 assertions, zero skipped steps) on
commit `7874754` (implementation `920698f` plus the UC-07 VSO-01 cleanup fix).
Everything ran on a new run-owned kind cluster, Compose project, workload Vault
and fixture ref, all verified absent afterwards. No fake adapter, stub, route
interception or API/DB seeding of catalog, Definitions, config, workloads or
deployments was used.

Raw evidence (sanitized, outside Git):
`~/.local/share/manifest-bundle-evidence/run10/` (`assertions.txt`, `facts.json`,
per-step Preview projections, screenshots, `cleanup.txt`, `secret-scan.txt`).
Selected screenshots: [assets](assets/git-render-bundles-20261024/).

## Real adapters and pins

| Item | Value |
|---|---|
| Runner | `bash backend/test/integration/git-render-bundle-kind.sh --evidence <dir> --run-id r10a20261024` |
| Cluster / Compose | kind `manifest-bundle-r10a20261024` (kindest/node v1.36.1), project `manifest-bundle-r10a20261024`, VSO 1.5.1, Vault 1.21 |
| Renderer | score-k8s `0.15.0` in the backend image, binary SHA-256 `52b7fb666f702b21f9d5433380bd4d8ac91a3192dd5e1f3b9c6a141a861d99dd` |
| Backend image | `sha256:13f28642ce1a77bc39a3d87e658e27d22211a9f8c28f79094e47979a620bb17f` (same before A, after B and after restart) |
| Source | `https://github.com/pr3s3nt/poc.git`, directory `backend/render-bundles/standard`, ref `test/manifest-bundle/r10a20261024` |
| Bundle A | commit `bd7074a86e9a5ca9b6f70f862c76e1abcaa4ba83`, content `417f7ba550a8c3ca4b011abf09fea6b739d3727154d0931a2870f9b92055bb21` |
| Bundle B | commit `9a25b1afbde68a03372dc23f9db35896ccb96901`, content `8e520c85aac49ea652299940f2a32ae2a5050b3103fa6bbe9f1d1704293992e6` |

A = reference defaults with CPU fallback 20m and Pod annotation
`example.com/render-policy: a`. B = CPU 30m, annotation `b`, Pod
`securityContext.seccompProfile: RuntimeDefault`, a changed normalizer (Score
annotation `example.com/normalized-by: b`) and a changed patch
(`revisionHistoryLimit: 5`). Typed input `team: payments` on both.

## Assertions (RB-E01..RB-E10)

| ID | Result |
|---|---|
| E01 | Empty catalog; Internal account READY by kubeconfig upload; five Types, workload Secret Store, existing-cluster and verified namespace/PostgreSQL/Redis Terraform Definitions created in the Console |
| E02 | Git inspect pinned commit A + content digest; no cache path/credential in replies; typed inputs and JSON round-trip; saved, reopened without a new inspection |
| E03 | Developer created Application, chose the Secret Store, saved variables and one masked secret, imported frontend/backend/worker Scores (backend explicit CPU 50m, worker memory limit only, frontend none) |
| E04 | Preview: every workload pinned to A (`Git backend/render-bundles/standard @ bd7074a86e9a` shown), manifest shows annotation a, CPU 20m, team label; 60 unresolved paths stay unknown; no namespace created |
| E05 | Live Deployments: frontend 20m/32Mi, backend 50m kept, worker memory request 96Mi from its limit; DB password by Secret reference through the output-only provisioner; job 1 DONE via backend → PostgreSQL → worker → Redis |
| E06 | Same directory re-inspected at ref B and saved; backend container image and start time unchanged between A and B |
| E07 | No Score draft: three workload UPDATEs; diff A→B (annotation, CPU, securityContext); live Deployments follow B normalization/defaults/patches; explicit values, images and ports unchanged; job 2 DONE; namespace/StatefulSet/PVC UIDs, Terraform instance IDs and state lineages unchanged |
| E08 | Reload and restart of the owned backend only (same image, new start); unchanged intent Preview is a no-op; jobs persisted; pins A/B AVAILABLE from the durable cache with no inspect call |
| E09 | Verified A re-selected in the Console; three UPDATEs; policy A restored; job 3 DONE; infrastructure identities unchanged |
| E10 | Three workloads removed through the UI (VSO delivery objects removed with them); PostgreSQL, Redis and namespace deleted through the resource detail UI; namespace absence proven by a successful lookup; no browser errors; secret scan clean |

Unchanged identities after A: namespace `7e12fd4d-4d5f-4f3c-96ca-541a24af070e`,
StatefulSets `0ec06ac3…` (PostgreSQL) / `41644f8d…` (Redis), PVCs `8172e794…` /
`2d861650…`, Terraform instances `7d8c5f52…` / `56899d19…` / `f11f1563…` with
unchanged state lineages through E07 and E09.

Earlier attempts r1–r9 failed on harness setup (image build directory, browser
locale, locators, port-forward, API defaulting of `securityContext`, list race)
and in r8/r9 on the namespace cleanup: removed workloads left VSO objects and
the executor's foreign-object guard refused the namespace. The user chose to
extend removal (UC-07 VSO-01/02); r10 passed with that fix. Every attempt's
teardown reported cluster, Compose project/volumes, workload Vault, fixture ref
and images absent.

## UI comparison

The edited source section reuses the Resource Definition Git source pattern
(segmented “Thêm từ Git”/“Bundle đã xác minh”, URL/directory/ref fields,
verify action, success status, technical details) at 1512px and 1280px
([source 1512](assets/git-render-bundles-20261024/ui-1512-template-source.png),
[source 1280](assets/git-render-bundles-20261024/ui-1280-template-source.png),
[reference 1280](assets/git-render-bundles-20261024/ui-1280-reference-definition-git.png)).
The credential-free note and installed-bundle guidance moved into collapsed help;
full pins stay in “Chi tiết kỹ thuật”. Shell, navigation, placement, companion
and criteria controls are unchanged.

## Checks

| Check | Result |
|---|---|
| `go build ./...` | Pass |
| `REQUIRE_POSTGRES=1 go test -timeout 45m ./...` (disposable PostgreSQL 16) | 63 packages pass, zero PostgreSQL skips. One load flake outside this change: `test/integration/v04run` `TestCleanupStopsExactOwnedAuxiliaryAndVerifiesServiceAbsence` (owned node child exited 1 instead of SIGTERM); the package passes alone with `-count=5`. `internal/application/tfexec` needs more than the default 10 min package timeout on this host |
| `npm run typecheck`, `npx eslint src`, `npm run build` | Pass |
| `npx vitest run --maxWorkers=8` | 69 files / 866 tests pass (default parallelism times out unrelated heavy tests at 5 s on this host) |
| `npm run lint` | Only the 46 preexisting errors in `design/resource-definition-*` and `test/e2e/ec2-role-real.mjs` (untouched) |
| `bash -n` changed scripts, `REQUIRE_PLANTUML=1 python3 scripts/check_docs.py`, `git diff --check` | Pass |
