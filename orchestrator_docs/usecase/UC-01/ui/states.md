---
id: UC-01-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-07
---

# UC-01 UI states

| Screen | State | Behavior |
|---|---|---|
| Apps home | Loading/empty/error | Skeleton, create CTA, retry; no substitute mocks. |
| Create | Validation/submitting/error/success | Name/Subdomain field errors, prevent duplicate submit, retain input, navigate staging. No connection choice request. |
| Settings target | UNCONFIGURED | Explain set once; load safe choices, explicit selection required. |
| Settings target | Choices loading/error/empty | Disable Set; retry; keep variables/secrets usable; ask PE to register READY choice when empty. |
| Settings target | Saving | Disable duplicate submit and selector; scope changes cancel/ignore obsolete replies. |
| Settings target | Validation/unavailable error | Retain fields; safe inline error; refresh choices and clear missing selected key, no silent default. |
| Settings target | Conflict | Reload authoritative Environment; show saved winner as locked or stale-version retry. |
| Settings target | Configured | Safe key/profile/region read-only; binding persists refresh/restart. |
| Home/Preview | UNCONFIGURED | Settings link and block Preview/Deploy; editing drafts remains enabled. |
| Environment tabs | Switch | Target, settings, preview token and responses follow current scope; never leak previous tab's selection/binding. |
