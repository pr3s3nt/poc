---
id: PROJECT-CURRENT-STATE
artifact: project-status
status: current
last_reviewed: 2026-09-21
---

# Current project state

## Lifecycle position

Phase 1–5 của Unified Process đã hoàn thành và design gate đã PASS. Executable
architecture cho UC-06/UC-08 cùng query path tối thiểu của UC-09 đã được hiện
thực. Internal happy path đã được kiểm chứng trên kind; cloud happy path đã được
kiểm chứng trên AWS với VPC, EKS và Aurora rồi cleanup.

Active iteration là
[I06-04 — UC-09 observability](iterations/I06-04-uc09-observability.md).

## Use-case delivery state

| UC | State | Current conclusion |
|---|---|---|
| UC-01 | Designed; seed-backed baseline | Domain/seed và application listing phục vụ deploy đã có; CRUD quản trị và UI đầy đủ chưa có. |
| UC-02 | Designed; seed-backed baseline | Resource Type catalog tồn tại trong seed/planner; API/UI quản trị chưa có. |
| UC-03 | Designed; seed-backed baseline | Resource Definition, matching và contract validation đã chạy trong planner; API/UI quản trị chưa có. |
| UC-04 | Designed; partial execution support | Connection/target seed và adapters kind/AWS đã chạy; registration/verification UI và persistence thật chưa có. |
| UC-05 | Planning core implemented; product flow missing | Planner pipeline và conformance đã có; preview operation/API/UI chưa hoàn thiện. |
| UC-06 | Implemented and E2E verified | Deploy qua HTTP → plan → UC-08 → workload apply; kind và AWS happy path đã pass. |
| UC-07 | Partial | Planner hỗ trợ before/shared update rules; update/remove system flow và UI chưa hoàn thiện. |
| UC-08 | Implemented and E2E verified | Kubernetes và Terraform resource execution, output propagation và persistence baseline đã pass. |
| UC-09 | Partially implemented; active | Deployment list/detail, graph, batches, resources, workloads và redacted outputs đã có; history/filter/state comparison chưa hoàn thiện. |

## Executable baseline

- Go HTTP API, application services, planner, resource executors, Kubernetes
  deployer và JSON snapshot store nằm dưới `backend/`.
- Web Console React/TypeScript có Deploy và Deployment Details.
- Planner product conformance chạy đủ 33 fixture: 27 accepted cases so sánh
  artifact; 6 rejected cases hiện mới xác nhận rejection status.
- Internal verification chạy trên cluster kind có sẵn, namespace riêng theo run
  ID và cleanup đã xác minh.
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
- Secret value không được persist trong state snapshot hoặc trả qua UC-09 view.
- Web Console và acceptance application frontend là hai artifact độc lập.

## Known limitations and release gate

- System-of-record hiện là in-memory map cộng JSON snapshot; PostgreSQL adapter
  chưa được hiện thực.
- Terraform state chưa có durable backend; physical cloud names còn chứa run ID.
- Runtime chỉ execute ba embedded Terraform modules; remote source execution
  chưa thuộc baseline.
- UC-01..UC-05 và UC-07 chưa có đầy đủ product management flow/UI.
- Rollback, failure recovery, RBAC, audit và secret lifecycle nằm ngoài MVP.
- AWS happy path gần nhất là run `aws-20260921052038`, trước một số thay đổi
  planning cuối. Phải chạy lại `aws-verify.sh` trước release.

Các vấn đề deferred được theo dõi tại [backlog](backlog/README.md); khác biệt
thiết kế–implementation nằm tại
[known deviations](implementation/deviations.md).
