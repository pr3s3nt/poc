---
id: RUNBOOK-K8S4F-DEV
artifact: operations-runbook
status: current
last_reviewed: 2026-10-12
---

# Môi trường dev Docker IDP và cụm K8S-4F

IDP chạy trên máy bằng Docker Compose. Ứng dụng mẫu frontend/backend/PostgreSQL
chạy trên cụm kind **mới** `k8s-4f`, context `kind-k8s-4f`.
Sau T29, Connection hiển thị **K8S-4F**, key `k8s-4f`, trỏ tới
`kind-k8s-4f` bằng KUBECONFIG. Metadata Connection cũ đã được backup trước
reset IDP; cluster `kind-idp-internal` được giữ nguyên.

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

Application bàn giao **k8s4f-t29-20261012-r1** có ID `47fb34b4-2d6e-493e-a2c5-880837abfc1c`.
Staging chọn Connection `k8s-4f` và Secret Store **K8S-4F Vault**
(`k8s-4f-vault`). Production chưa triển khai.

## Workload và dữ liệu giữ lại

Namespace: `app-47fb34b4-2d6e-493e-a2c5-880837abfc1c-staging`.
Image FE/BE: `acceptance-frontend:k8s4f-dev` và `acceptance-backend:k8s4f-dev`,
được load vào cụm mới. PostgreSQL là resource `db` của backend, có PVC 1 GiB.
Không có worker; job được nhận ở trạng thái PENDING.

```bash
kubectl --context kind-k8s-4f \
  -n app-47fb34b4-2d6e-493e-a2c5-880837abfc1c-staging \
  get deployment,statefulset,pod,pvc
```

Frontend truy cập qua <http://localhost:18481/> khi port-forward chạy. Để mở
lại đường truy cập sau khi khởi động máy:

```bash
kubectl --context kind-k8s-4f \
  -n app-47fb34b4-2d6e-493e-a2c5-880837abfc1c-staging \
  port-forward svc/frontend 18481:8080
```

Giữ terminal chạy; nếu port đang được supervisor của lượt triển khai sử dụng,
dùng URL có sẵn hoặc chọn port khác. Đóng port-forward không xóa workload.

Application từ lượt quay trước **k8s4f-dev** cũng được giữ lại, ID
`a43bb834-d2d7-42cd-a035-41841155b5f7`, namespace
`app-a43bb834-d2d7-42cd-a035-41841155b5f7-staging`. URL port 18481 được bàn giao
cho bản **k8s4f-t29-20261012-r1**; bản trước không bị xóa dữ liệu. T29 đã dừng FE/BE của bản này và bản final
`app-3c5e4c33-b724-4032-80b1-d270e5c32a35-staging` (replicas 0); cả hai
PostgreSQL vẫn 1/1 và PVC giữ nguyên. Chúng không còn metadata trong IDP mới.
Port 18480 cũ không serve; app mới dùng port 18481.

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

T28 prepared this procedure; T29 executed the approved fixed run described below.
The generic procedure remains a gate for any future reset, not authorization to rerun.
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

## T29 runner preparation before approval

The clean-baseline run must not require a historical `k8s-4f` Connection or its
HOST_CONTEXT predecessor. Capture all safe pre-existing Connection/Store metadata
at runner start and assert those records remain unchanged; an empty Connection
catalog is valid. Register the new KUBECONFIG Connection and ordinary workload
Store through UI. Resume is explicit, identity-checked, and may only reuse records
created in the approved run. Do not recreate the old cluster Connection for the
sake of historical assertions.

Require a new evidence directory outside Git, restrict it to owner access, and
preserve every failed recording. Runner preparation and local fake checks are
allowed before the live gate; invoking the retained runner, building/restarting
Compose, backups and cluster mutation are not. The run manifest must pin recovery
backend/frontend image IDs and resolve exact backup/restore commands before approval.

## T29 data decision and concrete run preparation

On 2026-10-12 the user selected: retain both old namespaces/PVC; stop their
backend/frontend Deployments; keep their PostgreSQL StatefulSets at one replica;
create the new application with an empty workload database; retain all three
logical backups for 30 days. No workload data restore is requested. This answers
only the data gate, not the final backup/reset/deploy authorization.

Prepare run `t29-20261012-r1`, new app/subdomain `k8s4f-t29-20261012-r1`,
port 18481, private directory
`/home/thanhnt1/.local/share/poc-T29/t29-20261012-r1` and public-safe evidence
`/tmp/poc-T29-live-20261012-r1`. Backup expiry is 30 days after actual backup
creation; do not install automatic deletion or delete on failure.

The new preparation wrapper must default to offline dry-run. Live invocation
requires an explicit apply argument and matching run ID after user approval.
Stages: revalidate exact owners/images/PVC/replicas and API operation metadata;
prepare source builds without restarting retained services; pin old recovery
images; quiesce only IDP backend/frontend and the two old app backend/frontend;
make three private dumps; verify SHA256, restore each into an isolated PostgreSQL
16 container with tmpfs data/network none/no ports/no retained mounts; compare
all source/restored public-table row counts and both workload jobs checksums;
remove only the label-owned smoke container; reset only DB `orchestrator`;
start current backend/frontend on the existing Compose networks/volumes; run
headed UI registration, settings, new workloads, Preview and Deploy; verify new
Ready workloads/PVC, four diagnostics, persistent state and old FE/BE at zero.
Stop at any failure; retain backups/evidence, never auto-replay deploy.

Safe revalidation observed different current image IDs from the first preflight:
backend `sha256:61b2fed3dc59ff9425199bce2439025ec151462e51c811d41a9cc0f8fc103571`,
frontend `sha256:de449a00db689693084656eeb9cd2f121cc7cf31dfb75eaef28516fa6a2763b9`.
Use those as expected recovery images and abort on further drift. Pin both before
build. Snapshot historical Compose source from `a0ef186` for recovery, override
image tags/pull_policy with `never`, run only backend/frontend with `--no-build
--no-deps --pull never`; old DB is never served by a fresh-baseline image.
Private recovery configuration belongs outside Git and video.

No private upload file currently exists at the historical default location.
After final approval, generate a flattened/minified kubeconfig for
`kind-k8s-4f` into the private run directory, preserving embedded credentials/CA
and changing only server to `https://k8s-4f-control-plane:6443`. Never print it.
Use the existing scoped workload Vault token only as masked registration input;
no Vault start/bootstrap script, token refresh or volume reset is authorized.

Reuse already-loaded `acceptance-backend:k8s4f-dev` and
`acceptance-frontend:k8s4f-dev` images, with separate runner image-tag and run-ID
inputs. Do not change old workload image tags or rebuild the cluster. New app ID
must differ from both retained IDs; unknown ID collision aborts before deploy.
A run-owned port-forward PID file must preserve the old supervisor handle.

Recovery is an explicit separate command in the same approved run scope:
quiesce any newly created app backend/frontend (retain its DB/PVC), stop current
IDP backend/frontend, verify private dump checksums, drop/create IDP database and
restore idp.dump, start pinned historical images with historical Compose source,
then restore exactly both old app backend/frontend to their inventoried one
replica. Before reset, recovery only restarts original containers/old replicas.
No namespace/PVC deletion, dump deletion, operator/cluster rebuild, unrelated
container mutation or automatic recovery that hides a failed run is allowed.

### Fixed run commands awaiting final user confirmation

Preparation code: `backend/test/integration/k8s4f-dev-reset.sh`. Reviewable
command manifest is generated offline with:

```bash
bash backend/test/integration/k8s4f-dev-reset.sh --dry-run
```

After explicit approval of run `t29-20261012-r1` only:

```bash
bash backend/test/integration/k8s4f-dev-reset.sh --apply --run-id t29-20261012-r1
```

If the approved run fails and scoped recovery is selected, the separate command is:

```bash
bash backend/test/integration/k8s4f-dev-reset.sh --recover --run-id t29-20261012-r1
```

These commands are not authorization by themselves. The wrapper refuses any
other run ID and changes in pinned image/PVC metadata. Default/dry-run calls
contact no Docker, Kubernetes or network service. Offline syntax, stubbed
ownership/failure checks and source review do not prove real backup readability;
three actual dumps must pass restore smoke during the approved run before reset.
Source IDP metadata is removed by reset; source workloads remain outside the
new IDP's management and quiesced. Current seed catalog is recreated, including
its own empty Deployment Set; historical deployments are not restored.
Console downtime begins at quiesce and ends after current services become healthy;
old application FE/BE remain stopped after success. Port 18480 no longer serves
those apps; new app access is port 18481. Preserve old port-forward recovery handle.
All private dump/recovery files remain owner-only for 30 days from backup creation;
retain failed-run backups and never upload private material or evidence.


Same-run partial continuation after a pre-deploy runner failure must use a fresh
phase evidence directory, explicitly pin whichever Connection/Store has already
been created, and preserve the failed phase. Validate identity before reuse;
never infer ownership from a name or replay Deploy automatically. Recovery reads
a restricted phase marker pointing inside the fixed run evidence directory so
it can quiesce the correct newly created app. The first phase's registration and
the continuation's deployment videos are separate execution segments; do not
present them as one uninterrupted recording.


### T29 successful handoff (2026-10-12)

Run `t29-20261012-r1` completed after a preserved pre-deploy locator failure
and one explicit partial continuation (`--continue --run-id t29-20261012-r1`).
App/new namespace/Connection/Store and URL above are current. Backups are private:
`/home/thanhnt1/.local/share/poc-T29/t29-20261012-r1`, expiry
`2026-11-11T04:05:55Z`; retain them, no automated expiry deletion is installed.
Restore smoke for all three real dumps/counts/job checksums passed before IDP reset.
New workload DB started empty, then accepted one PENDING diagnostic job; no worker
is deployed. Four diagnostics, UI reload and IDP backend restart persistence pass.

Real evidence: `/tmp/poc-T29-live-20261012-r1/ui/` (failed first phase, Connection
registration) and `ui-resume1/` (passed remaining UI flow and Deploy). These are
separate recordings. Full metadata/backup/cleanup/video review is in
[T29 evidence](../verification/2026-10-12-T29-dev-rebuild.md).
New frontend supervisor handle:
`~/.local/share/poc-k8s-4f-vault/frontend-forward-t29-20261012-r1-18481.pid`.
It is retained for access; the old handle was preserved.
Only the run-owned restore-smoke container and recording processes were cleaned.
No old namespace/PVC, cluster, VSO or Vault volume was deleted. Scoped Connection/
Store credential and new app secret delivery writes were made as approved.
Recovery command above restores the old IDP dump with pinned historical images
and re-enables old FE/BE; it first quiesces the newly created app and retains its
PVC. Recovery was verified offline and prepared, but not executed on live state
because the run succeeded. Do not use it casually against a healthy current app.


## V01 dữ liệu phát triển — quyết định 2026-10-17

Người dùng giao V01 và cho xóa dữ liệu sản phẩm phát triển cũ vì chưa có người
dùng thực tế; V02–V03 không cần migration dữ liệu. Quyền này cho phép Codex
reset database/state sản phẩm đúng instance sau preflight ownership và chuẩn bị
baseline V01; không cần lặp lại điểm duyệt T29 cho cùng phạm vi đã cho phép.
Không suy thành quyền xóa cluster, Docker metadata store, workload ngoài phạm
vi hoặc tài nguyên cloud. Không dùng `docker compose down -v` để reset tràn.
Writer không truy cập retained data hoặc credential; Codex chạy thật và ghi
phạm vi/identity/kết quả trước-sau trong [V01 handoff](../verification/2026-10-17-v01-handoff.md).
Khi Docker/cluster không truy cập được, không sửa/xóa metadata để ép reset;
kiểm chứng riêng với DB/Vault task-owned và báo rõ reset retained chưa thực hiện.
