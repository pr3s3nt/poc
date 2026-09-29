---
id: VERIFICATION-ACCEPTANCE-PLAYWRIGHT-KIND-20260929
artifact: verification-record
status: current
last_reviewed: 2026-09-29
---

# Diagnostic acceptance app through Web Console — 2026-09-29

- Command: `bash backend/test/integration/acceptance-playwright-kind.sh`.
- Context: `kind-idp-internal`; direct workload delivery with VSO. Run ID:
  `acceptance-20260929061451-435`; Application:
  `e15b7607-efb2-4c7d-92b9-d15546049340`.
- Playwright signed in, created the Application, saved two UC-12 Variables and
  one Secret, imported backend/frontend Score, previewed two workload changes
  and deployed both successfully through the Web Console.
- The browser opened the deployed frontend by port-forwarding its Service.
  Its diagnostic table showed `PASS` for backend connection, environment,
  secret and database. `/api/checks` returned all three backend booleans true
  without the secret value.
- The run-scoped namespace was deleted and absence checked. Images loaded into
  the kind node and Vault test revisions remain; the current Vault lifecycle
  does not automatically prune old revisions or bundles. Temporary logs are at
  `/tmp/tmp.nW8S5kPtS7`, outside the repository.
- Go tests/build, Web Console typecheck/lint/test/build, documentation checker,
  shell syntax and `git diff --check` passed. This run did not test the worker
  job flow, Fleet delivery, cloud delivery or ingress from an external host.
