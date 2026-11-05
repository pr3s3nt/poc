# Console user guide

Read-only browser checks for [GUIDE-01–05](../../../orchestrator_docs/architecture/ui/user-guides.md):
sidebar navigation, Developer/Platform Engineer access to the approved Developer
article, same-origin screenshots, image modal keyboard/focus, table-of-contents
anchors, reload, collapsed navigation and signed-out deep-link gating. Uses the actual Console/backend;
no mock responses. Does not deploy or modify applications/resources.

Prerequisites: a built/running Console with the new route, a ready backend and
an account with role DEVELOPER, PLATFORM_ENGINEER or ADMIN. Install dependencies
as in [E2E guide](../../README.md). Obtain credentials from private files; do not
place them in arguments, logs or Git. Use the existing shared login helper.

From the repository root:

```bash
ORCH_PLATFORM_URL='<actual Console origin>' \
ORCH_PLATFORM_USERNAME=developer \
ORCH_PLATFORM_PASSWORD_FILE='<private password file>' \
ORCH_GUIDE_ROLE=DEVELOPER ORCH_HEADLESS=1 \
ORCH_EVIDENCE_DIR=/tmp/orchestrator-user-guide \
node e2e/scenarios/user-guide/run.mjs
```

Repeat with the Platform Engineer account and `ORCH_GUIDE_ROLE=PLATFORM_ENGINEER`.
The runner checks authenticated role, 15 loaded illustrations, same-origin asset
URLs and no application mutation requests. It stores one screenshot per role
outside Git. It does not navigate to or mutate Kubernetes/cloud.

Side effects: a temporary authenticated browser session and sidebar display
preference in that disposable context. Cleanup signs out and closes that context;
no app/resource cleanup needed. Evidence is retained at ORCH_EVIDENCE_DIR.
This scenario does not establish that the tutorial's deploy steps were executed.
