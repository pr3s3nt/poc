---
id: VER-DEV-ENDPOINT-BOOTSTRAP-20261105
artifact: verification-record
status: current
last_reviewed: 2026-11-05
---

# Dev bootstrap acceptance app on a public endpoint

User-approved run of the git dev target (`scripts/bootstrap/git-targets/dev.json`,
assets pinned at `544caafa`) on dev kind k8s-4f after
[ADR-037](../architecture/decisions/ADR-037-retire-public-routes.md). The prior
`endpoint-domain-20261105` test app was deleted through the Console (setup RBAC
UID-deleted first) to free the only real test hostname.

Isolated Compose project `endpoint-dev` (images `ae692ada`, fresh schema 10),
Console on the Tailscale IP port 3050 through a local TCP proxy (Serve needs
sudo). Bootstrap `check` then `all` SUCCEEDED: platform, Application
`acceptance-endpoint-dev` (30b057fe-258b-40b9-a7d0-6f771bcc9eb1), Preview/Deploy,
Fleet, PVC file and business checks.

Result: one HTTPRoute `endpoint-a2004cb1ad96578201bb` (Accepted) for
`desktop-bdjp2rq-1.tail342836.ts.net` → Service `frontend:8080`; ledger holds
that hostname `/` for frontend. Through Tailscale :18095 → Traefik Gateway the
acceptance page answers 200 with backend, environment, secret, database and
cache PASS and a DONE persistent job. The Console Application page lists the
endpoint URL. HTTPS and company staging are not verified.

Retained: stack, app, routes, proxy and forwards for review. Private config,
logs and screenshots: `~/.local/state/endpoint-dev-20261105/`.
