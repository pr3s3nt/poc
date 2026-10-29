---
id: ADR-021
artifact: architecture-decision
status: current
last_reviewed: 2026-10-28
---

# ADR-021 — Git-owned Kubernetes manifest rendering policy

## Decision and scope

User decision 2026-10-24: keep infrastructure planning/provisioning and its
Terraform lifecycle unchanged; move the recipe for the subsequent Kubernetes
manifest generation into a versioned Git directory. Include normalization,
resource defaults, resource-output provisioners and manifest patches, not just
PE-authored patches. Deliver Console authoring and real-adapter Playwright
acceptance. This is accepted required design, **not implemented capability**;
see [IMP-035](../../implementation/deviations.md#imp-035--git-owned-render-bundles).

One directory is a self-contained render bundle. The platform accepts credential-free
HTTPS Git URL + relative directory + ref, resolves it once, inspects and caches
immutable content. Definitions reference the verified bundle plus typed literal
inputs. Plans pin the exact source commit/content, engine contract and binary.
Publishing Git changes does not mutate a Definition or automatically deploy.
An explicit Definition edit selects a new pin; Preview then detects rendering-only
changes even without a Score draft. No backend rebuild/restart is needed for a
valid bundle change under the supported engine contract.

## Closed implementation choices

- Continue score-k8s 0.15.0, with platform-owned binary verification. Do not add
  Helm, Kustomize, Python, shell plugins, arbitrary executables or another driver.
- Use Go `text/template` with a closed deterministic helper set for normalization
  and final patches. Use score-k8s native `template://` output-only provisioners.
  The exact stage formats and contexts are in the
  [Git bundle contract](../contracts/git-render-bundles.md).
- A Git bundle must not call the native Kubernetes renderer to construct a
  hidden policy manifest. Move configurable transformation/defaults/labels into
  files. Keep semantic validation and secret/ownership authorization in platform.
- *Superseded for Git bundles by [ADR-024](ADR-024-open-workload-objects.md)
  (2026-10-25).* Keep the current object/lifecycle envelope: one workload Deployment and its
  optional Service, plus existing platform-owned secret-delivery auxiliaries.
  Ingress remains owned by PublicRouteManager. No new ConfigMap/HPA/PDB/Job/
  StatefulSet/CRD support, resource ownership model or cleanup algorithm is added.
  Earlier conversation examples of wider object sets described future capacity;
  they are not this task's acceptance scope.
- Keep Terraform resources, namespace/cluster references, graph expansion,
  matching weights, targets, state, readiness, retries and resource cleanup intact.
  Output-only provisioners bridge already resolved values; they never recreate
  databases or namespace resources managed by Terraform.
- Default new Console drafts to Git. Keep existing installed bundle selection
  and its persisted plans as an explicit compatibility path; never silently
  convert/re-pin existing Definitions. Publish a standard Git bundle reproducing
  existing behavior and prove a new version changes behavior without Go changes.
- Git metadata is Organization scoped and immutable in PostgreSQL; content is
  held in a durable digest-verified cache. Preview/execution never fetch Git.
  Explicit re-inspection repairs a missing cache for the same immutable pin.
- Use fresh isolated acceptance databases under the existing fresh-baseline
  policy. Do not migrate/reset retained installations as part of this feature.

## Rationale and supersession

Keeping every default and normalization rule in Go requires a platform release
for policy changes. Templates alone would not solve that if native rendering
still overwrote their results. The new engine supplies neutral input and checks
meaningful invariants instead of comparing every output field to a Go-generated
policy manifest. Score-declared intent and authorized references remain protected.

This ADR replaces the **installed-only/no-Git source and Go-owned rendering policy**
restrictions in ADR-010, ADR-014's workload bundle clauses, ADR-016's exact-three-variable
restriction and the workload rendering contract **for Git bundles only**. Existing
installed selections retain their legacy contract. Infrastructure rules of these
ADRs remain current. UC-06 BR-11 now permits Git-owned missing-value defaults;
explicit requests and same-field limits keep precedence. This resolves the design
questions in issue [#5](https://github.com/pr3s3nt/poc/issues/5) within this scope;
it does not authorize an independent inline-patch editor.

## Canonical routes

- Behavior: UC-03 `RB-01..06`, UC-05 `RB-01..03`, UC-06 `RB-01..04`,
  UC-07 `RB-01..02` in their specifications.
- [Engine/source/API contract](../contracts/git-render-bundles.md).
- [Persistence](../database/schema.md#git-render-bundle-persistence--adr-021).
- [Implementation handoff](../../implementation/git-render-bundles.md).
- [Real acceptance procedure](../../operations/git-render-bundles-acceptance.md).

All choices above are decided for implementation. Routine implementation details
belong to the coding agent; no design-approval checkpoint is required. Missing
credentials or unavailable external services must be reported honestly, never
replaced by fake execution or weakened assertions.

ZIP source amendment (2026-10-28): [ADR-026](ADR-026-zip-source-upload.md)
adds uploaded archive preparation and ZIP provenance to this same verified engine
and cache. Existing Git and installed source selections retain their behavior.
