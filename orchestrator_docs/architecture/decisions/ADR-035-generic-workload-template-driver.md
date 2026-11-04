---
id: ADR-035
artifact: architecture-decision
status: current
last_reviewed: 2026-11-04
---

# ADR-035 — Generic Workload and Template Driver

## Decision

The approved change replaces Score, score-k8s and the Render Bundle product
with Generic Workload and the `template` driver. There are no retained running
applications requiring compatibility. Discard obsolete experimental workload
configuration only; preserve Terraform Definitions, accounts and connections.
Never reset the installation or modify released migrations.

One Resource Definition model owns identity, criteria, revisions, mappings and
companion resources. `workload` is an ordinary Resource Type using `template`.
Resource Definitions owns its editor; remove the separate Rendering Templates
navigation/page. Preserve the current shell and editor patterns.

Use Go text/template with the existing closed helpers and isolated bounded
worker. Templates generate objects directly: there is no Score intermediate,
score-k8s process, provisioner stage or native renderer fallback in production.
Reuse verified Git/ZIP content preparation, immutable cache and catalog releases
as template-source capabilities, not a second Definition system. Inline source
is prepared and pinned by content digest before saving a Definition.

Generic Workload preserves multiple containers, env/config/secret bindings,
commands, probes, compute resources, Service/public routes, storage, StatefulSet
and CronJob intent. A simple single-container form is only an authoring shorthand.
Explicit workload values take precedence over Definition defaults. Template inputs
cannot authorize new targets or Secret references. Existing dependency resolution
supplies Resource References; a redundant cluster reference is not required when
the existing placement mechanism already determines the target.

Preview pins workload content, Definition version/hash, template source/content,
effective inputs and dependency bindings. Unprovisioned outputs remain symbolic.
Deploy records the resolved nonsecret render input and rendered artifact/digest;
secrets remain approved versioned references. Replaying an old artifact and
deploying against changed dependencies are distinct operations; the latter creates
a new revision. Publishing source or changing a Definition never mutates old pins.

Keep semantic guards, ownership/namespace/Secret authorization, inventory,
readiness and cleanup. CD/Fleet receives the resulting manifests through its
existing interfaces. Terraform provisioning, state identity and delivery topology
are outside the redesign.

## Supersession and validation

This decision supersedes Score/score-k8s/Render Bundle requirements in ADR-010,
ADR-016, ADR-021 and workload-catalog clauses only. Their infrastructure, source
verification, ownership and secret protections continue. See the
[contract](../contracts/generic-workload.md).

Run focused unit/integration tests for changed behavior and direct consumers,
not whole suites or every scenario. External environment mutation requires its
own authorization. Implementation status and evidence are recorded separately.
