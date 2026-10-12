---
id: RUNBOOK-K8S4F-DEV
artifact: operations-runbook
status: current
last_reviewed: 2026-10-12
---

# Môi trường dev Docker IDP và cụm K8S-4F

IDP chạy trên máy bằng Docker Compose. Ứng dụng mẫu frontend/backend/PostgreSQL
chạy trên cụm kind **mới** `k8s-4f`, context `kind-k8s-4f`.
Connection hiển thị **K8S-4F**, key `k8s-4f-2`; Connection `k8s-4f` cũ vẫn
trỏ tới `kind-idp-internal`. Khi chọn đích, dùng tên hiển thị và key mới.

## Truy cập và khởi động IDP

```bash
docker compose -f docker-compose.yml -f deploy/local/compose.kind.yml up -d --wait
docker compose ps
kubectl --context kind-k8s-4f get nodes
```

Overlay giữ backend trên network `kind` để endpoint
`https://k8s-4f-control-plane:6443` của Connection truy cập được từ Docker.
Network và cụm đã có; không chạy lại `kind create cluster` khi cụm còn tồn tại.
Console: <http://localhost:3001/ui/>. Tài khoản local `platform-engineer` và
`developer` dùng password `test-password` theo [Compose runbook](docker-local.md).

Application bàn giao **k8s4f-dev-final** có ID `3c5e4c33-b724-4032-80b1-d270e5c32a35`.
Staging chọn Connection `k8s-4f-2` và Secret Store **K8S-4F Vault**
(`k8s-4f-vault`). Production chưa triển khai.

## Workload và dữ liệu giữ lại

Namespace: `app-3c5e4c33-b724-4032-80b1-d270e5c32a35-staging`.
Image FE/BE: `acceptance-frontend:k8s4f-dev` và `acceptance-backend:k8s4f-dev`,
được load vào cụm mới. PostgreSQL là resource `db` của backend, có PVC 1 GiB.
Không có worker; job được nhận ở trạng thái PENDING.

```bash
kubectl --context kind-k8s-4f \
  -n app-3c5e4c33-b724-4032-80b1-d270e5c32a35-staging \
  get deployment,statefulset,pod,pvc
```

Frontend truy cập qua <http://localhost:18480/> khi port-forward chạy. Để mở
lại đường truy cập sau khi khởi động máy:

```bash
kubectl --context kind-k8s-4f \
  -n app-3c5e4c33-b724-4032-80b1-d270e5c32a35-staging \
  port-forward svc/frontend 18480:8080
```

Giữ terminal chạy; nếu port đang được supervisor của lượt triển khai sử dụng,
dùng URL có sẵn hoặc chọn port khác. Đóng port-forward không xóa workload.

Application từ lượt quay trước **k8s4f-dev** cũng được giữ lại, ID
`a43bb834-d2d7-42cd-a035-41841155b5f7`, namespace
`app-a43bb834-d2d7-42cd-a035-41841155b5f7-staging`. URL port 18480 được bàn giao
cho bản **k8s4f-dev-final**; bản trước không bị xóa dữ liệu.

## Vault riêng cho cụm mới

Container `k8s-4f-workload-vault` nằm trên Docker network `kind`, dùng volume
riêng cho data/bootstrap, HTTP nội bộ `http://k8s-4f-workload-vault:8200`.
Backend và VSO dùng địa chỉ này; không cần Vault port-forward.
VSO chạy trong namespace `vault-secrets-operator-system` trên cụm mới.

Script [start.sh](../../deploy/local/workload-vault/start.sh) chuẩn bị Vault và
Kubernetes auth cho đúng cụm. Xem ownership preflight của script trước khi chạy
lại. Scoped token nằm ngoài Git trong
`~/.local/share/poc-k8s-4f-vault/token`; không hiển thị hoặc chia sẻ nội dung.
Root/unseal material giữ trong bootstrap volume, tách khỏi backend IDP.

Vault được đặt restart policy `unless-stopped`, unseal tự động và renew token.
Token có period 768 giờ; nếu hết hạn hoặc bị revoke và script tạo token mới,
credential đã lưu trong Secret Store không tự đổi theo file. Cần xử lý đăng ký
store/credential hợp lệ theo khả năng Console trước khi Deploy tiếp.

Giữ PostgreSQL/PVC, Vault data/bootstrap và các volume Compose. Xóa cluster,
PVC hoặc volume có thể mất dữ liệu; đây là môi trường dev trên một máy,
không thay thế backup.

## T29 backup/reset gate

T28 prepares this procedure; none of the mutation commands below have been run.
T29 must recheck metadata and present a concrete run manifest for approval right
before backup/reset/deploy, including retention and retain/restore versus delete
of each owned workload/PVC. Do not use `docker compose down -v`. Current source
requires an empty IDP baseline; do not restart the retained backend with it early.

Known ownership to revalidate:

| Target | Owned data | Treatment pending T29 approval |
|---|---|---|
| `orchestrator-local-postgres-1`, DB `orchestrator` | IDP metadata, volume `orchestrator-local_postgres-data` | Private logical dump; reset only this DB after restore smoke. |
| final app namespace `app-3c5e4c33-b724-4032-80b1-d270e5c32a35-staging` | backend/frontend, PostgreSQL StatefulSet/PVC | Inventory exact names, writer replicas and reclaim policy; consistent dump; retain quiesced or delete only as approved. |
| earlier app namespace `app-a43bb834-d2d7-42cd-a035-41841155b5f7-staging` | same kinds, independent data | Separate dump and cleanup decision; never infer ownership from the final app. |
| `orchestrator-local_vault-data`, `orchestrator-local_vault-bootstrap`, `orchestrator-local_connection-token`, `orchestrator-local_app-token` | IDP credential/bootstrap data | Preserve; private recovery handles, no content in evidence. |
| `k8s-4f-workload-vault-data`, `k8s-4f-workload-vault-bootstrap` | workload secrets/bootstrap | Preserve; no reset or token dump. |
| `kind-k8s-4f`, VSO and `kind-idp-internal` | clusters/operators/unrelated workloads | Preserve; do not create/delete clusters or reset VSO. |

Read-only preflight commands (never include Secrets, kubeconfig, full inspect
Environment or `docker compose config` in public evidence):

```bash
docker ps --format '{{.Names}} {{.Image}} {{.Status}}'
docker inspect -f '{{range .Mounts}}{{.Type}} {{.Name}} {{.Destination}}{{println}}{{end}}' orchestrator-local-postgres-1
kubectl --context kind-k8s-4f get nodes
kubectl --context kind-k8s-4f -n vault-secrets-operator-system get deployments
kubectl --context kind-k8s-4f -n app-3c5e4c33-b724-4032-80b1-d270e5c32a35-staging get deploy,sts,pod,pvc,svc
kubectl --context kind-k8s-4f -n app-a43bb834-d2d7-42cd-a035-41841155b5f7-staging get deploy,sts,pod,pvc,svc
```

Use authenticated safe API metadata privately to recheck both apps' bindings,
store identity, generation, active operations and current/history Set ownership.
T29 must resolve exact PostgreSQL pod/database/user and writer deployment names
from that inventory. Capture replica counts and image digests for recovery.
No automatic container-volume tar of a running PostgreSQL/Vault is a consistent
backup. Backup files may contain secrets; permissions 0700 directory/0600 files,
outside Git/video, no upload or stdout of their contents.

The approved shell runs with `set -euo pipefail` and `umask 077`. Example exact
IDP dump/reset commands for the known target, after backend/writers are quiesced:

```bash
T29_PRIVATE=/private/approved-T29-run
test ! -e "$T29_PRIVATE"
mkdir -m 700 "$T29_PRIVATE"
docker stop orchestrator-local-backend-1
docker exec orchestrator-local-postgres-1 pg_dump -U orchestrator -d orchestrator -Fc > "$T29_PRIVATE/idp.dump"
test -s "$T29_PRIVATE/idp.dump"
sha256sum "$T29_PRIVATE/idp.dump" > "$T29_PRIVATE/idp.dump.sha256"
```

For each inventoried workload DB, `kubectl ... scale deployment/<exact-writer>
--replicas=0` and wait for its pods to terminate before `kubectl ... exec
<exact-db-pod> -- pg_dump -U <exact-user> -d <exact-db> -Fc > <private-dump>`.
Resolve every placeholder into the approval manifest; PostgreSQL remains running
for consistent logical dumps. Record separate checksums and retained PVC identity;
if a writer is not owned or cannot be stopped, abort before destructive work.

Restore-smoke uses a separately approved uniquely labelled disposable PostgreSQL
container matching each source major version, ephemeral storage and no retained
network/volumes. Copy the private dump into it, run `pg_restore --list`, then
`pg_restore --exit-on-error --no-owner --no-acl -U <test-owner> -d <empty-test-db>`.
Compare restored row counts/integrity and readable application records privately;
record only success, object counts/checksums and cleanup in safe evidence. Remove
only that disposable container. A synthetic T28 smoke does not verify these real
backups. Stop on dump/restore failure and retain backups and quiesced source.

Only after verified backups and the approved data/cleanup decision:

```bash
docker exec orchestrator-local-postgres-1 dropdb -U orchestrator --force orchestrator
docker exec orchestrator-local-postgres-1 createdb -U orchestrator -O orchestrator orchestrator
docker compose -f docker-compose.yml -f deploy/local/compose.kind.yml up -d --build --wait
```

Resetting only this database preserves its volume, role and all Vault volumes.
New Application IDs avoid old namespace collisions. Retained source namespaces
stay quiesced with recovery metadata; deletion commands must name only approved
objects, check PVC/PV reclaim behavior, and verify no old writer/serve endpoint
remains. No broad namespace/PVC deletion is preapproved.

Re-register Connection and ordinary Store through UI, create the new app, select
staging target/store, set Variables/Secrets, save PostgreSQL/BE/FE workloads,
Preview and Deploy explicitly. T29 reviews/updates the historical runner's current
Vietnamese locators and safe credential handling before executing:

```bash
K8S4F_RUN_ID=<approved-run-id> K8S4F_APPLICATION=<new-app-name> K8S4F_SUBDOMAIN=<new-subdomain> K8S4F_FORWARD_PORT=<free-port> K8S4F_KUBECONFIG_FILE=<private-upload-path> bash backend/test/integration/k8s4f-dev-playwright.sh /tmp/poc-T29-<unique-evidence>
```

Private file paths are inputs only; never print file contents. Preserve failed
video/assertions/phase marks; decode/review captions and safe frames. Confirm new
namespace/PVC/pods Ready, diagnostics, backend reload persistence, chosen data
restore or accepted empty DB, and cleanup ownership. Record actual new IDs/keys/
URLs and backup expiry. No live replay merely for a better recording.

Recovery before reset: restart the original pinned backend and restore writer
replicas from inventory. Recovery after reset: stop the new backend, restore the
IDP dump into an empty `orchestrator` DB using `pg_restore --exit-on-error`, restore
the original pinned backend image compatible with that dump, then restore owned
source replicas/port-forward. Do not serve an old dump with the new baseline.
Workload restore, if needed, uses the approved source/destination DB and quiesced
writers. Failed runs retain all backups; no expiry cleanup without the agreed
retention. Exact recovery image/commands are part of T29 approval, not assumptions.
