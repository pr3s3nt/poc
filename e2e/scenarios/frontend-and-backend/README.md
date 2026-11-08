# Frontend and backend through Console UI — issue #47

Trial status, 2026-11-08: the user stopped further verification after a successful
batch deployment and a reachable Guest page. Guest sign-in/catalog did not pass
because the upstream frontend image throws `apiRef{core.toast}` after Enter.
Do not rerun deploy or image experiments without a new request. See the
[verification record](../../../orchestrator_docs/verification/2026-11-08-frontend-backend.md)
for prior failed attempts and operational assistance. `verify.mjs` retains the
business assertions and must not be reported as passing on this image.

Source: https://docs.score.dev/docs/examples/frontend-and-backend/ . Deploy the
example's separate Backstage frontend and backend, backed by PostgreSQL. Read
the source as input to Console forms; do not import Score or run its generators.
Source repository inspected at `974b897593f75e448157784a784049be76c95638`;
container image digests are pinned in the runner.

Prerequisites: the issue #47 feature stack with verified Git, Vault, Fleet,
cluster/namespace/workload Definitions and template v0.3.0; an authorized
kind-k8s-4f kubeconfig; an existing Gateway accepting the owned app namespace;
wildcard DNS and a persistent Tailscale listener reaching that Gateway.
Read e2e/README.md, UC-06 and the endpoint contract before running.

Use shared Console authentication variables. `ORCH_TEST_RUN_DIR` must be outside
Git and contain private `config.json` with `publicUrl`, `rootDomain`, `endpointName`,
`gatewayName`, `gatewayNamespace`, `gatewaySectionName`, `repository`,
`templateRevisionId` and `clusterKubeconfigFile`. Select an available workload
Definition revision for the current renderer, not an unavailable historical
default. Git credentials remain in the existing Console settings.
`publicUrl` is the common origin without a trailing slash and must match
`http://<endpointName>.<rootDomain>`.

Run `node e2e/scenarios/frontend-and-backend/run.mjs prepare` to create a new
Application, register scoped PostgreSQL/endpoint Definitions through UI, and save
both workloads with their bindings. Then run with `preview`, inspect its complete
two-workload batch, and run with `deploy`. One click submits the full batch;
individual workload deployments are not acceptance evidence. An observed request
is recorded before waiting; an uncertain or failed attempt cannot be replayed
by rerunning this runner. Diagnose first and preserve evidence. A confirmed
failed, released Operation may be retried explicitly with `ORCH_RETRY_CONFIRMED=1`;
the prior attempt is retained and the retry still submits the complete batch.
Run `verify.mjs` after a successful deployment for cluster and browser assertions.

Assertions: both drafts saved/reopened before Deploy; no individual workload
predeployment; exactly one batch Deploy request per recorded attempt; exact Operation succeeds;
both Deployments and PostgreSQL become Ready; VSO password uses a Secret ref;
HTTPRoutes share the hostname with `/` and `/api`; browser loads the real
Backstage page and signs in through the backend guest provider.

Adaptations: platform-native `endpoint` resources replace Score's shared dns and
route provisioners. Their URL includes the route path, so the common public
origin is configured once as the Environment variable `PUBLIC_ORIGIN`, selected
through the source picker for Backstage baseUrl/CORS instead of passing
the backend endpoint's `/api` URL as its baseUrl. The frontend image replaces
its compiled config with APP_CONFIG variables, so app.title and organization.name
are also supplied from the upstream app configuration. A backend startup probe allows
database/plugin initialization before liveness checks. PostgreSQL credentials are
selected from resource outputs, never literal values. Guest authentication is
enabled as in the example for this test instance. No product code is changed.

Side effects: one owned Application/namespace and private delivery repo, scoped
Definitions, PostgreSQL/PVC, VSO objects, two Deployments/Services/HTTPRoutes.
Technical Fleet RBAC and Gateway forwarding are setup, not product deployment.
Keep the app and checked Tailscale endpoint for user acceptance. Cleanup must
use Console deletion and ownership/UID verification; do not delete retained
storage or shared Gateway/controller resources implicitly. Evidence, sessions,
logs and configuration stay outside Git.
