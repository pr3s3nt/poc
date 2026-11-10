---
id: VERIFY-2026-11-10-ENDPOINT-SEED
artifact: verification
status: current
last_reviewed: 2026-11-10
---

# Endpoint Type and Definition seed — issue #66

The user explicitly requested implementation and tests directly on `main`.
Base: `48454c87`. Changed only the operator script/input, tests and docs;
backend/frontend product code and database schema are unchanged.

## Input and behavior

The same example now declares eight Types and eight Definitions. The additional
Type `endpoint` takes required `name`/`port` and optional `path`, and exposes
nonsecret string outputs `hostname`/`url`. Its built-in `endpoint` Definition
uses `endpointRootDomain`, `endpointScheme`, `gatewayName`, `gatewayNamespace`
and `gatewaySectionName` parameters. It has no attached Driver Account or Git
module/template source. Existing source inspection, conflict and reuse behavior
is preserved. See [procedure](../../scripts/bootstrap/README.md#chỉ-seed-resource-definitions-lần-đầu--issue-62)
and [EP contract](../architecture/contracts/endpoint.md).

## Real runtime verification

Created a fresh isolated installation in the authorized kind cluster using
PostgreSQL 16, Vault and backend image `main-de6c50df` (existing product APIs).
Prepared Admin, stored Git access and uploaded Kubernetes Account as separate
prerequisites. API reads before the command confirmed zero Types/Definitions.

The full example input used the user's source repo
`https://github.com/pr3s3nt/orchestrator-workload-templates`, commit
`2a9be1b1b8150ef146a5e629ea4d818eea03f2a8`. All eight Types and Definitions,
including MySQL and endpoint, were CREATED, followed by `Definitions SEEDED`.
The unchanged rerun reported eight Type and eight Definition REUSED entries,
then `Definitions SEEDED`. API read-back confirmed exact domain, scheme,
Gateway/listener policy, built-in driver, no attached account/module source,
nonsecret hostname/url outputs and no duplicate catalog records.

The test selected the existing Traefik Gateway `httproute-domain-gateway` in
`httproute-domain-test`. Read-only inspection confirmed Programmed=True and
HTTP listener `web` on port 8000 with allowedRoutes namespaces=All. Test policy
used scheme `http` and reserved example root domain `seed66.example`; no DNS
record or TLS availability is claimed. The Gateway/controller were not changed.
No workload deploy, HTTPRoute creation or browser endpoint test was performed:
seed validates registration policy, not public connectivity. Actual company
domain/Gateway/listener still need to be supplied in the private input.

## Automated checks and cleanup

- Focused seed/API tests: 26/26 pass. New checks cover endpoint creation/reuse
  without Git inspection, rejected account/source overrides, policy conflict
  without overwrite and all eight example entries.
- Full bootstrap suite in kind using the pinned Playwright tooling image:
  94/94 pass, zero fail/skip, including the existing Chromium/video/Git tests.
  Command: `node --test --test-concurrency=1 "lib/**/*.test.mjs"` from
  `scripts/bootstrap/`.
- Docs checker, whitespace and Node syntax checks pass. PlantUML unavailable
  locally; no diagram changed. Backend/frontend suites were not repeated because
  product code and contracts did not change.
- Run-owned namespace `seed66-runtime` and its storage PV deleted and verified
  absent. Temporary forward stopped. Shared dev data, Traefik/Gateway, other
  namespaces and sessions preserved. Private credentials/input/receipts/test
  payload/log stay outside Git. No product image rollout is needed for this CLI.
