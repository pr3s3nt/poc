---
id: OPERATIONS-INDEX
artifact: operations-index
status: current
last_reviewed: 2026-09-21
---

# Operations index

| Environment | Runbook | External mutation |
|---|---|---|
| Local fake adapters | [Local development](local.md) | No cluster/cloud mutation |
| Existing kind cluster | [kind verification](kind.md) | Creates a run-specific namespace and workloads |
| AWS | [AWS verification](aws.md) | Creates paid VPC/EKS/Aurora/ECR resources, then destroys them |

[UC-12 Vault on kind](vault-kind.md) documents the persistent Vault release,
its sealed/init handoff and the existing Agent Injector. It is separate from
the run-scoped kind verification cleanup procedure.

Runbook là procedure hiện hành. Kết quả của từng execution phải được ghi thành
dated record mới trong [verification index](../verification/README.md), không
ghi đè runbook hoặc evidence cũ.
