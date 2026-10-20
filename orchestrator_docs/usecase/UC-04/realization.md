---
id: UC-04-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-18
---

# UC-04 — Use Case Realization

## Trách nhiệm và delivery boundary

Thực hiện [specification đã duyệt](specification.md): đăng ký Kubernetes từ
kubeconfig và AWS access key onboarding theo ADR-009/014.
Kubernetes chỉ resolve scoped KUBECONFIG; không có host-context registration hoặc execution.

## System operations

- `InspectKubeconfig(ctx, org, document)` trả context/cluster/endpoint metadata,
  không trả credential, không lưu secret hoặc Connection, không gọi cluster.
- `RegisterKubeconfig(ctx, org, name, document, context)` parse/validate
  lại document, verify, ghi credential rồi insert Connection `READY`.
- `RegisterAWSDriverAccount(ctx, org, command)` nhận concealed access key,
  verify identity/region/capabilities, lưu Vault ref và READY metadata.

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
- `AWSConnectionVerifier`: read-only identity/region/capability verification;
  STS thành công một mình không đủ READY.
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
Resource execution resolves provider binding từ Environment Connection
hoặc completed EKS target; workload target
retains opaque Organization/Connection identity and secret reference only.
Each Kubernetes operation resolves a private short-lived kubeconfig file,
mode 0600 in private directory, removes it after the subprocess finishes. Never
persist the file path or content in plans, Target outputs, executor state or
snapshots. Apply/readiness/remove/public routes/VSO must use the selected target.
Missing/unknown authentication and missing scoped credentials fail closed; no
host default is consulted. AWS Terraform invocation resolve scoped Vault credential của Environment,
không dùng global/host credential. EKS resolver tái tạo private credential từ
opaque pinned target; state backend token tách khỏi provider credential.

## Validation and traces

Inspect/register HTTP role/Organization and limits; parser rejection and selected
context; normalized secret-only persistence; key collision/concurrency;
verification failure; store failure/cleanup; safe DTO/errors; scoped resolution;
Kubernetes provision/apply/remove/readiness/route paths; old-body/authentication rejection and cancellation cleanup.
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
refreshing an explicitly marked ordinary store's bootstrap credential.
Concurrent losers clean only their own attempt objects. Runtime uses the ordinary
SecretStoreResolver with scoped private credentials; refresh preserves store ID
and existing immutable references. Legacy store creation/backfill/conversion and
flag-configured runtime tokens are removed by T23.

## Implicit internal cluster collaboration

[ADR-013](../../architecture/decisions/ADR-013-implicit-existing-cluster.md)
changes only internal cluster binding: the enricher supplies the provider node,
trusted system binding bypasses criteria matching, and UC-08 resolves the selected
Environment Connection through existing-cluster. Preview is pure; execution
admits the reserved system Definition before persistence writes. Other resource
Definitions, executor scheduling and output propagation remain unchanged.


## V01 account collaboration

UC-04 AC-01..12 and ADR-015 add an Organization/PE/Admin-gated AccountController
(or equivalent routes in ConnectionController) over the same ConnectionService,
repository and scoped credential store. No second catalog or credential copy.
Public projection whitelists safe fields and maps KUBERNETES to INTERNAL.

Create reuses normalized kubeconfig/AWS verifiers; metadata edits take name and
expected version only. Reverify resolves the stored scoped secret, verifies
read-only outside a transaction, then CAS publishes metadata/version. Replacement
revalidates target identity, verifies supplied credential, writes one new attempt
object, then CAS publishes ref/config/verification/version atomically. Failure or
stale cleans only that attempt. Prior committed refs remain valid. Guard inputs
again in the transaction; never hold a database transaction during provider calls.

Frontend presents one account-management entry point, reusable onboarding inputs,
explicit rename/reverify/credential replacement and separate draft/request guards.
Current Connection consumer endpoints may remain for staged compatibility. New
account creation does not configure an Environment. Tests must cover version races,
unknown outcome/reload, permission/org isolation, target redirection rejection,
failed-attempt cleanup, safe DTO and durable SQL/Vault reopen.

## V04–V06 Definition-selected execution collaboration

[ADR-017](../../architecture/decisions/ADR-017-definition-dependency-execution.md)
supersedes the Environment Connection/implicit-cluster execution integration
above. Account registration/edit/verification still uses the shared scoped
repository and private credential store; no new account catalog is introduced.
The winning Definition supplies Driver Account and literal deployment region.
Internal cluster references resolve its selected account; Terraform-created EKS
returns safe typed identity pinned to its original account/version/region and
producing descriptor. Each invocation resolves private fresh authentication from
that original identity. No Environment/host/default/last-cluster fallback remains.

Read-only account verification does not replace execution-time EKS API/network/
Ready-node checks or authorize provisioning. Account verification regions remain
evidence, not a cross-resource equality restriction. The account catalog and
Secret Store workflows survive legacy executionProfile removal; historical
Connection storage identity is not an Environment deployment selector.

## EC2 role realization [EC2-01..05]

Controller calls RegisterEC2Role with scoped Organization/name/expected ARN/regions.
EC2RoleProvider reads fixed IMDSv2, validates STS assumed-role identity and IAM
GetRole canonical ARN/RoleId, then reuses region/capability verification. Service
creates the ordinary Connection without a Vault object. VerifyAccount dispatches
by authentication method and CAS publishes safe metadata, preserving roleId.
CredentialResolver resolves EC2 accounts separately from stored keys, rechecking
identity. ProviderBinder exposes a private authenticated refresh endpoint for the
Terraform invocation; readiness shares that endpoint and EKS resolves fresh keys.
See [ADR-018](../../architecture/decisions/ADR-018-ec2-role-driver-accounts.md).
