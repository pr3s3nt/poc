---
id: PROJECT-CURRENT-STATE
artifact: project-status
status: current
last_reviewed: 2026-10-12
---

# Current project state

## Lifecycle position

Phase 1–5 của Unified Process cho UC-01..UC-09 đã hoàn thành và design gate đã PASS.
UC-00 được bổ sung sau gate và đã có local/test implementation. Executable
architecture cho UC-06/UC-08 cùng query path tối thiểu của UC-09 đã được hiện
thực. Internal happy path đã được kiểm chứng trên kind; cloud happy path đã được
kiểm chứng trên AWS với VPC, EKS và Aurora rồi cleanup.

[I06-04 — Complete UC-09 observability](iterations/M02-usecase-completion/I06-04-uc09-observability/README.md)
thuộc [M02 — Use-case completion](iterations/M02-usecase-completion/README.md)
đã hoàn thành local verification ngày 2026-09-30. Hạng mục tiếp theo trong M02
là I06-08 (UC-05), nay cũng đã hoàn thành
[local/API/UI/PostgreSQL verification](verification/2026-09-30-uc05-preview-local.md).
I06-09 (UC-07 update/remove) cũng đã hoàn thành
[local API/UI/PostgreSQL verification](verification/2026-09-30-uc07-update-remove-local.md).
I06-10 đã hoàn thành [registration hardening safe slice](verification/2026-09-30-registration-hardening-local.md): insert-only duplicate guards,
safe errors, truthful UI reload states và no-restart catalog usage. I06-10/M02
chưa đóng; identifier/Driver Inputs policy đã được chốt và hoàn thành local code/API/UI
validation ngày 2026-10-02. AWS access key + Vault storage đã được chọn
theo ADR-009; phần AWS onboarding chưa triển khai.
[M00-a — Developer onboarding](iterations/M00-developer-onboarding/README.md) đã
hoàn thành 2026-09-30 với React UI, authenticated API, session, normalized
PostgreSQL persistence và local browser restart verification cho UC-00/UC-01.
[M01 — Contract hardening](iterations/M01-contract-hardening/README.md) đã hoàn
thành: I06-05 đóng IMP-010, I06-06 đóng IMP-008, I06-07 đóng IMP-009.

## Use-case delivery state

| UC | State | Current conclusion |
|---|---|---|
| UC-00 | Implemented; locally E2E verified | Fixed local/test accounts, opaque HttpOnly cookie session, restore/expiry/revocation và sign-out đã có; production profile không seed hoặc chấp nhận fixed credential từ state mới hay legacy. |
| UC-01 | Implemented; locally E2E verified | Authenticated Developer list/create/get Application qua API/UI; service tự sinh ID, staging/production, empty Deployment Sets và namespace identities. Create không deploy; browser restart test xác nhận durable local state. |
| UC-02 | Registration implemented | Platform Engineer/Admin có thể đăng ký và xem Resource Types trong Organization qua API/UI; seeded catalog và normalized PostgreSQL persistence dùng được. Production-grade RBAC chưa có. |
| UC-03 | Registration implemented | Platform Engineer/Admin có thể đăng ký Definition runtime-supported với criteria, driver inputs và provision rules qua API/UI; planner vẫn có profile guard và matching, catalog được persist trong PostgreSQL. Remote source và production-grade RBAC chưa có. |
| UC-04 | Kubernetes kubeconfig onboarding implemented and verified | Platform Engineer/Admin nhập tên, upload/dán kubeconfig, inspect/chọn context rồi verify API/RBAC read-only và lưu `READY`. Vault KV v2 lưu scoped credential; metadata/name/authentication type ở PostgreSQL, migration/race/reopen đã pass isolated local. Executor resolve credential bằng opaque target identity; T22 bỏ host-context/API body cũ, seed thiếu credential ở VERIFYING. Live kind upload và video đã pass ngày 2026-10-06; AWS infrastructure identity onboarding vẫn future scope theo ADR-009. |
| UC-05 | Implemented; locally verified | Standalone Score Preview service, authenticated scoped API và Console trả Delta/Candidate/graph/matches/batches/classification, consistent snapshot chung với direct Deploy, safe public projection và no-mutation tests. Pending-change Preview vẫn riêng; UI-only human-paced recording đã pass. Không chạy Terraform plan thật. |
| UC-06 | Executable baseline; partially conformant | HTTP → plan → UC-08 → target workload apply đã pass kind/AWS; optional Fleet GitRepo adapter cho internal kind đã pass Harbor-image deployment. Environment Ingress nhiều path và Fleet `_routes` bundle đã pass kind test với hai workload BusyBox, Fleet revision/prune và cleanup; route failure có route-only retry (unit test), Backstage cụ thể chưa kiểm chứng. DNS/TLS/chuyển controller ra ngoài chưa có. Mỗi Deployment persist immutable `DeploymentDeltaSnapshot`; UC-12/16 pending changes deploy theo Preview token với per-workload result. |
| UC-07 | Implemented; locally verified | UC-16 scoped Preview → Deploy update/remove, before/shared invariants, final Service validation, confirmation/Undo và busy/stale/reload UI states. READY resource marking UNREFERENCED commit cùng current set/SUCCEEDED; PostgreSQL rollback/reopen và UI-only recording pass. Không destroy/rollback; Application-wide cleanup ngoài scope. Fleet remove kind evidence là lịch sử, chưa rerun live cho mốc này. |
| UC-08 | Implemented and E2E verified | Kubernetes và Terraform resource execution, output propagation và persistence baseline đã pass. |
| UC-09 | Implemented; locally E2E verified | Authenticated Organization/Application/Environment-scoped history/filter và detail; consistent read snapshots, per-Deployment workload history, fail-closed output redaction và không trả resolved inputs. React loading/error/not-found/retry và browser redeploy/restart checks đã pass. Comparison và live status ngoài scope. |
| UC-12 | MVP path implemented on kind | Settings UI/API, immutable desired/applied revisions, Vault KV v2 adapter, scoped backend/workload policies và VSO → namespace Secret → Pod `secretKeyRef` đã pass kind; secret không xuất hiện trong read API, snapshot hoặc Pod spec. Production secret lifecycle/HA chưa có. |
| UC-16 | MVP path implemented | Form/Score import, typed resource params, public path + Service port rows, draft save/delete/undo, references, edit deployed workload bằng reconstructed Score, Preview → Deploy và partial retry đã có. Preview bỏ qua no-op draft nhưng giữ UC-12 revision update. Kind đã kiểm tra multi-path BusyBox `/` + `/api`, Fleet route và no-op Pod UID; Backstage cụ thể và broader update/cloud path chưa kiểm chứng. |

## Refactor T16 — Environment AWS account (2026-10-11)

[Local verification](verification/2026-10-11-T16-environment-aws-account.md): public
Terraform VPC/EKS chỉ nhận ConnectionKey rỗng; form không có selector và execution
inherit READY AWS Connection của Environment. Aurora giữ account riêng bắt buộc.
Effective target/hash/executor/ActiveResource nhất quán; malformed winner và
cross-account state reuse fail closed. New seed bỏ VPC/EKS key, không migrate
catalog/profile cũ hoặc đổi matcher/ties/reference fixtures. Go/conformance/build,
378 frontend tests và headed fake T16/T02 pass; video decoded/frame reviewed.
SQL opt-in/cloud/cluster checks không chạy; không có external mutation/live proof.

## Console refactor T15 (2026-10-11)

[Local verification](verification/2026-10-11-T15-deployment-details.md): chi tiết
triển khai dùng tiếng Việt cho metadata, trạng thái, graph/batches và UI states;
giữ snapshot/IDs/API và thời gian vi-VN. Failure dùng hướng dẫn an toàn, output
bí mật hiện “Đã che”, nested outputs và resolved inputs không render. Scope/ID/
lượt đọc cũ không ghi đè kết quả mới. 375 frontend tests và headed fake T15 pass,
video decoded/reviewed; graph/redaction/failure/status/delay dùng projection mocks.
Không external mutation/live proof; mapping lỗi chi tiết thuộc T20.

## Console refactor T13 (2026-10-11)

[Local verification](verification/2026-10-11-T13-score-preview.md): standalone
Xem trước Score dùng tiếng Việt cho form/kết quả/lỗi/retry và accessible labels;
giữ parser semantics, IDs, planning data/payload và `staging`/`production`.
Không lưu draft/Deployment hoặc provision; stale/input/scope guards giữ nguyên.
344 frontend tests và headed fake T13 pass, video được decode/frame review.
413/404/500/401/delay dùng projection mocks; chi tiết error code thuộc T20.
Không external mutation hoặc live adapter proof.

## Console refactor T12 (2026-10-11)

[Local verification](verification/2026-10-11-T12-application-home.md): trang
ứng dụng, Workload pending actions/results, triển khai gần đây và tóm tắt đích
dùng tiếng Việt; giữ IDs/payload và staging/production. Preview chỉ đọc, Deploy
tường minh, confirmation/Undo và stale/scope guards giữ nguyên. Raw lỗi dùng
fallback Việt an toàn; mapping đầy đủ thuộc T20. 336 frontend tests và headed
fake T12/T09 regression pass. Partial/stale/delay là projection mocks ghi riêng;
BUSY kiểm bằng unit, không chứng minh live deployment. Không external mutation.

## Console refactor T11 (2026-10-11)

[Local verification](verification/2026-10-11-T11-applications-list.md): danh sách
ứng dụng, cards và loading/empty/error/retry dùng nhãn tiếng Việt. Tên, endpoint,
thứ tự dữ liệu, create/card routes và deep links giữ nguyên; nhãn môi trường là
`staging`/`production`. 324 frontend tests và headed fake T11/T01 regression pass.
Empty/loading/500 là GET projection mock được ghi riêng; tạo/mở ứng dụng qua
backend fake local, không external mutation. Application home đã hoàn tất ở T12; code-based
errors còn thuộc T20.

## Console refactor T09 (2026-10-11)

[Local verification](verification/2026-10-11-T09-create-application.md): trang
Tạo ứng dụng dùng tiếng Việt, gợi ý subdomain từ tên đến khi người dùng sửa
thủ công; giữ cả lựa chọn xóa trống. Validation/trùng/lỗi chung giữ form và
retry tường minh. Payload name/subdomain, endpoints và navigation staging giữ
nguyên. 309 frontend tests và headed fake T09 pass; lỗi async 500 là mock,
không external mutation. Shared shell/list/home đã hoàn tất ở T10/T11/T12;
code-based errors còn ở T20.

## Console refactor T08 (2026-10-11)

[Local verification](verification/2026-10-11-T08-workload-editor.md): editor/key
picker Việt hóa; CPU/memory, resource class và target-port override trong phần
nâng cao với tóm tắt defaults hiệu lực. Collapse giữ imported/edited overrides;
Score không tự thêm capacity. Thêm route chỉ tự chọn đúng một cổng hợp lệ;
port rename/delete/invalid giữ đích cũ và chặn lưu, không retarget. Typed params,
key aliases, stale/BUSY/scope guards giữ nguyên. Headed T08 và T03 regression
pass với fake adapters; Secret catalog/stale/BUSY là projection mock. Backend
error mapping T20 và publicPort removal T25 đã hoàn tất theo các record riêng;
không live mutation.

## Console refactor T07 (2026-10-11)

[Local verification](verification/2026-10-11-T07-environment-settings.md):
Cài đặt môi trường và các dialogs cấu hình/chuyển đích/recovery dùng nhãn Việt.
Một READY choice hợp lệ chỉ được gợi ý draft khi chưa có binding; nhiều lựa chọn
phải chọn tường minh, không tự persist/default/fallback. Chỉ Lưu đổi binding;
binding đã có không bị thay. Reload/stale/reply muộn giữ pending edits và không
đưa cấu hình sai app/env vào trang đang xem. Secret không đọc lại; T07 giữ
rename-only, T30 bên dưới bổ sung lựa chọn explicit rewrite desired references. Backend/API/version/downtime/recovery confirmations giữ
nguyên. Headed local dùng fake/memory adapters; transition/recovery và lỗi
copy/stale/BUSY dùng projection mock, không chứng minh migration thật. T20 error
mapping và T30 rename-reference behavior được ghi ở các mục delivery tương ứng.

## Console refactor T06 (2026-10-11)

[Local verification](verification/2026-10-11-T06-connections.md): trang Kết nối
dùng tiếng Việt, upload tự inspect sau đọc file và paste debounce 500 ms.
Một context tự chọn; nhiều context không đoán theo current-context. Tên gợi ý
theo cluster/context chỉ khi chưa sửa; tên custom hoặc xóa trống được giữ.
Đổi document/source vô hiệu summary và file-read/inspect pending. Inspect không
lưu; kiểm tra/lưu vẫn tường minh. Kubeconfig paste mặc định che, lỗi dùng hướng
dẫn Việt an toàn; T06 giữ API/backend/HOST_CONTEXT tại thời điểm đó, T22 đã bỏ HOST_CONTEXT. Headed local pass;
parser/422 được kiểm qua backend local, success 201/list là mock projection,
không chứng minh cluster/credential persistence thật. T20/T22 vẫn deferred.

## Console refactor T05 (2026-10-11)

[Local verification](verification/2026-10-11-T05-secret-stores.md): trang Kho bí
mật dùng tiếng Việt, phần nâng cao giữ mount/CA với tóm tắt defaults hiệu lực.
Checkbox dùng cùng địa chỉ là opt-in, giữ draft workload riêng khi tắt. Local
validation chặn POST sai; server lỗi xóa token và giữ metadata. Danh sách tách
xác minh backend với Kubernetes auth; READY không cam kết deploy-ready. API và
Developer choices giữ nguyên; code-based errors còn chờ T20. Video headed local
fake pass; ABSENT/list failure là projection mock, không chứng minh Vault thật.

## Console refactor T04 (2026-10-11)

[Local verification](verification/2026-10-11-T04-resource-types.md):
Trang Loại tài nguyên dùng tiếng Việt, xem schema GET và sao chép schema sang
draft độc lập với ID mới, giữ types/required/secret. Duplicate không ghi đè
catalog; submit lỗi giữ form. Metadata authoritative T31 dùng chung với form
Cấu hình tài nguyên xác nhận cặp hỗ trợ; custom type như postgres-ha chỉ có
contract. CTA chọn sẵn type sau khi xác nhận catalog và metadata; workload dẫn
sang Mẫu dựng ứng dụng. T31 bổ sung API metadata chỉ đọc, không thêm executor;
code error mapping đầy đủ đã có ở T20.

## Console refactor T03 (2026-10-10)

[Local verification](verification/2026-10-10-T03-rendering-templates.md): trang
Mẫu dựng ứng dụng tại `/ui/platform/rendering-templates` dành PE/Admin đã có
list workload-only, ID/bundle ID và editor criteria chung. POST giữ BR-15/16,
không connection override/provision/source. Form/error/help dùng tiếng Việt;
T18 đã thay nhập bundle ID bằng selector từ registry; xem mục T18 bên dưới.

Headed local scenario kiểm native/selected renderer Preview và selected render
failure/no fallback qua Deploy. Infrastructure/delivery fake, score-k8s là stub
local cố ý generate lỗi; không claim output CLI thật hoặc live provisioning.
Runner live template đã đổi route/locator, chỉ kiểm syntax, chưa rerun live.

## Console refactor T02B (2026-10-10)

[Local verification](verification/2026-10-10-T02B-criteria-editor.md): editor
Điều kiện áp dụng dùng chung đã có bốn chế độ, dropdown app/env IDs thực,
copy độc lập và giữ criteria nâng cao/nhiều dòng khi chuyển mode. Xem trước
phạm vi và cảnh báo overlap vẫn là trợ giúp frontend; matching authoritative
đã được bổ sung ở T19 (xem mục bên dưới). Payload/matcher không đổi; headed fake evidence không chứng minh
live provisioning. T03 dùng lại editor cho Mẫu dựng ứng dụng.

## Console refactor T02 (2026-10-10)

[Local verification](verification/2026-10-10-T02-resource-form.md): frontend
checks và headed fake scenario pass; video/visibility và payload reviewed.

Form Cấu hình tài nguyên dùng tiếng Việt và chỉ cho đăng ký
Terraform/Kubernetes; driver/module tự chọn theo type, Terraform khóa `aws-eks`,
Kubernetes có Cluster nội bộ/Dùng chung. Connection lấy từ list org-scoped,
lọc READY đúng kind; Kubernetes mặc định dùng Connection của Môi trường,
override nằm ở nâng cao. Terraform vẫn cần AWS Connection tường minh tới T16.
JSON/criteria và lựa chọn theo driver được giữ khi đổi controls hoặc submit lỗi.

Workload Definitions không còn ở list/form resource; trang Mẫu dựng ứng dụng
đã được triển khai riêng ở T03. Backend rendering/API vẫn giữ nguyên. Runner live
`frontend/test/e2e/template-engine-kind-human.mjs` đã cập nhật route/locator tại
T03 nhưng chưa chạy lại live. Exact
matching-preview đã bổ sung ở T19; code-based error mapping vẫn thuộc T20.

## Definition-selected rendering (2026-10-06)

[ADR-010](architecture/decisions/ADR-010-score-k8s-workload-rendering.md) is accepted.
Registration → standalone/pending Preview → Deploy supports optional score-k8s
0.15.0 on internal-k8s through an installed immutable bundle. The native renderer
remains the default. Plans pin Definition/bundle/binary content, pending detects
renderer-only updates, and preflight runs before UC-08 side effects. Resources
are still provisioned by their existing executors; output-only CLI bindings carry
non-secret values and Secret references. Shared policy preserves resources,
probes, replicas, pull references, VSO refs and Agent injection.

[Local evidence](verification/2026-10-06-score-k8s-rendering-local.md) covers the
real CLI with fake infrastructure/delivery, HTTP flow and JSON-state reopen.
After a renderer upgrade the same Definition selects the installed bundle; old
previews are stale and pending proposes a renderer-only update (registration
fingerprint is audit-only).
No live kind, Fleet/VSO readiness, PostgreSQL database mutation or AWS run was
performed for this extension. The configured CLI is an external process
prerequisite; current deployment images do not automatically install it.

## Executable baseline

- Go HTTP API, application services, planner, resource executors, Kubernetes
  deployer, normalized PostgreSQL adapter và local/test JSON snapshot store nằm
  dưới `backend/`.
- Web Console React/TypeScript có sign-in, Applications list/create/home,
  UC-12 Settings, UC-16 editor, Application Preview/Deploy panel và UC-09
  recent deployments/history/detail với authenticated scoped read API.
- Planner product conformance chạy đủ 33 fixture: 27 accepted cases so sánh
  Humanitec-shaped Delta, Candidate Set/graph/matching/batches/classification/
  Terraform artifacts và kiểm `base + delta = candidate`; 6 rejected cases hiện
  mới xác nhận rejection status. Bundle không có case container resource
  requests/limits hoặc array diff; hai vùng này có product tests riêng.
- Internal verification chạy trên cluster kind có sẵn, namespace riêng theo run
  ID và cleanup đã xác minh; run gần nhất `kind-20260922114440-17489` (I06-07,
  [evidence](verification/2026-09-22-imp009-kind-rerun.md)) xác nhận live
  container resources của ba seeded workload: backend declared, worker partial,
  frontend omitted.
- AWS verification đã tạo VPC/EKS/Aurora tối thiểu, chạy acceptance job flow,
  không tạo public LoadBalancer và cleanup 17/17 truy vấn theo run ID.
- Một lượt kiểm thử riêng đã deploy Backstage 1.53.1 qua UC-12/16 trên kind:
  PostgreSQL resource, VSO configuration, Preview → Deploy, Traefik Ingress,
  health và guest-auth API đều pass; namespace/PVC run-scoped đã cleanup.
  Đây là evidence thử nghiệm, không phải một deployment Backstage thường trực.
- Orchestrator có container image cho API (kèm kubectl) và Web Console (nginx).
  Trên kind, host Orchestrator đã tự deploy hai image này qua Web Console; bản
  in-cluster sau đó deploy acceptance app và cả 4 check đều pass. Bản
  self-hosted này chỉ là demo: nhận admin kubeconfig qua UC-12 secret; logical
  state hiện được giữ trong PostgreSQL, nhưng credential delivery vẫn là
  limitation của IMP-014.

Chi tiết từng lần chạy nằm trong [verification index](verification/README.md);
evidence lịch sử không chứng minh checkout hiện tại vẫn pass.

## Current architectural baseline

- Go product module nằm tại root `backend/`; Orchestrator Web Console nằm tại
  root `frontend/` theo ADR-005.
- Planning deterministic và side-effect free.
- UC-08 chỉ execute resource nodes; UC-06 apply workload sau khi có outputs.
- Historical `fleet-gitrepo` mode (T22 hiện chặn trước Git/cluster mutation vì
  chưa có explicit credential transport) ghi non-secret workload manifests vào private
  GitOps repo; Fleet triển khai workload, còn UC-08 vẫn trực tiếp quản lý
  namespace/resource. Harbor chứa image, không có GitHub source build trong
  đường này. Image-pull credential nằm ở namespace Secret, không nằm trong Git.
- Internal-kind public access dùng một Traefik Ingress cho mỗi Environment,
  được reconcile sau khi workload Ready. Traefik Service hiện chỉ là ClusterIP;
  muốn truy cập từ máy ngoài cluster cần port-forward/edge mapping và DNS hoặc
  hosts mapping. TLS và AWS ingress chưa được triển khai.
- Resource graph dùng edge `consumer -> provider`; provider được schedule trước.
- Application `aws-eks` sở hữu VPC/EKS application-scoped; `internal-k8s` dùng
  registered cluster và Environment ánh xạ namespace.
- Candidate Deployment Set chỉ trở thành current sau workload readiness.
- Resource provisioning hiện chạy theo full-style behavior cho mọi resource
  node trong plan; workload execution chỉ apply workload mục tiêu của request.
- UC-12 secret value lưu trong Vault KV và, ở Kubernetes VSO mode, được đồng bộ
  vào namespace-local Kubernetes Secret. State snapshot, read API, GitOps
  manifests, Pod spec và UC-09 view không chứa giá trị. Pod nhận biến qua
  `secretKeyRef` mà không cần startup script. Agent file delivery vẫn có thể
  chọn như legacy mode. Production cần etcd encryption, least-privilege RBAC,
  chính sách dọn Secret/bundle cũ và Vault HA/backup.
- Web Console và acceptance application frontend là hai artifact độc lập.

## Known limitations and release gate

- Backend hỗ trợ normalized PostgreSQL system of record qua
  `-database-url-file`; repository ghi trực tiếp dedicated tables, dùng foreign
  keys/unique constraints, fresh baseline identity/validation và PostgreSQL transaction (T28).
  In-memory/JSON adapter vẫn là lựa chọn mặc định cho local/test. PostgreSQL 16
  riêng đã Ready trên kind trong `orchestrator-system` với PVC/Secret; automated
  backup scheduling/retention vẫn là release gate.
- Terraform state chưa có durable backend; physical cloud names còn chứa run ID.
- Runtime chỉ execute ba embedded Terraform modules; remote source execution
  chưa thuộc baseline.
- `DeploymentDeltaSnapshot` chỉ giữ `deployment_id`; PostgreSQL FK/UNIQUE cấm
  orphan hoặc nhiều Snapshot cho một Deployment, và deferred constraint trigger
  bắt buộc Snapshot trước khi Deployment commit ở `PROVISIONING`, `DEPLOYING`
  hoặc `SUCCEEDED`.
- Container resources chỉ được validate theo shape (`cpu`/`memory`, non-empty
  string); Kubernetes quantity semantics và cặp request > limit khai báo chỉ
  được Kubernetes API kiểm khi apply. Nhánh request thiếu lấy limit cùng field
  (BR-11) và request vượt limit mới được unit-test, không có seeded workload
  nào chạy trên kind.
- Deploy API chạy đồng bộ. UC-12/16 pending Preview → Deploy hỗ trợ nhiều
  workload tuần tự, per-workload result và retry phần chưa hoàn thành, nhưng
  chưa có standalone Humanitec Delta/Set deployment lifecycle.
- Public Resource Definition/Score boundary chưa tương thích Humanitec/Score:
  driver ID/account/secret refs, remote source mapping, context extensions,
  nested probe, replicas extension và namespace output được phân loại tại
  [compatibility matrix](implementation/humanitec-compatibility.md).
- Conformance loader bỏ Definition thiếu criteria hoặc `criteria: []` khỏi
  challenge catalog và giữ criterion `{}` thành wildcard điểm 0; product planner
  vẫn từ chối catalog có Definition không có criterion.
- UC-00 có cookie session cho local/test, chưa có production-grade RBAC. UC-04
  chưa có AWS registration/durable credential flow. UC-05 standalone Preview và
  UC-07 update/remove đã có local verification. UC-02/03 đã có đăng
  ký catalog và PostgreSQL persistence nhưng chưa có đầy đủ lifecycle quản trị.
- UC-12 fake provider chỉ dành local/test và mất giá trị khi process restart.
  Vault trên kind dùng file storage PVC, cần unseal thủ công sau restart; chưa
  có HA, backup, rotation/revocation tự động hoặc production RBAC. Workload
  phải tự source Agent file bằng startup script; không tự động sửa image/entrypoint.
- Fleet GitRepo mode mới kiểm chứng trên một internal kind cluster với image
  BusyBox mẫu từ Harbor; GitOps writer dùng local clone và host Git credential,
  chưa có distributed lock hay production-grade reconciliation. Harbor hiện
  dùng Service IP và HTTP node config riêng cho kind; phải cập nhật cấu hình
  nếu Service IP đổi. AWS vẫn dùng direct Kubernetes delivery.
- Rollback, failure recovery, RBAC, audit và secret lifecycle nằm ngoài MVP.
- AWS happy path gần nhất là run `aws-20260921052038`, trước một số thay đổi
  planning cuối và trước khi seeded Scores khai báo container resources. Phải
  chạy lại `aws-verify.sh` trước release.

Các vấn đề deferred được theo dõi tại [backlog](backlog/README.md); khác biệt
thiết kế–implementation nằm tại
[known deviations](implementation/deviations.md).

## Accepted decisions and verified local hardening (2026-10-02)

New public Resource Type/Definition ID policy and Driver Inputs schema checking
are implemented in UC-02/03. UC-16 validates all non-virtual dependencies on
Save/import before mutation. Backend full test/build and frontend
typecheck/lint/72 tests/build passed; code review required and verified fixes
for malformed nested placeholders and namespace name supply. See
[local evidence](verification/2026-10-02-catalog-workload-validation-local.md). IMP-002 remains open for the residual
AWS onboarding and arbitrary-Type/runtime-pair boundaries.
AWS access key + Vault storage has been selected in
[ADR-009](architecture/decisions/ADR-009-aws-access-key-storage.md); the decision
resolves credential selection, not AWS registration/executor delivery. AWS
onboarding and live kind/AWS verification remain separate work.

## UC-16 Application-key picker delivered (2026-10-02)

Per-container Variables/Secrets checklists with same-name defaults, optional
aliases and separate resource/Service sources are accepted under UC-16
BR-15–BR-18 and implemented in the Console. Same-scope stale reload retains
edits; real Application/Environment/workload changes reset local selections
and ignore late old replies. Frontend typecheck/lint/107 tests/build and local
HTTP browser checks passed. See [picker evidence](verification/2026-10-02-workload-key-picker-local.md).
Runtime API, configuration storage and Preview/Deploy behavior are unchanged.

## Human UI verification on kind (2026-10-02)

The current UC-16 Application-key picker was exercised in a browser-driven
acceptance deployment on kind with Vault/VSO: backend/frontend deployed,
reference selections survived Edit and all diagnostic checks passed. A separate
Platform Engineer recording covered Type/Definition registration and validation,
actual Kubernetes connection verification to READY, then Developer Preview
consumption. Both human-paced full-window MP4s were reviewed and published;
see [execution evidence](verification/2026-10-02-human-ui-kind-recordings.md).
This adds live UI verification, not production RBAC or AWS onboarding delivery.

## UC-04 upload delivery (2026-10-06)

[Approved specification](usecase/UC-04/specification.md), realization/shared
design, contracts, schema, UI and traceability are aligned with the Kubernetes
upload implementation. [Verification](verification/2026-10-06-uc04-kubeconfig-upload.md)
records code review, Go/frontend gates, disposable PostgreSQL tests and the
published kind read-only upload video. Direct executor resolution and
restart/remove are locally tested with stub kubectl; live workload deployment
using uploaded credential was not exercised by this registration recording.

The [desktop UI review](verification/2026-10-06-uc04-connections-ui.md) covers
corrected Upload/Paste controls, readable form/action placement, aligned list
columns and keyboard focus, with a revised published registration demo.

Connection credential store is explicit configuration: durable Vault for real
execution/persistent state, memory only for fake/no-persistent-state tests,
default none returns 503 for upload registration. New registration does not
change Organization default; UC-03 matching Definition can consume the new
Connection. T22 removes host-context execution: Kubernetes seed without credential
is VERIFYING, upload is the only registration body, and context-only targets fail
closed. Fleet fixed-cluster mode is currently unavailable before Git/cluster
mutation for all targets; adding explicit Fleet credential delivery is separate
scope. Direct uploaded KUBECONFIG and Terraform-generated EKS path targets remain
supported. Existing HOST_CONTEXT PostgreSQL rows require the approved dev reset,
not silent conversion. AWS
registration and Terraform account credential resolution remain deferred.

Local Compose builds backend/frontend source by default and runs public
PostgreSQL/Vault images. Explicit bootstrap now registers `platform-vault` /
`Platform Vault` as an ordinary verified store, using the shared
Vault verifier and private platform credential store. Runtime resolves its opaque
credential reference; the application token file is bootstrap input only.
T23 removes legacy flags/global tokens, store backfill/conversion and legacy
domain/API/UI flags. New Environments select stores explicitly; retained old
dev data requires the separately approved T29 reset. Bootstrap is idempotent, checks endpoint identity and
can refresh only managed credentials after successful verification/CAS admission.
Workload delivery still requires Vault reachability and Kubernetes auth for the
selected cluster. [Normal-store verification](verification/2026-10-08-compose-vault-normal-store.md)
records the new implementation; the earlier legacy-bootstrap evidence is historical.

T24 removes store-backed Variable reads/materialization and the Legacy configuration
DTO/error code. Variables require metadata Value only; Secrets require explicit
owning store/ref only. JSON and PostgreSQL revision reads/writes reject mixed or
old Variable formats; no retained data conversion is performed. Variable CRUD/list
never resolves Vault, and store switching copies only desired Secrets while
preserving Variables and old immutable Secret refs. Local service/API tests prove
these boundaries; PostgreSQL opt-in/live Vault are not reverified. T28 owns the
fresh SQL constraint baseline and T29 the approved retained-dev reset.

## Historical Environment execution binding (2026-10-07; superseded by T26–T28)

Developer creates an Application from Name/Subdomain; staging/production start
UNCONFIGURED. Each Environment Settings explicitly sets one Organization READY
Connection once, then shows a locked binding even before deployment. Repeated
sets return 409; defaults never retarget or auto-bind. Independent profile/region
and mixed Kubernetes/AWS targets are supported. Preview/Deploy use the Environment
binding, reject unconfigured targets and Definition target conflicts safely.
Legacy SQL/JSON binding is backfilled/locked without changing existing namespace,
TargetRef or AWS application-scope state. New AWS VPC/EKS are Environment-scoped,
with bounded provider-safe names; AWS credential onboarding remains deferred.

[Verification/video](verification/2026-10-07-environment-connection-kind.md)
covers real kind upload, two independent logical bindings, refresh/restart lock,
unconfigured Preview rejection, staging backend/frontend/PostgreSQL deployment
and Vault/VSO diagnostic PASS. Both Connections use the same physical cluster;
production is bound but not deployed. AWS scope/name semantics are covered by
local tests, not a cloud run. Run-owned resources/private credentials were removed.
Final review and validation passed, including isolated PostgreSQL race tests,
deterministic seed preservation tests and run-owned browser cleanup checks.

Previous [Application-selection local evidence](verification/2026-10-07-application-connection-selection-local.md)
and [kind evidence](verification/2026-10-07-application-connection-selection-kind.md)
remain historical; their Application-wide selection is superseded by ADR-011.

## Environment secret stores and editable destinations — verified first delivery

[ADR-012](architecture/decisions/ADR-012-environment-stores-and-transitions.md)
replaces permanent selection lock with versioned editable per-Environment execution
and Secret Store Connections, atomic admission, Vault transfer and PostgreSQL
migration with downtime. The first direct-Kubernetes delivery is implemented and
verified by the [real-kind human recording](verification/2026-10-08-environment-stores-kind.md).

Backend/frontend code and canonical design are present. Initial compilation,
recovery fencing, PostgreSQL command, routing preflight and Preview impact defects
were corrected. Independent Go race/PostgreSQL integration/vet/build and frontend
typecheck/lint/153 tests/build checks passed on 2026-10-08. The full human UI flow
passed on real kind with two workload Vault stores, separate platform credential
storage, scoped tokens, distinct VSO auth mounts, PostgreSQL migration and actual
Ingress HTTP/data checks. The reviewed H.264 recording has 33 phase marks and
full decode/frame validation; test namespaces, temporary Vaults and credentials
were cleaned after the run.

Review corrected recovery polling after a failed attempt, idle-to-busy UI refresh,
configuration/draft submission guards, early application-ID capture, owned process
groups, bounded calls and deterministic transition ownership cleanup. Current
acceptance helpers explicitly select a store before secret writes and current
Connection runners follow editable ADR-012 behavior. IMP-017 is resolved for this
delivery. The live run proves two logical Connections on one physical kind
cluster; it does not prove distinct-cluster/DNS transfer or AWS/Aurora migration.
Recovery/fencing failure paths are covered by local tests rather than this happy-path
recording. Other updated legacy runners were syntax-checked, not all replayed live.
The pre-existing AWS integration-tag test has an EKSDescriptor argument mismatch; no live AWS run
is authorized or claimed. Prior set-once evidence remains historical.

## Implicit internal cluster binding — local and kind verified 2026-10-09

[ADR-013](architecture/decisions/ADR-013-implicit-existing-cluster.md) removes
per-cluster user Definition registration by binding an implicit existing-cluster
node to the Environment Connection. Product planner now uses this by default;
the reference harness explicitly retains reference matching. Execution admits a
reserved system Definition before FK-dependent writes; public registration and
foreign-content collisions fail closed. Existing descriptor/Application scope,
historical Definitions and target restoration remain compatible. New seeds no
longer create the authored internal cluster Definition. Console shows the
Environment Connection binding; namespace/PostgreSQL and cloud matching remain.

[Local evidence](verification/2026-10-09-implicit-existing-cluster-local.md) records
Go tests/build, targeted race suites, frontend typecheck/lint/154 tests/build,
syntax checks and fake-adapter browser create/deploy/restart with no per-cluster
Definition registration. IMP-019 is resolved for this delivery.
[Real kind evidence](verification/2026-10-09-k8s4f-live.md) additionally covers
retained `k8s-4f` registration in the Docker Console and a separate isolated
real-adapter Playwright FE/BE/PostgreSQL Deploy, diagnostic/job submission,
builtin binding and ownership-checked cleanup. No AWS or SQL-system-store
integration run was performed. FK ordering is locally simulated; existing
transition tests pass, without a new live transition proof. Persistent Console
workload Vault auth was not established by the isolated test.

The subsequent [retained dev deployment](verification/2026-10-09-k8s4f-dev-retained.md)
uses the persistent Docker Console to deploy sample FE/BE/PostgreSQL onto the
new physical `kind-k8s-4f` cluster. New Connection `K8S-4F` (`k8s-4f-2`), a
dedicated persistent workload Vault and VSO establish actual secret delivery;
readiness/PVC, diagnostics and reload persistence pass. At that checkpoint both sample attempt and
final Applications were deployed; T29 below supersedes their current retained state. The reviewed Vietnamese-captioned video
joins registration and successful deployment sessions explicitly. See the
[dev runbook](operations/k8s4f-dev.md) for URLs/restart and token lifetime limits.

## Console refactor foundation — T01

Thuật ngữ nhãn tiếng Việt đã chốt trong [glossary](GLOSSARY.md#nhãn-web-console-vi-01).
Runner refactor local và locator role/label theo trang đã implement; headed
sign-in/navigation/sign-out smoke, assertion-failure retention và signal cleanup
đã kiểm trên fake adapters. [Evidence T01](verification/2026-10-10-T01-refactor-foundation.md).
Các trang hiện giữ nhãn cũ tới task Việt hóa tương ứng; chưa có thay đổi matching,
API hoặc deploy retained K8S-4F trong T01.

## Public existing-cluster registration closed — T17 (2026-10-11)

Public Resource Definition registration rejects every `existing-cluster` driver
with validation 400 before insert; reserved system keys remain protected for all
drivers. Trusted builtin construction, executor and FK admission are unchanged.
Existing records/history remain readable; no retained database or cluster was
mutated. ReferenceCluster conformance remains unchanged.
See [T17 local evidence](verification/2026-10-11-T17-existing-cluster-registration.md).

## Console refactor T18 (2026-10-11)

GET `/api/v1/render-bundles` dành PE/Admin với session Organization trả danh
sách ID/version/status AVAILABLE an toàn từ registry bất biến của backend,
được sắp theo ID và dùng cùng availability predicate với registration.
Trang Mẫu dựng ứng dụng chọn từ API: một bundle tự chọn khi draft chưa chọn,
nhiều bundle cần chọn tường minh, rỗng/loading/error khóa đăng ký và có hướng
dẫn/retry Việt. Refresh không thay lựa chọn cũ; option mất availability vẫn
hiển thị và không submit. POST giữ nguyên payload và revalidate trước insert.

[Local verification](verification/2026-10-11-T18-render-bundles.md) ghi HTTP
permissions/metadata/no-insert, frontend selector guards và headed fake scenario.
Registry một bundle/registration/Preview dùng backend local thật với fake adapters
và binary stub; 0/n/error là projection mocks. Không chứng minh renderer binary
thật hoặc live cluster/cloud; PostgreSQL opt-in không chạy. Matching/ADR-010 giữ nguyên.

## Console refactor T19 (2026-10-11)

POST `/api/v1/resource-definitions/matching-preview` đã có PE/Admin/session-org
gate, bounded strict safe input, scoped read-only snapshot app/env/READY target/
catalog và candidate replacement chỉ trong memory. Trả eligibility, best
criterion/score, unique winner hoặc winning ties, winning Connection conflict,
incomplete/system-binding/native-renderer và deterministic snapshot marker/version.
Weights/profile/ties, match.go, reference fixtures và full graph Preview không đổi.
Không write, inspect, render, provision hoặc secret resolution; kết quả không
bảo đảm registration/deploy, không phát token.

Hai form resource/template có control chung “Kiểm tra matching” dùng draft chưa
lưu/context mẫu. Input/context/metadata/session đổi thì invalidate result/pending;
transport errors tách khỏi matching states, retry tường minh. INCOMPLETE không
bị diễn giải thành không khớp; system node không do authored criteria điều khiển.
[Local verification](verification/2026-10-11-T19-matching-preview.md):
Go/conformance/build và frontend checks pass local; browser dùng backend thật
với fake adapters/state tạm, transport 500 là projection mock. SQL/kind/AWS
opt-in và retained K8S-4F không được chạy hoặc thay đổi trong T19.

## Console error contract T20 (2026-10-12)

Các handler Console dùng stable error code và câu an toàn cố định; field và
planner params chỉ từ allowlist, không echo submitted values/raw provider/decode
text. Unknown cause thành lỗi nội bộ, không phân loại bằng prefix tiếng Anh.
Client chỉ dùng code/status và metadata đã kiểm để chọn hướng dẫn Việt.
BUSY không bị coi là stale; reload stale chỉ báo thành công sau GET thành công,
failed reload giữ edits và khóa writes tới khi reload/review, scope đổi reset gate.
Retry mutation vẫn tường minh; không đổi scoping/CAS/planner/renderer semantics.

[Verification T20](verification/2026-10-12-T20-console-errors.md) ghi checks và
headed local fake evidence; BUSY/unknown/copy failure là projection mocks.
Không PostgreSQL/kind/AWS opt-in hoặc retained K8S-4F mutation trong task này.

## T21 — Environment context aliases removed (2026-10-12)

Product context now exposes profile/region only as `env.profile`/`env.region`
from the selected Environment. `app.profile`/`app.region` return unknown context
errors, including whole and interpolated placeholders; no rewrite/fallback.
Application identity and infrastructure scope/naming remain unchanged. Go suite,
build and uncached planning/conformance passed locally; no SQL opt-in or external
mutation. See [T21 evidence](verification/2026-10-12-T21-context-aliases.md).

## T22 — Explicit Kubernetes credential boundary

[Local verification](verification/2026-10-12-T22-explicit-kubeconfig.md) covers
upload-only registration, missing/foreign credential rejection, seed VERIFYING,
scoped target restore/cancellation cleanup and Fleet rejection before side
effects. Go/conformance/build, frontend 425 tests/build and headed fake T01/T07
plus application-connection local restart pass. SQL integration/live checks
were not run. Retained dev resources remain untouched; old HOST_CONTEXT DB
requires approved reset. T28 removes the historical SQL startup migration chain.

## Public route cleanup T25 (2026-10-12)

[Local verification](verification/2026-10-12-T25-public-routes.md): product chỉ
nhận `service.publicRoutes`; field `publicPort` bị từ chối kể cả rỗng/null/cùng
routes, với HTTP `INVALID_SCORE` và hướng dẫn Việt an toàn. Không convert draft
cũ, không route fallback; dữ liệu cũ chứa alias cần sửa/reset dev theo T29.
Decoder giữ strict nested validation; literal binding security, nhiều path,
port ownership, no-op/route-only retry và T08 controls giữ nguyên. Full Go/build/
conformance, frontend 429 tests và headed T25/T08 pass với fake adapters.
Không kiểm PostgreSQL opt-in, kind/AWS/Vault thật hoặc mutate retained state.

## T26 — Application metadata cleanup (2026-10-12)

Application domain and SQL/JSON Save/Get/List now carry only identity/current
metadata; execution profile/Connection/region/runtime and configuration provider
fields are removed. Environment bindings remain independent; Application saves
and JSON restart preserve historical Deployment targets/generations and workload
TargetRef snapshots. Application-to-Environment backfill is removed; retained
meaningful/malformed old target data fails startup with a safe reset-required error.
At the T26 checkpoint, transitional SQL migration 9 prepared fresh state; PostgreSQL integration was
not executed (no authorized disposable DB). Full uncached Go/conformance/build,
targeted race and docs/PlantUML checks pass. No browser/UI change or external mutation.
See [T26 evidence](verification/2026-10-12-T26-application-metadata.md).
Retained dev state requires the separately approved T29 reset before use. T27
scope/seed cleanup is described below; T28 replaces the transitional SQL chain.

## T27 — Environment-only infrastructure scope (2026-10-12)

Removed the LEGACY_APPLICATION product enum and cloud identity/runtime branches.
Execution binding scope is ENVIRONMENT only; domain/planner/target and SQL/JSON
Save/Bind/reopen reject explicit invalid scopes without conversion or scope-value
leakage. Current generation-0 namespace/descriptor/provider-name identities and
later-generation isolation remain; trusted internal cluster Application resource
scope is preserved under ADR-013. Settings drops its migrated-binding label.

Acceptance seed binds ENVIRONMENT explicitly and inserts missing Definitions only;
exact old seeded templates are no longer upgraded. Conformance needs no additional
historical path/name adapter; all 33 fixtures retain their existing outcomes and
matching weights/ties. At the T27 checkpoint, SQL migration 10 supplied the strict CHECK after historical
ledger replay; the migration-6 literal is replaced before serving and removed with
T28 baseline. PostgreSQL integration was unexecuted at the T27 checkpoint;
the current baseline is verified by T28 below. Retained dev state requires the separately approved T29 reset before
use; no retained database, cluster, Vault or AWS resources were mutated.

Full uncached Go/conformance/build and targeted race suites pass; frontend 429
checks and headed fake T07 regression pass. Video decode/frame/caption review
completed. See [T27 evidence](verification/2026-10-12-T27-environment-scope.md).

## T28 — Fresh persistence baseline (2026-10-12)

Code replaces PostgreSQL migration replay/backfills with empty-schema atomic
initialization and current baseline/structural validation on boot. Older ledgers,
unmarked state and schema drift fail before serving without repair. JSON requires
the current complete format and explicit current metadata; no defaults, Connection
inference or history reconstruction. Current Snapshot/CAS/fencing/generation and
Variable/Secret boundaries remain.

Full uncached Go suite/build/conformance and targeted races pass both locally and
on authorized isolated PostgreSQL 16. Fresh/reopen/rollback, schema drift rejection,
SQL constraints, registration and multi-backend ownership pass with no skipped
tests. Synthetic dump/restore and owned cleanup pass; failed first-run fixture
checks remain in evidence. T28 is complete. No retained DB/Vault/cluster was mutated;
T29 below completes the approved reset/rebuild for the retained dev stack.
Other historical databases still require an explicitly scoped reset before use.
See [T28 evidence](verification/2026-10-12-T28-fresh-baseline.md).


## T29 — Approved dev K8S-4F reset/rebuild (2026-10-12)

Real IDP backup and both workload PostgreSQL backups passed isolated restore,
row-count/job-checksum checks before resetting only the IDP database. Current
fresh baseline serves the new application `k8s4f-t29-20261012-r1`, ID
`47fb34b4-2d6e-493e-a2c5-880837abfc1c`; staging uses KUBECONFIG Connection
`k8s-4f`, ordinary Store `k8s-4f-vault`, ENVIRONMENT scope, generation 0.
Frontend/backend/PostgreSQL are Ready, PVC Bound; empty DB before first job,
four diagnostics, job/reload persistence and IDP restart persistence passed.
App URL is <http://localhost:18481/>; Console remains <http://localhost:3001/ui/>.

Both old namespaces/PVC remain with FE/BE at zero and PostgreSQL 1/1; old port
18480 no longer serves, and their metadata/history is not in the new IDP.
Vault volumes, VSO, clusters and unrelated systems are preserved. Private backups
are retained until 2026-11-11T04:05:55Z with pinned old-image recovery prepared.
One pre-deploy runner locator failure and its video are preserved; explicit
same-run continuation passed without replaying Deploy. Headed real video has
Vietnamese captions; it is separate from the first Connection-registration phase.
See [T29 evidence](verification/2026-10-12-T29-dev-rebuild.md) and
[dev runbook](operations/k8s4f-dev.md). No production/AWS run or new recovery
execution is claimed; live backup restore-smoke succeeded, offline recovery guards
pass. The sample has no worker, so its diagnostic job stays PENDING.

## T30 — Key rename and desired reference update (2026-10-12)

[Local verification](verification/2026-10-12-T30-key-rename.md): Settings có
checkbox mặc định bỏ chọn “Đổi tên và cập nhật tham chiếu”. Opt-in commits
config revision + affected drafts nguyên tử dưới config/env/draft/current-set
CAS/BUSY; deployed consumers không có draft được reconstruct/round-trip verified.
Giữ aliases/many-to-one, advanced fields, unrelated refs/context/escaped literals
và opaque Secret refs, không đọc/copy Secret bytes. Pending deletes không bị
hồi sinh; rename-only/delete giữ references. Runtime/current/applied state không
đổi tới Preview → Deploy tường minh; Preview cũ stale. Unsafe reconstruction
fail closed với hướng dẫn Việt. Production Preview CTA giữ đúng Environment.

Full Go/conformance/build, targeted races, frontend 444 tests/gates, headed T30
và T07/T08 regression pass. Video dùng real product API/planner với fake adapters
và memory state/store; T30 BUSY là projection mock. PostgreSQL atomic command và
opt-in rollback/race/reopen tests đã có nhưng chưa được thực thi vì chưa có
approval DB disposable cho T30; không claim PostgreSQL/live rollout proof.

## T31 — Typed Definition form and capability metadata (2026-10-12)

[Local verification](verification/2026-10-12-T31-typed-definitions.md):
GET capability metadata role/org-gated và chỉ đọc cung cấp cặp type/driver/module,
types/defaults/required/param sources và preset từ policy/embedded inspector.
Namespace name source giữ BR-12 required string; executor-owned/credentials
không có controls editable. Loại tài nguyên và CTA dùng metadata authoritative,
không fallback whitelist frontend khi tải lỗi.

Form variables/provision typed giữ JSON types, placeholders, default/param omission,
presets opt-in và draft từng cặp. Advanced JSON giữ lossless known fields hoặc
giữ mode khi không biểu diễn an toàn; params workload vẫn ưu tiên hơn fallback
Definition. Registration/planner tiếp tục validate independently; matcher/ranking/
ties, providers/renderer và fixtures không đổi.

Go/build/conformance, catalog/HTTP race, frontend 464 tests/gates và headed
T31/T02/T04/T16/T19 pass. T31 full Score Preview kiểm consumer → provider và
provider-first batches, không Deploy. Fake adapters/disposable state; metadata
500 projection mock, bool/map dùng synthetic metadata unit coverage. Video
decoded/frame/caption reviewed, failed attempts và bản gốc được giữ. Không chạy
SQL opt-in hoặc mutate retained/cloud/cluster; không claim live provider proof.
