---
id: VERIFICATION-ACCEPTANCE-PLAYWRIGHT-HUMAN-VIDEO-KIND-20260929
artifact: verification-record
status: current
last_reviewed: 2026-09-29
---

# Human-paced diagnostic acceptance flow on kind — 2026-09-29

- Command: `bash backend/test/integration/acceptance-playwright-kind.sh --human`.
- Context: `kind-idp-internal`; run ID
  `acceptance-20260929065646-20390`; Application
  `4729ac5c-f3d7-4c9b-923a-49b7673dad6e`.
- Playwright moved a visible cursor to each control, typed every value key by
  key and entered both workloads through the workload form: PostgreSQL
  resource, five resource-output bindings, two variable and one secret binding
  for `backend`; a Workload Service binding and public path `/` for
  `frontend`.
- Preview reported two affected workloads and Deploy succeeded for both.
- Backend connection, environment, secret and database checks all passed.
  A job submitted from the deployed page was stored and listed; no worker was
  deployed, so it stayed pending.
- Video: [release asset](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/acceptance-human-kind-2026-09-29.webm)
  in the private `acceptance-recordings` pre-release; WebM, 285.84 seconds,
  1280x800, 16,195,266 bytes, SHA-256
  `7ed611268838df7bce7c60bdb665b2621fcc6eeb5ccb5e6a9dcf4f20dd7b65cf`.
  Sampled frames were reviewed; password and secret inputs are masked.
- The empty Application page showed `Cannot read properties of null (reading
  'slice')` under Recent deployments before the first deployment; the flow
  was not blocked.
- The run-scoped namespace was deleted by the script cleanup. Images loaded
  into the kind node and Vault test revisions remain under the existing test
  lifecycle limitation.
