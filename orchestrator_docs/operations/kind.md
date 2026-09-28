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

Public Ingress check (run-scoped, separate from the acceptance workload):

```bash
bash backend/test/integration/public-ingress-kind-verify.sh
```

This check creates one Application/namespace, deploys a BusyBox HTTP workload,
selects its Service port as public, forwards the existing Traefik ClusterIP
Service to local port 18380, sends HTTP with the desired `Host` header, removes
the workload and verifies that the Ingress was removed. The script deletes
only its labeled namespace. This is not permanent Windows-host exposure:
configure controller port mapping and DNS/hosts separately. No TLS is set.

For a workload that remains deployed, a temporary local browser path is:

```bash
kubectl --context kind-idp-internal -n traefik port-forward svc/traefik 8081:80
```

While this command runs, send requests with the configured Host header, for
example `curl -H 'Host: staging.myapp.example.com' http://127.0.0.1:8081/`.
For a browser, map that hostname to loopback in the host's `hosts` file and
open `http://staging.myapp.example.com:8081/`. WSL2 localhost forwarding to
Windows depends on the host configuration and was not verified by this run.
Stop the port-forward to close the temporary path.

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
