---
id: VERIFY-WORKLOAD-DEFAULTS-20261106
artifact: verification-record
status: current
last_reviewed: 2026-11-06
---

# Issue #37 — workload defaults and runtime policy

Scope: [WD-01–10](../usecase/UC-16/workload-defaults-specification.md),
[implementation map](../implementation/workload-defaults.md).
Branch `features/bo_sung_default`, baseline main `493eecea9df0329358cb4560e958604b6a570e98`.
Initial working tree was clean. All changes in this handoff belong to issue #37.

## Source validation

- Go build passes. Focused domain environment/template source, planning/workloadspec,
  planner, workload configuration, renderer/semantic guard, deployment review,
  Kubernetes, Fleet/recovery, delivery coordinator and HTTP checks pass.
- Real PostgreSQL tests pass: execution/default policy JSONB reopen preserves
  false/zero, immutable pins and workload snapshots. No schema DDL was introduced;
  active baseline remains v10. Released migration/reference immutability guard passes.
- Full Go suite was executed. Its 56 failing top-level tests across 12 packages
  were all reproduced at baseline `493eecea`; no task-only failing test remained.
  Packages: templatesources, catalog, configuration, deployment, pending, preview,
  provisioning, bootstrap, conformance, e2e, v2cleanup and v2run. Baseline logs and
  comparison are retained outside Git; these failures are not presented as passes.
- Frontend full suite: 961/964 passed. Both historical label/assertion failures
  (`terraform-v2-labels.test.mjs`, `v04-internal-full.test.mjs`) reproduce on baseline.
  The additional observability polling timing failure passes when run focused.
  All 133 focused workload editor/defaults/lifecycle/storage/class tests pass, including inherited
  image editing while metadata refreshes and effective template mounts/probes.
- Typecheck, lint and production build pass. Lint retains the existing
  PlatformSettingsPage effect/ref warning; build retains its chunk-size warning.
  Documentation checker, migration checker and diff whitespace check pass.

## Authorized real environments

The user authorized a dedicated kind plus the existing dev Git/Fleet configuration,
limited to issue #37. Setup reused the existing reconciliation-kind and simplified-CD
setup helpers. Setup itself is not application acceptance.

- kind: `orch-reconciliation-issue37-20261106`, Kubernetes 1.35, Fleet 0.15.2.
- Compose: `issue37-defaults`, isolated PostgreSQL/Vault/cache/delivery Git state;
  real-adapter backend port 18338 and Console port 18339.
- READY account/CD binding: `issue37-defaults-20261106` in this isolated Console.
- Private receipts, logs, kubeconfigs, browser captures and local image build inputs:
  `/home/thanhnt1/.local/state/issue37` (outside Git). No credentials in this record.

## Runtime observations

[Browser/CD scenario](../../e2e/scenarios/workload-defaults/README.md) uses the
actual Tailscale origin and product Save/import, Preview and Deploy without response
interception. Application `23f116de-560a-4d12-afa5-fcfc46201c27` first deployed the
legacy Nginx configuration through Fleet. Reopen/Save preserved the raw document;
Preview then showed zero changes. Explicit adoption pinned `nginx-readonly@1.0.0`
and v1 staging policy. Real Deploy reached two Ready replicas with the same
Deployment UID, explicit token false/history zero, RollingUpdate 0/1, CPU request
100m, memory request 128Mi/limit 512Mi, no CPU limit, non-root/read-only security,
two emptyDirs and a matching PDB. Preview left controller UID/resourceVersion
unchanged and displayed the effective configuration artifact.

The final browser continuation (`acceptance-r12.log`) passed the complete recorded
legacy → explicit policy → form PDB disable sequence. PDB disable Save and Deploy
succeeded; the PDB was absent while Deployment UID and two Ready replicas remained.
Product workload removal and Application deletion then passed (`cleanup.log`);
namespace `app-23f116de-560a-4d12-afa5-fcfc46201c27-staging` and the dedicated
cluster's application Fleet GitRepo were independently confirmed absent. Failed
attempt receipts remain as historical diagnosis; they are not passed runs.

The first existing-controller preflight exposed client-side apply's port-list merge:
old HTTP port 80 and new 8080 produced duplicate names in dry-run. Admission now
uses complete-object server create/replace dry-run with live resourceVersion and a
regression test. The rejected update had not published either Git artifact; recovery
was extended to prove absent publications, the exact prior shared-repo pin/tree,
and unchanged source UIDs/revision/Helm ownership before releasing the hold. Real
Console recovery succeeded; it did not replay or mutate the rejected update.

The browser form also exposed a Save race during default refresh for a template-
provided image. The form now preserves image omission, defers inherited image
admission to the backend and keeps the real resolver gate before saving. Focused
regressions cover omission, PDB editing and metadata refresh; no guessed image is saved.

The opt-in real renderer/Kubernetes-adapter scenario passed in 181.99 seconds:

- Actual Nginx returned its welcome page as non-root with read-only root; both
  `/var/cache/nginx` and `/tmp` were writable through separate emptyDirs.
- PDB enable and inventory-based prune passed.
- Two Deployment consumers reused `shared-data` sequentially, preserving its UID
  and written data. Rendering created no second PVC; removal preserved the source.
  The independently owned source was explicit test setup, not Terraform acceptance.
- StatefulSet claim `data-stateful-0` retained UID/data across operational scale
  to zero and restore, and retained the PVC after adapter workload removal.
  Product authoring still permits StatefulSet replica one only. Explicit claim
  cleanup used its UID, then namespace cleanup used its UID.
- Namespace `issue37-20261106-131828`, UID
  `c132c5e1-bfe0-4ea7-848f-7ec29fdda4b9`, was independently confirmed absent.

This adapter storage run is separate from the browser/Fleet run. It does not prove
Terraform source provisioning or retained-claim automatic adoption. V1 intentionally
requires separate owned data cleanup; no automatic claim adoption/data migration UI.
No company staging or AWS mutation was performed.

## Console handoff and retained services

Verified Console page:
[Edit workload draft](http://desktop-bdjp2rq-1.tail342836.ts.net:18339/ui/applications/b1b983d7-cea1-4e4d-9a9e-63638919db9c/environments/staging/workloads/web).
At `2026-11-06T06:31:43Z`, the exact MagicDNS URL from Tailscale Serve was opened
in Chromium from this host, authenticated, and displayed the saved `web` draft
and effective CPU 100m/environment source. Its real same-origin resolver returned
200 and the expected values/sources; browser page errors were empty. This proves
that host access through that URL, not access from every tailnet device/ACL.

Retained `issue37-authoring` Application `b1b983d7-cea1-4e4d-9a9e-63638919db9c`
contains only an authoring draft (busybox, v1 staging policy); it was never Previewed
or Deployed. It is distinct from the deleted runtime acceptance Application.
The final real-adapter stack includes the strict invalid-envelope resolver fix;
its focused HTTP regression passes. The earlier auxiliary fake-adapter UI process
and its own port-18337 Serve route were stopped after the real stack was verified.
Keep the dedicated Compose stack and kind running; real application data is stored
in this project's isolated Docker volumes. Restart the retained stack with:

```sh
docker compose -p issue37-defaults -f docker-compose.yml \
  -f /home/thanhnt1/.local/state/issue37/compose.override.yml up -d --no-build --wait
```

Use the configured sign-in account; no secret is included here. Shared user stacks,
Tailscale routes, clusters and tmux sessions remain untouched. UI reference was the
existing Console shell/components/styles and shared UI design; browser comparison
retains that shell and uses compact scoped groups. Effective inherited mount/probe
summaries were added so blank override fields do not imply disabled configuration.
