---
id: VERIFY-2026-11-08-DB-CONNECTION-OUTPUTS
artifact: verification
status: current
last_reviewed: 2026-11-08
related: [UC-02, UC-16]
---

# DB connection-string outputs — issue #52

## Scope

Branch `features/db-connection-outputs`, commit `2a0b08f3`. Contract:
[module outputs](../architecture/contracts/terraform-modules.md#chuỗi-kết-nối--issue-52),
[UC-02 EXT](../usecase/UC-02/specification.md#bổ-sung-output-tuỳ-chọn--issue-52).
Scenario: [db-connection-outputs](../../e2e/scenarios/db-connection-outputs/README.md).

## Automated checks

- Terraform 1.9.8 (release-pinned binary digest): `fmt`, `validate` for every
  changed module; `terraform test` for mysql/mariadb/mongodb, including a
  password with `@ : / %` percent-encoded in `uri`/`connection`.
- Go with real PostgreSQL 16 and Terraform (`REQUIRE_POSTGRES=1`,
  `REQUIRE_TERRAFORM=1`): domain/resource, planning, adapters/terraform,
  adapters/store, delivery/http, seed and the new HTTP/PostgreSQL concurrency
  tests pass. catalog, configuration, deployment, pending, preview,
  provisioning, tfexec, adapters/postgres and test/e2e show the same failing
  tests as `origin/main` (pre-existing, e.g. "render bundle is unavailable");
  three extra failures seen under concurrent load passed when rerun alone.
- Frontend typecheck and platform tests pass; lint keeps the existing
  PlatformSettingsPage warning. Bootstrap `node --test` 82/82.

## Console trial stack

Feature Docker stack opened through its Tailscale URL in Chromium: a Type with
the old PostgreSQL contract gained `uri` (secret) and `jdbcUrl` through
"Thêm output"; a duplicate name kept the draft with Vietnamese guidance. The
Resource Types page layout and controls match the existing registration form.

## Full test on kind-k8s-4f

A Console built from the commit was installed in its own namespace and seeded
through the API (Git, cluster account, workload Vault with Kubernetes auth,
Fleet CD). As Platform Engineer, `mariadb` (registered without connection
strings) gained `uri`/`jdbcUrl` in the Console; five Git Definitions pinned to
the commit were accepted. Five workloads, each with one new database resource,
were authored and saved through the form; Preview listed all five; one Deploy,
Operation `bce96a34` SUCCEEDED on the first attempt. Read-only checks:

| Workload | Connection | JDBC URL | `DATABASE_URL` in Deployment |
|---|---|---|---|
| pg-check (postgres `uri`) | CONNECTION_OK | JDBC_OK | secretKeyRef |
| redis-check (redis `uri`) | CONNECTION_OK | — | secretKeyRef |
| mysql-check (mysql `uri`) | CONNECTION_OK | JDBC_OK | secretKeyRef |
| mariadb-check (mariadb `uri`) | CONNECTION_OK | JDBC_OK | secretKeyRef |
| mongo-check (mongodb `connection`) | CONNECTION_OK | — | secretKeyRef |

No database password appeared in saved workloads, Preview, Operation or
Deployment manifests (compared in memory).

## Findings outside this scope

1. Container args are not escaped like environment values: shell `${...}` in an
   argument becomes a resource placeholder and Preview fails with a bare
   `PLANNING_REJECTED` (stage `graph`, reason unclassified). The scenario avoids
   `${...}`.
2. The first Application deletion ended `CLEANUP_INCOMPLETE`: the Fleet access
   Role/RoleBinding granted by the test kept the namespace. After removing them,
   "Thử lại" completed the deletion; the scenario now removes them first.

## Cleanup

Application deleted through the Console (namespace and PVs absent, no GitRepo
left); the Console namespace, its ClusterRoleBinding, port-forward and the
imported node images were removed. The delivery repository
`idp-acme-dbconn-issue52-deploy-*` remains on the platform Git account (ADR-033
retention). Private inputs and evidence stay outside Git.
