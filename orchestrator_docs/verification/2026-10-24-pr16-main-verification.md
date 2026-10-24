---
id: VER-20261024-PR16-MAIN
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-24
related: ADR-021, IMP-035, PR #16
---

# PR #16 — main integration and independent real acceptance

## Outcome

[PR #16](https://github.com/pr3s3nt/poc/pull/16) merged to main at
`e1a235b2d90c7651876ea5b88d7110cf06de4052`, after conflict resolution commit
`91839815373f894c90bc0bfcb893bfc62039bb81`. Independent end-user acceptance on
that product tree plus runner fix `99c1726` passed **47/47 assertions**, zero
skipped steps, with real adapters in run `c16a20261024`. No product-flow failure
was found in the new run. The runner's cleanup exit-code defect was reproduced,
fixed and checked before the live run.

Evidence outside Git: `~/.local/share/manifest-bundle-evidence/pr16-c16a20261024/`
(`facts.json`, `assertions.txt`, screenshots, cleanup and secret-scan reports).
Private credentials were removed after successful cleanup. Existing dated
[implementation evidence](2026-10-24-git-render-bundles.md) is preserved.

## Conflict resolution and regression

- Planning snapshot retains both `BaseProofs` from PR #15 and scoped
  `Catalog.RenderBundles` from PR #16. Neither feature was dropped.
- The existing Git A→B→A pending test now starts with a UC-16-accepted ordinary
  `LOG_LEVEL` value and proves it remains readable after a rendering-only update.
- VI/EN catalogs were merged by JSON key with three-way value comparison; both
  environment-editor and render-bundle key sets survive.
- Additive docs, traceability and verification entries from both branches remain.
  Main already owned IMP-034 for the environment editor, so Git rendering is
  consistently referenced as IMP-035; historical execution records are unchanged.

## Runner defect and fix

The cleanup group was piped to `tee`, so assignments to its failure status happened
in a subshell. Reproduction showed `final_status=1` but process exit 0 when owned
containers/volumes remained. Failed Git lookup was also treated as absence, and
image deletion was reported without checking the result.

Fix `99c1726` keeps status in the current shell, checks Docker availability,
requires a successful Git listing to establish ref absence, and verifies image
removal. Six shell-unit fault-injection cases check success and failed container,
volume, Git, image and Docker cleanup. These unit doubles do not participate in
live acceptance; the actual runner uses real Docker/Git/Kubernetes/Terraform/CLI.

## Live flow and facts

```bash
GOMAXPROCS=4 bash backend/test/integration/git-render-bundle-kind.sh \
  --evidence /home/thanhnt1/.local/share/manifest-bundle-evidence/pr16-c16a20261024 \
  --run-id c16a20261024
```

| Item | Observed result |
|---|---|
| Platform | Fresh run-owned kind `manifest-bundle-c16a20261024`, Compose project of the same name, real PostgreSQL/Vault/VSO, Terraform and score-k8s 0.15.0 |
| Browser | Headed Chromium on Xvfb; catalog, Git inspection, inputs, app/config/Score, Preview/Deploy and resource deletion through UI; no fake adapter, stub response or feature API/DB seeding |
| Source | `https://github.com/pr3s3nt/poc.git`, `backend/render-bundles/standard` |
| A | `11da654583c319f6cb4d2ecc79e10630b84f2bc1`; content `417f7ba550a8c3ca4b011abf09fea6b739d3727154d0931a2870f9b92055bb21` |
| B | `956a805a5ac6b1826b2208a681a1b9036b85fe22`; content `8e520c85aac49ea652299940f2a32ae2a5050b3103fa6bbe9f1d1704293992e6` |
| Backend image | `sha256:5f4d6aa33fc526ebba48803bef823cc3b2a15ae8342ac49dfc7388e3b3a177ed`; same image/start between A and B |
| Policy | CPU fallback 20m→30m, normalization annotation, Pod securityContext and patch revisionHistoryLimit follow B; explicit requests/limits/images/ports remain |
| Business | Jobs 1, 2, 3 DONE through frontend/backend/PostgreSQL/worker/Redis after A, B and A again |
| Restart | Only owned backend restarted; same image, new start; jobs/pins/applied baseline persisted, no-change Preview a no-op, cached A/B available without inspect |
| Infrastructure | Namespace UID `49c5208b-db41-43a2-a63b-25a6f896ef89`, StatefulSet/PVC UIDs, Terraform instance IDs and state lineages unchanged through A→B→restart→A |
| Safety | No browser errors; public evidence scan found no run secret, Vault token or kubeconfig material |
| Product cleanup | Workloads removed, PostgreSQL/Redis/namespace destroyed through UI; namespace absence confirmed by successful API lookup |
| Teardown | Compose containers/volumes, workload Vault, kind cluster, fixture ref and run images verified absent; `final_status=0` |

Independent post-run Docker and Git reads confirmed no Compose containers/volumes
or fixture ref remained. Separate test PostgreSQL and preflight image tags were
removed too. Retained orchestrator-manifest, preview-kind, orchestrator-poc and
other user stacks were neither restarted nor reset; original checkout stayed
untouched, with review/test work in `/tmp/poc-pr16-review`.

## Checks and UI comparison

| Check | Result |
|---|---|
| `REQUIRE_POSTGRES=1 REQUIRE_TERRAFORM=1 GOMAXPROCS=4 go test -p 4 -timeout 30m ./...` | Pass with disposable PostgreSQL; existing SQL role/credential boundaries enforced |
| `go build ./...`, `go vet ./...` | Pass |
| Git/literal integration regression | Pass with actual installed score-k8s |
| `npm run typecheck`, `npm run lint`, `npm test -- --maxWorkers=4`, `npm run build` | Pass; 73 files / 893 tests, including 6 cleanup cases |
| `bash -n backend/test/integration/git-render-bundle-kind.sh` | Pass |
| Documentation/PlantUML and diff checks | Pass; repeated after this evidence/index update |
| [Merged-main Product CI](https://github.com/pr3s3nt/poc/actions/runs/40111242934) | Pass (backend full tests/build/vet/race and frontend) |
| [Merged-main Documentation CI](https://github.com/pr3s3nt/poc/actions/runs/40111242915) | Pass |

The initial local test server used trust authentication, which correctly failed
its wrong-password assertion. Its owned pg_hba was corrected to require SCRAM
for generated test roles; the package and the full suite then passed. The host's
Go 1.27 tfexec package took 904.7s but passed. An extra Go 1.25 diagnostic probe was
stopped after that concern resolved; no required gate was omitted.

Actual source and reference screenshots at 1512px/1280px were viewed: Rendering
Templates inherits the existing Resource Definitions Git source controls, shell,
spacing and colors, with no clipped controls. Full pins and Git-auth explanations
stay collapsed; effective input/default hints stay near their controls. Schema
labels/descriptions come from the author bundle, not the Console message catalog.
No UI redesign was introduced. No recording or cloud/shared-cluster run was
required; live coverage is the isolated Kubernetes scope defined by ADR-021.
