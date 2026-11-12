# mongodb — Kubernetes resource

Xem [Type, Definition và contract](../../resource-catalog/mongodb/README.md).
Module dùng image digest-pinned, mật khẩu do executor sinh (Secret riêng của
instance), PVC riêng và internal Service. Xuất `password` và chuỗi kết nối
bí mật (issue #52). Không apply ngoài target được cấp quyền.

Type chỉ nhận `database` bắt buộc, `username`/`storage` tuỳ chọn. Module mặc
định username `app`, storage `10Gi`; Definition quản lý image và placement.
Sáu outputs: `host`, `port`, `database`, `username`, `password`, `connection`.
