---
id: PROJECT-GLOSSARY
artifact: glossary
status: current
last_reviewed: 2026-10-10
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

## Nhãn Web Console (VI-01)

Bảng dưới đây sở hữu nhãn Console; domain terms và contract kỹ thuật ở trên
không đổi. Nhãn không đổi API enum hoặc matching semantics.

| Khái niệm/giá trị | Nhãn Console | Quy tắc |
|---|---|---|
| Application | Ứng dụng | Name/ID người dùng giữ nguyên. |
| Environment | Môi trường | Tab và giá trị vẫn `staging`, `production`. |
| Resource Type | Loại tài nguyên | Kèm Resource Type trong trợ giúp nếu cần tra contract. |
| Resource Definition | Cấu hình tài nguyên | Mô tả: cách tạo tài nguyên và điều kiện áp dụng. |
| Connection | Kết nối | Settings dùng “Kết nối triển khai”; account riêng dùng “Kết nối AWS”. |
| Secret Store | Kho bí mật | Không gọi là kho mật khẩu. |
| Execution profile | Phạm vi triển khai | Phân biệt với Environment type và criteria. |
| `internal-k8s` | Cluster nội bộ | Tooltip: cluster Kubernetes đã kết nối; không suy vị trí vật lý. |
| `aws-eks` | AWS | Có thể thêm “tạo EKS” trong hướng dẫn về Environment. |
| profile rỗng | Dùng chung | Giải thích: xét ở mọi profile, vẫn phải thỏa điều kiện áp dụng. |
| Terraform | Tạo trên AWS (Terraform) | Payload driver `terraform` giữ nguyên. |
| Kubernetes | Tạo trong cluster (Kubernetes) | Payload driver `kubernetes` giữ nguyên. |
| score-k8s UI entry | Mẫu dựng ứng dụng | Technical driver/bundle ID giữ nguyên. |
| Matching criteria | Điều kiện áp dụng | Modes: Mọi nơi; Theo loại môi trường; Theo ứng dụng; Tùy chỉnh nâng cao. |
| Driver variables | Tham số cấu hình | Không dịch JSON keys hoặc placeholder. |
| Provision rules | Quy tắc tạo tài nguyên liên quan | Không gọi chung là phụ thuộc vì hướng dependency có thể khác. |
| Workload / Container / Service | Workload / Container / Service | Giữ thuật ngữ kỹ thuật; hướng dẫn tiếng Việt. |
| Preview / Deploy | Xem trước / Triển khai | Không tự deploy khi lưu draft. |
| Variable / Secret | Biến / Bí mật | Giá trị Secret không được hiển thị lại. |
| READY / PENDING / FAILED / SUCCEEDED | Sẵn sàng / Đang chờ / Thất bại / Thành công | Chỉ dịch nhãn; enum/API giữ nguyên, ID nhỏ có thể hiện bên cạnh. |
| UNCONFIGURED / UNREFERENCED | Chưa cấu hình / Không còn được tham chiếu | Không diễn giải UNREFERENCED là đã xóa. |
| Platform Engineer / Developer / Admin | Kỹ sư nền tảng / Nhà phát triển / Quản trị viên | Role IDs không đổi. |

Text tĩnh, validation FE, aria-label, empty/loading/error states, notification và
thông báo trợ giúp đều dùng tiếng Việt. Tên do người dùng đặt, image, descriptor,
module, key, driver ID, JSON, URLs và giá trị API không dịch. Không dùng regex
thay thế từ trong raw backend errors để Việt hóa.
