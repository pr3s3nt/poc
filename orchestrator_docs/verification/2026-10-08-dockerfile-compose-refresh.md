---
id: VERIFY-20261008-DOCKERFILE-COMPOSE-REFRESH
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-08
---

# Backend/frontend Dockerfile and Compose refresh

Both Dockerfiles match the content supplied by the user. Local `clauded`
implemented the Docker changes; the coordinator reviewed them and updated the
[Compose runbook](../operations/docker-local.md). Compose uses `pull_policy: build`
for both application images so ordinary `docker compose up` builds current source.

## Final combined verification

After the frontend Dockerfile update and Compose review, one combined
`docker compose up -d --wait --wait-timeout 180` passed. Backend, frontend,
PostgreSQL and Vault were healthy; existing database/Vault volumes were preserved.
Backend `/api/v1/healthz` and the frontend proxy at the same path returned
HTTP 200 with `status=ok`. Runtime `nginx -t` passed.

A headed Playwright smoke test exercised the rebuilt frontend: typed the local
platform-engineer login, signed in and opened Secret stores. The automatically
seeded ordinary Platform Vault displayed READY. Full-window recording:
[Final Docker UI smoke](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/docker-final-smoke-20261008.mp4).
The MP4 passed full ffmpeg decode validation.

## Checks and limits

Compose config, documentation checker and whitespace checks passed. Image builds
compile the backend and build the frontend bundle. Product Go/TypeScript source
was unchanged, so full unit/lint suites were not repeated. This verifies local
Docker startup and UI/API access, not Kubernetes deployment or cloud operations.
Synthetic test inputs and build logs remain outside Git under
`/tmp/poc-docker-refresh-review`.
