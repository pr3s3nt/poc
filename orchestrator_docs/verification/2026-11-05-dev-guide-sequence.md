---
id: VERIFY-20261105-DEV-GUIDE-SEQUENCE
artifact: verification-record
status: evidence
last_reviewed: 2026-11-05
---

# DEV guide — correct screenshots to match creation sequence

Corrects the illustration approach in the [first draft record](2026-11-05-dev-guide.md).
The initial published draft used an existing deployed app, which did not match
the empty state immediately after creation. Those images were removed from the
[current guide](../guides/dev/acceptance-app.md); a deployed app is no longer
presented as a creation-stage illustration.

## Actual walkthrough

Using the existing shared Console helper and role DEVELOPER, opened the Console
through its existing Tailscale MagicDNS origin on port 3043. Created a dedicated
Application through the UI:

- Name/subdomain: `acceptance-dev-guide`.
- ID: `3cbd3e82-777d-4f7b-9374-f2b54d9dbb94`.
- Environment modified: staging only; production untouched.

Captured 15 images in sequence: filled creation form, newly created empty home,
workload template picker, untouched backend creation form, backend basic inputs,
PostgreSQL only, PostgreSQL plus Redis, first output binding picker, masked secret
form, completed backend before save, home with backend only, worker creation
form, home with two drafts, frontend creation form with public `/`, home with
three drafts. Images show actual product states, not mocked API responses.
The first home explicitly shows no workloads, no resources and no deployments.

Created a random test-only Application secret through the UI using the existing
`Git dev Vault` store identified by the environment setup. The value was never
printed or included in plaintext screenshots. Stored only its reference in the
workload and a public SHA-256 check value. Backend draft declares new shared
PostgreSQL/Redis; worker selects those existing dependencies through the form.
Frontend selects `backend / http` and declares public route `/` to `http:8080`.

All three workload drafts were saved through the UI. Home screenshots after each
save confirm one, two, then three pending workloads; no provisioned resources or
deployments. No Preview/Deploy, Kubernetes/cloud mutation, job submission or
volume write was performed. Existing acceptance app was not changed.
Remaining runtime sections explicitly have no screenshot evidence in this run.

## Review and retention

[Static guide review](http://desktop-bdjp2rq-1.tail342836.ts.net:5173/) is the updated
document, not Console or workload web. Chromium opened this exact Tailscale URL
and checked that all 15 replacement images loaded. Images were visually checked
for the empty initial state and final pending state. This host's verification
does not establish access from another tailnet device.

Screenshots/HTML remain outside Git at
`/home/thanhnt1/.local/share/orchestrator-dev-guide/`; the existing static server
and Tailscale Serve mapping remain active. Restart command is in the prior
record. Original screenshots remain as historical private capture files but are
not referenced by the revised guide. Application, three drafts and test secret
are retained for review; no application runtime data was created.

Documentation checker and `git diff --check` passed. Product suites were not run
because only documentation changed. No user-owned local edits existed at start.
