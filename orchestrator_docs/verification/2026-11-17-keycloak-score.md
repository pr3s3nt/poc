---
id: VER-KEYCLOAK-SCORE-20261117
artifact: verification-record
status: current
last_reviewed: 2026-11-17
---

# Keycloak Score deployment and managed endpoint — issue #83

Scope: diagnose the user's failed Keycloak trial on the retained Score feature
stack and verify the repaired deployment. The user selected a trial Tailscale
domain through the existing platform Gateway. Sources:
[example](../../backend/examples/keycloak/README.md),
[Score contract](../architecture/contracts/score-workload.md),
[endpoint contract](../architecture/contracts/endpoint.md),
[original Score verification](2026-11-16-score-authoring.md).

## Failed attempt and cause

The user's first attempt ended FAILED during the deployment preflight, before
namespace creation or workload apply. The operation and deployment history are
retained. The backend logged UNCLASSIFIED; the public failure text did not expose
the precise prerequisite. A direct read using the ordinary application Vault
token returned 404 for the configured Kubernetes auth config path. The feature
Vault had not been configured for Kubernetes workload authentication.

The selected Store also advertised its Compose-local hostname as its workload
address. That hostname was not an appropriate Kubernetes/VSO address. The first
failure did not reach this later network dependency, so it is recorded as an
additional setup issue rather than another observed failed deployment.

## Repair without changing the Score or password

Only the task's feature Vault received the Kubernetes auth configuration, using
the existing authorized TokenReview identity and cluster CA. A task-owned Docker
network alias supplies a certificate-compatible API hostname through the existing
cluster relay. No shared cluster RBAC, Gateway, controller or other task's objects
were changed. The application-token config read now succeeds.

An independent task-owned ingress relay exposes this same Vault at an address
reachable from the cluster. A new ordinary Secret Store registration retains the
same backend, KV mount and auth mount with that workload address. The product's
Secret Store transition copied the user's configured Secret and retained its
name/kind/configured metadata. No password was printed, replaced or requested
again; no database rows or immutable Store endpoints were patched manually.
A PostgreSQL Pod subsequently reached the workload Vault health endpoint.

## Fresh Preview and retry

After verifying every previous operation was terminal, a fresh real Console
Preview contained only Keycloak and its dependencies. One explicit Environment
Deploy through the Console returned HTTP 200 / SUCCEEDED. The earlier failure
remains FAILED; it is not relabelled as a successful first attempt.

The product created the namespace; the existing bounded setup helper verified
namespace identity/UID and supplied app-scoped Fleet Role/RoleBinding, including
HTTPRoute rights. Their UIDs are recorded privately. PostgreSQL and Keycloak
are both Ready with no restarts observed at handoff. VaultAuth, VaultConnection
and VaultStaticSecret report healthy/ready and the configuration Secret is synced.
The standard Keycloak image completed its first configuration build and created
the bootstrap admin account.

The platform-managed HTTPRoute has Accepted and ResolvedRefs at its current
generation. The exact Tailscale admin URL was opened in a fresh browser context;
the real username/password login form is visible. The same-origin master realm
OIDC discovery returns HTTP 200, and its issuer and authorization endpoint use
the expected public origin. No admin password was read or submitted; this proves
login-page access and OIDC URL configuration, not an authenticated admin session.

## Checks, retention and limits

The focused validation is the actual configuration preflight, Store-copy API,
Console Preview/Deploy, native readiness/VSO/HTTPRoute reads and real browser/OIDC
checks described above, plus docs checker and whitespace validation. No runtime
product source changed; Go/frontend suites were not rerun for this operational
repair. The original baseline-red backend comparison remains as documented in
the preceding Score record. No diagrams changed; host PlantUML is unavailable.

The user's Application, database/PVC, route and configured Secrets are retained
for their trial. Feature stack, Gateway ingress relay and the Vault address/auth
support remain running; private handoff records exact URLs, restart commands,
object UIDs and logs. Do not clean up the user's trial as test disposable data.
When removal is requested, use product deletion and the existing receipt-owned
setup RBAC cleanup after CD withdrawal; preserve unrelated resources. No merge
or retained-main deployment was performed.
