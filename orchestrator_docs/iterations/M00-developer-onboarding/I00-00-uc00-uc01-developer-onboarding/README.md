---
id: I00-00
artifact: iteration-plan
status: current
last_reviewed: 2026-09-23
related: UC-00, UC-01, IMP-002, IMP-012
---

# I00-00 — UC-00 and UC-01 developer onboarding

## Objective

Thiết kế và hiện thực một onboarding flow chạy được từ sign-in tới tạo
Application. UX của hai use case được thiết kế cùng nhau vì chúng là một hành
trình liên tục, nhưng mỗi screen/state vẫn trace về UC-00 hoặc UC-01.

## Canonical inputs

- [UC-00 context](../../../usecase/UC-00/README.md) và [UC-01 context](../../../usecase/UC-01/README.md).
- [Operation contracts](../../../architecture/contracts/operation-contracts.md),
  [domain model](../../../architecture/domain/domain-objects.md) và
  [database schema](../../../architecture/database/schema.md).
- [ADR-004 Web Console](../../../architecture/decisions/ADR-004-react-web-console.md).
- [UC-00 design-gate follow-up](../../../traceability/design-gate.md) và
  [known deviations](../../../implementation/deviations.md).

## In scope

- Thiết kế sign-in, Application list/empty, create Application và success/error
  states; thêm UI navigation/shell tối thiểu cho hành trình này.
- Backend UC-00: fixed test accounts, password verification, opaque session,
  authentication middleware và sign-out.
- Frontend UC-00: sign-in, authenticated shell, session-expired/error states
  và sign-out.
- Backend UC-01: generated Application ID, Name/Subdomain validation, default
  target resolution và atomic `staging`/`production` creation.
- Frontend UC-01: Applications list, empty state, create form, validation,
  success state và environment endpoint display.
- Backend/frontend/e2e tests, traceability, implementation docs và dated
  verification evidence.

## Out of scope

- Admin account-management UI/API; test accounts remain fixed seed data.
- Workload editing, Score persistence/preview and deploy-flow redesign.
- Actual DNS, ingress/route, certificate or infrastructure provisioning during
  Application creation.

## Exit criteria

- A fixed test user can sign in and cannot access protected routes after
  sign-out or session expiry.
- Application API does not trust organization/role values submitted by UI.
- Developer can create an Application with only Name/Subdomain and see exactly
  `staging` and `production` without a deploy side effect.
- All UC-00/UC-01 main-flow steps have UI state, operation, code and tests.
- Go test/build, frontend typecheck/lint/test/build, docs checker and relevant
  local integration validation pass.

## First next action

Create the paired UC-00/UC-01 screen-flow and API-state map before creating
backend or frontend implementation tasks.

## Outcome

Chưa thực hiện.
