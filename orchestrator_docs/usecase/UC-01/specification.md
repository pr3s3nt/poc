---
id: UC-01-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-28
---

# UC-01 — Create Application and configure Environment destinations

Current replacement design is the V04–V06 contract below, together with
[ADR-017](../../architecture/decisions/ADR-017-definition-dependency-execution.md).
Deployment Connection selection/transition passages describe the staged legacy
behavior until its replacement gate; they do not authorize that path after V06.08.
Actual implementation/acceptance status belongs to CURRENT_STATE.

## Mục tiêu và actor

Developer tạo Application, chọn và thay đổi Deployment Connection và Secret Store
Connection riêng cho staging/production. Platform Engineer đăng ký các đích qua
UC-04; UC-12 quản lý variables/secrets. [ADR-012](../../architecture/decisions/ADR-012-environment-stores-and-transitions.md)
owns shared transition/admission/recovery design.

## Tiền điều kiện

- **PRE-01:** Organization tồn tại và Developer đã đăng nhập với quyền Application.
- **PRE-02:** Platform base domain đã cấu hình; tạo Application không cần default.
- **PRE-03:** Khi chọn đích, có READY Connection đúng loại thuộc Organization.

## Main success scenario

1. **MS-01:** Developer nhập Name/Subdomain; không chọn đích ở cấp Application.
2. **MS-02:** Validate tên và subdomain, uniqueness.
3. **MS-03:** Sinh system Application ID bất biến.
4. **MS-04:** Tạo Application identity, không chọn provider/Connection mặc định.
5. **MS-05:** Tạo staging/production với hai lựa chọn đích chưa cấu hình.
6. **MS-06:** Tạo empty Deployment Sets và namespace identity cơ sở.
7. **MS-07:** Suy ra desired endpoints, chưa provision workload/hạ tầng/routes.
8. **MS-08:** Persist atomically, mở Application home staging.

## Environment Settings flow

- **ES-01:** Hiển thị hai mục Deployment Connection và Secret Store Connection,
  version hiện tại, pending/runtime destinations và operation đang chạy nếu có.
- **ES-02:** Load safe READY Organization choices cho từng loại; không auto-select default.
- **ES-03:** Developer chọn đích và Save với expectedVersion. Có thể đổi sau lưu.
- **ES-04:** Backend validate ownership, capability, READY và version; operation
  đang chạy trả 409. Không tin profile/region/Organization từ client.
- **ES-05:** Đích triển khai chưa có runtime: CAS metadata. Có runtime: chọn
  deploy-new hoặc migrate PostgreSQL with downtime; hiển thị impact và yêu cầu
  xác nhận downtime trước migration. Kho secret: copy theo UC-12 trước commit.
- **ES-06:** Trả version/operation progress; Environment khác không đổi. Refresh
  và restart phải hiển thị đúng persisted selection/progress, không khóa vĩnh viễn.

## Hậu điều kiện

- **POST-01:** Application thuộc Organization; đúng hai Environment độc lập.
- **POST-02:** Initial create không có external side effects.
- **POST-03:** Secret store change chỉ commit khi copy thành công. Migration chỉ
  đổi active runtime/current Set sau restore/readiness/cutover thành công.
- **POST-04:** Secret bytes không xuất hiện trong public metadata, logs hoặc artifacts.

## Quy tắc nghiệp vụ

- **BR-01:** System Application ID bất biến, unique; Name unique không phân biệt hoa thường trong Organization.
- **BR-02:** Subdomain lowercase DNS label, unique trong platform base domain.
- **BR-03:** Đúng staging/production; endpoints `<subdomain>.<base-domain>` và `staging.<subdomain>.<base-domain>`.
- **BR-04:** Application chỉ giữ identity/config metadata hiện hành: ID/key, Organization, Name, Subdomain và version. Không lưu profile, Connection, region, runtime status hoặc configuration provider; không có Application-wide target/provider hay implicit fallback. Không backfill binding cũ từ Application; historical target snapshots vẫn giữ nguyên.
- **BR-05:** Hai Connection lựa chọn thuộc Environment, độc lập, cho phép thay đổi.
- **BR-06:** Execution Connection chỉ READY Kubernetes/AWS đúng Organization;
  AWS cần region. Secret store chỉ READY supported store cùng Organization.
- **BR-07:** Strict request validation; missing/blank/null key/version 400; scoped
  missing app/env 404; unavailable/foreign/unsupported target safe 422; stale or
  active operation 409. Same key/current version no-op không migrate/copy lại.
- **BR-08:** CAS theo version, claim operation nguyên tử; normal Save không được
  bypass selection validation/operation ownership. Không giữ DB transaction khi gọi mạng.
- **BR-09:** Settings/config/draft thay đổi làm Preview cũ hết hiệu lực. Deploy
  admission kiểm tra đầy đủ snapshot và claim operation trong một transaction.
- **BR-10:** Draft/variable editing được phép trước chọn đích. Secret write cần
  kho đã chọn; Preview/Deploy cần execution target và secret store nếu có secret.
- **BR-11:** Source/destination generation tách tài nguyên/state cả khi hai keys
  dùng cùng cluster. Old deployments/resources giữ original targets. Generation 0
  giữ current ENVIRONMENT identity; product từ chối LEGACY_APPLICATION (T27).
- **BR-12:** Chuyển triển khai không reuse current resources ở nơi cũ. Deploy-new
  tạo hệ thống mới; migrate-data dừng ghi, backup/restore PostgreSQL tương thích,
  deploy/verify/cutover; không tự chuyển arbitrary storage hoặc tự xóa nơi cũ.
  Transition Preview hiển thị toàn bộ desired Set sau merge drafts và desired
  config revision, gồm workload không đổi. Cutover thành công consume đúng pinned
  drafts; thất bại giữ pending edits. Workload bị xóa khỏi desired không được
  xóa ngầm ở nguồn; DB nguồn không có mapping bị reject trước quiesce.
- **BR-13:** Lỗi trước cutover giữ source authoritative, khôi phục writers/routes
  nếu có thể và báo trạng thái recovery thật. Cleanup source là thao tác riêng,
  chỉ generation cũ owned/unreferenced; không tự rollback dữ liệu sau cutover.
- **BR-14:** Create vẫn chỉ Name/Subdomain; unknown connectionKey bị reject 400.

## Ngoài phạm vi

Environment clone/promotion, thêm Environment, zero-downtime transfer, arbitrary
DB/storage migration, AWS onboarding mới, tự động reverse migration. AWS live
verification không được cấp quyền trong task này.

## Delivery state

Thiết kế ADR-012 accepted; implementation status và evidence ở CURRENT_STATE.
Bản set-once ADR-011 là hành vi lịch sử đang được thay thế.


## V03 internal Settings replacement

[ADR-016](../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) supersedes the internal deployment Connection selector and deployment-transition flow above. Application create still takes Name/Subdomain only. Internal Environment Settings retain Secret Store/configuration, version/operation/error/reload states, but contain no deployment Connection picker. Internal Preview/Deploy resolve matched Definition references without a selected Environment Connection. Legacy deployment-target write fields/endpoints are rejected clearly after V03.10; do not silently accept or derive another target. AWS legacy consumers remain scoped to V05–V06, not newly exposed through an internal selector. Target versions and owner/fence checks remain necessary per resource. No migration required; startup never resets old data.

## V04–V06 — final Settings and creation contract

This extends the V03 replacement to AWS after the corresponding replacement
gates. Application creation keeps Name/Subdomain, two independent Environments,
empty Sets and no external side effects/default infrastructure. Environment
Settings manage the Secret Store/configuration, version and operation state;
they do not select a deployment Connection, executionProfile, account or region.
The old deployment-target transition/migrate-data entry is retired with that
binding. This does not introduce a new cross-target migration feature.

Preview/Deploy use Definition matching and references from UC-03/05/06;
per-resource/workload target pins preserve the originally resolved account,
region, cluster and namespace across update/restart/cleanup. An Environment
without the retired binding is valid; required matching, credential/store,
reference and readiness checks still apply before the relevant operation.
Secret Store selection/copy, explicit versioned Save, safe errors, stale Preview
invalidation and operation owner/fence protections remain in force.

After V06.08, retired deployment fields/commands must not be silently accepted
or inferred. Fresh persistence omits their fields; incompatible older data fails
explicitly without automatic migration/reset. Historical deployment/resource
targets remain provenance rather than a selector for new execution. No automatic
retargeting, shared-resource deletion or retained-data reset is authorized.


The final removal also drops the unused Organization default-choice marker and
its seed/SQL update path. Historical choice/default passages above do not define
a surviving public entry. An account registration must preserve Organization
metadata; it cannot select a target for a new or existing Environment.

## Optional Environment delivery — ADR-027

- **CD-01:** Each Environment defaults to DIRECT. In Settings, PE/Admin may
  explicitly enable a verified CD provider, independently for staging/production.
  Fleet is the first provider. No process-wide flag silently changes existing
  Environments. Cluster placement remains Definition-selected.
- **CD-02:** Save persists desired delivery only; current applied delivery stays
  visible until Preview → Deploy succeeds. A selection-only change is deployable
  even with no Score changes. Changing manager on deployed workloads requires
  the ownership transition in [CDR-07](../../architecture/contracts/delivery-reconciliation.md#cdr-07--delivery-transition).
- **CD-03:** Self-heal is a separate explicit capability-gated option, initially
  off; enabling CD does not implicitly enable repair. Infrastructure behavior is
  independent. Busy, stale, unsupported and unknown-outcome states are explicit.
