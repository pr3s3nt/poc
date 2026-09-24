---
id: UC-01-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-09-23
related: UC-01
---

# UC-01 UI screens

## Applications home

This is the post-sign-in landing page. It uses the authenticated sidebar from
shared UI and has `Applications` selected.

| Area | Content and behavior |
|---|---|
| Header | `Your applications`, supportive copy, and primary `+ Create application` action. |
| Empty state | Explains that an Application creates both staging and production; CTA opens Create Application. |
| Application list | A card per Application with Name, production hostname and two concise Environment summaries. Selecting a card opens Application home. |

## Create Application

Use a focused page or right-side panel; it contains exactly these editable
fields:

| Field | Rule |
|---|---|
| Application name | Required; unique in the authenticated Organization. |
| Subdomain | Required DNS label; UI previews production and staging URLs from the platform base domain. |

The submit action reads `Create application`. The form does not expose profile,
connection, region, Environment creation, credential or infrastructure choices.

## Application home

After creation or selection, the Application page contains:

| Area | Content and behavior |
|---|---|
| Header | Application Name, production hostname and an `Open application` link only when UC-06 later reports a reachable endpoint. |
| Environment tabs | `Staging` and `Production`; changing tab scopes the Workload list and recent deployments. |
| Workloads | Name, status and action menu. `+ Add workload`, `Edit` and `Delete` are intentionally disabled with `Available when workload editing is enabled` until UC-16 is implemented; preview and runtime deployment remain separate flows. |
| Recent deployments | Compact list linking to UC-09 deployment detail when records exist. |
