---
id: VER-ENDPOINT-DOMAIN-20261105
artifact: verification-record
status: current
last_reviewed: 2026-11-05
---

# Public endpoint → hostname ledger → Fleet → Traefik → real domain

The user approved continuing the endpoint work on `main`, live acceptance on dev
kind-k8s-4f, deleting the retained httproute-domain test Application (it held
the only real test hostname) and upgrading the isolated `application-fleet-live`
installation from schema 8 to 9. No company cluster was used.
[Contract](../architecture/contracts/endpoint.md),
[scenario](../../e2e/scenarios/endpoint-domain/README.md).

## Schema 9 upgrade on retained data

Owner `pg_dump` backup was taken first (outside Git). Images
`orchestrator-backend:endpoint-635c417b` / `orchestrator-frontend:endpoint-635c417b`
restarted the same Compose project; its owner setup upgraded v8 → v9 in place:
`orchestrator-schema-v9`, history version 9, installation ID unchanged,
`endpoint_reservations` present. Row fingerprints of applications, environments,
deployments, active resources and Terraform states are identical before/after.

## Old test Application cleanup

Application 606223b6-2ea7-4604-badd-e895291ac3e4 was deleted through the Console
deletion dialog. The first attempt removed its workload/route and stopped at
CLEANUP_INCOMPLETE because the setup-owned `bootstrap-fleet-access`
Role/RoleBinding were not removed first (runner ordering error). Both were
deleted with UID preconditions after owner-label checks; the product retry
SUCCEEDED, the namespace is absent, no route keeps the hostname and Gateway UID
cb646fd4-9904-4b88-aabf-54e013d5c140 is unchanged.

## Result: PASS

Application 8736557e-e1f2-45fb-b4f8-9f7672c89805 (`endpoint-domain-20261105`),
namespace app-8736557e-e1f2-45fb-b4f8-9f7672c89805-staging. PE registered the
`endpoint` Type and app-scoped Definitions through Console forms (including the
bootstrap endpoint form filler); policy root `tail342836.ts.net`, scheme http,
Gateway httproute-domain-gateway/httproute-domain-test, listener web.

| Phase | Hostname | Ledger after Deploy | Browser on real domain |
|---|---|---|---|
| publish | desktop-bdjp2rq-1.tail342836.ts.net | that hostname only | 200, nginx page |
| rename | endpoint-renamed.tail342836.ts.net | renamed hostname only | 404 from Traefik |
| restore | desktop-bdjp2rq-1.tail342836.ts.net | original hostname only | 200, nginx page |

Every Preview changed only frontend and showed the exact URL in the Console
"Địa chỉ public" panel. The single HTTPRoute `endpoint-09b97ec2227bced1d981`
(UID a03d1e2a-bf6e-4426-bf20-10288c849f35) kept its name/UID through generations
1–3, was Accepted/ResolvedRefs each time, targeted frontend:80 and was present
in the actual Fleet bundle. Deployments 8a08fa08-b14b-4efd-bdca-b480e119bc91
(rename) and 619a80ae-d61a-4b48-bc82-083cfc9f231f (restore) SUCCEEDED.

Live runs exposed and fixed: the bootstrap scheme locator (a select's name
includes its option in Chromium), a shared runtime identity that never reopened
a receipt with an omitted optional field, and browser cache masking the 404
check (fresh contexts now). One rename verification was resumed from its
already SUCCEEDED deploy; no deploy was repeated.

Not verified: HTTPS/certificates, company Gateway/DNS, and the Preview graph,
which places the endpoint node under "Vị trí chưa xác định" (cosmetic).

## Retention

App, Definitions, namespace RBAC, route and the existing Traefik forward plus
Tailscale port 18095 remain for review. The acceptance-fleet app on the same
stack keeps its data, but its template runtime pin belongs to the previous
binary: run `refresh-runtime.mjs` before its next Preview. Private config,
backup, logs and screenshots: `~/.local/state/endpoint-live-20261105/`.
Restart with that directory's `start-live.sh`; keep the Compose volumes.
