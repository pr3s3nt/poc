# Workload authoring controls — issue #47

Goal: verify picker Cancel/Escape while loading, reopening/custom selection,
default-visible Service/dependencies, explicit inline endpoint fields and editor
Cancel preserving Environment. Uses the actual Console and data, no mock routes.
See UC-16 specification/UI, endpoint contract, e2e/README.md and the Tailscale
handoff runbook. This is authoring verification, not deployment acceptance.

Prerequisites: the feature Console, an accessible Application, registered endpoint
Resource Type and existing cluster/Definitions. Set shared Console login variables,
`ORCH_APPLICATION_KEY`, optional `ORCH_ENVIRONMENT` (default staging), and
`ORCH_TEST_RUN_DIR` outside Git. Run `node e2e/scenarios/workload-authoring-ui/run.mjs`.

Assertions: picker closes and can reopen, groups are visible without advanced
disclosure, endpoint is absent until explicitly requested, name/port/path are
editable, Cancel returns to the same Environment and no drafts are saved.
Side effects: sign-in/out and browser-local form edits only; no Save/Preview/Deploy
or platform mutation. Screenshots and receipts remain outside Git. Closing the
browser cancels local edits; no Kubernetes/application cleanup is required.
