---
id: VERIFY-2026-11-09-STABLE-TEMPLATE-RENDERER-PIN
artifact: verification
status: current
last_reviewed: 2026-11-09
related: [UC-03, ADR-041]
---

# Stable template renderer pin — issue #57

## Scope

Branch `features/template-renderer-pin`, commit `4e02db2c`. Decision:
[ADR-041](../architecture/decisions/ADR-041-stable-template-renderer-pin.md),
refining [DV-05](../usecase/UC-03/definition-versions.md).

## Automated checks

- `identityFrom`: deterministic; an unrelated dependency version does not change
  it; a Go toolchain, renderer library or replaced renderer library change does.
  The running renderer reports a 64-hex identity.
- Template driver, render engine and template source packages pass, including the
  golden `TestStandardBundleExamples`.
- With real PostgreSQL (`REQUIRE_POSTGRES=1`), template sources, catalog, planning,
  deployment and HTTP delivery packages fail the same 15 tests as unmodified `main`
  `cc4320af` (identical list); no new failure. Vet, gofmt and documentation checks pass.

## Kind run

Two images were built from the commit: A as is, and B with one log string changed
in an unrelated package. Their executables differ (SHA-256 prefixes `2aa91b66`
and `13350959`). The images were imported into the kind node through a temporary
`kubectl debug node` pod (WSL interop to the cluster distro was unavailable),
which was deleted right after the import.

| Step | Result |
|---|---|
| Install isolated Console with A, seed (template registered under A) | template source AVAILABLE |
| Application `tpl57`: workload saved through the form, Deploy | SUCCEEDED |
| Upgrade backend/frontend to B | same template source still AVAILABLE |
| Change the workload arguments, Deploy on B | SUCCEEDED; Deployment runs the new arguments |
| Delete the Application through the Console | SUCCEEDED, namespace absent |

The renderer-change direction (UNAVAILABLE after a Go, library or renderer source
change) is covered by the unit tests, not by a kind run.

## Cleanup

The isolated Console namespace and its cluster role binding were deleted. The
imported test images remain in the node image store; the delivery Git repository
is retained per ADR-033.
