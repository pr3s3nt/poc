# Humanitec Planner Challenge v4

Bộ bài gồm đề chi tiết, starter Go, grader Go và 33 testcase công khai. Bắt đầu tại [PROBLEM.md](PROBLEM.md), xem nguồn chuẩn tại [SOURCES.md](SOURCES.md), và ma trận coverage tại [TESTCASES.md](TESTCASES.md).

## Cấu trúc

```text
humanitec-planner-challenge-v4/
├── PROBLEM.md
├── SOURCES.md
├── TESTCASES.md
├── README.md
├── testcases.yaml
├── starter-go/
├── grader/
└── testcases/
    └── 05-add-private/
        ├── case.yaml
        ├── context.yaml
        ├── current-deployment-set.yaml
        ├── before.score.yaml
        ├── after.score.yaml
        ├── resource-types/
        ├── resource-definitions/
        ├── terraform/checkouts/
        ├── active-resources.yaml
        └── expected/
```

## Chạy grader

```bash
cd grader
go test ./... -args \
  -planner /absolute/path/to/planner \
  -cases /absolute/path/to/humanitec-planner-challenge-v4/testcases
```

Chạy một case:

```bash
go test ./... -args \
  -planner /absolute/path/to/planner \
  -cases /absolute/path/to/humanitec-planner-challenge-v4/testcases \
  -case 15-definition-reference
```

Expected result đầy đủ nằm ở `expected/result.json`; ba file YAML thành phần giúp đọc và review dễ hơn.

## Lưu ý áp dụng thực tế

`case.yaml`, `terraformSourceMap`, projection Active Resources và `challengePlan` là harness của bài. Score, Deployment Set/Delta, Resource Definition, resource descriptor, reference, co-provisioning và matching giữ cấu trúc/ý nghĩa Humanitec trong subset đã nêu.

