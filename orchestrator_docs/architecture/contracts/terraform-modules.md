---
id: TERRAFORM-MODULE-CONTRACT
artifact: operation-contract
status: current
last_reviewed: 2026-10-28
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
vào verified bundle. Cho phép child module local trong cùng revision và child
HTTPS Git thuộc Admin allowlist, pin full commit + content digest; không tự giới
hạn toàn bộ child modules về local. Remote source chưa được phép hoặc mutable
phải bị từ chối. Cache chứa đủ dependency đã xác minh trước execution.
Provider lockfile chỉ pin providers, không thay module pin.

Trong contract của bundle, `transitive` liệt kê từng thư mục module con đã
xác minh, gồm `sourceId`, `revision`, `path` và `contentSha256`; cùng thư mục
chỉ có một mục. `moduleCalls` mô tả cây gọi, gồm `key` (ví dụ `net.subnets`),
`source` nguyên văn và `transitive` là chỉ số tới mục tương ứng. Metadata này
nằm trong contract JSON, không chứa credential hay đường dẫn cache nội bộ.
Trước execution, kiểm lại digest của root và mọi module con. Thiếu artifact
hoặc digest khác thì dừng; không tải lại trong Preview hoặc execution.

Content SHA-256 của một module directory là SHA-256 của danh sách files sắp
xếp theo slash path: mỗi entry gồm path, byte NUL, SHA-256 nội dung dưới dạng
lowercase hex, newline. Directory selection theo subdirectory; digest cover
names và content, không metadata thời gian. Artifact format/cache path không
phải thông tin công khai. Prepared release công bố pins/digests để PE dùng đúng
contract, không đoán SHA từ file tar có metadata không ổn định.

Release manifest chứa exact Terraform version + binary digest, provider source/
exact version + lockfile hashes, module commit/content digest và score-k8s bundle
digests. Không gọi `init -upgrade`; init dùng readonly provider lockfile. Catalog
metadata published sau inspect, gồm schemas/defaults và provider requirements.
Version literals/digests được tạo khi chuẩn bị modules, trước live run.

### Provider installation trong môi trường offline

Backend image chứa filesystem mirror read-only cho toàn bộ providers của
embedded release. Image build tải archive từ nguồn platform tin cậy và kiểm
SHA-256 archive với `zh:` hashes trong release manifest trước khi đưa vào mirror.
Runtime dùng `provider_installation` chỉ có `filesystem_mirror`, không có
`direct` fallback; thiếu provider/version/platform hoặc sai lock hash thì fail
closed. Build image cần mạng; chạy Terraform cho packaged modules không cần
Internet. Git module/render bundle sources vẫn cần kết nối tới Git server.

Admin chọn CLI config bằng `-terraform-cli-config-file` (env
`ORCHESTRATOR_TERRAFORM_CLI_CONFIG_FILE`); image mặc định trỏ tới cấu hình mirror
đi kèm. Bootstrap kiểm file tuyệt đối, regular và đọc được trước khi dựng
executor; không nhận file từ Definition/module hoặc kế thừa `TF_CLI_CONFIG_FILE`
của host. Chạy binary ngoài image không đặt option giữ hành vi hiện tại.
Plugin cache chỉ tối ưu cài đặt, không thay mirror; verified module cache là
artifact riêng. Không mount cache volume đè lên mirror trong image. Khi nâng
release, giữ provider lịch sử trong mirror platform được cấu hình nếu còn
resource cần cleanup với release cũ.

Để cleanup dùng đúng release lịch sử, platform giữ manifest, binary và module
cache của các release còn resource chưa xóa. Cấu hình Admin
`-terraform-retained-releases-file` (env
`ORCHESTRATOR_TERRAFORM_RETAINED_RELEASES_FILE`) trỏ tới JSON array; mỗi mục có
`release` theo cùng shape của platform release manifest và `terraformBinary`
là đường dẫn tuyệt đối tới binary đã pin. Reject unknown fields, manifest sai
hoặc đường dẫn tương đối. Đây là cấu hình platform, không phải input của PE
hoặc Workload.

DESTROY và absence probe xác minh persisted bundle với current hoặc retained
release phù hợp, copy và hash đúng binary, dùng original provider lock/module
cache. Không fetch module, upgrade hoặc re-match khi cleanup. Release mới dùng
packaged revision mới; giữ cache cũ và provider plugins cho lock cũ đến khi các
instance của release đó đã được xác minh hết. Thiếu release/artifact thì từ
chối cleanup và giữ remaining IDs; không thay bằng release hiện tại.

Public Definition creation là insert-only; V02 bổ sung explicit version-CAS
update theo UC-03 V02-06. PE có thể chọn verified module/pin khác của cùng Type
hoặc tạo ID mới; không overwrite concurrent winner hoặc historical resource pins.
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
`{target: account}` với Definition Driver Account và literal region trong variables;
Kubernetes giữ explicit legacy `{target: environment}` tới V03 hoặc
`{target: resource, resource_ref: <cluster descriptor>}` cho EKS dependency.
Provider output reference phải tạo graph edge và resolve scoped opaque target;
không nhận endpoint/credential trong provider binding; account/region chỉ đến
từ Definition fields V02. Known field/schema
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

Output được phân loại bí mật không được bind vào module input không bí mật,
kể cả khi placeholder nằm trong list/map hoặc chuỗi ghép. Kiểm classification
của Resource Type và module nguồn, không suy từ tên key của input hoặc annotation.
Nguồn đã biết phải bị chặn khi validation/planning; classification chỉ có ở
runtime phải được kiểm trước khi lưu resolved inputs hoặc chạy consumer. Không
đưa giá trị bí mật vào lỗi, preview, attempt metadata, labels hoặc annotations.

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

VPC bổ sung output tùy chọn `privateSubnetIds`: danh sách ID subnet private,
không bí mật (Type schema hiện hành dùng `any`; module kiểm list(string)).
Giữ `subnetIds` cho subnet node EKS và các outputs hiện hành. EKS target là scoped opaque
identity đủ tái tạo credential, không persist kubeconfig/temp path. Resource
không Ready hoặc output sai contract thì không render/apply consumer.

## Resource completion and supplemental readiness

UC-08 BR-10 admits every managed resource type of a verified module using the
platform release's approved providers. A new Terraform resource type does not
require a Go readiness allowlist entry. Successful execution, acknowledged
state and validated outputs establish provider completion; this delegates
type-specific creation/update semantics to the pinned Terraform provider.

The acknowledged resource inventory must be nonempty and every entry must
have a nonblank address and type. Physical IDs are required by probes that use
them; schemas without an `id` attribute (including `kubernetes_manifest`) rely
on their acknowledged provider state. Never invent a physical ID or claim a
separate API observation for those resources. Deadline/cancellation still
apply. Existing additional probes for Kubernetes Namespace/Deployment/
StatefulSet and AWS VPC/EKS/RDS/ElastiCache remain mandatory whenever those
objects occur. An inventory with no additional probes succeeds from provider
completion; in a mixed inventory every applicable probe must still succeed.
Unavailable/auth-failed/not-ready probes never fall back to provider completion.

Provider completion does not claim an independent application-level health or
connectivity check for other types. Module authors must configure provider
waiters/timeouts and declare dependencies needed by consumers. Output-only
modules without managed state, new provider installation, arbitrary readiness
scripts, ownership/admission, EKS usability, namespace provenance and destroy
absence verification are unchanged by this rule.

## Provider bindings và dependency

Namespace module/executor chỉ quan sát ServiceAccount mặc định để ghi baseline
UID theo [state contract](terraform-state.md), không mutate/adopt SA mặc định
hoặc cài thành phần cluster. Nếu dùng data source, nó không được coi là managed
resource cần destroy; cập nhật state/refresh không thay baseline bất biến.

| Module | Provider source | Required dependency |
|---|---|---|
| VPC | Environment AWS Connection + region | Không có |
| EKS | Cùng AWS Connection + region | VPC/mạng/access configuration |
| Namespace k8s-4f | Environment Kubernetes Connection | Trusted existing cluster binding |
| Namespace EKS | EKS target từ completed node | EKS Ready và access hợp lệ |
| Redis/PostgreSQL in-cluster | Cùng Kubernetes target | Namespace + storage/readiness capability cần thiết |
| ElastiCache/Aurora | Cùng Environment AWS Connection + region | VPC/privateSubnetIds dùng chung; subnet node EKS/security groups cho connectivity |

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

Module EKS nhận `public_access_cidrs` kiểu list(string), bắt buộc và không rỗng,
do PE/Admin cấu hình trong Definition. Trong release IPv4 hiện tại, các mục là
IP IPv4 cụ thể dạng `/32`, được kiểm hợp lệ trước execution. Không có default
mọi IP, không tự bổ sung IP executor. Public endpoint dùng danh sách này;
private endpoint cho các node vẫn bật và IAM/RBAC vẫn bắt buộc. Acceptance
preflight kiểm IP executor được cho phép trước run; thay đổi danh sách phải
đi qua cấu hình và Preview theo lifecycle hiện hành.

## Execution lifecycle

VPC module sở hữu private data subnets, route table và associations trong cùng
state Environment/generation. Aurora/ElastiCache nhận required input
`private_subnet_ids` list(string) từ Resource Reference tới `privateSubnetIds`
của VPC, và chỉ sở hữu subnet group, security group và dịch vụ riêng của instance.
Không tạo subnet/CIDR trong từng database/cache; không tự cấp phát mạng theo hash.
Các consumer cùng tham chiếu một VPC descriptor dùng chung node graph. Destroy
consumer giữ private network; destroy VPC gồm mạng đó sau khi hết consumer theo
reverse graph. PostgreSQL/Redis output contracts và state identities không đổi.

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

## V02 Definition account binding — replaces Environment AWS binding

UC-03 V02-03/04 owns selection/validation. Terraform `provider_bindings.aws`
is `{target: account}`; Definition `driverAccount` supplies scoped READY AWS
identity, and `values.variables.region` supplies literal region. Region becomes
editable schema metadata for AWS modules; account verification regions are probe
evidence, never a deployment allowlist. Never infer account/region from Environment
or host. Provider pins/PlanHash include actual Definition/account/version/region;
credential resolution is private and per invocation. Validate winning binding
before provision; no fallback account or different Definition. Cross-node
account/region equality is not a platform invariant.

Kubernetes resource bindings retain `{target: resource, resource_ref: ...}`;
no account on consumer; dependency target/identity supplies access. Explicit
legacy `{target: environment}` remains only until V03 cluster/Generic Workload
replacement. No new existing-cluster public driver in V02. All source/pin,
mappings, inputs, secret validation, dependency edges, state/locking/fencing and
retained cleanup release requirements above continue unchanged.

Definition update is now version CAS per UC-03 V02-06, replacing the insert-only
paragraph above. Existing executions/cleanup use persisted original Definition
content/target/module snapshot, not latest mutable catalog. New planning reads
updated content and invalidates old Preview; resource updates still require
explicit Preview/Deploy and preserve state identity.


## V03 internal provider placement

[ADR-016](../decisions/ADR-016-definition-selected-workload-targets.md) removes internal `{target: environment}` fallback. Kubernetes modules use `{target: resource, resource_ref: <cluster descriptor>}`. That cluster is a normal matched resource with its own Internal Driver Account. Namespace and workload typed cluster references must agree with this provider pin; known mismatch fails planning, unresolved output mismatch fails before consumer call. Do not derive account from Environment or the last cluster output. Original state/target/module/digest checks remain; AWS removal deferred V05–V06.

## V04–V06 dependency and target replacement

[ADR-017](../decisions/ADR-017-definition-dependency-execution.md) supersedes
remaining Environment/profile provider sources in the earlier table. AWS VPC,
EKS, Aurora and ElastiCache each bind their Definition's own Driver Account and
literal region; Kubernetes namespace/PostgreSQL/Redis bind explicit cluster
resource references. No runtime `{target: environment}` or profile-derived
infrastructure after replacement/removal gates. Matching class selects the
implementation without a profile guard.

EKS publishes Type `cluster` as the existing typed Kubernetes-target output,
extended with `provider: eks` and `region`, safe cluster/Organization/account/
accountVersion/descriptor identity. Internal targets retain existing shape;
validate the discriminated forms strictly. Publish only mapped Type outputs.
Runtime identity is derived from the producing node's pinned AWS provider, never
consumer Environment or an arbitrary authored output account. Treat credentials,
CA/endpoint access material and temporary kubeconfig as private invocation data.
Unknown outputs have declared types/provenance at Preview and resolve after
acknowledged provision/readiness, before provider consumers or rendering.

Required EKS readiness includes authenticated/authorized API access, executor
network reachability and usable Ready node capacity; ACTIVE alone fails the gate.
Resolve fresh temporary auth at each operation from original scoped AWS identity.
Managed data inputs must reference actual VPC/subnet/security connectivity needs;
module-internal depends_on does not replace platform graph references. Preserve
module/state/target/digest/fence and Secret-reference contracts during reuse,
restart, update and reverse cleanup. Cross-account/region validity is checked by
actual inputs/provider requirements, not an Environment-wide equality rule.

## EC2 role credential binding

UC-04 EC2-04 / ADR-018 extends AWS identity resolution for all existing AWS/EKS
provider pins. Region remains Definition-selected. AWS_EC2_ROLE uses an internal
per-invocation authenticated credential endpoint with expiration and identity
checks on refresh. Module inputs and persisted pins do not contain credentials;
retained-state/ownership/cleanup contracts are unchanged.


## Direct Git inspection — issue #7

Theo amendment ADR-014 ngày2026-10-20, PE/Admin có thể inspect HTTPS Git URL
không cần policy entry. `POST /api/v1/terraform-modules/inspect` nhận đúng
`resourceType`, `url`, `path`, `ref`; Type phải thuộc session Organization.
Empty path dùng `.`; ref bắt buộc là tag/branch/full commit, không nhận option/
refspec. Downloader resolve rồi fetch và kiểm đúng full commit, không checkout/
exec code. Digest tự tính theo thuật toán trên. Inspection có deadline hữu hạn;
provider release/type/content và transitive full-commit guards giữ nguyên.

Inspection ghi verified bundle/cache, không Definition/criteria/state hoặc cloud.
Response `{capability}` dùng cùng projection với capability list. Bundle contract
JSON bổ sung optional `sourceUrl` cho safe HTTPS origin; source ID sinh từ URL,
module ID sinh từ path/Type trừ release-equivalent identity quy định bên dưới;
không sinh ID từ mutable ref. Registration dùng persisted pin,
kiểm lại digest/release/cache; restart không cần thêm policy cho origin này.
Remote child HTTPS modules vẫn phải pin full commit và được inspect/cache trong
bundle. Execution/Preview/cleanup không resolve ref hoặc fetch version mới.
Full digest chỉ là chi tiết kỹ thuật ở UI. Legacy pins/allowlist config vẫn đọc
được; không có schema migration vì bundle contract là JSON.

Theo UC-03 GIT-03, Console mặc định author Definition mới từ Git. Packaged
modules tiếp tục phục vụ pins/lifecycle cũ và lựa chọn tường minh; không tự
chọn làm nguồn mặc định của form tạo mới.

Git copies whose root content SHA-256 and requested Resource Type match an
entry in the trusted platform release retain that entry's module ID and
executor-owned output metadata. Git URL/source ID, commit, subdirectory and
digest remain the actual immutable Git pin. Matching by path/name alone is
forbidden. Unmatched Git modules keep their generated IDs and inspected
outputs. This preserves EKS target recognition and namespace creation baseline
for unchanged published modules without granting arbitrary Git code platform
module semantics.

## ZIP upload — ADR-026

`POST /api/v1/terraform-modules/upload` accepts multipart `file` (exactly one ZIP),
`path` (optional, default `.`), `resourceType` (required scoped Type).
Unknown or repeated fields are rejected. Return `200 {capability: ...}` using
existing metadata projection; source identity and archive bounds follow
[ADR-026](../decisions/ADR-026-zip-source-upload.md). Malformed requests return
INVALID_REQUEST, oversized requests REQUEST_TOO_LARGE (413); invalid content
uses existing module/RENDER_BUNDLE_INVALID classifications. ZIP uses the same
verified cache, inspector, input schema and execution guards as Git. No mutable
ref or Git URL is required. Rendering engine `git-render-v1` is unchanged for ZIP.
