---
id: VERIFY-2026-11-02-CD-ONLY
artifact: verification-record
status: current
last_reviewed: 2026-11-02
related: [ADR-032]
---

# CD-only application manifests — local verification

## Scope

The user approved retiring Direct application delivery. This change requires a
verified Environment CD binding for real Preview/Deploy, removes the Console
toggle and process-wide CLI switch, and moves nonsecret auxiliaries and secret
delivery declarations into the CD artifact. Terraform ownership stays unchanged.
Historical Direct receipts and exact-owner cleanup remain for migration; no new
Direct destination is executable. Fake adapters use explicit simulation services.

## Checks

- Backend `go test ./...` passed. After subsequent secret-delivery refinements,
  focused application/adapter/bootstrap/HTTP tests and `go build ./...` passed.
- Regression coverage includes rejecting Direct before provisioning; denied
  Direct destination/selection; complete CD-owned dependency secret delivery and
  cleanup through both Vault Agent and VSO with a collaborator that panics on
  direct application operations; unchanged historical recovery; VSO destination
  ownership/required keys; and certificate-only public CA admission.
- Frontend typecheck, lint and production build passed. The final full suite
  passed **776 tests across 82 files** with `--maxWorkers=4`. An earlier default
  concurrency run had one observability timing failure; its focused rerun and
  the final complete suite passed without changing observability code/tests.
- Documentation checker and `git diff --check` passed. PlantUML regenerated the
  updated delivery sequence PNG. Changed E2E JavaScript passed `node --check`.

## Browser review

Chromium rendered the actual AppShell, styles and DeliverySelectionPanel with
fixture API responses before and after the change. The new panel shows the
required provider selector, no enable/disable toggle, an unapplied-CD state and
disabled Save until an eligible provider is chosen. No browser runtime exception
was observed. Component tests additionally cover stale-save reload, read-only
Developer access and no verified provider. Local screenshots are outside Git.
This is fixture UI evidence, not live Git/Fleet/VSO acceptance or a hosted preview.

## Limits

No PostgreSQL test server was configured and `REQUIRE_POSTGRES` was unset, so
database integration cases used their existing skip behavior. No schema or
versioned migration changed. No retained database, workload, cluster or cloud
resource was mutated. Real Fleet/VSO prerequisite readiness, autonomous drift
correction and child garbage collection still require a separately authorized
live acceptance run. Historical optional-CD acceptance does not establish those
new semantics. Existing applications were not automatically migrated to CD.
