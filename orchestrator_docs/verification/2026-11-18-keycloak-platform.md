---
id: VERIFY-KEYCLOAK-PLATFORM-20261118
artifact: verification-record
status: current
last_reviewed: 2026-11-18
---

# Keycloak deployed through the main Console — issue #88

The user approved deploying Keycloak with the existing Platform Engineer account
and enabling the already implemented OIDC integration on the retained kind
installation. This is an operational follow-up to
[the accepted local-mode rollout](2026-11-17-keycloak-main.md), not another product
authentication implementation or an account migration.

## Main preflight and Console deployment

Direct Kubernetes reads confirm backend/frontend use `main-57ecc971`, each Ready
1/1. Current main `725541ef` differs only in documentation. Browser login on the
exact Tailscale Console URL succeeds as the existing `platform-engineer`, role
PLATFORM_ENGINEER, Organization `acme`. Auth configuration initially reports local.

Through that browser session, created a separate Application, registered an
app-scoped endpoint Definition, selected the existing platform Vault Store, saved
a generated admin password as an Environment Secret and imported the existing
Keycloak Score/profile. The separate Score-preview Keycloak is preserved.

Initial Preview returns PLANNING_REJECTED/resource-no-match: the main PostgreSQL
Definition requires class `in-cluster`. Adding that class to the application
input resolves matching. This failure happens before deployment; it is not a
failed Deploy subsequently relabelled successful. A fresh Preview includes only
the Keycloak workload and its dependencies; one Environment Deploy returns HTTP
200/SUCCEEDED. No manual workload apply substitutes for Console deployment.

The existing bounded helper grants Fleet access only inside the product-created
namespace after checking Application identity, managed labels and namespace UID.
Role/RoleBinding UIDs are retained in the private receipt. PostgreSQL PVC is
Bound, both Pods Ready without restart at verification, VSO Synced/Healthy/Ready,
and HTTPRoute Accepted/ResolvedRefs at its current generation.

## Endpoint and OIDC configuration

The shared Gateway already accepts the new route, but the external feature proxy
initially returns 404 for the new hostname. A task-owned relay/container and two
task-owned tmux sessions connect that exact hostname to the existing Gateway.
Shared Gateway, proxy configuration, other feature stacks and user sessions are
preserved. Host and backend both reach the published discovery URL; Chromium
opens the real Keycloak login form and the dedicated realm's discovery document
reports the exact issuer.

Created a dedicated realm and confidential `orchestrator` client with exact
Console callback, Authorization Code, PKCE S256 and client roles in ID tokens.
Three employee accounts use `sso-`-prefixed usernames to avoid the existing local
usernames; client roles map to the existing DEVELOPER/PLATFORM_ENGINEER/ADMIN.
Employees receive no Keycloak realm-management grants. Direct password grants
and service accounts are disabled for the platform client.

The operator CLI creates the first personal `local-admin` in the existing
Organization using an owner-only generated password file. The original two local
accounts retain their identities, roles and status; no username linking, password
reset or direct database account patch ran. OIDC configuration selects only that
personal Admin for the SSO-07 local path.

All Environment operations are terminal before backend rollout. A custom-format
PostgreSQL backup is created privately; `pg_restore -l` reads its archive. This
does not claim an isolated restore rehearsal. The completed archive-reader exec
stream remained open and was terminated after its success message; the backup
file is retained. No database migration/reset/owner setup is needed for enabling
existing OIDC. The pre-rollout Deployment and database identity snapshots are
retained privately.

The [optional deployment patch](../../deploy/kind-k8s-4f/22-oidc.patch.yaml)
passes server-side dry-run, then rolls out successfully. Its init container copies
the Secret into a memory volume: runtime file owner/group 65532, mode 0600,
read-only mount. Backend stays at one replica and receives no owner DB credential.
Auth configuration now reports `mode: oidc`, `localLoginAllowed: true`.

## Validation and retention

Real Chromium checks on the exact Tailscale Console URL pass all three SSO roles,
Applications/session APIs, existing navigation and backend role gates, opaque
HttpOnly/SameSite=Lax cookies, no provider tokens in session DTOs and logout.
Personal local Admin signs in on the same Console without any Keycloak request;
employee-management APIs remain blocked. The UI uses the existing shell and
sign-in disclosure from UC-00/shared UI design; no UI source or content was added.
Backend restart rejects the old SSO cookie; a fresh browser login preserves the
SSO User ID. The eligible personal local Admin session survives restart. Final
browser opens the deployed Application page and its same-origin APIs successfully.
The operational browser assertion initially read an incorrect identity field;
it was corrected to the existing `UserID` DTO field, then the login/restart check
was repeated successfully without changing product code or assertions.

Before/after database comparison preserves schema/installation baseline,
Organization, all three local account records (including the newly bootstrapped
Admin), 15 persisted Application rows including deleted history, 34 Definitions,
47 Definition versions, nine Resource Types and 30 Environment rows. These are
database counts, not the number of visible active Application cards. The new
Application/endpoint are included in the pre-SSO snapshot; their creation is not
represented as an unchanged pre-task inventory.

Documentation/whitespace checks and Kubernetes server dry-run pass. Product
Go/frontend full suites were not rerun: runtime product images/source/schema are
unchanged, and this task changes operational configuration/documentation only.
The accepted main full-run baseline remains red as recorded in the preceding
verification; this operational success does not make that test gate green.
PlantUML is unavailable locally; no diagrams changed. Checks from this host do
not prove access from every tailnet device.

Keycloak uses the existing pinned dev-mode Score with HTTP on the test tailnet
and PostgreSQL persistence; this is not a production/HA rollout. Console remains
single-instance, with the documented provider-session lifecycle. Application,
PVC, realm/users, OIDC Secret, Vault data, forwards and task relay remain running
for user acceptance. Exact URLs, passwords, UID receipts, screenshots, backup,
restart/rollback and cleanup instructions are in private handoff outside Git;
none are posted to the public issue. No unrelated resources were deleted.

Repository changes are the optional OIDC patch and operational documentation.
The task branch is pushed; no merge or issue closure before user acceptance.
