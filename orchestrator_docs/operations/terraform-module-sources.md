---
id: OPS-TERRAFORM-MODULE-SOURCES
artifact: operational-runbook
status: current
last_reviewed: 2026-11-09
---

# Terraform modules from GitHub

## Nguồn phát hành cho Console — issue #54

Console trên `kind-k8s-4f` lấy module Terraform và workload template từ một repo
Git phát hành: `https://github.com/pr3s3nt/orchestrator-workload-templates.git`
(private; Console đọc bằng Git access của `github.com/pr3s3nt`). Release hiện tại
là tag `v0.4.1`, commit `f9bf7251fde0b4ae9224f960b476219765f1e2ea`. Không dùng
`v0.4.0` (MySQL contract cũ).

| Thư mục | Dùng cho |
|---|---|
| `templates/standard` | Template source `standard-workload` của Definition workload |
| `terraform/namespace` | `k8s-namespace` |
| `terraform/postgres` | `postgres`, có output `uri` (secret) và `jdbcUrl` |
| `terraform/redis` | `redis`, có `uri` (secret) và `connection_string` |
| `terraform/persistent-storage` | `persistent-storage` |
| `terraform/mysql` | `mysql`, password do executor quản lý, có `uri`/`jdbcUrl` |

Bản trong repo `poc` (`backend/template-sources/standard`,
`backend/internal/adapters/terraform/modules`, `projects/platform-resources`) vẫn
là nguồn phát triển và test; `SOURCE.md` của repo phát hành ghi commit và đường
dẫn gốc của từng file. Khi phát hành bản mới:

1. Đồng bộ file từ `poc`, chạy `terraform fmt -check`, `validate` (và `terraform
   test` nếu module có test) với Terraform 1.9.8, cập nhật README/SOURCE.md.
2. Gắn tag mới; không di chuyển tag cũ.
3. Trên Console: kiểm tra module/template từ tag mới, sửa Definition để tạo
   revision mới rồi promote. Environment đã đóng băng revision cũ giữ nguyên tới
   khi được chọn nâng cấp. Nếu module có output mới, thêm output vào Resource Type
   và ánh xạ trong Definition trước khi workload dùng.

The seven reference modules are published at
[pr3s3nt/terraform-modules](https://github.com/pr3s3nt/terraform-modules).
Release `v1.0.0` resolves to commit
`6b991b16c39ac8485c74fa6756cf3223a129c8c9`; module contents match the
product's packaged release `v4-2026.10.15` unchanged. Packaged release
`v5-2026.11.08` (issue #52) adds connection-string outputs (`uri`, `jdbcUrl`)
to postgres-k8s, redis-k8s, aurora and elasticache; that external repository
has not been republished, so its `v1.0.0` modules lack these outputs.

## Register a Definition

1. Open Resource Definitions → Tạo cấu hình and select the Resource Type.
   Git is the initial source per [UC-03 GIT-03](../usecase/UC-03/specification.md).
2. Enter Git HTTPS URL `https://github.com/pr3s3nt/terraform-modules.git`.
3. Choose a directory from the table below and enter tag `v1.0.0` or the full
   commit above. Click Kiểm tra module; the server pins commit and content digest.
4. Configure typed variables, mappings and provider bindings, then save.
   AWS modules require a READY Driver Account and explicit region. Kubernetes
   consumers bind to their matched cluster resource. Apply presets/overrides
   explicitly; Git inspection does not create resources.

| Directory | Resource Type |
|---|---|
| `modules/vpc` | `vpc` |
| `modules/eks` | `k8s-cluster` |
| `modules/aurora` | `postgres` |
| `modules/elasticache` | `redis` |
| `modules/k8s-namespace` | `k8s-namespace` |
| `modules/redis-k8s` | `redis` |
| `modules/postgres-k8s` | `postgres` |

Editing an existing Definition preserves its stored pin. Packaged snapshots
remain available for original plans/state/cleanup. A new Git-sourced Definition
does not migrate an existing deployed resource. Release-equivalent identity
and executor outputs follow the [module contract](../architecture/contracts/terraform-modules.md).

## Verify the public source without deployment

From `frontend/`, run `npm run build`. From `backend/`, run:

```bash
PUBLISHED_MODULE_COMMIT=6b991b16c39ac8485c74fa6756cf3223a129c8c9 \
  go run ./test/integration/gitmodulebrowser \
  -scenario ../frontend/test/e2e/published-modules-local.mjs
```

This uses an isolated in-memory catalog and browser session, reads the public
GitHub source, inspects all seven modules and checks the exact release digests,
Git pins and EKS executor-output mapping. No provider apply or cloud/database
mutation runs. General Git create/edit/CAS/reset coverage uses the same harness
with its default scenario and temporary HTTPS Git fixture.

## Publish later versions

Use a new tag for changed module contents; never move `v1.0.0`. Confirm the
platform provider release and module/executor contract before authoring a new
Definition. A changed module does not inherit the release-equivalent module
identity solely because its path/name matches; arbitrary custom EKS/namespace
lifecycle extensions require their own design. Existing pins keep their source,
commit, digest and historical lifecycle artifacts.
