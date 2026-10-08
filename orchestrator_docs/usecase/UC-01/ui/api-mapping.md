---
id: UC-01-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-10-07
---

# UC-01 UI API mapping

Session owns Organization/role. Profiles/regions are derived from scoped
Connection; public payloads exclude credentials, SecretRef/config/verification.

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
