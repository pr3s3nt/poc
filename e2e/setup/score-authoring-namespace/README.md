# Score authoring namespace access

Optional setup imported by the [Score acceptance runner](../../scenarios/score-authoring/README.md).
Uses the existing bootstrap namespace-access helper and bounded acceptance rules.
It does not create/adopt namespaces or modify shared controllers/cluster RBAC.

Set ORCH_SCORE_KUBECONFIG to a private existing kind admin kubeconfig and
ORCH_SCORE_CONTEXT to the explicitly authorized context. Optional
ORCH_SCORE_FLEET_ACCOUNT / ORCH_SCORE_FLEET_NAMESPACE select the installed Fleet
subject (defaults orchestrator-delivery / cattle-fleet-system).

After product namespace creation, the helper checks managed labels, Application
identity and namespace UID, creates or verifies the app-labelled Role/RoleBinding,
and records their UIDs in the private scenario receipt. This is setup, not proof
of successful workload deployment. Failures retain all receipts for inspection.

Cleanup after controller/workload withdrawal: verify namespace UID, exact app
labels, Role/RoleBinding UIDs and expected rules/subject; delete only these two
setup objects with UID/resourceVersion preconditions, then resume product cleanup.
Never remove unrelated objects or bypass namespace foreign-object guards.
