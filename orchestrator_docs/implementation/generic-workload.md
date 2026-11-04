---
id: IMP-GENERIC-WORKLOAD
artifact: implementation-handoff
status: current
last_reviewed: 2026-11-04
---

# Generic Workload / Template Driver implementation

Approved scope: [ADR-035](../architecture/decisions/ADR-035-generic-workload-template-driver.md),
[contract](../architecture/contracts/generic-workload.md),
[sequence](../architecture/components/generic-workload-sequence.puml).

Implementation is complete on `features/refactor_workload`. Tests are focused
on changed behavior and direct consumers, per the user's explicit instruction;
whole-suite checkpoints and unrelated scenarios are excluded.

The new source registry reuses hardened Git/ZIP inspection, bounded Go workers,
cache and immutable publication. Inline accepts a bounded map of file paths to
content and prepares the same contract. A source record's identity includes the
renderer fingerprint, including the running executable and linked dependencies,
so upgrading a runtime does not overwrite an old pin.
Missing historical engine/content blocks rendering rather than silently changing
an old revision. Initial standard sources are immutable prepared content, not a
native-renderer fallback.

Resource References reuse existing graph scanning, output schema validation and
bindings. Namespace placement supplies the cluster when it is already determined;
a redundant explicit cluster remains optional and is checked for consistency.
Secret outputs cannot be passed as plain template Resource References. Applied
revision metadata stores nonsecret values; existing encrypted CD artifacts store
the actual manifests. Source edits require new preparation and Definition revisions.

PostgreSQL version 8 removes obsolete experimental workload configuration and
renames only affected source/draft persistence. It refuses retained workload
history requiring an explicit reviewed cleanup. No shared installation has been
upgraded or reset for acceptance. Versioned migrations/references 1–7 stay frozen.

The dedicated acceptance Console uses its own Compose project, PostgreSQL/Vault,
cache and ports. User authorization permits task-owned resources on k8s-4f only.
The first deployment exposed missing Fleet permissions in the newly provisioned
namespace; a task-owned Role/RoleBinding supplies the existing acceptance rules.
The failed operation is recovered through the product API/UI after proving its
execution stopped. Setup is not counted as application deployment acceptance.
Baseline, update, exact Fleet commit/Bundle verification and product cleanup passed.
See the [execution record](../verification/2026-11-04-generic-workload.md).
The current editor was compared in a real browser with the existing Definition
editor and shared UI patterns. Shell, tabs, criteria, JSON review and version
history are reused. Schema/defaults, pins and companion JSON are disclosures;
Overview shows the selected template source rather than an irrelevant module.
