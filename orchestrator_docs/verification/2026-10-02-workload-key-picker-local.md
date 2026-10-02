---
id: VERIFY-20261002-WORKLOAD-KEY-PICKER
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-02
---

# Workload Application-key picker — local verification

## Delivered behavior

UC-16 BR-15–BR-18: each container selects existing Application variables and
secrets from separate checklists. Nothing is implicitly selected; selected
names default to the key name. An optional alias changes only the destination
container name. Other sources retain resource-output and Service flows.
Workload Score stores references, never copies configuration values.

Edit/import preserves custom names and multiple mappings of the same key.
Unavailable selected keys remain visible and prevent Save. Empty alias and
cross-source duplicate destination names fail before serialization. Catalog
loading/error states are explicit and retryable. Controls introduced for
selection/alias/other sources are disabled during Save.

A same-scope stale reload preserves edits. Changing Application, Environment
or workload resets selection, import, blocked, saving and reloading state;
late old catalog/file-read/import/Save/reload responses are ignored. Dynamic
Score dictionary names, including prototype-member names, survive form
serialization as own properties. Unset resource inputs read as empty values.
API/schema/provider and Preview → Deploy contracts are unchanged.

## Documentation ownership and review

Codex updated canonical specification, realization, sequence/VOPC, shared
contract and UI artifacts before assigning code work. Claude ran through
`clauded` in tmux `codex-workload-keys-20261002`, restricted to frontend
code/tests. Progress was checked approximately every four minutes.
Documentation hash audits found no Claude edits or newly created documents.

Four review rounds completed:

1. Initial checklists and behavior tests. Codex found scope carryover and
   prototype-name serialization loss; scope clarification was documented
   before the next assignment.
2. Scope reset and late import/Save guards; regressions demonstrated pre-fix
   failures. Codex found pending reload state still leaked across scopes.
3. Reload reset, safe dictionary construction and deterministic deferred-reply
   tests. Pre-fix reload cases had a disabled destination Reload button;
   prototype dictionary cases lost containers/resources/ports. Guard mutation
   checks detected regressions. Claude reported remaining unset-input reads.
4. Own-property reads in validation/rendering. Before the fix, string inputs
   displayed `[object Object]` and required inputs threw on `.trim()`.
   Optional/required string and boolean variants now pass.

## Independent checks

- Frontend typecheck, lint, test and production build: passed; 14 test files,
  107 tests. Regression coverage lives in
  [WorkloadEditorKeys.test.tsx](../../frontend/src/features/workloads/WorkloadEditorKeys.test.tsx)
  and [WorkloadEditorPage.test.tsx](../../frontend/src/features/workloads/WorkloadEditorPage.test.tsx).
- Playwright against production Console with an actual local HTTP backend:
  sign in, create Application, create variable/secret through Settings, select
  API_URL and DATABASE_PASSWORD, leave LOG_LEVEL unchecked, alias the secret
  to PGPASSWORD, Save, then reopen Edit. Exact reference-only Score payload and
  restored selections/alias were asserted; fixture values were absent from
  workload UI/payload. Server used fake adapters, in-memory configuration and
  explicit empty database/Vault flags. No Deploy was invoked.
- Claude's additional mocked browser smoke exercised imported multiple aliases,
  missing-key blocking and keyboard checkbox operation. Codex inspected both
  mocked and real-HTTP screenshots.
- Documentation checker with PlantUML and whitespace diff checks: passed.
  Sequence/VOPC PNGs were regenerated from updated sources.

## Limits and handoff

Backend source was unchanged, so Go test/build gates were not repeated for
this frontend task. No live PostgreSQL, kind, Vault or AWS integration was run;
no cluster/cloud mutation or credential reads were required. The temporary
in-memory server was stopped after verification. No persistent test data was
written. Temporary prompts, logs, browser scripts and screenshots stay outside
the repository; tmux is retained idle for inspection.

The repository was clean at task start. All delivery changes belong to this
task; the user explicitly requested commit and push. AWS onboarding and other
roadmap work remain outside this change.
