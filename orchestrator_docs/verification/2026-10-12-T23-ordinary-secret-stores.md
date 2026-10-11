---
id: VERIFY-2026-10-12-T23
artifact: verification-record
status: current
last_reviewed: 2026-10-12
---

# T23 — Ordinary workload Secret Stores

Branch `refactor_ui`; LEG-06. Initial working tree clean. Codex updated ADR-012,
ADR-006 supersession, UC-04 SS-07..08/realization/sequence/UI, UC-12 BR-15 and
realization, shared domain/operation/schema contracts and Docker/self-host runbooks
before Claude implementation. UC-04 sequence PNG regenerated with PlantUML.

## Outcome and review

Workload Vault resolution uses only the persisted Organization/store-scoped
private credential reference. Missing credential store/ref/token, empty token and
wrong Organization/store scope fail closed. Removed global workload Vault flags,
LegacyConfig, startup legacy seed, store backfill/conversion APIs, domain legacy
flag and public DTO/verification legacy fields. Console list and choices no
longer display legacy labels. Owning immutable Secret references remain intact.

Ordinary Compose bootstrap retains validation/real Vault verifier, attempt-owned
probe cleanup before READY, private credential persistence and restart idempotence.
Managed refresh checks the compose marker, expected ID/credential ref and endpoint
identity in service and persistence. Unmarked user stores cannot be refreshed,
even with a matching token; concurrent losers clean only their own credentials.
SQL migration's old legacy column remains inert until the T28 fresh baseline;
it is not read or written by store runtime/admission. Variable legacy
materialization remains T24; Application/scope cleanup remains T26/T27. Agent/VSO
delivery remains supported. No matcher or reference fixture changes.

Removed flags' current runner consumers use ordinary bootstrap through the shared
`platform-store-lib.sh` and require a separate scoped credential-store token path.
Container entrypoint supports private platform/connection token files separately.
These live runner changes have syntax verification only. Historical self-host
recording predates T22/T23; no current live proof or retained-state restart implied.
Local wrappers explicitly disable platform bootstrap to preserve fake isolation.

## Checks and evidence

Claude session `codex-T23-20261012`; reports and independent check logs outside
Git under `/tmp/poc-T23-handoff-20261012/`.

- Codex full Go tests/build: exit 0 with PostgreSQL/kind opt-ins unset;
  `codex-go-tests.log`, `codex-go-build.log`. Full suite includes conformance.
- Codex uncached race checks on secretstores/store/vault/bootstrap/conformance:
  exit 0, `codex-race.log`. SQL integration skipped by its opt-in gate.
- Codex frontend typecheck/lint/426 tests/production build: exit 0,
  `codex-fe-{typecheck,lint,tests,build}.log`.
- Changed shell scripts `bash -n` and changed JS modules `node --check`: pass.
- Documentation checker, PlantUML regeneration and whitespace validation: pass.

Headed T23 `/tmp/poc-T23-ui-T23-run2` (run ID
`refactor-20261011202632-6662`) passed UI assertions, raw/captioned decode and
nine phase marks. Flow: create unconfigured app; no store guidance; PE registers
ordinary store; Developer reviews suggestion and explicitly saves selection;
writes a concealed Secret and observes configured-only metadata. Product steps
use UI; API reads assert selection and safe response contents. Fake adapters,
memory credentials/state and fake verifier prove local collaboration, not real
Vault/cluster deployment. GET projection injects a stale legacy flag after
capturing real safe responses to assert the UI ignores the obsolete label.

Codex reviewed registration/selection/secret-saved frames, assertions and captions.
Captioned 1440x1020 video has a separate bottom caption strip; browser remains
visible. Independent full captioned MP4 decode passed (`codex-T23-decode.log`).
Video: `/tmp/poc-T23-ui-T23-run2/T23-captioned.mp4`; captions, marks and assertions
are alongside it. Initial `/tmp/poc-T23-ui-1` passed product assertions but failed
the minimum phase-mark video gate (five marks); all artifacts retained. Scenario
then added marks at actual UI stages and passed without weakening the video gate.

Regression T05 `/tmp/poc-T23-ui-T05-run2` (run ID
`refactor-20261011202843-19610`) passed, with 13 phase marks. Registration,
verification failure/token clear and list backend/auth projections remain correct.
Regression T07 `/tmp/poc-T23-ui-T07-run2` (run ID
`refactor-20261011203134-9699`) passed, with 28 phase marks: explicit selection,
Secret concealment, ordinary store switch/copy, failure/stale/BUSY guards, editor
retention and safe transition/recovery projections. T05/T07 projection mocks
remain labeled in assertions/captions. Codex reviewed ordinary list/store-change
frames and independently decoded both captioned videos (`codex-T05-decode.log`,
`codex-T07-decode.log`). Runner raw/captioned decode checks passed as well.

Claude full Go tests/build and uncached verbose conformance passed
(`/tmp/poc-T23-go-test.log`, `/tmp/poc-T23-go-build.log`,
`/tmp/poc-T23-conformance.log`); frontend checks also passed. Report and DONE
marker retained under the handoff directory. Independent checks above verify
these outcomes rather than relying solely on the report.

SQL integration, live Compose/Vault, kind/AWS and retained dev mutation skipped:
no external verification/disposable SQL authorization. No credential files or
local `.env` read; no deploy, recording upload or retained resource cleanup.
