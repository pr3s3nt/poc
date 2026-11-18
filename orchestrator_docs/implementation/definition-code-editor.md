---
id: IMP-DEFINITION-CODE-EDITOR
artifact: implementation-handoff
status: current
last_reviewed: 2026-11-18
---

# Synchronized Definition code editor — issue #87

Approved 2026-11-18: [issue #87](https://github.com/pr3s3nt/poc/issues/87), branch
features/definition-code-editor from origin/main `725541ef`, separate checkout.
#84 branch and its trial are preserved. Required behavior:
[CODE-01–06](../usecase/UC-03/specification.md#sửa-definition-bằng-code--issue-87-approved-2026-11-18),
[screens](../usecase/UC-03/ui/screens.md#sửa-yamljson-đồng-bộ--issue-87),
[states](../usecase/UC-03/ui/states.md#definition-code-edit--issue-87),
[API](../usecase/UC-03/ui/api-mapping.md#definition-code-edit--issue-87).

DefinitionEditMethods reuses tabs/textarea/buttons/source summary. Creation import
stays separate; editing shares one authoring draft. definitionEditDraft applies
form deltas losslessly and protects data keys from prototype mutation. Code is
parsed strictly, without filtering unknown/system fields. useDefinitionCodeEditor
uses existing prepare, checks context identity, hydrates the form and compares
canonical round-trip; lossy projection preserves code with an error. Both Saves
use the same PUT/version-CAS and stale/unknown hold/reload. Checked pins are
cached only for unchanged content/scope; Save never resolves a branch again.
Explicit empty workload resource references survive conversion. No schema,
backend behavior/API, shared Kubernetes or cloud changes.

Focused tests cover four drivers, two-way sync, pins/accounts, advanced fields,
invalid code/identity/system fields, stale/network/uncertain Save and unmount.
Existing form/import/revision tests remain. Real isolated setup verified an
Internal kind account read-only and prepared four sample Definitions. Browser
passed both directions and both Save methods for all four: exact version bumps,
unchanged active revision/source/account pins, no Application/deployment created.
Samples and stack volumes are retained for review.

This main revision has no #84 Gateway driver; the generic authoring DTO path is
independent of driver-specific forms and can be retained on #84 integration.
Final full frontend on kind passes 116 files / 1093 tests and typecheck/lint/build;
both full namespaces are cleaned. Unchanged backend evidence is explicitly reused,
retaining its baseline failures/skips/PG timeout. Final visual/browser checks pass.
See [verification](../verification/2026-11-18-definition-code-editor.md). Trial stack
and four examples are retained for acceptance. No main merge/rollout approved.

## Combined acceptance — approved 2026-11-18

The user requested one integrated trial with #84. features/definition-code-editor
is being merged into features/Httproute; it is not merged into main. The Gateway
Echo/legacy and Gateway-bound endpoint forms retain class controls and typed
metadata while sharing the CODE-01–06 bridge. New integration tests cover both
Gateway layouts and reference-only Endpoint UI↔code conversion. Earlier isolated
#87 results do not substitute for this combined check. Main rollout remains
subject to user acceptance.
