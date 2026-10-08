---
id: UC-04-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-06
---

# UC-04 — Use Case Realization

## Trách nhiệm và delivery boundary

Thực hiện [specification đã duyệt](specification.md): đăng ký Kubernetes từ
kubeconfig trước; AWS account provisioning theo ADR-009 ở giai đoạn sau.
Host-context records cũ vẫn được resolve; UI mới không yêu cầu host setup.

## System operations

- `InspectKubeconfig(ctx, org, document)` trả context/cluster/endpoint metadata,
  không trả credential, không lưu secret hoặc Connection, không gọi cluster.
- `RegisterKubeconfig(ctx, org, name, document, context)` parse/validate
  lại document, verify, ghi credential rồi insert Connection `READY`.
- `RegisterKubernetesCluster(ctx, org, key, clusterId, kubeContext)` keeps the
  explicit legacy API contract and does not accept uploaded credential.
- `RegisterAWSDriverAccount(ctx, org, command)` là operation thiết kế tương lai;
  không publish endpoint cho đến khi verification/executor resolution hoàn tất.

## Participants

- `ConnectionController`: session Organization + Platform Engineer/Admin gate,
  bounded strict request decoder, public metadata DTO.
- `ConnectionService`: name/key generation, parser, verifier, secret lifecycle,
  Organization lookup và insert-only repository transaction.
- `KubeconfigParser`: YAML/JSON parser; kiểm tra context references; chỉ nhận
  embedded token hoặc embedded client certificate/key; xuất normalized config
  chỉ chứa selected context và dữ liệu cần thiết.
- `KubernetesConnectionVerifier`: API/RBAC read-only bằng selected config,
  deadline và safe error categories; không dùng credential host.
- `ConnectionCredentialStore`: scope-aware Put/Get/Delete, opaque immutable refs,
  riêng khỏi UC-12 workload values; không public secret-read operation.
- `ConnectionCredentialResolver`: kiểm tra Organization/kind/authentication type,
  resolve reference cho adapter trong thời gian thực thi; fail closed.
- `ConnectionRepository`, `UnitOfWork`: insert-only, unique Organization/key.

## Flow và transaction boundary

MS-01..03: UI gửi kubeconfig qua inspect boundary rồi chọn context. MS-04..05:
register tự parse/validate lại payload; metadata do client cung cấp không được
coi là trusted. Chỉ selected context được sử dụng; rejected external-file/exec
material không được chuyển cho kubectl. Verify trước database transaction.

MS-06: Put credential với object ID mới, immutable theo Organization + generated
Connection key + attempt ID. MS-07: transaction kiểm tra Organization và insert
Connection `READY`; database unique constraint là final concurrent guard.
Generated key derives from name, bounded readable slug + collision suffix;
collision never updates another record. Registration does not change default.

Insert failure: Delete chỉ immutable credential của attempt đó, bằng bounded
cleanup context không bị request cancellation hủy ngay. Cleanup error phải
observable bằng safe reference, không làm registration có vẻ thành công.
No automatic retry that duplicates a successful insert after a lost response.

MS-08..09: trả public DTO, clear submitted credential on success, reload list;
reload error separate from committed registration. Read DTO excludes secretRef
and verification/provider details that can contain credentials.

## Execution integration

Shared design: [Connection credentials](../../architecture/connection-credentials.md).
Resource execution resolves matched Definition's Connection; workload target
retains opaque Organization/Connection identity and secret reference only.
Each Kubernetes operation resolves a private short-lived kubeconfig file,
mode 0600 in private directory, removes it after the subprocess finishes. Never
persist the file path or content in plans, Target outputs, executor state or
snapshots. Apply/readiness/remove/public routes/VSO must use the selected target.
Legacy host-context resolution remains explicit; registered credentials never
fall back to host default. AWS Terraform credential resolution is designed,
not enabled by this Kubernetes delivery.

## Validation and traces

Inspect/register HTTP role/Organization and limits; parser rejection and selected
context; normalized secret-only persistence; key collision/concurrency;
verification failure; store failure/cleanup; safe DTO/errors; scoped resolution;
Kubernetes provision/apply/remove/readiness/route paths; legacy compatibility.
Tests and actual delivery are recorded in traceability/current state after code.

## Secret stores

SecretStoreController/Service use PE/Admin gates, scoped insert-only store
repository, VaultStoreVerifier and existing platform credential port. Verify
KV v2/capabilities and attempt-owned probe cleanup, persist token privately then
metadata in local transaction; failure cleanup cannot touch another registration.
SecretStoreResolver produces scoped provider/delivery config, never raw public
credentials. This is separate from execution Connection kind/matching.

Compose registration bootstrap invokes shared SecretStore validation/verifier and
platform credential persistence with fixed `platform-vault` identity. Transactional
admission checks matching identity and expected prior credential before creating,
converting the Compose legacy record or refreshing its bootstrap credential.
Concurrent losers clean only their own attempt objects. Runtime uses the ordinary
SecretStoreResolver; conversion preserves store ID and existing references.
