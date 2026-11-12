# Keycloak SSO acceptance

Checks [UC-00 SSO-01..06](../../../orchestrator_docs/usecase/UC-00/specification.md#keycloak-sso--issue-77)
against the real dedicated Keycloak and PostgreSQL platform. Setup:
[private test inputs](../../setup/keycloak/README.md),
[operations](../../../orchestrator_docs/operations/keycloak-sso.md).

Requires Node 22+, `npm ci --prefix e2e`, `npm ci --prefix scripts/bootstrap`,
and Playwright Chromium installed through `scripts/bootstrap`. Uses the shared
Console helper; no fake API/identity provider. The runner refuses any issuer
whose path does not end in `/realms/platform-test`.

Run from repository root:

```bash
ORCH_PLATFORM_URL=http://keycloak.<domain>:8088 \
ORCH_OIDC_ISSUER=http://sso-keycloak.<domain>:8088/realms/platform-test \
ORCH_PLATFORM_USERNAME=developer \
ORCH_PLATFORM_PASSWORD_FILE=/private/keycloak/developer-password \
KEYCLOAK_PRIVATE_DIR=/private/keycloak ORCH_HEADLESS=1 \
ORCH_RESULT_DIR=/private/evidence/keycloak \
  node e2e/scenarios/keycloak-sso/run.mjs
```

The domain must be the actual Tailscale feature path validated per handoff rules.
Passwords are read only from private files, never from CLI arguments. The shared
helper confirms `ORCH_OIDC_ISSUER` before filling the Keycloak form.

Assertions: all three roles sign in through browser; correct platform navigation;
backend rejects Developer platform-authoring calls; local login/user management
are blocked in SSO mode; opaque HttpOnly cookie; no provider tokens in session DTO;
no-role account rejected; an existing session observes role promotion/demotion
and account disabling after the 30-second refresh interval; Console logout ends
the platform session. Failed body validation after authorization is used to check
PE/Admin admission without creating catalog resources. Assertions must all pass.

Side effects: first SSO login persists four-claim identity information for the
three role accounts (issuer, subject, username, mapped role); session hash rows
remain for ordinary lifecycle retention. The no-role account gets no platform
account. The runner temporarily grants PE to test Developer, removes it, disables
then restores the user. `finally` restores enablement/removes only the added PE
role, signs out test contexts, and verifies the final account/role state via the
Keycloak API. It does not delete the dedicated preview stack or platform data.
If cleanup fails, preserve evidence and restore these test-account fields before
retrying. Do not run against a shared/company realm.

Screenshots and safe `result.json` remain outside Git; no traces/storage-state or
provider responses are saved. A PASS proves this installation/path at run time,
not company realm compatibility, HA, or every tailnet client's ACL access.

For SSO-07, also set `ORCH_LOCAL_TEST_ADMIN_USERNAME` and
`ORCH_LOCAL_TEST_ADMIN_PASSWORD_FILE` to the operator-bootstrapped personal local
Admin. The same Console must advertise localLoginAllowed. The scenario verifies
local login without any provider requests, UserManagement=false, blocked employee
APIs, and the own-password form. It submits a deliberately incorrect current
password to confirm local verification without changing the real password. The
local context is signed out in cleanup; its account/password are preserved.
