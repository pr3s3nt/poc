---
id: RUNBOOK-PUBLIC-ROUTES
artifact: operations-runbook
status: current
last_reviewed: 2026-10-26
---

# Public routes: Ingress or Gateway API HTTPRoute

[ADR-024](../architecture/decisions/ADR-024-open-workload-objects.md) makes the
Environment public route configurable at backend start. The platform owns one
object named `orch-public` per Environment namespace; DNS, TLS certificates and
the Gateway or Ingress controller stay cluster setup.

| Flag | Environment variable | Default | Meaning |
|---|---|---|---|
| `-public-route-kind` | `ORCHESTRATOR_PUBLIC_ROUTE_KIND` | `ingress` | `ingress` or `httproute` |
| `-public-gateway` | `ORCHESTRATOR_PUBLIC_GATEWAY` | — | `namespace/name[/sectionName]`, required for `httproute` |
| `-public-base-domain` | `ORCHESTRATOR_PUBLIC_BASE_DOMAIN` | `example.com` | Host is `<subdomain>.<base>`; Environment `staging` uses `staging.<subdomain>.<base>` |
| `-public-ingress-class` | `ORCHESTRATOR_PUBLIC_INGRESS_CLASS` | `traefik` | `ingressClassName` of `ingress` routes |

## Staging cluster with a Gateway

1. Find the Gateway that serves browser traffic:
   `kubectl get gateway -A` (note namespace, name and the listener name).
2. The listener must accept routes from application namespaces
   (`allowedRoutes.namespaces.from: All`, or a selector matching them) and its
   `hostname` (if set) must cover `*.<base>`.
3. Point a wildcard DNS record `*.<base>` (or the exact hosts) to the Gateway
   address; TLS is a listener certificate on the Gateway.
4. Start the backend with, for example:

   ```
   ORCHESTRATOR_PUBLIC_ROUTE_KIND=httproute
   ORCHESTRATOR_PUBLIC_GATEWAY=gateway-system/public/https
   ORCHESTRATOR_PUBLIC_BASE_DOMAIN=staging.example.com
   ```

5. Deploy an Application whose Score declares `service.publicRoutes`. Check:
   `kubectl -n <namespace> get httproute orch-public -o yaml` shows the host and
   `status.parents[].conditions` `Accepted=True`; open `http://<host>/`.

Switching kinds removes the owned `orch-public` object of the previous kind on
the next route reconciliation. A route object not owned by the Environment is
never overwritten. Bundles may add further HTTPRoutes (header/canary routing);
the platform route stays the Environment default.

Real verification on kind (Envoy Gateway) is part of the
[workload manifests acceptance](../verification/2026-10-26-open-workload-objects.md).
