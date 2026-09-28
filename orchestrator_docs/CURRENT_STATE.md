---
id: PROJECT-CURRENT-STATE
artifact: project-status
status: current
last_reviewed: 2026-09-28
---

# Current project state

## Lifecycle position

Phase 1–5 của Unified Process cho UC-01..UC-09 đã hoàn thành và design gate đã PASS.
UC-00 được bổ sung sau gate và đã có local/test implementation. Executable
architecture cho UC-06/UC-08 cùng query path tối thiểu của UC-09 đã được hiện
thực. Internal happy path đã được kiểm chứng trên kind; cloud happy path đã được
kiểm chứng trên AWS với VPC, EKS và Aurora rồi cleanup.

Active iteration là
[I00-00 — UC-00 and UC-01 developer onboarding](iterations/M00-developer-onboarding/I00-00-uc00-uc01-developer-onboarding/README.md)
thuộc [M00-a — Developer onboarding](iterations/M00-developer-onboarding/README.md).
M00-a đã có React UI, API, session và JSON snapshot persistence cho
sign-in/self-service Application ở local/test baseline. I06-04 được
reprioritize sang deferred.
[M01 — Contract hardening](iterations/M01-contract-hardening/README.md) đã hoàn
thành: I06-05 đóng IMP-010, I06-06 đóng IMP-008, I06-07 đóng IMP-009.

## Use-case delivery state

| UC | State | Current conclusion |
|---|---|---|
| UC-00 | Implemented; local/test baseline | Fixed seeded `developer` account, opaque HttpOnly cookie session, session restore và sign-out đã có; production profile không seed test account. |
| UC-01 | Implemented; local/test baseline | Authenticated Developer có thể list/create/get Application qua API; service tự sinh ID, staging/production, empty Deployment Sets và namespace identities. `acme` resolve `internal-cluster` như platform default, không hiển thị target chooser. |
| UC-02 | Designed; seed-backed baseline | Resource Type catalog tồn tại trong seed/planner; API/UI quản trị chưa có. |
| UC-03 | Designed; seed-backed baseline | Resource Definition, profile eligibility trước matching và contract validation đã chạy trong planner; seeded PostgreSQL Definitions dùng được cho Application mới cùng profile. API/UI quản trị chưa có. |
| UC-04 | Designed; partial execution support | Connection/target seed và adapters kind/AWS đã chạy; registration/verification UI và persistence thật chưa có. |
| UC-05 | Pending-change Preview implemented; broader contract gaps | Planner pipeline, transient Humanitec-shaped Delta và read-only multi-workload Preview/API/UI đã có; broader UC-05 contract coverage còn hạn chế. |
| UC-06 | Executable baseline; partially conformant | HTTP → plan → UC-08 → target workload apply đã pass kind/AWS; optional Fleet GitRepo adapter cho internal kind đã pass Harbor-image deployment. Environment Ingress nhiều path và Fleet `_routes` bundle đã pass kind test với hai workload BusyBox, Fleet revision/prune và cleanup; route failure có route-only retry (unit test), Backstage cụ thể chưa kiểm chứng. DNS/TLS/chuyển controller ra ngoài chưa có. Mỗi Deployment persist immutable `DeploymentDeltaSnapshot`; UC-12/16 pending changes deploy theo Preview token với per-workload result. |
| UC-07 | Update/remove executable for UC-16 | Planner hỗ trợ before/shared rules và Delta; Preview → Deploy update/remove workload trên kind, có partial retry. Fleet GitRepo remove đã pass kind; broader lifecycle UI/history còn thiếu. |
| UC-08 | Implemented and E2E verified | Kubernetes và Terraform resource execution, output propagation và persistence baseline đã pass. |
| UC-09 | Partially implemented; deferred in M02/I06-04 | Deployment list/detail, Delta Snapshot document, graph, batches, resources, workloads và redacted outputs đã có; history/filter/state comparison là scope còn lại của I06-04 sau M00-a. |
| UC-12 | MVP path implemented on kind | Settings UI/API, immutable desired/applied revisions, Vault KV v2 adapter, scoped backend/workload policies và VSO → namespace Secret → Pod `secretKeyRef` đã pass kind; secret không xuất hiện trong read API, snapshot hoặc Pod spec. Production secret lifecycle/HA chưa có. |
| UC-16 | MVP path implemented | Form/Score import, typed resource params, public path + Service port rows, draft save/delete/undo, references, edit deployed workload bằng reconstructed Score, Preview → Deploy và partial retry đã có. Preview bỏ qua no-op draft nhưng giữ UC-12 revision update. Kind đã kiểm tra multi-path BusyBox `/` + `/api`, Fleet route và no-op Pod UID; Backstage cụ thể và broader update/cloud path chưa kiểm chứng. |

## Executable baseline

- Go HTTP API, application services, planner, resource executors, Kubernetes
  deployer và JSON snapshot store nằm dưới `backend/`.
- Web Console React/TypeScript có sign-in, Applications list/create/home,
  UC-12 Settings, UC-16 editor và Application Preview/Deploy panel. Deployment
  Details UI chưa được nối vào Web Console.
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

- System-of-record hiện là in-memory map cộng JSON snapshot; PostgreSQL adapter
  chưa được hiện thực.
- Terraform state chưa có durable backend; physical cloud names còn chứa run ID.
- Runtime chỉ execute ba embedded Terraform modules; remote source execution
  chưa thuộc baseline.
- `DeploymentDeltaSnapshot` được persist trong JSON snapshot store với
  association một-một tới Deployment; store chỉ bắt buộc Snapshot khi
  Deployment rời `PLANNING`, chưa enforce `NOT NULL` của schema (IMP-011).
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
- UC-00 có cookie session cho local/test, chưa có production-grade RBAC. UC-02..UC-05
  và UC-07 chưa có đầy đủ product management flow/UI.
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
