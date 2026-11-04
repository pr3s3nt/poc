# mysql — Kubernetes resource

Xem [database setup và contract](../../bootstrap/DATABASES.md).
Module dùng image digest-pinned, Secret có sẵn, PVC riêng và internal Service.
Không apply ngoài target được cấp quyền; không đọc mật khẩu vào Terraform.
