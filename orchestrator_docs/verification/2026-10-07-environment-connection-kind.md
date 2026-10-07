---
id: VERIFY-20261007-ENVIRONMENT-CONNECTION-KIND
artifact: verification-record
status: evidence
last_reviewed: 2026-10-07
related: [UC-01, UC-03, UC-04, UC-05, UC-06, UC-08, UC-12, ADR-011]
---

# Environment Settings set-once connection — live kind verification

## Outcome and scope

The [UC-01 design](../usecase/UC-01/specification.md) and
[ADR-011](../architecture/decisions/ADR-011-environment-execution-binding.md)
replace the Application create selector with independent Environment Settings
bindings. Claude implemented code/tests/scripts under Codex coordination;
Codex authored canonical docs, reviewed code and video, ran independent gates,
and publishes the reviewed recording. No AWS cloud mutation or new cluster.

Live run `envconn-kind-20261007121743-27878` used existing `kind-idp-internal`
with the real Kubernetes executor/deployer and Vault/VSO configuration delivery.
Application `2f5795ee-ec0f-45a2-bf67-244f02a4929e` deployed staging namespace
`app-2f5795ee-ec0f-45a2-bf67-244f02a4929e-staging`.

| Assertion | Observed result |
|---|---|
| Connection onboarding | Platform Engineer uploads kubeconfig twice, verifies and registers READY `staging-kind-327878` and `production-kind-327878`, plus matching staging cluster Definition through UI |
| Create | Developer enters Name/Subdomain only; both Environments initially UNCONFIGURED, Application stores no execution target |
| Unconfigured gate | Preview refused safely; no default fallback or provisioning |
| Staging Settings | Explicitly set once, read-only Locked; refresh retains selected Connection |
| Production independence | Remains unset after staging set, then explicitly set to another key, Locked |
| Restart | Same JSON state/backend port; both targets still served/locked after browser refresh |
| Set-once API | Separate negative assertions repeat PUT for both targets; `409 ALREADY_CONFIGURED`, including same key; UI has no change/reset controls |
| Runtime | Real staging backend/frontend Deployments available, Services present, Pods Running/Completed; database/environment/secret/backend diagnostic PASS |
| Persisted identity | Every staging Active Resource and workload TargetRef uses staging Connection; production Connection executes nothing |
| Credential control | Private backend host credential rejected by cluster; uploaded credential accepted, so successful execution cannot use host fallback |
| Safe evidence | Credential-content scan passes; uploaded document is not displayed on-screen or in public responses |

## Human UI recording

[Download/watch MP4 on GitHub](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/environment-connection-kind-envconn-kind-20261007121743-27878.mp4)
([recordings release](https://github.com/pr3s3nt/poc/releases/tag/acceptance-recordings)).
H.264, 1440×900 full browser window, visible cursor, 15 fps, 515.8 seconds
(8m36s), 4,114,153 bytes, 31 phase marks/settled nonblank frames, full decode PASS.
GitHub asset downloaded and compared byte-for-byte with the reviewed original.
SHA-256: `19b72afb2a0f559e7a4ed0abc3ad62f12fb736a9b74480c0b7b34935b8de7992`.
Codex sampled create/unconfigured, staging Locked, production Locked and deployed
PASS screens. Human helper types with pauses and uses native dropdown interaction;
product mutations shown in the recording are UI actions. API and kubectl observers
verify persisted/live facts; repeat-PUT negatives occur outside the shown flow.

Useful timestamps: 02:21 unconfigured App; 02:35 refused Preview; 02:41 staging
choices; 02:57 staging Locked; 03:05 production still unset; 03:19 production
Locked; 03:29 restart lock; 07:59 Deploy succeeded; 08:10 diagnostic app.

Local full evidence stays outside Git at
`/tmp/poc-environment-connection-review/envconn-kind-20261007121743-27878/`:
MP4, marks, frames, ffprobe, public views, persisted-binding projection, checks,
cleanup proof and logs. Raw credentials/private state are deleted, not uploaded.

## Validation and review

- Claude: Go vet/race test/build, with isolated Docker PostgreSQL; frontend
  typecheck/lint/129 tests/build; local connection/onboarding Playwright PASS.
- Codex independently: Go test/build with the same isolated test PostgreSQL;
  frontend typecheck/lint/129 tests/build; documentation checker/PlantUML and
  diff whitespace; changed shell and mjs syntax PASS.
- Meaningful coverage includes pre-change SQL migration/reopen/idempotence,
  immutable SQL trigger/JSON save guards/concurrent set winner, JSON legacy load,
  independent provider/profile/region, unconfigured zero-side-effect failure,
  new AWS Environment graph/@infra/names versus legacy scope and plan hash,
  scoped HTTP safe errors, UI retry/late responses and route environment sync.
- Initial review corrected authored Definition refresh, binding consistency,
  stored defaults, Application target reads, route scope and response generations.
- Final review found additional seed-only overwrite cases, AWS UUID/run-name
  bounds and run-owned recording-process cleanup. Corrected canonical naming
  before implementation follow-up; final revision results are recorded below.

## Cleanup and limits

Runner and Codex independently query the Kubernetes API successfully and find
staging namespace absent and production namespace absent (never created). Run
Vault container is absent; private credential directory/state removed; current
kubectl context remains `kind-idp-internal`. Existing operator workloads/Vault
service are preserved. Run-specific platform Vault revisions/policies/auth roles
and loaded acceptance images retain the existing local lifecycle limits.

Both logical Connections use the same physical cluster and credential; this
proves independent stored bindings and selected credential execution, not
separate physical clusters. Only staging is deployed live. AWS scopes are tested
locally without cloud calls; AWS credential onboarding remains deferred under
ADR-009. Job submission was accepted as PENDING; worker processing is not tested.
The video precedes AWS-only naming and seed/process cleanup review revisions;
those do not change the recorded Kubernetes binding/UI/execution behavior.

## Initial final-review failure (resolved below)

Claude completed the additional AWS bounded-name and exact-template seed refresh
changes before its session limit. A follow-up Claude invocation was rejected by
quota. Codex's subsequent full Go race suite found three failures in new seed
assertions: `toString` iterates maps without stable ordering, so equivalent maps
produce unequal strings. Production inputs match, but final gate is FAILED until
those assertions use deterministic structural/canonical comparison and rerun.
Recording process-group cleanup is also pending. No commit/push at this checkpoint;
initial local PASS and live PASS above do not imply the final working tree is accepted.
Codex removed its final-review PostgreSQL container after the failed gate.

## Final accepted revision

The user authorized Codex to finish the remaining fixes after Claude's quota
limit. Seed test comparisons now use deterministic JSON serialization, including
full Definition values (the old helper returned an empty string for structs).
The runner starts Node/Chromium/recorders in its own session, terminates only
that process group on success/error/interruption, verifies no live group members
remain, avoids an orphan tee process, bounds the browser flow to 1200 seconds
and backend shutdown to a five-second grace period before forced termination.

Final gates passed:

- `go test ./internal/seed -count=20`.
- `go test -race ./...`, `go vet ./...`, `go build ./...` with an isolated
  PostgreSQL 16 container; container removed by the cleanup trap.
- Controlled local test of the actual runner cleanup function: owned parent and
  descendant processes terminated while an unrelated sentinel remained alive.
- Changed shell/mjs syntax, documentation checker and `git diff --check`.
- Frontend typecheck/lint/129 tests/build passed independently during review;
  no frontend changes followed that gate.

The successful live recording was not repeated for these test/process cleanup
changes or the AWS-only naming revisions. AWS cloud execution remains untested
by this task. The earlier failure above is retained as review provenance; it no
longer blocks acceptance of this revision.
