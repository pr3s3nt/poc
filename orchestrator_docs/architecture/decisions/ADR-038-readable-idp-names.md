---
id: ADR-038
artifact: architecture-decision
status: current
last_reviewed: 2026-11-06
related: [ADR-033, ADR-036, UC-01, UC-06]
---

# Readable names for IDP-managed application resources

## Decision

New Applications keep their human display name and an internal immutable UUID.
Their immutable application key is `idp-<organization-key>-<application-slug>-<suffix>`.
The slug is derived once from the submitted name: lowercase Latin letters and
digits, Vietnamese accents folded (including đ), punctuation/whitespace collapsed
to hyphens, no leading/trailing hyphen. The Organization key uses the same
normalization. A name without a usable slug is rejected on the Name field.

The user approved a short random suffix on 2026-11-06. Generate six lowercase
base32 characters from cryptographic randomness once per new Application and
persist the complete key atomically. Every derived resource name shares this
same suffix at its end, after the purpose or Environment. Never regenerate it on
Deploy, retry, update, display-name edit or restart. A recreated deleted app gets
a fresh suffix, UUID, Environment identities and repository; its old history stays.

The generated key, including suffix, is at most 38 characters, preserving the
existing 49-character infrastructure-name budget including `-production`. Reject
overlong/unusable names on Name without silently truncating the readable stem.
Case-insensitive active display-name uniqueness within the Organization remains;
different names that normalize to the same slug get different keys. Retry a
rare generated-key collision before any writes, including tombstone collisions.
The database globally unique key remains the final concurrent creation guard;
no database schema change is needed.

| Object | Name for Organization `nangcap`, Application `Order Management` |
|---|---|
| Application display name | `Order Management` |
| Application key | `idp-nangcap-order-management-a7k3m2` |
| Private delivery Git repository | `idp-nangcap-order-management-deploy-a7k3m2` |
| Managed Fleet GitRepo | `idp-nangcap-order-management-deploy-a7k3m2` |
| Staging namespace/infrastructure stem | `idp-nangcap-order-management-staging-a7k3m2` |
| Production namespace/infrastructure stem | `idp-nangcap-order-management-production-a7k3m2` |

Git and Fleet share the same persisted application repository name across both
Environments. Existing environment/workload/phase paths remain. Workloads keep
their short authored names (`web`, `api`, `worker`, `postgres`, `redis`) inside the
namespace. Source repositories and preexisting/shared platform resources are not
renamed. Provider release/unit IDs and ownership metadata remain internal; names
are never authority to adopt or delete resources.

## Stability and compatibility

Changing a display name never regenerates the key, repository or namespaces.
Existing Applications, allocations, Fleet controller names, receipts and target
pins keep their historical naming rules. Deploy, observation, retry, recovery and
cleanup resolve those same identities; no live migration, cluster mutation or
remote repository rename is part of this change. New Applications use the new
key even when the platform's pinned binding predates this decision.

Generation zero uses the readable infrastructure stem. Later target generations
retain a bounded readable prefix and a deterministic generation/digest suffix to
preserve provider limits and generation isolation. UUIDs, exact repository IDs,
markers, target UIDs, receipts and operation fences remain cleanup authority.

## Validation

Cover Vietnamese normalization, punctuation, empty/overlong slugs, random suffix
shape/collision retry, concurrent duplicate display names, normalized aliases,
Organization separation, deleted-name recreation, tombstones, atomic
rollback, immutable names after display-name edits, shared staging/production
allocation/controller identity, historical names and provider length limits.
Local tests do not establish a real GitHub/Fleet deployment.
