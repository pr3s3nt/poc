---
id: UC-01-UI
artifact: use-case-ui-design
status: current
last_reviewed: 2026-10-07
related: UC-01, UC-05, UC-07, UC-09, UC-16
---

# UC-01 UI — Application onboarding and home

## UX outcome

Developer lands on Applications home after sign-in, creates an Application with
Name/Subdomain, then sees a single Application home with `staging` and
`production` tabs. The selected Environment displays its Workloads and recent
deployments. Connection is set exactly once in each Environment Settings; see [screens](screens.md).

Add/Edit/Delete workload affordances are visible in this home because that is
where a Developer expects them. Their configuration behavior belongs to
[UC-16](../../UC-16/ui/README.md); preview belongs to UC-05 and runtime
update/removal to UC-07. M00-a keeps the affordances unavailable until the
respective flows are implemented.

## Read in this order

1. [Screens](screens.md)
2. [States](states.md)
3. [API mapping](api-mapping.md)
4. [Wireframe](wireframe.puml)
5. [Shared shell](../../../architecture/ui/README.md)
