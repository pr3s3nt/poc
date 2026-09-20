# Tài liệu chuẩn dùng cho đề

Đề là một subset có quy tắc deterministic để chấm offline. Khi có khác biệt giữa tài liệu ngoài và `PROBLEM.md`, quy tắc trong đề quyết định expected output; các cấu trúc Humanitec không thuộc test harness vẫn giữ tên và ý nghĩa từ các nguồn dưới đây.

## Score

- [Score overview trong Humanitec](https://developer.humanitec.com/platform-orchestrator/docs/score/overview/)
- [Score specification reference](https://docs.score.dev/docs/score-specification/score-spec-reference/)
- [Humanitec CLI Score integration](https://developer.humanitec.com/platform-orchestrator/docs/platform-orchestrator/reference/cli-references/#score-integration)
- [Archived `score-humanitec` source](https://github.com/score-spec/score-humanitec)

Các điểm dùng trong đề: một Score file mô tả một workload; `resources` có `type`, optional `class`, `id`, `metadata`, `params`; cùng type/class/id đại diện cùng resource giữa các workload; placeholder được dùng trong container variables và resource params.

## Deployment Set và Delta

- [Deployment Sets and Deltas](https://developer.humanitec.com/platform-orchestrator/docs/platform-orchestrator/reference/deployment-sets-and-deltas/)
- [RFC 6902 — JSON Patch](https://www.rfc-editor.org/rfc/rfc6902)
- [RFC 6901 — JSON Pointer](https://www.rfc-editor.org/rfc/rfc6901)

Humanitec dùng tên lịch sử `modules` cho workloads và `externals` cho private resource dependencies. Mã nguồn `score-humanitec` xác nhận `shared` trong Delta là danh sách JSON Patch operation.

## Resource Definition và matching

- [Resource Definitions](https://developer.humanitec.com/platform-orchestrator/docs/platform-orchestrator/resources/resource-definitions/)
- [Placeholders](https://developer.humanitec.com/platform-orchestrator/docs/platform-orchestrator/reference/placeholders/)
- [Resource Types](https://developer.humanitec.com/platform-orchestrator/docs/platform-orchestrator/reference/resource-types/)

Entity YAML fixtures dùng `apiVersion: entity.humanitec.io/v1b1`, `kind: Definition`, `metadata.id` và các field trong `entity`. Humanitec docs cũng minh họa API envelope `core.api.humanitec.io/v1` với top-level `criteria`; envelope đó không được trộn vào testcase v4.

Matching specificity theo Humanitec: environment type 1, application ID 2, environment ID 4, resource ID 8, resource class 16.

## Resource Graph

- [Resource Graph](https://developer.humanitec.com/platform-orchestrator/docs/platform-orchestrator/resources/resource-graph/)
- [Resource Graph patterns](https://developer.humanitec.com/platform-orchestrator/docs/platform-orchestrator/examples/resource-graph-patterns/)

Các điểm dùng trong đề: consumer phụ thuộc provider; graph được topo-sort để provider provision trước; Resource Reference có descriptor type/class/id; class hoặc ID thiếu/`@` kế thừa context hiện tại; `entity.provision` hỗ trợ `is_dependent`, `match_dependents`, `params`.

## Terraform Driver

- [Humanitec Terraform Driver](https://developer.humanitec.com/platform-orchestrator/docs/integration-and-extensions/drivers/generic-drivers/terraform/)
- [Terraform input variables](https://developer.hashicorp.com/terraform/language/values/variables)
- [Terraform output values](https://developer.hashicorp.com/terraform/language/values/outputs)
- [HashiCorp HCL](https://github.com/hashicorp/hcl)

Resource inputs từ Score/co-provision params trở thành Terraform variables cùng tên. `driver_inputs.values.variables` là input bổ sung. Humanitec resolve placeholders trước khi gọi Driver. Module Terraform cần output phù hợp Resource Type output schema.

## Active Resources

- [Active Resources](https://developer.humanitec.com/platform-orchestrator/docs/platform-orchestrator/resources/active-resources/)

Humanitec thử provision mọi graph resource ở mỗi deployment. Resource không còn được graph tham chiếu vẫn tồn tại dưới dạng unreferenced; chỉ xóa dependency không đồng nghĩa Terraform destroy.

## Thuật toán

- [Topological sorting](https://en.wikipedia.org/wiki/Topological_sorting)
- [SHA-256, FIPS 180-4](https://csrc.nist.gov/pubs/fips/180-4/upd1/final)

