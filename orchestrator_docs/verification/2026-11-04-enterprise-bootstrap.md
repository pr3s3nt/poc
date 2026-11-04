---
id: VERIFY-ENTERPRISE-BOOTSTRAP-20261104
artifact: verification-record
status: current
last_reviewed: 2026-11-04
---

# GitHub Enterprise bootstrap input/setup kit

Approved scope: input-driven company staging setup and internal-AI instructions,
continuing issue #33. See [implementation](../implementation/direct-bootstrap.md)
and [setup kit](../../scripts/bootstrap/enterprise/README.md).

Implemented explicit scalar `${bootstrap.NAME}` parameters in parsed config,
Git-owned Definition JSON and Workload YAML. Raw Git bytes remain SHA-256 verified
before expansion; product placeholders keep their meaning. Added portable Git
business documents, a data-only staging preset and an offline lock helper deriving
exact file/tree pins from committed local Git objects with matching HTTPS origin.
The locked input remains next to its private source input and is not overwritten.

Fixed the platform-shared Git binding to inherit its Enterprise clone/API roots
and CA, retaining GitHub.com behavior. Repository HTTPS API adds the declared CA,
keeps certificate verification and forbids redirects with credentials.

59 bootstrap tests pass, including a complete offline load of 11 Git-owned files,
six expanded Definitions, pin comparison, exact-commit locking, wrong-origin/ref
rejection and preservation of product placeholders. Relevant Go delivery/GitHub
adapter tests and `go build ./...` pass. New CLI syntax and docs/whitespace checks
pass. Real HTTPS test server checks Enterprise `/api/v3` and custom CA trust;
no company GitHub Enterprise/cluster was contacted or mutated.

Full project checks were also attempted but are not claimed green: frontend has
959 passing tests and two historical Score scenario failures reproduced on the
unchanged base. Full Go suite reports failures outside the touched delivery and
GitHub adapter packages (render/template compatibility, conformance and historical
scenario/process tests). These are retained as unresolved broad-suite limitations;
no assertions were removed, skipped or weakened to report a green suite.
Further broad baseline diagnosis was stopped to keep scope focused.

The source-summary UI is already live on Console 3043 and separately recorded in
[UI verification](2026-11-04-definition-source-provenance.md). Enterprise backend
changes are in source for the company's reviewed release build; the retained demo
backend was not replaced, avoiding unrelated template-engine pin churn.

Setup prerequisites and field explanations are in `enterprise/README.md`,
`INPUT_REFERENCE.md`, `AI_HANDOFF.md` and `scripts/bootstrap/AGENTS.md`. Input has
placeholders for actual company endpoints, full commits, accounts, credential file
paths, CA, cluster, Vault, registry and access URL. Production/staging permissions,
installation, internal dependency mirrors, custom contracts and actual corporate
runtime acceptance remain the internal operator's explicit responsibilities.
