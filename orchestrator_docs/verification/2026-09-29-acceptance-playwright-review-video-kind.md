---
id: VERIFICATION-ACCEPTANCE-PLAYWRIGHT-REVIEW-VIDEO-KIND-20260929
artifact: verification-record
status: current
last_reviewed: 2026-09-29
---

# Review-paced diagnostic acceptance flow on kind — 2026-09-29

- Command: `bash backend/test/integration/acceptance-playwright-kind.sh`.
- Context: `kind-idp-internal`; run ID
  `acceptance-20260929064042-15570`; Application
  `54877317-69fe-4127-ada7-cafa0024ffc3`.
- Playwright used a 150 ms action delay and explicit review pauses at login,
  Application creation, UC-12 entries, each UC-16 import, Preview, Deploy result
  and the final diagnostic screen.
- Backend connection, environment, secret and database checks all passed.
- Video: [review-paced browser flow](artifacts/acceptance-playwright-kind-review-2026-09-29.webm);
  WebM, 80.36 seconds, 1280x800 and 4,747,459 bytes. Chromium loaded its
  metadata successfully. Password and secret inputs are masked by their form
  controls.
- The run-scoped namespace was deleted and its absence confirmed. Images loaded
  into the kind node and Vault test revisions remain under the existing test
  lifecycle limitation.
