# Synchronized Definition editing — issue #87

Scope: [UC-03 CODE-01–06](../../../orchestrator_docs/usecase/UC-03/specification.md#sửa-definition-bằng-code--issue-87-approved-2026-11-18).
Use an isolated task Docker installation, Node >=22 and the shared Console helper.
Install frontend/bootstrap/e2e dependencies. Credentials, screenshots and receipts
stay outside Git. Do not run against the retained Console or existing catalog.

Setup: `e2e/setup/definition-code-editor/configure.mjs` requires shared Console
environment, private `ORCH_EVIDENCE_DIR`, scoped `ORCH_KUBECONFIG`, explicit
`ORCH_CONTEXT=kind-k8s-4f` and immutable `ORCH_TEMPLATE_REF`. The kube endpoint
must be reachable from the task backend without weakening TLS. It verifies one
Internal account using read-only Kubernetes access, inspects Git module/template
pins, then registers four isolated sample Definitions. Setup is not deployment
acceptance; no Application or workload is created, no cloud access is used.

Run `node e2e/scenarios/definition-code-editor/run.mjs` with the same Console and
evidence environment. It exercises existing-cluster, endpoint, Terraform namespace
and workload-template forms, UI → YAML → UI → JSON, two real version-CAS Saves,
immutable source/account/driver/input pins, unchanged active revision and complete
revision count. It verifies no Application/deployment is created, then opens the
actual catalog over the supplied Tailscale URL. Error/stale/advanced-field cases
are focused component tests; runtime follows the normal path.

Side effects: four uniquely named sample Definitions and their proposed revisions,
one verified Internal account and source caches in the isolated installation.
Keep these examples and stack volumes for user review; cleanup the task stack
and its volumes after acceptance. The runner never deletes or modifies Kubernetes
objects or shared Gateway/controller/DNS/TLS. Never purge retained revision history.

Full frontend check: `node e2e/lib/full-frontend-kind.mjs` requires external
`ORCH_EVIDENCE_DIR`, explicit scoped `ORCH_KUBECONFIG` and `ORCH_CONTEXT`. It tests
an exact Git HEAD archive in one owner-labelled disposable namespace, Node 22
Bookworm, one worker, install/test/typecheck/lint/build; private receipts/logs
record every exit status. Namespace deletion uses UID/resourceVersion conditions
and confirms absence. It is frontend-only: unchanged backend evidence must be
attributed separately by exact Git tree; a failed baseline stays failed.
