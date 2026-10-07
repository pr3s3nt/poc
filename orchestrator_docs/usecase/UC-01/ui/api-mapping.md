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
| Set once | `PUT /api/v1/applications/{app}/environments/{env}/connection` body `{connectionKey, expectedVersion}` | Persist Environment target once and return safe Environment view. 400 invalid key/version/body; 404 scoped app/env; 422 unavailable/foreign/nonREADY/unsupported/AWS-without-region key; 409 already set (including same key) or stale version. |

Client retains form on failure, reloads authoritative Environment after 409 and
renders locked binding when winner exists. On unavailable choice refresh, clear
removed selection, never silently substitute default. Ignore late replies from
another app/env or unmounted screen. Choices authorization does not grant UC-04
credential management. Preview/Deploy consume stored Environment target only;
UNCONFIGURED failure 422 field connectionKey links to Settings.
