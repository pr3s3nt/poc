---
id: UC-16-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-09-24
related: UC-16
---

# UC-16 UI states

| Screen | State | UI behavior |
|---|---|---|
| Application home | Loading/empty | Preserve selected Environment; show list skeleton or `No workloads` and Add action. |
| Application home | Pending | Show added/edited/deleted labels and Preview changes; no runtime-success claim. |
| Application home | API error | Show retry without replacing data with mock workload rows. |
| Form | Loading/new/edit | Show Environment, empty form for Add or existing configuration for Edit. |
| Form | Missing/invalid reference | Identify variable/secret row and source; disable Save until corrected. |
| Form | Missing UC-12 key | Explain the missing key in this Environment; link to Application Variables & Secrets. |
| Form | Invalid Service target | Explain missing target, missing Service/port or target pending deletion; retain other form fields. |
| Form/Preview | Public port conflict | Form requires one declared port; Preview rejects a second public workload in the same Environment. |
| Form | Missing/invalid resource input | Identify dependency and required input; retain entered values and prevent invalid save. |
| Form | Secret source | Show name/output classification only; never echo secret value. |
| Import | Parsing/invalid | Show file progress or field-level error; direct literal binding is rejected. |
| Save | Submitting/error/success | Prevent double submit; preserve form on error; return to Application home with pending label on success. |
| Delete | Confirm/cancel/success | Require confirmation; cancel does nothing; success shows pending deletion and Undo. |
