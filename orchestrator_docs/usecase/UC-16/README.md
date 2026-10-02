---
id: UC-16-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-02
---

# UC-16 context — Manage workload configuration

## Delivery state

Specification and UI design approved. UC-01 Application home now links to
UC-16 draft list/editor and UC-12 settings. Form, Score import and draft
save/delete/undo are implemented with reference validation. Save and Score
import also validate all non-virtual dependency params against the Resource Type
catalog before mutation (local code/API/UI checks on 2026-10-02). Editing a deployed
workload without a saved draft reconstructs a reference-based Score from the
current Deployment Set. Preview → Deploy and runtime reference resolution are
implemented; create and configuration-only redeploy have kind verification.
Resource dependency inputs, including PostgreSQL `database` and `username`,
are editable on the form and persisted as Score params.
Per-container Application variable/secret checklists select existing UC-12 keys
with same-name defaults and optional aliases. Resource/Service references use
a separate editor. Missing keys, duplicate container names and late replies
from a previous scope cannot silently replace or drop selected bindings.
One optional Service port can be selected as the Environment's public entry;
it remains pending until Preview → Deploy.

## Read in this order

1. [Specification](specification.md)
2. [UI design](ui/README.md)
3. [UC-01 Application home](../UC-01/ui/README.md)
4. [UC-05 preview](../UC-05/README.md)
5. [Realization](realization.md), [sequence](sequence.puml) and [VOPC](vopc.puml)
6. [UC-12 Application variables and secrets](../UC-12/README.md)
7. [UC-06 deployment](../UC-06/README.md) and [UC-07 update/removal](../UC-07/README.md)

UC-12 owns Application variables/secrets; saving a draft never changes the
running workload until an explicit Preview → Deploy.
