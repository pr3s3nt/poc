---
id: UC-16-REALIZATION
artifact: use-case-realization
status: draft
last_reviewed: 2026-09-27
---

# UC-16 — Use Case Realization (design pending)

This collaboration traces the approved [specification](specification.md).
Application keys use the virtual Score `environment` resource and
`${resources.env.KEY}` syntax. HTTP and state contracts are defined in
[OC-12/OC-16](../../architecture/contracts/operation-contracts.md).

## Responsibilities

- Workload configuration boundary: present and parse form/Score input; never
  offer a literal variable/secret value field.
- Reference catalog: list UC-12 keys for the selected Application/Environment,
  declared resource outputs by classification, and same-Environment workload
  Services and ports.
- Workload configuration control: validate the selected references, save a
  desired change or mark a workload pending deletion; do not call deployment.
- Desired configuration store: retain versioned pending changes separately
  from the current Deployment Set. Preview pins its draft version and UC-12
  revision so a later edit makes the preview stale.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01–MS-02 | Read selected Environment's workload list and open the form or Score importer. |
| MS-03 | Collect workload fields and declared resource dependencies. |
| MS-04–MS-05 | Read eligible UC-12, resource-output and Service references; keep values of secrets hidden. |
| MS-06 | Validate each reference against scope, output classification and target Service/port; return field-level errors on failure. |
| MS-07 | Save desired configuration, return pending status and a UC-05 Preview affordance. |
| VAR-01 | Parse one imported Score, run the same validation and show the resulting form before save. |
| VAR-02 | Confirm deletion, mark desired change pending and permit Undo before deploy. |

## Boundary

UC-16 has no runtime side effects. UC-05 reads pending desired configuration to
produce preview; UC-06/UC-07 apply an approved change later against the pinned
draft version and UC-12 revision.
