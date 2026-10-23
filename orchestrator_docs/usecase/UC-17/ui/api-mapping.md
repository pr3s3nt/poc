---
id: UC-17-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-10-23
---

# UC-17 API mapping

Contract: [OC-20](../../../architecture/contracts/operation-contracts.md#oc-20-effectivenessservicereport--records--baselines).

| UI action | Request | Ghi chú |
|---|---|---|
| Mở trang / đổi phạm vi / đổi kỳ | `GET /api/v1/effectiveness/report?from&to&application&workload` | Danh sách Application cho select lấy từ `GET /api/v1/applications` đã tải ở shell. |
| Danh sách Workload | `breakdown.rows` của report cấp Application | Workload chỉ xuất hiện khi có lịch sử. |
| Mở chi tiết chỉ số | `GET /api/v1/effectiveness/records?metric=<id>&…` | Không gọi khi `NOT_COLLECTED`. |
| “Mở” một bản ghi | Route `deployment` (UC-09) với `lastRunId`, hoặc route `resource` với `resourceId` | Không có lịch sử song song. |
| Xuất CSV | report hiện có + `GET /api/v1/effectiveness/records?…` không `metric` | Chỉ định dạng, không tính lại. |
| Mở “Mốc so sánh” | `GET /api/v1/effectiveness/baselines?application=` | |
| Lưu / Ngừng áp dụng | `POST /api/v1/effectiveness/baselines` với `expectedVersion=currentVersion` | Sau `201` tải lại report. |
