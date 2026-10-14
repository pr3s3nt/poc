---
id: TERRAFORM-STATE-CONTRACT
artifact: operation-contract
status: current
last_reviewed: 2026-10-13
related: [ADR-014, UC-06, UC-07, UC-08]
---

# Terraform v2 — durable state, locking, retry và cleanup

## Identity và storage

Canonical identity tuple: Organization ID, Application/Environment ID, target
generation, resource descriptor, scope type và scope ID. Backend opaque state ID
là digest của canonical tuple, ánh xạ một-một qua unique database constraint.
State không chứa run ID trong identity và không đổi khi Definition/version đổi.
Mỗi instance quản lý toàn bộ objects của module trong một state. Shared scope
luôn gắn Environment/generation, không chia sẻ state xuyên Environment.

Physical name suy ra từ stable instance identity, trong provider bounds; stable
password reference cũng gắn instance. State ref giữ account/region/Connection/
target snapshot và pinned module revision/digest để cleanup dùng đúng lịch sử.
Không reuse state ở target generation khác, kể cả cùng cluster/account.

Terraform HTTP backend nội bộ lưu ciphertext trong platform PostgreSQL. Metadata
công khai không chứa state hoặc hash secret. AES-256-GCM với nonce mới mỗi write,
AAD chứa org/state ID, key ID và revision; key nằm trong platform credential
store/config secret độc lập database. Key thiếu/sai thì fail closed, không tạo
state rỗng thay ciphertext không đọc được. Database permissions tách quyền state
blob khỏi public query. Rotation/DR thuộc D14.

Người dùng tạm hoãn TLS theo [issue #2](https://github.com/pr3s3nt/poc/issues/2).
Hiện không bắt buộc TLS cho kết nối PostgreSQL hoặc Terraform state HTTP
backend, kể cả ngoài loopback. Không thêm giới hạn thay thế như yêu cầu DB
cùng máy. Listener loopback hiện tại có thể giữ như lựa chọn implementation
nếu đủ cho executor; đó không phải constraint triển khai mới. Encryption at
rest, role/capability/scope/fencing và redaction vẫn là yêu cầu hiện hành.

Người dùng chọn A: ba DB role và kết nối riêng trong cùng một database.
Owner chỉ dùng cho khởi tạo schema/cấp quyền; runtime Console không sở hữu
bảng, không có quyền SELECT state blob hoặc token hash/capability credentials.
State role có quyền tối thiểu để quản lý state/locks/capabilities/attempts dưới
claim/fencing; không có quyền DDL hoặc quyền owner. Console chỉ được truy cập
metadata cần cho operation release/recovery, không có đường nâng quyền đọc blob.
Lịch sử Deployment được đọc projection an toàn của attempts đã liên kết đúng
Deployment/Environment: stage/status/attempt, action/ownership, mã lỗi được phép,
descriptor/scope, module ID/revision và timestamps. Console chỉ có SELECT các
cột metadata cần dùng trên instances/attempts; snapshots, target credential
references, physical intent/IDs, state serial và recovery handle thuộc State.
Mã không được nhận diện phải hiển thị thông báo chung, không trả nguyên văn.
Console được đọc logical `state_key` và module bundle ID cần để ghim admission
trong metadata instance/confirmed scope. Các IDs này không cấp quyền đọc state;
HTTP state vẫn cần capability riêng đúng scope/owner/fence. Không đưa logical
state key, backend reference hoặc capability credential vào DTO, token có thể
giải mã, log hay video. Target reference vẫn riêng tư; confirmed scope dùng
digest để kiểm đúng đích gốc mà không chứa reference đó.
Credentials runtime lưu trong file riêng chỉ owner OS được đọc; key mã hóa
độc lập database. Không dùng owner connection như fallback khi thiếu runtime
credentials; cấu hình sai phải fail closed. Test dùng đúng login roles để chứng
minh SELECT bị từ chối ở database, không chỉ kiểm interface Go. Chi tiết provisioning
role thuộc runbook; không thay quyền hoặc reset database retained chưa được giao.

## HTTP backend operations

Internal endpoint `/internal/terraform/states/{opaqueStateID}` chỉ cho executor,
không dùng Console session. Short-lived Basic-auth invocation token có hash lưu
ở private capability record, pin Organization, state, Environment operation,
fencing generation, action và deadline. Không truyền token bằng argv/backend
file; dùng private subprocess env. GET/POST, LOCK/UNLOCK theo
[Terraform HTTP protocol](https://developer.hashicorp.com/terraform/language/backend/http).
Mỗi request xác minh capability và current owner trong transaction ngắn.

| Operation | Required semantics |
|---|---|
| GET | Đọc/decrypt state của đúng scope; chỉ state key hợp lệ/đúng scope chưa có blob mới trả no-state. Key sai định dạng trả 400 hoặc 403, không 404; không tạo state |
| LOCK | Atomic claim theo state, Terraform lock ID và operation/fence; cùng lock/owner/fence có thể acknowledgement idempotent; 200 khi thành công, 409/423 với safe metadata khi bị owner khác khóa |
| POST | Cần query lock ID khớp và owner/fence hiện hành; validate lineage/serial tiến lên, encrypt/commit nguyên tử; replay cùng lineage/serial với bytes identical được ack idempotent, bytes khác hoặc serial lùi bị chặn |
| UNLOCK | Chỉ owner/fence và lock ID đúng được release; không tự unlock khi deadline/heartbeat hết |
| DELETE | Không cấp quyền purge cho executor thường; destroy ghi empty terminal state, metadata/tombstone giữ lại cho audit |

Không `-lock=false` hoặc force-unlock tự động. State write thất bại chặn kết quả
READY dù provider đã tạo resource. Lock ID, owner, operation, deadline là safe
metadata; không trả backend token, raw state, raw provider diagnostics.

## Hai lớp khóa

Environment operation claim bảo vệ toàn pipeline từ admission tới final commit
hoặc failure/recovery hold. Deploy, resource cleanup và transition cùng dùng
claim của ADR-012; conflicting Settings/config/draft writes trả BUSY.
Terraform state lock bảo vệ mỗi instance trong invocation. Lấy Environment claim
trước state lock, release state lock khi invocation đã hoàn tất, rồi release
Environment claim khi không còn subprocess đang chạy hoặc unknown outcome.

Persist heartbeat/deadline/stage nhưng expiry không chứng minh process đã dừng.
Interrupted ownership giữ hold; manual intervention xác nhận process group đã
dừng, tăng fencing và revoke capability cũ trước unlock/retry/cleanup. Stale
owner không được write/release bằng operation ID cũ. Khóa database transaction
chỉ dùng lúc claim/write, không giữ transaction qua Terraform/provider calls.

## Retry và gián đoạn tối thiểu trong v2

Mỗi invocation có timeout hữu hạn và process group riêng; cancellation phải
terminate/confirm group stopped trước release. Retry tự động tối đa một lần cho
lỗi transient đã phân loại, có backoff; refresh/read state và kiểm actual result
trước replan. Provider/HTTP request retry nằm trong deadline này, không kéo dài
run vô hạn. Validation, auth, permission, destructive plan và unknown outcome
không automatic retry apply. Scenario retry tối đa một lần là giới hạn khác,
được ghi trong acceptance runbook.

Write-ahead execution record trước side effect: operation/run ID, state ref,
target/owner, action, module pin, physical-name/tag intent, ownership classification,
attempt và stage. Sau từng bước ghi observed physical IDs, state serial và safe
result. CREATED_BY_RUN chỉ khi có bằng chứng mới tạo bởi run; prior state/record
luôn PREEXISTING hoặc UPDATED_BY_RUN. Không đổi classification khi retry.

Sau crash, ghi INTERRUPTED khi quan sát owner không hoạt động, không claim thành
công/thất bại giả và không tự resume apply. Nếu provider objects có thể đã tạo
nhưng state chưa ghi, dừng retry tạo mới và báo NEEDS_ATTENTION ở execution record;
operator đối chiếu run tags/physical intent và xử lý thủ công. Automated resume
ở D12, import/reconcile orphan ở D13. Không import tài nguyên v1.

Nếu state backend không lưu được, giữ encrypted/private local state recovery
artifact do Terraform sinh nếu có, mode directory 0700/file 0600 và audit handle.
Không commit/publish nó, không đưa contents vào output. Không xóa workspace
khi đó là bản state duy nhất cần cho xử lý và cleanup.

## Cleanup operation

Cleanup có hai cách cấp quyền: user deletion theo UC-07 DEL-* và test-run
cleanup. User deletion chỉ PE/Admin, root UNREFERENCED, không consumer/busy;
PrepareDeletion hiển thị candidates; chỉ root tick mặc định, dependency do
người dùng chọn. Execute chỉ xóa selected set hợp lệ, không tự thêm mục; cảnh báo dữ liệu
và yêu cầu exact root descriptor. OC-19 pin scope/versions/actor/token và nhận
operation nguyên tử. Graph/versions đổi phải review lại, không mở rộng tập xóa.

Test-run cleanup dùng capability riêng và chỉ CREATED_BY_RUN. Không áp quy tắc
run provenance để ngăn user xóa resource managed thuộc quyền nhưng tạo ở run cũ;
ngược lại, không dùng role user để nâng quyền cleanup script. Cả hai kiểm
ownership/consumers/dependencies, original target và state lock dưới Environment
claim. Không account-wide/namespace-prefix wildcard; namespace ownership không
cấp quyền xóa objects của bên khác. Public HTTP và idempotency ở OC-19.

Stages: REQUESTED -> CHECKING -> DESTROYING -> VERIFYING -> SUCCEEDED; lỗi là
FAILED hoặc INTERRUPTED/NEEDS_ATTENTION và lưu remaining IDs. Xóa consumers
trước providers. Delete namespace chỉ khi toàn bộ nội dung có quyền xóa và đã
xử lý dữ liệu/PVC do run tạo; không delete namespace để bỏ qua Terraform states.
Nếu consumer destroy lỗi, giữ dependency còn cần, tiếp tục nhánh độc lập an toàn.

Destroy success phải kèm query provider chứng minh resource absence theo physical
IDs/tags; state giữ terminal snapshot/tombstone. Retry cleanup dùng original
target/module/state; state/credentials thiếu thì giữ evidence và báo chưa hoàn
tất. Evidence chỉ safe IDs/status. Acceptance cleanup independent supervisor
phải tồn tại ngoài browser/runner process; restart thủ công supervisor được hỗ
trợ, autonomous platform recovery không được claim.

## Verification obligations

Real disposable PostgreSQL tests chạy với `REQUIRE_POSTGRES=1`; thiếu
`ORCHESTRATOR_POSTGRES_TEST_URL` phải fail thay vì skip. State reopen, ciphertext/no plaintext, wrong
key, scope rejection, unique identity, two-process lock race, stale fence write/
unlock, lineage/serial rejection, failed write rollback và no unlock on expiry.
Executor tests: stable names/credentials across retry, no duplicate CREATE on
unknown state, update preserves identity/data, destructive update rejected,
destroy ownership/consumer/dependency checks, retry timeout/cancellation và failed
cleanup retained metadata. Acceptance giữ hai exceptions đã chốt, thêm bước user deletion trong V2-A01.
Các tests khác không mở thêm live update/delete matrix hoặc disaster recovery.
