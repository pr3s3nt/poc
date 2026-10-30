---
id: OPERATIONS-INDEX
artifact: operations-index
status: current
last_reviewed: 2026-10-28
---

# Operations index

Trước khi thêm runner E2E/setup mới, dùng [quy ước thư mục E2E](../../e2e/README.md).
Scripts nằm dưới `e2e/`; procedure vẫn ở index này và evidence ở `verification/`.
Các runner hiện có giữ đường dẫn cho tới migration/refactor liên quan.

[Generic Terraform resource completion on kind](terraform-completion-kind.md)
verifies UC-08 BR-10 with real Git modules and the full application, including
shared/private resources, valid updates and product cleanup.

[Application deletion acceptance](application-deletion-acceptance.md) runs the
UC-18 Developer deletion at the end of the existing kind and AWS A02 harnesses.

| Environment | Runbook | External mutation |
|---|---|---|
| Application telemetry | [Native logging/monitoring](observability.md) | Read-only collector; Metrics Server/RBAC/database preparation is an explicit operator action |
| Local fake adapters | [Local development](local.md) | No cluster/cloud mutation |
| Offline staging providers | [Image check and staging configuration](terraform-offline.md) | Local image build/check; staging rollout is a separate operator action |
| Personal-machine containers | [Docker Compose](docker-local.md) | Local Docker volumes; cluster mutation only on explicit deploy |
| UC-09 review video | [Human-paced local recording](local.md#uc-09-human-paced-review-recording) | Local browser/display only; publish reviewed evidence separately |
| Existing kind cluster | [kind verification](kind.md) | Creates a run-specific namespace and workloads |
| Persistent Preview Console | [Preview on kind](preview-kind.md) | Separate local services; deployment is explicit |
| AWS | [AWS verification](aws.md) | Creates paid VPC/EKS/Aurora/ECR resources, then destroys them |

[UC-12 Vault on kind](vault-kind.md) documents the persistent Vault release,
VSO, its sealed/init handoff and the legacy Agent Injector. It is separate from
the run-scoped kind verification cleanup procedure.

[Triển khai acceptance app thủ công lên k8s-4f](acceptance-app-manual-k8s4f.md)
hướng dẫn Connection, Vault/VSO, Secret Store, image và FE/BE/PostgreSQL qua Console.
Ứng dụng được giữ lại sau khi thao tác; không có cleanup tự động như E2E runner.

[Môi trường dev K8S-4F mới](k8s4f-dev.md) phân biệt cụm vật lý `kind-k8s-4f`
và Connection mới `K8S-4F` (`k8s-4f-2`) với Connection cũ trên `idp-internal`;
ghi cách truy cập IDP, frontend và dữ liệu được giữ lại.

[Fleet GitRepo on kind](fleet-gitrepo-kind.md) documents the optional
Harbor-image workload delivery path and its separate Git/registry credentials.

[Orchestrator PostgreSQL on kind](postgres-kind.md) documents the dedicated
persistent database installed for the future system store and Terraform state.

[Self-hosted Orchestrator on kind](self-host-kind.md) documents the container
images and the recorded flow in which the Orchestrator deploys itself and then
the acceptance app.

Runbook là procedure hiện hành. Kết quả của từng execution phải được ghi thành
dated record mới trong [verification index](../verification/README.md), không
ghi đè runbook hoặc evidence cũ.

[Codex điều phối Claude qua tmux](claude-tmux.md) quy định procedure code/review/validation/commit/push và cleanup session riêng của task.

- [Terraform v2 acceptance and authorized execution](terraform-v2-acceptance.md) — required live Web Console/app flow, real adapters and ownership cleanup.

- [Terraform module GitHub sources](terraform-module-sources.md): published modules, Definition authoring and isolated verification.

- [Public routes: Ingress or Gateway API HTTPRoute](public-routes.md) — ADR-024 runtime flags and staging Gateway setup.
- [Git render-bundle real acceptance](git-render-bundles-acceptance.md) — required new isolated kind/Compose Playwright procedure; runner is an implementation deliverable, no fake-adapter acceptance.

[Kubernetes staging catalog bootstrap](staging-catalog.md) — Types-only startup và Playwright đăng ký catalog sau Driver Account `k8s-4f`.

ZIP source preparation: [module and render-bundle upload](zip-source-upload.md).

[Optional CD and drift reconciliation](delivery-reconciliation.md) — runtime flags, verified binding/Environment flow, reviewed repair, owner-only retention and recovery boundaries.
