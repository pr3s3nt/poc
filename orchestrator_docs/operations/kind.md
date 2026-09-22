---
id: RUNBOOK-KIND
artifact: operations-runbook
status: current
last_reviewed: 2026-09-22
---

# kind verification

## Safety boundary

- Script dùng cluster kind có sẵn; không tạo hoặc xóa cluster.
- Mọi object phải nằm trong namespace riêng có run ID hoặc mang label run ID.
- Không đổi current Kubernetes context và không sửa workload ngoài namespace
  test.
- Cleanup verification là một phần bắt buộc của run.

## Automated verification

```bash
cd backend
bash test/integration/kind-verify.sh
```

Script build ba acceptance images, load vào cluster, chạy orchestrator qua HTTP,
deploy backend/worker/frontend, kiểm job flow và Web Console, sau đó xóa
namespace trong cleanup trap.

## Required evidence

- Context/cluster/version đã resolve.
- Namespace và run ID.
- PostgreSQL StatefulSet/Service/PVC ready.
- Backend, worker và frontend ready.
- Live Deployment container resources khớp Score theo UC-06 BR-11
  (`container-resources.txt`, assertion trong `TestKindInternalVerification`).
- Frontend → backend → PostgreSQL → worker → backend → frontend job flow pass.
- Password chỉ đi qua Kubernetes Secret và bị redact ở API/UI.
- Sau cleanup không còn object mang run ID; cluster vẫn tồn tại.

Ghi kết quả thành dated record mới dưới `orchestrator_docs/verification/`.
