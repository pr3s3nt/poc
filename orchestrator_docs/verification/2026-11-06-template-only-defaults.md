---
id: VERIFY-TEMPLATE-ONLY-DEFAULTS-20261106
artifact: verification-record
status: current
last_reviewed: 2026-11-06
---

# Template-owned defaults and removal of backend policy

Scope: user decision on `features/bo_sung_default`, starting from clean commit
`ad80f333`: remove the backend defaults policy and leave values in templates.
User explicitly authorized deletion of the old task test data. This record
supersedes earlier backend-policy behavior and authoring links; historical records
are retained. Canonical sources are [ADR-039](../architecture/decisions/ADR-039-template-owned-workload-defaults.md),
[WD specification](../usecase/UC-16/workload-defaults-specification.md) and
[contract](../architecture/contracts/workload-defaults.md).

## Outcome

Removed the backend policy type, numerical/environment defaulting and UI selector.
All six standard templates publish version 1.1.0 with their own runtimeDefaults,
including named environment overlays. The generic backend merger preserves sparse
user overrides, false/zero and conditional enablement. Save freezes verified
`templateDefaults` metadata; source pin/profile/environment mismatches are rejected.
Custom workloads without templates get no backend authoring profile. Render-bundle
fallbacks remain in template files. Security/storage/identity guards stay active.
No SQL DDL or migration/reference change was needed.

Old policy documents are rejected. Immutable registry records bearing the retired
capability remain readable metadata, excluded from authoring and rejected for
execution; no policy evaluator remains. An initial real Preview exposed the need
for that metadata reader: rejecting old registry records made an unrelated source
scan fail with HTTP 500. The reader fix was built, tested and the same acceptance
flow then passed. New regression coverage proves read-only retirement.

Self-review also found that a sparse CronJob/PDB document could be accepted before
Save but rejected when reopened with its frozen metadata. Raw intent validation
now permits template-inherited missing leaves both before and after Save; full
resolved validation still requires them. Save/reopen/resolve/save tests cover both.
An authored PDB minimum or maximum suppresses the alternative template quantity.

## Automated checks

- Affected domain, template registry, planning/workloadspec, workload configuration,
  deployment review, renderer and HTTP packages pass. Final targeted checks include
  sparse CronJob/PDB reopen, retired source metadata and both PDB budget alternatives.
  `go build ./...` passes. Logs: `/tmp/issue37-template-only-final-regressions-pass.log`,
  `...-reopen.log`, `...-retired-source.log`, `...-pdb-final.log`.
- Full Go suite was run: the same 56 top-level failures as the earlier issue #37
  run remain, with no new failing test names. Those failures were already reproduced
  on baseline `493eecea`; this is not reported as a green full suite. Original logs
  are `/tmp/issue37-template-only-full-go.log` and `/tmp/issue37-go-tests.log`.
- All 135 workload UI tests pass; typecheck, lint and production build pass.
  Existing PlatformSettingsPage effect/ref and chunk-size warnings remain.
  Full UI suite: 969/972 pass, with the two existing E2E source-assertion failures
  and one App deep-link timing failure; all three related App deep-link tests pass
  when run focused. Logs: `/tmp/issue37-template-only-ui-tests.log`,
  `...-full-ui.log`, `...-app-focused.log`, `...-typecheck.log`, `...-lint.log`,
  `...-frontend-build.log`.
- Real PostgreSQL, REQUIRE_POSTGRES=1: JSONB execution and template snapshot/
  false/zero reopen tests both pass, zero skips. Private log:
  `/home/thanhnt1/.local/state/issue37/template-only-postgres.log`.
- Documentation checker, migration CI guard, E2E syntax and diff checks pass.

## Real Kubernetes and Fleet

Dedicated context `kind-orch-reconciliation-issue37-20261106`, Kubernetes 1.35,
Fleet 0.15.2. Only the issue-owned dev Git/Fleet configuration was used; no AWS run.

The owned-kind adapter test passed in 169.39 seconds: actual non-root/read-only
Nginx HTTP with separate cache/tmp emptyDirs, PDB creation/pruning, two consumers
reusing one PVC UID/data, StatefulSet scale-down/restore preserving claim UID/data,
workload removal retaining storage, then explicit owned storage cleanup.
Namespace `issue37-20261106-153817`, UID
`ffc90b5d-8276-4db8-9302-e8ebc28f2f24`, was verified absent after cleanup.
Private log: `template-only-kind.log` under the issue evidence directory.

The real Console/Fleet scenario passed using Application
`7376312e-6f89-4147-92f0-6af332bfd1b1`: legacy Deploy, no-op Save/Preview, explicit
pinned template adoption, two ready replicas with preserved Deployment UID,
false token/zero history, template resources, effective Preview artifact/provenance,
Preview resourceVersion purity, PDB enable then prune. Receipt records
`acceptancePassed:true`, `cleanupPassed:true`; product removal and Application
deletion passed. Its namespace and Fleet GitRepo
`fleet-local/orch-app-70f9762cf26e3ca275e8eb8f` were independently verified absent.
Private evidence: `template-only-acceptance/flow.json`,
`template-only-acceptance-r2.log`, `template-only-acceptance-cleanup.log`.

Earlier authoring test Application `b1b983d7-cea1-4e4d-9a9e-63638919db9c` was
also deleted through the product, after both environments were checked for zero
resources. Both deleted Applications return 404. Shared stacks and user data were
not reset. The final narrow raw-validation/PDB-alternative corrections were
validated by targeted tests and live Console/API reopen; the unchanged full
Fleet/PVC scenarios were not repeated for those corrections.

## Console handoff

[Verified Console draft](http://desktop-bdjp2rq-1.tail342836.ts.net:18339/ui/applications/5c7bc93a-edd3-4ee2-a618-f3a4a60d9853/environments/staging/workloads/web)
belongs to the replacement `issue37-template-authoring` Application. Its only draft
is `web`, busybox image with pinned backend 1.1.0; no workload was deployed here.

Real browser checks use this exact MagicDNS origin published by Tailscale Serve,
real authenticated API calls and no interception. VI/EN, basic-only initial form,
advanced disclosures, empty actual CPU value with muted 100m hint, typing/clear,
absence of policy/source labels, sparse Save, no-op reopen/Save and environment
profiles (staging 100m, production 200m from template) passed. Browser errors were
empty. After the final backend image restart, sparse CronJob and enabled-PDB
drafts were saved, reopened, resolved and saved again through the real Console/API,
then deleted; the final target page was reverified at `2026-11-06T08:54:27Z`.
Private receipt and screenshots: `template-only-handoff.json`,
`template-only-basic-vi.png`, `template-only-advanced-vi.png`,
`template-only-en.png`, `template-only-handoff.png`. Evidence directory:
`/home/thanhnt1/.local/state/issue37` (outside Git).

Visual reference: existing Console AppShell, shared styles/components and approved
scoped workload form. Navigation, theme and layout remain consistent. Source/policy
explanations were removed from the form; secondary controls stay under advanced.
The exact target page and relevant same-origin APIs were checked from this host;
this does not assert access from every tailnet device/ACL.

Keep Compose `issue37-defaults`, kind and Serve 18339 running. The isolated Docker
volumes retain the draft, registry and dev configuration. Restart from repo root:

```sh
docker compose -p issue37-defaults -f docker-compose.yml \
  -f /home/thanhnt1/.local/state/issue37/compose.override.yml up -d --no-build --wait
```

Sign in with the configured account; no credentials, browser sessions or cluster
configuration are committed. Existing user tmux sessions were preserved.
