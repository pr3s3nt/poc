---
id: VERIFY-T31-TYPED-DEFINITIONS-20261012
artifact: verification-record
status: evidence
last_reviewed: 2026-10-12
---

# T31 — Typed Definition form and authoritative capabilities

Task T31 / RD-04, RD-05, RT-04 on `refactor_ui`. Initial tree was clean;
no user-owned changes were overwritten. Codex updated UC-03 BR-23–26,
realization/sequence/VOPC/UI, OC-04d and UC-02 capability UI mapping before
Claude implemented code/tests in task session `codex-T31-20261012`.

## Outcome and review

GET `/api/v1/resource-definition-capabilities` is session-org/PE/Admin gated.
Registration and metadata share supported pairs. Terraform variables/types/defaults
come from the embedded inspector; Kubernetes metadata describes existing executor
policy. Resource Type param sources are scoped. Namespace name counts as provided
only by a required string input (BR-12). Editable metadata excludes executor-owned
variables; safe errors expose no inspector path, provider diagnostics or secrets.
The endpoint reads metadata and inspects embedded contracts only; no writes,
provider calls, system admission or execution.

Resource Types support/CTA and Definition choices use this endpoint without
frontend whitelist fallback. Typed controls retain JSON types and deferred
placeholders; defaults/params remain omitted unless the author opts in. Presets
are explicit, and workload params retain precedence over Definition fallbacks.
Per-pair drafts survive switching. Advanced variables/provision remain authoritative
when they cannot be represented safely, preserving references, escaped/padded
strings, explicit false flags and empty params during conversion. Provision rows
explain consumer/provider, is_dependent and match_dependents in Vietnamese.

Review corrected whitespace trimming, any-string conversion, false/empty provision
field loss, param precedence guidance and required-string namespace metadata.
Unknown/null/credential/master_password rejection stays authoritative at registration.
Terraform region/tags remain hidden in typed controls; this task does not add a new
registration rejection for previously valid region/tags advanced inputs.
Matcher weights/ties, resource-param precedence, renderer selection and executor
semantics are unchanged; reference fixtures and match.go have no diff.

## Checks

Logs: `/tmp/poc-T31-handoff-20261012/`.

| Check | Result |
|---|---|
| Full Go tests/build, external opt-ins unset | Independent exit 0; `codex-go-final.log`, `codex-build-final.log`. Includes HTTP/e2e and conformance. |
| Uncached conformance | Exit 0; `codex-conformance.log`. |
| Catalog/HTTP race | Final exit 0; `codex-race-final.log`. |
| Frontend typecheck/lint/test/build | Exit 0; `fe-gates-final2.log`, 25 files / 464 tests. Bool/map coverage uses explicitly synthetic metadata because current editable modules have neither type. |
| Shell/JS syntax, docs/PlantUML/whitespace | Wrapper bash syntax and JS syntax checked; UC-03 PNGs regenerated. Checker and whitespace pass. |

An intermediate independent frontend run failed lint because a new graph scenario
block was inserted into an unrelated scenario. Corrected placement was reviewed;
final lint passes. An earlier successful frontend run had 460 tests; the four
synthetic metadata regression tests are included in the final 464-test run.

## Headed browser evidence

Backend is real product HTTP/planner with fake adapters and disposable state,
not real Kubernetes/AWS/PostgreSQL/Vault provisioning. T31 setup creates/binds an
application through UI. Only the metadata transport-500 projection is mocked;
registration, matching snapshot and full Score Preview responses are product code.
No cloud execution or Deploy occurs in T31.

| Run | Path / result |
|---|---|
| T31 r7, refactor-20261012070753-27645 | `/tmp/poc-refactor-T31-r7-140752`: PASS. |
| T04 regression | `/tmp/poc-refactor-T04-T31reg-141753`: PASS. |
| T19 regression | `/tmp/poc-refactor-T19-T31reg-142307`: PASS. |
| T02 rerun | `/tmp/poc-refactor-T02-T31reg2-142649`: PASS. |
| T16 rerun | `/tmp/poc-refactor-T16-T31reg2-142914`: PASS. |

T31 has raw/captioned MP4, captions SRT/ASS, marks, assertions and result.
Independent full captioned decode exits 0 (`codex-video-decode.log`), resolution
1440×996, duration 402.2 seconds. Reviewed typed storage/default/param omission,
provision flags and batch-order frames are in `review-frames/`; captions are below
the browser, cursor/input/click visible, no real credentials shown. All successful
assertions and phase marks retained. `T31-captioned-retimed.mp4` with
`captions-retimed.srt/.ass` is a timing-only additional edition: step 2 starts
at 64.9s instead of 66.9s; step 12 starts at 295.3s instead of 299.3s, with the
prior cue ending at that boundary. Footage is unchanged, original artifacts
retained; no replay. Independent final decode is in `codex-retimed-decode.log`.

Full read-only Score Preview asserts PostgreSQL consumes the default namespace,
its is_dependent companion consumes PostgreSQL, and provider-first batches are
cluster → default namespace → PostgreSQL → companion namespace. The full Preview
selects the authored unique-class Definition and does not register another one.
Matching-preview alone is not claimed as graph proof.

T31 r1–r6 are preserved at the paths in `ui-path.txt`; failures were scenario
selectors and hidden/disabled controls during type/driver transitions. The
misplaced graph block was independently caught by lint during implementation;
correction did not modify unrelated scenario behavior. T02/T16 first
regression attempts are preserved; they expected pre-T20 raw error text and were
updated to current Vietnamese guidance or stable code/field/no-write assertions.
Tests were not weakened and unsafe backend messages were not restored.

## Limits and external state

PostgreSQL opt-in registration/integration tests were not run: no disposable DB
approval in T31. No retained IDP, Vault, K8S-4F, cluster/cloud resources or workloads
were mutated. Fake footage does not imply real provider verification. Evidence is
outside Git, not uploaded. Only task-owned runner processes and tmux session are
cleaned; existing user sessions are retained.
