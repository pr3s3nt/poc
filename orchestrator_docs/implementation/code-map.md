---
id: IMPLEMENTATION-CODE-MAP
artifact: design-to-code-map
status: current
last_reviewed: 2026-10-12
---

# Design-to-code map

| Design concern | Implementation entry point |
|---|---|
| T06 Vietnamese Connections, automatic inspect, name suggestion ownership and stale response guards | [`page`](../../frontend/src/features/platform/ConnectionsPage.tsx), [`tests`](../../frontend/src/features/platform/ConnectionsPage.test.tsx), [`headed scenario`](../../frontend/test/e2e/refactor-local.mjs), [`local runner`](../../backend/test/integration/refactor-ui-local.sh) |
| Refactor Console local smoke and Vietnamese locator foundation | [`runner`](../../backend/test/integration/refactor-ui-local.sh), [`scenario registry`](../../frontend/test/e2e/refactor-local.mjs), [`page locators`](../../frontend/test/e2e/locators.mjs), [`human input`](../../frontend/test/e2e/human.mjs) and [`captions`](../../frontend/test/e2e/captions.mjs) |
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
| Normalized PostgreSQL store and migrations | [`backend/internal/adapters/postgres`](../../backend/internal/adapters/postgres/) |
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
| T04 Vietnamese Resource Types details/copy, support whitelist and Definition prefill | [`types page`](../../frontend/src/features/platform/ResourceTypesPage.tsx), [`types tests`](../../frontend/src/features/platform/ResourceTypesPage.test.tsx), [`shared support table`](../../frontend/src/features/platform/resourceSupport.ts), [`route query`](../../frontend/src/app/routes.ts), [`Definition form`](../../frontend/src/features/platform/ResourceDefinitionsPage.tsx), [`local scenario`](../../frontend/test/e2e/refactor-local.mjs) |
| T03 Vietnamese rendering templates page, PE/Admin route gate and unchanged renderer registration payload | [`templates page`](../../frontend/src/features/platform/RenderingTemplatesPage.tsx), [`page tests`](../../frontend/src/features/platform/RenderingTemplatesPage.test.tsx), [`App role gates`](../../frontend/src/app/App.test.tsx), [`local scenario`](../../frontend/test/e2e/refactor-local.mjs), [`failure stub`](../../backend/test/integration/stubs/score-k8s-render-fail-stub.sh), [`live template locator`](../../frontend/test/e2e/template-engine-kind-human.mjs) |
| UC-03 Resource Definition registration API/UI and embedded Terraform contract inspection | [`backend/internal/application/catalog`](../../backend/internal/application/catalog/), [`backend/internal/delivery/http/resource_definitions.go`](../../backend/internal/delivery/http/resource_definitions.go), [`backend/internal/adapters/terraform/inspect.go`](../../backend/internal/adapters/terraform/inspect.go), [`frontend/src/features/platform/ResourceDefinitionsPage.tsx`](../../frontend/src/features/platform/ResourceDefinitionsPage.tsx) |
| UC-04 upload-only kubeconfig inspect/register and fail-closed scoped execution | [`backend/internal/application/connection`](../../backend/internal/application/connection/), [`backend/internal/adapters/kubernetes/connection_verifier.go`](../../backend/internal/adapters/kubernetes/connection_verifier.go), [`backend/internal/delivery/http/connections.go`](../../backend/internal/delivery/http/connections.go), [`frontend/src/features/platform/ConnectionsPage.tsx`](../../frontend/src/features/platform/ConnectionsPage.tsx) |
| UC-04 kubeconfig normalization and safe parser errors | [`parser`](../../backend/internal/application/connection/parser.go), [`parser tests`](../../backend/internal/application/connection/parser_test.go) |
| UC-04 scoped credential persistence and rollback | [`port`](../../backend/internal/ports/credentials/credentials.go), [`Vault KV v2`](../../backend/internal/adapters/vault/connection_credentials.go), [`explicit local memory adapter`](../../backend/internal/adapters/credentialmemory/store.go) |
| UC-04 execution credential resolution and private kubectl invocation | [`resolver`](../../backend/internal/application/connection/resolver.go), [`kubectl transport`](../../backend/internal/adapters/kubernetes/kubectl.go), [`leak tests`](../../backend/internal/adapters/kubernetes/kubectl_leak_test.go), [`restart tests`](../../backend/internal/application/deployment/credential_restart_test.go) |
| UC-04 Connection migration/backfill and concurrent upload | [`migration 5`](../../backend/internal/adapters/postgres/store.go), [`migration test`](../../backend/internal/adapters/postgres/connection_migration_integration_test.go), [`registration race/reopen`](../../backend/internal/application/connection/postgres_integration_test.go) |
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
Prior Application-selection evidence above is historical; scripts now follow Environment Settings.

## Environment binding implementation (ADR-011)

- [Domain](../../backend/internal/domain/environment/environment.go), [binding service](../../backend/internal/application/application/service.go), [target resolver](../../backend/internal/application/target/target.go).
- [PostgreSQL migration 6](../../backend/internal/adapters/postgres/store.go), [SQL binding](../../backend/internal/adapters/postgres/repositories.go), [JSON binding/load](../../backend/internal/adapters/store/store.go), [adapter contract](../../backend/internal/ports/persistence/persistencetest/binding.go).
- [Settings connection section](../../frontend/src/features/configuration/EnvironmentConnection.tsx), [Settings routes](../../frontend/src/app/routes.ts), [new AWS scope tests](../../backend/internal/planning/environment_scope_test.go).
- [Live runner](../../backend/test/integration/application-connection-kind-video.sh), [human scenario](../../frontend/test/e2e/application-connection-kind-human.mjs), [reviewed evidence/video](../verification/2026-10-07-environment-connection-kind.md).


## Environment stores and transitions (ADR-012)

This is code navigation. Current limits remain in CURRENT_STATE; first-delivery
live evidence is in the [reviewed recording](../verification/2026-10-08-environment-stores-kind.md).

- [Secret Store domain](../../backend/internal/domain/secretstore/), [registration service](../../backend/internal/application/secretstores/), [Vault registry/verifier](../../backend/internal/adapters/vault/), [registration UI](../../frontend/src/features/platform/SecretStoresPage.tsx).
- [Configuration and copy-first store switch](../../backend/internal/application/configuration/), [store selection UI](../../frontend/src/features/environment/SecretStoreSelection.tsx), [VSO delivery](../../backend/internal/adapters/kubernetes/vso.go).
- [Operation manager and fencing tests](../../backend/internal/application/envops/), [SQL operations](../../backend/internal/adapters/postgres/operations.go), [JSON operations](../../backend/internal/adapters/store/ops.go), [persistence contracts](../../backend/internal/ports/persistence/persistence.go).
- [Transition service/tests](../../backend/internal/application/transition/), [real Kubernetes scale/backup/restore/cleanup adapter](../../backend/internal/adapters/kubernetes/transition.go), [HTTP routes](../../backend/internal/delivery/http/transitions.go).
- [Transition UI](../../frontend/src/features/environment/TransitionPanel.tsx), [operation/recovery UI](../../frontend/src/features/environment/OperationBanner.tsx), [frontend transition tests](../../frontend/src/features/environment/environment.test.tsx).
- [Generation identity tests](../../backend/internal/planning/generation_test.go), [namespace generation tests](../../backend/internal/domain/environment/generation_test.go), [Terraform executor](../../backend/internal/adapters/terraform/executor.go).
- [Human real-kind recorder](../../backend/test/integration/environment-stores-kind-video.sh), [browser scenario](../../frontend/test/e2e/environment-stores-kind-human.mjs), [ownership identity observer](../../backend/test/integration/runidentity/main.go), [masked-input leak checks](../../frontend/test/e2e/secret-scan.mjs).

## Implicit internal cluster (ADR-013)

- [Trusted Definition and binding validation](../../backend/internal/planning/builtin.go),
  [default matcher bypass](../../backend/internal/planning/match.go) and
  [planner contract tests](../../backend/internal/planning/connection_binding_test.go).
- [Idempotent execution admission](../../backend/internal/application/provisioning/builtin.go),
  [execution/Connection checks](../../backend/internal/application/provisioning/service.go),
  [race/collision/FK ordering tests](../../backend/internal/application/provisioning/builtin_test.go).
- [Read-only Preview projection](../../backend/internal/application/preview/service.go),
  [pure Preview tests](../../backend/internal/application/preview/service_test.go),
  [Console binding display](../../frontend/src/features/preview/ScorePreviewPage.tsx).
- [JSON reopen/retry/remove tests](../../backend/internal/application/deployment/builtin_cluster_test.go),
  [stored-target guard](../../backend/internal/application/deployment/service_restore_test.go),
  [public existing-cluster/reserved-key guard](../../backend/internal/application/catalog/service.go),
  [legacy seed preservation](../../backend/internal/seed/binding_test.go).
- [Explicit reference compatibility](../../backend/test/conformance/loader.go),
  [local browser flow](../../frontend/test/e2e/application-connection-local.mjs) and
  [local execution evidence](../verification/2026-10-09-implicit-existing-cluster-local.md).
- [Real kind runner](../../backend/test/integration/application-connection-kind-video.sh)
  and [human browser flow](../../frontend/test/e2e/application-connection-kind-human.mjs)
  support an exact staging Connection name, isolated workload Vault and
  ownership-checked cleanup.
- [Retained Console Connection helper](../../frontend/test/e2e/k8s4f-register.mjs)
  and [optional kind networking](../../deploy/local/compose.kind.yml) support
  private kubeconfig registration/reuse from the Docker Console.
- [Retained Docker Console dev runner](../../backend/test/integration/k8s4f-dev-playwright.sh),
  [human scenario](../../frontend/test/e2e/k8s4f-dev-human.mjs),
  [workload Vault setup](../../deploy/local/workload-vault/start.sh) and
  [port-forward supervisor](../../backend/test/integration/retained-port-forward.sh)
  prepare and retain the new physical K8S-4F sample deployment.
- [Caption generation](../../frontend/test/e2e/captions.mjs),
  [ASS rendering](../../frontend/test/e2e/render-captions.mjs) and
  [two-session video assembly](../../backend/test/integration/k8s4f-dev-compose-video.sh)
  keep Vietnamese captions below the browser window; title cards disclose the
  separate recorded sessions.

## T07 — Environment Settings UI

[Environment Settings verification](../verification/2026-10-11-T07-environment-settings.md)
traces ENV-01 and VI-02/04/05. Draft-only single READY suggestions and explicit
Save live in [EnvironmentConnection](../../frontend/src/features/configuration/EnvironmentConnection.tsx)
and [SecretStoreSelection](../../frontend/src/features/environment/SecretStoreSelection.tsx).
[SettingsPage](../../frontend/src/features/configuration/SettingsPage.tsx) retains
open editors on refresh and scopes late replies; environment labels, transition
and operation components provide Vietnamese static/accessibility text.
[Local scenario T07](../../frontend/test/e2e/refactor-local.mjs) uses memory
state/fake adapters with explicitly marked projection mocks; it proves no live
migration. Backend error mapping remains T20; rename-reference rewrite remains T30.

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
