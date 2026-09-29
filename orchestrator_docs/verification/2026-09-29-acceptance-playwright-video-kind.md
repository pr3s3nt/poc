---
id: VERIFICATION-ACCEPTANCE-PLAYWRIGHT-VIDEO-KIND-20260929
artifact: verification-record
status: current
last_reviewed: 2026-09-29
---

# Recorded diagnostic acceptance flow on kind — 2026-09-29

- Command: `bash backend/test/integration/acceptance-playwright-kind.sh`.
- Context: `kind-idp-internal`; run ID
  `acceptance-20260929062014-12783`; Application
  `157972c5-3efb-4955-ac08-19d645dfb3ae`.
- Playwright recorded one continuous browser session from sign-in, Application
  creation, UC-12 configuration and UC-16 Score import through Preview,
  successful Deploy and the deployed diagnostic page.
- Backend connection, environment, secret and database checks all passed.
- Video: `/tmp/tmp.RH5w8zpFHT/acceptance-full.webm`; WebM, 22.28 seconds,
  1280x800, 1,226,305 bytes, mode `0600`. Chromium loaded its metadata
  successfully. The video is local evidence and is not tracked by Git.
- The run-scoped namespace was deleted and its absence confirmed. Images loaded
  into the kind node and Vault test revisions remain under the existing test
  lifecycle limitation.
