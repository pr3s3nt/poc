---
id: ADR-037
artifact: architecture-decision
status: current
last_reviewed: 2026-11-05
related: [ADR-024, ADR-027, ADR-035, ADR-036]
---

# Retire public routes and Application subdomains; endpoint is the only address

## Accepted decision — 2026-11-05

The user decided that the public endpoint Resource Type
([contract](../contracts/endpoint.md)) is the only way to expose a workload.
The system has no users yet, so retired behavior is removed without a
conversion path. Retired together:

1. **`service.publicRoutes`** (UC-16 BR-12, UC-06 public-route rules). Workload
   documents and drafts that declare it are rejected; the field is no longer part
   of the Generic Workload or the Console workload form.
2. **Platform route manager** (ADR-024 public-route part): backend flags
   `-public-route-kind`, `-public-gateway`, `-public-base-domain`,
   `-public-ingress-class`, their environment variables, the `orch-public`
   Ingress/HTTPRoute object, the `_routes` delivery unit, `publicRoutesPending`
   and the route retry action. Other ADR-024 open-workload-object rules stay.
3. **Template-generated routes**: the standard template's `httproute.tpl`, its
   `httpRoute*` inputs and the `routing.orchestrator.dev/*` annotations. A
   template must not render HTTPRoute/Ingress for an endpoint workload (EP-04 guard
   stays). Older template sources pinned by commit remain immutable history.
4. **Application subdomain** (UC-01 MS-02/BR-03): creation no longer takes or
   stores a subdomain, and the Console no longer shows `<subdomain>.<base>`
   addresses. The Application page shows the actual endpoint URLs of each
   Environment. Application name and immutable key remain the identities.

## Endpoint path sharing (EP-07)

Workloads of one Application Environment may share one endpoint hostname with
distinct path prefixes (for example `shop` `/` for frontend and `shop` `/api` for
backend). A hostname belongs to exactly one Organization/Application/Environment/
target generation; each (hostname, path) belongs to exactly one workload. Another
Environment or Application cannot claim a held hostname, and two workloads cannot
claim the same hostname and path. Each workload still declares at most one
endpoint. Each workload emits its own HTTPRoute; the Gateway selects the longest
matching prefix. Release keeps the hostname while any path of its owner remains.

## Data and upgrade

Schema version 10 (ADR-027) is an owner-only transition:

- replace `endpoint_reservations` with `endpoint_hosts` (hostname owner) and
  `endpoint_paths` (hostname, path → workload). Each version-9 reservation becomes
  one host plus a `/` path claim of its workload: v9 granted the whole hostname,
  so this preserves the held claim; the next acknowledged deploy narrows it;
- drop `applications.subdomain` and its unique index;
- drop `environments.public_routes_pending`;
- remove `service.publicRoutes` from stored workload drafts.

Immutable Deployment Sets, plans and receipts are not rewritten. A historical set
that still carries `publicRoutes` is read without it. Delivery phase `ROUTE` stays
readable for historical receipts but is never produced. Runtime performs no DDL;
Console CRUD only on the two endpoint tables, no State grant.

## Consequences

- Existing `orch-public` objects on retained development clusters become
  unmanaged; operators delete them with an ownership check. No product cleanup is
  added for them.
- Bootstrap inputs drop `application.subdomain`; staging Gateway parameters live
  only on the endpoint Definition.
- Account diagnostics drop the "https" use case (`k8s.route.controller`,
  `uc.https.*`): it inspected the platform route and subdomain host. Stored
  reports that contain it stay readable; it is no longer selectable.
- Earlier verification records that used public routes or subdomain addresses are
  historical and are not re-run.

## Validation

PostgreSQL upgrade test from populated version 9 (reservation, subdomain, pending
flag, draft with publicRoutes), rollback, repeat and grants; ledger test for
shared hostnames and path conflicts; existing tests adjusted to the retired
behavior, retired-feature tests removed. The user asked to keep testing lean: a
live run with two workloads on one hostname is not part of this change.
