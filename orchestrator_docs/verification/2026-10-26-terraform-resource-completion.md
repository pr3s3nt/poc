---
id: VER-20261026-TERRAFORM-RESOURCE-COMPLETION
artifact: verification-record
status: evidence
last_reviewed: 2026-10-26
---

# Terraform resource completion — real happy-path acceptance

User authorized real local/kind acceptance on 2026-10-26, explicitly excluding
fake/mock services and deliberate failing live scenarios. Branch
`refactor_production` was fast-forwarded to main `72d086d` before implementation.
Canonical behavior: [UC-08 BR-10](../usecase/UC-08/specification.md),
[module contract](../architecture/contracts/terraform-modules.md#resource-completion-and-supplemental-readiness).

## Successful run

- Run: `tc1026r4`; source and Git module pin:
  `04110845e4ec8180a669b7cf85d50be2716c907a`.
- Result: **57/57 assertions PASS**, including **21 TC completion assertions**;
  browser scenario duration 493 seconds, no browser errors.
- Evidence directory (outside Git):
  `/home/thanhnt1/.codex-runs/terraform-completion-20261026-r4/`.
  `assertions.json`, `facts.json`, `provider-schema-summary.json`, screenshots,
  `secret-scan.txt`, `run-manifest.json` and `cleanup.txt` record the observations.
- Real components: headed Playwright Chromium on Xvfb; current Go API/React
  Console; Terraform 1.9.8 with pinned Kubernetes provider 2.38.0; PostgreSQL
  system/state stores; Vault and VSO; score-k8s 0.15.0; new kind cluster with
  Envoy Gateway/HTTPRoute. No intercepted responses, fake adapters, stub CLIs
  or mock services establish this result. No AWS calls/deployment.
- Module was fetched from `https://github.com/pr3s3nt/poc.git`, directory
  `backend/test/acceptance/terraform-completion`, through ordinary Console Git
  inspection. Rendering bundle came from
  `https://github.com/pr3s3nt/workload-manifests.git`, `bundles/production`, commit
  `9d5b31be06f3b0b7d4051296e98f4176514ee5fc`.

| Successful user flow | Observed result |
|---|---|
| PE onboarding and authoring | Account, Type/Definitions, Vault store and Git module registered through Console controls |
| Preview | Three workloads plus HPA/PDB/Job/CronJob/PVC shown; namespace remained absent before Deploy |
| Provider completion | Shared and private modules each created native ConfigMap/ServiceAccount plus a ConfigMap through `kubernetes_manifest`; both instances READY with acknowledged state |
| Output consumption | Backend/worker share one resource and use its ConfigMap output for the application's environment check; frontend has a different private instance |
| Mixed module | Real PostgreSQL module reached READY with StatefulSet probe and provider-completed Service/Secret/PVC; Redis also passed readiness |
| Application | All five browser checks PASS; HTTPRoute host served the app; submitted job reached DONE through backend, PostgreSQL, worker and Redis |
| Valid update/reuse | Private frontend resource revision became v2; shared resource stayed v1. ConfigMap/ServiceAccount/manifest UIDs, all Terraform instance/state identities and PostgreSQL/Redis StatefulSets stayed stable |
| Application after update | Domain and all checks PASS; second job DONE, first job still visible after update |
| Product cleanup | Workloads and HTTPRoute removed through Console; all five managed Terraform instances DESTROYED and namespace absent before runner teardown |

Read-only API/Kubernetes/SQL observations supplement the browser mutations.
Screenshots `E05-domain.png` and `E06-app.png` were visually inspected: the
application shows every deployment check PASS and both jobs DONE after update.

## Validation and earlier attempts

- `go test ./...`: 1,614 tests passed, no failures. 271 optional integration
  tests skipped: 269 without a PostgreSQL test URL and two without their
  explicit kind opt-in. The real E2E above independently exercised PostgreSQL,
  encrypted state, actual provider execution and kind for the changed flow.
- `go build ./...` passed. Readiness unit regressions cover provider completion,
  schemas without `id`, mixed modules and preserved supplemental-query failures.
- Frontend typecheck/lint/build passed; full Vitest rerun with two workers:
  **918/918** across 76 files. One unrelated effectiveness-page async assertion
  failed in the initial high-concurrency run; its focused rerun and the complete
  two-worker run passed without changing product code. After browser-helper
  edits, the 144 helper checks and lint passed again.
- `terraform validate` passed with the release-pinned provider; actual
  `terraform providers schema -json` confirmed the manifest schema has no `id`.
  The [provider schema](https://github.com/hashicorp/terraform-provider-kubernetes/blob/v2.38.0/manifest/provider/provider.go)
  corroborates this. Terraform formatting, Node/Bash syntax, documentation
  checker (including PlantUML) and diff whitespace checks passed.
- Runs `tc1026r1`/`tc1026r2` stopped at module selection. A wrapping-label selector
  incorrectly matched text inside a Type's options (`app-config` awaiting its
  first module). Waiting alone did not fix it; matching the label prefix did.
- Run `tc1026r3` exposed an invalid acceptance-module argument,
  `wait_for_service_account_token`. It was removed after checking the
  [pinned ServiceAccount schema](https://github.com/hashicorp/terraform-provider-kubernetes/blob/v2.38.0/kubernetes/resource_kubernetes_service_account_v1.go).
  The module was validated before the successful rerun. The browser success
  wait was stopped after the product had reported failure; the runner still
  performed cleanup. Failure evidence remains separate from the passing run.
- These were actual harness/fixture failures, not planned negative scenarios.
  No business assertion was removed or reduced; all failed runs verified their
  owned containers, volumes, Vault, kind cluster and run image tags absent.

## Cleanup and limits

`tc1026r4` ended with scenario exit 0 and `final_status=0`. The product proved
namespace absence. Runner cleanup independently proved its Compose containers/
volumes, workload Vault, kind cluster and run image tags absent. Secret scanning
found no run secret, Vault token or kubeconfig material in public evidence.
Only preexisting `idp-internal` and `k8s-4f` kind clusters remained; retained
installations and user tmux sessions were not changed. Successful-run private
credentials were removed by the runner.

Coverage demonstrates generic completion with three real Kubernetes resource
schemas and the existing PostgreSQL/Redis modules. S3/SQS/IAM completion uses the
same generic rule and unit coverage, but no paid AWS execution is claimed.
Approved provider/version/module contracts, unknown-outcome holds, EKS usability,
namespace provenance, destructive-update rejection and cleanup authorization
remain in force. Provider completion does not claim a separate application-level
health check for every possible resource type.
