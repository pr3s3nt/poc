---
id: VERIFY-2026-10-12-T22
artifact: verification-record
status: current
last_reviewed: 2026-10-12
---

# T22 — Explicit Kubernetes credentials

Branch `refactor_ui`; LEG-05. Initial working tree clean. Codex updated UC-04
BR-15, realization/VOPC, ADR-013, credential/domain/schema/operation contracts
and UC-06/07/08 target design before Claude implementation. Shared classes/VOPC
PNG regenerated with PlantUML. Fleet's host transport removal is clarified in
ADR-007 and its runbook, without adding a new credential onboarding path.

## Outcome and review

Upload-only `{name,kubeconfig,context}` replaces the old registration shape.
Old/mixed bodies return 400 before verification, credential write or insertion.
Kubernetes authentication is explicitly KUBECONFIG; HOST_CONTEXT, missing type,
missing scoped credentials and foreign identity fail closed. Existing-cluster
execution has no host branch. OpenCLI and low-level Run prevent context-only,
empty/missing kubeconfig targets from invoking kubectl. Explicit Terraform EKS
kubeconfig paths retain their direct-delivery behavior. Cancellation/private
file cleanup, safe provider errors and restart/removal identity remain tested.

Product seed uses KUBECONFIG/VERIFYING, no credential reference and verified=false.
The Organization default marker references an existing record; new Applications
still create UNCONFIGURED Environments. Tests prepare synthetic READY metadata
separately, preserving existing Connection IDs and uploaded records. Real
resolver assertions use the same injected scoped credential store. Pure fake
execution may reference throwaway synthetic credentials; it proves no live API
access. Local runner memory scenarios use a test-only fakeserver; JSON scenarios
prepare disposable state with readystate. Product binary has no READY shortcut.

PostgreSQL migration 8 rejects HOST_CONTEXT with safe dev-reset guidance; no
conversion to fabricated READY KUBECONFIG. Remaining historical migration 5 is
not runtime authentication support and is removed with baseline work in T28.
SQL positive fixtures declare auth explicitly. SQL checks are not live-proven.
In-memory SaveConnection remains lenient for malformed fixtures; invocation
resolver/adapter/CLI guards enforce the credential boundary.

Fleet fixed-host delivery has no explicit credential transport and now rejects
all targets before Git/cluster mutation, including empty readiness references.
Direct delivery remains supported. Older kind runners depending on seeded READY
host state are classified in the kind runbook; approved upload/rebuild belongs
to T29, with Fleet credential delivery separately scoped. No matcher or reference
fixture changes. Historical verification records remain unchanged.

## Checks and evidence

Claude session `codex-T22-20261012`; handoff/evidence outside Git under
`/tmp/poc-T22-handoff-20261012/`.

- Claude full uncached Go tests, build and vet: pass with PostgreSQL/kind opt-ins unset; conformance and e2e included.
- Codex independent uncached connection/Kubernetes/PostgreSQL package/auth/seed/conformance checks: exit 0 (`codex-targeted.log`). PostgreSQL integration cases skipped by their gate.
- Codex independent uncached Fleet/deployment/provisioning/transition/e2e checks: exit 0 (`codex-execution.log`).
- Codex final full Go tests/build with opt-ins unset: exit 0; unchanged packages may use cache.
- Codex frontend typecheck/lint and 425 tests: exit 0 (`codex-frontend-tests.log`). Codex production build: exit 0 (`codex-frontend-build.log`); bundle also rebuilt by Claude for local recording.
- Changed shell scripts `bash -n`, changed scenario modules `node --check`: exit 0.
- Documentation checker and whitespace validation: exit 0 (279 Markdown files).

Headed T01 fake navigation smoke passed (`ui-T01`); raw/captioned MP4,
assertions/marks/captions retained. Codex reviewed captioned frame and assertions;
runner full decode checks pass. Application-connection local create/restart
passed (`ui-appconn`, `run-appconn.log`), prepared synthetic second Connection
is setup rather than UI registration proof.

T07 initial run failed on stale English BUSY expectation from before T20; UI
returned the correct Vietnamese message. Assertion corrected to the current UI.
Second run failed on the obsolete raw recovery-detail expectation; safe summary
was correct. Failures retained in `ui-T07-first-failed` and `ui-T07-second-failed`;
a timeout-interrupted attempt is retained in `ui-T07-killed-by-my-timeout`.
The next run passed with decode/caption checks (`ui-T07-before-message-fix`).
Final T07 passed (`ui-T07`, `run-T07-final.log`, run ID
`refactor-20261011195753-20527`) with explicit raw-detail-absence assertion and
accurate evidence text. Codex reviewed binding/recovery frames and captions,
assertions and phase marks. Independent full captioned MP4 decode: exit 0
(`codex-T07-decode.log`); runner raw/captioned decode and 28-frame checks passed.
All failure/earlier-pass artifacts remain retained.

No new T22 product browser scenario: upload UI unchanged from T06; recordings
above are regression of runner setup. SQL integration, kind/AWS/live Fleet and
retained dev mutation skipped because no disposable SQL/external verification
was authorized. No deploy, upload or retained resource cleanup performed.
