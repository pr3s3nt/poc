---
id: RUNBOOK-POSTGRES-HA-CNPG
artifact: operations-runbook
status: current
last_reviewed: 2026-11-11
---

# PostgreSQL HA trên kind bằng CNPG

Theo [ADR-043](../architecture/decisions/ADR-043-postgres-ha-cnpg.md).
Chỉ thực hiện trên context `kind-k8s-4f` theo quyền issue #75.
Không đổi current context. Không cài vào cụm khác khi chưa có quyền riêng.

## Cài và kiểm tra

```bash
kubectl --context kind-k8s-4f apply --server-side -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/v1.30.1/releases/cnpg-1.30.1.yaml
kubectl --context kind-k8s-4f wait --for=condition=Established crd/clusters.postgresql.cnpg.io --timeout=120s
kubectl --context kind-k8s-4f -n cnpg-system rollout status deployment/cnpg-controller-manager --timeout=180s
```

Ghi version, CRD Established và deployment Available vào verification.
Kiểm tra operator/webhook trước khi inspect Terraform manifest: provider cần
CRD thật để plan. Operator quản lý ngoài state ứng dụng và phải được giữ chạy
cho tới khi mọi Cluster/Pod/PVC thử đã dọn xong.

## Definition và nghiệm thu

Tạo trên Console `staging-k8s-4f-postgres-ha`, Type `postgres`, driver Terraform,
Git repo phát hành, path `terraform/postgres-ha`, ref commit SHA đã kiểm tra.
Resource Type `postgres` phải khai báo `asyncUri` tùy chọn, string, secret
(người dùng duyệt 2026-11-12); nếu thiếu, thêm qua UI mở rộng output UC-02,
không sửa/xóa output cũ. Sao variables/input_mapping/output_mapping/provider_bindings từ Definition
`in-cluster`, đổi criteria thành `{env_type: staging, class: ha}`; giữ đủ tám
output. Không sửa Definition nguồn, không lưu Definition/credential vào Git.

Tạo Application/Environment riêng có run ID; workload dùng dependency postgres
class `ha`. Preview phải chọn đúng Definition. Deploy chờ hai instance Ready;
ghi currentPrimary, UID Cluster và PVC. Dùng app ghi/đọc qua cả host và URI,
ghi dữ liệu kiểm thử trước failover. Xóa đúng Pod primary của Cluster thử,
chờ standby thành primary và endpoint Service `-rw` chuyển, đo thời gian app
đọc/ghi trở lại (~60 giây) và xác minh dữ liệu cũ. Redeploy không đổi phải giữ
UID Cluster và dữ liệu. Kiểm tra class `in-cluster` trên tài nguyên thử riêng.

Xóa Environment hoặc gỡ dependency qua product; xác minh Cluster, Pod, PVC,
Secret và namespace thử không còn. Lỗi cleanup phải điều tra ownership/state,
không bypass guard hoặc xóa object không thuộc task. Chỉ kiểm localhost hoặc
HTML shell không đủ: mở Console qua URL đã xác minh theo
[Tailscale handoff](tailscale-handoff.md). Không ghi URL private vào issue public.
Full test theo [kind runbook](kind.md), namespace riêng và xác minh cleanup.

## Gỡ operator

Chỉ gỡ khi đã liệt kê Cluster trên toàn cụm và không còn Cluster nào; không
xóa CRD khi còn dữ liệu. Không gỡ operator do tác vụ khác sử dụng.

```bash
kubectl --context kind-k8s-4f get clusters.postgresql.cnpg.io -A
# Chỉ chạy khi danh sách trống và operator thuộc task:
kubectl --context kind-k8s-4f delete -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/v1.30.1/releases/cnpg-1.30.1.yaml
```

Sau nghiệm thu mới merge/tag `v0.6.0`, chuyển Definition sang tag và kiểm tra
pin đã resolve đúng commit nghiệm thu. Không tự nâng version operator/image.
