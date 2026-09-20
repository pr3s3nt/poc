# Grader

The grader executes one planner process per case with a ten-second timeout and compares its single stdout JSON value structurally with `expected/result.json`.

```bash
go test ./... -args -planner /abs/path/planner -cases /abs/path/testcases
```

