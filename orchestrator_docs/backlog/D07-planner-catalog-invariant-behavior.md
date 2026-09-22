---
id: D07
artifact: backlog-item
status: deferred
last_reviewed: 2026-09-22
---

# D07 — Validation boundary cho Resource Definition catalog

UC-03 BR-07 đã chốt product registration phải từ chối Definition thiếu Matching
Criteria trước persistence. Registration chưa được hiện thực và catalog hiện
đến từ seed/store. `loadCatalog` nạp toàn bộ Definitions; sau đó
`newDefinitionMatcher` validate toàn bộ catalog trước khi match graph. Vì vậy
một Definition không hợp lệ làm fail lần planning, kể cả khi Definition đó
thuộc Resource Type workload không dùng
(`backend/internal/planning/match.go`,
`backend/internal/domain/resource/definition.go`,
`backend/internal/application/deployment/service.go`).

Đây là quyết định về integrity/resilience của product catalog, độc lập với
conformance adapter. I06-05 đã đóng IMP-010: adapter bỏ external Definition
thiếu/`[]` khỏi challenge catalog thay vì biến chúng thành wildcard `{}`.

## Deferred decision

Chọn và tài liệu hóa validation boundary:

- validate toàn catalog tại registration, seed/bootstrap hoặc repository load;
  catalog hỏng làm service không sẵn sàng, planner vẫn giữ validation phòng thủ;
- hoặc để planner fail từng request khi toàn catalog có bất kỳ Definition không
  hợp lệ, như behavior code hiện tại;
- hoặc repository/planner chỉ load và validate candidate Definitions cần cho
  graph, kể cả các type được khám phá khi fixed-point expansion.

Hướng ưu tiên để đánh giá là fail sớm tại write/bootstrap/load, giữ defensive
validation trong planner và không để lỗi catalog xuất hiện như lỗi ngẫu nhiên
của một deployment. Đây chưa là accepted architecture cho tới khi quyết định
được duyệt.

Khi chốt, cập nhật UC-05/06 preconditions, catalog repository/bootstrap,
CURRENT_STATE và tests cho invalid seed, corrupt persisted catalog và
unrelated Resource Type. Không thay đổi conformance adapter semantics đã chốt
tại I06-05.
