---
id: VERIFICATION-SELF-HOST-PLAYWRIGHT-KIND-20260929
artifact: verification-record
status: current
last_reviewed: 2026-09-29
---

# Self-hosted Orchestrator deploying the acceptance app on kind — 2026-09-29

- Command: `bash backend/test/integration/self-host-playwright-kind.sh`
  ([runbook](../operations/self-host-kind.md)).
- Context: `kind-idp-internal`; run ID `selfhost-20260929073559-32744`.
- Images `orchestrator-backend` and `orchestrator-frontend` were built from
  `backend/Dockerfile` and `frontend/Dockerfile` and loaded into kind with the
  acceptance images. A local smoke test of both images with the fake adapter
  returned `302 /ui/` on `/`, `200` on a deep `/ui/` route, and API health
  through the nginx proxy.
- The host Orchestrator deployed Application `Orchestrator`
  (`e9728c4b-751e-4c7c-80c7-82b5c89ff02c`): five variables, the kubeconfig and
  Vault token secrets, and workloads `backend` and `frontend` with public path
  `/`. Deploy succeeded for both.
- Chromium opened `http://staging.orchestrator.example.com/` through Traefik.
  The in-cluster Orchestrator deployed Application
  `7e0af19e-e0af-400f-85a4-9722e592b410` with the acceptance workloads. Deploy
  succeeded. The app's public route `staging.<run ID>.example.com` showed
  `PASS` for backend connection, environment, secret and database, and stored
  one submitted job (`PENDING`, no worker deployed).
- Video: [release asset](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/self-host-kind-2026-09-29.mp4)
  in the private `acceptance-recordings` pre-release. It records the full
  Chromium window (tabs and address bar) from Xvfb: H.264 MP4, 576.87 seconds,
  1280x900, 4,011,983 bytes, SHA-256
  `0bba39f53deb612091fa7a57b8dad08f6936a1fdbc9523182383f42bf7abe0ae`.
  Sampled frames were reviewed; the pasted kubeconfig, Vault token and
  acceptance secret appear only as masked password input.
- The acceptance namespace was deleted. The self-hosted Orchestrator namespace
  `app-e9728c4b-751e-4c7c-80c7-82b5c89ff02c-staging` was kept by request, and
  its API answered health through Traefik after the run. IMP-014 limits apply.
- Earlier attempts of the same day failed before the recording completed:
  Xvfb could not bind under WSLg's read-only `/tmp/.X11-unix`, and a click
  landed on the editor's sticky action bar. Both were fixed in the script and
  helpers before this run.
- Also visible: the `Cannot read properties of null (reading 'slice')` message
  under Recent deployments on a new Application, and the sidebar
  `Orchestrator` label rendered dark on the dark sidebar.
