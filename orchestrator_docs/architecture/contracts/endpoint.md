---
id: ENDPOINT-CONTRACT
artifact: shared-contract
status: current
last_reviewed: 2026-11-05
---

# Public endpoint resource

EP-01 (UC-03/16): `endpoint` is a private workload Resource Type. Developer params
are `name` (one lowercase DNS label), `port` (named Service port), `path`
(absolute prefix, default `/`). One endpoint per workload; shared IDs are refused.
Outputs are nonsecret `hostname` and `url`. No Gateway fields in workload input.

EP-02 (UC-03): built-in `endpoint` driver has no account or Terraform source.
Definition variables contain literal `rootDomain`, `scheme` (`http`/`https`),
`gatewayName`, `gatewayNamespace`, optional `gatewaySectionName`, and optional
`reservedNames` (DNS labels already used outside this installation). These are
policy, never overridden by resource params. Matching/version pins stay standard.
Infrastructure owns Gateway, Traefik, DNS and TLS. Delegate an exclusive wildcard
subdomain to this installation, or inventory all external names in reservedNames
before admitting endpoints; the database cannot detect unrelated cluster routes.

EP-03 (UC-05/08): planning validates policy, port and path and exposes prospective
hostname/url. Preview checks current reservations without writes; availability
is advisory. Deploy atomically reserves hostname, globally within the database,
for Organization/Application/Environment/generation/workload. Exactly one owner
wins concurrent claims. Same-owner retries are idempotent. No automatic TTL.

EP-04 (UC-06): built-in routing emits HTTPRoute into the workload's normal manifest
publication/inventory, using the actual rendered Service name and numeric port.
No Terraform Kubernetes write and no second route publisher. Reject missing or
ambiguous Services, legacy publicRoutes and template-generated routes when an
endpoint is requested. Gateway must already allow the workload namespace.

EP-05 (UC-07): retain old and new hostname claims across failed/unknown delivery.
Release obsolete claims only after successful replacement/pruning acknowledgement,
or after confirmed workload removal. Removal failure never frees a hostname.
A failed reservation may remain conservatively held until successful retry/removal;
no automatic adoption or hostname transfer. Existing pinned legacy route templates
continue working; migration requires an explicit workload/template change.

EP-06: schema v9 adds endpoint_reservations; owner setup upgrades populated v8
without changing existing product rows, installation identity, encrypted state or
PVCs. Runtime performs no DDL. Console owns reservation CRUD; State has no access.
