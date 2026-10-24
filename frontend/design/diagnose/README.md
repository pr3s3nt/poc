# Diagnose — UI mock for review

Interactive proposal for checking whether a selected infrastructure target can
support selected platform use cases. This is a product UI with fictional data,
not an approved specification or an implemented diagnostic service.

The mock imports the existing Console `AppShell`, `Button`, message catalog and
styles. Its proposed **Chẩn đoán** navigation entry is attached only within the
prototype. Product routes, navigation sources, backend and deployment behavior
are unchanged. New prototype labels use the runtime catalog; fixture findings
are Vietnamese demonstration data.

## Try the prototype

1. Open the default report: Vault CA trust, missing StorageClass and an expired
   application certificate fail; dependent checks are blocked; execution and
   cleanup remain unverified in the conditions-only mode.
2. Select a check to see observations, required conditions, affected use cases,
   remediation and expandable technical evidence. Blocked checks link to the
   condition that must be fixed first.
3. Open **Thiết lập kiểm tra** to choose a Kubernetes or AWS sample target,
   application, environment, AWS region, use cases and verification level.
4. Run a simulation. Pending checks stay pending until their simulated step
   completes. The run can be stopped, preserving the prior completed report.
5. Switch **Tình huống mẫu** to **Đã sửa cấu hình**. Conditions-only results still
   distinguish unverified execution. The execution mode demonstrates successful
   temporary-resource cleanup. Both modes are entirely simulated.
6. Use **Theo use case**, search/status filters, in-memory history and the JSON
   report download. Exports include `mock: true` and `execution: SIMULATED`.

The scenario selector is a review aid, not a proposed product control. The
empty/error scenarios demonstrate first use and a failed report load. The mock
never asks for credentials or calls cloud, Kubernetes, Vault or product APIs.
History and report configuration reset on reload; only the imported shell's
existing sidebar preference uses localStorage. Recheck buttons rerun the whole
selected scope, including related conditions; selective backend retries are not
implemented. Timing, identities, findings and remediation examples are fixtures.

## Run locally

From `frontend/`, with the repository's existing dependencies installed:

```bash
node node_modules/typescript/bin/tsc -p design/diagnose/tsconfig.json
node node_modules/eslint/bin/eslint.js design/diagnose/*.ts design/diagnose/*.tsx
node node_modules/vite/bin/vite.js build --config design/diagnose/vite.config.ts
node design/platform-effectiveness/serve.mjs /tmp/diagnose-mock-dist 127.0.0.1 18497
```

Open `http://127.0.0.1:18497/`. The reused static server serves only the built
directory, not repository files. It has no authentication. Bind a different
explicit address only when needed for review. Development is available with
`node node_modules/vite/bin/vite.js --config design/diagnose/vite.config.ts`
on loopback port 18496.

Review instance: `http://100.77.224.24:18497/`, bound to this machine's Tailscale
address in the dedicated tmux session `diagnose-mock-18497`. Access depends on
tailnet connectivity and ACLs. Only this mock's session should be stopped when
review ends; other sessions and services are preserved.

## Visual reference and screens

Before designing, the existing `ApplicationsPage` and `AppShell` were rendered
in Chromium with two fictional applications and visually inspected. The
reference can be reopened at `/?reference=1`.

- [Existing Console reference](screenshots/console-reference.png)
- [Diagnostic results and Vault remediation](screenshots/desktop.png)
- [Target and use-case setup](screenshots/setup.png)
- [AWS IAM finding](screenshots/aws.png)
- [Completed simulated execution](screenshots/ready.png)
- [First-run state](screenshots/empty.png)
- [Narrow viewport](screenshots/narrow.png)

The sidebar, wordmark, typography and green primary action come from the real
components. The existing blue-primary prose mismatch is already tracked as
IMP-029; this proposal preserves the implementation. Findings use readable
labels and symbols in addition to color. Remediation stays next to the selected
check; identities and technical details are collapsed. A compact mock notice
and scenario control distinguish demonstration data from operational evidence.
The default view prioritizes actionable findings. Narrow viewport checks reuse
the existing navigation rather than redesigning it.

## Validation

Strict prototype TypeScript, scoped ESLint and standalone production build are
the code checks. Browser review covers Kubernetes/AWS target selection,
dependencies, filters/search, use-case drilldown, modal validation, Escape/focus,
sidebar collapse without losing selection, running/stopped states,
conditions-versus-execution outcomes, simulated cleanup, history, JSON download,
empty/error/retry states and page overflow at 1512, 1280 and 390 pixels.

No product API requests are expected during this review. Product frontend and
backend full suites and live infrastructure verification are outside this mock's
scope: product sources and runtime behavior are unchanged. Live diagnostics,
permission probes and cloud cleanup must be designed and verified separately
before claiming operational readiness.

Review on 2026-10-24: the listed TypeScript, ESLint, build and Chromium checks
passed. No browser page errors or product API requests were observed. The
documentation checker and whitespace check passed. Screenshots were inspected
against the Console reference. Vite reports a non-blocking bundle-size warning
because this isolated prototype includes the existing Console message catalog.
