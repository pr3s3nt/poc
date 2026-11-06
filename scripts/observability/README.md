# Portable observability installer — issue #35, chặng 0

Bash + Helm, chạy độc lập với Orchestrator. Copy nguyên thư mục này sang máy
vận hành; không cần Go, Node.js hoặc Python. Cài mới phần đã chọn và reuse phần
có sẵn. Không tự nhận quản lý release cũ, reset dữ liệu hoặc uninstall.

## Chuẩn bị

Linux, Bash **4.4+**, Helm **3.19.x**, kubectl tương thích cluster, jq 1.6+,
curl, sha256sum, flock và realpath. Chart metrics yêu cầu Kubernetes >=1.25;
đối chiếu từng chart với distribution/API/Pod Security thực tế. Có quyền đọc
Helm inventory/workloads/CRDs toàn cluster; cài chart cần thêm quyền tạo tài nguyên
namespaced/cluster-scoped tương ứng. Thiếu quyền kiểm kê phải dừng, không coi là
chưa có công cụ. Ví dụ là cấu hình cluster nhỏ, không phải chuẩn production.

## Chạy

1. Copy một example, sửa `context`, `installationId`, `namespace`, `storageClass`.
   Giữ `installationId` ổn định và dùng state directory riêng cho mỗi cluster.
2. Sửa `valuesFile`: dùng đường dẫn tuyệt đối hoặc đường dẫn tương đối **tính từ config**, không từ working directory.
   Sửa mọi `CHANGE_ME` trong values được bật. Với Alloy: khai báo namespace được
   phép thu logs trong cả `rbac.namespaces` và discovery, cùng Loki push URL.
   Nếu dùng names ví dụ và cùng namespace: `http://obs-loki-gateway/loki/api/v1/push`.
3. Kiểm tra CPU/RAM, PVC/quota, storage class, DNS/image access, security policy,
   chart versions/checksums và stack hiện có. Các storage/PVC requests trong ví dụ
   cần ít nhất 20Gi cho metrics/Grafana/Alertmanager/Loki, chưa gồm overhead.
4. Chạy plan, xem kết quả và manifests riêng tư rồi apply bằng UID được in ra:

```sh
./setup.sh plan --config /private/cluster.json --state-dir /private/obs-state
./setup.sh apply --config /private/cluster.json --state-dir /private/obs-state \
  --cluster-uid '<UID từ plan>'
./setup.sh verify --config /private/cluster.json --state-dir /private/obs-state
```

`plan` chỉ đọc cluster, có tải chart và ghi artifact local ngoài Git.
`apply` tự kiểm kê/validate lại trước mutation. Thêm hoặc đổi values có thể cập
nhật release đã sở hữu; **chạy lại plan và review trước mỗi thay đổi**. Không tự
chuyển kube context. Không tự cài kubectl/Helm/cluster hoặc sửa DNS/ingress.

## Cluster đã có một phần

Mỗi component chọn `install`, `reuse` hoặc `skip` có lý do.

```json
"grafana": {
  "mode": "reuse",
  "resources": [
    {"kind": "deployment", "namespace": "shared-monitoring", "name": "grafana"}
  ]
}
```

`reuse` kiểm tra controller Ready và pin UID khi apply, không đọc/sửa cấu hình
hoặc credential của nó. Các loại controller: `deployment`, `statefulset`,
`daemonset`; khai báo đủ controller cần kiểm tra. Có thể thêm `expectedUID`.
Bộ cài tự tắt Grafana tương ứng trong chart metrics; tương tự với Prometheus,
Alertmanager, Operator, kube-state-metrics và node-exporter.

- `examples/new-cluster.json`: cài metrics stack, Loki, Alloy.
- `examples/existing-stack.json`: reuse Grafana/Operator, cài phần còn lại.
- `examples/with-exporters.json`: thêm PostgreSQL/blackbox exporter; phải chuẩn bị
  DB monitoring account/Secret và hoàn thiện values trước khi bật. Script không
  tạo tài khoản DB, bật Traefik hay sửa Gateway/Admin.
- Nếu reuse toàn bộ metrics stack: đặt từng component thành reuse/skip và bỏ entry
  `metrics` khỏi `releases`. Tương tự Loki/Alloy/exporters.

Release có sẵn dù trùng name cũng không được tự adopt. Reuse nó hoặc chọn tên
release mới và chỉ bật thành phần còn thiếu. Fingerprints image upstream giúp
phát hiện duplicate khác tên; custom images/distribution khác phải kiểm kê thủ công.
Script không thể suy ra mọi stack chỉ từ tên/image.

Nếu có monitoring CRDs: dùng `crds: "reuse"` sau khi kiểm tra schema/version và
operator compatibility. Script kiểm tra một số API v1 bắt buộc, không chứng nhận
đủ compatibility. Không force-upgrade CRDs. Operator có sẵn phải watch namespace
mới; Prometheus có sẵn cần selectors/scrape/rules phù hợp — reuse không sửa chúng.
KPS vẫn tạo một số scrape resources theo values: review rendered manifests.
Khi không cài Prometheus, bộ cài tắt default cluster rules và default Prometheus
datasource của Grafana để không trỏ tới service không tồn tại; cấu hình datasource
cho Prometheus reuse trong company values trước khi nghiệm thu kết nối.

## Chạy lại và lỗi giữa chừng

State cần giữ cùng bản backup:

```text
/private/obs-state/
├── state.json           # Cluster/namespace UID, ownership, hashes, intent/done
├── plan.json
├── rendered/            # Có thể chứa Secrets; không commit/chia sẻ
├── *.tgz                # Chart cache đã kiểm checksum
├── error.*.log          # Giữ riêng mỗi lỗi; không mất khi plan/verify lại
├── last-error.log       # Lỗi gần nhất, có thể chứa thông tin nhạy cảm
└── last-helm.log
```

State directory phải mới/rỗng hoặc do installer tạo, owner-only, ngoài repo/bundle.
Không dùng `/tmp`, home hoặc một thư mục dữ liệu đang có. Một state directory chỉ
cho một process; dùng một writer duy nhất cho cùng installation/release.

- Cùng inputs, live Helm values/chart không đổi và receipt hợp lệ: **NOOP**, không
  gọi install/upgrade. Readiness vẫn được kiểm tra.
- Thay đổi values: chỉ upgrade release có cả receipt và Helm ownership label.
- Có drift live Helm values/chart, namespace/reused UID thay đổi, thiếu receipt,
  release failed/pending hoặc outcome không rõ: dừng để đối soát.
- Intent được ghi trước Helm, giữ receipt hoàn tất trước đó trong `previous` khi upgrade. Không tự retry/reset sau lỗi; giữ logs/receipts,
  kiểm tra `helm status/history`, tài nguyên/PVC và xác nhận outcome. Không xóa
  state để chạy lại. Operator sửa nguyên nhân và reconcile Helm release đã sở hữu.
  Sau khi release `deployed` và live values khớp chính xác saved intent, chạy
  `reconcile --config ... --state-dir ... --cluster-uid ...` để kiểm tra chart,
  ownership, values và readiness rồi hoàn tất receipt. Lệnh này chỉ đọc cluster;
  không sửa release, nhận ownership, bỏ qua drift hoặc thay inputs của intent.
- Không tự gỡ component đã quản lý hoặc orphan release khỏi config. Cần kế hoạch
  migration/removal riêng. Operator/CRD version upgrades cũng cần review riêng.
- Helm giữ kiểm tra ownership mặc định; không dùng `--take-ownership`, `--force`,
  `--replace`, `--atomic` hoặc tự xóa PVC. Xung đột object cụ thể có thể chỉ được
  Helm phát hiện khi apply; các release trước đó có thể đã cài và sẽ được giữ lại.

Installer không reconcile mọi thay đổi trực tiếp trên Kubernetes objects. Các
controllers chưa Ready làm verify fail; đây không phải bằng chứng dữ liệu đầy đủ.

## Kiểm chứng và vận hành

```sh
bash -n setup.sh test/run.sh
bash test/run.sh
```

Contract tests dùng cluster/CLI mô phỏng; không chứng minh cài thật. Các chart
pin đã được tải, kiểm SHA và lint/render với Helm thật. `verify` đọc controller
readiness, không chứng minh metrics/logs received, persistence hoặc notification.

Sau cài cần kiểm tra targets/queries Prometheus, Loki ingestion/query, Grafana
datasources, retention/PVC sau restart và receiver firing/resolved riêng. Default
Alertmanager gửi về discard; Loki ví dụ không có auth và phải giữ trong mạng
được bảo vệ, không expose public. Không mặc định bật SQL parameter logging.
Các việc này và product binding thuộc các gate tiếp theo của issue #35.

Không có auto-uninstall. Trước cleanup/upgrade: inventory, ownership/UID, backup,
shared consumers, PVC retention và CRD dependencies phải được đối soát. Xem
[operator runbook](../../orchestrator_docs/operations/observability-installation.md)
khi dùng trong repository; standalone bundle sử dụng hướng dẫn trong file này.

### Cluster identity in collected samples

Replace `CHANGE_ME_CLUSTER_UID` in the metrics and Alloy values with the selected
cluster's kube-system UID. The metrics values label kube-state-metrics and
kubelet/cAdvisor samples; Prometheus external labels alone do not scope local
queries. Reused stacks need equivalent operator-reviewed relabeling. Preserve
existing filtering/redaction rules when adding this label.

The Alloy example maps trusted Kubernetes pod labels to bounded scope labels,
keeps Pod UID as structured metadata and uses a representative redaction rule.
It does not guarantee removal of all secrets printed by an application. Helm
renders the embedded Alloy template before `alloy validate`; validate the rendered
configuration, not its Helm-escaped source.
