---
id: UC-04-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-09-29
related: UC-04
---

# UC-04 UI screens

`Platform / Connections` liệt kê connection ID, kind, cluster ID, kube context
và trạng thái trong Organization. Form `Register Kubernetes cluster` có đúng
ba ô: connection ID, cluster ID, kube context. Ghi chú cho biết context phải
được cấu hình trước trên máy backend và backend sẽ kiểm tra API/quyền.
Không có credential text area, upload kubeconfig hoặc AWS form trong MVP.
