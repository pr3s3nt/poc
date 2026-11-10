---
id: VERIFY-2026-11-10-RECOVERY-CONTROLLER-IDENTITY
artifact: verification
status: current
last_reviewed: 2026-11-10
related: [UC-06]
---

# Controller identity check in first-Rollout absence recovery — issue #48

## Scope

Final run on `main` `adf15e96` after merging issues #55 and #48. Branch
`features/recovery-controller-identity`, commit `35bc637f`.

## Finding on main

Automated checks on `main` passed (backend: only the 62 known failures; frontend
1007/1007, typecheck, build; bootstrap 88/88; e2e lib 6/6). On kind, the issue #55
scenario passed (FAILED in 78 s, released; fix SUCCEEDED; deletion SUCCEEDED).
The issue #48 scenario's first Recover stopped with `stage=absence code=OWNERSHIP`
although Git, Bundle, BundleDeployment, Helm and live objects held no Rollout
trace. A build logging each ownership refusal passed on retry. The absence proof
rereads the GitRepo and compared `resourceVersion`; Fleet keeps writing GitRepo
status, observed as resourceVersion 5279997 → 5280077 within 32 s with generation
unchanged. A status write during the proof refused a proven absence.

## Fix

The reread controller must keep its UID, generation, labels, annotations (unit
pins) and spec and must not be deleting; resourceVersion is not compared. Unit
tests cover a status-only write (accepted) and each identity change (refused).
Delivery and Fleet packages pass; backend fails only the 62 known tests; docs pass.

The scenario now reads the unfiltered operation list, which classifies a stale
heartbeat as the Settings page does; the `reviewRequestId` filter only reads.

## Kind runs

Isolated Console upgraded to `issue48b-35bc637f`.

| Run | Interrupt | Recover | Redeploy | Delete |
|---|---|---|---|---|
| `fix-issue48` | PASS | PASS | SUCCEEDED | SUCCEEDED |
| `fix2-issue48` | PASS | PASS | SUCCEEDED | SUCCEEDED |
| `fix3-issue48`, uninterrupted scenario | PASS | PASS | SUCCEEDED | SUCCEEDED |

Each Recover kept the Prerequisite intent, Service UID and controller UID and
applied no Rollout workload.

## Test-harness note

In the failed final run, a stage loop continued after the refused Recover and its
cleanup stage removed the test Fleet access; the next Deploy then ended FAILED
from Fleet `ErrApplied` (forbidden) and deletion was refused (issue #58). Those
resources were removed by hand; this is not a product defect.
