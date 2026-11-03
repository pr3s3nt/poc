---
id: IMP-DEFINITION-VERSIONS
artifact: implementation-map
status: current
last_reviewed: 2026-11-03
---

# Controlled Definition upgrades

Source of behavior: [UC-03 DV-01–07](../usecase/UC-03/definition-versions.md).
The existing Definition and Rendering Template forms remain the authoring entry
points, including Catalog/Git/ZIP module sources.

| Concern | Implementation |
|---|---|
| Immutable snapshots and revision policies | `domain/resource/revision.go`, `ports/persistence/definition_versions.go` |
| Active defaults, frozen Environment resolution and explicit selection | `application/definitionversions/service.go`, `application/catalog/versions.go`, `application/catalogrelease/selection.go` |
| Memory/JSON and PostgreSQL persistence | both `adapters/{store,postgres}/definition_versions.go` |
| Owner upgrade and legacy configuration capture | `adapters/postgres/migrations/006-definition-versions.sql`, complete `version-006.sql` |
| Preview tokens, exact snapshots and fenced activation | `planning/model.go`, `planning/plan.go`, `application/deployment/{snapshot,service}.go`, `application/pending/{preview,deploy}.go` |
| Original applied evidence and role-scoped endpoints | `delivery/http/definition_versions.go` |
| History, compare, default/archive and Environment selection | `frontend/src/features/platform/{DefinitionVersions,EnvironmentDefinitionVersions}.tsx` |
| Retained executables and filesystem provider mirror | Terraform registry and score-k8s `retained_tools.go`, `registry/retained_providers.go`, `bootstrap/retained_providers.go` |

## Important boundaries

Migration captures current configuration and release bindings, not lost historical
revisions. Applied Usage derives from actual resource/workload Plans; missing IDs
remain unverified. A full applied Environment policy advances only after successful
completion; a partial deployment may report multiple actual resource revisions.

Durable module/render cache volumes must survive platform recreation. On startup,
verified current binaries and release metadata are archived there. Future compatible
Git rendering can use the original archived renderer binary; provider archives are
copied only from the product's explicit filesystem-only installation policy.
Arbitrary custom Terraform CLI policies remain operator-managed. No runtime fetch
or substitution repairs missing/corrupt artifacts.

The installed renderer also includes Go adapter/provisioner behavior. This task does
not preserve old Go programs. If that contract changes, a historical installed pin
is blocked until an explicit compatible revision is selected; existing Pods keep
running. Historical binaries not previously archived must be supplied and verified
by the operator. Database/storage migration and data rollback are not implemented:
existing data components must prove RETAIN intent during an upgrade.

Verification and exact runtime scope: [2026-11-03 evidence](../verification/2026-11-03-definition-versions.md).
