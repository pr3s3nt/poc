# UC-05 — Validate and Preview Score Changes

## Mục tiêu

Kiểm tra Score và hiển thị thay đổi dự kiến trước khi thực hiện deployment.

## Primary actors

- Developer
- CI/CD System

## Tiền điều kiện

- **PRE-01:** Application và Environment đã tồn tại.
- **PRE-02:** Environment có Deployment Set hiện tại, kể cả Deployment Set rỗng.
- **PRE-03:** Các Resource Type và Resource Definition cần thiết đã được đăng ký.
- **PRE-04:** Application có Execution Profile và connection trạng thái `READY`.

## Trigger

**TRG-01:** Developer hoặc CI/CD gửi Score và yêu cầu validate/preview trên một Environment.

## Main success scenario

1. **MS-01:** Orchestrator đọc Score và Deployment Set hiện tại.
2. **MS-02:** Orchestrator validate Score theo Score contract và Resource Type schemas.
3. **MS-03:** Orchestrator chuyển Score thành workload fragment.
4. **MS-04:** Orchestrator tạo Deployment Delta và Candidate Deployment Set.
5. **MS-05:** Orchestrator enrich implicit resources theo Execution Profile, dựng Resource Graph và match Resource Definitions.
6. **MS-06:** Orchestrator kiểm tra graph, references và Terraform contracts.
7. **MS-07:** Orchestrator tính provision batches và phân loại Active Resources.
8. **MS-08:** Orchestrator trả preview gồm Delta, Candidate Deployment Set, resource changes và provision order mà không thực thi runtime change.

## Hậu điều kiện

- **POST-01:** Không có resource hoặc workload nào được provision và không đổi Deployment Set hiện tại.
- **POST-02:** Preview phản ánh thay đổi dự kiến trên đúng Environment.
- **POST-03:** Preview chứa đủ thông tin planning để đối chiếu với UC-06.

## Quy tắc nghiệp vụ

- **BR-01:** Preview và deploy phải dùng cùng planning pipeline và cùng input snapshot để tránh kết quả khác nhau.
- **BR-02:** Invariant bắt buộc là `current Deployment Set + Delta = Candidate Deployment Set`.
- **BR-03:** Graph edge có chiều `consumer -> provider`; batches luôn đặt provider trước consumer.
- **BR-04:** Preview là read-only và không được gọi Resource Executor hoặc Kubernetes apply.

## Luồng nội bộ

```text
UC-05 Validate and Preview
├── Validate Score and schemas
├── Build Delta and Candidate Set
├── Build and validate Resource Graph
├── Match Resource Definitions
├── Calculate provision order
└── Return deployment preview
```

## Trạng thái implementation hiện tại

- Planner hiện tại đã triển khai phần lớn luồng validate và preview trong phạm vi challenge.
- `challengePlan` hiện là artifact riêng của bài toán, chưa phải persistent deployment preview hoặc API chính thức.

## Ngoài phạm vi happy path

- **OOS-01:** Lưu và quản lý nhiều bản preview.
- **OOS-02:** Approval workflow.
- **OOS-03:** Chi phí dự kiến hoặc policy evaluation.
- **OOS-04:** So sánh preview với runtime state thực tế.
- **OOS-05:** Preview Terraform plan thật.
