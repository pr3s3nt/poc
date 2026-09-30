---
id: I06-10-WORK
artifact: iteration-work-items
status: current
last_reviewed: 2026-09-30
related: I06-10
---

# I06-10 work items

## Implementation order

1. Reconcile remaining UC-02..04 traceability against current code and tests.
2. Complete application services and repository behavior for remaining
   Resource Type, Resource Definition and Connection gaps.
3. Add HTTP contracts with validation and stable identifiers.
4. Add Web Console management flows and complete UI states.
5. Add planner integration proving newly registered catalog/connections work.
6. Run backend/frontend validations and update implementation docs/evidence.

## Handoff checklist

Safe slice completed and independently verified on 2026-09-30:
[evidence and UI recording](../../../verification/2026-09-30-registration-hardening-local.md).
The iteration remains current, not complete. Tests are focused on meaningful
regressions; redundant race repetitions were removed per user direction.
An additional pre-existing gap remains: UC-16 parse/draft Save do not validate
resource params against registered inputs; Preview planning does.

Current safe slice: insert-only duplicate guards; scoped strict management HTTP
and fail-closed error projection; no-restart catalog/Connection usability and
PostgreSQL reopen tests; existing READY-only Connection list/context display;
human-paced UI-only registration recording. No new ID format/reserved-key policy
or driver-input shape restrictions before a decision. AWS credential-dependent
implementation remains paused pending the user's choice, not implicitly approved
by a default option. This slice alone cannot close IMP-002/I06-10.

| Audited gap | Current action |
|---|---|
| List-then-upsert permits concurrent replacement | Insert-only registration ports; test winner unchanged on memory/PostgreSQL. |
| Unsafe store/inspector/verifier messages | Scoped safe errors and strict one-object body tests. |
| No-restart/reopen usability evidence | Register through API, then validate Score/match a supported Definition; isolated PostgreSQL reopen/isolation/concurrency checks. |
| Missing reference/criteria negative cases | Reject wrong kind, non-READY/foreign/missing Connection, empty criteria and incomplete references before persistence. |
| Connection list/context and registration reload UI | Existing READY-only/context contract; preserve POST outcome separately from reload error with tests and recording. |
| New ID/reserved-key and Driver Inputs policy | Unresolved; no arbitrary restrictions introduced in the safe slice. |
| Brand-new type has no supported executor pair | Registration validates Score contract only; adding runtime drivers is outside this slice. |
| AWS registration/credential/runtime binding | Await explicit storage choice; no live AWS verification or fake completion claim. |

- [ ] No requirement is inferred only from current seed shape.
- [ ] Resource Definition registration enforces at least one criterion.
- [ ] Connection persists only opaque secret references.
- [ ] Newly created data is usable without process restart.
- [ ] I00-00 UC-01 behavior remains covered by regression tests.
- [ ] IMP-002 removed only when all remaining UC-02..04 gaps are delivered.
