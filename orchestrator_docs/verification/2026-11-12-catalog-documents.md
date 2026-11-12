---
id: VERIFY-CATALOG-DOCUMENTS-20261112
artifact: verification-record
status: current
last_reviewed: 2026-11-12
---

# Catalog document registration — issue #76

Implemented on `features/dangkiresource`, based on `origin/main` at `9d80dbe4`.
Implementation checkpoint `a0cefc46`; traceability/typed test controls `61488afb`.
No merge, production deployment or issue closure is authorized before acceptance.

## Outcome and scope

UC-02/03 FILE-01..05 add one-document UTF-8 YAML/JSON upload/edit/paste and YAML
export without changing the existing forms. Strict JSON import endpoints preserve
registration validation, source pin/reference requirements and insert-only writes.
Duplicate keys compare typed public contracts plus module/template defaults:
CREATED / REUSED / CONFLICT, with field paths and no configuration values in the
conflict result. Reuse/conflict do not update catalog records or revision history.
Export allowlists public authoring fields from the displayed Type/Definition or
revision, excluding system metadata; source pins and symbolic references remain.

## Focused and browser validation

- Backend import/catalog and strict HTTP tests pass, including authorization,
  system/retired/unknown fields, body size, conflict without overwriting,
  Definition revision preservation, default equivalence and meaningful null values.
- Frontend parser, import control, Type form and revision tests pass. Parser checks
  duplicate keys, cyclic aliases, malformed/multiple documents, oversized content,
  non-string mapping keys and non-finite numbers; bounded ordinary aliases work.
- Real feature-stack browser scenario `e2e/scenarios/catalog-document/run.mjs`
  passes upload/edit/create/reuse/conflict, error draft retention, YAML download
  and round-trip for Type and endpoint Definition. It compares Type/revision
  history before and after reuse/conflict. Repeated runs use distinct keys.
- UI reference: live Resource Types and Resource Definitions pages, shared
  AppShell/styles/Button and existing form/disclosure patterns. Visual comparison
  keeps shell/navigation and form layout. Format help is collapsed; no introductory
  banner was added. The textarea has a stable accessible label after upload.

## Real Preview / Deploy

Git, Internal account and Fleet default were prepared in the isolated feature
stack using existing bootstrap tooling. Imported four Definitions through the UI
with real inspected Git pins: existing-cluster, Terraform namespace, Terraform
PostgreSQL and template workload. Each new criterion was scoped to a newly created
Application, so preexisting Definition selectors were not edited.

A workload and two PostgreSQL dependencies were authored through the existing
form, then one Environment Preview/Deploy selected all four imported Definitions.
Deploy succeeded; namespace/two PostgreSQL resources were READY and the workload
had a Ready replica. Username params were verified against the usernames delivered
through Kubernetes Secret references; password values were never read or printed.
Both params overrode the authored Definition fallback. The existing PostgreSQL
Type requires username: omitting it was rejected, and that Type was not modified.

The deployed PostgreSQL image matched the `image` default published by the exact
source pin. Reimporting that default explicitly returned REUSED and preserved the
entire revision history. Runtime script: `e2e/scenarios/catalog-document/runtime.mjs`.
The first diagnostic script run and the reusable runner preserve separate private
receipts. Runner corrections involved selectors, required Type inputs, resource
projection (platform cluster is excluded), Secret-backed usernames and pinned image
defaults; these were not product bypasses or reductions of business assertions.

## Full validation and baseline limits

Full checks ran on the existing kind cluster in a disposable namespace, with real
PostgreSQL and `REQUIRE_POSTGRES=1`. Main comparison used an independent checkout
of `9d80dbe4` in the same test environment.

- Frontend: typecheck, lint, **110 files / 1048 tests**, production build pass.
  Lint retains the existing `PlatformSettingsPage` effect-ref warning; build retains
  the existing bundle-size notice.
- Backend: full `go test -p 2 -parallel 4 -timeout 20m ./...` fails; `go build ./...`
  passes. The main comparison also fails. Neither run is reported as green.
- After correcting omitted fixture directories in test packaging and rerunning
  the affected packages, the observed top-level failure sets match: **69 tests**
  across **13 packages**, no task-only observed failure. Both PostgreSQL packages
  hit the 20-minute package timeout, so these runs do not prove every remaining
  test completed. The test image also lacks Terraform CLI, which is an environment
  cause for `TestBuildRefusesMemoryOutputSecretsWithRealAdaptersAndDurableState`;
  it is not presented as a new product defect or a passing check.

Affected baseline packages: adapters/postgres, adapters/templatesources,
application/catalog, application/configuration, application/deployment,
application/pending, application/preview, application/provisioning, bootstrap,
test/conformance, test/e2e, test/integration/v2cleanup and test/integration/v2run.
Representative failures include `TestBaselineEnforcesCurrentConstraints`,
`TestWorkloadRegistration_TypedPlacementInputsAndCompanions`,
`TestPlannerMatchesChallengeFixtures`, `TestDeliveryCleansAppliedTargetsWithoutLegacyEnvironmentBinding`
and `TestTheScenarioScriptRecordsExactlyTheAssertionsTheRunnerRequires`.

The initial frontend packaging omitted backend fixtures; after including the
full fixture paths, the full frontend run passed. Initial backend packaging omitted
`platform-catalog` and conformance references; catalog release tests passed after
restoring them, and conformance reproduced the same two main failures. No assertions
were weakened to achieve baseline parity. Comparison receipts remain outside Git.

Documentation checker and `git diff --check` pass. New sequence diagrams were
rendered with the PlantUML Docker image; the host checker notes that no local
PlantUML executable is installed.

## Handoff and retained data

The feature Docker stack and its database, Vault, source cache and delivery Git
volume remain running for review. The verified feature Console URL and private
password-file location are supplied only in the private handoff, not this public
record or the issue. Browser checks used the exact tailnet hostname, real session,
and same-origin catalog APIs; no mock proved reachability.

Private inputs/receipts are under `~/.local/state/issue76/`; diagnostic logs and
screenshots are under `/tmp/issue76-*`. Catalog fixtures remain in the feature DB.
Restart uses the feature URL, kind-network and bootstrap overlays in the
[Tailscale handoff runbook](../operations/tailscale-handoff.md) plus the temporary
loopback-port overlay recorded in private runtime inputs. The normal tailnet
proxy/forward remains available. Full checks remain baseline-red as described;
acceptance does not mean the full suite passed. The exact restart command is
retained privately in `~/.local/state/issue76/restart-stack.sh`. Browser reachability
was verified from the host; no second-tailnet-device verification is claimed.

Both runtime Applications were removed through product deletion, with namespace
UID/ownership checks and removal of only their labelled technical RBAC objects.
Namespace absence was verified. The full-check namespace was separately deleted
with its recorded UID precondition and cleanup verified; no user-owned namespace
or existing local service was removed. The starting checkout was clean, so no
preexisting user-owned changes were included or discarded.
