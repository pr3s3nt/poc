---
id: OPS-KEYCLOAK-SSO
artifact: operations-runbook
status: current
last_reviewed: 2026-11-12
---

# Keycloak SSO

[UC-00 SSO-01..06](../usecase/UC-00/specification.md#keycloak-sso--issue-77)
owns product behavior. This procedure is for issue #77's dedicated test setup;
no company Keycloak is changed. Existing backend authorization remains in place.

## Backend configuration

Set `ORCHESTRATOR_OIDC_CONFIG_FILE` to an owner-only JSON file readable by the
backend runtime user. It contains `issuer`, `clientId`, `clientSecret`, `publicUrl`
(the Console origin), `organizationKey` (an existing Organization), and optional
`developerRole`, `engineerRole`, `adminRole` names. Defaults are developer,
platform-engineer, admin. `allowHttp` defaults false; enable only in the isolated
HTTP test environment. Use HTTPS/trusted CA when connecting a company realm.
Configuration/discovery/Organization errors block startup; no fallback local login.

The OIDC client is confidential; enable Authorization Code and require PKCE S256.
Allow only `<publicUrl>/api/v1/auth/oidc/callback`. Add a client-role protocol
mapper to ID tokens with claim `resource_access.<clientId>.roles`, multivalued
string. Configure only that client's three roles or map existing company roles
into them. Realm-admin and roles from other clients never grant platform access.
Do not grant platform users Keycloak realm-management privileges.

When OIDC is configured, local password login, local user management and password
APIs are disabled. Remove SSO configuration to return to deliberate local mode;
external accounts remain unable to use local passwords. No account migration or
automatic email/username linking is implemented. Duplicate usernames are refused.

## Dedicated feature preview

Follow [Tailscale feature handoff](tailscale-handoff.md#bản-thử-của-tính-năng)
to determine `FEATURE_DOMAIN` and establish the shared proxy/forward. Then:

```bash
export FEATURE=keycloak
export FEATURE_DOMAIN=<verified-domain>
export KEYCLOAK_PRIVATE_DIR=<absolute-private-directory-outside-git>
export BACKEND_IMAGE=poc-feature-backend:keycloak
export FRONTEND_IMAGE=poc-feature-frontend:keycloak
node e2e/setup/keycloak/prepare.mjs

docker compose -p poc-keycloak -f docker-compose.yml \
  -f deploy/local/compose.feature-url.yml \
  -f deploy/local/compose.keycloak.yml up -d --build
```

The overlay explicitly uses the production seed profile to avoid local fixture
accounts colliding with SSO usernames; this does not claim production readiness.
It pins Keycloak 26.4.7 and generates the dedicated `platform-test` realm.
Backend issuer discovery uses the same hostname/port as the browser through a
Docker network alias. Token validation does not disable issuer/TLS checks.
The input-copy job gives backend config UID 65532/mode 0600 and realm import
UID 1000/mode 0600. No owner database credential is mounted into backend.

Console route is `http://keycloak.<domain>:8088/ui/`; Keycloak route is
`http://sso-keycloak.<domain>:8088/`. Verify actual URLs in browser before handoff.
Usernames: developer, platform-engineer, admin. Their random passwords are in
`KEYCLOAK_PRIVATE_DIR/<username>-password`. Read locally; do not post credentials
in issues, URLs or logs. Company configuration can replace connection/mapping
values later, after that realm's token claims/TLS/client settings are verified.

## Session and data lifecycle

Accounts and domain data live in PostgreSQL. Keycloak dev H2 data lives in its
own volume. Provider tokens/refresh credentials and pending OIDC transactions
are held only in backend memory; restarting backend forces SSO login again while
preserving account IDs/data. This slice supports a single backend instance;
shared/durable encrypted provider-session storage and multi-instance routing are
not implemented. Do not deploy multiple replicas with this configuration.

Existing sessions refresh at most every 30 seconds on request, earlier if their
validated grant expires. Role changes and disabled users are observed at that
refresh; provider failure terminates the session rather than preserving stale
privileges. Idle sessions perform no network traffic and cannot exceed 8 hours.
Logout revokes the local session and best-effort provider refresh token; it does
not initiate global SSO logout. The next sign-in may reuse a valid Keycloak SSO
session. Callback query strings must be excluded from reverse-proxy access logs;
the Console nginx template handles its exact callback route.

Run the [browser scenario](../../e2e/scenarios/keycloak-sso/README.md) before
handoff. Verify backend restart requires fresh login and preserves User ID. Keep
stack, feature proxy and Tailscale forward running until user acceptance.

Restart with the same exports and Compose files (`up -d --no-build`); do not run
input preparation again over retained credentials. Rebuilding updates input-copy
files but Keycloak's import skips an existing realm; change retained realm fields
through its admin API/Console. A changed issuer creates a distinct identity.

After acceptance/merge, `down -v` with all three Compose files removes only
`poc-keycloak` volumes/data; remove the private input directory and task images.
Keep shared proxy and other feature stacks. Kind full-check namespaces use their
own run label and must be UID/ownership verified and removed independently.
