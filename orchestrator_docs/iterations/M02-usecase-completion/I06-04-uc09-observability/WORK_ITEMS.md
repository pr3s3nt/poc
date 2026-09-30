---
id: I06-04-WORK
artifact: iteration-work-items
status: current
last_reviewed: 2026-09-30
related: I06-04
---

# I06-04 work items

## Implementation order

1. Re-audit UC-09 `MS-nn` against query service, HTTP handlers and Web Console.
2. Complete deployment history/filter repository/query contracts.
3. Complete API request/response and validation/error states.
4. Complete frontend list/detail loading, empty, success and error behavior.
5. Preserve redaction and add backend/frontend regression tests.
6. Update traceability/current state/code map and dated verification evidence.

## Handoff checklist

- [ ] No secret value appears in API, UI, logs or test snapshots.
- [ ] History/filter semantics match UC-09, not UI convenience behavior.
- [ ] Backend and frontend validation suites pass.
- [ ] IMP-005 removed only after all main-flow steps are implemented.
