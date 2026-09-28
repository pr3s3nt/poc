---
id: VER-2026-09-28-PUBLIC-INGRESS-KIND
artifact: verification-evidence
status: current
last_reviewed: 2026-09-28
---

# Public Ingress on kind — 2026-09-28

Run `route-20260928100656-22299` on `kind-idp-internal` passed
`backend/test/integration/public-ingress-kind-verify.sh`. It created a new
Application and staging namespace, saved a workload with `service.publicPort`,
Previewed and Deployed it, observed a Ready Pod and Traefik Ingress targeting
the selected Service port, then received `ingress-ok` via HTTP through a local
port-forward with the desired Host header. Removing the workload removed its
Ingress. The run-scoped namespace was deleted (`status=0`).

This check did not prove DNS resolution, TLS, permanent Windows-host access,
AWS ingress, or the combined Fleet + Ingress path. Traefik remained a
ClusterIP Service and no controller installation/change was performed.
