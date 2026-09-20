# Lời giải Go — Humanitec Planner Challenge v4

Lời giải cho [`../PROBLEM.md`](../PROBLEM.md).
Chương trình nhận một `case.yaml`, chạy toàn bộ pipeline lập kế hoạch và ghi đúng một JSON object ra `stdout`.

## Build và chạy

```bash
go build -o planner .
./planner --case ../testcases/01-noop/case.yaml
```

## Chấm bằng grader của đề

```bash
go build -o /tmp/planner .
cd ../grader
go test ./... -args \
  -planner /tmp/planner \
  -cases "$PWD/../testcases"
```

## Test nội bộ

```bash
go test ./planner/
```

Bộ test tự tìm thư mục `testcases` bằng cách đi ngược lên từ thư mục package; đặt biến môi trường
`CHALLENGE_TESTCASES` để chỉ định đường dẫn khác.

- `TestAgainstExpected` so output của mọi testcase công khai với `expected/result.json`, cả case được
  chấp nhận lẫn case bị từ chối.
- `TestDeltaInvariant` chứng minh `currentDeploymentSet + delta == deploymentSet` bằng cách replay
  Deployment Delta lên Deployment Set hiện tại.
- `TestPointerEscaping`, `TestArrayDiff`, `TestEscapedPlaceholder` phủ các quy tắc dễ sai của JSON
  Pointer, diff array và placeholder escaped.

## Cấu trúc package `planner`

| File | Trách nhiệm |
|---|---|
| `phases.go` | Pipeline theo đúng thứ tự phase của đề, load case, before-check, Candidate Set, Delta, phân loại Active Resource |
| `score.go` | Chuyển Score workload thành fragment `modules`/`shared`, rewrite placeholder, validate theo Resource Type schema |
| `jsonpatch.go` | Diff RFC 6902 deterministic và bộ áp patch |
| `graph.go` | Dựng Resource Graph ban đầu từ Candidate Set, Kahn batching, phát hiện cycle |
| `match.go` | Matching Criteria và trọng số của Humanitec |
| `descriptor.go` | Parse Resource descriptor, Resource Reference và placeholder `${context.*}` |
| `expand.go` | Mở rộng reference và co-provisioned resource tới fixed point |
| `terraform.go` | Đọc HCL bằng `hashicorp/hcl/v2`, dựng contract input/output và fingerprint SHA-256 |
| `render.go` | Sinh `challengePlan` với toàn bộ thứ tự sort theo đề |
| `util.go` | Load YAML, deep equal/copy, JSON Pointer, quét placeholder |

Ranh giới phase được giữ nguyên để logic diff, graph, matching và Terraform inspection dùng lại được
trong hệ thống thật.
