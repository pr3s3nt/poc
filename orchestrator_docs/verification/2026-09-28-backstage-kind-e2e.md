---
id: VERIFICATION-BACKSTAGE-KIND-E2E-20260928
artifact: verification-record
status: current
last_reviewed: 2026-09-28
---

# Backstage through Orchestrator on kind — 2026-09-28

- Script: `bash backend/test/integration/backstage-kind-verify.sh`.
- Context: `kind-idp-internal`; direct workload delivery. Image
  `ghcr.io/backstage/backstage:1.53.1` was preloaded into the amd64 kind node.
- Run ID: `backstage-20260928162951-30267`; Application created over HTTP,
  staging Environment namespace
  `app-03bf50dd-ae0d-479a-a8db-2acdbc593426-staging`.
- The test saved the public URL and guest-auth test flag as UC-12 Variables in
  Vault; UC-16 Score referenced those keys and PostgreSQL resource outputs.
  VSO supplied the configuration as a namespace-local Secret. No raw password
  was included in the Score fixture.
- Preview reported one `DEPLOY` change. Deploy status was `SUCCEEDED`;
  PostgreSQL StatefulSet and Backstage Deployment were Ready. UC-09 view
  contained `k8s-cluster`, `k8s-namespace` and `postgres` resources, with the
  Backstage workload `READY`.
- Through the Traefik Ingress with the staging Host header, `/` returned the
  Backstage HTML, `/.backstage/health/v1/readiness` passed, and
  `/api/auth/guest/refresh` returned a nonempty guest identity token. The
  token was validated in a pipe and not saved in the repository.
- The run-scoped namespace, including PostgreSQL PVC, was deleted. The
  preloaded image remains on the kind node. This does not verify Fleet mode,
  Windows-host DNS/TLS, or a long-lived deployment.

Guest sign-in is enabled here only for local verification; the production
image's guest provider normally refuses non-development use.
