---
id: I06-05-WORK
artifact: iteration-work-items
status: current
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

- [ ] No production planner/domain behavior changed.
- [ ] Missing/`[]` Definition cannot become a wildcard candidate.
- [ ] Explicit `{}` remains a wildcard.
- [ ] 33/33 fixtures pass.
- [ ] IMP-010 removed only after tests pass.
- [ ] `go test ./...`, `go build ./...`, `go vet ./...` pass under `backend/`.
