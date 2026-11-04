---
id: VER-HTTPROUTE-DOMAIN-20261105
artifact: verification-record
status: current
last_reviewed: 2026-11-05
---

# HTTPRoute template → Fleet → Traefik → real domain

User requested live domain verification and explicitly approved installing
Gateway API plus a dedicated Traefik/Gateway in existing kind-k8s-4f. No company
cluster was used. Initial discovery confirmed no Gateway API resource types and
no Traefik controller there. The app and preexisting PVCs were not modified.

Source under test: 57e98d94b2ff76c123b14be6414539d8dd16665b, Git template path
projects/platform-resources/templates/standard, content SHA256
e609e15a4c2f28ffb7e6d82b2fc9ab358028879bfbdf564741e2a948bf8d152c.
[Contract/setup](../../projects/platform-resources/bootstrap/HTTPROUTE.md),
[reproducible scenario](../../e2e/scenarios/httproute-domain/README.md).

## Result: PASS

- Gateway API v1.6.3; Traefik v3.7.17, Helm chart 42.3.0, dedicated namespace
  httproute-domain-test. Gateway httproute-domain-gateway, UID
  cb646fd4-9904-4b88-aabf-54e013d5c140; listener web (HTTP 8000).
- Application 606223b6-2ea7-4604-badd-e895291ac3e4, namespace
  app-606223b6-2ea7-4604-badd-e895291ac3e4-staging, namespace UID
  51ebc07a-9097-4a02-98e3-80f41b2efe94. A single nginx frontend was created and
  Preview/Deployed through Console controls, using app-scoped Definitions and
  the actual pinned Git workload template; no direct workload/route apply.
- Deployment 5d7cd2f9-45a7-453f-9ee2-b34cc54c1092 SUCCEEDED. HTTPRoute frontend
  owned by this app/workload, current generation 1: Accepted=True and
  ResolvedRefs=True, controller traefik.io/gateway-controller, correct parent
  namespace/name/listener, hostname and frontend Service backend port 80.
- Actual Fleet bundle contains httproute-e23216c019846efa3fd9.yaml at commit
  5fd0444f81f0d0faf000f88e6e4d8eb14f006705; rollout bundle at commit
  9497061537ebcf0dc3eb41181fb9555608b07f61. Both GitRepo observed commits match
  exact desired pins and Bundles are Ready 1/1.
- Playwright opened
  [the real domain](http://desktop-bdjp2rq-1.tail342836.ts.net:18095/): HTTP 200,
  visible Welcome to nginx! heading. No Host override or DNS override. Windows
  Invoke-WebRequest to the same URL also returned 200 and the welcome content.
- Previous acceptance PVC UID 8723c8eb-9afc-4eeb-93ec-728c64a36877 remains Bound
  and unchanged. No previous app/namespace/PVC deletion or data reset.

Traffic path: Tailscale DNS/HTTP Serve :18095 → retained local forward to
Traefik Service → Gateway/HTTPRoute → frontend Service → nginx Pod. The forward
is to Traefik, not directly to the app. This proves HTTP routing on this dev
installation; not HTTPS/certificate or real company staging verification.

## Evidence and retention

Private run directory /home/thanhnt1/.local/state/httproute-domain-20261105/
contains private input references, installation receipt/logs, state, exact
native observations, screenshot and final-success video. No credential values,
kubeconfig data, cookies or Terraform state are published here. The short video
shows successful domain navigation/content, not the earlier Console credential
entry or every setup step. One pre-run config validation failure was corrected
before product mutation; the live deploy itself completed once without retry.

App, scoped namespace RBAC, Git/CD repos, Gateway/Traefik/CRDs, Service forward
and only the new Tailscale HTTP port 18095 remain for user review. Shared
Tailscale routes are preserved. Cleanup is separate: product remove and
ownership/UID checks; never bulk-delete cluster CRDs or previous app resources.
Source runner syntax and documentation checks are the relevant code checks;
no unrelated full-suite reruns or second deployment were performed.
