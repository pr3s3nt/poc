---
id: UC-17-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-23
---

# UC-17 — Use Case Realization

Truy vết [specification](specification.md). HTTP mapping thuộc
[OC-20](../../architecture/contracts/operation-contracts.md#oc-20-effectivenessservicereport--records--baselines).

## Responsibilities

- **Effectiveness page (boundary):** chọn kỳ và phạm vi Toàn tổ chức →
  Application → Workload, hiển thị danh mục chỉ số, phân bố, xu hướng; mở chi
  tiết chỉ số với bản ghi nguồn; xuất CSV từ dữ liệu server đã tính; form
  baseline.
- **EffectivenessService (control):** xác thực PE/Admin và Organization, chuẩn
  hóa kỳ theo timezone báo cáo, đọc nguồn qua ports, gọi bộ dựng công việc và
  danh mục chỉ số, chọn phiên bản baseline áp dụng.
- **WorkItemBuilder (control):** dựng công việc logic từ lượt chạy theo khóa
  chuỗi; thuần, không I/O.
- **MetricCatalog (control):** danh sách mục chỉ số có ID ổn định; mỗi mục có
  bộ tính hoặc không (`NOT_COLLECTED`). Thêm nguồn mới = thêm bộ tính.
- **Run sources (interfaces):** Deployment history (mọi store), cleanup history
  (chỉ PostgreSQL; vắng mặt thì báo nguồn không khả dụng trong độ phủ), User
  Account lookup.
- **BaselineRepository (interface):** append-only phiên bản baseline với CAS
  phiên bản; memory/JSON và PostgreSQL.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Page gọi `GET /effectiveness/report` không tham số phạm vi; service mặc định Toàn tổ chức, 30 ngày. |
| MS-02 | Service đọc Deployment, cleanup history, Application, User Account của Organization trong một read snapshot. |
| MS-03 | WorkItemBuilder dựng chuỗi theo `(Environment, workload, action)` và `(Environment, root resource)`. |
| MS-04 | MetricCatalog tính từng mục cho phạm vi, kèm tử số/mẫu số/độ phủ/status. |
| MS-05 | Service thêm phân bố phạm vi con và xu hướng theo ngày/tuần. |
| MS-06 | Page gọi lại với `application`/`workload`. |
| MS-07 | Page gọi `GET /effectiveness/records?metric=`; mỗi dòng có Deployment ID hoặc cleanup operation để mở UC-09. |
| MS-08 | Page tạo CSV từ report + records của cùng tham số; không tính lại chỉ số. |
| ALT-01 | Page gửi `POST /effectiveness/baselines` với `expectedVersion`; repository chèn version+1 hoặc trả conflict. |

## Boundary

Không gọi executor, provider, cluster hoặc Terraform state. Không thêm bảng sự
kiện song song: lịch sử thực thi hiện có là nguồn sự kiện. Response chỉ mang ID,
key, action, status, timestamp và username/actor; không resolved inputs, outputs,
failure reason hoặc target.
