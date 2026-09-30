---
id: I06-08-WORK
artifact: iteration-work-items
status: historical
last_reviewed: 2026-09-30
related: I06-08
---

# I06-08 work items

## Implementation order

1. Add PreviewService contract/no-mutation tests.
2. Implement planning snapshot load and shared PlanningService call.
3. Add HTTP DTO/handler validation and response mapping.
4. Add Web Console Preview flow and complete UI states.
5. Add backend/frontend integration tests and update implementation docs.
6. Record a headed, human-paced UI-only Preview demonstration with visible URL,
   clicks and typing; independently review before committing/pushing milestone.

## Handoff checklist

- [x] Preview calls no resource executor or Kubernetes deployer.
- [x] Preview does not persist Deployment or change current set.
- [x] Preview/deploy planning outputs match for one versioned snapshot.
- [x] IMP-003 removed only after API/UI and tests pass.
- [x] UI-only review recording published after independent review.
