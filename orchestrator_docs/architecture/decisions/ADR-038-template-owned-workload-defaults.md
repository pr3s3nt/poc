---
id: ADR-038
artifact: architecture-decision
status: current
last_reviewed: 2026-11-06
---

# ADR-038 — Template-owned workload defaults

User decision 2026-11-06 supersedes the backend `workload-defaults-v1` policy from
issue #37. Default values belong to the selected immutable template. A separate
platform default policy/version/dropdown is redundant and obscures authoring.
The user explicitly permits deleting the old task-owned test workload data;
there is no migration or backwards reader for the removed policy contract.

`runtimeDefaults` supports container/image/network/storage defaults, probe timings,
execution defaults and named environment variants. Backend resolution merges only
missing leaves: user values override the template, including explicit false/zero.
Environment overrides are template data; no CPU/RAM/security/rollout/Cron constants
are supplied by backend defaulting code. Conditional template defaults for enabled
PDBs, existing probes, persistent Deployment mounts and explicitly created Stateful
claims never enable features just because a control exists.

Admission verifies the template pin and freezes its selected environment profile
in `workloadExecution.templateDefaults` {environment,profile}. This is immutable
verified metadata, not a selectable policy or a set of workload overrides. The
planner checks it against the selected source. Renderer, Service readers and
Preview use the same pure merge. Source/profile hash changes require explicit new
publication/selection. New standard template versions are 1.1.0; the capability
is `template-defaults-v1`. Sources advertising the retired backend-policy capability
are excluded from new authoring. Import rejects the removed `defaults` field.

A workload without a template receives no runtime authoring profile from the
backend. Render-bundle fallback constants remain owned by template files, rather
than duplicated in backend or form code. The form shows only resolved values as
muted hints, preserves omission when untouched and has no policy selector.

Security, identity, capability, resource validity, storage retention/ownership and
UID cleanup checks remain backend responsibilities. They are constraints, not a
second defaults policy. No schema DDL or released migration changes are required:
JSONB stores the replacement snapshot metadata. Existing task test data is deleted
through the product before running the new contract, never silently reinterpreted.
