# Public endpoint through a real DNS domain and Traefik Gateway API

Scope: [endpoint contract EP-01–06](../../../orchestrator_docs/architecture/contracts/endpoint.md),
[platform setup](../../../projects/platform-resources/bootstrap/HTTPROUTE.md),
[CD](../../../orchestrator_docs/operations/delivery-reconciliation.md).

Through actual Console controls a Platform Engineer registers the `endpoint`
Resource Type and app-scoped cluster, namespace, Git template workload and
built-in `endpoint` Definitions (the bootstrap endpoint form filler). A developer
imports one nginx frontend whose `resources.public` is an endpoint, then runs
Preview/Deploy three times: publish the real domain, rename it, restore it.

Prerequisites: explicit dev cluster/Git/CD mutation authorization; a schema-9
Console with READY account, Secret Store and Fleet default; an existing Gateway
whose listener admits application namespaces; HTTPRoute v1 CRD; a real DNS
name `<name>.<root domain>` that reaches that Gateway (the 2026-11-05 dev setup
is in [httproute-domain](../httproute-domain/README.md)); no other route may use
that hostname. Test-only access adds a bounded Role/RoleBinding (including
httproutes) in the product-created app namespace. Install E2E/bootstrap
dependencies per the [shared guide](../../README.md).

```sh
node e2e/scenarios/endpoint-domain/run.mjs --config /private/endpoint-domain/config.json
```

Private 0600 config fields: `name`, `console`/`developer` (`url`, `username`,
`passwordFile`), `stateDir`, `reportDir`, `kubeconfigFile`, `context`,
`accountName`, `storeName`, `gateway` (`name`, `namespace`, `listener`), `url`
(real domain, path `/`, may carry the exposed port), `renamedName` (another DNS
label under the same root), `image`, `templateSource` (Git pin), and `database`
(`container`, `user`, `name`) for read-only ledger queries of that stack.

Assertions per phase: Preview changes only frontend and shows the exact URL in
the Console "Địa chỉ public" panel; Deploy SUCCEEDED; the ledger holds exactly
the current hostname; one workload-labelled `endpoint-<hash>` HTTPRoute with the
same name across phases is Accepted/ResolvedRefs at its current generation with
the hostname, listener and frontend:80 backend; the actual Fleet bundle carries
it. Browser checks use a fresh context per attempt: the real domain returns the
nginx page while held and 404 from Traefik after the rename.

Side effects: creates and retains the receipt-owned app, Definitions, template
registration, namespace RBAC and route. Reruns verify input identity, skip
completed phases and resume a phase whose Deploy already SUCCEEDED at
verification. Unknown outcomes stop for reconciliation. Cleanup: UID-delete the
setup-owned `bootstrap-fleet-access` Role/RoleBinding first, then product
Application deletion; verify namespace, route and reservation absence. Shared
Gateway/Traefik/CRDs/Tailscale settings are never cleanup targets.
