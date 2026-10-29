---
id: VERIFY-20261027-APPLICATION-DELETION-KIND
artifact: verification-record
status: evidence
last_reviewed: 2026-10-27
related: [UC-18, ADR-025]
---

# Application deletion — real kind acceptance

Run `xoaappk1`, procedure [application deletion acceptance](../operations/application-deletion-acceptance.md#kind).
Result: **PASS, 46/46 assertions, 0 failures**, teardown verified.

## Source

Working tree of `features/xoa_app` on top of `4764b1139dd7ca09b196628836a685900856a2c8`
before the first feature commit (backend, frontend and harness changes of
UC-18 uncommitted at run time). After the run only the Console read-only lock
while `DELETING` (Settings/Application home buttons disabled), documentation
and the AWS harness changed. Bundle `bundles/production` of
`https://github.com/pr3s3nt/workload-manifests.git` at
`9d5b31be06f3b0b7d4051296e98f4176514ee5fc`. Tools: Docker 29.5.2, kind 0.32.0,
kubectl 1.36.1, Helm 3.21.0, Node 24.16.0, Go 1.27.1.

## Real components

New run-owned kind cluster with Envoy Gateway and VSO, run-owned workload
Vault, Compose PostgreSQL/Vault/backend/frontend built from the tree, real
Terraform Kubernetes modules, score-k8s 0.15.0, the acceptance application and
headed Chromium. No fake adapter or intercepted response.

## Observed

WM-E01..E06 passed unchanged: catalog, Git bundle, three workloads with
PostgreSQL/Redis/VSO secret/HTTPRoute, Preview purity, deploy, Gateway domain,
business job, update and prune. WM-E08 (Developer, about 93 s from confirmation to SUCCEEDED):

- dialog: staging 3 workloads, 3 owned resources, 1 secret; production empty;
  case-folded name kept the confirm disabled;
- `PUT .../workloads/backend` while deleting answered `409 APPLICATION_DELETING`;
- deletion `SUCCEEDED`, Application `DELETED`; staging removed backend,
  frontend, worker; one cleanup operation destroyed
  `postgres.in-cluster#shared.acceptance-db`, `redis.in-cluster#shared.acceptance-cache`
  and the namespace, each verified by the engine; 1 secret value purged;
- the list showed "Đã xóa ứng dụng …", `GET` answered 404;
- namespace absent (successful lookup), no HTTPRoute in the cluster, 0 owned
  instances not DESTROYED, tombstone and Deployment history rows present;
- the Application's Vault value path held 3 objects before and none after;
- the kind cluster kept serving; a new Application with the same name and
  subdomain was created.

No browser console error. Secret scan of public evidence passed.

## Teardown

Compose containers and volumes, workload Vault, kind cluster and run image tags
verified absent (`final_status=0`). Evidence outside Git:
`/home/thanhnt1/e2e-evidence/xoaapp-kind-r1`.

## Limits

Kind only; AWS is a separate record. Output-secret, delivery policy/role and
in-process run limits are listed in [IMP-038](../implementation/deviations.md#imp-038--application-deletion-2026-10-27).
