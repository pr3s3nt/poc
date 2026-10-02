---
id: VERIFY-20261002-CATALOG-WORKLOAD-VALIDATION
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-02
---

# Catalog and workload validation — local verification

## Delivered behavior

- New public Resource Type/Definition IDs use lowercase ASCII letters, digits
  and hyphens without normalization. Virtual Type keys `environment`/`service`
  cannot be registered. Existing catalog reads, domain fixture validation and
  planner identifier behavior are unchanged.
- UC-03 rejects unknown Driver Inputs fields, wrong/null shapes, unsupported
  variable names, wrong literal and collection-element types and credentials.
  Valid full placeholders, string interpolation and escaped literals remain
  supported. Namespace name must be provided in variables or via a required
  string Type input; optional/wrong-type Type inputs do not satisfy this rule.
- UC-16 Save/import validates every non-virtual resource dependency, including
  dependencies unused by container bindings. Invalid params return safe paths
  and leave draft version/drafts/current-set pointer unchanged. Form errors
  retain input; successful import/save is not reported on rejection.

## Implementation and independent review

The user requested Codex to own documentation and delegate code/test edits to
Claude through `clauded` in tmux. Session: `codex-shortterm-20261002`.
Codex updated specification, realization/sequence, shared contracts/design and
ADR before assignment. Claude was limited to backend/frontend code and tests;
required documentation updates were reported to Codex. Progress was polled at
approximately four-minute intervals, not continuously.

Review round 1 found two defects: nested malformed placeholders were accepted,
and the namespace name exception allowed optional/wrong-type Type inputs.
Claude added regressions and recorded pre-fix failures: HTTP nested-placeholder
registration returned 201, five malformed nested cases were accepted, and six
invalid Type-name contracts passed. Round 2 fixed both; Codex inspected the
final implementation and regression tests. No additional documentation changes
from Claude were detected by the documentation manifest audit.

## Checks

Codex independently ran:

- `cd backend && go test ./... && go build ./...` on round 1.
- `cd frontend && npm run typecheck && npm run lint && npm test && npm run build`:
  13 test files, 72 tests passed. Frontend was unchanged in round 2.
- Final backend: `env -u ORCHESTRATOR_POSTGRES_TEST_URL go test -count=1 ./...`
  and `go build ./...`: passed, including HTTP E2E, existing pending
  Preview/update/remove tests and planner conformance. This process environment
  intentionally disables external PostgreSQL integration.
- Documentation checker with installed PlantUML and `git diff --check`: passed.
  Modified sequence PNGs were regenerated from their PlantUML sources.

Relevant new regression coverage: `TestRegisterResourceType_PublicIDPolicy`,
`TestRegisterResourceDefinition_PublicIDPolicy`,
`TestRegisterResourceDefinition_SeededShapesStillRegister`,
`TestRegisterResourceDefinition_StrictDriverInputs`,
`TestRegisterResourceDefinition_PlaceholderAwareTyping`,
`TestRegisterResourceDefinition_NamespaceNameFromTypeContract`,
`TestSaveAndImportValidateUnboundResourceParams`,
`TestResourceParamValidationStoreFailureIsInternal`,
`TestUC02UC03RegistrationPolicyHTTP` and
`TestUC16ResourceParamsRejectedOnSaveAndImportHTTP`.

## Limits and remaining work

No browser recording, live PostgreSQL, kind, Vault or AWS verification was run.
No cluster/cloud mutation, credential read, commit or push was performed.
The repository was clean at task start; there were no pre-existing user changes.
The tmux session and task/review logs are local temporary artifacts, not
repository delivery artifacts.

[ADR-009](../architecture/decisions/ADR-009-aws-access-key-storage.md) records
accepted AWS access key + Vault storage. Full AWS registration, verifier,
credential lifecycle and executor resolution remain unimplemented; this local
validation evidence does not close UC-04 or IMP-002/I06-10/M02. Terraform
region/tags retain their existing contract; no new ownership restriction was
introduced. Virtual resource validation retains its existing behavior; this
change's all-dependency validation applies to non-virtual types.
