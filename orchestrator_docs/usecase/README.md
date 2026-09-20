# Orchestrator Use Case Model

## 1. Mục tiêu

Xây dựng orchestrator nội bộ sử dụng các contract và khái niệm gần với Humanitec để giảm chi phí chuyển đổi sang Humanitec trong tương lai.

Ưu tiên cố định các contract bên ngoài như Score, Deployment Set, Deployment Delta, Resource Definition, Resource Descriptor và Active Resource. Phần implementation nội bộ có thể phát triển độc lập.

## 2. Actors

- **Developer:** khai báo và triển khai workload.
- **Platform Engineer:** cấu hình environment, resource và hạ tầng.
- **CI/CD System:** tự động kích hoạt deployment.
- **Organization Administrator:** quản lý người dùng và quyền truy cập.
- **Resource Driver:** provision resource như Terraform hoặc Echo.
- **Kubernetes Cluster/Operator:** thực thi workload và resource manifests.
- **Secret Store:** cung cấp secrets cho workload và driver.

## 3. Danh sách use case

| ID | Use case | Actor chính | Ưu tiên |
|---|---|---|---|
| [UC-01](UC-01/specification.md) | Quản lý Application và Environment | Platform Engineer | P0 |
| [UC-02](UC-02/specification.md) | Đăng ký Resource Type | Platform Engineer | P0 |
| [UC-03](UC-03/specification.md) | Đăng ký Resource Definition và Matching Criteria | Platform Engineer | P0 |
| [UC-04](UC-04/specification.md) | Cấu hình Execution Profile, cluster và Driver Account | Platform Engineer | P0 |
| [UC-05](UC-05/specification.md) | Validate và preview thay đổi từ Score | Developer, CI/CD | P0 |
| [UC-06](UC-06/specification.md) | Deploy workload | Developer, CI/CD | P0 |
| [UC-07](UC-07/specification.md) | Cập nhật hoặc xóa workload | Developer, CI/CD | P0 |
| [UC-08](UC-08/specification.md) | Provision private/shared resources | Orchestrator | P0 |
| [UC-09](UC-09/specification.md) | Xem deployment status, graph và resource outputs | Developer | P0 |
| UC-10 | Redeploy hoặc rollback deployment cũ | Developer | P1 |
| UC-11 | Quản lý Active Resource lifecycle | Platform Engineer | P1 |
| UC-12 | Quản lý shared values và secrets | Developer, Platform Engineer | P1 |
| UC-13 | Trigger deployment qua API, CLI hoặc pipeline | CI/CD | P1 |
| UC-14 | Quản lý user, token và RBAC | Administrator | P2 |
| UC-15 | Audit deployment và configuration changes | Administrator | P2 |

## 4. Architecturally significant use cases

Các use case ảnh hưởng trực tiếp đến kiến trúc và nên được ưu tiên trong Unified Process:

- **UC-06 — Deploy workload:** kiểm chứng toàn bộ luồng plan và execute, bao gồm implicit infrastructure theo Execution Profile.
- **UC-07 — Update workload:** kiểm chứng khả năng giữ và tái sử dụng resource state.
- **UC-08 — Provision resources:** kiểm chứng application-scoped infrastructure, workload resources, dependency và output propagation.
- **UC-09 — Observe deployment:** kiểm chứng mô hình trạng thái deployment và resource.
- **UC-10 — Rollback:** kiểm chứng deployment history và khả năng redeploy trạng thái cũ.
- **UC-11 — Active Resource lifecycle:** kiểm chứng unreference, deletion và deprovision.

## 5. Phạm vi theo Unified Process

### Inception

- Chốt actors, UC-01 đến UC-09 và phạm vi happy path.
- Chỉ hỗ trợ Terraform và Kubernetes.
- Chốt các contract tương thích Humanitec.

### Elaboration

- Xây executable architecture cho UC-06.
- Chạy Cloud happy path: tự thêm VPC/EKS, provision Aurora và triển khai workload lên EKS.
- Chạy Internal happy path: dùng cluster có sẵn, provision PostgreSQL StatefulSet và triển khai workload.
- Kiểm chứng planner, executor, output propagation và Kubernetes deployer.

### Construction

1. Update workload và tái sử dụng resource.
2. Shared resources.
3. Deployment history và rollback.
4. Secrets.
5. Resource deletion và failure handling.
6. API, pipeline và RBAC.

### Transition

- Chạy thử với ứng dụng nội bộ.
- Kiểm thử tương thích bằng Humanitec-style fixtures.
- Chuẩn hóa quy trình migration sang Humanitec.

## 6. Cấu trúc tài liệu

Mỗi use case thuộc phạm vi MVP có một thư mục `UC-xx`. File `specification.md` là nguồn yêu cầu chuẩn; các artifact realization, sequence và VOPC sẽ được bổ sung trong cùng thư mục ở giai đoạn thiết kế.

## 7. Thuật ngữ chuẩn

| Thuật ngữ | Nghĩa trong MVP |
|---|---|
| Organization | Biên sở hữu Application, Resource Type, Resource Definition và connection. |
| Application | Đơn vị ứng dụng sở hữu một Execution Profile cố định; với `aws-eks`, đây cũng là scope của VPC/EKS. |
| Environment | Môi trường triển khai thuộc đúng một Application, có Deployment Set hiện tại và namespace identity riêng. |
| Execution Profile | Chính sách thực thi `aws-eks` hoặc `internal-k8s`, quyết định connection và tập Resource Definition phù hợp. |
| Deployment | Một lần plan và execute thay đổi trên một Environment. |
| Deployment Set | Desired-state snapshot đầy đủ của các workload và shared resource trong Environment. |
| Deployment Delta | Thay đổi từ Deployment Set hiện tại sang Candidate Deployment Set. |
| Resource Type | Contract inputs/outputs độc lập với implementation của một loại resource. |
| Resource Definition | Cách hiện thực một Resource Type, gồm Driver, inputs, Matching Criteria và provision rules. |
| Resource Descriptor | Identity dạng `type.class#res_id`, dùng làm node identity và khóa đối chiếu resource. |
| Resource Graph | DAG có edge `consumer -> provider`; provider phải được provision trước consumer. |
| Active Resource | Resource instance đã được orchestrator ghi nhận cùng descriptor, Definition, scope, state và outputs. |
| Workload | Module được chuyển từ một Score document và triển khai lên Kubernetes. |

## 8. Quy ước traceability

- `PRE-nn`: precondition.
- `TRG-nn`: trigger.
- `MS-nn`: bước trong main success scenario.
- `VAR-nn`: biến thể vẫn thuộc happy path.
- `BR-nn`: business rule/invariant.
- `POST-nn`: postcondition.
- `OOS-nn`: extension/exception cố ý nằm ngoài happy path.

ID chỉ ổn định trong phạm vi một use case và sẽ được dùng lại trong realization, sequence, VOPC, operation contract và test.

## 9. Quan hệ giữa các use case

- UC-06 `«include»` UC-08 tại bước provision resource.
- UC-07 `«include»` UC-08 khi update cần provision hoặc reconcile desired resource.
- UC-05 dùng chung planning pipeline với UC-06 nhưng không provision và không thay đổi runtime state.
- UC-09 chỉ đọc kết quả đã persist bởi UC-06, UC-07 và UC-08.

## 10. Design artifacts

| UC | Specification | Realization | Sequence | VOPC |
|---|---|---|---|---|
| UC-01 | [spec](UC-01/specification.md) | [realization](UC-01/realization.md) | [PlantUML](UC-01/sequence.puml) | [PlantUML](UC-01/vopc.puml) |
| UC-02 | [spec](UC-02/specification.md) | [realization](UC-02/realization.md) | [PlantUML](UC-02/sequence.puml) | [PlantUML](UC-02/vopc.puml) |
| UC-03 | [spec](UC-03/specification.md) | [realization](UC-03/realization.md) | [PlantUML](UC-03/sequence.puml) | [PlantUML](UC-03/vopc.puml) |
| UC-04 | [spec](UC-04/specification.md) | [realization](UC-04/realization.md) | [PlantUML](UC-04/sequence.puml) | [PlantUML](UC-04/vopc.puml) |
| UC-05 | [spec](UC-05/specification.md) | [realization](UC-05/realization.md) | [PlantUML](UC-05/sequence.puml) | [PlantUML](UC-05/vopc.puml) |
| UC-06 | [spec](UC-06/specification.md) | [realization](UC-06/realization.md) | [shared](UC-06/sequence.puml), [cloud](UC-06/sequence-cloud.puml), [internal](UC-06/sequence-internal.puml) | [PlantUML](UC-06/vopc.puml) |
| UC-07 | [spec](UC-07/specification.md) | [realization](UC-07/realization.md) | [PlantUML](UC-07/sequence.puml) | [PlantUML](UC-07/vopc.puml) |
| UC-08 | [spec](UC-08/specification.md) | [realization](UC-08/realization.md) | [PlantUML](UC-08/sequence.puml) | [PlantUML](UC-08/vopc.puml) |
| UC-09 | [spec](UC-09/specification.md) | [realization](UC-09/realization.md) | [PlantUML](UC-09/sequence.puml) | [PlantUML](UC-09/vopc.puml) |
