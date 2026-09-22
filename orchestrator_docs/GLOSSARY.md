---
id: PROJECT-GLOSSARY
artifact: glossary
status: current
last_reviewed: 2026-09-22
---

# Project glossary

| Term | Meaning in this project |
|---|---|
| Organization | Biên sở hữu Application, Resource Type, Resource Definition và Connection. |
| Application | Đơn vị ứng dụng sở hữu một Execution Profile cố định; với `aws-eks`, đây cũng là scope VPC/EKS. |
| Environment | Môi trường thuộc một Application, có current Deployment Set và namespace identity riêng. |
| Execution Profile | Chính sách `aws-eks` hoặc `internal-k8s`, quyết định target và tập Definition phù hợp. |
| Connection | Metadata và secret reference dùng để truy cập AWS identity hoặc Kubernetes target đã đăng ký. |
| Score | Tài liệu khai báo desired state của đúng một workload. |
| Deployment | Một lần plan và execute thay đổi cho một Environment. |
| Deployment Set | Desired-state snapshot đầy đủ của modules và shared resources trong Environment. |
| Candidate Deployment Set | Deployment Set sau khi áp delta, chưa trở thành current cho tới khi execution thành công. |
| Deployment Delta | Humanitec artifact có identity/lifecycle riêng; mutable cho tới khi archive. Public-compatible lifecycle này đang deferred ở D05. |
| Deployment Delta Snapshot | Tài liệu bất biến gắn với một Deployment MVP, mô tả thay đổi từ base Set sang Candidate Set bằng Humanitec-shaped Delta document. |
| Resource Type | Contract input/output độc lập implementation của một loại resource. |
| Resource Definition | Cách hiện thực Resource Type, gồm driver, inputs, matching criteria và provision rules. |
| Resource Descriptor | Identity `type.class#res_id` dùng làm graph node và resource identity. |
| Resource Graph | DAG có edge `consumer -> provider`; provider được provision trước consumer. |
| Active Resource | Resource instance đã được ghi nhận cùng descriptor, Definition, scope, state và outputs. |
| Workload | Module được chuyển từ một Score và triển khai lên Kubernetes. |
| Workload Instance | Bản ghi runtime của workload thuộc một Deployment. |
| Orchestrator Web Console | React admin UI nằm trong root `frontend/`, gọi Go API. |
| Acceptance application frontend | Workload mẫu dưới `backend/examples/acceptance-app/`; không phải Web Console. |
| Verification record | Observation bất biến của một lần test cụ thể; không định nghĩa requirement hiện tại. |
