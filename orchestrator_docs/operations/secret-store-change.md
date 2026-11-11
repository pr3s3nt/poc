---
id: RUNBOOK-SECRET-STORE-CHANGE
artifact: operations-runbook
status: current
last_reviewed: 2026-11-10
related: [ADR-042, UC-12]
---

# Đổi Secret Store của một Environment

Theo [ADR-042](../architecture/decisions/ADR-042-environment-bundle-versioned-secrets.md),
kho secret khóa theo Environment. Ai có quyền với Application (kể cả Developer)
chọn kho một lần khi Environment chưa có kho; đã chọn thì chỉ PE/Admin đổi, và
chỉ khi Environment không còn workload đang chạy.

## Chọn kho lần đầu

Mở Application → Settings → tab Environment → **Kho bí mật**, chọn kho Sẵn sàng
rồi bấm **Lưu kho bí mật**; hoặc chọn ngay trong hộp Tạo bí mật của form workload.

## Đổi kho

Điều kiện: Environment không còn workload đang chạy; nếu còn, Console trả lỗi
`SECRET_STORE_LOCKED` và không đổi gì.

1. Báo Developer dừng Deploy trên Environment, thống nhất thời gian gián đoạn.
2. Developer xóa mọi workload của Environment bằng Preview → Deploy, hoặc xóa
   Application nếu không giữ lại.
3. PE/Admin mở Settings và chọn kho mới. Hệ thống chép giá trị Secret UC-12 mong
   muốn sang kho mới, kiểm tra rồi mới lưu lựa chọn; lỗi thì giữ kho cũ.
4. Developer thêm lại workload và Deploy. Path workload ở kho mới bắt đầu từ
   version 1.
5. Giá trị trong kho cũ không bị xóa tự động. Khi không còn cần cho lịch sử, PE
   xóa path `orchestrator/apps/<app>/envs/<env>/values/` ở kho cũ bằng Vault CLI
   với token có quyền quản trị kho đó.

Không sửa trực tiếp lựa chọn kho trong PostgreSQL. Không đổi kho khi Environment
còn workload: VSS và Secret đích của kho cũ vẫn đang được pod dùng.
