# External Kubernetes monitoring — issue #35 Slice 1

Validates [UC-09 OBS](../../../orchestrator_docs/usecase/UC-09/specification.md#external-kubernetes-monitoring-and-alerts--obs),
[ADR-040](../../../orchestrator_docs/architecture/decisions/ADR-040-external-observability.md)
and the [external contract](../../../orchestrator_docs/architecture/contracts/external-observability.md).

Requires explicit dev cluster/Git/notification mutation authorization, a dedicated
schema-11 Console and bootstrap-owned acceptance app, actual pinned dev stack and
test receiver from [setup](../../setup/observability-stack/README.md), Node 22+,
Playwright dependencies in e2e and scripts/bootstrap, kubectl and accessible
backend endpoints. Config/session/credentials/evidence remain outside Git.
No mock endpoints, metric samples or notifications are used as runtime evidence.

Run from repository root:

```sh
node e2e/scenarios/kubernetes-monitoring/run.mjs \
 --config /private/bootstrap.json --evidence /private/monitoring-evidence \
 --prometheus http://private-address:19091 --alertmanager http://private-address:19093 \
 --grafana http://verified-tailnet-host:3065 --cluster-uid <verified-uid> --phase connection
```

`connection` creates the PE connection via actual form, explicitly uses labelled
dev rules, tests connectivity/coverage/physical cluster and saves its identity.
`binding` requires completed bootstrap, selects the verified connection in
Environment Settings, and verifies actual binding scope/revision. These are
setup checkpoints, not proof of alert delivery or completed Slice 1.

Side effects: connection metadata and one binding in the receipt-owned app only.
State checkpoints prevent duplicate connections on rerun. Stop on a failed
mutation and inspect product state before retry; do not reset receipt identity.
A successful save followed by reload failure can have committed.

Remaining live assertions are real app metrics/alerts, app-owned Git/Fleet rules,
controlled failure/firing and receiver receipt, Console drill-down, recovery with
resolved receipt, redeploy idempotency, source gaps/isolation and owned cleanup.
Do not mark this whole scenario accepted from the two setup checkpoints.

Cleanup uses product deletion of only receipt-owned app/connection after verifying
identity and referencing bindings. Native cleanup may remove only receipt-owned
test resources after original UID checks; preserve shared stack and unrelated apps.
Keep private checkpoints/screenshots/notifications for diagnosis on failure.
