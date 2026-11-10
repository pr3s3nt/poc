// First-installation Definition import. All Git access uses backend settings.
import { dirname, resolve } from 'node:path';
import { homedir } from 'node:os';
import { SeedError, readInput, privateFile, target, definitionMatches, matches } from './api-seed.mjs';

const fail = message => { throw new SeedError(message); };
const commit = /^[a-f0-9]{40}$/;
const digest = /^[a-f0-9]{64}$/;
const directory = value => typeof value === 'string' && value.length > 0 && !value.startsWith('/') && !value.includes('\\') && value.split('/').every(p => p && p !== '.' && p !== '..');

function checkTypeMappings(d, type) {
 const values = d.driverInputs.values;
 const inputs = Object.values(values.input_mapping ?? {}).map(x => x.param);
 const outputs = Object.values(values.output_mapping ?? {});
 if ((type.inputs ?? []).some(x => !inputs.includes(x.name)) || inputs.some(name => !(type.inputs ?? []).some(x => x.name === name)) || (type.outputs ?? []).some(x => x.required && !outputs.includes(x.name)) || outputs.some(name => !(type.outputs ?? []).some(x => x.name === name))) fail(`Definition ${d.key}: input/output mappings differ from existing Resource Type ${d.resourceType}; review input or omit this Definition`);
}

export function substitute(value, parameters) {
 if (typeof value === 'string') return value.replace(/\$\{seed\.([A-Za-z][A-Za-z0-9]*)\}/g, (_, key) => {
  if (typeof parameters[key] !== 'string' || !parameters[key]) fail(`Missing seed parameter: ${key}`);
  return parameters[key];
 });
 if (Array.isArray(value)) return value.map(v => substitute(v, parameters));
 if (value && typeof value === 'object') return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, substitute(v, parameters)]));
 return value;
}

export function loadDefinitionConfig(file) {
 const c = readInput(file);
 if (!c || c.schemaVersion !== 1 || Object.keys(c).some(k => !['schemaVersion', 'console', 'repository', 'parameters', 'resourceTypes', 'definitions'].includes(k))) fail('Invalid Definition seed schema');
 if (!c.console?.username || !c.console.passwordFile || !c.console.expectedOrganization) fail('Console identity, expectedOrganization and passwordFile are required');
 c.console.origin = target(c.console.url, c.console.tailnet);
 const passwordPath = c.console.passwordFile;
 c.console.password = privateFile(passwordPath.startsWith('~/') ? resolve(homedir(), passwordPath.slice(2)) : resolve(dirname(file), passwordPath));
 let url;
 try { url = new URL(c.repository?.url); } catch { fail('Invalid Git repository URL'); }
 if (Object.keys(c.repository).some(k => !['url','ref'].includes(k)) || url.protocol !== 'https:' || url.username || url.password || url.search || url.hash || !commit.test(c.repository.ref)) fail('Git requires credential-free HTTPS URL and full 40-character commit');
 if (!c.parameters || typeof c.parameters.driverAccountKey !== 'string' || !c.parameters.driverAccountKey) fail('Existing driverAccountKey is required');
 if (!Array.isArray(c.definitions) || !c.definitions.length) fail('Definitions are required');
 if (c.resourceTypes === undefined) c.resourceTypes = [];
 if (!Array.isArray(c.resourceTypes)) fail('Invalid Resource Types');
 const typeKeys = new Set();
 for (const type of c.resourceTypes) {
  if (!type || typeof type.key !== 'string' || !/^[a-z0-9][a-z0-9_-]{0,127}$/.test(type.key) || typeKeys.has(type.key) || Object.keys(type).some(k => !['key','inputs','outputs'].includes(k))) fail('Invalid or duplicate Resource Type');
  typeKeys.add(type.key);
  for (const field of ['inputs','outputs']) {
   if (!Array.isArray(type[field])) fail('Resource Type inputs/outputs must be arrays');
   const names = new Set();
   for (const entry of type[field]) {
    if (!entry || typeof entry.name !== 'string' || !entry.name || names.has(entry.name) || !['string','number','bool','any'].includes(entry.type) || Object.keys(entry).some(k => !['name','type','required',...(field === 'outputs' ? ['secret'] : [])].includes(k)) || ['required','secret'].some(k => entry[k] !== undefined && typeof entry[k] !== 'boolean')) fail('Invalid or duplicate Resource Type field');
    names.add(entry.name);
   }
  }
 }
 c.definitions = substitute(c.definitions, c.parameters);
 const keys = new Set();
 for (const d of c.definitions) {
  if (!d || typeof d.key !== 'string' || !/^[a-z0-9][a-z0-9_-]{0,127}$/.test(d.key) || keys.has(d.key) || !d.name || !d.resourceType || !d.driverInputs?.values) fail('Invalid or duplicate Definition');
  keys.add(d.key);
  const fields = ['key','name','resourceType','driverType','driverAccount','driverInputs','provision','criteria','modulePath','templatePath'];
  if (Object.keys(d).some(k => !fields.includes(k))) fail('Unsupported Definition field');
  if (!['terraform','template','existing-cluster'].includes(d.driverType)) fail('Unsupported Definition driver');
  if (d.driverType === 'terraform' ? !directory(d.modulePath) || d.templatePath || d.driverInputs.values.source : d.driverType === 'template' ? !directory(d.templatePath) || d.modulePath || d.driverInputs.values.variables?.template_source : d.modulePath || d.templatePath) fail('Definition source must be supplied by inspection');
 }
 return c;
}

export async function seedDefinitions(c, { fetcher = fetch, report = console.log } = {}) {
 let cookie;
 async function api(method, path, body) {
  let response;
  try {
   response = await fetcher(c.console.origin + '/api/v1' + path, {
    method, redirect: 'error', signal: AbortSignal.timeout(180000),
    headers: { 'Content-Type': 'application/json', ...(cookie ? { Cookie: cookie } : {}) },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
   });
  } catch { fail(`${method} ${path}: transport failed; read server state before retry`); }
  if (!response.ok) {
   let code = '', field = '';
   try {
    const b = await response.json();
    if (/^[A-Z][A-Z0-9_]{0,80}$/.test(b.code)) code = b.code;
    if (typeof b.field === 'string' && /^[a-zA-Z][a-zA-Z0-9_.]{0,160}$/.test(b.field)) field = ` field=${b.field}`;
   } catch { /* No raw provider diagnostics. */ }
   fail(`${method} ${path}: HTTP ${response.status} ${code}${field}`);
  }
  let result = {};
  if (response.status !== 204) { try { result = await response.json(); } catch { fail(`${method} ${path}: invalid response`); } }
  return { response, result };
 }
 const get = async path => (await api('GET', path)).result;
 const post = async (path, body) => (await api('POST', path, body)).result;
 try {
  const login = await api('POST', '/auth/sign-in', { username: c.console.username, password: c.console.password });
  cookie = login.response.headers.getSetCookie()[0]?.split(';')[0];
  if (!cookie) fail('Sign-in did not provide session');
  if (login.result.user?.OrganizationKey !== c.console.expectedOrganization || !['ADMIN','PLATFORM_ENGINEER'].includes(login.result.user?.Role)) fail('Unexpected Organization or insufficient role');
  const types = (await get('/resource-types')).resourceTypes;
  const accounts = (await get('/driver-accounts')).accounts;
  const account = accounts.find(a => a.key === c.parameters.driverAccountKey);
  if (!account || account.type !== 'INTERNAL' || account.status !== 'READY') fail('Existing Kubernetes Driver Account must be READY');
  const typePlan = [];
  for (const input of c.resourceTypes ?? []) {
   const found = types.filter(t => t.key === input.key);
   if (found.length > 1 || found.length === 1 && !matches(found[0], input)) fail(`ResourceType ${input.key}: CONFLICT`);
   typePlan.push({ input, reused: found.length === 1 });
  }
  const effectiveTypes = [...types, ...typePlan.filter(t => !t.reused).map(t => t.input)];
  for (const d of c.definitions) {
   const type = effectiveTypes.find(t => t.key === d.resourceType);
   if (!type) fail(`Missing Resource Type: ${d.resourceType}`);
   if (d.driverAccount && d.driverAccount !== account.key) fail(`Unexpected Driver Account: ${d.key}`);
   if (d.modulePath) checkTypeMappings(d, type);
  }
  // Check every declared Type conflict and Definition mapping before creating Types.
  for (const { input, reused } of typePlan) {
   if (!reused) {
    await post('/resource-types', input);
    const actual = (await get('/resource-types')).resourceTypes.find(t => t.key === input.key);
    if (!actual || !matches(actual, input)) fail(`ResourceType ${input.key}: read-back mismatch`);
   }
   report(`ResourceType ${input.key} ${reused ? 'REUSED' : 'CREATED'}`);
  }
  const existing = (await get('/resource-definitions')).resourceDefinitions;
  const planned = [];
  for (const input of c.definitions) {
   const { modulePath, templatePath, ...d } = structuredClone(input);
   if (modulePath) {
    const capability = (await post('/terraform-modules/inspect', { ...c.repository, path: modulePath, resourceType: d.resourceType })).capability;
    const pin = capability?.source;
    if (!pin || capability.resourceType !== d.resourceType || capability.driverType !== 'terraform' || pin.repositoryUrl !== c.repository.url || pin.revision !== c.repository.ref || pin.subdirectory !== modulePath || !pin.sourceId || !pin.moduleId || !digest.test(pin.contentSha256)) fail(`Module inspection pin mismatch: ${d.key}`);
    d.driverInputs.values.source = { source_id: pin.sourceId, module_id: pin.moduleId, revision: pin.revision, subdirectory: pin.subdirectory, content_sha256: pin.contentSha256 };
   }
   if (templatePath) {
    const source = (await post('/template-sources/inspect', { ...c.repository, path: templatePath })).templateSource;
    if (!source?.id || source.status !== 'AVAILABLE' || source.source?.url !== c.repository.url || source.source?.revision !== c.repository.ref || source.source?.path !== templatePath || !digest.test(source.source?.contentSha256)) fail(`Template inspection pin mismatch: ${d.key}`);
    d.driverInputs.values.variables = { ...d.driverInputs.values.variables, template_source: source.id };
   }
   const found = existing.filter(x => x.key === d.key);
   if (found.length > 1 || found.length === 1 && !definitionMatches(found[0], d)) fail(`Definition ${d.key}: CONFLICT`);
   planned.push({ definition: d, reused: found.length === 1 });
  }
  // Check every existing Definition before creating any new Definition.
  for (const { definition: d, reused } of planned) {
   if (!reused) {
    await post('/resource-definitions', d);
    const actual = (await get('/resource-definitions')).resourceDefinitions.find(x => x.key === d.key);
    if (!actual || !definitionMatches(actual, d)) fail(`Definition ${d.key}: read-back mismatch`);
   }
   report(`Definition ${d.key} ${reused ? 'REUSED' : 'CREATED'}`);
  }
  report('Definitions SEEDED');
 } finally {
  if (cookie) await api('POST', '/auth/sign-out').catch(() => {});
 }
}
