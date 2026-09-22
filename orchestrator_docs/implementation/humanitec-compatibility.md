---
id: HUMANITEC-COMPATIBILITY
artifact: compatibility-matrix
status: current
last_reviewed: 2026-09-22
---

# Humanitec and Score compatibility matrix

Tài liệu này phân biệt bốn loại quan hệ với contract bên ngoài:

- **Aligned:** thiết kế hiện tại cùng semantics cần thiết.
- **Implementation gap:** canonical design đã yêu cầu nhưng code chưa đạt.
- **Product extension:** behavior nội bộ có chủ ý, không được quảng bá như
  Humanitec/Score contract.
- **Deferred compatibility:** chưa là requirement của happy path hiện tại; phải
  có quyết định/migration riêng trước khi mở public compatibility boundary.

33 fixture của planner chỉ chứng minh các trường được harness so sánh. Kết quả
đó không tự chứng minh Delta lifecycle, public API, structured error, container
resources hoặc các dòng khác trong bảng này.

| Area | Humanitec/Score contract | Product baseline | Classification | Tracking |
|---|---|---|---|---|
| Delta document shape | `modules.add/remove/update` và `shared`; module patch relative với module, shared patch relative với shared object. | Planner sinh cùng shape, relative patches, array diff theo index/`/-` và no-op `{}`; 27 accepted fixtures so byte-for-byte với `expected/delta.yaml`. | Aligned | I06-06 |
| Delta lifecycle | Humanitec Delta có identity riêng, có thể cập nhật và chỉ không còn cập nhật được sau khi archive. | MVP tạo một snapshot bất biến cho từng Deployment; không gọi snapshot này là Humanitec Delta entity. | Deferred compatibility | D05 |
| Deploy API/lifecycle | Deploy có thể tham chiếu Delta/Set và theo dõi lifecycle bất đồng bộ; full/incremental là capability riêng. | Happy-path API nhận Score, chạy đồng bộ, provision theo full-style resource batches và chỉ apply workload mục tiêu. | Deferred compatibility | D05 |
| Deployment Set identity | Humanitec-oriented compatibility cần content-addressed identity. | MVP lưu UUID và `document_hash` riêng. | Deferred compatibility | D05 |
| Resource Definition driver identity | Humanitec public contract dùng driver ID có namespace, ví dụ `humanitec/terraform`. | Domain/executor registry dùng enum nội bộ như `terraform`, `kubernetes`, `existing-cluster`. | Product extension; boundary mapping deferred | D06 |
| Driver account | Humanitec Resource Definition có `driver_account`. | MVP liên kết Definition với `ConnectionKey`/Connection nội bộ. | Deferred compatibility | D06 |
| Driver secrets | Humanitec Resource Definition hỗ trợ `driver_inputs.secret_refs`. | MVP có Connection `secret_ref`, Secret Store và output redaction tối thiểu, nhưng chưa có contract/lifecycle tương thích `secret_refs`. | Deferred compatibility | D06 |
| Terraform source | Humanitec-compatible remote source dùng `url`/`rev`/`path`. | Runtime MVP dùng `source.module` trỏ tới module nhúng; inspector mới hiểu remote identity. | Product extension + runtime gap | IMP-007, D03, D06 |
| Context placeholders | Các key aligned là `org.id`, `app.id`, `env.id`, `env.type`, `res.id`, `res.class`, `res.type`. | MVP còn mở rộng `org.key`, `app.key/name/profile/region`, `env.key/name/namespace`, `connection.key/kind/cluster/context`, `deployment.id`, `run.id`. | Aligned core + product extension | D06 |
| Descriptor tokens | Humanitec reference contract không yêu cầu token `@app`, `@env`, `@connection`. | MVP dùng các token này để tạo identity ổn định cho implicit resources. | Product extension | UC-06 BR-12, D06 |
| Implicit infrastructure and scopes | Planner reference v4 không tự thêm VPC/EKS/namespace; điều này không đại diện toàn bộ graph nội bộ của Humanitec. | Execution Profile tự enrich VPC → EKS → namespace; VPC/EKS có Application scope, namespace có Environment scope và descriptor dùng `applications.*`/`environments.*`/`connections.*`. | Product extension | ADR-001, UC-06 BR-02/03 |
| Container resources | Score dùng `containers.*.resources.requests/limits` cho `cpu` và `memory`. | Canonical UC-05/06 giữ typed values; parser/renderer hiện chưa làm được. | Implementation gap | IMP-009 |
| Probes | Score dùng `livenessProbe`/`readinessProbe` với nested `httpGet`. | MVP model hiện dùng probe phẳng `{path, port}`. | Deferred compatibility | D06 |
| Replicas | Score workload contract không có top-level `replicas`. | MVP chấp nhận top-level `replicas` như extension. | Product extension | D06 |
| Namespace output | Humanitec namespace resource output dùng key `namespace`. | Seed/runtime MVP hiện công bố key `name`. | Deferred compatibility | D06 |
| Empty matching criteria | Definition thiếu criteria hoặc có `criteria: []` không được xét; một criterion `{}` là wildcard điểm 0. | Conformance harness đã aligned: adapter bỏ Definition thiếu/`[]` khỏi challenge catalog và giữ `{}` thành wildcard điểm 0. Product validation stricter: registration từ chối Definition vi phạm invariant và planner fail cả catalog thay vì bỏ qua Definition đó. | Deferred compatibility (harness aligned) | UC-03 BR-07, D07, [I06-05 evidence](../verification/2026-09-22-imp010-conformance-catalog.md) |

## Boundary rule

Internal enum, placeholder hoặc storage model có thể giữ nguyên nếu một adapter
ngoài biên ánh xạ lossless sang contract đã chọn. Không công bố endpoint hoặc
schema là Humanitec-compatible trước khi các dòng Deferred tương ứng được duyệt,
test contract và có migration cho dữ liệu/client hiện tại.

## References

- [Humanitec Delta API](https://api-docs.humanitec.com/#tag/Delta) — create,
  update and archive operations.
- [Humanitec Namespace resource](https://developer.humanitec.com/platform-orchestrator/docs/integration-and-extensions/containerization/namespaces/) — canonical `namespace` output.
- [Score specification reference](https://docs.score.dev/docs/score-specification/score-spec-reference/) — container resources and probe shapes.
- [`PROBLEM.md`](../../orchestrator_reference/humanitec-planner-challenge-v4/PROBLEM.md) — planner challenge subset, matching and criteria behavior.
