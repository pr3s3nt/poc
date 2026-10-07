---
id: UC-01-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-07
related: UC-01
---

# UC-01 UI states

| Screen | State | UI behavior |
|---|---|---|
| Applications home | Loading | Render list skeleton; preserve sidebar. |
| Applications home | Empty | Explain first Application and show `+ Create application`. |
| Applications home | API error | Show retryable callout; do not substitute seed/mock data. |
| Create Application | Connections loading/error/empty | Disable submit until choices loaded; retry errors without losing fields; empty state asks Platform Engineer to register a READY connection. Ignore stale/unmounted replies. |
| Create Application | Validation | Inline Name/Subdomain/Connection messages and URL preview remains visible. |
| Create Application | Submitting | `Creating application…`; prevent duplicate submit. |
| Create Application | API error | Retain entered fields and selected connection; show form-level error. Refresh choices on unavailable-target errors. Preserve a still-valid explicit selection; if it is removed, clear it and require a new explicit choice. Never replace a failed user-selected target with default automatically. |
| Create Application | Success | Navigate to Application home with `staging` selected and success callout. |
| Application home | No workloads | Explain that workload editing is coming next; keep disabled Add action visible. |
| Application home | No deployments | Show `No deployments yet` without treating it as an error. |
