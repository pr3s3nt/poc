// Creates test-only secrets and realm import outside Git; never prints values.
import { mkdirSync, writeFileSync, existsSync } from 'node:fs';
import { resolve } from 'node:path';
import { randomBytes } from 'node:crypto';
const directory = resolve(process.env.KEYCLOAK_PRIVATE_DIR ?? '');
if (!process.env.KEYCLOAK_PRIVATE_DIR || directory.includes(`${process.cwd()}/`)) throw new Error('Set KEYCLOAK_PRIVATE_DIR outside the checkout');
const domain = process.env.FEATURE_DOMAIN;
if (!domain || !/^[a-zA-Z0-9.-]+$/.test(domain)) throw new Error('Set FEATURE_DOMAIN');
mkdirSync(directory, { recursive: true, mode: 0o700 });
if (existsSync(`${directory}/oidc.json`)) throw new Error('Existing configuration retained; reuse it or choose a fresh directory');
const secret = () => randomBytes(24).toString('base64url');
const put = (name, value) => writeFileSync(`${directory}/${name}`, value, { mode: 0o600, flag: 'wx' });
const clientId = 'orchestrator';
const clientSecret = secret();
const controllerSecret = secret();
const consoleURL = `http://keycloak.${domain}:8088`;
const providerURL = `http://sso-keycloak.${domain}:8088`;
const localTestAdminUsername = process.env.KEYCLOAK_LOCAL_TEST_ADMIN_USERNAME ?? '';
if (localTestAdminUsername && !/^[a-z0-9][a-z0-9._-]{1,62}$/.test(localTestAdminUsername)) throw new Error('Invalid local test Admin username');
const roles = ['developer', 'platform-engineer', 'admin'];
const users = [...roles, 'no-role'].map((username) => {
  const password = secret(); put(`${username}-password`, password);
  return { username, email: `${username}@example.invalid`, enabled: true, emailVerified: true, firstName: 'Test', lastName: username,
    credentials: [{ type: 'password', value: password, temporary: false }],
    clientRoles: roles.includes(username) ? { [clientId]: [username] } : {} };
});
put('keycloak-admin-password', secret());
put('controller-secret', controllerSecret);
put('oidc.json', JSON.stringify({ issuer: `${providerURL}/realms/platform-test`, clientId, clientSecret, publicUrl: consoleURL, organizationKey: 'acme', allowHttp: true, ...(localTestAdminUsername ? { localTestAdminUsername } : {}) }, null, 2));
put('realm.json', JSON.stringify({ realm: 'platform-test', enabled: true, sslRequired: 'none', registrationAllowed: false, resetPasswordAllowed: false,
  accessTokenLifespan: 120, ssoSessionIdleTimeout: 1800, ssoSessionMaxLifespan: 28800,
  roles: { client: { [clientId]: roles.map((name) => ({ name })) } },
  clients: [
    { clientId, secret: clientSecret, enabled: true, protocol: 'openid-connect', publicClient: false, standardFlowEnabled: true, directAccessGrantsEnabled: false,
      redirectUris: [`${consoleURL}/api/v1/auth/oidc/callback`], webOrigins: [], fullScopeAllowed: true,
      attributes: { 'pkce.code.challenge.method': 'S256' },
      protocolMappers: [{ name: 'platform-client-roles', protocol: 'openid-connect', protocolMapper: 'oidc-usermodel-client-role-mapper',
        config: { 'usermodel.clientRoleMapping.clientId': clientId, 'claim.name': `resource_access.${clientId}.roles`, 'jsonType.label': 'String', multivalued: 'true', 'id.token.claim': 'true', 'access.token.claim': 'true' } }] },
    { clientId: 'test-controller', secret: controllerSecret, enabled: true, protocol: 'openid-connect', publicClient: false, serviceAccountsEnabled: true,
      standardFlowEnabled: false, directAccessGrantsEnabled: false }
  ], users: [...users, { username: 'service-account-test-controller', enabled: true, serviceAccountClientId: 'test-controller', clientRoles: { 'realm-management': ['realm-admin'] } }] }, null, 2));
console.log('Private Keycloak test inputs created. No company realm is accessed.');
