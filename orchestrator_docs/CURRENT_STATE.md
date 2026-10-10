---
id: PROJECT-CURRENT-STATE
artifact: project-status
status: current
last_reviewed: 2026-10-10
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
| UC-04 | Kubernetes kubeconfig onboarding implemented and verified | Platform Engineer/Admin nhập tên, upload/dán kubeconfig, inspect/chọn context rồi verify API/RBAC read-only và lưu `READY`. Vault KV v2 lưu scoped credential; metadata/name/authentication type ở PostgreSQL, migration/race/reopen đã pass isolated local. Executor resolve credential bằng opaque target identity; host-context cũ tương thích. Live kind upload và video đã pass ngày 2026-10-06; AWS infrastructure identity onboarding vẫn future scope theo ADR-009. |
| UC-05 | Implemented; locally verified | Standalone Score Preview service, authenticated scoped API và Console trả Delta/Candidate/graph/matches/batches/classification, consistent snapshot chung với direct Deploy, safe public projection và no-mutation tests. Pending-change Preview vẫn riêng; UI-only human-paced recording đã pass. Không chạy Terraform plan thật. |
| UC-06 | Executable baseline; partially conformant | HTTP → plan → UC-08 → target workload apply đã pass kind/AWS; optional Fleet GitRepo adapter cho internal kind đã pass Harbor-image deployment. Environment Ingress nhiều path và Fleet `_routes` bundle đã pass kind test với hai workload BusyBox, Fleet revision/prune và cleanup; route failure có route-only retry (unit test), Backstage cụ thể chưa kiểm chứng. DNS/TLS/chuyển controller ra ngoài chưa có. Mỗi Deployment persist immutable `DeploymentDeltaSnapshot`; UC-12/16 pending changes deploy theo Preview token với per-workload result. |
| UC-07 | Implemented; locally verified | UC-16 scoped Preview → Deploy update/remove, before/shared invariants, final Service validation, confirmation/Undo và busy/stale/reload UI states. READY resource marking UNREFERENCED commit cùng current set/SUCCEEDED; PostgreSQL rollback/reopen và UI-only recording pass. Không destroy/rollback; Application-wide cleanup ngoài scope. Fleet remove kind evidence là lịch sử, chưa rerun live cho mốc này. |
| UC-08 | Implemented and E2E verified | Kubernetes và Terraform resource execution, output propagation và persistence baseline đã pass. |
| UC-09 | Implemented; locally E2E verified | Authenticated Organization/Application/Environment-scoped history/filter và detail; consistent read snapshots, per-Deployment workload history, fail-closed output redaction và không trả resolved inputs. React loading/error/not-found/retry và browser redeploy/restart checks đã pass. Comparison và live status ngoài scope. |
| UC-12 | MVP path implemented on kind | Settings UI/API, immutable desired/applied revisions, Vault KV v2 adapter, scoped backend/workload policies và VSO → namespace Secret → Pod `secretKeyRef` đã pass kind; secret không xuất hiện trong read API, snapshot hoặc Pod spec. Production secret lifecycle/HA chưa có. |
| UC-16 | MVP path implemented | Form/Score import, typed resource params, public path + Service port rows, draft save/delete/undo, references, edit deployed workload bằng reconstructed Score, Preview → Deploy và partial retry đã có. Preview bỏ qua no-op draft nhưng giữ UC-12 revision update. Kind đã kiểm tra multi-path BusyBox `/` + `/api`, Fleet route và no-op Pod UID; Backstage cụ thể và broader update/cloud path chưa kiểm chứng. |

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
- Optional `fleet-gitrepo` mode ghi non-secret workload manifests vào private
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
  keys/unique constraints, versioned migration ledger và PostgreSQL transaction.
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
Connection. Fleet fixed-cluster mode rejects credential-backed targets. AWS
registration and Terraform account credential resolution remain deferred.

Local Compose builds backend/frontend source by default and runs public
PostgreSQL/Vault images. Explicit bootstrap now registers `platform-vault` /
`Platform Vault` as an ordinary verified store (legacy=false), using the shared
Vault verifier and private platform credential store. Runtime resolves its opaque
credential reference; the application token file is bootstrap input only.
Existing Compose compatibility records convert in place with store identity,
Environment selections and immutable secret references preserved. New Environments
select stores explicitly. Bootstrap is idempotent, checks endpoint identity and
can refresh only managed credentials after successful verification/CAS admission.
Workload delivery still requires Vault reachability and Kubernetes auth for the
selected cluster. [Normal-store verification](verification/2026-10-08-compose-vault-normal-store.md)
records the new implementation; the earlier legacy-bootstrap evidence is historical.

## Environment execution binding (2026-10-07)

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
readiness/PVC, diagnostics and reload persistence pass. Both sample attempt and
final Applications remain deployed. The reviewed Vietnamese-captioned video
joins registration and successful deployment sessions explicitly. See the
[dev runbook](operations/k8s4f-dev.md) for URLs/restart and token lifetime limits.

## Console refactor foundation — T01

Thuật ngữ nhãn tiếng Việt đã chốt trong [glossary](GLOSSARY.md#nhãn-web-console-vi-01).
Runner refactor local và locator role/label theo trang đã implement; headed
sign-in/navigation/sign-out smoke, assertion-failure retention và signal cleanup
đã kiểm trên fake adapters. [Evidence T01](verification/2026-10-10-T01-refactor-foundation.md).
Các trang hiện giữ nhãn cũ tới task Việt hóa tương ứng; chưa có thay đổi matching,
API hoặc deploy retained K8S-4F trong T01.
