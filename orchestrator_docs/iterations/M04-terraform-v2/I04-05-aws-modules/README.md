---
id: I04-05
artifact: iteration-plan
status: deferred
last_reviewed: 2026-10-13
related: [M04, ADR-014, IMP-021]
---

# I04-05 — Module VPC, EKS, ElastiCache và Aurora

## Chặng và trạng thái

Chặng 2; xem [M04](../README.md). Chưa bắt đầu; không suy status này thành task bị bỏ hoặc đã implement.

## Phạm vi

Chuẩn bị immutable AWS modules/network/access, shared account/region contract, small compatible sizing và cleanup policy. Chặng 2 không apply AWS.

## Nguồn chuẩn

- [architecture/contracts/terraform-modules.md](../../../architecture/contracts/terraform-modules.md)
- [architecture/connection-credentials.md](../../../architecture/connection-credentials.md)
- [operations/aws.md](../../../operations/aws.md)

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

Pins/validate/contract tests đạt, private network EKS->database/cache rõ, không LB/public DB không cần; plan allowlist/counts và cost inputs có thể review.

## Checks

- terraform init lock readonly, terraform validate từng AWS module
- `cd backend && REQUIRE_POSTGRES=1 go test ./... && go build ./...`
- Read-only AWS/pricing preparation khi cần, không create/apply/destroy account resources

## Handoff

Codex giao Claude source/scope/criteria/checks qua clauded trong tmux task riêng
khi code stage được duyệt. Claude không sửa docs/commit/push. Codex review và
verify độc lập; kiểm tiến độ bình thường mỗi 240 giây, không poll liên tục.
Evidence thật/fake và checks skipped phải phân biệt; failed video/phase marks
giữ lại. Code-map/CURRENT_STATE/traceability chỉ ghi actual results sau delivery.
