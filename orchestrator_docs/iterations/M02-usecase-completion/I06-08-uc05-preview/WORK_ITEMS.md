---
id: I06-08-WORK
artifact: iteration-work-items
status: deferred
last_reviewed: 2026-09-22
related: I06-08
---

# I06-08 work items

## Implementation order

1. Add PreviewService contract/no-mutation tests.
2. Implement planning snapshot load and shared PlanningService call.
3. Add HTTP DTO/handler validation and response mapping.
4. Add Web Console Preview flow and complete UI states.
5. Add backend/frontend integration tests and update implementation docs.

## Handoff checklist

- [ ] Preview calls no resource executor or Kubernetes deployer.
- [ ] Preview does not persist Deployment or change current set.
- [ ] Preview/deploy planning outputs match for one versioned snapshot.
- [ ] IMP-003 removed only after API/UI and tests pass.
