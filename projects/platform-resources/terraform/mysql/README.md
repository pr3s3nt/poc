# mysql — Kubernetes resource

Xem [database setup và contract](../../bootstrap/DATABASES.md).
Module dùng image digest-pinned, mật khẩu do executor sinh (Secret riêng của
instance), PVC riêng và internal Service. Xuất `password` và chuỗi kết nối
bí mật (issue #52). Không apply ngoài target được cấp quyền.
