---
id: TERRAFORM-MODULE-CONTRACT
artifact: operation-contract
status: current
last_reviewed: 2026-10-13
related: [ADR-014, UC-03, UC-05, UC-06, UC-08, UC-16]
---

# Terraform v2 — module và provider contract

[ADR-014](../decisions/ADR-014-terraform-v2-lifecycle.md) sở hữu quyết định.
UC-03 sở hữu registration behavior; tài liệu này sở hữu shared module contract.
Các shape dưới đây là contract v2 cần implement, không phải API đã có.

## Module registration và release

Definition driver `terraform` tham chiếu module bằng `module_id`, nguồn
`source_id`, `revision`, `subdirectory`, `content_sha256`, input mapping,
output mapping và provider bindings. Module ID và version/digest là metadata
công khai có kiểm soát; credentials và đường dẫn cache nội bộ không công khai.

Admin allowlist sources là cấu hình platform được review; PE không tự thêm trust
root qua form. Initial source kinds: packaged platform modules hoặc HTTPS Git
repository allowlisted với full commit. Artifact package phải có SHA-256 cố
định. Git phải verify commit và digest nội dung; path tương đối nằm trong source,
không symlink/path traversal ra workspace. Không nhận credential trong URL,
SSH/exec downloader, arbitrary local path, module provisioner `local-exec`/
`remote-exec` hoặc executable hook. Provider binaries chỉ từ registry/mirror
platform tin cậy. Module download credential, nếu cần, thuộc platform store.

Fetch/inspect ở registration hoặc platform preparation, không ở pure planner.
Materialize cache theo digest ngoài repository, private invocation workspace;
verify digest lại trước execution. Artifact thiếu/drift thì dừng trước provision,
không tự lấy version mới. Transitive modules cũng phải immutable và được đưa
vào verified bundle. Provider lockfile chỉ pin providers, không thay module pin.

Release manifest chứa exact Terraform version + binary digest, provider source/
exact version + lockfile hashes, module commit/content digest và score-k8s bundle
digests. Không gọi `init -upgrade`; init dùng readonly provider lockfile. Catalog
metadata published sau inspect, gồm schemas/defaults và provider requirements.
Version literals/digests được tạo khi chuẩn bị modules, trước live run.

Public Definition registration giữ insert-only: nâng module bằng Definition ID
mới và thay criteria có specificity/context tạo unique winner đã review, không ngầm thêm
API overwrite/update Definition.
Nếu hai Definition đồng hạng thì Preview chặn; PE phải cấu hình criteria cho một
winner. State của resource hiện hữu không bị thay theo Definition ID mới.

## Driver Inputs shape

Giữ root `values`; các field hợp lệ của Terraform là `source`, `variables`,
`input_mapping`, `output_mapping`, `provider_bindings`. `source` có đúng
`source_id`, `module_id`, `revision`, `subdirectory`, `content_sha256` theo
verified registry; không nhận arbitrary URL/local path/credential.
`variables` chứa typed literals/approved placeholders và Definition fallbacks.
`input_mapping` ánh xạ tên module variable tới `{param: <Type input name>}`;
Type param có giá trị ưu tiên fallback/default của module variable tương ứng.
Không nhận arbitrary expression hoặc secret field qua param mapping.
`output_mapping` là map module output name -> Type output name.
`provider_bindings` keyed bởi approved provider requirement: AWS dùng
`{target: environment}`, Kubernetes dùng cùng target hoặc
`{target: resource, resource_ref: <cluster descriptor>}` cho EKS dependency.
Provider output reference phải tạo graph edge và resolve scoped opaque target;
không nhận account/region/endpoint/credential override. Known field/schema
validation thuộc UC-03 BR-11/12; các source pins cụ thể do registry release công bố.
Workload renderer giữ riêng `values.variables.render_bundle`, không nhận
Terraform source/mappings/provisioners.

## Inputs và mapping

Input schema có name, type (`string`, `number`, `bool`, list/map có element type),
required/default, bounds/enum/pattern nếu cần. Giữ subset validation hiện hành;
không mặc định mở toàn bộ JSON Schema. Validation module bổ sung cho Type schema,
không tự suy product Resource Type từ tên Terraform variable.

Mỗi module input có đúng một nguồn đã khai báo: resource param, Definition
fallback, context, completed provider output hoặc executor-owned secret/setting.
Thứ tự param -> Definition fallback -> module default giữ contract hiện hành;
executor-owned không được override bằng param/Definition. Namespace/name/region/
provider credentials được pin từ target/identity. Missing/wrong literal input,
unknown key và contract mismatch bị chặn trước execution. Placeholder chưa resolve
được phải type-check sau binding và trước apply của node; secret không echo lỗi.

Không cho nhập password/token/provider config qua Workload params. Password tạo
một lần theo resource identity, lưu qua secret store trước apply, retry dùng lại
opaque reference; không sinh mật khẩu khác chỉ vì run ID đổi.

## Resource Type outputs

Output mapping là tên module output -> tên Resource Type output; không phải
arbitrary expression/script. Cần đủ required outputs, đúng kiểu và secret
classification. Extra internal outputs không được tự publish. Secret flag có
hiệu lực nếu Type hoặc module đánh dấu secret; không downgrade để hiển thị.

| Type | Contract dùng cho ứng dụng |
|---|---|
| `postgres` | Giữ contract catalog hiện hành: `host` string, `port` number, `database` string, `username` string, `password` secret string |
| `redis` | `host` string, `port` number, `username` string, `password` secret string, `tls` bool; credentials không rỗng khi auth yêu cầu |

Redis v2 dùng auth cho cả in-cluster/managed; in-cluster có thể không TLS,
ElastiCache dùng TLS và app phải hỗ trợ theo output `tls`. Module sinh username/
password phù hợp implementation, không hard-code credential vào app. PostgreSQL
giữ database/username params hiện hành; Redis cung cấp class và binding outputs,
không thêm user-facing module selector. Sizing nằm trong module schema/Definition;
chỉ expose param nghiệp vụ đã được Resource Type đăng ký, không expose mọi AWS knob.

VPC/EKS/namespace giữ các output contract hiện hành; EKS target là scoped opaque
identity đủ tái tạo credential, không persist kubeconfig/temp path. Resource
không Ready hoặc output sai contract thì không render/apply consumer.

## Provider bindings và dependency

| Module | Provider source | Required dependency |
|---|---|---|
| VPC | Environment AWS Connection + region | Không có |
| EKS | Cùng AWS Connection + region | VPC/mạng/access configuration |
| Namespace k8s-4f | Environment Kubernetes Connection | Trusted existing cluster binding |
| Namespace EKS | EKS target từ completed node | EKS Ready và access hợp lệ |
| Redis/PostgreSQL in-cluster | Cùng Kubernetes target | Namespace + storage/readiness capability cần thiết |
| ElastiCache/Aurora | Cùng Environment AWS Connection + region | VPC/subnets/security groups; connectivity từ EKS |

No explicit AWS Connection/region override trong Definition v2. Reuse kiểm
Organization/account/region/target generation và state owner; mismatch fail,
không fallback. Credential resolver đọc scoped Vault ref theo ADR-009/UC-04,
inject private subprocess environment/kubeconfig, loại host default chain khỏi
product invocation. Host AWS credential chain chỉ dành preflight/bootstrap
được giao rõ; không thay UI onboarding trong acceptance.

EKS invocation resolves cluster endpoint/CA and short-lived auth token with
scoped AWS credential; it does not execute untrusted kubeconfig exec plugins.
EKS access được tạo cho execution principal đúng Connection và node/workload
network paths. Kubernetes modules không cài cluster operators hoặc thay platform
storage class/ingress controller ngoài scope. Managed database/cache private,
chỉ allow ingress từ nguồn EKS cần thiết; không mở public database/Redis.

## Execution lifecycle

Logical `ExecuteResource(action, identity, pinnedModule, inputs, providerBindings,
operationOwner, stateRef)` trả safe status/outputs/physical refs. Actions CREATE,
UPDATE, DESTROY có preconditions khác nhau, dùng một state backend đã chốt.
Executor ghi intent/ownership trước call, init/validate, plan, review rồi apply
saved plan. State không tồn tại chỉ là CREATE khi chưa từng execution hoặc đã
xác minh destroy; state thiếu với prior attempt/ActiveResource thì chặn.

CREATE cho identity mới; UPDATE refresh state, giữ objects và dữ liệu, từ chối
delete/replace trước apply. Module fingerprint đổi cần Preview mới; executor
không reinterpret state theo schema khác khi chưa validation. DESTROY chỉ qua
cleanup riêng đã admit, dùng pinned old module/provider/target refs, kiểm consumer
và ownership, reverse dependency; không lấy module/Connection hiện tại thay lịch sử.

Secret outputs chuyển qua secret store, metadata chỉ giữ typed opaque refs.
CLI stdout/stderr, plan JSON/tfvars/state được xem là sensitive, không làm public
logs/evidence. Khi CLI hoặc HTTP backend ghi state lỗi, giữ private recovery
artifact và audit handle; không xóa bản state duy nhất. Saved plan chỉ có hiệu
lực trong cùng invocation có khóa, không dùng plan cũ sau retry/target change.

## Matching và renderer

`in-cluster`: Redis/PostgreSQL trong Kubernetes ở internal-k8s hoặc aws-eks.
`cloud-managed`: ElastiCache/Aurora, chỉ aws-eks. Metadata dependency công bố
classes từ catalog/capabilities authoritative; custom class vẫn phải match.
Không tự map `default` thành một trong hai class; giữ default semantics hiện
hành và để matching báo thiếu Definition nếu platform không cấu hình default.
Shared ID phải thống nhất type/class/params giữa mọi consumer; conflict chặn.

Workload matching giữ `workload.default#modules.<id>` và score-k8s-only bundle
selection trên cả hai profiles. Required bundle absent/no match/tie là lỗi trước
resource execution. Các binding đã resolve và Secret refs đi qua
[rendering contract](workload-rendering.md); không tạo database/cache/namespace
qua score-k8s hoặc fallback native khi render lỗi.
