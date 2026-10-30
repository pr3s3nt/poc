# Focused acceptance workload/PVC check

Update an **existing** acceptance backend through Console UI, enable its
Persistent storage panel, write a public test value, recreate only its verified
owned Pod with a UID precondition, and verify the same value/hash and PVC UID.
Headless, no artificial delays, no app creation, login/settings acceptance,
async jobs, PostgreSQL/Redis scenarios, CronJobs or whole-app deletion.

Sources: [PVC procedure](../../../orchestrator_docs/operations/acceptance-storage-schedule.md),
[UC-16 catalog/mount behavior](../../../orchestrator_docs/usecase/UC-16/workload-catalog-specification.md),
[shared storage contract](../../../orchestrator_docs/architecture/contracts/workload-catalog.md).
The script changes a workload configuration and creates a real managed PVC;
it does not edit product UI or hide an unconfigured storage message.

## Prerequisites and setup

Node 22.15+/24+, dependencies and Chromium installed per [E2E guide](../../README.md).
Real Kubernetes adapters, reachable cluster and a running acceptance app with
`backend`/`main`, `frontend`, `worker`, acceptance images supporting storage,
configured shared database/cache/secret references, and a reachable frontend URL.
Use only an Application/cluster explicitly authorized for this check.
No fixture server or synthetic storage response is used.

Register `persistent-storage` Type, its Terraform Definition and the standard
Git render bundle using [existing setup](../../../scripts/bootstrap/README.md).
Start from that setup's `config.example.json`, keeping only the storage Type and
storage Definition; reuse the already configured namespace/cluster/database.
Set `verify.applicationKey` to the target Application. The renderer Definition's
criteria must contain all of `app_id=<Application ID>`, `env_type=staging` and
`res_id=modules.backend`; keep the storage Definition scoped to the same
app/staging and `class=default`. This avoids renderer updates to other workloads.
Pin Git sources to an actual full commit. Setup verifies five catalog choices
and the Add volume mount dialog; it does not deploy unrelated infrastructure.

## Run from repo root

```bash
ORCH_HEADLESS=1 \
ORCH_PLATFORM_URL=http://localhost:3001 \
ORCH_PLATFORM_USERNAME=developer \
ORCH_PLATFORM_PASSWORD_FILE=~/.orch/local-password \
ORCH_APPLICATION_ID=<existing-application-uuid> \
ORCH_KUBE_CONTEXT=kind-k8s-4f \
ORCH_ACCEPTANCE_URL=http://127.0.0.1:18482 \
ORCH_EVIDENCE_DIR="$HOME/.local/state/orch-acceptance/<unique-pvc-run>" \
node e2e/scenarios/acceptance-pvc/run.mjs
```

Password is read only through the shared private-file helper. Evidence directory
must be outside Git; use a new directory per attempt. Maintain the frontend
port-forward independently; the scenario does not start/stop another session's
forward. `ORCH_ACCEPTANCE_URL` is the acceptance app, not the Console.

## Assertions and effects

- Real catalog contains the verified Backend API template; save a screenshot.
- Preserve backend's existing Score/resources/bindings; add the Backend API pin,
  literal `ACCEPTANCE_STORAGE_DIR=/data` and a 1Gi managed PVC through the mount
  form. An already mounted backend reuses its existing configuration.
- No unrelated pending workload is accepted. Preview must affect **backend only**
  before Deploy; other specs/deployment UIDs must stay unchanged.
- Write through the visible acceptance storage form; check value and SHA-256.
- Record actual PVC UID/Bound status, Pod identity and Pod → ReplicaSet →
  Deployment ownership. Delete only that Pod using a UID-preconditioned API call.
- Observe a different backend process, the same value/hash and the same PVC UID.

PASS exits0; assertion/API/ownership failure exits1 and leaves evidence/state
for diagnosis. No automatic mutation retries, draft rollback, cleanup of earlier
attempts or credential output. Existing non-storage custom execution is refused;
review it explicitly rather than overwrite its pin. The selected Application
and PVC/data are retained for the user, including the public test value. A failed
run can leave a pending backend draft or an already deployed PVC; inspect those
before rerunning. No PVC, namespace, database, shared resource or Application is
deleted. The one restarted Pod is replaced by its existing controller.

Video/screenshots/result/UID evidence stay outside Git. The Console and storage
recordings may be concatenated/published when the user has requested video upload;
no unrelated workflow is included. This check proves Pod-recreation retention,
not update/removal/volume deletion, backups or scheduled execution.
