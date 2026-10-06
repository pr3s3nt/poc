---
id: VERIFY-2026-10-06-SCORE-K8S-RENDERING-KIND
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-06
---

# Definition-selected score-k8s rendering — kind browser verification

## Scope and result

Run `rendered-20261006105838-5338` passed (runner exit 0) on the existing
`kind-idp-internal` (Kubernetes v1.36.1) with real Kubernetes/Vault/VSO adapters,
a private JSON state and `-score-k8s` configured (score-k8s 0.15.0, SHA-256
`52b7fb666f702b21f9d5433380bd4d8ac91a3192dd5e1f3b9c6a141a861d99dd`). Earlier
runs 1–6 failed on test defects only (locator, API response key, an assumption
about labels, an assumption that Application variables are plain env values, and a
runtime-check retry); no product defect was found. Their namespaces were cleaned.

```bash
cd frontend && npm run build && cd ..
bash backend/test/integration/template-engine-playwright-kind.sh
```

Scenario `frontend/test/e2e/template-engine-kind-human.mjs` (headed Chromium on
private Xvfb, ffmpeg 15 fps, typing 85–110 ms/char, visible cursor, 6–8 s pauses):

1. Developer creates the Application and Variables & Secrets, including literal
   values `$HOME`, `${HOME}`, `$${HOME}`, `price=$5 x$${y} $$` and `{{ .Values.name }}`.
2. Platform Engineer registers a `score-k8s` workload Definition with criterion
   `app_id=<this application>`, `env_id=staging`; the stored criteria match exactly.
3. Developer enters backend (PostgreSQL, config, secret, literals, explicit
   requests/limits) and frontend (limits only) workloads.
4. Deployment preview shows `score-k8s 0.15.0 (<definition>)` for both workloads,
   not “built-in Kubernetes”. Deploy succeeds for both.
5. Read-only kubectl compares live objects; the diagnostic page shows PASS for
   backend connection, environment, secret and database.

## Observations

- Invocation witness: the orchestrator's `-score-k8s` path was a logging wrapper
  that execs the real pinned binary. Its log shows `--version`, then `init` and
  `generate --namespace <run namespace>` once per workload. The bundle digest
  therefore covers the wrapper, not the pinned binary itself.
- The renderer's metadata patch restores protected identity: Deployment/Service
  names, namespace, selectors and labels (`managed-by=orchestrator`) equal the
  native contract. score-k8s's own labels are therefore not a usable witness.
- Resources: backend requests 50m/64Mi, limits 200m/128Mi preserved; frontend
  limits-only gives requests equal to limits (UC-06 BR-11). Ports 8080, Service
  selector equals Deployment selector, `readyReplicas == replicas`.
- `PGPASSWORD` and `ACCEPTANCE_SECRET` are `secretKeyRef`; no secret bytes appear
  in Deployment JSON or on screen. `PGHOST` is the shared PostgreSQL Service DNS
  name; the StatefulSet/PVC exist and were not produced by score-k8s.
- With VSO delivery every Application variable, including non-secret literals, is a
  `secretKeyRef` to the synced Secret (`main_<KEY>`), not a plain `value`. The five
  literal values were read from the running backend process environment through an
  ephemeral debug container (`LIT_` keys only) and equal the typed values exactly.

## Not verified

- Probes: the product Score/UI has no probe field; live Deployments have none.
- Empty literal value: the UI cannot save an empty variable; covered by CLI tests only.
- Plain (non-Secret) literal env values through score-k8s on live kind, since VSO
  mode routes variables through Secrets.
- Standalone “Preview Score” page provenance (covered by the HTTP test).
- Typing was 85–110 ms/char, slightly under the 100–150 ms target.
- Worker processing: only backend/frontend were deployed; the submitted job
  remains PENDING. The four diagnostic checks passed, not the complete worker flow.

## Validation

The live runner passed with exit 0. Frontend typecheck, lint, all 112 unit tests
and production build passed. Shell syntax, documentation checker and
`git diff --check` passed. Go test/build were not rerun separately because no
Go source changed; the live runner built the backend executable.

## Cleanup

Namespace absence was confirmed by a successful API lookup; the cluster, Vault and
VSO were not modified except the run's own VSO objects and Vault KV revisions,
whose retention was not inspected. Run-tagged images were removed from Docker and
the node cache on a best-effort basis (not verified).

## Evidence

Retained outside Git: MP4 `/tmp/template-engine-kind-xTkU9X/evidence-run7/template-engine-review.mp4`
(490.3 s, H.264, full decode clean, 13 settled frames), `marks.json`,
`live-manifests.json`, `score-k8s-invocations.log`, `runtime-debug.json`, `run.json`.
The coordinator reviewed the settled Preview, Deploy and final diagnostic
frames: the address bar is visible and the app shows all four checks PASS.
Review was sampled, not a full manual playback. At the user's subsequent request,
the video was uploaded with a run-specific name to the existing
`acceptance-recordings` release:
[review video](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/template-engine-kind-rendered-20261006105838-5338.mp4).
The downloaded asset matches the local file's SHA-256:
`5b18d8e59df3c392ee2b77c632e26f32d882f1e1efed11414db4869f294c6ef9`.

Design: [ADR-010](../architecture/decisions/ADR-010-score-k8s-workload-rendering.md),
[rendering contract](../architecture/contracts/workload-rendering.md),
[local record](2026-10-06-score-k8s-rendering-local.md),
[runbook](../operations/kind.md).
