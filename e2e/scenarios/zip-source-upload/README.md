# ZIP module and bundle upload

Verifies [UC-03 ZIP-01..04](../../../orchestrator_docs/usecase/UC-03/specification.md#zip-source-upload--adr-025)
and [ADR-025](../../../orchestrator_docs/architecture/decisions/ADR-025-zip-source-upload.md).

Prerequisites: a **disposable loopback** platform built from this branch, seeded
Kubernetes Types, configured pinned score-k8s 0.15.0 binary and private module/
render caches; PLATFORM_ENGINEER account. Node 22+, `zip`, `npm ci` and
`npx playwright install chromium` from `e2e/`. Product archive inspection and
cache adapters are real; a local platform may use fake deployment adapters
because this scenario never deploys. It does not verify Terraform apply or
Kubernetes execution.

From `e2e/`, use the shared Console configuration from [the E2E guide](../../README.md):

```bash
ORCH_PLATFORM_URL=http://127.0.0.1:18528 \
ORCH_PLATFORM_USERNAME=platform-engineer \
ORCH_PLATFORM_PASSWORD_FILE=/private/password \
ORCH_HEADLESS=1 ORCH_ZIP_DISPOSABLE=1 \
ORCH_ZIP_EVIDENCE=/tmp/zip-source-evidence \
node scenarios/zip-source-upload/run.mjs
```

The runner archives repository module/bundle fixtures without changing their
layout, uploads both through the browser, checks source/revision/path and catalog
metadata, registers a rendering Definition and reopens it without Git inspection.
Screenshots at 1512px/1280px and overflow assertions cover both upload forms.
Any missing assertion or unexpected HTTP status fails the run. Screenshots are
optional, outside Git, and show only seeded/local fixture data.

Side effects: immutable verified module/bundle records, one uniquely named
rendering Definition and a temporary session. Re-running creates another owned
Definition. There is no DELETE API for immutable pins: run only against a
throwaway platform. The runner signs out and removes its temporary ZIP/source
directory even on failure; the caller must stop its own platform process and
remove its disposable state/cache directory, verifying those owned resources
are absent. Keep evidence outside that directory if desired. Shared, staging,
cloud and Kubernetes platforms are outside this runner's scope.
