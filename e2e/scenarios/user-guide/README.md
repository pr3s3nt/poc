# Console User Guide — issue #82

Checks [GUIDE-01–04](../../../orchestrator_docs/usecase/UC-16/user-guide-specification.md)
against a real feature Console/backend. Uses the shared
[Console helper](../../README.md) and [Tailscale handoff](../../../orchestrator_docs/operations/tailscale-handoff.md).

## Prerequisites and run

Node 22+, Chromium, a running Console with the feature frontend and a test account.
Install `npm ci --prefix e2e` and `npm ci --prefix scripts/bootstrap` from repository
root; install Chromium with `cd e2e && npx playwright install chromium` if absent.
No fake backend, source fixtures, cluster credentials or application data required.

From repository root:

```sh
ORCH_PLATFORM_URL=<verified-console-origin> \
ORCH_PLATFORM_USERNAME=<test-account> \
ORCH_PLATFORM_PASSWORD_FILE=<private-password-file> \
ORCH_HEADLESS=1 ORCH_TEST_RUN_DIR=<absolute-directory-outside-git> \
node e2e/scenarios/user-guide/run.mjs
```

## Assertions

Capture real Applications reference, open User Guide with keyboard from sidebar,
check article/table of contents, keyboard anchor to secret instructions, exact
synthetic marker example and independent Secret boundary. Verify deep-link,
refresh/back/forward, active collapsed link, 1280/1512 width without overflow,
VI/EN through product language selector, no browser errors or content mutations,
and anonymous deep-link auth gate. The guide is the final sidebar entry, after
disabled History. Select both environment and Workload articles; verify query
URLs, heading focus, per-article anchors, Workload example and pending deletion
instructions, refresh/history and VI/EN without blending article bodies. The component test validates all examples
against the actual .env parser. This runner reads instructions; it does not claim
that environment authoring or deployment was retested.

## Side effects and cleanup

Creates test browser sessions and temporary browser display/language preferences.
No Application, Variable, Secret, workload, Definition or Kubernetes mutation.
Shared helper signs out and closes its context on success/failure; the anonymous
context is also closed. Only screenshots in ORCH_TEST_RUN_DIR are retained;
keep them outside Git. The running feature stack is retained for acceptance.
