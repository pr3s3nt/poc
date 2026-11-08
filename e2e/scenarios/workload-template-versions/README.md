# Grouped workload template versions

Verifies UC-16 Add Workload grouping, latest numeric version by default, explicit
older version selection and exact immutable pins in editor navigation. Uses the
existing Console shell and actual Environment-scoped metadata, with no mocked
responses. See UC-16 specification/screens, e2e/README.md and the Tailscale handoff
runbook.

Prerequisites: install e2e and scripts/bootstrap dependencies, Chromium, a running
Console, an authorized user and Application with multiple `backend` versions.
Set shared ORCH_PLATFORM_URL/USERNAME/PASSWORD_FILE/HEADLESS login variables,
ORCH_APPLICATION_KEY, optional ORCH_ENVIRONMENT (staging default), and
ORCH_TEST_RUN_DIR outside Git. From repository root run:
`node e2e/scenarios/workload-template-versions/run.mjs`.

Assertions: one card per template ID, newest default pin, collapsed version
control, version change alone does not navigate, older selected pin opens its
editor, Cancel returns to the same Environment, unchanged workloads/draft data.
Side effects: authentication and unsaved form navigation only; no Save, Preview,
Deploy or cluster mutation. Cleanup closes unsaved forms/browser; no product
resource is created. Screenshot/receipt/error evidence stays outside Git.
