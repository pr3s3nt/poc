---
id: IMPLEMENTATION-CODE-MAP
artifact: design-to-code-map
status: current
last_reviewed: 2026-10-28
---

# Design-to-code map

## Custom workload runtime authoring — GW-06/07

`environment/volumes.go`, `probes.go`, `execution.go` and `workloadspec/workload.go`
validate lifecycle/volumes/probes. `templatesource/workload_capabilities.go` and
`planning/workload_execution.go` gate matched sources; `templatedriver/pod_volumes.go`
and standard `container-lifecycle.tpl`, `execution.tpl`, `configmaps.tpl` render
and guard intent. `deploymentreview/pod_projection.go` preserves safe review
structure. Frontend `ContainerVolumeFields` / `VolumeSourceFields`, `ProbeFields`, `ExecutionFields`,
`runtimeIntent` and the existing editor/create dialog implement authoring.
Tests include the runtime form component tests and
`e2e/scenarios/workload-runtime-form/`. See
[verification](../verification/2026-11-06-chinhform-runtime.md).

## Workload class authoring — UC-16 BR-31/32

`frontend/src/features/workloads/WorkloadClassControl.tsx` reuses `Advanced` and
the published `workload` Type classes retained by `WorkloadEditorPage.tsx`.
`WorkloadClassMetadata.test.tsx` covers default omission, authoritative choices,
retained classes, metadata failure/retry and withdrawal without mutation.
Backend matching/API/persistence contracts are unchanged.

## Local API platform seed and kind Console

Operator procedure: [runbook](../operations/platform-seed.md). Local CLI:
`scripts/bootstrap/seed-api.mjs`; API orchestration, private input validation and
rerun checks: `scripts/bootstrap/lib/api-seed.mjs`; focused tests beside it.
Nonsecret Kubernetes installer manifests: `deploy/kind/console.mjs`; reuses
`deploy/local/` credential/PostgreSQL/Vault scripts. Existing product registration
services, HTTP authorization and schema stay unchanged.

## Readable IDP application names

[ADR-038](../architecture/decisions/ADR-038-readable-idp-names.md), UC-01 BR-15
and UC-06 CD-09 map to `domain/application/naming.go`, the Application creation
transaction, `application/delivery/repositories.go`, Fleet allocation binding/
configuration/controller identity, and `planning/context.go` infrastructure names.
UUID persistence and existing globally unique application keys are unchanged.
Service, PostgreSQL concurrency/reopen, HTTP creation, planner-name and Fleet
two-Environment lifecycle tests cover the new rule and historical identities.
The shared name helper places the one persisted random suffix after the purpose
or Environment; generated-key collision retries happen before writes. HTTP/real
PostgreSQL deletion/recreation confirms fresh identities with the same display name.

## Controlled Definition upgrades

[UC-03 DV-01–07](../usecase/UC-03/definition-versions.md) maps to immutable revision/Environment repositories, catalog resolution, Plan snapshots, fenced activation and the existing forms. See the [implementation route](definition-versions.md) for paths and supported tool-retention boundaries.

## PostgreSQL versioned upgrades

[ADR-027](../architecture/decisions/ADR-027-postgres-upgrades.md): `backend/internal/adapters/postgres/upgrade.go`, frozen `migrations/` SQL, owner `Setup` and runtime marker/history validation. Tests: `upgrade_integration_test.go`; guard: `scripts/check_migrations.py`; Compose orchestration: `deploy/local/upgrade-database.sh`. Procedure: [runbook](../operations/postgres-upgrades.md).

## Generic Terraform resource completion — UC-08 BR-10

- Completion after successful execution/state/output validation:
  `application/tfexec/run.go`, `adapters/terraform/readiness/readiness.go`,
  `ports/terraformexec/terraformexec.go`. Additional Kubernetes/AWS probes remain
  mandatory for every applicable object in a mixed module.
- Real Git module: `backend/test/acceptance/terraform-completion/main.tf`.
  ConfigMap, ServiceAccount and `kubernetes_manifest` objects feed two shared
  consumers and one private consumer in the real full-application scenario;
  manifest resources exercise provider schemas without an `id` attribute.
- Acceptance: `workload-manifests-kind.sh --completion-ref <published-commit>`,
  `frontend/test/e2e/terraform-completion-flows.mjs`; see
  [the runbook](../operations/terraform-completion-kind.md) and
  [57-check real acceptance](../verification/2026-10-26-terraform-resource-completion.md).
  Subsequent AWS execution and its unresolved node-join gate are recorded in
  [AWS attempt/cleanup](../verification/2026-10-26-terraform-completion-aws-r5.md).
  [The scope/source review](../verification/2026-10-26-terraform-completion-scope-review.md)
  maps the completed positive AWS flows reused for this fix and preserves the
  limits of the failed full historical harness.
  The current AWS fixture uses `terraform-v2-acceptance.sh` and
  `restart-owned-product.sh` for durable Git cache and actual process restart.

## Verified feature defect fixes — F3/F4/F5

- UC-16 BR-17 shared names: `domain/environment/resource_aliases.go`,
  `application/workloadconfig/resource_aliases.go`, reconstruction, deployment
  snapshot/service and PostgreSQL `storedSet`; private metadata does not enter
  planner documents, hashes or public review.
- Namespace recreation: `application/tfexec/namespace.go` captures an immutable
  baseline per instance/namespace UID; `adapters/terraform/recovery` accepts the
  scoped handle. Historical matching baselines remain readable.
- Git source initialization: `RenderingTemplatesPage.tsx` resolves the saved
  bundle independently of the selected source mode and invalidates pending
  initialization on source edits, reset or another edit.

Regression mapping: [traceability](../traceability/matrix.md#f3f4f5-regressions).

## Account diagnostics — ADR-022

Canonical behavior: [UC-04 DIAG-01..14](../usecase/UC-04/specification.md#account-diagnostics--diag-0114),
[contract](../architecture/contracts/account-diagnostics.md),
[delivery and acceptance](account-diagnostics.md).

- Aggregate, safe fact registry and owner/lease/revision transitions:
  [`domain/diagnostics`](../../backend/internal/domain/diagnostics/).
  Repository/probe ports: [`ports/diagnostics`](../../backend/internal/ports/diagnostics/).
- Async worker, cancellation, expiry/recovery and requirement resolution:
  [`application/accountdiag`](../../backend/internal/application/accountdiag/).
  Full desired-state snapshot: `pending.Service.Desired` in
  [`pending/preview.go`](../../backend/internal/application/pending/preview.go).
- Actual saved-credential Kubernetes observations/execution/cleanup:
  [`diagnostics.go`](../../backend/internal/adapters/kubernetes/diagnostics.go),
  [`diagnostics_exec.go`](../../backend/internal/adapters/kubernetes/diagnostics_exec.go).
  AWS signed observations and selected policy families:
  [`awsaccess/diagnostics.go`](../../backend/internal/adapters/awsaccess/diagnostics.go).
  Certificate and Vault evidence:
  [`tlsprobe`](../../backend/internal/adapters/tlsprobe/),
  [`vault/inspect.go`](../../backend/internal/adapters/vault/inspect.go).
- Durable reports and shared repository contract:
  [`postgres/diagnostics.go`](../../backend/internal/adapters/postgres/diagnostics.go),
  [`store/diagnostics.go`](../../backend/internal/adapters/store/diagnostics.go),
  [`persistencetest/diagnostics.go`](../../backend/internal/ports/persistence/persistencetest/diagnostics.go).
  Fresh schema/grants: [`baseline.sql`](../../backend/internal/adapters/postgres/baseline.sql),
  [`grants.go`](../../backend/internal/adapters/postgres/grants.go).
- Scoped HTTP routes, safe DTOs, staleness and account summaries:
  [`account_diagnostics.go`](../../backend/internal/delivery/http/account_diagnostics.go),
  [`driver_accounts.go`](../../backend/internal/delivery/http/driver_accounts.go).
  Wiring: [`bootstrap.go`](../../backend/internal/bootstrap/bootstrap.go).
- Integrated list/detail and real-report UI:
  [`DriverAccountsPage`](../../frontend/src/features/platform/accounts/DriverAccountsPage.tsx),
  [`diagnostics`](../../frontend/src/features/platform/accounts/diagnostics/),
  [`routes`](../../frontend/src/app/routes.ts), VI/EN catalogs and
  [`diagnostics.css`](../../frontend/src/styles/diagnostics.css).
- Tests: application/adapter tests beside those sources,
  [`HTTP end-to-end contracts`](../../backend/test/e2e/account_diagnostics_http_test.go),
  [`DiagnoseTab tests`](../../frontend/src/features/platform/accounts/diagnostics/DiagnoseTab.test.tsx),
  [`account integration tests`](../../frontend/src/features/platform/accounts/DriverAccountsDiagnostics.test.tsx).
  Real Console/runtime runner:
  [`diagnostics-e2e.sh`](../../backend/test/integration/diagnostics-e2e.sh),
  [`browser scenario`](../../frontend/test/e2e/account-diagnostics.mjs),
  [`explicit Compose overlay`](../../deploy/local/compose.diagnostics-e2e.yml),
  [`API fixture setup`](../../backend/test/integration/diagfixture/main.go).
  [Independent acceptance](../verification/2026-10-25-account-diagnostics.md)
  records source-bound DG-E01..09 evidence.

| Design concern | Implementation entry point |
|---|---|
| Real new-user native telemetry acceptance | `backend/test/integration/observability-kind.sh`, `frontend/test/e2e/observability-kind.mjs`, `backend/examples/observability-app/`; [48-check evidence](../verification/2026-10-25-native-observability-live.md) |
| ADR-023 / UC-09 TEL native collection and history | `backend/internal/domain/telemetry/`, `ports/observability/`, `ports/persistence/telemetry.go`, `adapters/{kubernetes,store,postgres}/telemetry.go`, `application/observability/`, `delivery/http/observability.go`, `bootstrap/bootstrap.go`; [handoff](native-observability.md) |
| Approved application telemetry UI | `frontend/src/features/observability/`, ApplicationHomePage, App/routes, existing AppShell, `frontend/test/e2e/observability.mjs`; [verification](../verification/2026-10-25-native-observability.md) |
| ADR-020 Console message catalogs and language selector | `frontend/public/locales/` (`manifest.json`, `vi.json`, `en.json`), `frontend/src/shared/i18n/index.ts` (`t`, locale pick/load, `intlLocale`), `frontend/src/main.tsx` (load before App import), `frontend/src/shared/ui/LanguageSelect.tsx`; tests `frontend/src/shared/i18n/i18n.test.ts`, `frontend/src/test/setup.ts` |
| ADR-019 safe review and applied baseline | `backend/internal/application/deploymentreview/`; `registry.Service.ReadPinned`; `deployment/snapshot.go`, `deployment/service.go`; `planning.Plan.Review` |
| Correlated operation, durable review and observed progress | `envops/manager.go`; `adapters/store/ops.go`; `adapters/postgres/operations.go`, `deployment.go`, `baseline.sql`; `deployment/query.go`, `node_progress.go`; HTTP `operations.go`, `pending.go` |
| Visual graph, source/manifest diff and polling | `frontend/src/features/preview/DeploymentReview.tsx`, `reviewObservation.ts`, `reviewEdges.ts` (deduplicated reason-preserving obstacle routing), `reviewRequestId.ts` (cryptographic UUID fallback for HTTP demo); ApplicationHomePage, ScorePreviewPage and DeploymentDetailsPage; `frontend/test/e2e/deployment-review-local.mjs`; `backend/test/fixtures/deployment-review/` |
| Issue #8 Resource Definition v3 detail/cards/tabs/rules, atomic advanced mode, metadata default previews | `frontend/src/features/platform/ResourceDefinitionsPage.tsx`, `CriteriaEditor.tsx` (rule projection for Definitions), `CriteriaRules.tsx`, `MappingEditor.tsx`/`ParameterSources.tsx` (was TypedVariablesEditor)/`ProvisionEditor.tsx` (Mẫu dựng ứng dụng) and existing converters; `DefinitionV3.test.tsx`; focused real HTTP/Git browser `frontend/test/e2e/resource-definition-v3-local.mjs` via `backend/test/integration/gitmodulebrowser/` |
| Issue #11 module forms: connection, source chooser, parameter cards, companions | `frontend/src/features/platform/InfrastructureConnection.tsx`, `valueSource.ts`, `ValueSourceDialog.tsx`, `ParameterSources.tsx`, `CompanionResources.tsx`, `ResourceDefinitionsPage.tsx`; tests `ModuleForms.test.tsx`, `valueSource.test.ts`; e2e helper `frontend/test/e2e/definition-sources.mjs`; mock [`frontend/design/resource-definition-module-forms`](../../frontend/design/resource-definition-module-forms/README.md) |
| Git source default + published release-equivalent modules | `ResourceDefinitionsPage.tsx` create/reset/edit source state; `registry/git_inspect.go` content+Type lifecycle identity; `git_inspect_test.go`; `published-modules-local.mjs`; [runbook](../operations/terraform-module-sources.md) |
| Issue #7 direct Git Terraform module authoring | `backend/internal/adapters/terraform/registry/git_inspect.go`, `fetch.go`, `backend/internal/application/catalog/git_inspect.go`, `backend/internal/delivery/http/terraform_modules.go`, `frontend/src/features/platform/GitModuleSource.tsx`; focused browser: `backend/test/integration/gitmodulebrowser/`, `frontend/test/e2e/git-module-local.mjs` |
| V03 Definition-selected cluster/workload placement (ADR-016) | `backend/internal/planning/workload.go`, `schedule.go`, `backend/internal/application/deployment/service.go`, `backend/internal/ports/execution/clustertarget.go`; target pins in `backend/internal/domain/environment/document.go` |
| V03 existing-cluster authoring and strict renderer inputs | `backend/internal/application/catalog/capabilities.go`, `backend/internal/domain/resource/rendering.go`, `frontend/src/features/platform/ResourceDefinitionsPage.tsx`, `RenderingTemplatesPage.tsx`, `frontend/src/features/preview/Placements.tsx` |
| V03 real frontend acceptance, restart and owned cleanup | `backend/test/integration/v03-internal-frontend.sh`, `v03-supervisor.sh`, `v03-cleanup.sh`, `backend/test/integration/v03run/`, `frontend/test/e2e/v03-internal-frontend.mjs`, `v03-reopen.mjs`; [coverage and execution record](../verification/2026-10-18-v03-handoff.md) |
| V02 provision authoring and stored rules → Preview graph/batches | `HTTP planner tests` (editor HTTP test retired 2026-11-02; see Catalog release verification), [`focused runner`](../../backend/test/integration/v02-provision-local.sh), [`scenario`](../../frontend/test/e2e/v02-provision-scenario.mjs) |
| V02 Definition CAS/Driver Inputs/account and shared matching | [`catalog regression`](../../backend/internal/application/catalog/definition_update_test.go), [`matcher`](../../backend/internal/domain/resource/match.go), [`headed runner`](../../backend/test/integration/v02-definitions-local.sh), [`scenario`](../../frontend/test/e2e/v02-definitions-local.mjs), [`verification`](../verification/2026-10-17-v02-handoff.md) |
| V01 organization infrastructure accounts, CAS, same-target credentials and uncertain commit recovery | [`account service`](../../backend/internal/application/connection/accounts.go), [`public account API`](../../backend/internal/delivery/http/driver_accounts.go), [`PostgreSQL CAS`](../../backend/internal/adapters/postgres/registration.go), [`commit outcome`](../../backend/internal/adapters/postgres/store.go), [`Console`](../../frontend/src/features/platform/accounts/), [`API tests`](../../backend/test/e2e/driver_accounts_http_test.go), [`headed runner`](../../backend/test/integration/v01-accounts-local.sh), [`scenario`](../../frontend/test/e2e/v01-accounts-local.mjs) |
| I04-01 completed: encrypted state, role A, invalid-key fail closed, mandatory SQL validation | [`state service`](../../backend/internal/application/statebackend/service.go), [`state persistence`](../../backend/internal/adapters/postgres/tfstate.go), [`role grants`](../../backend/internal/adapters/postgres/grants.go), [`cipher`](../../backend/internal/adapters/statecipher/statecipher.go), [`internal handler`](../../backend/internal/delivery/http/terraform_state.go) |
| I04-02 immutable module registry, source policy, transitive pins and metadata-only planner | [`module domain`](../../backend/internal/domain/tfmodule/), [`registry ports`](../../backend/internal/ports/moduleregistry/), [`registry`](../../backend/internal/adapters/terraform/registry/), [`catalog validation`](../../backend/internal/application/catalog/terraform.go), [`bundle persistence`](../../backend/internal/adapters/postgres/modulebundles.go), [`planner inspector`](../../backend/internal/planning/registry.go) |
| T06 Vietnamese Connections, automatic inspect, name suggestion ownership and stale response guards | [`page`](../../frontend/src/features/platform/ConnectionsPage.tsx), [`tests`](../../frontend/src/features/platform/ConnectionsPage.test.tsx), [`headed scenario`](../../frontend/test/e2e/refactor-local.mjs), [`local runner`](../../backend/test/integration/refactor-ui-local.sh) |
| Refactor Console local smoke and Vietnamese locator foundation | [`runner`](../../backend/test/integration/refactor-ui-local.sh), [`scenario registry`](../../frontend/test/e2e/refactor-local.mjs), [`page locators`](../../frontend/test/e2e/locators.mjs), [`human input`](../../frontend/test/e2e/human.mjs) and [`captions`](../../frontend/test/e2e/captions.mjs) |
| Historical I04-06 AWS onboarding/scoped credentials/allowed regions (superseded by V01; S5 retirement staged) | [`registration`](../../backend/internal/application/connection/aws.go), [`read-only verifier`](../../backend/internal/adapters/awsaccess/verify.go), [`HTTP`](../../backend/internal/delivery/http/connections.go), [`form`](../../frontend/src/features/platform/AwsConnectionForm.tsx), [`SQL tests`](../../backend/internal/application/connection/postgres_aws_integration_test.go), [`headed runner`](../../backend/test/integration/aws-connection-local.sh), [`scenario`](../../frontend/test/e2e/aws-connection-local.mjs) |
| I04-05 AWS modules/shared network/EKS API allowlist | [`modules`](../../backend/internal/adapters/terraform/modules/), [`module tests`](../../backend/internal/adapters/terraform/awsmodules_test.go), [`shared graph`](https://github.com/pr3s3nt/poc/blob/512d0d305c2d556314ec90294d016a02fee3b29f/backend/internal/planning/awsshared_plan_test.go), [`cleanup regression`](../../backend/internal/application/tfexec/cleanup_test.go), [`egress preflight helper`](../../backend/internal/adapters/terraform/preflight/) |
| I04-04 Kubernetes modules and secret binding guards | [`modules`](../../backend/internal/adapters/terraform/modules/), [`module tests`](../../backend/internal/adapters/terraform/k8smodules_test.go), [`catalog guards`](../../backend/internal/application/catalog/terraform.go), [`planner guards`](../../backend/internal/planning/contract.go), [`runtime regression`](../../backend/internal/application/provisioning/secret_input_test.go) |
| I04-03 Terraform lifecycle/state/provider/absence | [`executor service`](../../backend/internal/application/tfexec/), [`runner`](../../backend/internal/adapters/terraform/runner/), [`providers`](../../backend/internal/adapters/terraform/providers/), [`readiness`](../../backend/internal/adapters/terraform/readiness/), [`encrypted recovery`](../../backend/internal/adapters/terraform/recovery/), [`AWS transport`](../../backend/internal/adapters/awsaccess/), [`provider graph`](../../backend/internal/planning/provider_bindings.go) |
| I04-07 Definition/class/renderer/Preview và safe progress | `Definition form` (restored by UC-03 CM-01–04), [`module mapping`](../../frontend/src/features/platform/moduleMapping.ts), [`strict metadata`](../../frontend/src/features/platform/metadata.ts), [`classes`](../../backend/internal/application/catalog/classes.go), [`Workload controls`](../../frontend/src/features/workloads/ClassControls.tsx), [`intent`](../../backend/internal/planning/intent.go), [`baseline snapshot`](../../backend/internal/application/deployment/baseline.go), [`pending consistency tests`](../../backend/internal/application/pending/snapshot_consistency_test.go), [`safe progress`](../../backend/internal/domain/deployment/deployment.go), [`SQL projection`](../../backend/internal/adapters/postgres/deployment.go), [`headed runner`](../../backend/test/integration/definition-workload-local.sh) |
| I04-08 resource detail/confirmed deletion/progress | [`admission/planner/query`](../../backend/internal/application/resourcecleanup/), [`safe SQL reads`](../../backend/internal/adapters/postgres/resources.go), [`HTTP`](../../backend/internal/delivery/http/resource_cleanup.go), [`engine`](../../backend/internal/application/tfexec/cleanup.go), [`Console`](../../frontend/src/features/resources/), [`selected-set UI rules`](../../frontend/src/features/resources/selection.ts), [`SQL/HTTP tests`](../../backend/test/e2e/resource_cleanup_http_test.go), [`headed runner`](../../backend/test/integration/resource-delete-local.sh), [`scenario`](../../frontend/test/e2e/resource-delete-local.mjs) |
| I04-09 Redis acceptance app | [`cache/config/resolver`](../../backend/examples/acceptance-app/internal/jobcache/), [`durable job/marker`](../../backend/examples/acceptance-app/internal/jobstore/), [`backend`](../../backend/examples/acceptance-app/backend/), [`worker`](../../backend/examples/acceptance-app/worker/), [`app UI`](../../backend/examples/acceptance-app/frontend/), [`real fixture`](../../backend/examples/acceptance-app/internal/testfixture/), [`fixture runner`](../../backend/test/integration/acceptance-redis-local.sh) |
| Historical I04-10 profile guard; retained cleanup/acceptance runner | [`historical profile guard`](https://github.com/pr3s3nt/poc/blob/512d0d305c2d556314ec90294d016a02fee3b29f/backend/internal/bootstrap/realprofile.go), [`durable outputs`](../../backend/internal/adapters/vault/output_secrets.go), [`original-engine cleanup`](../../backend/internal/application/runcleanup/), [`delivery removal`](../../backend/internal/application/deployment/remove_delivered.go), [`process kill-chain`](../../backend/internal/application/tfexec/killchain_test.go), [`runner`](../../backend/test/integration/terraform-v2-acceptance.sh), [`supervisor/manifest/ECR/probes`](../../backend/test/integration/v2run/), [`cleanup wiring`](../../backend/test/integration/v2cleanup/), [`headed scenarios`](../../frontend/test/e2e/terraform-v2-acceptance.mjs) |
| Process bootstrap and adapter wiring | [`backend/internal/bootstrap`](../../backend/internal/bootstrap/) |
| HTTP API and `/ui/` delivery | [`backend/internal/delivery/http`](../../backend/internal/delivery/http/) |
| UC-02..UC-04 seed/catalog baseline | [`backend/internal/seed`](../../backend/internal/seed/) and persistence adapters |
| UC-02/03 new-public-ID and strict Driver Inputs validation | [`catalog policy`](../../backend/internal/application/catalog/policy.go), [`policy tests`](../../backend/internal/application/catalog/policy_test.go) and [`HTTP policy tests`](../../backend/test/e2e/registration_policy_http_test.go) |
| UC-16 all-dependency params validation before Save/import | [`workload configuration service`](../../backend/internal/application/workloadconfig/service.go) (`validateResourceParams`), [`resource params tests`](../../backend/internal/application/workloadconfig/resource_params_test.go) and [`shared input type check`](../../backend/internal/domain/resource/contract.go) |
| UC-02..04 insert-only registration, race/rollback and truthful UI states | [`PostgreSQL registration`](../../backend/internal/adapters/postgres/registration.go), [`repository contract`](../../backend/internal/ports/persistence/persistencetest/registration.go), [`HTTP integration`](../../backend/test/e2e/uc02_04_http_test.go), `UI state tests` (restored by UC-03 CM-01–04) |
| UC-05 planning core and pending Preview → Deploy | [`backend/internal/planning`](../../backend/internal/planning/) and [`backend/internal/application/pending`](../../backend/internal/application/pending/) |
| UC-05 standalone Score Preview | [`read-only service`](../../backend/internal/application/preview/), [`shared planning snapshot`](../../backend/internal/application/deployment/snapshot.go), [`HTTP`](../../backend/internal/delivery/http/workload_preview.go) and [`Console`](../../frontend/src/features/preview/) |
| UC-05 human-paced UI recording | [`Playwright`](../../frontend/test/e2e/uc05-video-local.mjs) and [`local runner`](../../backend/test/integration/uc05-video-local.sh) |
| T13 Vietnamese standalone Score Preview | [`Console and parser messages`](../../frontend/src/features/preview/), [`T13 scenario`](../../frontend/test/e2e/refactor-local.mjs), [`refactor runner`](../../backend/test/integration/refactor-ui-local.sh); UC-05 UI screens/states own labels, parser and API semantics unchanged |
| UC-06, UC-07 and UC-09 orchestration/query | [`backend/internal/application/deployment`](../../backend/internal/application/deployment/) |
| UC-07 final resource marking and Service-reference guards | [`deployment service`](../../backend/internal/application/deployment/service.go), [`planner`](../../backend/internal/planning/plan.go), [`Service validation`](../../backend/internal/planning/service_refs.go), [`PostgreSQL atomicity tests`](../../backend/internal/application/deployment/postgres_uc07_integration_test.go) |
| UC-07 pending UI and human-paced recording | [`Application home`](../../frontend/src/features/applications/ApplicationHomePage.tsx), [`editor`](../../frontend/src/features/workloads/WorkloadEditorPage.tsx), [`Playwright`](../../frontend/test/e2e/uc07-video-local.mjs) and [`runner`](../../backend/test/integration/uc07-video-local.sh) |
| Definition-selected workload rendering | [`catalog validation`](../../backend/internal/domain/resource/rendering.go), [`planner selection`](../../backend/internal/planning/rendering.go), [`CLI adapter`](../../backend/internal/adapters/templatedriver/), [`deployment wiring`](../../backend/internal/bootstrap/bootstrap.go), [`pending comparisons`](../../backend/internal/application/pending/preview.go), `HTTP flow tests` (editor HTTP test retired 2026-11-02; see Catalog release verification) |
| ADR-024 open object set, inventory, Gateway routes | [`guard`](../../backend/internal/adapters/templatedriver/guard.go), [`resources stage`](../../backend/internal/adapters/templatedriver/git.go), [`object discovery/ownership/UID removal`](../../backend/internal/adapters/kubernetes/objects.go), [`inventory and apply order`](../../backend/internal/application/deployment/inventory.go), [`kind runner`](../../backend/test/integration/workload-manifests-kind.sh), [`browser scenario`](../../frontend/test/e2e/workload-manifests-kind.mjs) |
| V04 full reference/provision graph and safe blocked-consumer progress | [`planning graph`](../../backend/internal/planning/graph.go), [`blocked projection`](../../backend/internal/application/deployment/blocked.go), [`Console graph`](../../frontend/src/shared/ui/DependencyGraph.tsx) |
| V04 full-app acceptance, private output cleanup and completed predecessor evidence | [`integrated runner`](../../backend/test/integration/v04-internal-full.sh), [`browser scenario`](../../frontend/test/e2e/v04-internal-full.mjs), [`operator inventory`](../../backend/test/integration/v2ctl/v04.go), [`admission proof`](../../backend/test/integration/v2run/v04_admission.go) |
| V05 Definition-selected EKS, deferred cluster completion and per-resource fresh authentication | [`planner catalog contract`](../../backend/internal/planning/definition_catalog_plan_test.go), [`EKS completion`](../../backend/internal/application/provisioning/service.go), [`cluster readiness`](../../backend/internal/adapters/kubernetes/cluster_ready.go), [`restore target`](../../backend/internal/application/deployment/target_restore_test.go) |
| V05 full real app, original native cleanup and complete SQL history | [`browser scenario`](../../frontend/test/e2e/terraform-v2-aws-definitions.mjs), [`SQL/API reader`](../../frontend/test/e2e/terraform-v2-definition-reopen.mjs), [`safe SQL inventory`](../../backend/test/integration/v04run/state.go), [`owned CSI cleanup`](../../backend/test/integration/v2run/ebs.go); gate/source/reuse evidence in [handoff](../verification/2026-10-18-v040506-handoff.md) |
| V06 managed graph/account-region contracts | [`catalog contracts`](../../backend/internal/planning/definition_catalog_v06_test.go), [`managed-only graph`](../../backend/internal/planning/managed_only_graph_test.go); no new public no-workload entry |
| V06 actual A02/A03 and final profile retirement | [`AWS browser main`](../../frontend/test/e2e/terraform-v2-aws-definitions.mjs), [`durable UID manifest`](../../backend/test/integration/v2run/manifest.go), [`operator cleanup`](../../backend/test/integration/v2ctl/extra.go), [`operation APIs retained after transition removal`](../../backend/internal/delivery/http/operations.go); final47/47/native clean and source/check mapping in [handoff](../verification/2026-10-18-v040506-handoff.md) |
| UC-08 resource provisioning | [`backend/internal/application/provisioning`](../../backend/internal/application/provisioning/) |
| Application/Connection domain | [`backend/internal/domain/application`](../../backend/internal/domain/application/) |
| Environment/Deployment Set domain | [`backend/internal/domain/environment`](../../backend/internal/domain/environment/) |
| Deployment lifecycle/read model | [`backend/internal/domain/deployment`](../../backend/internal/domain/deployment/) |
| `DeploymentDeltaSnapshot`, `DeltaDocument`, `ModuleDelta`, `JSONPatchOperation` | [`backend/internal/domain/deployment/delta.go`](../../backend/internal/domain/deployment/delta.go) |
| `DeltaBuilder.BuildHumanitecDelta`, `DiffDeploymentSets`, `ApplyHumanitecDelta`, `VerifyDelta` | [`backend/internal/planning/delta.go`](../../backend/internal/planning/delta.go) |
| Deterministic relative JSON Patch diff/apply | [`backend/internal/planning/jsonpatch`](../../backend/internal/planning/jsonpatch/) |
| Resource contracts and lifecycle | [`backend/internal/domain/resource`](../../backend/internal/domain/resource/) |
| Score/delta/graph/matching/batches | [`backend/internal/planning`](../../backend/internal/planning/) |
| Profile eligibility before five-field Resource Definition matching | [`backend/internal/domain/resource/definition.go`](../../backend/internal/domain/resource/definition.go), [`backend/internal/planning/match.go`](../../backend/internal/planning/match.go), [`backend/internal/seed/seed.go`](../../backend/internal/seed/seed.go) |
| Persistence ports | [`backend/internal/ports/persistence`](../../backend/internal/ports/persistence/) |
| Execution ports | [`backend/internal/ports/execution`](../../backend/internal/ports/execution/) |
| In-memory + JSON snapshot store | [`backend/internal/adapters/store`](../../backend/internal/adapters/store/) |
| Normalized PostgreSQL store and current fresh baseline | [`backend/internal/adapters/postgres`](../../backend/internal/adapters/postgres/) |
| Fake walking-skeleton adapters | [`backend/internal/adapters/fake`](../../backend/internal/adapters/fake/) |
| Kubernetes executor/deployer | [`backend/internal/adapters/kubernetes`](../../backend/internal/adapters/kubernetes/) |
| Environment public-path validation and route reconciliation | Retired by [ADR-037](../architecture/decisions/ADR-037-retire-public-routes.md); see the endpoint resource rows. | — |
| Optional Fleet GitRepo workload delivery (kind) | [`backend/internal/adapters/gitops`](../../backend/internal/adapters/gitops/), [`deploy/kind/fleet-poc-gitrepo.yaml`](../../deploy/kind/fleet-poc-gitrepo.yaml) |
| Terraform executor/modules/inspection | [`backend/internal/adapters/terraform`](../../backend/internal/adapters/terraform/) |
| Seed catalog and profiles | [`backend/internal/seed`](../../backend/internal/seed/) |
| UC-00/UC-01 Web Console and local browser verification | [`frontend/src/app`](../../frontend/src/app/), [`features/auth`](../../frontend/src/features/auth/), [`features/applications`](../../frontend/src/features/applications/), [`frontend/test/e2e/onboarding-local.mjs`](../../frontend/test/e2e/onboarding-local.mjs) and [`backend/test/integration/onboarding-playwright-local.sh`](../../backend/test/integration/onboarding-playwright-local.sh) |
| T10 Vietnamese sign-in/shared shell verification | `frontend/src/features/auth/SignInPage.tsx`, `frontend/src/app/AppShell.tsx`, `frontend/src/app/App.tsx`, `frontend/test/e2e/refactor-local.mjs` scenario T10; [evidence](../verification/2026-10-11-T10-sign-in-shell.md) |
| T11 Vietnamese applications list verification | `frontend/src/features/applications/ApplicationsPage.tsx`, `frontend/src/app/App.test.tsx`; applications locator group, human helper và scenario T11 trong `frontend/test/e2e/`; [evidence](../verification/2026-10-11-T11-applications-list.md) |
| UC-00 authentication/session | [`backend/internal/application/authentication`](../../backend/internal/application/authentication/), [`domain/identity`](../../backend/internal/domain/identity/) and [`platform/password`](../../backend/internal/platform/password/) |
| UC-00 user management (BR-08/09/10/11) | [`backend/internal/delivery/http/users.go`](../../backend/internal/delivery/http/users.go), [`frontend/src/features/users`](../../frontend/src/features/users/), [`frontend/src/features/auth/ChangePasswordPage.tsx`](../../frontend/src/features/auth/ChangePasswordPage.tsx), show/hide [`PasswordInput`](../../frontend/src/shared/ui/PasswordInput.tsx), `-bootstrap-admin` in [`cmd/orchestrator/main.go`](../../backend/cmd/orchestrator/main.go) and [`e2e/scenarios/user-management`](../../e2e/scenarios/user-management/) |
| UC-01 self-service creation | [`backend/internal/application/application`](../../backend/internal/application/application/) and [`backend/internal/domain/application`](../../backend/internal/domain/application/) |
| UC-02 Resource Type registration API/UI and Organization-scoped catalog | [`backend/internal/application/catalog`](../../backend/internal/application/catalog/), [`backend/internal/delivery/http/resource_types.go`](../../backend/internal/delivery/http/resource_types.go), [`frontend/src/features/platform/ResourceTypesPage.tsx`](../../frontend/src/features/platform/ResourceTypesPage.tsx) and [`backend/internal/adapters/store`](../../backend/internal/adapters/store/) |
| T02 Vietnamese resource form, automatic driver/module/profile projection and READY Connection dropdowns | `resource form` (restored by UC-03 CM-01–04), `form tests` (restored by UC-03 CM-01–04), `state tests` (restored by UC-03 CM-01–04), [`T02 local UI scenario`](../../frontend/test/e2e/refactor-local.mjs) |
| T02B shared Vietnamese criteria modes, independent copy and assistive scope preview | [`CriteriaEditor`](../../frontend/src/features/platform/CriteriaEditor.tsx), [`editor tests`](../../frontend/src/features/platform/CriteriaEditor.test.tsx), `resource integration tests` (restored by UC-03 CM-01–04), [`T02B local UI scenario`](../../frontend/test/e2e/refactor-local.mjs) |
| T05 Vietnamese Secret Store form, advanced defaults, same-address draft and safe verification projection | [`page`](../../frontend/src/features/platform/SecretStoresPage.tsx), [`tests`](../../frontend/src/features/platform/SecretStoresPage.test.tsx), [`scenario`](../../frontend/test/e2e/refactor-local.mjs), [`local runner`](../../backend/test/integration/refactor-ui-local.sh) |
| T04 Vietnamese Resource Types details/copy, capability-based support (T31) and Definition prefill | [`types page`](../../frontend/src/features/platform/ResourceTypesPage.tsx), [`types tests`](../../frontend/src/features/platform/ResourceTypesPage.test.tsx), [`capability metadata`](../../frontend/src/features/platform/capabilities.ts), [`route query`](../../frontend/src/app/routes.ts), `Definition form` (restored by UC-03 CM-01–04), [`local scenario`](../../frontend/test/e2e/refactor-local.mjs) |
| T03 Vietnamese rendering templates page, PE/Admin route gate and unchanged renderer registration payload | `templates page` (restored by UC-03 CM-01–04), `page tests` (restored by UC-03 CM-01–04), [`App role gates`](../../frontend/src/app/App.test.tsx), [`local scenario`](../../frontend/test/e2e/refactor-local.mjs), [`failure stub`](../../backend/test/integration/stubs/score-k8s-render-fail-stub.sh), [`live template locator`](../../frontend/test/e2e/template-engine-kind-human.mjs) |
| UC-03 Resource Definition registration API/UI and verified Terraform registry contract inspection | [`backend/internal/application/catalog`](../../backend/internal/application/catalog/), [`backend/internal/delivery/http/resource_definitions.go`](../../backend/internal/delivery/http/resource_definitions.go), [`registry inspector`](../../backend/internal/adapters/terraform/registry/inspect.go), `frontend/src/features/platform/ResourceDefinitionsPage.tsx` (restored by UC-03 CM-01–04) |
| UC-04 upload-only kubeconfig inspect/register and fail-closed scoped execution | [`backend/internal/application/connection`](../../backend/internal/application/connection/), [`backend/internal/adapters/kubernetes/connection_verifier.go`](../../backend/internal/adapters/kubernetes/connection_verifier.go), [`backend/internal/delivery/http/connections.go`](../../backend/internal/delivery/http/connections.go), [`frontend/src/features/platform/ConnectionsPage.tsx`](../../frontend/src/features/platform/ConnectionsPage.tsx) |
| UC-04 kubeconfig normalization and safe parser errors | [`parser`](../../backend/internal/application/connection/parser.go), [`parser tests`](../../backend/internal/application/connection/parser_test.go) |
| UC-04 scoped credential persistence and rollback | [`port`](../../backend/internal/ports/credentials/credentials.go), [`Vault KV v2`](../../backend/internal/adapters/vault/connection_credentials.go), [`explicit local memory adapter`](../../backend/internal/adapters/credentialmemory/store.go) |
| UC-04 execution credential resolution and private kubectl invocation | [`resolver`](../../backend/internal/application/connection/resolver.go), [`kubectl transport`](../../backend/internal/adapters/kubernetes/kubectl.go), [`leak tests`](../../backend/internal/adapters/kubernetes/kubectl_leak_test.go), [`restart tests`](../../backend/internal/application/deployment/credential_restart_test.go) |
| UC-04 explicit Connection metadata and concurrent upload | [`fresh baseline`](../../backend/internal/adapters/postgres/baseline.sql), [`startup validation`](../../backend/internal/adapters/postgres/baseline.go), [`registration race/reopen`](../../backend/internal/application/connection/postgres_integration_test.go) |
| UC-04 human-paced upload review recording | [`runner`](../../backend/test/integration/uc04-kubeconfig-video-local.sh), [`Playwright`](../../frontend/test/e2e/uc04-kubeconfig-video-local.mjs) |
| UC-04 synthetic layout and keyboard-focus screenshots | [`Playwright screenshot helper`](../../frontend/test/e2e/uc04-connections-screenshots.mjs) |
| UC-12 desired configuration, Vault KV adapter and Settings UI | [`backend/internal/application/configuration`](../../backend/internal/application/configuration/), [`backend/internal/adapters/vault`](../../backend/internal/adapters/vault/) and [`frontend/src/features/configuration`](../../frontend/src/features/configuration/) |
| Developer kind deployment and Platform Engineer human UI recordings | [`acceptance scenario`](../../frontend/test/e2e/acceptance-kind-human.mjs), [`platform scenario`](../../frontend/test/e2e/uc02-04-video-local.mjs), [`human input`](../../frontend/test/e2e/human.mjs), [`recording helpers`](../../frontend/test/e2e/video.mjs), [`kind runner`](../../backend/test/integration/acceptance-playwright-kind.sh), [`platform runner`](../../backend/test/integration/uc02-04-video-local.sh) and [`video validation`](../../backend/test/integration/video-lib.sh) |
| UC-16 pending workload drafts and editor | [`backend/internal/application/workloadconfig`](../../backend/internal/application/workloadconfig/), [`backend/internal/domain/environment/draft.go`](../../backend/internal/domain/environment/draft.go) and [`frontend/src/features/workloads`](../../frontend/src/features/workloads/) |
| UC-16 unified environment-variable table, inline key/store/dependency dialogs | [`EnvironmentVariablesTable`](../../frontend/src/features/workloads/EnvironmentVariablesTable.tsx), [`bindings`](../../frontend/src/features/workloads/bindings.ts), [`CreateKeyDialog`](../../frontend/src/features/workloads/CreateKeyDialog.tsx), [`AddResourceDialog`](../../frontend/src/features/workloads/AddResourceDialog.tsx), [`DependencyFields`](../../frontend/src/features/workloads/DependencyFields.tsx), [`Dialog`](../../frontend/src/shared/ui/Dialog.tsx) |
| Create-only configuration key (OC-12) | [`CreateKey`](../../backend/internal/application/configuration/service.go), [`POST handler`](../../backend/internal/delivery/http/configuration.go), [`createKey client`](../../frontend/src/features/configuration/api.ts) |
| Ordinary-value provenance (UC-16 BR-23) | [`literal.go`](../../backend/internal/application/workloadconfig/literal.go), [`literal_proof.go`](../../backend/internal/domain/environment/literal_proof.go), deployer [`service.go`](../../backend/internal/application/deployment/service.go), jsonb member in [`repositories.go`](../../backend/internal/adapters/postgres/repositories.go) |
| UC-16 Resource Type input form and Score params mapping | [`frontend/src/features/workloads/WorkloadEditorPage.tsx`](../../frontend/src/features/workloads/WorkloadEditorPage.tsx) and [`backend/internal/domain/resource/contract.go`](../../backend/internal/domain/resource/contract.go) |
| UC-16 inline storage resource and mount entry | [`ExecutionFields`](../../frontend/src/features/workloads/ExecutionFields.tsx), [`WorkloadEditorPage`](../../frontend/src/features/workloads/WorkloadEditorPage.tsx), shared [`AddResourceDialog`](../../frontend/src/features/workloads/AddResourceDialog.tsx); [`regressions`](../../frontend/src/features/workloads/WorkloadStorageMount.test.tsx) |
| Acceptance workload PVC file and scheduled receipts | [`volume`](../../backend/examples/acceptance-app/internal/volume/volume.go), [`scheduled`](../../backend/examples/acceptance-app/scheduled/main.go), [`receipt store`](../../backend/examples/acceptance-app/internal/jobstore/scheduled.go), [`app UI`](../../backend/examples/acceptance-app/frontend/lifecycle.go), [`browser observations`](../../e2e/scenarios/acceptance-storage-schedule/README.md); [procedure](../operations/acceptance-storage-schedule.md) |
| Deployed Score reconstruction (references plus proven ordinary values) | [`backend/internal/application/workloadconfig/reconstruct.go`](../../backend/internal/application/workloadconfig/reconstruct.go) |
| UC-12 Vault bundle, VSO synchronization, `secretKeyRef` rendering and legacy Agent mode | [`backend/internal/adapters/vault/provider.go`](../../backend/internal/adapters/vault/provider.go), [`backend/internal/adapters/kubernetes/vso.go`](../../backend/internal/adapters/kubernetes/vso.go) and [`backend/internal/adapters/kubernetes/renderer.go`](../../backend/internal/adapters/kubernetes/renderer.go) |
| Application Preview/Deploy UI | [`frontend/src/features/applications/ApplicationHomePage.tsx`](../../frontend/src/features/applications/ApplicationHomePage.tsx) |
| UC-09 recent deployments, Environment history/filter and detail UI | [`frontend/src/features/deployments`](../../frontend/src/features/deployments/) |
| T15 Vietnamese persisted deployment details, safe failure/redaction and scoped late-response guards | [`detail page`](../../frontend/src/features/deployments/DeploymentDetailsPage.tsx), [`state tests`](../../frontend/src/features/deployments/DeploymentPages.test.tsx), [`T15 scenario`](../../frontend/test/e2e/refactor-local.mjs); [verification](../verification/2026-10-11-T15-deployment-details.md) |
| UC-09 consistent scoped query, workload history and local browser verification | [`query.go`](../../backend/internal/application/deployment/query.go), [`persistence ports`](../../backend/internal/ports/persistence/persistence.go), [`adapter contract tests`](../../backend/internal/ports/persistence/persistencetest/), [`uc09-local.mjs`](../../frontend/test/e2e/uc09-local.mjs) and [`local runner`](../../backend/test/integration/uc09-playwright-local.sh) |
| Acceptance workloads | [`backend/examples/acceptance-app`](../../backend/examples/acceptance-app/) |
| Local Compose Vault bootstrap and explicit platform store seed | [`docker-compose.yml`](../../docker-compose.yml), [`start-vault.sh`](../../deploy/local/start-vault.sh), [`application policy`](../../deploy/local/applications-policy.hcl), [`connection policy`](../../deploy/local/connections-policy.hcl) and [`normal-store bootstrap`](../../backend/internal/bootstrap/platformvault.go), [`shared managed registration`](../../backend/internal/application/secretstores/bootstrap.go) and [`scoped runtime registry`](../../backend/internal/adapters/vault/registry.go) |
| Orchestrator container images (API with kubectl, Web Console on nginx) | [`backend/Dockerfile`](../../backend/Dockerfile), [`backend/deploy/docker-entrypoint.sh`](../../backend/deploy/docker-entrypoint.sh), [`frontend/Dockerfile`](../../frontend/Dockerfile) and [`frontend/deploy/nginx.conf.template`](../../frontend/deploy/nginx.conf.template) |
| Offline Terraform provider installation | [`release-verified mirror builder`](../../backend/cmd/prepare-provider-mirror/main.go), [`image CLI policy`](../../backend/deploy/terraform-offline.tfrc), [`bootstrap configuration`](../../backend/internal/bootstrap/terraform.go), [`network-isolated image test`](../../backend/internal/adapters/terraform/runner/offline_provider_test.go), [`offline provider runbook`](../operations/terraform-offline.md) |
| HTTP end-to-end tests | [`backend/test/e2e`](../../backend/test/e2e/) |
| Planner challenge conformance | [`backend/test/conformance`](../../backend/test/conformance/) |
| kind/AWS verification | [`backend/test/integration`](../../backend/test/integration/) |

`DeploymentDeltaSnapshotRepository` được hiện thực bởi
`DeltaSnapshotRepository` trong persistence ports và `SaveDeltaSnapshot`/
`GetDeltaSnapshot` của JSON snapshot store; `DeploymentService` lưu Snapshot
trong transaction A và `QueryService` expose `delta`/`deltaDocumentHash` cho
UC-09.

Container resource requests/limits (UC-05 BR-07, UC-06 BR-11):

- `ContainerResourceRequirements`/`ComputeResources` nằm trong
  [`backend/internal/domain/environment/document.go`](../../backend/internal/domain/environment/document.go)
  dưới `Container.Resources`.
- Score shape validation nằm tại
  [`backend/internal/planning/workloadspec/resources.go`](../../backend/internal/planning/workloadspec/resources.go);
  `Document.Fragment` copy requirements vào module.
- Kubernetes mapping và request policy (request > limit > default) nằm tại `containerResources`
  trong [`backend/internal/adapters/kubernetes/renderer.go`](../../backend/internal/adapters/kubernetes/renderer.go).
- Seeded acceptance Scores tại
  [`backend/internal/seed/scores.go`](../../backend/internal/seed/scores.go)
  bao phủ full (backend), partial (worker) và omitted (frontend).

Nếu path/module thay đổi, cập nhật map này cùng imports, build tooling, runbook và
links trong cùng logical change.

## Connection selection implementation paths (2026-10-07)

- [Application creation and safe choices](../../backend/internal/application/application/service.go),
  [HTTP views/endpoints](../../backend/internal/delivery/http/server.go),
  [Create UI](../../frontend/src/features/applications/CreateApplicationPage.tsx),
  [target label](../../frontend/src/shared/ui/ApplicationTarget.tsx).
- [Shared binding predicate and winning-match guard](../../backend/internal/planning/match.go),
  [executor guard](../../backend/internal/application/provisioning/service.go),
  [selected target execution tests](../../backend/internal/application/deployment/selected_target_test.go),
  [HTTP contract tests](../../backend/test/e2e/application_connection_http_test.go).
- [Local Playwright runner](../../backend/test/integration/application-connection-playwright-local.sh),
  [browser flow](../../frontend/test/e2e/application-connection-local.mjs).

- [UC-01/04 live kind recording runner](../../backend/test/integration/application-connection-kind-video.sh)
  and [human UI scenario](../../frontend/test/e2e/application-connection-kind-human.mjs)
  cover uploaded Connection selection with real executors and diagnostics;
  [live evidence](../verification/2026-10-07-application-connection-selection-kind.md).

Historical Environment Settings set-once delivery follows
[ADR-011](../architecture/decisions/ADR-011-environment-execution-binding.md);
resolver/service and migration paths are mapped below.
Prior Application-selection evidence and Internal Settings runners are historical;
V03.10 retires that runtime route. Use the V03 route above.

## Historical Environment binding implementation (ADR-011)

- [Domain](../../backend/internal/domain/environment/environment.go), [binding service](../../backend/internal/application/application/service.go), [target resolver](../../backend/internal/application/target/target.go).
- [PostgreSQL fresh baseline](../../backend/internal/adapters/postgres/baseline.sql), [SQL binding](../../backend/internal/adapters/postgres/repositories.go), [JSON binding/load](../../backend/internal/adapters/store/store.go), [historical binding contract](https://github.com/pr3s3nt/poc/blob/512d0d305c2d556314ec90294d016a02fee3b29f/backend/internal/ports/persistence/persistencetest/binding.go); surviving claims/generation/store contract: [operations](../../backend/internal/ports/persistence/persistencetest/operations.go).
- [Settings page (Connection selector removed in V03.10)](../../frontend/src/features/configuration/SettingsPage.tsx), [Settings routes](../../frontend/src/app/routes.ts), [new AWS scope tests](../../backend/internal/planning/environment_scope_test.go).
- [Live runner](../../backend/test/integration/application-connection-kind-video.sh), [human scenario](../../frontend/test/e2e/application-connection-kind-human.mjs), [reviewed evidence/video](../verification/2026-10-07-environment-connection-kind.md).


## Environment stores and historical Connection transitions (ADR-012)

Connection-transition service/UI/HTTP/observer links below are historical baseline
references; V06.08 removed them after A02/A03 replacement passed. Secret Store selection and operation
fencing remain current. This is code navigation. Current limits remain in CURRENT_STATE; first-delivery
live evidence is in the [reviewed recording](../verification/2026-10-08-environment-stores-kind.md).

- [Secret Store domain](../../backend/internal/domain/secretstore/), [registration service](../../backend/internal/application/secretstores/), [Vault registry/verifier](../../backend/internal/adapters/vault/), [registration UI](../../frontend/src/features/platform/SecretStoresPage.tsx).
- [Configuration and copy-first store switch](../../backend/internal/application/configuration/), [store selection UI](../../frontend/src/features/environment/SecretStoreSelection.tsx), [VSO delivery](../../backend/internal/adapters/kubernetes/vso.go).
- [Operation manager and fencing tests](../../backend/internal/application/envops/), [safe heartbeat loss diagnostics regressions](../../backend/internal/application/envops/heartbeat_diagnostics_test.go), [SQL operations](../../backend/internal/adapters/postgres/operations.go), [JSON operations](../../backend/internal/adapters/store/ops.go), [persistence contracts](../../backend/internal/ports/persistence/persistence.go).
- [Transition service/tests](https://github.com/pr3s3nt/poc/tree/512d0d305c2d556314ec90294d016a02fee3b29f/backend/internal/application/transition/), [historical Kubernetes transition adapter (removed in V06.08)](https://github.com/pr3s3nt/poc/blob/c6dad76dd10a1edb27b5c9c88773646751665ab1/backend/internal/adapters/kubernetes/transition.go), [HTTP routes](https://github.com/pr3s3nt/poc/blob/512d0d305c2d556314ec90294d016a02fee3b29f/backend/internal/delivery/http/transitions.go).
- [Transition UI](https://github.com/pr3s3nt/poc/blob/512d0d305c2d556314ec90294d016a02fee3b29f/frontend/src/features/environment/TransitionPanel.tsx), [operation/recovery UI](../../frontend/src/features/environment/OperationBanner.tsx), [frontend transition tests](../../frontend/src/features/environment/environment.test.tsx).
- [Generation identity tests](../../backend/internal/planning/generation_test.go), [namespace generation tests](../../backend/internal/domain/environment/generation_test.go), [Terraform v2 executor](../../backend/internal/application/tfexec/service.go). Executor cũ xem `git show 654e3d6:backend/internal/adapters/terraform/executor.go`.
- [Human real-kind recorder](../../backend/test/integration/environment-stores-kind-video.sh), [browser scenario](../../frontend/test/e2e/environment-stores-kind-human.mjs), [ownership identity observer](https://github.com/pr3s3nt/poc/blob/512d0d305c2d556314ec90294d016a02fee3b29f/backend/test/integration/runidentity/main.go), [masked-input leak checks](../../frontend/test/e2e/secret-scan.mjs).

## Implicit internal cluster (ADR-013)

Historical implementation, superseded by ADR-016 and removed in V03.10. For the
old source use `git show 7a00e43:backend/internal/planning/builtin.go`,
`backend/internal/application/provisioning/builtin.go` and their tests at that
commit. The old Environment Connection selector and implicit-cluster admission
are absent from the current runtime. Reference-only planning remains isolated
in [conformance loading](../../backend/test/conformance/loader.go).

[Historical local evidence](../verification/2026-10-09-implicit-existing-cluster-local.md)
and [historical real-kind evidence](../verification/2026-10-09-k8s4f-live.md)
retain their original scope. Internal Connection runners now fail fast through
`backend/test/integration/retired-runner.sh`; use the V03 acceptance runner and
coverage map above. Retained dev fixtures are not permission to deploy or delete
resources and are not current acceptance evidence.

## T07 — Environment Settings UI

[Environment Settings verification](../verification/2026-10-11-T07-environment-settings.md)
historically traced ENV-01 and VI-02/04/05. The Connection selector was removed
in V03.10; its old source is available at `7a00e43`. Store selection remains in
[SecretStoreSelection](../../frontend/src/features/environment/SecretStoreSelection.tsx).
[SettingsPage](../../frontend/src/features/configuration/SettingsPage.tsx) retains
open editors on refresh and scopes late replies; environment labels, transition
and operation components provide Vietnamese static/accessibility text.
[Local scenario T07](../../frontend/test/e2e/refactor-local.mjs) uses memory
state/fake adapters with explicitly marked projection mocks; it proves no live
migration. Backend error mapping is documented in T20; explicit rename-reference rewrite is mapped in T30 below.

## T09 — Tạo ứng dụng UI

[CreateApplicationPage](../../frontend/src/features/applications/CreateApplicationPage.tsx)
và tests sở hữu slug suggestion, manual-edit guard, validation/error tiếng Việt
và contract name/subdomain. [Scenario T09](../../frontend/test/e2e/refactor-local.mjs)
kiểm UI creation/duplicates qua backend fake; async 500 là projection mock.
[Verification](../verification/2026-10-11-T09-create-application.md) ghi evidence
và giới hạn; human helper và runners cũ dùng label mới, replace subdomain gợi ý.

## T08 — Workload editor UI

[Verification](../verification/2026-10-11-T08-workload-editor.md) traces WL-01/02
and VI-02/04/05. [Editor](../../frontend/src/features/workloads/WorkloadEditorPage.tsx)
owns advanced toggles, effective summaries and explicit Add-route unique-port
selection/invalid-target retention. `ApplicationKeyPicker` (removed 2026-10-24)
and [bindings](../../frontend/src/features/workloads/bindings.ts) supply Vietnamese
labels/validation with reference payloads unchanged. [Editor tests](../../frontend/src/features/workloads/WorkloadEditorPage.test.tsx)
and [table regressions](../../frontend/src/features/workloads/EnvironmentVariablesTable.test.tsx)
cover typed params, overrides, aliases and stale/BUSY/scope.
[T08 scenario](../../frontend/test/e2e/refactor-local.mjs) uses fake backend with
marked Secret-catalog/stale/BUSY mocks; [human helper](../../frontend/test/e2e/human.mjs)
and [locators](../../frontend/test/e2e/locators.mjs) follow the translated controls.

## T12 — Trang ứng dụng UI

[ApplicationHomePage](../../frontend/src/features/applications/ApplicationHomePage.tsx)
và tests giữ pending Preview/Deploy, confirmations/Undo, safe Vietnamese errors
và stale/scope guards. [RecentDeployments](../../frontend/src/features/deployments/RecentDeployments.tsx)
và DeploymentPages tests kiểm nhãn/status/order/link/retry và obsolete replies.
[ApplicationTarget](../../frontend/src/shared/ui/ApplicationTarget.tsx) dịch safe
target summary dùng chung; shared human helper và e2e selectors theo nhãn mới.
[Scenario T12](../../frontend/test/e2e/refactor-local.mjs) có native confirmation
visible và fake/projection boundaries trong [verification](../verification/2026-10-11-T12-application-home.md).

## T16 — Terraform Environment account inheritance

UC-03 BR-08 → [catalog registration](../../backend/internal/application/catalog/service.go),
[registration tests](../../backend/internal/application/catalog/registration_test.go),
HTTP tests (editor HTTP test retired 2026-11-02; see Catalog release verification) và
[new product seeds](../../backend/internal/seed/seed.go).
UC-06 BR-20/UC-08 BR-07 → unchanged matcher guard,
[connection tests](../../backend/internal/planning/connection_binding_test.go),
[effective account/prior-state validation](../../backend/internal/application/provisioning/service.go)
và [provision tests](../../backend/internal/application/provisioning/service_test.go).
Console form (restored by UC-03 CM-01–04)
giữ Aurora selector và luôn gửi empty VPC/EKS key;
[scenario/runner](../../frontend/test/e2e/refactor-local.mjs) và
[local shell](../../backend/test/integration/refactor-ui-local.sh) kiểm headed fake.
[Evidence](../verification/2026-10-11-T16-environment-aws-account.md) ghi giới hạn SQL/live.

T17 public registration rejection is in the catalog service linked above, with
[service regression tests](../../backend/internal/application/catalog/registration_test.go)
and [HTTP regressions](../../backend/test/e2e/uc02_04_http_test.go).
See [local evidence](../verification/2026-10-11-T17-existing-cluster-registration.md).

## T18 installed bundle discovery and selector

- [Catalog safe query](../../backend/internal/application/catalog/service.go)
  uses [shared availability predicate](../../backend/internal/domain/resource/rendering.go).
- [HTTP endpoint](../../backend/internal/delivery/http/resource_definitions.go)
  and [permissions/safety/registration tests](../../backend/internal/delivery/http/render_bundles_test.go).
- Console selector (restored by UC-03 CM-01–04)
  and unit guards (restored by UC-03 CM-01–04).
- [Headed T18/T03 scenarios](../../frontend/test/e2e/refactor-local.mjs),
  [fake runner](../../backend/test/integration/refactor-ui-local.sh),
  [local evidence](../verification/2026-10-11-T18-render-bundles.md).

## T19 single-context matching preview

- [Catalog read service](../../backend/internal/application/catalog/matching_preview.go)
  và [tests](../../backend/internal/application/catalog/matching_preview_test.go):
  one scoped ReadSnapshot, in-memory candidate replacement, domain weights và
  winning ConnectionMismatch guard, deterministic snapshot marker; no writes.
- [HTTP route](../../backend/internal/delivery/http/resource_definitions.go),
  safe boundary tests (restored by UC-03 CM-01–04).
- [Shared panel](../../frontend/src/features/platform/MatchingPreviewPanel.tsx),
  panel/integration tests (restored by UC-03 CM-01–04),
  [Environment metadata fingerprint](../../frontend/src/features/platform/environmentSnapshot.ts)
  trong CriteriaEditor/resource/template forms.
- [Headed T19 scenario](../../frontend/test/e2e/refactor-local.mjs); existing
  local fake runner, no retained DB/cluster/cloud mutation.

## T20 stable HTTP errors and Vietnamese presentation

- [HTTP classifier/envelope](../../backend/internal/delivery/http/apierror.go)
  và [code/status/secret safety tests](../../backend/internal/delivery/http/apierror_test.go)
  dùng typed errors trong catalog/connection/preview/workloadconfig/Score;
  all Console handlers share the boundary, no raw English prefix classifier.
- [Client](../../frontend/src/shared/api/client.ts),
  [Vietnamese mappings and metadata allowlists](../../frontend/src/shared/api/errors.ts),
  [client safety/retry tests](../../frontend/src/shared/api/client.test.ts).
- Settings/EnvironmentConnection/SecretStoreSelection/WorkloadEditor/ApplicationHome
  phân biệt BUSY/stale và giữ scope/edits; stale reload failure gates có tests.
- [Headed T20 scenario](../../frontend/test/e2e/refactor-local.mjs) và
  [fake runner](../../backend/test/integration/refactor-ui-local.sh) kiểm real
  local HTTP errors và labelled projection mocks; [evidence](../verification/2026-10-12-T20-console-errors.md).

## T21 — Environment-only profile/region context

[`Context.Values/ResolveContext`](../../backend/internal/planning/context.go)
provides canonical Environment target values and rejects removed Application
aliases. [`context regression tests`](../../backend/internal/planning/environment_scope_test.go)
cover unknown resolution/placeholder expansion, conflicting old Application
fields and independent Environment values. No conformance adapter is needed:
reference fixtures do not use these aliases.

## T24 — Metadata Variables and immutable Secret ownership

- [Configuration domain](../../backend/internal/domain/configuration/configuration.go)
  validates mutually exclusive Variable Value and Secret StoreKey/ValueRef.
- [Configuration service/tests](../../backend/internal/application/configuration/)
  own store-free Variable CRUD, Secret-only store copy and legacy-format rejection.
- [JSON revision repository](../../backend/internal/adapters/store/store.go) and
  [SQL revision repository](../../backend/internal/adapters/postgres/configuration.go)
  validate reads and writes; no Variable-ref conversion is supported.
- [HTTP error mapping](../../backend/internal/delivery/http/apierror.go) and
  [Console errors](../../frontend/src/shared/api/errors.ts) drop the obsolete
  LEGACY_VALUE_UNREADABLE code. Deployment/transition test Secret kind directly.

## T25 — Current public routing input

UC-16 BR-12/UC-06 routing use only `publicRoutes`.
[`Service` decoder](../../backend/internal/domain/environment/document.go)
rejects the removed alias in Score and Deployment Set JSON;
[`Score tests`](../../backend/internal/planning/workloadspec/score_test.go),
[`domain tests`](../../backend/internal/domain/environment/service_test.go) and
[`HTTP no-write tests`](../../backend/test/e2e/public_port_removed_test.go)
cover rejection and strict nested fields. The
[`editor`](../../frontend/src/features/workloads/WorkloadEditorPage.tsx) never
converts old drafts; the existing route manager and pending retry retain their
ownership. [Local verification](../verification/2026-10-12-T25-public-routes.md).

## T26 — Identity-only Application persistence

- `backend/internal/domain/application/application.go` defines Application identity
  and current metadata only; `application/application/service.go` creates identity
  and two unconfigured Environments.
- `adapters/store/store.go` owns identity Save/Get/List, retired-target startup
  rejection and independent Environment defaults; no Application target backfill.
- `adapters/postgres/repositories.go` reads/writes only Application metadata;
  `postgres/baseline.go` rejects old/unmarked schemas; `baseline.sql` has only current
  Application columns. T28 removes the transitional migration 9 implementation.
- Domain serialization/service tests, JSON `load_test.go`, shared
  `persistencetest/operations.go` and `workloads.go` cover metadata, generation independence
  and historical target preservation. PostgreSQL fresh/reopen/rejection tests are
  unexecuted at the T26 checkpoint; current baseline SQL checks pass in T28 below.
  See [T26 evidence](../verification/2026-10-12-T26-application-metadata.md).

## T27 — Environment-only infrastructure binding scope

- [Environment domain](../../backend/internal/domain/environment/environment.go)
  accepts ENVIRONMENT only, rejects explicit invalid scopes without normalization,
  and retains domain defaults for fresh writes; T28 requires explicit persisted metadata.
- [Planner context](../../backend/internal/planning/context.go) resolves @infra and
  provider names to Environment/generation identity only; [reference expansion](../../backend/internal/planning/expand.go)
  rejects hardcoded Application VPC/EKS references. Matching weights/ties remain unchanged.
- [Seed](../../backend/internal/seed/seed.go) uses explicit ENVIRONMENT bindings
  and inserts missing Definitions only; exact old templates are never upgraded.
- JSON startup/Save/Bind and SQL Save/Bind enforce scope admission; the T28 fresh
  baseline validates the ENVIRONMENT-only CHECK without converting retained state.
  Transitional migration 6/10 implementations are removed.
- [Scope tests](../../backend/internal/domain/environment/scope_test.go),
  planner environment_scope_test.go, seed binding_test.go and shared persistence
  binding contracts cover safe rejection/no writes, current generation-0 identity,
  later-generation isolation and seed preservation. SQL opt-in was skipped at T27;
  the current baseline is verified by T28 below.
- [Conformance loader](../../backend/test/conformance/loader.go) supplies a synthetic
  ENVIRONMENT binding; fixtures need no path/name adapter and remain read-only.

Settings removes the retired binding label; frontend gates and headed fake T07
regression pass. See [T27 verification](../verification/2026-10-12-T27-environment-scope.md).

## T28 — Fresh persistence baseline (LEG-08)

- [`postgres/baseline.sql`](../../backend/internal/adapters/postgres/baseline.sql)
  creates the complete current schema only from an empty database;
  [`baseline.go`](../../backend/internal/adapters/postgres/baseline.go) serializes
  initialization and validates baseline marker and structural catalog on reopen.
  It replaces the old `migration.sql` and startup migration chain entirely.
- [`store/load.go`](../../backend/internal/adapters/store/load.go) reads only the
  complete current JSON format, without map/binding defaults, Connection
  inference or workload-history reconstruction. Safe rejection leaves the file
  untouched. Current domain/seed writes provide explicit Connection metadata.
- [`postgres-baseline-local.sh`](../../backend/test/integration/postgres-baseline-local.sh)
  prepares isolated ephemeral PostgreSQL verification, per-step statuses,
  synthetic dump/restore and owned-resource cleanup. Its execution requires the
  separate disposable-database approval described in the
  [Compose runbook](../operations/docker-local.md#t28-fresh-baseline-and-isolated-postgresql-verification).
- Current binding/registration/operations/multi-backend/history persistence
  suites remain required; historical migration/backfill tests are replaced by
  fresh/reopen/drift/reset-rejection coverage. No browser flow is added.

Retained dev IDP/K8S-4F backup/reset/rebuild belongs to T29; see its
[review gate](../operations/k8s4f-dev.md#t29-backupreset-gate).

T28 authorized isolated PostgreSQL full suite and targeted races pass, including
fresh/reopen/rollback, current constraints and multi-backend fencing. Synthetic
backup/restore and cleanup pass; failed first-run fixture checks are retained.
See [T28 verification](../verification/2026-10-12-T28-fresh-baseline.md).


## T29 — Scoped dev backup/reset runner

- [`k8s4f-dev-reset.sh`](../../backend/test/integration/k8s4f-dev-reset.sh)
  prepares the fixed run manifest, pins source images/PVC identities, quiesces
  named writers, privately dumps IDP and both workload DBs, verifies isolated
  restore/counts/checksums before reset, and invokes one real UI deployment.
  Default dry-run is offline; apply/recovery need the approved run ID. Recovery
  is separate, restores historical IDP images/configuration, and keeps PVCs.
- [`k8s4f-dev-playwright.sh`](../../backend/test/integration/k8s4f-dev-playwright.sh)
  uses fresh owner-only evidence, separate run/image IDs and private per-run
  supervisor handles. [`k8s4f-dev-human.mjs`](../../frontend/test/e2e/k8s4f-dev-human.mjs)
  tolerates a clean catalog, retains pre-existing metadata assertions and checks
  new app identity/empty workload DB before submitting the first diagnostic job.
- [`readystate/main.go`](../../backend/test/integration/readystate/main.go)
  supplies the owning Organization before creating fake Connections, satisfying
  T28 snapshot integrity for local regression without weakening product validation.

See [scoped run procedure](../operations/k8s4f-dev.md#t29-data-decision-and-concrete-run-preparation)
and [T29 evidence](../verification/2026-10-12-T29-dev-rebuild.md) for execution
status, authorized scope and actual backup/rebuild results. Offline tests are
preparation evidence only; live success is not implied by these code paths.

## T30 — Rename key with pending reference update

[Configuration service](../../backend/internal/application/configuration/service.go)
reads snapshot guards and prepares the atomic rename. The
[persistence command](../../backend/internal/ports/persistence/persistence.go)
is implemented in [memory/JSON store](../../backend/internal/adapters/store/store.go)
and [PostgreSQL](../../backend/internal/adapters/postgres/configuration.go).
[Reference rewrite](../../backend/internal/application/workloadconfig/rewrite.go)
and [verified reconstruction](../../backend/internal/application/workloadconfig/reconstruct.go)
keep aliases, other references and advanced fields lossless.

[Strict PATCH](../../backend/internal/delivery/http/configuration.go),
[Settings UI](../../frontend/src/features/configuration/SettingsPage.tsx),
[API client](../../frontend/src/features/configuration/api.ts) and
[rename UI tests](../../frontend/src/features/configuration/SettingsRename.test.tsx)
implement unchecked opt-in, CAS/error retention and correct app/env handoff.
[Routes](../../frontend/src/app/routes.ts) and
[Application home](../../frontend/src/features/applications/ApplicationHomePage.tsx)
preserve production for the new Preview CTA. The existing
[headed local runner](../../backend/test/integration/refactor-ui-local.sh)
dispatches T30 in [scenarios](../../frontend/test/e2e/refactor-local.mjs) with
fake adapters and memory credentials/state; PostgreSQL test execution remains
an explicit disposable gate, separate from this browser proof.

## T31 — Authoritative capability metadata and typed Definition form

[Catalog capability projection](../../backend/internal/application/catalog/capabilities.go)
uses registration [policy](../../backend/internal/application/catalog/policy.go)
supported pairs, embedded Terraform inspector types/defaults and scoped Resource
Type param sources. [HTTP controller](../../backend/internal/delivery/http/resource_definitions.go)
role-gates the read-only OC-04d endpoint; it exposes no editable executor-owned
variables, credentials or internal source paths. Registration/planner stay
authoritative; namespace param source respects required-string BR-12.

[Metadata client](../../frontend/src/features/platform/capabilities.ts) feeds
[Resource Types](../../frontend/src/features/platform/ResourceTypesPage.tsx)
support/CTA and Definition form (restored by UC-03 CM-01–04).
[Typed controls](../../frontend/src/features/platform/ParameterSources.tsx),
[variable conversion](../../frontend/src/features/platform/typedVariables.ts),
[provision editor](../../frontend/src/features/platform/ProvisionEditor.tsx) and
[provision conversion](../../frontend/src/features/platform/typedProvision.ts)
keep omitted defaults/params, explicit presets, per-pair drafts and lossless
advanced JSON. Runtime dependencies, executor behavior and matcher are unchanged.

Catalog/HTTP/e2e capability tests verify safe/auth/scoped reads and registration
guidance; TypedDefinitionForm/typedConversion tests cover typed values, mode and
draft preservation. [Headed scenarios](../../frontend/test/e2e/refactor-local.mjs)
include T31 forms and explicit read-only Score Preview graph/batch assertions;
T02/T16 JSON locators use the new explicit advanced mode.

Execution evidence: [T31 local verification](../verification/2026-10-12-T31-typed-definitions.md).

## Terraform v2 implementation route

[ADR-014](../architecture/decisions/ADR-014-terraform-v2-lifecycle.md) và
[IMP-021](deviations.md#imp-021-terraform-v2-designimplementation-gap) phân biệt
scope thiết kế với code/evidence. Các paths I04-01..10 phía trên đã hiện thực
state/locks/registry/executor/modules/Connections/Console deletion, Redis app
và runner. [I04-10 local verification](../verification/2026-10-14-i04-10-integration.md)
chỉ chứng minh local checks; I04-11/12 đã bổ sung năm kịch bản live,
cleanup/SQL/video trong [biên bản cuối](../verification/2026-10-16-m04-completion.md).
D12 automatic recovery, D13 orphan reconciliation và D14 backup/restore còn
hoãn, không coi safety net của owned acceptance run là hoàn tất các hạng mục đó.

I04-11 cleanup fixes `6bffefa`/`ea101e0`: [Kubernetes UID deletion](../../backend/internal/adapters/kubernetes/kubectl.go),
[renderer ownership/removal](../../backend/internal/adapters/kubernetes/deployer.go),
[owner preservation](../../backend/internal/application/deployment/service.go),
[namespace immutable baseline/absence guard](../../backend/internal/application/tfexec/namespace.go),
[namespace inspection](../../backend/internal/adapters/terraform/readiness/inspect.go),
[layered physical verification](../../backend/test/integration/v2run/k8s.go),
[headed native confirmation](../../frontend/test/e2e/terraform-v2-acceptance.mjs).
Local checks1394Go/race/0skip/622frontend đạt; live acceptance vẫn đang I04-11.

AWS preflight coverage `3136dcc`/`6d9fb2f`: shared [complete action accessor](../../backend/internal/adapters/awsaccess/verify.go)
and [runner action/chunk/result checks](../../backend/test/integration/v2run/aws.go),
[regression tests](../../backend/test/integration/v2run/aws_test.go). Fullchecks
1395Go/race/0skip/622frontend đạt; AWS live vẫn gated. Provider schema runtime
preflight/root ngắn là cấu hình vận hành theo acceptance runbook, không đổi
namespace/module source pin.


Historical runtime evidencefix34a9dcc (current restart uses Definition-aware verifier): [scoped restart verifier](https://github.com/pr3s3nt/poc/blob/512d0d305c2d556314ec90294d016a02fee3b29f/frontend/test/e2e/terraform-v2-reopen.mjs),
[HTTP regression tests](https://github.com/pr3s3nt/poc/blob/512d0d305c2d556314ec90294d016a02fee3b29f/frontend/test/e2e/terraform-v2-reopen.test.mjs),
[overall completion receipt](../../backend/test/integration/terraform-v2-acceptance.sh),
[AWS gate](../../backend/test/integration/v2run/aws.go),
[explicit closure guard](../../backend/test/integration/v2run/scan.go).
1396Go/race/0skip/632frontend vàfullchecks đạt; liveacceptance còn riêng.


I04-12 runtime checks: [đọc đầu vào sau cổng supervisor](../../frontend/test/e2e/terraform-v2-lib.mjs)
và [luồng browser](../../frontend/test/e2e/terraform-v2-acceptance.mjs),
[hồi quy file thật](../../frontend/test/e2e/terraform-v2-input-gate.test.mjs),
[chẩn đoán Preview AWS](../../frontend/test/e2e/terraform-v2-aws.mjs) và
[hồi quy ownership/purity](../../frontend/test/e2e/terraform-v2-aws-purity.test.mjs).
Các bản sửa này chỉ củng cố runner, không thay bằng chứng chạy AWS thật;
xem [I04-12 verification](../verification/2026-10-15-i04-12-aws.md).


I04-12 EKS workload credential fix `3c7e986`: [scoped EKS source](../../backend/internal/adapters/kubernetes/eks_credentials.go),
[bootstrap wiring](../../backend/internal/bootstrap/adapters.go),
[opaque target persistence/restore](../../backend/internal/application/deployment/service.go),
[lifecycle regression](../../backend/internal/bootstrap/eks_target_credentials_test.go).
[Absence verifier](../../backend/test/integration/v2run/aws.go) và
[stale-index tests](../../backend/test/integration/v2run/aws_stale_index_test.go)
chỉ chấp nhận typed native NotFound của subnet/SG đã ghi nhận đúng ownership.

I04-12 cleanup/CLI fix `f918a76`: [reverse-DAG leaf ordering](../../backend/internal/application/tfexec/cleanup.go),
[priority/dependency regression](../../backend/internal/application/tfexec/cleanup_order_test.go),
[real Terraform/PostgreSQL cleanup integration](../../backend/internal/application/tfexec/cleanup_order_integration_test.go),
[AWS CLI stderr regression qua ExecAWS](../../backend/test/integration/v2run/aws_cli_error_format_test.go).

Main-flow V04 acceptance uses [shared54assertions](../../backend/test/integration/v2run/testdata/v04-required-assertions.json),
[receipt admission](../../backend/test/integration/v2run/v04_admission.go),
[headed scenario](../../frontend/test/e2e/v04-internal-full.mjs) and
[read-only reopen](../../frontend/test/e2e/v04-reopen.mjs).
[Operator selector](../../backend/test/integration/v2cleanup/selection.go) and its
original-engine pre-apply proof tests retain historical negative-run provenance;
only the existing delivery cleanup is invoked by current main-flow acceptance.

## Issue #6 — EC2 instance-role authentication

UC-04 EC2-01..05 / ADR-018: [account orchestration](../../backend/internal/application/connection/ec2_role.go),
[fixed IMDSv2 + STS/IAM provider](../../backend/internal/adapters/awsaccess/ec2_role.go),
[private refresh endpoint](../../backend/internal/adapters/awsaccess/credential_endpoint.go),
[Terraform binder](../../backend/internal/adapters/terraform/providers/binder.go),
[readiness](../../backend/internal/adapters/terraform/readiness/readiness.go),
[HTTP account API](../../backend/internal/delivery/http/driver_accounts.go) and
[Console](../../frontend/src/features/platform/accounts/).

Validation: [service/resolver tests](../../backend/internal/application/connection/ec2_role_test.go),
[HTTP contract tests](../../backend/test/e2e/ec2_role_http_test.go),
[Go live Terraform workload](../../backend/test/acceptance/ec2-role/main.go),
[browser workload](../../frontend/test/e2e/ec2-role-real.mjs).
Deployment: [EC2 Compose overlay](../../docker-compose.ec2.yml),
[start command](../../deploy/ec2/start.sh), [Docker runbook](../operations/docker-local.md).

UI3-05 unified existing-cluster catalog/editor uses ResourceDefinitionsPage and
ExistingClusterDefinition.test.tsx; the separate ExistingClusterSection was removed.

Application scope availability is mapped from advertised keys in `App.tsx` and
`Application.availableEnvironments`, with no requests for unsupported scopes.
`EnvironmentResources.tsx` distinguishes known backend unavailability from an
empty inventory and ordinary load errors.

IMP-032 persistent kind runtime: [`preview-kind.sh`](../../deploy/local/preview-kind.sh)
fixes an isolated Compose project with [`real-runtime overlay`](../../deploy/local/compose.preview-kind.yml),
explicit score-k8s and persistent module/provider caches. Browser onboarding and
deployment use [`preview-kind-live.mjs`](../../frontend/test/e2e/preview-kind-live.mjs);
job/update/restart/cleanup observations use
[`preview-kind-check.mjs`](../../frontend/test/e2e/preview-kind-check.mjs).
Shared full-app flows use the current v3 Definition criteria/advanced controls.
See [live evidence](../verification/2026-10-23-preview-kind-live.md).

IMP-033: DeploymentReview validates the incoming review before mounting its
stateful renderer. DeploymentReview.test.tsx covers zero-value/unsupported/null
collections and availability transitions; ApplicationHomePage.test.tsx uses the
actual no-change wire response and preserves the disabled Deploy control.

Desktop shell width preference, role-filtered SVG navigation and accessible
collapse/expand live in AppShell.tsx, with state/storage/gate coverage in
AppShell.test.tsx. DeploymentReview supplies type attributes and cluster/namespace
SVG illustrations; app.css owns the shared icon palette, sticky desktop rail and
wider Preview pages. DeploymentDetailsPage uses the same desktop width treatment.
See [visual and browser checks](../verification/2026-10-23-sidebar-preview-space.md).

VIS-09 safe scalar projection is in
[`properties.go`](../../backend/internal/application/deploymentreview/properties.go),
called by the existing review assembler. Its tests cover sensitive/unclassified
inputs, unknown expressions, secret outputs and numeric literal port bounds.
[`reviewProperties.ts`](../../frontend/src/features/preview/reviewProperties.ts)
derives public facts from that projection and sanitized manifests; shared
[`reviewMetrics.ts`](../../frontend/src/features/preview/reviewMetrics.ts) keeps
card dimensions consistent with routing. `reviewEdges.ts` assigns separate tracks,
outer lanes and attachment columns, with dense-graph geometry regressions.

## UC-17 — Platform effectiveness

[Specification](../usecase/UC-17/specification.md) → measurement contract and
catalog [domain](../../backend/internal/domain/effectiveness/) (work items,
period/timezone, metrics, baseline); [service](../../backend/internal/application/effectiveness/);
ports [baseline/cleanup history](../../backend/internal/ports/persistence/effectiveness.go);
adapters [PostgreSQL](../../backend/internal/adapters/postgres/effectiveness.go)
(+ `effectiveness_baselines` in [baseline.sql](../../backend/internal/adapters/postgres/baseline.sql),
SELECT/INSERT [grants](../../backend/internal/adapters/postgres/grants.go)) and
[memory/JSON](../../backend/internal/adapters/store/effectiveness.go);
[HTTP OC-20](../../backend/internal/delivery/http/effectiveness.go);
Console [feature](../../frontend/src/features/effectiveness/), route/sidebar in
[routes](../../frontend/src/app/routes.ts) and [AppShell](../../frontend/src/app/AppShell.tsx).
Adding a metric: a catalog entry in `metrics.go` (Compute + Records) and its
copy in `labels.ts`; a new external source also needs spec/schema first.

## Git render bundles — ADR-021

| Concern | Code |
|---|---|
| Bundle manifest, pin record, typed inputs | [`renderbundle`](../../backend/internal/domain/templatesource/), selection pin in [`rendering.go`](../../backend/internal/domain/resource/rendering.go) |
| Registry port and errors | [`renderregistry`](../../backend/internal/ports/renderregistry/renderregistry.go), repository [`renderbundles.go`](../../backend/internal/ports/persistence/templatesources.go) |
| Inspect, durable cache, loader | [`adapters/renderbundles`](../../backend/internal/adapters/templatesources/) (Git transport shared via [`shared.go`](../../backend/internal/adapters/terraform/registry/shared.go)) |
| Persistence | [`render_bundles` DDL](../../backend/internal/adapters/postgres/baseline.sql), [PostgreSQL](../../backend/internal/adapters/postgres/templatesources.go), [JSON store](../../backend/internal/adapters/store/templatesources.go), [grants](../../backend/internal/adapters/postgres/grants.go) |
| Template engine, worker, patches, tree validation | [`renderengine`](../../backend/internal/renderengine/); worker entry in [`main.go`](../../backend/cmd/orchestrator/main.go) |
| Git render pipeline and invariant guard | [`git.go`](../../backend/internal/adapters/templatedriver/git.go), [`guard.go`](../../backend/internal/adapters/templatedriver/guard.go); secret bridge [`delivery.go`](../../backend/internal/adapters/kubernetes/delivery.go) |
| Catalog/HTTP | [`catalog/renderbundles.go`](../../backend/internal/application/catalog/templatesources.go), `render_bundle_sources.go` (restored by UC-03 CM-01–04), codes in [`apierror.go`](../../backend/internal/delivery/http/apierror.go) |
| Planner, snapshot, pending, review | [`planning/rendering.go`](../../backend/internal/planning/rendering.go), [`deployment/snapshot.go`](../../backend/internal/application/deployment/snapshot.go), [`pending/preview.go`](../../backend/internal/application/pending/preview.go), [`deploymentreview/review.go`](../../backend/internal/application/deploymentreview/review.go) |
| Wiring/runtime | [`bootstrap.go`](../../backend/internal/bootstrap/bootstrap.go), [`Dockerfile`](../../backend/Dockerfile), [`docker-compose.yml`](../../docker-compose.yml) cache volume |
| Console | `RenderingTemplatesPage` (restored by UC-03 CM-01–04), [`GitBundleSource`](../../frontend/src/features/platform/GitBundleSource.tsx), [`RenderInputsEditor`](../../frontend/src/features/platform/RenderInputsEditor.tsx), [`renderInputs`](../../frontend/src/features/platform/renderInputs.ts), Preview provenance in [`DeploymentReview`](../../frontend/src/features/preview/DeploymentReview.tsx) |
| Reference bundle | [`backend/render-bundles/standard`](../../backend/template-sources/standard/README.md) with executable examples |
| Tests | domain/engine/registry/guard/stage/pending/HTTP/PostgreSQL tests beside each package; UI `GitRenderBundle.test.tsx` (restored by UC-03 CM-01–04) |
| Real acceptance | [`git-render-bundle-kind.sh`](../../backend/test/integration/git-render-bundle-kind.sh), [`git-render-bundle-kind.mjs`](../../frontend/test/e2e/git-render-bundle-kind.mjs), [`compose overlay`](../../deploy/local/compose.git-render-bundle.yml) |

## Staging bootstrap (2026-10-26)

- `backend/internal/seed/seed.go`: staging missing-only Type bootstrap; explicit fixtures giữ nguyên.
- `backend/cmd/orchestrator/main.go`: startup catalog default staging.
- `backend/deploy/docker-entrypoint.sh`: installed score-k8s path.
- `frontend/src/app/AppShell.tsx`, `App.tsx`, `routes.ts`: Driver Accounts entry, retire Connections screen.
- `frontend/test/e2e/bootstrap-staging-catalog.mjs`: operator Playwright registration/preflight/rerun checks.
- [Runbook](../operations/staging-catalog.md): operator inputs, installed bundle và staging matching.

## Application deletion — UC-18 / ADR-025

| Design | Code / tests |
|---|---|
| Lifecycle, progress model | [`domain/application`](../../backend/internal/domain/application/application.go), [`ports/persistence/lifecycle.go`](../../backend/internal/ports/persistence/lifecycle.go), [`postgres/lifecycle.go`](../../backend/internal/adapters/postgres/lifecycle.go), [`store/lifecycle.go`](../../backend/internal/adapters/store/lifecycle.go), baseline `applications` |
| Preview, admission, staged run, Vault purge | [`application/appdeletion`](../../backend/internal/application/appdeletion/), [`vault.Registry.PurgeValues`](../../backend/internal/adapters/vault/registry.go) |
| Whole-generation confirmed cleanup | [`resourcecleanup/environment.go`](../../backend/internal/application/resourcecleanup/environment.go) |
| HTTP routes, lifecycle guard, error codes | [`delivery/http/application_deletion.go`](../../backend/internal/delivery/http/application_deletion.go) |
| Console | [`features/applications/deletion`](../../frontend/src/features/applications/deletion/), Settings danger zone, Application home banner |
| Tests | [`test/e2e/application_deletion_test.go`](../../backend/test/e2e/application_deletion_test.go) (real PostgreSQL, labelled fake engine), [`DeleteApplicationDialog.test.tsx`](../../frontend/src/features/applications/deletion/DeleteApplicationDialog.test.tsx), kind WM-E08 and AWS `appdel.*` acceptance |
| Integrated form-only acceptance | [`e2e/scenarios/application-deletion-form`](../../e2e/scenarios/application-deletion-form/README.md), shared [`delete-application.mjs`](../../e2e/lib/delete-application.mjs), workload form scenario and owned-kind harness `--form-deletion` |

## ENV-UX — direct workload variable entry (2026-10-27)

- `EnvironmentVariablesTable.tsx` owns direct Name/Value entry, combined source
  search, scoped labels and the Secret creation entry.
- `WorkloadEditorPage.tsx` resets inactive binding payload with `changeSource`
  on inline key/resource creation; stable row/scope and CAS guards remain.
- `workloads.envEntry.*` in the VI/EN catalogs owns new visible/accessible copy;
  `styles/app.css` extends only the table/picker layout.
- Real UI form acceptance and optional video recording use
  [`e2e/scenarios/workload-env-form`](../../e2e/scenarios/workload-env-form/README.md)
  and the shared `signInContext` helper; no Score import in this scenario.


## Workload catalog — issue #24

`domain/environment/execution.go`, Score conversion/reconstruction and
`adapters/postgres/workload_execution_integration_test.go` carry the typed
execution intent. `domain/renderbundle/workload_templates.go`, registry metadata,
workload catalog HTTP and `planning/workload_execution.go` validate immutable
pins. `planning/graph.go` and `storage_intent.go` cover storage dependencies and
parameter changes. Standard Git `patches/execution.tpl` and score-k8s guards
render the primary controller; Kubernetes objects enforce safe CronJob removal.
`application/observability/jobs.go`, `adapters/kubernetes/jobs.go` and HTTP
observability routes expose UID-owned live Job history; `JobHistoryDialog.tsx`
uses the current home row. Terraform source modules are in
`backend/terraform-modules/`. Generic E2E lives in
`e2e/scenarios/workload-catalog/`; [handoff](workload-catalog.md) owns gaps.

## ZIP module and render-bundle sources — ADR-026

- `backend/internal/platform/sourcezip/`: bounded, non-extracting archive reader.
- Registry `zip_inspect.go` adapters reuse HCL/renderer inspection and verified
  caches; Terraform ZIPSnapshot retains same-archive local module paths.
- HTTP `source_upload.go` and catalog upload methods: PE/Admin multipart upload,
  scoped metadata response and immutable pins. PostgreSQL stores render source_kind.
- Console `ZIPSource.tsx`: shared upload/inspect controls in Resource Definitions
  and Rendering Templates; existing typed inputs/Create/CAS persist prepared pins.
- [Browser scenario](../../e2e/scenarios/zip-source-upload/README.md) exercises
  both upload forms and rendering-template save/reopen with real inspection/cache.


## Default shared workload resources — UC-16 BR-28–30

- [sharedResources](../../frontend/src/features/workloads/sharedResources.ts):
  generated IDs, deduplicated safe desired projections and stale selection checks.
- [DependencyFields](../../frontend/src/features/workloads/DependencyFields.tsx):
  new/existing selector and advanced private scope, reused by AddResourceDialog.
- [shared resource tests](../../frontend/src/features/workloads/sharedResources.test.ts),
  WorkloadClassMetadata and EnvironmentVariablesTable cover projection, scope,
  identity preservation and both entry points. Browser runner:
  [workload-shared-resources](../../e2e/scenarios/workload-shared-resources/README.md).

## Operator bootstrap — 2026-10-30 path migration

[`scripts/bootstrap/run.mjs`](../../scripts/bootstrap/run.mjs) replaces the former
E2E register-render-bundles setup entrypoint. Its config, infrastructure/render
helpers and config tests live alongside its own package/lockfile. Shared Console
access is implemented in `scripts/bootstrap/lib/console.mjs`; E2E keeps a re-export
at `e2e/lib/console.mjs`. See [installation/usage](../../scripts/bootstrap/README.md).
This is a behavior-preserving move; target presets are not yet implemented.

Migration validation: bootstrap config/reuse tests15/15 and shared Console tests6/6
passed; new entrypoint help works from repository root and an unrelated working
directory. Read-only headless sign-in and catalog GET via the migrated helper
passed; E2E re-export points to the exact same implementation. No setup/seed,
cluster/cloud mutation or deployment was rerun. Product Go/React suites were
not run for this path-only migration. CI now installs both packages and checks
bootstrap help plus the two helper suites. Documentation/link and whitespace
checks passed.

Bootstrap presets (2026-10-30): `scripts/bootstrap/targets/dev.json` and
`staging.json` seed local and confirmed staging URL/user/Ceph configuration.
`lib/target-config.mjs` resolves --target/--kubeconfig, private credential paths
and headless/zero-pacing defaults; --config remains supported. Account discovery
uses existing READY account metadata or a single UI-inspected kubeconfig context;
ambiguous contexts and unhealthy accounts fail. New renderer criteria class pvc
avoid mutating default workload selection. Seed succeeds without Application;
explicit missing verify.applicationKey still fails. No password enters Git.

Preset validation: bootstrap tests19/19 and shared Console tests6/6 pass. Read-only
headless dev authentication with preset defaults reuses the exact existing kind
account without kubeconfig reads or mutation. CLI syntax/help, documentation and
whitespace checks pass. No staging access, seed writes, cluster deployment or
broad Go/React suites were run. The staging URL/shared credentials/StorageClass
were supplied or confirmed by the user; staging account creation remains unrun.
Private dev/staging password files were prepared without overwriting existing
files; copying the staging password file to the execution host is still required.

Bootstrap seed-login default (2026-10-30 user correction): dev/staging presets
now include the existing product fixture login, so --target needs no password
file. readConsoleConfig accepts a preset-only fallback password; explicit file
overrides take priority and failures do not fall back. Legacy --config/E2E flows
still require private password files. No secret values are printed. Previously
prepared private files are left untouched and no longer required by presets.

Validation of seed-login correction: bootstrap20/20 and shared Console6/6 tests,
CLI help, documentation and whitespace checks pass. Real headless dev sign-in
with an empty environment/default seed login and no password file passed;
no seed/deployment/cluster mutations or staging access were performed.

Bootstrap Console URL override: --url works with --target or --config and wins
over ORCH_PLATFORM_URL/preset, preserving origin validation before browser launch.
Only URL and kubeconfig need operator input for the default seed-login presets.

Portable catalog snapshot: [`platform-catalog/`](../../platform-catalog/README.md)
contains the nine current Terraform modules and standard render bundle for
copying into a separate company Git repository. Source code remains in the
original backend directories; SOURCE_MAP.json records source revision, paths
and exported hashes. Exported runtime files are byte-identical; the bundle
README link is made standalone. Runtime does not load this distribution copy.

Snapshot validation: exported file hashes and source/runtime byte parity pass,
all nine module trees parse with Terraform fmt (no writes), and real score-k8s
primary/mount plus standard-example tests pass on the identical canonical files.
Docs/whitespace checks pass. No Terraform init/apply/destroy, cluster mutation,
provider download or unrelated full suite was run for this distribution copy.

## Bootstrap company Git migration — 2026-10-30

`git-overrides.mjs` rewrites source URL/ref from CLI and converts known installed
Terraform modules to the exported `backend/terraform-modules/<module>` layout.
The explicit company commit is required for installed→Git conversion. Original
paths/mappings/variables/criteria/placement and existing-cluster account stay
unchanged. --config and presets both support overrides; no override retains old
behavior. Existing source changes require --update-existing, implemented via
`definition-action.mjs` (source-only diff guard) and `definition-update-ui.mjs`
(browser PUT payload + expected-version guard). Infrastructure edits and renderer
bundle selection use normal Console edit/CAS flows. HTTP409 or uncertain outcomes
stop without retry. Runtime workloads and resource state are never reset/deployed.

Validation: bootstrap26/26 tests pass, including exported source conversion,
invalid URL/ref, preserving non-source fields, forbidden account/criteria/mapping
changes and payload/version guards. Explicit local browser check
`scripts/bootstrap/test/company-git-ui.mjs` passed installed→company Git module
update, exact matching rerun with no additional PUT, renderer source update and
one-PUT HTTP409 stop. It uses actual local product UI and read-only metadata;
all inspect/catalog writes are intercepted fixtures. No actual company Git,
staging mutation, reset, Terraform or cluster scenario was run. CLI syntax/help,
shared Console helper tests, documentation and whitespace checks pass; product
Go/React suites were not repeated for this operator-script change.

## Platform Git access

- [Service](../../backend/internal/application/gitaccess/service.go): scope,
  write-only token retention, CAS and per-Organization resolution.
- [HTTP](../../backend/internal/delivery/http/gitsettings.go) and
  [Console](../../frontend/src/features/platform/PlatformSettingsPage.tsx).
- [Transport](../../backend/internal/adapters/terraform/registry/fetch.go):
  isolated auth environment and temporary CA shared by module/render inspection.
- [Repository](../../backend/internal/ports/persistence/gitsettings.go),
  PostgreSQL `gitsettings.go`, JSON `store/gitsettings.go`; immutable migration
  `002-git-settings.sql` and frozen `version-002.sql`.

## Delivery reconciliation foundation — ADR-030

- Domain selection/capabilities/revision observations and mutation-state guards:
  [delivery domain](../../backend/internal/domain/delivery/delivery.go),
  [mutation lifecycle](../../backend/internal/domain/delivery/intent.go).
- Provider/ownership contracts and explicit registry:
  [delivery port](../../backend/internal/ports/delivery/delivery.go),
  [registry](../../backend/internal/ports/delivery/registry.go).
- Optional runtime wiring lives in [bootstrap](../../backend/internal/bootstrap/delivery.go)
  and the [coordinator](../../backend/internal/application/delivery/coordinator.go).
  Remaining acceptance gates are in the [implementation plan](delivery-reconciliation.md).


ADR-030 storage and publication additions:
[PostgreSQL binding/selection repository](../../backend/internal/adapters/postgres/delivery.go),
[JSON/store repository](../../backend/internal/adapters/store/delivery.go),
[shared persistence contract](../../backend/internal/ports/persistence/persistencetest/delivery.go),
[Git publisher](../../backend/internal/adapters/gitpublication/git.go),
[Fleet packaging](../../backend/internal/adapters/delivery/fleet/package.go),
[controller management](../../backend/internal/adapters/delivery/fleet/manager.go).


Stored delivery observations: [worker/service](../../backend/internal/application/delivery/observations.go),
[repository contract](../../backend/internal/ports/persistence/delivery_observation.go),
[PostgreSQL leases](../../backend/internal/adapters/postgres/delivery_observation.go),
[HTTP read](../../backend/internal/delivery/http/reconciliation.go) and
[Console panel](../../frontend/src/features/delivery/ReconciliationPanel.tsx).
Retired direct auxiliary cleanup: [deployment hook](../../backend/internal/application/deployment/retired_auxiliary.go)
before Application namespace destruction. Terraform drift's separate private-plan
parser is [domain/drift](../../backend/internal/domain/drift/plan.go);
[encrypted baseline capture](../../backend/internal/application/tfexec/baseline.go),
[private-copy inspection](../../backend/internal/application/tfexec/drift.go) and
[private PostgreSQL pins](../../backend/internal/adapters/postgres/drift_baselines.go)
are implemented. [Scan worker](../../backend/internal/application/drift/service.go),
[scan API](../../backend/internal/delivery/http/drift.go),
[reviewed repair](../../backend/internal/application/drift/repair.go),
[private repair executor](../../backend/internal/application/tfexec/drift_repair.go),
[review persistence](../../backend/internal/adapters/postgres/drift_repair_reviews.go),
[scan panel](../../frontend/src/features/delivery/InfrastructureDriftPanel.tsx) and
[repair dialog](../../frontend/src/features/delivery/InfrastructureRepairDialog.tsx)
are in runtime acceptance. The parser preserves the existing cleanup parser's
delete-only drift contract and never serializes private comparison digests.

Explicit recovery: [receipted Submit](../../backend/internal/application/delivery/submission_recovery.go),
[handoff](../../backend/internal/application/delivery/transfer_recovery.go),
[UID-proof removal](../../backend/internal/application/delivery/removal_recovery.go).
Fleet-specific [policy field ownership](../../backend/internal/adapters/delivery/fleet/policy_fields.go)
and [downstream release retirement](../../backend/internal/adapters/delivery/fleet/withdraw_wait.go)
stay behind the provider port.

ADR-030 retention: [owner maintenance](../../backend/internal/adapters/postgres/drift_retention.go) is invoked by `-prune-drift-history`; exact catalog/role checks and schema triggers preserve active/recovery evidence without runtime DELETE privileges. [Historical dependency comparison](../../backend/internal/application/tfexec/drift_dependencies.go) retains original consumer inputs across stable-output acknowledgements.

## Versioned catalog releases — issue #28

[Implementation route](catalog-releases.md): domain manifest/selection/diff in
`backend/internal/domain/catalogrelease`; complete publication/selection in
`application/catalogrelease`; hardened Git discovery/inspection in
`adapters/terraform/registry/catalog_*.go`; durable source/release/Environment
repositories in memory/JSON and PostgreSQL, migration 004. Planning snapshots and
pending Preview/Deploy resolve release-scoped Definitions and fence applied
activation. Template authoring filters verified metadata to the selected release.
HTTP: `delivery/http/catalogreleases.go`; Console:
`frontend/src/features/catalog/`, Application Settings and existing Preview UI.

## Catalog-only configuration (2026-11-02)

- [Resolution](../../backend/internal/application/catalogrelease/selection.go):
  selected release required, no global fallback.
- [Release detail API](../../backend/internal/delivery/http/catalogreleases.go):
  PE/Admin-scoped snapshots and template metadata.
- [Catalog detail UI](../../frontend/src/features/catalog/ReleaseDetails.tsx)
  and [tests](../../frontend/src/features/catalog/ReleaseDetails.test.tsx).
- [Bootstrap](../../scripts/bootstrap/run.mjs): prerequisites, Git source, discovery.
- [Export manifest](../../platform-catalog/catalog.yaml): Git-owned Definitions.
- [Test fixture publisher](../../backend/internal/seed/seedtest/catalog.go):
  explicit synthetic release snapshots; not product discovery.

## Required CD application manifests — ADR-032

- `application/deployment`, `application/pending`, `application/preview`: strict
  production constructors and explicit fake-adapter simulation constructors.
- `application/delivery/bindings.go`, `coordinator.go`: no selectable or executable
  Direct destination; historical source receipts remain usable for migration.
- `adapters/kubernetes/vso.go`: pure CD declaration rendering; `secret_metadata.go`
  reads only Secret identity/key names. `adapters/delivery/fleet/secrets.go` gates
  prerequisite readiness and waits for VSO child garbage collection.
- `adapters/vault/provider.go`, `registry.go`: private dependency values and
  separate CD ServiceAccount identities. `ports/execution/certificates.go`
  validates the public-CA-only Secret exception.
- `features/delivery/CDProvidersPage.tsx`: platform default and GitHub repository policy; application delivery authoring is retired.

See [implementation and checks](delivery-reconciliation.md).

## Platform-managed delivery — ADR-033

- Domain: `domain/delivery/repository.go`; ports: `persistence/delivery_repository.go`,
  `repositoryhosting/hosting.go`, `delivery.ApplicationRepositoryBinding`.
- Services: `application/delivery/repositories.go` pins defaults and reserves/acknowledges
  allocations under an admitted owner; Application creation pins the binding without HTTP.
- Persistence: Store/PostgreSQL `delivery_repository.go`; immutable migration 005 and
  frozen version-005; Console SELECT/INSERT/UPDATE, no State or remote-delete privileges.
- Adapters: `repositoryhosting/github` uses private initialized repositories, durable
  markers, observed remote ID and branch; Fleet binds internal destinations and discovers
  a unique controller/local cluster registration. Production registers no Direct provider.
- HTTP: platform-only GET/PUT `/delivery-default`; retired per-Environment PUT `/delivery`.
  Console platform form authors owner/credentials; Application Settings authors neither.
- Verification: [results and limits](../verification/2026-11-02-platform-managed-cd.md).

## Shared Git for platform CD setup

- `application/gitaccess/writer.go`: resolves current Organization-scoped Git
  settings into private writer credentials; validates scope and durable storage.
- `application/delivery/bindings.go`, `delivery/http/delivery.go`: author and
  expose the credential-source choice without returning credential references.
- `adapters/repositoryhosting/github/github.go`,
  `adapters/gitpublication/factory.go`: resolve live platform credentials;
  publication cache identity changes on credential rotation.
- `features/delivery/CDProvidersPage.tsx`: default shared Git when configured,
  automatic Fleet discovery on account selection, connection summary, actionable
  bounded verification reasons and existing explicit platform default action.

## Definition forms and Catalog modules — 2026-11-02

The earlier Catalog-only retirement entries above are superseded by UC-03 CM-01–04.
ResourceDefinitionsPage and RenderingTemplatesPage, their routes/navigation, strict
Definition CRUD/matching and render authoring handlers are restored. Catalog module
metadata is served by `delivery/http/catalog_modules.go` and
`application/catalog/catalog_modules.go`; `CatalogModuleSource.tsx` selects source,
release and compatible component. Provenance persists beside `driverInputs.values`.
`catalogrelease.ResolveDefinitions` composes authored and optional release
Definitions for Preview, Deploy and matching preview; authored keys replace the
same snapshot key. Metadata/diagnostics follow the same authoring semantics.


## Simplified CD connections — ADR-034 / issue #31

Rancher-compatible discovery extends this route: Fleet `topology.go` shares
controller/agent workload identity, version and served-schema policy across
Inspect/Verify; `target_identity.go` reads authenticated target UID and nonsecret
agent clientID, rejects ambiguous physical matches and derives existing
namespace/account policy. `Config` retains optional pins for new bindings while
legacy versions remain readable. `Provider.preflight` checks the saved physical
UID against the actual planner-selected target. `topology_test.go` covers
Rancher-shaped fixtures and opt-in read-only kind probing; `e2e/scenarios/rancher-fleet`
reuses real application acceptance and independently checks topology evidence.

- `application/connection/service.go`, `accounts.go`, `delivery/http/connections.go`:
  scoped `GIT`/`GIT_TOKEN` registration and safe list; infrastructure account APIs
  exclude Git Connections. Migration/reference 007 extends only kind/auth checks.
- `application/gitaccess/writer.go`, hosting/publication factories: live scoped
  credential resolution; publication cache rotates with selected credential bytes.
- `application/delivery/targets.go`, Fleet `inspect.go`, `target_identity.go`:
  workspace-scoped registration discovery, authenticated physical UID matching and
  bounded ineligible choices. Opaque custom identities remain unsupported.
- Fleet `managed_reader.go`: derive owned per-binding-version Secret, create/refresh
  only during admitted Deploy; retain external Secrets and reusable credentials.
- Fleet `package.go`, `execution.go`: YAML publication and JSON/YAML artifact reading.
- `application/delivery/pending_prune.go`, Fleet `pending_prune.go`: fenced
  deletion-only retirement of acknowledged omitted UIDs before clean reobservation.
- Fleet `git_repo_metadata.go`, `git_render_generation.go`: preserved controller
  metadata, per-intent non-Job rendering and receipt-scoped recovery.
- `CDProvidersPage.tsx`, `CDKubernetesConnectionDialog.tsx`: four-field authoring,
  inline reuse/registration, private input clearing and uncertain-outcome holds.
- `e2e/scenarios/simplified-cd/`: real inline setup, technical Git/Fleet proof,
  connection reuse/default stability, image rollback and ownership cleanup.
- `scripts/bootstrap/lib/platform.mjs`: updated ordinary-form selectors/configuration;
  shared acceptance Score uses the canonical `svc_backend_http` service alias.
- `scripts/bootstrap/lib/verification.mjs`: reconnect an owned service forward
  after rollout before real web/data verification.

## Generic Workload replacement — ADR-035

- Generic Workload admission and dependency conversion: `backend/internal/planning/workloadspec/`.
- Immutable template source domain/registry: `backend/internal/domain/templatesource/`, `backend/internal/adapters/templatesources/`.
- Direct renderer and semantic guards: `backend/internal/adapters/templatedriver/`; bounded worker/helpers: `backend/internal/renderengine/`.
- Standard manifest sources and initial immutable registration: `backend/template-sources/`.
- Shared Definition form: `frontend/src/features/platform/ResourceDefinitionsPage.tsx` and `TemplateDriverFields.tsx`; separate Rendering Templates page removed.
- Owner-only PostgreSQL transition: `migrations/008-generic-workload.sql`, frozen `version-008.sql`.
- API replacements: `/template-sources` (GET, inspect, upload, inline), workload request field `workload`, standalone `/workload-preview`, `/workload-samples`.


## Direct Definition bootstrap — issue #33

- Config schema 2/3: `scripts/bootstrap/lib/bootstrap-config.mjs`; source/Definition
  files and immutable pins: `direct-config.mjs`, `direct-targets/`, `definitions/`.
- Console source preparation, form review/read-back and conflict checks:
  `scripts/bootstrap/lib/direct-platform.mjs`; shared platform/CD helpers remain
  in `platform.mjs`, `git-access-config.mjs` and `infrastructure-ui.mjs`.
- Platform → acceptance orchestration: `direct-runner.mjs`; reuse `application`,
  `environment`, `workloads`, `deployment`, `namespace-access`, `verification` and
  `runtime` helpers. Catalog v2 stays in `runner.mjs`.
- Focused config/reuse/resume/pin tests: `direct-bootstrap.test.mjs`; real kind
  smoke/unchanged-rerun comparison: `e2e/scenarios/bootstrap-acceptance/run.mjs`.
- [Scope and status](direct-bootstrap.md), [procedure](../../scripts/bootstrap/README.md).

Git-only follow-up: `scripts/bootstrap/lib/git-inputs.mjs` reads and verifies pinned
Git files; `git-targets/` supplies dev/staging input examples and `git-assets/` the
committed business documents. `verification.mjs` checks backend PVC/file identity;
`e2e/scenarios/bootstrap-git-pvc/run.mjs` verifies owned Pod recreation and rerun.
`video.mjs` publishes only successful workload recordings; `video.test.mjs` covers
its actual Chromium lifecycle.

Enterprise bootstrap: `scripts/bootstrap/enterprise/` contains input and internal-AI
setup docs; `projects/platform-resources/`, `input-parameters.mjs`, `enterprise-lock.mjs` and
`lock-enterprise-input.mjs` implement data portability/offline pinning. Enterprise
host/API/CA inheritance is in `application/delivery/bindings.go`; repository API
TLS trust is in `adapters/repositoryhosting/github/github.go`.

Enterprise infrastructure image overrides: `parameters.postgresImage` and
`redisImage` in `scripts/bootstrap/enterprise/staging.input.json` are expanded
into the publishable PostgreSQL/Redis Definitions. Embedded release unchanged.

Database Git assets: `projects/platform-resources/terraform/{mysql,mariadb,mongodb}`
and matching `bootstrap/definitions/*.json`; Types are declared in
`bootstrap/resource-types.json`, input/pin fields in enterprise staging input.
Setup and credential/storage contract: `projects/platform-resources/bootstrap/DATABASES.md`.
The MySQL Type schema remains compatible; new types use the same nonsecret
Secret-reference contract. No embedded release/runtime code changed.

Console staging bootstrap: `execution-profile.mjs` guards native CLI access;
`enterprise-prepare.mjs` / `prepare-enterprise-input.mjs` prepare local snapshots;
`prepared-inputs.mjs` checks their hashes without shell Git. `runner.mjs`,
`direct-runner.mjs` and `verification.mjs` separate Console/browser observations
from explicit dev-test cluster checks; runtime receipts pin the profile.

Workload HTTPRoute assets: `projects/platform-resources/templates/standard/resources/httproute.tpl`
and its schema/bundle entry. Gateway inputs flow through the workload Definition;
frontend hostname/path/Service port annotations come from enterprise input.
Route ownership/lifecycle uses existing rendered workload inventory and Fleet.
Setup/migration: `projects/platform-resources/bootstrap/HTTPROUTE.md`.
No separate HTTPRoute Terraform module/Type/Definition in the publishable set.
Focused real-renderer coverage: `company_httproute_test.go` in templatedriver.

## Application-scoped Fleet delivery — ADR-036

- Fleet `repositories.go` injects internal Application/Environment identity from the
  durable allocation. `package.go` emits readable Environment/workload/phase YAML
  paths and separate nonsecret revision metadata; the template driver is unchanged.
- `adapters/delivery/fleet/application.go` owns the shared GitRepo's per-unit pins,
  guarded pointer, sibling-tree verification, selective withdrawal, lost-receipt
  observation and exact first-unit admission during explicit recovery. Legacy `manager.go` and EXISTING receipt handling remain for cleanup.
- `provider.go`, `pending_prune.go`, `recovery.go`, `recovery_wake.go` select the exact
  Bundle by release and verify its tree even after another unit advances the SHA.
- `application/delivery/coordinator.go` rejects mixed legacy/shared live layouts;
  `submission_recovery.go` can persist an observed lost receipt before recovery;
  `removal.go` resolves the same allocated Application repository before withdrawal.
- `adapters/gitpublication/git.go` verifies multiple retained trees with one fetch.
- Bootstrap `fleet-verification.mjs` is shared with the HTTPRoute E2E assertion;
  application identity replaces the old Environment GitRepo label/count assumption.

- `planning/intent.go` normalizes an empty secret-output contract before comparing
  persisted execution intent; regression tests keep unchanged namespace/storage
  RETAIN while a real secret-output contract change remains UPDATE.
- `e2e/scenarios/application-fleet-gitrepo/` contains checkpointed dual-Environment
  acceptance plus exact-operation recovery and runtime-pin refresh helpers.

- `ports/delivery.SubmissionAbsenceRecovery` allows only read-only proof of an
  unsubmitted first prerequisite. The coordinator atomically settles its FAILED
  intent and publication lease; in-memory execution/lease methods compose inside
  an existing transaction, matching PostgreSQL. No Job absence is inferred.
- Coordinator normal/recovery source selection respects the latest acknowledged
  withdrawal, so a restored unit does not inherit the retired revision/inventory.

- `application/deployment/baseline.go` follows only immutable, successful removal
  base-set lineage for UNREFERENCED resources whose latest removal plan omits
  their execution evidence. It preserves cleanup ownership, snapshot isolation,
  exact scope matching and fail-closed behavior for missing/foreign/cyclic history.

## Public endpoint resource — EP-01..06

- `domain/endpoint` parses the Definition policy and resolves name/port/path to
  hostname/url; `planning/endpoint.go` plans one endpoint per workload with fixed
  public reasons (`ReasonEndpoint*`).
- `application/pending/preview.go` exposes `changes[].endpoint` and checks the
  ledger read-only; `application/provisioning/endpoint.go` reserves on Deploy.
- `application/deployment/endpoint.go` emits the `endpoint-<hash>` HTTPRoute with
  the rendered Service's ownership labels; `retainEndpoints` releases after
  acknowledged Deploy, prune or removal.
- `adapters/postgres/endpoints.go` + migration `009-endpoint-reservations.sql`;
  `adapters/store/endpoints.go` is the in-memory twin.
- Console: `features/platform/EndpointDriverFields.tsx`,
  `features/preview/PreviewEndpoints.tsx`. Bootstrap: endpoint Type/Definition in
  `projects/platform-resources/bootstrap`, form filler in `direct-platform.mjs`.
- E2E: `e2e/scenarios/endpoint-domain/`.

## ADR-037 retirement

- Removed: `planning/public_routes.go`, `adapters/kubernetes/public_routes.go`,
  `adapters/gitops/public_routes.go`, `execution.PublicRoute*`, backend
  `-public-*` flags, `SetPublicRoutesPending`, Application `Subdomain`.
- `workloadspec` rejects new `service.publicRoutes`; `environment.Service`
  decoding drops it from historical Deployment Sets.
- Ledger: `adapters/postgres/endpoints.go` (hosts + paths), migration 010.
- HTTP `environments[].endpoints` comes from READY endpoint Active Resources.
- Console: `CreateApplicationPage` (name only), `DependencyFields` (private
  endpoint, Service-port select), Application page endpoint list.

HostPath extension WC-21 (2026-11-06) uses the same `PodVolume`/`ContainerVolumeFields` / `VolumeSourceFields`
flow: node path/type, per-container read mode/subPath, host-path-volumes-v1 source
admission, standard execution template, invariant guards and safe Preview.
Coverage: environment hostPath validation/clone, workloadconfig round-trip, planner
capability admission, templatedriver multi-controller/source/mode invariants,
deploymentreview projection, WorkloadRuntimeIntent UI and workload-runtime-form E2E.
See [generic contract](../architecture/contracts/generic-workload.md) and the runtime verification record. Kubernetes execution remains unverified.

WC-22 — Container volume/file authoring (2026-11-06): WorkloadEditorPage now
places ContainerVolumeFields in every app/init/sidecar card. Add volume or file
creates/attaches a source atomically or reuses an existing source; VolumeSourceFields
provides shared source settings. ExecutionFields owns runtime/schedule/preset
inputs only. The four global storage/source/mount/Secret sections are removed.
Mount paths/modes survive inline PVC/Secret creation; orphan targets are visible
for explicit repair. API/JSON/backend contracts and runtime guards are unchanged.
See WC-22 and [verification](../verification/2026-11-06-container-volume-ui.md) for checks and UI evidence.

WC-23 — Reuse-first volume picker (2026-11-06): ContainerVolumeFields initially
shows searchable/filterable sources with type and known mount users. It includes
current-workload named sources, local storage dependencies, eligible shared PVC
resources from the current app/environment and configured Secrets. New source
creation is explicit. WorkloadEditorPage reuses shared PVC identity/typed params
with a collision-free alias and existing reuse guards, adding dependency + mount
atomically. Other workloads' Pod-local sources are excluded. No API/backend/schema
change. [Verification](../verification/2026-11-06-reuse-first-volumes.md) records checks and real reuse/Preview.

## Workload defaults v1 — WD-01–10

[Field/contract/UI/render map](workload-defaults.md). Domain resolution and typed
constraints live in `backend/internal/domain/environment/runtime_policy.go`;
template merging lives in `backend/internal/domain/environment/template_defaults.go`;
scoped metadata validation/freezing in `application/workloadconfig/defaults.go`.
`planning/workload_execution.go` gates profiles/capabilities;
`templatedriver/runtime_policy.go` guards semantics. Kubernetes runtime admission
uses `adapters/kubernetes/runtime_policy.go` before direct/Fleet publication.
`deploymentreview/defaults.go` projects safe effective values/provenance.
Frontend `RuntimePolicyFields.tsx`, `ExecutionFields.tsx` and `WorkloadEditorPage.tsx`
reuse current shell/controls. Tests cover typed round-trip, legacy behavior,
source precedence/reset, JSONB and owned-kind/browser acceptance.

ADR-039: `environment/template_defaults.go` owns generic template/environment
profile merging and frozen metadata. `runtime_policy.go` owns constraints only;
no backend defaults policy remains. Standard profiles live in source `bundle.yaml`.

`frontend/src/features/workloads/DefaultSelect.tsx` shares muted inherited select
hints, hidden placeholder options and explicit reset across Service/runtime/schedule
fields. `WorkloadDefaults.test.tsx` covers distinct choices and sparse reset/Save.
