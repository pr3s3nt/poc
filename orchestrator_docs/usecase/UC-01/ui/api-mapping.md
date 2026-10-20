---
id: UC-01-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-10-18
---

# UC-01 UI API mapping

Session owns Organization/role. Current UC-01 V04–V06 Settings contract and
ADR-017 omit all deployment Connection/profile/region selection; placement belongs
to matched Definitions/references. Secret Store/configuration and fenced-operation
APIs remain; public payloads exclude credentials and private verification data.

## Historical deployment-binding table — superseded after V06.08

The table preserves ADR-011/012 navigation and endpoint provenance. Its
Application/Environment target fields, application-choice/default and deployment
connection/transition commands are retired. They are not current API options.
Secret-store/operation rows retain their contracts, as stated in the current
replacement section below.

| UI action | HTTP contract | Result/error |
|---|---|---|
| Applications home | `GET /api/v1/applications` | Session-scoped apps with Environment summaries/targets. |
| Create | `POST /api/v1/applications` body `{name, subdomain}` only | Application plus two UNCONFIGURED Environments. Unknown `connectionKey` rejected 400; Name/Subdomain validation and duplicate guards remain. |
| App home | `GET /api/v1/applications/{app}` | Environment entries include safe connectionKey, executionProfile, region, runtimeStatus, infrastructureScope and version; empty target reports UNCONFIGURED. No shared Application target. |
| Connection choices | `GET /api/v1/application-connections` | Existing safe scoped READY choices and eligible default marker retained, now consumed in Environment Settings. Default never persisted automatically. |
| Set/change before runtime | `PUT /api/v1/applications/{app}/environments/{env}/connection` body `{connectionKey, expectedVersion}` | Scoped CAS; 409 STALE_VERSION/ENVIRONMENT_BUSY, safe 422 unavailable Connection. Runtime changes use explicit transition flow. |
| Secret-store choices | `GET /api/v1/secret-store-choices` | Scoped READY safe store metadata. |
| Set/change store | `PUT /api/v1/applications/{app}/environments/{env}/secret-store` body `{secretStoreKey, expectedVersion, expectedConfigVersion}` | Copy/verify first, then atomic desired revision/selection commit; safe operation/error status. |
| Preview target transition | `POST /api/v1/applications/{app}/environments/{env}/connection-transition/preview` | Destination, mode/resource mapping; returns capability/impact and pinned token. |
| Execute transition | `POST /api/v1/applications/{app}/environments/{env}/connection-transitions` | Exact token and downtime acknowledgement where required; persisted progress and safe recovery status. |
| Refresh transition history/progress | `GET /api/v1/applications/{app}/environments/{env}/connection-transitions` and `GET .../connection-transitions/{transition}` | Safe persisted source/destination, stages, compensation and source retention; no backup bytes or credentials. |
| Operation status | `GET /api/v1/applications/{app}/environments/{env}/operations` | Scoped active/interrupted operation status; no automatic unlock. |
| Recover interrupted operation | `POST /api/v1/applications/{app}/environments/{env}/operations/{operation}/recover` body `{priorExecutionStopped: true}` | User explicitly confirms prior execution stopped; backend records session actor, fences previous owner and runs bounded recovery. Missing confirmation 422; nonrecoverable 409. Recovery failure preserves need for intervention. |
| Cleanup retained source | `POST /api/v1/applications/{app}/environments/{env}/connection-transitions/{transition}/cleanup-source` | Explicit action; rejects current/referenced or unowned source; resolves historical target and reports cleanup outcome. |

Client retains fields on errors, reloads authoritative Environment after 409 and
requires explicit resubmission. No silent default/retry or permanent lock. Scope
changes ignore late replies. Operation-owned busy state is temporary. Preview
and Deploy obey [ADR-012](../../../architecture/decisions/ADR-012-environment-stores-and-transitions.md).

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.


## V03 UI/API replacement

[ADR-016](../../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) defines current internal placement. Remove deployment Connection selection after replacement pass; keep Secret Store/configuration/version/error/operation state. Definition authoring supports existing-cluster account choice and workload namespace/cluster references, Type params and provision flags; consumers inherit account. Score import/edit/export preserves metadata.class. Preview shows each workload/resource Definition/account/cluster/namespace and provider-first graph/batches without secrets, and blocks missing/ambiguous/mismatched/unready inputs as appropriate. No Environment target is required. Retired deployment-Connection writes fail explicitly; safe matching and stale-token gates remain. UI multi-cluster V03 unsupported error occurs before execution. Progress/readiness/errors/update/remove use per-workload targets; failure never shown as success.


## V04–V06 final Settings replacement

After V06.08, Application/Environment reads omit retired deployment
Connection/profile/region/runtime selection fields; no synthetic defaults.
Create still accepts name/subdomain only. Deployment choice/set/transition
commands in the historical table are retired, not silently accepted or mapped to
a new target. Secret Store/configuration and operation APIs retain their existing
scope/version/owner/error contracts. Placement comes from matched Definition and
resource references in UC-05/06/08, not an Environment target payload.
