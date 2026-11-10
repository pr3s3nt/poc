---
id: RUNBOOK-SECRET-STORE-CHANGE
artifact: operations-runbook
status: current
last_reviewed: 2026-11-10
related: [ADR-042, UC-12]
---

# Đổi Secret Store của một Environment

Theo [ADR-042](../architecture/decisions/ADR-042-environment-bundle-versioned-secrets.md),
kho secret khóa theo Environment. Developer không chọn hoặc đổi kho. PE/Admin chọn
kho một lần và chỉ đổi khi Environment không còn workload đang chạy.

## Chọn kho lần đầu

1. Đăng nhập Console bằng tài khoản PE hoặc Admin.
2. Mở Application → Settings → tab Environment cần chọn.
3. Trong mục **Kho bí mật**, chọn kho Sẵn sàng rồi bấm **Lưu kho bí mật**.

Sau khi lưu, Developer tạo Secret và Deploy bình thường.

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
