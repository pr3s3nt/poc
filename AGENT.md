# Orchestrator Development Workflow

Tài liệu này là quy trình bắt buộc cho mọi phiên làm việc với dự án orchestrator trong workspace này.

## 1. Mục tiêu và phạm vi

- Xây dựng orchestrator nội bộ theo Unified Process (UP), dùng các contract và khái niệm gần Humanitec để giảm chi phí chuyển đổi sau này.
- Phạm vi MVP hiện tại chỉ gồm UC-01 đến UC-09 trong `orchestrator_docs/usecase/`.
- Thiết kế chỉ nhắm happy path cho hai Execution Profile: `aws-eks` và `internal-k8s`.
- Planner, executor, CLI/API service, backend, acceptance application workloads và automated test phía backend của orchestrator phải viết bằng Go.
- Orchestrator Web Console là ngoại lệ đã duyệt: dùng React + TypeScript strict + Vite và frontend test bằng Vitest/Testing Library. Không tạo Python implementation cho bất kỳ phần nào của sản phẩm.
- Không tự ý mở rộng sang rollback, secrets, RBAC, audit hoặc failure recovery khi chưa cập nhật phạm vi.

## 2. Nguồn thông tin và quyền sở hữu

Khi tài liệu mâu thuẫn, áp dụng thứ tự sau:

1. `orchestrator_docs/usecase/UC-xx/specification.md` sở hữu yêu cầu và hành vi của use case.
2. `realization.md`, `sequence.puml` và `vopc.puml` của use case sở hữu cách các đối tượng cộng tác để thực hiện yêu cầu.
3. Tài liệu trong `orchestrator_docs/architecture/` sở hữu mô hình dùng chung, database, operation contract và state machine.
4. `plan.md` sở hữu thứ tự công việc và trạng thái thực hiện, nhưng không được thay đổi yêu cầu.
5. Code và test phải hiện thực thiết kế đã duyệt; chúng không tự động trở thành yêu cầu mới.

Các thư mục sau chỉ dùng để tham khảo và phải giữ nguyên:

- `orchestrator_reference/humanitec-planner-challenge-v4/`: nguồn kỹ thuật cho planner của UC-06.
- `final_idp/`: nguồn tham khảo cách tổ chức UP, artifact và traceability.
- `humanitec-planner-challenge-v4/`: bản nguồn ban đầu; không sửa hoặc dùng làm code sản phẩm.

Không copy nguyên kiến trúc, domain model hoặc code của tài liệu tham khảo vào sản phẩm nếu chưa ánh xạ về yêu cầu của orchestrator.

Riêng `final_idp/idp/frontend` được dùng làm nguồn tham khảo read-only cho cách chia `app/features/shared/styles`, draft/reducer, typed API client, same-origin delivery và UI states. Không copy nguyên domain, authentication flow, API contract hoặc source code sang sản phẩm.

Script Python có sẵn trong thư mục reference chỉ được chạy để kiểm tra nguyên trạng fixture; nó không phải dependency, build step hay code của orchestrator.

## 3. Bắt đầu mỗi phiên làm việc

1. Đọc `AGENT.md` và `plan.md`.
2. Đọc `orchestrator_docs/INDEX.md` và use case index.
3. Xác định phase và hạng mục chưa hoàn thành đầu tiên trong `plan.md`.
4. Đọc toàn bộ specification và các artifact liên quan trước khi chỉnh sửa.
5. Kiểm tra đường dẫn, thay đổi hiện có và không ghi đè công việc ngoài phạm vi.
6. Nêu ngắn gọn sẽ làm artifact nào trước khi bắt đầu.

Kết thúc phiên phải cập nhật trạng thái, quyết định mới, kết quả kiểm tra và bước kế tiếp trong `plan.md`.

## 4. Quy trình UP của dự án

Thực hiện thiết kế cho toàn bộ UC-01 đến UC-09 trước khi code sản phẩm:

1. **Specification:** chuẩn hóa actor, precondition, trigger, main flow, postcondition, business rule và extension point; gán ID ổn định cho từng bước và rule.
2. **Use Case Realization:** xác định boundary, control, entity, repository, gateway và system operation; ghi rõ tên method dự kiến.
3. **Sequence:** vẽ sequence diagram bám từng bước của main flow; UC-06 phải có riêng cloud path và internal path.
4. **VOPC:** vẽ class tham gia cho từng use case, gồm method, dependency và trách nhiệm.
5. **Shared design:** hợp nhất design class, domain/persistence classification và ranh giới component.
6. **Persistence:** thiết kế database schema, khóa, quan hệ, constraint và ERD từ lifecycle thật của entity.
7. **Operation contracts:** ghi precondition, postcondition, object created/updated và association change cho các operation quan trọng.
8. **State machines:** mô tả trạng thái và transition của Deployment, Resource và các aggregate có lifecycle.
9. **Traceability review:** chứng minh chuỗi `UC step -> operation -> sequence -> class/method -> table/contract/state -> test`.
10. **Design gate:** chỉ bắt đầu code khi không còn gap P0 trong chuỗi traceability.

Một thay đổi hành vi phải đi theo thứ tự: specification -> realization/diagram -> shared architecture -> traceability -> code/test. Không sửa code trước rồi hợp thức hóa tài liệu sau.

## 5. Quy tắc riêng cho UC-06

Trước khi realization UC-06:

1. Đọc đầy đủ `PROBLEM.md`, `SOURCES.md`, `TESTCASES.md`, source planner, grader và fixtures trong bản reference.
2. Chạy test/grader để xác nhận baseline.
3. Viết `orchestrator_docs/implementation/uc06-planner-reference.md`, phân loại rõ:
   - khái niệm/thuật toán được tái sử dụng;
   - thành phần chỉ thuộc challenge harness;
   - khoảng trống phải bổ sung cho orchestrator thực.
4. Chỉ dùng kết quả ánh xạ đó làm input cho realization; challenge không thay thế specification.

Các phần cần đánh giá gồm Score conversion, Deployment Delta, resource graph, matching specificity, definition reference, co-provisioning, Terraform contract scanning và topological scheduling. `case.yaml`, `terraformSourceMap`, Active Resource projection và `challengePlan` là harness, không phải domain sản phẩm.

UC-06 phải giữ các quyết định đã duyệt:

- `aws-eks`: mỗi Application có VPC và EKS ở application scope; orchestrator tự bổ sung hai dependency implicit, provision hạ tầng trước rồi deploy workload.
- `internal-k8s`: dùng cluster đã đăng ký; Environment ánh xạ vào namespace.
- PostgreSQL workload dependency được chọn qua Resource Definition: Aurora trên cloud, StatefulSet trong internal Kubernetes.
- Không nhúng nhánh provider-specific rải rác trong orchestration flow; lựa chọn đi qua Execution Profile, Resource Definition và executor adapter.
- UC-08 là use case được UC-06 gọi để provision resource, không phải logic vô danh nằm trong UC-06.

## 6. Quy ước artifact

Mỗi thư mục `orchestrator_docs/usecase/UC-xx/` dùng tên cố định:

- `specification.md`
- `realization.md`
- `sequence.puml`
- `vopc.puml`

Tài liệu dùng chung đặt trong:

- `orchestrator_docs/architecture/domain/`
- `orchestrator_docs/architecture/database/`
- `orchestrator_docs/architecture/contracts/`
- `orchestrator_docs/architecture/state-machines/`
- `orchestrator_docs/traceability/`
- `orchestrator_docs/implementation/`

Tên class, operation, method, table và trạng thái phải nhất quán giữa các artifact. Nội dung giải thích có thể viết tiếng Việt; identifier kỹ thuật dùng tiếng Anh.

Mọi diagram của dự án phải viết bằng PlantUML và lưu source `.puml`. Bao gồm sequence, VOPC, consolidated class, component, ERD và state machine; không dùng Mermaid hay diagram chỉ có ảnh mà thiếu source.

## 7. Kiểm tra chất lượng

Trước khi đánh dấu một hạng mục hoàn thành:

- kiểm tra link và đường dẫn không còn trỏ tới cấu trúc cũ;
- render/validate PlantUML nếu môi trường có công cụ;
- đối chiếu diagram với specification, operation contract và schema;
- chạy test phù hợp nếu hạng mục đã có executable artifact;
- với thay đổi web console, chạy typecheck, lint, Vitest và production build; không commit `node_modules/`, `dist/` hoặc coverage output;
- ghi bằng chứng kiểm tra và bước tiếp theo vào `plan.md`.

Không đánh dấu hoàn thành nếu chỉ tạo file khung, còn TODO quan trọng, hoặc artifact chưa nối được vào traceability.
