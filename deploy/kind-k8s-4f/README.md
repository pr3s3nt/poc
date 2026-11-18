# Console trên cụm kind-k8s-4f

Manifest của Console chạy trong namespace `orchestrator-dev-3070` trên cụm
`kind-k8s-4f`. Cụm nằm ở distro WSL `Ubuntu`, cùng máy Windows với `Dev-Ubuntu-24.04`
nên gọi được qua loopback. Thư mục này không chứa secret: credential được sinh trong
PVC `platform-storage` khi cài lần đầu. Không xóa PVC này.

## Biến dùng chung

Chạy từ gốc repo, trên commit cần triển khai. `IMAGE_TAG` trong manifest được thay
bằng `TAG` khi apply.

```bash
export KUBECONFIG=~/.local/state/orchestrator-dev-3070/private/kubeconfig
K="kubectl --context kind-k8s-4f -n orchestrator-dev-3070"
TAG=main-$(git rev-parse --short=8 HEAD)
render() { sed "s/IMAGE_TAG/$TAG/g" "deploy/kind-k8s-4f/$1"; }
```

## Build và nạp image

Manifest dùng `imagePullPolicy: Never`, nên image phải có sẵn trong node kind.

```bash
docker build -t "orchestrator-backend:$TAG" backend
docker build -t "orchestrator-frontend:$TAG" frontend
for image in backend frontend; do
  docker save "orchestrator-$image:$TAG" |
    wsl.exe -d Ubuntu -- docker exec -i k8s-4f-control-plane ctr -n k8s.io images import -
done
```

## Cài mới

Chỉ dùng khi namespace chưa tồn tại. Namespace đã có thì dùng mục Nâng cấp.

```bash
kubectl --context kind-k8s-4f apply -f deploy/kind-k8s-4f/00-namespace.yaml
kubectl create configmap installation-scripts -n orchestrator-dev-3070 \
  --from-file=deploy/local/init-db-credentials.sh \
  --from-file=deploy/local/postgres-init-roles.sh \
  --from-file=deploy/local/start-vault.sh --from-file=deploy/local/vault.hcl \
  --from-file=deploy/local/connections-policy.hcl \
  --from-file=deploy/local/applications-policy.hcl \
  --from-file=deploy/local/outputs-policy.hcl --dry-run=client -o yaml |
  kubectl label --local -f - -o yaml \
    app.kubernetes.io/part-of=orchestrator-dev-3070 \
    app.kubernetes.io/managed-by=poc-local-installer |
  kubectl --context kind-k8s-4f apply -f -
kubectl --context kind-k8s-4f apply -f deploy/kind-k8s-4f/01-storage.yaml \
  -f deploy/kind-k8s-4f/02-postgres.yaml -f deploy/kind-k8s-4f/03-vault.yaml
$K wait --for=condition=available deployment/postgres deployment/vault --timeout=180s
# Vault Kubernetes auth cho workload, xem mục bên dưới
kubectl --context kind-k8s-4f apply -f deploy/kind-k8s-4f/04-vault-kubernetes-auth.yaml
deploy/kind-k8s-4f/vault-kubernetes-auth.sh
render 10-db-setup.yaml | kubectl --context kind-k8s-4f apply -f -
$K wait --for=condition=complete "job/db-setup-$TAG" --timeout=180s
render 20-backend.yaml | kubectl --context kind-k8s-4f apply -f -
render 21-frontend.yaml | kubectl --context kind-k8s-4f apply -f -
$K wait --for=condition=available deployment/backend deployment/frontend --timeout=180s
```

## Nâng cấp

Dữ liệu được giữ lại, nên làm theo
[runbook nâng cấp PostgreSQL](../../orchestrator_docs/operations/postgres-upgrades.md).
Không reset. Dừng writer và có backup riêng đã kiểm tra trước bước owner setup:

```bash
$K scale deployment/backend deployment/frontend --replicas=0
# Backup PostgreSQL, Vault, credential và state key ra ngoài Git, rồi kiểm tra bản backup.
render 10-db-setup.yaml | kubectl --context kind-k8s-4f apply -f -
$K wait --for=condition=complete "job/db-setup-$TAG" --timeout=180s
render 20-backend.yaml | kubectl --context kind-k8s-4f apply -f -
render 21-frontend.yaml | kubectl --context kind-k8s-4f apply -f -
$K scale deployment/backend deployment/frontend --replicas=1
$K wait --for=condition=available deployment/backend deployment/frontend --timeout=180s
```

Job cũ được giữ làm dấu vết của từng lần nâng cấp.

## Bật Keycloak SSO trên installation hiện có — issue #88

Đây là thao tác vận hành tích hợp OIDC đã có, không cài lại Console/database.
Đọc [SSO lifecycle](../../orchestrator_docs/operations/keycloak-sso.md) trước.
Triển khai Keycloak bằng tài khoản Platform Engineer qua Console theo
[mẫu Score](../../backend/examples/keycloak/README.md). Trên installation có
Definition PostgreSQL `in-cluster`, khai báo `class: in-cluster` cho resource
`db`. Dùng Application, PostgreSQL/PVC, Secret admin và endpoint riêng; không
chiếm hostname hoặc dữ liệu của bản thử khác. Lưu toàn bộ draft trước một lần
Environment Preview/Deploy. Bounded Fleet namespace setup theo runbook của mẫu.

Chỉ bật SSO sau khi workload/database Ready, VSO synced, HTTPRoute được chấp nhận,
và browser mở được đúng endpoint qua Tailscale. Backend cũng phải truy cập được
issuer/discovery/JWKS/token endpoint bằng hostname đã công bố. Tạo realm/client
confidential với Authorization Code, PKCE S256, callback chính xác của Console
và client-role mapper trong ID token. Dùng username SSO khác username local đã
có: sản phẩm không liên kết/migrate tài khoản trùng tên. Giữ một personal local
ACTIVE Admin cùng Organization qua `localTestAdminUsername`; bootstrap bằng CLI
với password file riêng nếu chưa có. Không cấp quyền quản trị Keycloak cho các
role nhân viên của nền tảng.

Trước rollout, kiểm tra không có operation đang chạy/giữ môi trường; lưu inventory,
Deployment hiện tại và backup PostgreSQL ra ngoài Git. Chuẩn bị file OIDC JSON
0600 ngoài Git theo SSO runbook. `OIDC_CONFIG_FILE` dưới đây là đường dẫn tới file
riêng đó, không phải giá trị secret. Không in nội dung hoặc dùng secret trên CLI.

```bash
kubectl --context kind-k8s-4f -n orchestrator-dev-3070 create secret generic backend-oidc \
  --from-file=oidc.json="$OIDC_CONFIG_FILE"
# TAG phải trùng image backend đang chạy, không lấy HEAD docs-only để đoán image.
sed "s/IMAGE_TAG/$TAG/g" deploy/kind-k8s-4f/22-oidc.patch.yaml > "$PRIVATE_DIR/oidc.patch.yaml"
kubectl --context kind-k8s-4f -n orchestrator-dev-3070 patch deployment backend \
  --type=strategic --patch-file "$PRIVATE_DIR/oidc.patch.yaml"
$K rollout status deployment/backend --timeout=180s
```

Patch sao chép Secret sang memory volume, owner 65532 và mode 0600; backend chỉ
mount bản sao read-only, không có owner database credential. Một replica theo
giới hạn provider-session hiện có. Đổi Secret không tự reload cấu hình: restart
backend sau khi xác minh thay đổi. Khi upgrade base manifest, áp dụng lại patch
trước khi start runtime để tránh vô tình đổi authentication mode.

Kiểm tra browser trên đúng URL Console/Keycloak qua Tailscale: ba role SSO,
Applications/session APIs, personal local Admin và logout. Restart backend phải
yêu cầu SSO login lại nhưng giữ User ID và domain data. Giữ config, dữ liệu và
forward/relay cần cho các URL. Exact URLs, password files, inventory và resource
UIDs chỉ ở private handoff, không đăng issue public.

Nếu bật SSO thất bại, kiểm tra log an toàn và khôi phục Deployment đã lưu trước
rollout (hoặc `rollout undo` sau khi xác nhận revision đích đúng bản local trước
đó). Không reset database, xóa external accounts hay đổi schema marker. Giữ
Secret cấu hình và Keycloak/PVC để chẩn đoán; rollback authentication không phải
cleanup workload. Khi người dùng yêu cầu gỡ, dùng product Application deletion
và cleanup bounded RBAC theo UID/ownership sau CD withdrawal.

## Vault Kubernetes auth cho workload

Workload nhận secret qua VSO, nên Vault của Console cần auth mount `kubernetes`.
Pod Vault không mount ServiceAccount token, vì vậy dùng một ServiceAccount
reviewer riêng có `system:auth-delegator` và token Secret do Kubernetes sinh.
Đây là bước của mục Cài mới; với namespace đã cài từ trước, chạy riêng:

```bash
kubectl --context kind-k8s-4f apply -f deploy/kind-k8s-4f/04-vault-kubernetes-auth.yaml
deploy/kind-k8s-4f/vault-kubernetes-auth.sh
```

Script bật mount nếu chưa có và ghi lại cấu hình mỗi lần chạy, không in token/CA.
Trạng thái Kubernetes auth trên trang Secret Store chỉ được ghi lúc đăng ký store,
nên store đăng ký trước bước này vẫn hiện "Chưa cấu hình"; Deploy dùng cấu hình
thực tế của Vault. Thiết lập lần đầu ngày 2026-11-08 (issue #51).

## Link Tailscale

Console mở tại `http://<IP Tailscale>:3070/ui/`, qua hai forward chạy trong tmux. Vòng
lặp giúp port-forward tự nối lại khi API server khởi động lại. Sau khi khởi động
lại máy, chạy lại cả hai lệnh, rồi mở link bằng browser để kiểm tra.

```bash
TS_IP=$(ip -4 -o addr show tailscale0 | awk '{print $4}' | cut -d/ -f1)
tmux new-session -d -s orch3070-cluster "while :; do KUBECONFIG=$KUBECONFIG kubectl --context kind-k8s-4f \
  -n orchestrator-dev-3070 port-forward --address 127.0.0.1 svc/frontend 13070:8080; sleep 2; done"
tmux has-session -t orch3070-tailnet 2>/dev/null || tmux new-session -d -s orch3070-tailnet \
  "socat TCP4-LISTEN:3070,bind=$TS_IP,reuseaddr,fork TCP4:127.0.0.1:13070"
```
