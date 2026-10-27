---
id: RUNBOOK-APPLICATION-DELETION-ACCEPTANCE
artifact: operations-runbook
status: current
last_reviewed: 2026-10-27
related: [UC-18, ADR-025]
---

# Application deletion acceptance (UC-18)

User authorization 2026-10-27: verify Application deletion end to end as a
real user, on kind and on AWS. Behavior: [UC-18](../usecase/UC-18/specification.md).
Both runs reuse existing owned-run harnesses; nothing in them is faked.

## kind

```bash
bash backend/test/integration/workload-manifests-kind.sh \
  --evidence /absolute/new/evidence/directory --run-id uniqueid --app-deletion
```

The runner owns a new kind cluster, Compose project, workload Vault and run
image tags, and its EXIT trap removes exactly those. `--app-deletion` replaces
the WM-E07 removal with WM-E08: after the full application (Git bundle objects,
PostgreSQL, Redis, VSO secret, HTTPRoute) was deployed, updated and exercised,
the Developer opens Cài đặt, deletes the Application with the exact name and
waits for the list notice. Checks: dialog scope per Environment, case-folded
name refused, 409 on a write while deleting, deletion SUCCEEDED, Application
absent from the list and 404, namespace and HTTPRoute absent, every owned
Terraform instance DESTROYED, tombstone and Deployment history retained, the
Application's Vault value path empty, kind cluster still serving, and the same
name/subdomain accepted by a new Application.

## AWS (A02)

```bash
ORCH_E2E_APP_DELETION=1 bash backend/test/integration/terraform-v2-acceptance.sh \
  --config /private/config.json --browser-only
```

Follow [AWS verification](aws.md) and the [acceptance contract](terraform-v2-acceptance.md):
`--prepare-only` first, fresh egress `/32`, pins, STS, permissions, capacity,
estimate and predecessor gates; the independent supervisor is armed before the
first create. With `ORCH_E2E_APP_DELETION=1`, after the full A02 flow (EKS,
Aurora, ElastiCache, job/cache/update/restart) the Developer deletes the
Application through the Console. `appdel.*` assertions require every staging
resource DESTROYED by the product and native AWS queries proving the
`app-resources`, `eks` and `vpc` layers absent. The supervisor then runs as the
safety net and must report CLEAN; the SQL reopen expects the tombstone and a
SUCCEEDED deletion record.

Evidence stays outside Git. An execution result belongs in a dated
verification record.
