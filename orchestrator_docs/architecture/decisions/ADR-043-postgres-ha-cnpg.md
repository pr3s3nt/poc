---
id: ADR-043
artifact: architecture-decision
status: current
last_reviewed: 2026-11-11
related: [ADR-014, UC-03]
---

# ADR-043 — PostgreSQL HA bằng CloudNativePG

Accepted: 2026-11-11, issue #75 đã được người dùng duyệt.

## Quyết định

Dependency `postgres` class `ha` dùng CloudNativePG (CNPG) **1.30.1**,
bản stable mới nhất trên GitHub khi thực hiện. Operator được cài riêng theo
[runbook](../../operations/postgres-ha-cnpg.md), không nằm trong deploy app;
quản lý add-on tự động thuộc issue #74.

Module phát triển trực tiếp tại repo phát hành
`pr3s3nt/orchestrator-workload-templates`, thư mục `terraform/postgres-ha`.
Terraform quản lý Secret basic-auth và `postgresql.cnpg.io/v1 Cluster`;
operator quản lý primary, standby, failover và các Pod/PVC/Service con.
Mặc định hai instance; validation yêu cầu số nguyên ít nhất hai. Module chờ
condition `Ready=True` trước khi trả output; timeout create/update 15 phút.

Giữ tám output `host`, `port`, `database`, `username`, `password`, `uri`,
`jdbcUrl`, `asyncUri`; host dùng Service `<cluster>-rw.<namespace>.svc`.
Password và hai URI chứa password giữ sensitive; username/password bootstrap
lấy từ Secret do executor cung cấp, không ghi literal trong Definition.

CNPG dùng image PostgreSQL riêng của operator, không dùng official Alpine
image của module đơn instance. Chọn dòng PostgreSQL **16.14-system-trixie**
cùng major 16 với module cũ, pin digest công bố ở README module. Đây là
ngoại lệ cho mặc định `image` theo quyết định chọn image CNPG trong issue #75;
các tên/kiểu/mặc định biến khác giữ nguyên, thêm `instances=2`.

Cleanup chỉ nhận CNPG Cluster đúng API version/kind, tên, namespace và UID
trong Terraform state (computed `object`, không chỉ manifest mong muốn).
Object con phải có chuỗi ownerReference khớp từng kind/name/UID tới Cluster
đó. Không nhận theo label, prefix hoặc tên giống nhau. Manifest loại khác,
owner sai UID, thiếu bằng chứng và storage không có owner vẫn bị từ chối.
Khi namespace đã mất, truy vấn chính xác `clusters.postgresql.cnpg.io` để
chứng minh Cluster không còn; lỗi API không phải bằng chứng vắng mặt.

## Giới hạn

Một node kind cho phép kiểm chứng failover tiến trình/Pod, không chứng minh
HA khi mất node. Không backup/PITR, pooler, đa node, Aurora hoặc HA MySQL/Redis.
Class `in-cluster` và StatefulSet workload một replica không đổi.
Nghiệm thu dùng commit SHA; chỉ merge và tag `v0.6.0` sau người dùng nghiệm thu.

## Nguồn

- [CNPG release 1.30.1](https://github.com/cloudnative-pg/cloudnative-pg/releases/tag/v1.30.1).
- [Image mặc định của release](https://github.com/cloudnative-pg/cloudnative-pg/blob/v1.30.1/pkg/versions/versions.go).
