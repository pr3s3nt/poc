---
id: ADR-015
artifact: architecture-decision
status: current
last_reviewed: 2026-10-17
---

# ADR-015 — Infrastructure accounts for Definition-selected drivers

Status: Accepted for V01 account catalog; downstream delivery is V02–V06.
Date: 2026-10-17

## Context and authorization

[Issue #4](https://github.com/pr3s3nt/poc/issues/4) replaces Environment-wide
execution policy with Definition-selected drivers/accounts and dependency targets.
The user authorized V01 and accepted Organization ownership, PE/Admin management,
AWS access keys/Internal kubeconfig, existing immutable Vault scope, versioned
updates and verification before replacing credentials. The user explicitly
decided that V02–V03 need no old-data migration because there are no real users.
No task may erase retained resources or their ownership implicitly.

## Decision

1. Infrastructure account is one Organization-owned stable identity, with ID,
   key, name, infrastructure type `AWS` or `INTERNAL`, authentication type,
   non-secret configuration, private credential reference, verification and
   optimistic version. No account applicability criteria, Environment assignment
   or account-wide execution profile is introduced. AWS supplies identity;
   Internal supplies an existing-cluster descriptor plus private auth reference.
2. Use the existing Connection aggregate/repository and `connections` rows as
   backing identity in V01, not a second copy of the credential catalog. Public
   account `type` projects legacy kind `AWS` to `AWS`, `KUBERNETES` to `INTERNAL`.
   Infrastructure type and authentication remain separate. Stable ID/key and
   refs used by current consumers are preserved. Physical renames/removal of
   compatibility paths are later work, not a permanent second product flow.
3. PE/Admin can create/list/view/edit/verify accounts in their own Organization.
   Developer has no account-management or credential-read permission. Later
   driver execution may use only an explicitly selected, scoped READY compatible
   account after server-side authorization; V01 provides a catalog, not the
   V02 selection or planner implementation. Public DTOs never return secret
   references, kubeconfig or access keys, even to Admin.
4. Retain ADR-009 access-key and selected-context kubeconfig mechanisms and the
   existing platform Vault namespace
   `orchestrator/connections/<org>/<key>/credentials/<object-id>`.
   Existing scope is the storage boundary, not an account applicability scope.
   KV v2 objects are immutable/create-only and separate from workload stores.
   No arbitrary secret-ref input, host credential fallback, user-supplied
   exec plugin, external file lookup or credential read endpoint is added.
5. New accounts start version 1. Rename changes only name, preserving identity,
   target and verification. All updates/verify/credential replacement require
   the expected version; one transaction CAS publishes verified metadata/ref and
   increments version. Stale requests fail 409 without overwriting the winner.
   Request/provider/network work stays outside the database transaction.
6. Explicit **Kiểm tra kết nối** resolves stored credentials privately, performs
   existing read-only identity/connectivity/permissions checks and publishes a
   success only after the version guard. A failed check returns a safe error
   and leaves the last committed record intact; UI must show the failure and
   explain that READY records the last successful check, not current health.
7. Credential replacement is explicit and write-only. Normalize/verify before
   creating a new immutable object, then CAS on stable key/version. Failure or
   concurrent loss proven to have rolled back deletes only the attempt-owned new
   object. An uncertain database commit must retain the new object and require
   GET recovery; neither creation nor replacement may delete a potentially
   committed credential or automatically retry the mutation. Bounded cleanup
   failures are observable. Never delete a prior successful object in this
   slice: persisted targets may still pin it. Lifecycle garbage collection is
   deferred. Replacement cannot redirect the account to another AWS account
   ID or Internal cluster/context/endpoint/trust configuration; create a new
   account for a different target. Metadata edits cannot bypass this check.
8. Account registration/verification never provisions or deletes infrastructure.
   AWS `verificationRegions` (1–10 regions) specify read probes and permission
   evidence; they are not an applicability filter or fixed deployment region.
   Region selection belongs to Driver Inputs in V02. Reuse the current AWS
   verifier and keep the old allowed-region metadata only for the temporary
   legacy Connection execution path, with no claim V01 removes that policy.
   Replacement credentials must pass checks for every retained legacy allowed
   region before publication; never silently reroute an existing environment
   or retain permission evidence belonging only to the previous credential.
9. V01 retains existing Environment/Definition execution behavior and the old
   HTTP consumer contract while exposing the new catalog and UI. V02–V03 replace
   consumers before removing old binding paths. No old-data migration is required
   in V02–V03; any development reset must be explicit and separately scoped.
   No startup DDL, retained-state rewrite, default inference or resource deletion.
   Fresh V01 baseline has the version constraint; older SQL/JSON state receives
   a safe incompatibility/reset-required diagnostic, not an automatic conversion.

## Consequences and boundaries

V01 is a usable catalog across UI/API/Vault/SQL, with edits, read-only verify and
restart, while current deployment remains the M04 contract. ADR-012/013/014 keep
their current consumer behavior until each downstream replacement is verified.
Multiple accounts and clusters may be registered; no new same-account/region/
cluster restriction is introduced. V01 does not claim multi-account provisioning,
Generic Workload, driver/account selection, automated rotation, deletion, account
GC, new authentication methods or live AWS/Kubernetes acceptance for V02–V06.

Canonical account behavior/API belongs to [UC-04](../../usecase/UC-04/specification.md),
persistence to [schema](../database/schema.md), credential mechanics to
[shared design](../connection-credentials.md). Actual completion is recorded
separately in [V01 handoff](../../verification/2026-10-17-v01-handoff.md).


## Quyền reset dữ liệu phát triển — bổ sung 2026-10-17

Người dùng yêu cầu xóa dữ liệu sản phẩm cũ vì chưa có người dùng thực tế.
V01 được phép reset database/state phát triển cũ của dự án sau khi xác định rõ
instance/namespace/ownership và có baseline mới để khởi tạo. Không cần migration
hay giữ tương thích dữ liệu cũ. Đây là quyền reset tường minh của task, không phải
cơ chế tự reset khi startup hoặc quyền xóa tài nguyên không thuộc dữ liệu sản phẩm.
Codex thực hiện và ghi evidence phạm vi/kết quả; writer không truy cập retained
state hoặc tự chạy thao tác hạ tầng. Các yêu cầu secret scope, CAS, pinned targets
trên baseline mới và bảo vệ tài nguyên ngoài task vẫn áp dụng.

## V02 — Definition authoring và matching, 2026-10-17

V02 áp dụng [UC-03 V02-01..09](../../usecase/UC-03/specification.md).
Definition chọn AWS account riêng và region trong Driver Inputs; Kubernetes
consumer giữ dependency target thay vì chọn lại account. Năm criteria là toàn
bộ matching predicate; loại bỏ Definition profile guard. Environment target
và implicit system cluster chưa bị xóa trước lát cắt V03 được kiểm chứng.

Người dùng đã chốt sửa toàn bộ cấu hình Definition, trừ ID và Resource Type.
Version CAS bảo vệ concurrent edit; existing immutable snapshots và resource
execution/cleanup pins không bị rewrite. Preview/Deploy mới đọc catalog mới,
Preview cũ stale. Không thêm xóa Definition; không migration dữ liệu cũ.
Account là Connection backing identity của V01; không copy credential/catalog.
Physical Definition FK có thể giữ `connection_id` làm driver account backing;
public authoring chỉ có `driverAccount`. Shared contract cập nhật trong schema
và module contract; không suy account/region từ Environment cho AWS node.
