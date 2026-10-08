---
id: PROJECT-GLOSSARY
artifact: glossary
status: current
last_reviewed: 2026-09-23
---

# Project glossary

| Term | Meaning in this project |
|---|---|
| Organization | Biên sở hữu Application, Resource Type, Resource Definition và Connection. |
| User Account | Internal account thuộc một Organization, có username, password hash, role và status. |
| Session | Opaque authenticated session gắn với User Account; database chỉ lưu token hash. |
| Application | Đơn vị ứng dụng sở hữu identity; lựa chọn nơi triển khai và kho secret thuộc Environment. |
| Environment | Môi trường thuộc Application, có versioned execution/secret-store selections, target generation, current Deployment Set và namespace riêng. |
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

## Environment destinations (ADR-012)

| Term | Meaning |
|---|---|
| Secret Store Connection | Kho Vault KV v2 thuộc Organization, có credential ref riêng; Environment chọn độc lập với Deployment Connection. |
| Environment Operation | Persisted owner/stage claim bảo vệ admission và migration; không phải khóa lựa chọn vĩnh viễn. |
