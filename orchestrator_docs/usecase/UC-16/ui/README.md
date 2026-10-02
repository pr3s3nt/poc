---
id: UC-16-UI
artifact: use-case-ui-design
status: current
last_reviewed: 2026-10-02
related: UC-16, UC-01, UC-05, UC-12
---

# UC-16 UI — Workload configuration

## UX outcome

From the UC-01 Application home, Developer selects `staging` or `production`,
then adds, edits or marks a workload for deletion. A form is the default; Score
import is optional. Developer ticks existing Application variables/secrets
per container; container names default to key names, with optional overrides.
Resource outputs and Services use a separate source editor. No values are
re-entered or copied. Saving creates pending configuration; Preview changes and
Deploy are separate steps.
One optional declared Service port can be chosen as the public entry; the
Environment host is configured only on Deploy, not on Save.

The Application-level [Variables & Secrets page](../../UC-12/ui/README.md)
belongs to UC-12, not UC-16. It has Staging/Production tabs; each tab shows
Environment variables and Secrets on the same page. A key may exist in only
one Environment. UC-16 links there when a required key is missing.

## Read in this order

1. [Screens and layout](screens.md)
2. [States and validation](states.md)
3. [UC-16 specification](../specification.md)
4. [Shared shell](../../../architecture/ui/README.md)

The interactive HTML preview used during review is not product code. This
package is the durable UI design; API mapping follows contract design.
