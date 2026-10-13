---
id: I04-04
artifact: iteration-plan
status: current
last_reviewed: 2026-10-13
related: [M04, ADR-014, IMP-021]
---

# I04-04 — Module namespace, Redis và PostgreSQL

## Chặng và trạng thái

Chặng 2; bắt đầu sau I04-03 đã qua review và validation. Xem
[evidence executor](../../../verification/2026-10-13-i04-03-terraform-executor.md).
Chưa có kết quả module/live acceptance cho task này.

## Phạm vi

Chuẩn bị packaged/allowlisted immutable Kubernetes modules, input/output mappings/auth/Secret/storage/readiness; bind k8s-4f hoặc EKS target.

## Nguồn chuẩn

- [architecture/contracts/terraform-modules.md](../../../architecture/contracts/terraform-modules.md)
- [usecase/UC-08/specification.md](../../../usecase/UC-08/specification.md)
- [operations/terraform-v2-acceptance.md](../../../operations/terraform-v2-acceptance.md)

## Code impact để giao Claude

- [backend/internal/adapters/terraform/modules](../../../../backend/internal/adapters/terraform/modules/)
- [backend/internal/seed](../../../../backend/internal/seed/)

New state/cleanup ports/adapters được đặt theo dependency rule của project;
không copy code từ orchestrator_reference. Exact new paths cập nhật code map
sau implement, không tạo link tới file chưa có.

## Dependencies và quyền

Sau gate 1 và các task trước trong bảng M04; task registry/executor/modules có thể ghép batch khi dependency đã đủ.
Chỉ các môi trường và mutation trong chặng hiện tại được giao. Không reset
retained database, sửa cluster-wide component hoặc tạo AWS ngoài gate.
Module/provider pins, deadline và small sizing ghi trước execution.

## Acceptance criteria

Contracts/common Type outputs hợp lệ; stable identity và dependencies namespace->cluster; test modules đủ readiness và fault image cho E02. Không cài cluster-wide component ngoài quyền.

## Checks

- terraform init với provider lock readonly, terraform validate cho từng module
- `cd backend && REQUIRE_POSTGRES=1 REQUIRE_TERRAFORM=1 go test ./... && go build ./...`

## Handoff

Codex giao Claude source/scope/criteria/checks qua clauded trong tmux task riêng
khi code stage được duyệt. Claude không sửa docs/commit/push. Codex review và
verify độc lập; kiểm tiến độ bình thường mỗi 240 giây, không poll liên tục.
Evidence thật/fake và checks skipped phải phân biệt; failed video/phase marks
giữ lại. Code-map/CURRENT_STATE/traceability chỉ ghi actual results sau delivery.
