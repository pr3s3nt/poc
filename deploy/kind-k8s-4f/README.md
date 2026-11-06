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
