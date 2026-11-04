---
id: VERIFY-GENERIC-WORKLOAD-20261104
artifact: verification-record
status: current
last_reviewed: 2026-11-04
---

# Generic Workload / Template Driver acceptance

Scope is [ADR-035](../architecture/decisions/ADR-035-generic-workload-template-driver.md),
[contract](../architecture/contracts/generic-workload.md) and
[implementation](../implementation/generic-workload.md). User authorized discarding
obsolete experimental workload configuration and mutating only task-owned resources
on k8s-4f. User explicitly restricted testing to changed behavior/direct consumers.
Initial branch was `features/refactor_workload`, working tree clean. No preexisting
user changes were modified. No shared installation was reset or upgraded.

## Focused checks

Passed:

- Generic shorthand, multi-container authoring/round-trip, explicit replicas zero;
  planning/matching, Definition-only changes and stale admission.
- Inline preparation and default editor templates, verified Git/ZIP/source cache,
  immutable pin/reinspection, direct rendering/guards, nonsecret Resource References,
  deployment selection/snapshot and existing encrypted CD artifacts.
- Real PostgreSQL: populated frozen schema 7 to 8, exact retained connection,
  Terraform Definition/instance/state and installation identity; obsolete-source
  cleanup; selection revision/document consistency; repeated setup; failed-grant
  rollback; runtime DDL denial. Active baseline matches frozen version 8. Focused
  supported-source version-7 and state/isolation version-6 consumers passed.
- Console: 84 selected Definition/template/deployment-review tests; 20 selected
  navigation/matching/metadata tests; affected Workload editor/parser/preview tests.
  Final overview change has focused tests. TypeScript typecheck, affected-file
  ESLint, frontend production build and backend build passed. Filtered-out tests
  were intentionally not executed; compile-only package checks are not acceptance.
- Bootstrap workload helper tests: 12 passed. Changed shell syntax and new E2E
  JavaScript syntax passed. Documentation/PlantUML, migration immutability/version
  guard and Git whitespace checks passed.

No whole-suite checkpoint, unrelated E2E, AWS run or intentional failure runtime
scenario was executed. Versioned migrations/references 1–7 were not changed.
Historical Score E2E wrappers requiring the removed renderer are retired; mixed
historical scripts retain their paths/provenance, not current rendering assertions.

## Real browser / CD / Fleet

The tracked [scenario](../../e2e/scenarios/generic-workload/README.md) uses real
Console/backend, Terraform namespace provisioning, Git and Fleet. It creates an
Inline workload Definition with Deployment, conditional Service and ConfigMap,
uses the existing namespace Resource Reference, imports Generic Workload shorthand,
then Preview/Deploy and a replicas/env update. It checks immutable source/Definition
pins, namespace reuse, controller readiness, real resources and Fleet GitRepo/Bundle
exact SHA and readiness. No API response interception or direct workload apply.
Namespace access setup creates only a labelled Role/RoleBinding after the product
namespace exists, with exact UID checks; it never changes shared Fleet permissions.

The first rehearsal exposed missing Fleet permissions, then succeeded after
scoped access setup and product recovery of the proven stopped operation. Its
Application `3a0f7bb8-ae4a-4a1a-a13e-a3125c4e3f54` was deleted via product cleanup;
namespace UID `a347d8e2-6f34-4388-95d0-d3823a66ce70` is absent.

The full Inline/reference/update run used Application
`1a1438ba-d31a-4201-9c8b-b9a1efeb6a0b`, namespace UID
`506e6868-cf9f-4ced-9449-f869138ca0bd`. Baseline deployment
`341ea030-5adc-4405-9030-744d3a2a51fa` and update
`9b41fa61-b173-44b7-b5f4-537a2135c646` succeeded. Source digest:
`be4eac9332940afcc16071e3b521111846b50d6480beeab06b8fbd7e7f10dfe6`.
Verified Fleet unit commits were `a9319de7df5e2aa65affcb14847cff3b62c3b6bd`
and `bc30fa3939c7de3756e6c747a86654ecc646734e`. Both rendered Bundles were 1/1
ready at the pinned SHA. Product remove/deploy and Application deletion passed;
namespace is absent. Retired reader Secrets were deleted with binding ownership
and UID preconditions. The task-only rehearsal Compose projects/data were removed
after product cleanup; shared ports 3001/3031 and unrelated resources were untouched.

Final executable pin hardening was checked locally and rerun on the handoff stack;
final Application `b515188a-0f3e-45e5-aaa5-702f91a6eb8e`, namespace UID `095cbdd7-75d8-4c9d-bb59-6f50464a9343`
passed baseline/update with source digest `be4eac9332940afcc16071e3b521111846b50d6480beeab06b8fbd7e7f10dfe6`.
Deployments `b1eee0f6-938c-4665-8d77-cfecaeac4bfc` and
`49ad96ca-37fd-4b3d-9bf2-7de2ef867563` succeeded. Both Fleet Bundles were 1/1
ready at `f42a1043ad31c3ccce01d9d71388ad873a24b365` and
`e93477a2b35f3a650a020a710ef542d5f0deba40`. Runtime executable SHA-256 matches
the final image: `5e3d927946c172e67d77dc91ff169bc60939d042c2ea2a74c9f636bb97698e20`.
Product workload remove/deploy and Application deletion passed. The exact
namespace, task GitRepos and Bundles are absent. Namespace access Role/RoleBinding
were removed with ownership/UID preconditions before Application deletion.
The current platform-owned reader Secret remains with the reusable CD binding.

## Console handoff

Real Console, not mock:
[Resource Definitions / workload](http://desktop-bdjp2rq-1.tail342836.ts.net:3040/ui/platform/resource-definitions?resourceType=workload).
The exact MagicDNS URL is published by Tailscale Serve. Browser checks through this
URL verified login, the actual Definition editor, available-source API, Inline
fields and no browser errors from this host. This does not assert other-device ACLs.

UI reference: existing Resource Definitions editor on the actual port-3001 Console,
[shared UI design](../architecture/ui/README.md), existing shell/styles/components.
Visual comparison retained the existing shell, tabs, overview, review/save/history.
Schema/defaults, source pins and companion JSON are disclosures; unrelated module
version is omitted from template Overview. Credentials are never in screenshots,
tracked files or URLs.

Hosting remains Compose project `workload-template-final-20261104`, isolated
PostgreSQL/Vault/cache and backend/frontend images tagged `generic-workload-final`.
Private evidence/config/password files are outside Git under
`~/.local/state/generic-workload/{20261104,final-20261104,handoff-20261104}`.
The handoff stack retains reusable Connections/CD default and Definition examples;
the task acceptance Application/resources are cleaned up. PostgreSQL/Vault/cache
volumes remain for this Console; no shared database migration is claimed.

Restart on this machine from repository root (private override contains no secret):

```sh
docker compose -p workload-template-final-20261104 -f docker-compose.yml \
  -f /home/thanhnt1/.local/state/generic-workload/handoff-20261104/compose.override.yml up -d
sudo tailscale serve --bg --http=3040 http://127.0.0.1:3040
```

The Serve/hosting processes remain running for handoff. The retired task-only port
3038 proxy was removed. Existing shared Serve routes were preserved. Sign in with
the locally configured Platform Engineer account; no password is published here.
