# UC-07 — Update or Remove Workload

## Mục tiêu

Cập nhật hoặc xóa một workload trong Environment mà vẫn giữ nguyên các workload và resource không bị ảnh hưởng.

## Primary actors

- Developer
- CI/CD System

## Supporting actors

- Resource Driver
- Kubernetes Cluster/Operator

## Tiền điều kiện

- **PRE-01:** Application, Environment và workload đã tồn tại.
- **PRE-02:** Score `before` khớp workload trong current Deployment Set.
- **PRE-03:** Environment đã được kết nối với deployment target.
- **PRE-04:** Active Resource state của deployment trước có thể truy vấn được.

## Trigger

**TRG-01:** Developer hoặc CI/CD gửi Score mới để cập nhật workload, hoặc yêu cầu xóa workload hiện có.

## Main success scenario

1. **MS-01:** Orchestrator đọc current Deployment Set và cấu hình workload trước/sau thay đổi.
2. **MS-02:** Orchestrator xác nhận trạng thái `before` khớp contribution hiện tại của workload.
3. **MS-03:** Orchestrator tạo Delta chỉ thay đổi contribution của workload mục tiêu và dựng Candidate Deployment Set.
4. **MS-04:** Orchestrator dựng lại desired Resource Graph và phân loại Active Resources.
5. **MS-05:** UC-07 `«include»` UC-08 để provision/reconcile resource còn được yêu cầu theo dependency order.
6. **MS-06:** Orchestrator cập nhật hoặc xóa Kubernetes workload manifests theo VAR-01/VAR-02.
7. **MS-07:** Orchestrator đánh dấu resource không còn được graph tham chiếu là `unreferenced`.
8. **MS-08:** Orchestrator lưu Candidate Deployment Set thành current và deployment status `SUCCEEDED`.

## Luồng biến thể trong happy path

- **VAR-01 — Update:** `after Score` tồn tại; workload manifests được render/apply lại từ Candidate Deployment Set.
- **VAR-02 — Remove:** `after Score` là `null`; workload manifests bị xóa và resource chỉ được đánh dấu `unreferenced`, không tự destroy.

## Hậu điều kiện

- **POST-01:** Workload được cập nhật hoặc không còn trong Environment.
- **POST-02:** Các workload khác không thay đổi.
- **POST-03:** Shared resource vẫn còn nếu vẫn được workload khác sử dụng.
- **POST-04:** Resource không còn được tham chiếu ở trạng thái `unreferenced` và chưa bị destroy.
- **POST-05:** Candidate Deployment Set trở thành current Deployment Set và deployment có trạng thái `SUCCEEDED`.

## Quy tắc nghiệp vụ

- **BR-01:** `before Score` phải deep-equal contribution hiện tại của workload trước khi tạo Delta, gồm cả module contribution lẫn từng shared entry mà `before Score` khai báo.
- **BR-02:** Delta không được thay đổi module/shared contribution ngoài phạm vi workload, trừ shared entry do chính workload thêm/bỏ và không xung đột consumer khác.
- **BR-03:** Loại dependency không đồng nghĩa với deprovision resource.
- **BR-04:** Resource có cùng descriptor được xem xét tái sử dụng qua UC-08.
- **BR-05:** Nếu workload khai báo một shared ID đã tồn tại với type/class/params khác, và chính workload đó chưa khai báo shared ID này trong `before Score`, planning phải từ chối vì xung đột thay vì ghi đè.
- **BR-06:** Khi workload thôi khai báo một shared resource, entry chỉ rời Deployment Set nếu không còn module nào tham chiếu nó. Đây là phần mở rộng so với planner challenge, nơi entry bị xóa ngay theo khai báo của workload.

## Luồng nội bộ

```text
UC-07 Update or Remove Workload
├── Verify current workload state
├── Build scoped Deployment Delta
├── Rebuild desired Resource Graph
├── Reconcile required resources
├── Update Kubernetes workload
├── Classify unreferenced resources
└── Persist new Deployment Set
```

## Trạng thái implementation hiện tại

- Planner đã validate module và từng shared entry trong `before Score`, từ chối shared conflict, tạo Delta/Candidate Set và giữ nguyên các module khác.
- Shared entry chỉ bị loại khi workload thôi khai báo và không còn module khác tham chiếu; planner đã phân loại Active Resource thành `existing`, `new` và `unreferenced`.
- Runtime update/delete workload, reconcile state và API/UI cho UC-07 chưa được wire; phần này vẫn thuộc Phase 6 bước 7.

## Ngoài phạm vi happy path

- **OOS-01:** Concurrent workload updates.
- **OOS-02:** Automatic rollback.
- **OOS-03:** Scheduled deletion và deprovision resource.
- **OOS-04:** Recovery khi update Kubernetes thất bại.
- **OOS-05:** Canary, blue/green hoặc progressive delivery.
