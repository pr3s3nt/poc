---
id: IMPLEMENTATION-CODE-MAP
artifact: design-to-code-map
status: current
last_reviewed: 2026-10-23
---

# Design-to-code map

| Design concern | Implementation entry point |
|---|---|
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
| V02 provision authoring and stored rules → Preview graph/batches | [`HTTP planner tests`](../../backend/test/e2e/provision_planning_http_test.go), [`focused runner`](../../backend/test/integration/v02-provision-local.sh), [`scenario`](../../frontend/test/e2e/v02-provision-scenario.mjs) |
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
| I04-07 Definition/class/renderer/Preview và safe progress | [`Definition form`](../../frontend/src/features/platform/ResourceDefinitionsPage.tsx), [`module mapping`](../../frontend/src/features/platform/moduleMapping.ts), [`strict metadata`](../../frontend/src/features/platform/metadata.ts), [`classes`](../../backend/internal/application/catalog/classes.go), [`Workload controls`](../../frontend/src/features/workloads/ClassControls.tsx), [`intent`](../../backend/internal/planning/intent.go), [`baseline snapshot`](../../backend/internal/application/deployment/baseline.go), [`pending consistency tests`](../../backend/internal/application/pending/snapshot_consistency_test.go), [`safe progress`](../../backend/internal/domain/deployment/deployment.go), [`SQL projection`](../../backend/internal/adapters/postgres/deployment.go), [`headed runner`](../../backend/test/integration/definition-workload-local.sh) |
| I04-08 resource detail/confirmed deletion/progress | [`admission/planner/query`](../../backend/internal/application/resourcecleanup/), [`safe SQL reads`](../../backend/internal/adapters/postgres/resources.go), [`HTTP`](../../backend/internal/delivery/http/resource_cleanup.go), [`engine`](../../backend/internal/application/tfexec/cleanup.go), [`Console`](../../frontend/src/features/resources/), [`selected-set UI rules`](../../frontend/src/features/resources/selection.ts), [`SQL/HTTP tests`](../../backend/test/e2e/resource_cleanup_http_test.go), [`headed runner`](../../backend/test/integration/resource-delete-local.sh), [`scenario`](../../frontend/test/e2e/resource-delete-local.mjs) |
| I04-09 Redis acceptance app | [`cache/config/resolver`](../../backend/examples/acceptance-app/internal/jobcache/), [`durable job/marker`](../../backend/examples/acceptance-app/internal/jobstore/), [`backend`](../../backend/examples/acceptance-app/backend/), [`worker`](../../backend/examples/acceptance-app/worker/), [`app UI`](../../backend/examples/acceptance-app/frontend/), [`real fixture`](../../backend/examples/acceptance-app/internal/testfixture/), [`fixture runner`](../../backend/test/integration/acceptance-redis-local.sh) |
| Historical I04-10 profile guard; retained cleanup/acceptance runner | [`historical profile guard`](https://github.com/pr3s3nt/poc/blob/512d0d305c2d556314ec90294d016a02fee3b29f/backend/internal/bootstrap/realprofile.go), [`durable outputs`](../../backend/internal/adapters/vault/output_secrets.go), [`original-engine cleanup`](../../backend/internal/application/runcleanup/), [`delivery removal`](../../backend/internal/application/deployment/remove_delivered.go), [`process kill-chain`](../../backend/internal/application/tfexec/killchain_test.go), [`runner`](../../backend/test/integration/terraform-v2-acceptance.sh), [`supervisor/manifest/ECR/probes`](../../backend/test/integration/v2run/), [`cleanup wiring`](../../backend/test/integration/v2cleanup/), [`headed scenarios`](../../frontend/test/e2e/terraform-v2-acceptance.mjs) |
| Process bootstrap and adapter wiring | [`backend/internal/bootstrap`](../../backend/internal/bootstrap/) |
| HTTP API and `/ui/` delivery | [`backend/internal/delivery/http`](../../backend/internal/delivery/http/) |
| UC-02..UC-04 seed/catalog baseline | [`backend/internal/seed`](../../backend/internal/seed/) and persistence adapters |
| UC-02/03 new-public-ID and strict Driver Inputs validation | [`catalog policy`](../../backend/internal/application/catalog/policy.go), [`policy tests`](../../backend/internal/application/catalog/policy_test.go) and [`HTTP policy tests`](../../backend/test/e2e/registration_policy_http_test.go) |
| UC-16 all-dependency params validation before Save/import | [`workload configuration service`](../../backend/internal/application/workloadconfig/service.go) (`validateResourceParams`), [`resource params tests`](../../backend/internal/application/workloadconfig/resource_params_test.go) and [`shared input type check`](../../backend/internal/domain/resource/contract.go) |
| UC-02..04 insert-only registration, race/rollback and truthful UI states | [`PostgreSQL registration`](../../backend/internal/adapters/postgres/registration.go), [`repository contract`](../../backend/internal/ports/persistence/persistencetest/registration.go), [`HTTP integration`](../../backend/test/e2e/uc02_04_http_test.go), [`UI state tests`](../../frontend/src/features/platform/RegistrationStates.test.tsx) |
| UC-05 planning core and pending Preview → Deploy | [`backend/internal/planning`](../../backend/internal/planning/) and [`backend/internal/application/pending`](../../backend/internal/application/pending/) |
| UC-05 standalone Score Preview | [`read-only service`](../../backend/internal/application/preview/), [`shared planning snapshot`](../../backend/internal/application/deployment/snapshot.go), [`HTTP`](../../backend/internal/delivery/http/score_preview.go) and [`Console`](../../frontend/src/features/preview/) |
| UC-05 human-paced UI recording | [`Playwright`](../../frontend/test/e2e/uc05-video-local.mjs) and [`local runner`](../../backend/test/integration/uc05-video-local.sh) |
| T13 Vietnamese standalone Score Preview | [`Console and parser messages`](../../frontend/src/features/preview/), [`T13 scenario`](../../frontend/test/e2e/refactor-local.mjs), [`refactor runner`](../../backend/test/integration/refactor-ui-local.sh); UC-05 UI screens/states own labels, parser and API semantics unchanged |
| UC-06, UC-07 and UC-09 orchestration/query | [`backend/internal/application/deployment`](../../backend/internal/application/deployment/) |
| UC-07 final resource marking and Service-reference guards | [`deployment service`](../../backend/internal/application/deployment/service.go), [`planner`](../../backend/internal/planning/plan.go), [`Service validation`](../../backend/internal/planning/service_refs.go), [`PostgreSQL atomicity tests`](../../backend/internal/application/deployment/postgres_uc07_integration_test.go) |
| UC-07 pending UI and human-paced recording | [`Application home`](../../frontend/src/features/applications/ApplicationHomePage.tsx), [`editor`](../../frontend/src/features/workloads/WorkloadEditorPage.tsx), [`Playwright`](../../frontend/test/e2e/uc07-video-local.mjs) and [`runner`](../../backend/test/integration/uc07-video-local.sh) |
| Definition-selected workload rendering | [`catalog validation`](../../backend/internal/domain/resource/rendering.go), [`planner selection`](../../backend/internal/planning/rendering.go), [`CLI adapter`](../../backend/internal/adapters/scorek8s/), [`deployment wiring`](../../backend/internal/bootstrap/bootstrap.go), [`pending comparisons`](../../backend/internal/application/pending/preview.go), [`HTTP flow tests`](../../backend/test/e2e/rendering_http_test.go) |
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
| Environment public-path validation, direct/Fleet Ingress reconciliation and route-only retry | [`backend/internal/planning/public_routes.go`](../../backend/internal/planning/public_routes.go), [`backend/internal/adapters/kubernetes/public_routes.go`](../../backend/internal/adapters/kubernetes/public_routes.go), [`backend/internal/adapters/gitops/public_routes.go`](../../backend/internal/adapters/gitops/public_routes.go), [`backend/internal/application/pending`](../../backend/internal/application/pending/), [`frontend/src/features/workloads/WorkloadEditorPage.tsx`](../../frontend/src/features/workloads/WorkloadEditorPage.tsx) |
| Optional Fleet GitRepo workload delivery (kind) | [`backend/internal/adapters/gitops`](../../backend/internal/adapters/gitops/), [`deploy/kind/fleet-poc-gitrepo.yaml`](../../deploy/kind/fleet-poc-gitrepo.yaml) |
| Terraform executor/modules/inspection | [`backend/internal/adapters/terraform`](../../backend/internal/adapters/terraform/) |
| Seed catalog and profiles | [`backend/internal/seed`](../../backend/internal/seed/) |
| UC-00/UC-01 Web Console and local browser verification | [`frontend/src/app`](../../frontend/src/app/), [`features/auth`](../../frontend/src/features/auth/), [`features/applications`](../../frontend/src/features/applications/), [`frontend/test/e2e/onboarding-local.mjs`](../../frontend/test/e2e/onboarding-local.mjs) and [`backend/test/integration/onboarding-playwright-local.sh`](../../backend/test/integration/onboarding-playwright-local.sh) |
| T10 Vietnamese sign-in/shared shell verification | `frontend/src/features/auth/SignInPage.tsx`, `frontend/src/app/AppShell.tsx`, `frontend/src/app/App.tsx`, `frontend/test/e2e/refactor-local.mjs` scenario T10; [evidence](../verification/2026-10-11-T10-sign-in-shell.md) |
| T11 Vietnamese applications list verification | `frontend/src/features/applications/ApplicationsPage.tsx`, `frontend/src/app/App.test.tsx`; applications locator group, human helper và scenario T11 trong `frontend/test/e2e/`; [evidence](../verification/2026-10-11-T11-applications-list.md) |
| UC-00 authentication/session | [`backend/internal/application/authentication`](../../backend/internal/application/authentication/), [`domain/identity`](../../backend/internal/domain/identity/) and [`platform/password`](../../backend/internal/platform/password/) |
| UC-01 self-service creation | [`backend/internal/application/application`](../../backend/internal/application/application/) and [`backend/internal/domain/application`](../../backend/internal/domain/application/) |
| UC-02 Resource Type registration API/UI and Organization-scoped catalog | [`backend/internal/application/catalog`](../../backend/internal/application/catalog/), [`backend/internal/delivery/http/resource_types.go`](../../backend/internal/delivery/http/resource_types.go), [`frontend/src/features/platform/ResourceTypesPage.tsx`](../../frontend/src/features/platform/ResourceTypesPage.tsx) and [`backend/internal/adapters/store`](../../backend/internal/adapters/store/) |
| T02 Vietnamese resource form, automatic driver/module/profile projection and READY Connection dropdowns | [`resource form`](../../frontend/src/features/platform/ResourceDefinitionsPage.tsx), [`form tests`](../../frontend/src/features/platform/ResourceDefinitionsPage.test.tsx), [`state tests`](../../frontend/src/features/platform/RegistrationStates.test.tsx), [`T02 local UI scenario`](../../frontend/test/e2e/refactor-local.mjs) |
| T02B shared Vietnamese criteria modes, independent copy and assistive scope preview | [`CriteriaEditor`](../../frontend/src/features/platform/CriteriaEditor.tsx), [`editor tests`](../../frontend/src/features/platform/CriteriaEditor.test.tsx), [`resource integration tests`](../../frontend/src/features/platform/ResourceDefinitionsPage.test.tsx), [`T02B local UI scenario`](../../frontend/test/e2e/refactor-local.mjs) |
| T05 Vietnamese Secret Store form, advanced defaults, same-address draft and safe verification projection | [`page`](../../frontend/src/features/platform/SecretStoresPage.tsx), [`tests`](../../frontend/src/features/platform/SecretStoresPage.test.tsx), [`scenario`](../../frontend/test/e2e/refactor-local.mjs), [`local runner`](../../backend/test/integration/refactor-ui-local.sh) |
| T04 Vietnamese Resource Types details/copy, capability-based support (T31) and Definition prefill | [`types page`](../../frontend/src/features/platform/ResourceTypesPage.tsx), [`types tests`](../../frontend/src/features/platform/ResourceTypesPage.test.tsx), [`capability metadata`](../../frontend/src/features/platform/capabilities.ts), [`route query`](../../frontend/src/app/routes.ts), [`Definition form`](../../frontend/src/features/platform/ResourceDefinitionsPage.tsx), [`local scenario`](../../frontend/test/e2e/refactor-local.mjs) |
| T03 Vietnamese rendering templates page, PE/Admin route gate and unchanged renderer registration payload | [`templates page`](../../frontend/src/features/platform/RenderingTemplatesPage.tsx), [`page tests`](../../frontend/src/features/platform/RenderingTemplatesPage.test.tsx), [`App role gates`](../../frontend/src/app/App.test.tsx), [`local scenario`](../../frontend/test/e2e/refactor-local.mjs), [`failure stub`](../../backend/test/integration/stubs/score-k8s-render-fail-stub.sh), [`live template locator`](../../frontend/test/e2e/template-engine-kind-human.mjs) |
| UC-03 Resource Definition registration API/UI and verified Terraform registry contract inspection | [`backend/internal/application/catalog`](../../backend/internal/application/catalog/), [`backend/internal/delivery/http/resource_definitions.go`](../../backend/internal/delivery/http/resource_definitions.go), [`registry inspector`](../../backend/internal/adapters/terraform/registry/inspect.go), [`frontend/src/features/platform/ResourceDefinitionsPage.tsx`](../../frontend/src/features/platform/ResourceDefinitionsPage.tsx) |
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
| UC-16 per-container Application-key checklists, aliases and reference validation | [`ApplicationKeyPicker`](../../frontend/src/features/workloads/ApplicationKeyPicker.tsx), [`bindings`](../../frontend/src/features/workloads/bindings.ts), [`editor`](../../frontend/src/features/workloads/WorkloadEditorPage.tsx) and [`selection/scope regressions`](../../frontend/src/features/workloads/WorkloadEditorKeys.test.tsx) |
| UC-16 Resource Type input form and Score params mapping | [`frontend/src/features/workloads/WorkloadEditorPage.tsx`](../../frontend/src/features/workloads/WorkloadEditorPage.tsx) and [`backend/internal/domain/resource/contract.go`](../../backend/internal/domain/resource/contract.go) |
| Reference-based deployed Score reconstruction | [`backend/internal/application/workloadconfig/reconstruct.go`](../../backend/internal/application/workloadconfig/reconstruct.go) |
| UC-12 Vault bundle, VSO synchronization, `secretKeyRef` rendering and legacy Agent mode | [`backend/internal/adapters/vault/provider.go`](../../backend/internal/adapters/vault/provider.go), [`backend/internal/adapters/kubernetes/vso.go`](../../backend/internal/adapters/kubernetes/vso.go) and [`backend/internal/adapters/kubernetes/renderer.go`](../../backend/internal/adapters/kubernetes/renderer.go) |
| Application Preview/Deploy UI | [`frontend/src/features/applications/ApplicationHomePage.tsx`](../../frontend/src/features/applications/ApplicationHomePage.tsx) |
| UC-09 recent deployments, Environment history/filter and detail UI | [`frontend/src/features/deployments`](../../frontend/src/features/deployments/) |
| T15 Vietnamese persisted deployment details, safe failure/redaction and scoped late-response guards | [`detail page`](../../frontend/src/features/deployments/DeploymentDetailsPage.tsx), [`state tests`](../../frontend/src/features/deployments/DeploymentPages.test.tsx), [`T15 scenario`](../../frontend/test/e2e/refactor-local.mjs); [verification](../verification/2026-10-11-T15-deployment-details.md) |
| UC-09 consistent scoped query, workload history and local browser verification | [`query.go`](../../backend/internal/application/deployment/query.go), [`persistence ports`](../../backend/internal/ports/persistence/persistence.go), [`adapter contract tests`](../../backend/internal/ports/persistence/persistencetest/), [`uc09-local.mjs`](../../frontend/test/e2e/uc09-local.mjs) and [`local runner`](../../backend/test/integration/uc09-playwright-local.sh) |
| Acceptance workloads | [`backend/examples/acceptance-app`](../../backend/examples/acceptance-app/) |
| Local Compose Vault bootstrap and explicit platform store seed | [`docker-compose.yml`](../../docker-compose.yml), [`start-vault.sh`](../../deploy/local/start-vault.sh), [`application policy`](../../deploy/local/applications-policy.hcl), [`connection policy`](../../deploy/local/connections-policy.hcl) and [`normal-store bootstrap`](../../backend/internal/bootstrap/platformvault.go), [`shared managed registration`](../../backend/internal/application/secretstores/bootstrap.go) and [`scoped runtime registry`](../../backend/internal/adapters/vault/registry.go) |
| Orchestrator container images (API with kubectl, Web Console on nginx) | [`backend/Dockerfile`](../../backend/Dockerfile), [`backend/deploy/docker-entrypoint.sh`](../../backend/deploy/docker-entrypoint.sh), [`frontend/Dockerfile`](../../frontend/Dockerfile) and [`frontend/deploy/nginx.conf.template`](../../frontend/deploy/nginx.conf.template) |
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
  [`backend/internal/planning/score/resources.go`](../../backend/internal/planning/score/resources.go);
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
selection/invalid-target retention. [Key picker](../../frontend/src/features/workloads/ApplicationKeyPicker.tsx)
and [bindings](../../frontend/src/features/workloads/bindings.ts) supply Vietnamese
labels/validation with reference payloads unchanged. [Editor tests](../../frontend/src/features/workloads/WorkloadEditorPage.test.tsx)
and [key regressions](../../frontend/src/features/workloads/WorkloadEditorKeys.test.tsx)
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
[HTTP tests](../../backend/test/e2e/terraform_connection_http_test.go) và
[new product seeds](../../backend/internal/seed/seed.go).
UC-06 BR-20/UC-08 BR-07 → unchanged matcher guard,
[connection tests](../../backend/internal/planning/connection_binding_test.go),
[effective account/prior-state validation](../../backend/internal/application/provisioning/service.go)
và [provision tests](../../backend/internal/application/provisioning/service_test.go).
[Console form](../../frontend/src/features/platform/ResourceDefinitionsPage.tsx)
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
- [Console selector](../../frontend/src/features/platform/RenderingTemplatesPage.tsx)
  and [unit guards](../../frontend/src/features/platform/RenderingTemplatesPage.test.tsx).
- [Headed T18/T03 scenarios](../../frontend/test/e2e/refactor-local.mjs),
  [fake runner](../../backend/test/integration/refactor-ui-local.sh),
  [local evidence](../verification/2026-10-11-T18-render-bundles.md).

## T19 single-context matching preview

- [Catalog read service](../../backend/internal/application/catalog/matching_preview.go)
  và [tests](../../backend/internal/application/catalog/matching_preview_test.go):
  one scoped ReadSnapshot, in-memory candidate replacement, domain weights và
  winning ConnectionMismatch guard, deterministic snapshot marker; no writes.
- [HTTP route](../../backend/internal/delivery/http/resource_definitions.go),
  [safe boundary tests](../../backend/internal/delivery/http/matching_preview_test.go).
- [Shared panel](../../frontend/src/features/platform/MatchingPreviewPanel.tsx),
  [panel/integration tests](../../frontend/src/features/platform/MatchingPreviewPanel.test.tsx),
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
[`Score tests`](../../backend/internal/planning/score/score_test.go),
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
support/CTA and [Definition form](../../frontend/src/features/platform/ResourceDefinitionsPage.tsx).
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
