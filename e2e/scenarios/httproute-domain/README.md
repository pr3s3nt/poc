# HTTPRoute domain through Traefik Gateway API

Scope: [workload route contract](../../../projects/platform-resources/bootstrap/HTTPROUTE.md),
[Generic Workload](../../../orchestrator_docs/architecture/contracts/generic-workload.md),
[CD](../../../orchestrator_docs/operations/delivery-reconciliation.md).

One isolated frontend is created/imported/Previewed/Deployed through Playwright
Console controls. PE registers an app-scoped workload Definition using the real
pinned Git standard template and existing verified namespace/cluster capabilities.
No direct HTTPRoute/Deployment/Service apply or fake executor is used. Native
read-back verifies owned route UID, Gateway Accepted/ResolvedRefs at current
observed generation, backend Service/ready workload and exact Fleet commit.
A real browser opens the supplied DNS domain without Host overrides or resolver
mapping and saves only the final successful video/screenshot. HTTP and HTTPS
must be reported as actually exercised; no company Traefik claim from dev alone.

Prerequisites: explicit dev cluster/Git/CD mutation authorization, real Console,
READY account/store/Fleet default, existing Traefik Gateway API/Gateway/listener,
HTTPRoute v1 CRD, reachable pinned Git template and web image. Install E2E and
bootstrap dependencies per [shared guide](../../README.md). Test-only access adds
a bounded Role/RoleBinding in the product-created app namespace (including
httproutes); it does not change shared Fleet controller permissions. Traefik/CRD/
Gateway installation and domain exposure require separate target authorization;
this runner does not install them or change DNS/Tailscale settings.

Run from root with a private 0600 JSON config outside Git:

```sh
node e2e/scenarios/httproute-domain/run.mjs --config /private/httproute-domain/config.json
```

Config fields: `name`, `console`/`developer` (`url`, `username`, `passwordFile`),
`stateDir`, `reportDir`, `kubeconfigFile`, `context`, `accountKey`, `storeKey`,
`gateway` (`name`, `namespace`, `listener`), `url` (real DNS hostname), `image`,
`templateSource` (`name`, `kind=git`, `url`, `path`, `ref`, `pin` with revision and
contentSha256). Gateway references, domain and source identity are pinned into
the run receipt. Use a dedicated state/report directory, never reset another run.

Assertions: one configured frontend, one app-owned HTTPRoute emitted by the
Git template (correct source digest and rendering Definition), matching
Gateway/listener/hostname/backend Service and Service port, current route
Accepted/ResolvedRefs, Ready Deployment, exact pinned Fleet commit/Bundle Ready,
browser status 200 and nginx welcome page via the actual domain. It retains the
app/namespace/Git repo/route and reports their safe identities for review.

Unknown/failed deployment outcomes stop for operator reconciliation; no blind
resubmission/recovery. Reruns reuse only receipt-owned Application/input, no
forced adoption or overwrite. Cleanup is separately authorized product workload
remove → Preview/Deploy → Application delete after ownership/UID checks; delete
only receipt-owned namespace access, and verify route/namespace absence. Shared
accounts/store/controllers/CRDs/Gateway/Tailscale settings are not cleanup targets
of this runner. Evidence/logs/video/credential files stay outside Git.

## Authorized dev setup used on 2026-11-05

The user separately approved Gateway API/Traefik installation on kind-k8s-4f.
The fresh names were verified absent before creating anything. Gateway API
standard v1.6.3 manifest (SHA256
356c2b8e286a964dc20995435659ad1095386329974793af10bbfe3eff977fe5)
was installed from the official release; Traefik Helm chart 42.3.0 / image
v3.7.17 was installed into httproute-domain-test as release httproute-domain.
Values disabled Kubernetes Ingress/IngressRoute providers, default IngressClass
and dashboard; enabled Gateway API only, GatewayClass httproute-domain-traefik,
Gateway httproute-domain-gateway / listener web port 8000. Service is ClusterIP,
not a cloud LoadBalancer. Existing ingress controllers/routes were not adopted.
Gateway listener allows application namespaces for this dev test only.

A retained kubectl port-forward maps localhost:18095 to the **Traefik** Service
port 80. Tailscale Serve HTTP :18095 proxies to that listener, preserving the
real DNS host. It does not forward to the frontend Service. Route target remains
frontend:80, so the domain request traverses the Traefik HTTPRoute decision.
No Host header override, hosts-file entry, browser DNS remapping or HTTPS bypass.
This verifies HTTP; it does not verify HTTPS listener/certificate provisioning.

Private setup values, exact CRD manifest, process IDs, reports, screenshots and
final-success video: /home/thanhnt1/.local/state/httproute-domain-20261105/.
On restart, check the saved Gateway/Service UIDs and receipt-owned process before
restarting only the recorded Traefik Service forward. The app, namespace,
Gateway/controller/CRDs and existing Tailscale services are retained for review.
Do not delete CRDs as a convenience cleanup; they are cluster-scoped and may
have acquired consumers. Cleanup/teardown of this retained setup is separate.

Recorded forward restart (after verifying saved Service UID and that the old
owned forward is stopped; run in WSL and retain it for access):

```sh
kubectl --kubeconfig /home/thanhnt1/.orch/bootstrap/dev/cluster-kubeconfig \
  --context kind-k8s-4f -n httproute-domain-test port-forward \
  --address 127.0.0.1 svc/httproute-domain-traefik 18095:80
```

The saved background Tailscale Serve mapping is persistent. If missing, restore
only the test port after confirming no different owner uses it:

```sh
sudo tailscale serve --bg --http=18095 http://127.0.0.1:18095
```
