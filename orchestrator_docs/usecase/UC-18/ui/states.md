---
id: UC-18-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-27
related: UC-18
---

# UC-18 UI states

| State | Trigger | UI |
|---|---|---|
| Idle | Application `ACTIVE` | Danger button enabled |
| Busy | Any Environment operation active | Button disabled with reason |
| Loading preview | Dialog opened | Spinner; confirm disabled |
| Blocked | Preview has blockers | Reasons + links; no name field |
| Ready to confirm | Preview loaded, no blockers | Scope, kept items, name field |
| Name mismatch | Typed name differs | Confirm disabled |
| Stale | 409 on confirm | Close dialog; ask to review the new preview |
| Deleting | Application `DELETING`, stages running | Banner, read-only pages, progress per Environment |
| Failed | A stage `FAILED` | Stage, remaining IDs, Thử lại |
| Interrupted | Operation `INTERRUPTED` | Existing recover action, then Thử lại |
| Deleted | Application `DELETED` | Applications list with notice; deep links show not found |

Polling reads persisted state only. A poll error keeps the last state and never
resends the deletion request.
