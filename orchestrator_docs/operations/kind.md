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

The UC-04 host-context verifier can be checked without creating objects:

```bash
cd backend
ORCH_KIND_VERIFY=1 go test ./internal/adapters/kubernetes -run TestKindConnectionVerifierReadOnly -count=1 -v
```

It checks the named context, API, endpoint and create permissions using
`kubectl auth can-i`; it does not create a namespace or workload.

```bash
cd backend
bash test/integration/kind-verify.sh
```

Script build ba acceptance images, load vào cluster, chạy orchestrator qua HTTP,
deploy backend/worker/frontend, kiểm job flow và Web Console, sau đó xóa
namespace trong cleanup trap.

The separate Backstage check uses the preloaded
`ghcr.io/backstage/backstage:1.53.1` amd64 image, the existing Vault/VSO
installation and Traefik. It creates an Application, stores URL settings via
UC-12, deploys a PostgreSQL-backed Backstage workload through UC-16, checks
the public HTML/health/guest API, then deletes only its run-scoped namespace:

```bash
bash backend/test/integration/backstage-kind-verify.sh
```

For this local check, guest sign-in is explicitly allowed in the container;
do not reuse that setting for production. The script needs the scoped Vault
backend token file documented in [Vault on kind](vault-kind.md). It neither
rebuilds nor mirrors the Backstage image to Harbor.

The browser-driven acceptance check deploys the repository's two diagnostic
workloads through the Web Console, including UC-12 variable/secret references
and a PostgreSQL resource. Playwright then opens the deployed app and requires
`PASS` for backend connectivity, environment, secret and database. It uses the
existing kind cluster, Vault/VSO and a run-scoped namespace:

```bash
cd frontend && npm run build
cd ..
bash backend/test/integration/acceptance-playwright-kind.sh
```

The script needs the scoped Vault backend token file documented in
[Vault on kind](vault-kind.md), stores temporary evidence outside the repository,
and deletes its own namespace after execution. It prints a `video=...` path to
`acceptance-full.webm`, covering the browser flow from sign-in to the final
diagnostic page. The video is retained on success and browser-test failure;
review it for local test data before sharing. It does not test cloud delivery
or the worker job flow.

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
