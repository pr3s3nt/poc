---
id: I06-05-WORK
artifact: iteration-work-items
status: historical
last_reviewed: 2026-09-22
related: I06-05
---

# I06-05 work items

## Implementation order

1. Add loader tests for missing `criteria`, `criteria: []` and `criteria: [{}]`.
2. Change `backend/test/conformance/loader.go` to skip only the first two forms.
3. Assert explicit `{}` remains a product `Criterion{}` with score `0`.
4. Run loader tests, planner tests and all 33 conformance fixtures.
5. Update current state, compatibility matrix, deviations, traceability and a
   dated verification record; do not edit canonical UC-03 behavior.

## Handoff checklist

- [x] No production planner/domain behavior changed.
- [x] Missing/`[]` Definition cannot become a wildcard candidate.
- [x] Explicit `{}` remains a wildcard.
- [x] 33/33 fixtures pass.
- [x] IMP-010 removed only after tests pass.
- [x] `go test ./...`, `go build ./...`, `go vet ./...` pass under `backend/`.
