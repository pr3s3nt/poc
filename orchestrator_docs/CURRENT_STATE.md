---
id: PROJECT-CURRENT-STATE
artifact: project-status
status: current
last_reviewed: 2026-09-30
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
chưa đóng; identifier/Driver Inputs policy còn mở. Lựa chọn
credential storage cho AWS Connection cần xác nhận trước phần implementation
phụ thuộc quyết định đó.
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
| UC-04 | Local/kind Kubernetes registration implemented | Platform Engineer/Admin đăng ký cluster ID + host kube context qua API/UI; verifier kiểm tra API/RBAC đọc-only rồi persist connection `READY` trong PostgreSQL theo Organization. AWS registration và durable credential store chưa có. |
| UC-05 | Implemented; locally verified | Standalone Score Preview service, authenticated scoped API và Console trả Delta/Candidate/graph/matches/batches/classification, consistent snapshot chung với direct Deploy, safe public projection và no-mutation tests. Pending-change Preview vẫn riêng; UI-only human-paced recording đã pass. Không chạy Terraform plan thật. |
| UC-06 | Executable baseline; partially conformant | HTTP → plan → UC-08 → target workload apply đã pass kind/AWS; optional Fleet GitRepo adapter cho internal kind đã pass Harbor-image deployment. Environment Ingress nhiều path và Fleet `_routes` bundle đã pass kind test với hai workload BusyBox, Fleet revision/prune và cleanup; route failure có route-only retry (unit test), Backstage cụ thể chưa kiểm chứng. DNS/TLS/chuyển controller ra ngoài chưa có. Mỗi Deployment persist immutable `DeploymentDeltaSnapshot`; UC-12/16 pending changes deploy theo Preview token với per-workload result. |
| UC-07 | Implemented; locally verified | UC-16 scoped Preview → Deploy update/remove, before/shared invariants, final Service validation, confirmation/Undo và busy/stale/reload UI states. READY resource marking UNREFERENCED commit cùng current set/SUCCEEDED; PostgreSQL rollback/reopen và UI-only recording pass. Không destroy/rollback; Application-wide cleanup ngoài scope. Fleet remove kind evidence là lịch sử, chưa rerun live cho mốc này. |
| UC-08 | Implemented and E2E verified | Kubernetes và Terraform resource execution, output propagation và persistence baseline đã pass. |
| UC-09 | Implemented; locally E2E verified | Authenticated Organization/Application/Environment-scoped history/filter và detail; consistent read snapshots, per-Deployment workload history, fail-closed output redaction và không trả resolved inputs. React loading/error/not-found/retry và browser redeploy/restart checks đã pass. Comparison và live status ngoài scope. |
| UC-12 | MVP path implemented on kind | Settings UI/API, immutable desired/applied revisions, Vault KV v2 adapter, scoped backend/workload policies và VSO → namespace Secret → Pod `secretKeyRef` đã pass kind; secret không xuất hiện trong read API, snapshot hoặc Pod spec. Production secret lifecycle/HA chưa có. |
| UC-16 | MVP path implemented | Form/Score import, typed resource params, public path + Service port rows, draft save/delete/undo, references, edit deployed workload bằng reconstructed Score, Preview → Deploy và partial retry đã có. Preview bỏ qua no-op draft nhưng giữ UC-12 revision update. Kind đã kiểm tra multi-path BusyBox `/` + `/api`, Fleet route và no-op Pod UID; Backstage cụ thể và broader update/cloud path chưa kiểm chứng. |

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
