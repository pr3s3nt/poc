---
id: PROJECT-CURRENT-STATE
artifact: project-status
status: current
last_reviewed: 2026-09-23
---

# Current project state

## Lifecycle position

Phase 1–5 của Unified Process cho UC-01..UC-09 đã hoàn thành và design gate đã PASS.
UC-00 được bổ sung sau gate và cần targeted design-gate review trước implementation. Executable
architecture cho UC-06/UC-08 cùng query path tối thiểu của UC-09 đã được hiện
thực. Internal happy path đã được kiểm chứng trên kind; cloud happy path đã được
kiểm chứng trên AWS với VPC, EKS và Aurora rồi cleanup.

Active iteration là
[I00-00 — UC-00 and UC-01 developer onboarding](iterations/M00-developer-onboarding/I00-00-uc00-uc01-developer-onboarding/README.md)
thuộc [M00-a — Developer onboarding](iterations/M00-developer-onboarding/README.md).
M00-a đã có UI design và React prototype cho sign-in/self-service Application;
API, session và persistence vẫn phải được hiện thực trước khi coi UC-00/UC-01
là executable. I06-04 được reprioritize sang deferred.
[M01 — Contract hardening](iterations/M01-contract-hardening/README.md) đã hoàn
thành: I06-05 đóng IMP-010, I06-06 đóng IMP-008, I06-07 đóng IMP-009.

## Use-case delivery state

| UC | State | Current conclusion |
|---|---|---|
| UC-00 | Implemented; local/test baseline | Fixed seeded `developer` account, opaque HttpOnly cookie session, session restore và sign-out đã có; production profile không seed test account. |
| UC-01 | Implemented; local/test baseline | Authenticated Developer có thể list/create/get Application qua API; service tự sinh ID, staging/production, empty Deployment Sets và namespace identities. `acme` resolve `internal-cluster` như platform default, không hiển thị target chooser. |
| UC-02 | Designed; seed-backed baseline | Resource Type catalog tồn tại trong seed/planner; API/UI quản trị chưa có. |
| UC-03 | Designed; seed-backed baseline | Resource Definition, matching và contract validation đã chạy trong planner; API/UI quản trị chưa có. |
| UC-04 | Designed; partial execution support | Connection/target seed và adapters kind/AWS đã chạy; registration/verification UI và persistence thật chưa có. |
| UC-05 | Planning baseline; accepted contract gaps | Planner pipeline và scoped conformance đã có; Planner sinh transient Humanitec-shaped Delta và giữ container requests/limits nguyên văn trong Candidate Set (I06-07); preview/API/UI chưa hoàn thiện. |
| UC-06 | Executable baseline; partially conformant | HTTP → plan → UC-08 → target workload apply đã pass kind/AWS; mỗi Deployment persist immutable `DeploymentDeltaSnapshot` (I06-06); container requests/limits từ Score tới live Deployment theo BR-11 (I06-07); kind chỉ kiểm ba seeded case declared/partial/omitted, nhánh limit fallback và request vượt limit chỉ có unit test. |
| UC-07 | Partial | Planner hỗ trợ before/shared rules và sinh Humanitec-shaped `modules.add/remove/update` + `shared` Delta; update/remove system flow và UI chưa hoàn thiện. |
| UC-08 | Implemented and E2E verified | Kubernetes và Terraform resource execution, output propagation và persistence baseline đã pass. |
| UC-09 | Partially implemented; deferred in M02/I06-04 | Deployment list/detail, Delta Snapshot document, graph, batches, resources, workloads và redacted outputs đã có; history/filter/state comparison là scope còn lại của I06-04 sau M00-a. |
| UC-12 | Draft only | Application-level Variables & Secrets configuration chưa được implement; secret lifecycle/contract còn cần chốt. |
| UC-16 | Specification and UI approved; not implemented | Form/Score import, pending workload configuration, UC-12 references và workload Service references chưa có trong API/Web Console. |

## Executable baseline

- Go HTTP API, application services, planner, resource executors, Kubernetes
  deployer và JSON snapshot store nằm dưới `backend/`.
- Web Console React/TypeScript hiện là M00-a prototype cho UC-00/UC-01:
  sign-in, Applications list/create và Application home; chưa gọi API. Deploy
  và Deployment Details UI sẽ được làm lại khi UC-06/UC-09 được lên lịch.
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
- Resource graph dùng edge `consumer -> provider`; provider được schedule trước.
- Application `aws-eks` sở hữu VPC/EKS application-scoped; `internal-k8s` dùng
  registered cluster và Environment ánh xạ namespace.
- Candidate Deployment Set chỉ trở thành current sau workload readiness.
- Resource provisioning hiện chạy theo full-style behavior cho mọi resource
  node trong plan; workload execution chỉ apply workload mục tiêu của request.
- Secret value không được persist trong state snapshot hoặc trả qua UC-09 view.
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
- Deploy API chạy đồng bộ, apply một workload mục tiêu và chưa có incremental
  mode hoặc standalone Humanitec Delta/Set deployment lifecycle.
- Public Resource Definition/Score boundary chưa tương thích Humanitec/Score:
  driver ID/account/secret refs, remote source mapping, context extensions,
  nested probe, replicas extension và namespace output được phân loại tại
  [compatibility matrix](implementation/humanitec-compatibility.md).
- Conformance loader bỏ Definition thiếu criteria hoặc `criteria: []` khỏi
  challenge catalog và giữ criterion `{}` thành wildcard điểm 0; product planner
  vẫn từ chối catalog có Definition không có criterion.
- UC-00 chưa có authentication/authorization boundary; UC-01..UC-05 và UC-07
  chưa có đầy đủ product management flow/UI.
- Rollback, failure recovery, RBAC, audit và secret lifecycle nằm ngoài MVP.
- AWS happy path gần nhất là run `aws-20260921052038`, trước một số thay đổi
  planning cuối và trước khi seeded Scores khai báo container resources. Phải
  chạy lại `aws-verify.sh` trước release.

Các vấn đề deferred được theo dõi tại [backlog](backlog/README.md); khác biệt
thiết kế–implementation nằm tại
[known deviations](implementation/deviations.md).
