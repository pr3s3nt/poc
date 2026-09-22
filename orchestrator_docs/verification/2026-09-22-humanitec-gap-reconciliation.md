---
id: VERIFY-2026-09-22-HUMANITEC-GAP-RECONCILIATION
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-22
---

# 2026-09-22 — Humanitec gap documentation reconciliation

## Scope reviewed

- Humanitec/challenge Deployment Delta shape and array diff contract.
- Score subset field `containers.*.resources.requests/limits`.
- Product scheduling boundary between UC-08 resource nodes and UC-06 workload
  rendering.
- Scope actually asserted by the 33-fixture product conformance harness.
- Full-style resource provisioning versus deferred Humanitec external
  deployment lifecycle compatibility.
- Resource Definition/secret/context/Score boundary differences and the
  empty-criteria conformance-loader behavior.

## Outcome

- UC-05/06/07 now require Humanitec-shaped `modules.add/remove/update` and
  `shared` Delta semantics; domain/database design models an immutable
  `DeploymentDeltaSnapshot` for execution while preview remains transient.
- Mutable Humanitec Delta lifecycle is kept separate at D05 instead of being
  conflated with the MVP Snapshot.
- UC-05/06 now require typed CPU/memory requests/limits to survive Score
  conversion and reach Kubernetes workload manifests.
- UC-08 explicitly excludes container requests/limits from resource-node
  execution.
- IMP-008/009 record that current code still uses a flat whole-document patch,
  rejects container resources and renders hard-coded requests.
- The compatibility matrix records driver/account/secret/source/context,
  probe/replicas and namespace-output differences; D06 owns the future public
  boundary decisions.
- UC-03 BR-07 rejects missing/empty criteria at product registration; IMP-010
  separately records that the conformance adapter must skip such external
  Definitions instead of manufacturing wildcard `{}`.
- UC-06 BR-12 owns scoped descriptor tokens; the compatibility matrix now also
  records `res.*` context keys and profile-driven implicit infrastructure as a
  product extension.
- D05 keeps standalone Delta API, async deploy, whole-set workload apply,
  content-addressed Set IDs and incremental mode deferred rather than silently
  adding them to the current MVP.
- Historical execution evidence was not rewritten.

## Validation

- `python3 scripts/check_docs.py`: pass after all Markdown/PlantUML changes.
- Modified PlantUML sources rendered successfully and their sibling PNG files
  were regenerated.
- No Go/frontend/kind/AWS test was run because this change is documentation-only
  and does not authorize external mutation.
