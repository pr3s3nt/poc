# First-run Console setup

Specification: [UC-00 SETUP](../../../orchestrator_docs/usecase/UC-00/first-run-setup.md).
Uses real backend/PostgreSQL and the user's explicit Git repository through the
feature Tailscale URL. Install `npm ci --prefix frontend` and Playwright Chromium.
Start a task-owned Docker stack with `deploy/local/compose.first-run.yml` and
feature URL overlay per the [handoff runbook](../../../orchestrator_docs/operations/tailscale-handoff.md).
Copy its private setup token to an owner-only file outside Git; create a private
Admin password file. Never point the scenario at a shared installation.

```sh
SETUP_URL=http://verified-feature-host:8088 \
SETUP_TOKEN_FILE=/private/token SETUP_PASSWORD_FILE=/private/password \
SETUP_GIT_URL=https://git.example.com/platform SETUP_GIT_REF=reviewed-ref \
SETUP_GIT_PATH=resource-catalog SETUP_EVIDENCE=/private/evidence \
node e2e/scenarios/first-run-setup/run.mjs
```

Assertions: browser initializes the Organization/Admin when needed, local sign-in,
closed claim state, every platform form loads, Git discovers workload Type and
Definition, explicit import of every Type and the workload Definition with a real Git template, REUSED rerun and persisted progress on
reload. The reference repository supplies 10 Types and the workload Definition. Other
platform Definition acceptance requires environment values, real accounts,
module inspection and existing controller prerequisites. It is not workload deploy
acceptance. No fixture backend, cloud mutation or controller installation.

Side effects: setup-trial Organization, setup-admin and selected Types/Definition persist in
the task stack. Repeat uses that same account and data. Browser cookies remain in
memory and screenshots are taken only after credential fields have been cleared.
Evidence lives outside Git, without browser storage state or raw API bodies.
Cleanup: remove only the owned Compose project/volumes after user acceptance;
keep it running while handing off. Do not reset an existing installation to rerun.
