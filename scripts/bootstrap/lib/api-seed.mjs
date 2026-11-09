// Local API importer. No browser, kubectl, shell commands or secret persistence.
import { readFileSync, statSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { homedir } from 'node:os';
import { isIP } from 'node:net';
import { parseDocument } from 'yaml';

export class SeedError extends Error {}
const fail = message => { throw new SeedError(message); };
export function matches(actual, expected) {
 if (expected === null || typeof expected !== 'object') return actual === expected;
 if (Array.isArray(expected)) return Array.isArray(actual) && actual.length === expected.length && expected.every((v, i) => matches(actual[i], v));
 return actual != null && Object.entries(expected).every(([k, v]) => matches(actual[k] ?? (v === false ? false : Array.isArray(v) ? [] : v && typeof v === 'object' ? {} : undefined), v));
}
function canonical(value) {
 if (Array.isArray(value)) return value.map(canonical);
 if (value && typeof value === 'object') return Object.fromEntries(Object.keys(value).sort().map(k => [k, canonical(value[k])]));
 return value;
}
export function definitionMatches(actual, expected) {
 const fields = ['key', 'name', 'resourceType', 'driverType', 'driverAccount'];
 if (!fields.every(k => (actual[k] ?? '') === (expected[k] ?? ''))) return false;
 return ['driverInputs', 'provision', 'criteria'].every(k => JSON.stringify(canonical(actual[k] ?? (k === 'criteria' ? [] : {}))) === JSON.stringify(canonical(expected[k] ?? (k === 'criteria' ? [] : {}))));
}
export function expand(value, refs) {
 if (typeof value === 'string') return value.replace(/\$\{(accounts|templateSources)\.([^}]+)\}/g, (raw, group, name) => refs[group]?.[name] ?? fail(`Missing ${group} reference`));
 if (Array.isArray(value)) return value.map(v => expand(v, refs));
 if (value && typeof value === 'object') return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, expand(v, refs)]));
 return value;
}
export function privateFile(file) {
 let stat, value;
 try { stat = statSync(file); } catch { fail('Credential file unavailable'); }
 if (!stat.isFile() || (stat.mode & 0o077)) fail('Credential file must be regular and owner-only (0600/0400)');
 if (stat.size > 2 ** 20) fail('Credential file exceeds limit');
 try { value = readFileSync(file, 'utf8').trim(); } catch { fail('Cannot read credential file'); }
 if (!value) fail('Credential file is empty');
 return value;
}
export function target(raw, tailnet) {
 let u; try { u = new URL(raw); } catch { fail('Invalid Console URL'); }
 if (u.username || u.password || u.search || u.hash || !['', '/', '/ui', '/ui/'].includes(u.pathname)) fail('Console URL must be an origin');
 const ip = u.hostname.split('.').map(Number);
 const tailnetIP = isIP(u.hostname) === 4 && ip[0] === 100 && ip[1] >= 64 && ip[1] <= 127;
 if (u.protocol !== 'https:' && !(u.protocol === 'http:' && (['127.0.0.1', 'localhost', '[::1]'].includes(u.hostname) || tailnet === true && (u.hostname.endsWith('.ts.net') || tailnetIP)))) fail('Use HTTPS, loopback or explicitly verified Tailscale');
 return u.origin;
}
export function readInput(file) {
 let c; try {
  const stat = statSync(file); if (!stat.isFile() || stat.size > 16 * 2 ** 20 || (stat.mode & 0o077)) fail('Config must be owner-only and bounded');
  const doc = parseDocument(readFileSync(file, 'utf8'), { uniqueKeys: true });
  if (doc.errors.length) fail('Invalid YAML');
  c = doc.toJS({ maxAliasCount: 0 });
 } catch (e) { if (e instanceof SeedError) throw e; fail('Cannot load seed YAML'); }
 return c;
}
export function organizationIdentity(value) {
 if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).some(k => !['key','name'].includes(k)) || typeof value.key !== 'string' || !/^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(value.key) || typeof value.name !== 'string' || !value.name || value.name !== value.name.trim() || [...value.name].length > 200 || /[\p{Cc}\p{Cs}]/u.test(value.name)) fail('Invalid Organization identity');
 return { key: value.key, name: value.name };
}
export function prepareOrganization(file, output) {
 const c = readInput(file);
 if (c?.schemaVersion !== 1) fail('Invalid seed schema');
 const org = organizationIdentity(c.organization);
 if (c.console?.expectedOrganization && c.console.expectedOrganization !== org.key) fail('Organization key conflicts with expectedOrganization');
 try { writeFileSync(output, JSON.stringify(org) + '\n', { mode: 0o600, flag: 'wx' }); } catch { fail('Cannot create Organization file; output must be new'); }
}
export function loadConfig(file) {
 const c = readInput(file);
 const allowed = ['organization', 'schemaVersion', 'console', 'git', 'accounts', 'gitAccounts', 'vaults', 'resourceTypes', 'templateSources', 'definitions', 'delivery'];
 if (!c || c.schemaVersion !== 1 || Object.keys(c).some(k => !allowed.includes(k))) fail('Invalid seed schema');
 if (c.organization !== undefined) {
  c.organization = organizationIdentity(c.organization);
  if (c.console?.expectedOrganization && c.console.expectedOrganization !== c.organization.key) fail('Organization key conflicts with expectedOrganization');
 }
 if (!c.console?.username || !c.console.passwordFile || !(c.organization?.key || c.console.expectedOrganization)) fail('Console identity and passwordFile are required');
 c.console.origin = target(c.console.url, c.console.tailnet);
 const local = p => typeof p === 'string' ? (p.startsWith('~/') ? resolve(homedir(), p.slice(2)) : resolve(dirname(file), p)) : fail('Invalid file reference');
 c.console.password = privateFile(local(c.console.passwordFile));
 const materialize = (value, credentialObject = false) => {
  if (Array.isArray(value)) return value.map(v => materialize(v));
  if (!value || typeof value !== 'object') return value;
  const out = {};
  for (const [k, v] of Object.entries(value)) {
   if (credentialObject && ['token', 'password', 'kubeconfig', 'accessKeyId', 'secretAccessKey', 'writerPassword'].includes(k)) fail('Credentials must use file references');
   if (credentialObject && ['tokenFile', 'kubeconfigFile', 'accessKeyIdFile', 'secretAccessKeyFile', 'writerPasswordFile'].includes(k)) {
    const text = privateFile(local(v));
    if (k !== 'kubeconfigFile' && /[\x00-\x1f\x7f]/.test(text)) fail('Credential file contains invalid control characters');
    out[k.slice(0, -4)] = text;
   }
   else out[k] = materialize(v);
  }
  return out;
 };
 const document = (item, credentialObject = false) => {
  if (!item.file) return materialize(item, credentialObject);
  if (Object.keys(item).some(k => !['file', 'inspect'].includes(k))) fail('Document reference accepts file and inspect only');
  let body; try { body = JSON.parse(readFileSync(local(item.file), 'utf8')); } catch { fail('Invalid local product document'); }
  return { ...materialize(body, credentialObject), ...(item.inspect ? { inspect: materialize(item.inspect) } : {}) };
 };
 for (const key of ['accounts', 'gitAccounts', 'vaults', 'resourceTypes', 'templateSources', 'definitions', 'delivery']) {
  const list = c[key] ?? []; if (!Array.isArray(list)) fail(`Invalid ${key} list`);
  c[key] = list.map(item => document(item, ['accounts', 'gitAccounts', 'vaults'].includes(key)));
  const ids = c[key].map(x => x.name ?? x.key);
  if (ids.some(x => !x) || new Set(ids).size !== ids.length) fail(`Duplicate or missing ${key} identity`);
 }
 if (c.git) c.git = materialize(c.git, true);
 for (const t of c.templateSources) if (!/^[a-f0-9]{40}$/.test(t.ref) || !/^[a-f0-9]{64}$/.test(t.contentSha256)) fail('Git template requires full commit and SHA256');
 for (const a of c.accounts) if (!['kubernetes', 'aws', 'ec2-role'].includes(a.kind)) fail('Unsupported Driver Account kind');
 if (c.delivery.filter(d => d.setDefault).length > 1) fail('Only one default CD binding is allowed');
 return c;
}

export async function seed(c, { fetcher = fetch, report = console.log } = {}) {
 const origin = c.console.origin;
 let cookie;
 const emit = (kind, status) => report(`${kind} ${status}`); // Never log input identities or bodies.
 const api = async (method, path, body) => {
  let r;
  try { r = await fetcher(origin + '/api/v1' + path, { method, redirect: 'error', headers: { 'Content-Type': 'application/json', ...(cookie ? { Cookie: cookie } : {}) }, ...(body === undefined ? {} : { body: JSON.stringify(body) }), signal: AbortSignal.timeout(180000) }); }
  catch { fail(`${method} ${path}: transport failed; read server state before retry`); }
  if (!r.ok) {
   let code = ''; try { const b = await r.json(); if (/^[A-Z][A-Z0-9_]{0,80}$/.test(b.code)) code = b.code; } catch { /* Never echo raw provider text. */ }
   fail(`${method} ${path}: HTTP ${r.status} ${code}`);
  }
  let result = {};
  if (r.status !== 204) { try { result = await r.json(); } catch { fail(`${method} ${path}: invalid response`); } }
  return { result, response: r };
 };
 const get = async path => (await api('GET', path)).result;
 const post = async (path, body) => (await api('POST', path, body)).result;
 const ensure = async (kind, list, find, expected, create, compare = matches) => {
  const found = list.filter(find); if (found.length > 1) fail(`${kind}: ambiguous identity`);
  if (found.length) { if (!compare(found[0], expected)) fail(`${kind}: CONFLICT`); emit(kind, 'REUSED'); return found[0]; }
  const result = await create(); if (!compare(result, expected)) fail(`${kind}: read-back mismatch`);
  emit(kind, 'CREATED'); return result;
 };
 const refs = { accounts: {}, templateSources: {} };
 try {
  const { result: login, response } = await api('POST', '/auth/sign-in', { username: c.console.username, password: c.console.password });
  cookie = response.headers.getSetCookie()[0]?.split(';')[0];
  if (!cookie) fail('Sign-in did not provide session');
  if (login.user?.OrganizationKey !== (c.organization?.key ?? c.console.expectedOrganization) || !['ADMIN', 'PLATFORM_ENGINEER'].includes(login.user?.Role)) fail('Unexpected Organization or insufficient role');
  if (c.organization) {
   const actual = (await get('/organization')).organization;
   if (!actual || actual.key !== c.organization.key || actual.name !== c.organization.name) fail('Organization: CONFLICT');
   emit('Organization', 'REUSED');
  }
  if (c.git) {
   const old = (await get('/platform/settings/git')).settings;
   const expected = { url: c.git.url, username: c.git.username, authenticated: true, caPem: c.git.caPem ?? '' };
   if (old.version > 0) { if (!matches(old, expected) || !old.tokenConfigured) fail('Git: CONFLICT'); emit('Git', 'REUSED'); }
   else { await api('PUT', '/platform/settings/git', { ...expected, token: c.git.token, expectedVersion: 0 }); const read = (await get('/platform/settings/git')).settings; if (!matches(read, expected) || !read.tokenConfigured) fail('Git read-back mismatch'); emit('Git', 'CREATED'); }
  }
  for (const input of c.accounts) {
   const { kind, ...body } = input;
   const list = (await get('/driver-accounts')).accounts;
   const type = kind === 'kubernetes' ? 'INTERNAL' : 'AWS';
   const expected = { name: body.name, type, status: 'READY' };
   if (kind === 'kubernetes') expected.config = { kubeContext: body.context, endpoint: body.endpoint };
   else expected.config = { verificationRegions: body.verificationRegions, ...(kind === 'ec2-role' ? { roleArn: body.roleArn } : {}) };
   const { endpoint, ...payload } = body;
   const account = await ensure('DriverAccount', list, x => x.name === body.name, expected, async () => {
    const created = await post(kind === 'kubernetes' ? '/driver-accounts/internal' : kind === 'aws' ? '/driver-accounts/aws' : '/driver-accounts/aws/ec2-role', payload);
    return get('/driver-accounts/' + encodeURIComponent(created.key));
   });
   refs.accounts[body.name] = account.key;
  }
  for (const body of c.gitAccounts) {
   const list = (await get('/connections')).connections;
   const record = await ensure('GitAccount', list, x => x.name === body.name, { name: body.name, kind: 'GIT', status: 'READY', config: { url: body.url } }, async () => {
    const created = await post('/connections/git', body);return (await get('/connections')).connections.find(x => x.key === created.key);
   }); refs.accounts[body.name] = record.key;
  }
  for (const body of c.vaults) {
   const { token, ...expected } = body;
   await ensure('Vault', (await get('/secret-stores')).secretStores, x => x.name === body.name, { ...expected, status: 'READY' }, async () => {
    const created = await post('/secret-stores', body); return (await get('/secret-stores')).secretStores.find(x => x.key === created.key);
   });
  }
  for (const body of c.resourceTypes) await ensure('ResourceType', (await get('/resource-types')).resourceTypes, x => x.key === body.key, body, async () => {
   await post('/resource-types', body); return (await get('/resource-types')).resourceTypes.find(x => x.key === body.key);
  });
  for (const t of c.templateSources) {
   const list = (await get('/template-sources')).templateSources;
   const expected = { source: { kind: 'git', url: t.url, path: t.path, revision: t.ref, contentSha256: t.contentSha256 }, status: 'AVAILABLE' };
   const record = await ensure('GitTemplate', list, x => x.source?.url === t.url && x.source?.path === t.path && x.source?.revision === t.ref, expected, async () => {
    const created = await post('/template-sources/inspect', { url: t.url, path: t.path, ref: t.ref });
    return (await get('/template-sources/' + encodeURIComponent((created.templateSource ?? created).id))).templateSource;
   }); refs.templateSources[t.name] = record.id;
  }
  for (const doc of c.definitions) {
   const { inspect, ...body } = expand(doc, refs);
   const list = (await get('/resource-definitions')).resourceDefinitions;
   await ensure('Definition', list, x => x.key === body.key, body, async () => {
    if (inspect) {
     const capability = (await post('/terraform-modules/inspect', inspect)).capability;
     // Registration independently verifies the Definition's exact immutable pin.
     if (!capability) fail('Terraform module inspection missing capability');
    }
    await post('/resource-definitions', body);
    return (await get('/resource-definitions')).resourceDefinitions.find(x => x.key === body.key);
   }, definitionMatches);
  }
  for (const d of c.delivery) {
   const accounts = (await get('/driver-accounts')).accounts;
   const account = accounts.find(x => x.key === refs.accounts[d.account]); if (!account) fail('CD management account missing from seed');
   const requested = { ...(d.config ?? {}) };
   if (Object.keys(requested).some(k => !['workspace', 'targetCluster'].includes(k))) fail('CD config accepts workspace and targetCluster only');
   const discovery = await post('/delivery-providers/fleet/inspect', { managementAccountKey: account.key, managementAccountVersion: account.version, config: requested });
   const options = (discovery.options?.targetCluster ?? []).filter(o => !o.disabled && (!requested.workspace || o.values?.workspace === requested.workspace) && (!requested.targetCluster || o.value === requested.targetCluster));
   if (options.length !== 1) fail('CD requires one eligible discovered target; select workspace/targetCluster');
   const config = { ...options[0].values, ...requested };
   if (d.gitAccount) { if (!refs.accounts[d.gitAccount]) fail('CD Git account missing'); config.gitConnectionKey = refs.accounts[d.gitAccount]; }
   const list = (await get('/delivery-bindings')).bindings;
   let b = list.find(x => x.name === d.name);
   if (list.filter(x => x.name === d.name).length > 1) fail('CD: ambiguous identity');
   if (b) {
    const details = await get(`/delivery-bindings/${b.id}?version=${b.version}`);
    if (!matches(details, { managementAccountKey: account.key, managementAccountVersion: account.version, usePlatformGit: d.usePlatformGit ?? false, config }) || b.provider !== 'fleet') fail('CD: CONFLICT');
    emit('CD', 'REUSED');
   } else {
    b = await post('/delivery-bindings', { name: d.name, provider: 'fleet', managementAccountKey: account.key, managementAccountVersion: account.version, usePlatformGit: d.usePlatformGit ?? false, config }); emit('CD', 'CREATED');
   }
   if (b.status !== 'READY') { b = await post(`/delivery-bindings/${b.id}/verify`, { version: b.version, expectedRevision: b.revision }); if (b.binding) b = b.binding; if (b.status !== 'READY') fail('CD verification not READY'); emit('CD', 'VERIFIED'); }
   if (d.setDefault) {
    const old = await get('/delivery-default');
    if (old.bindingId && (old.bindingId !== b.id || old.bindingVersion !== b.version)) fail('CD default: CONFLICT');
    if (!old.bindingId) await api('PUT', '/delivery-default', { bindingId: b.id, bindingVersion: b.version, expectedRevision: old.revision });
   }
  }
  emit('Platform', 'BOOTSTRAPPED');
 } finally {
  if (cookie) await api('POST', '/auth/sign-out').catch(() => {});
 }
}
