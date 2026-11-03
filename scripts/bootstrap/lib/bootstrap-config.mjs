import { readFileSync } from 'node:fs';
import { dirname, resolve, join } from 'node:path';
import { homedir } from 'node:os';
import { parse as parseYaml } from 'yaml';
import { parseResourceTypes, parseCatalogConfig } from './catalog-config.mjs';
import { parseInfrastructure } from './infrastructure-config.mjs';
import { parseGitAccess } from './git-access-config.mjs';
import { readConsoleConfig, readPrivateFile } from './console.mjs';

export class BootstrapError extends Error { constructor(message) { super(message); this.name = 'BootstrapError'; } }
export function requireConfig(ok, message) { if (!ok) throw new BootstrapError(message); }
const obj = v => v && typeof v === 'object' && !Array.isArray(v);
const text = v => typeof v === 'string' && v.trim() === v && v.length > 0;
export function onlyKeys(value, keys, label) { requireConfig(obj(value) && Object.keys(value).every(k => keys.includes(k)), `${label}: unknown field or invalid object`); }
export function loadBootstrapConfig(file) {
 const absolute = resolve(file), root = dirname(absolute);
 let raw; try { raw = JSON.parse(readFileSync(absolute, 'utf8')); } catch { throw new BootstrapError('Cannot read JSON config'); }
 onlyKeys(raw, ['schemaVersion','environment','console','developer','platform','application','upgrade','verification','runtime'], 'config');
 requireConfig(raw.schemaVersion === 2 && ['dev','staging','production'].includes(raw.environment), 'schemaVersion must be 2; environment must be dev/staging/production');
 const path = v => { requireConfig(text(v), 'File path required'); return v.startsWith('~/') ? join(homedir(), v.slice(2)) : resolve(root, v); };
 const auth = v => { onlyKeys(v,['url','username','passwordFile'],'console account'); return readConsoleConfig({ORCH_PLATFORM_URL:v.url ?? raw.console.url, ORCH_PLATFORM_USERNAME:v.username, ORCH_PLATFORM_PASSWORD_FILE:path(v.passwordFile), ORCH_HEADLESS:'1'}); };
 onlyKeys(raw.runtime, ['mode','headless','timeoutMs','stateDir','reportDir','clusterKubeconfigFile','recoverCompletedDeploy'], 'runtime');
 requireConfig(['all','check','bootstrap','acceptance','upgrade','verify'].includes(raw.runtime.mode), 'runtime.mode invalid');
 requireConfig(typeof raw.runtime.headless === 'boolean' && Number.isInteger(raw.runtime.timeoutMs) && raw.runtime.timeoutMs >= 1000 && raw.runtime.timeoutMs <= 3600000,'runtime headless/timeoutMs invalid');
 requireConfig((raw.developer.url??raw.console.url)===raw.console.url,'Developer and platform Console must be the same target');
 const config = structuredClone(raw);
 config.console = auth(raw.console); config.developer = auth(raw.developer);
 config.console.headless = config.developer.headless = raw.runtime.headless;
 config.runtime.clusterKubeconfigFile = path(raw.runtime.clusterKubeconfigFile); readPrivateFile(config.runtime.clusterKubeconfigFile,'cluster kubeconfig');
 config.runtime.recoverCompletedDeploy=raw.runtime.recoverCompletedDeploy??true;requireConfig(typeof config.runtime.recoverCompletedDeploy==='boolean','recoverCompletedDeploy invalid');
 config.runtime.stateDir = path(raw.runtime.stateDir); config.runtime.reportDir = path(raw.runtime.reportDir);
 requireConfig(config.runtime.stateDir !== config.runtime.reportDir,'stateDir and reportDir must differ');
 onlyKeys(raw.platform, ['gitAccess','account','resourceTypes','catalog','secretStore','delivery'], 'platform');
 const p = config.platform;
 p.account = parseInfrastructure({account:p.account}).account;
 requireConfig(p.account && text(p.account.context) && text(p.account.endpoint), 'Explicit account context and endpoint required');
 p.account.kubeconfigFile = path(raw.platform.account.kubeconfigFile);
 readPrivateFile(p.account.kubeconfigFile, 'kubeconfig');
 p.resourceTypes = parseResourceTypes(p.resourceTypes);
 if (p.gitAccess?.tokenFile) p.gitAccess.tokenFile = path(p.gitAccess.tokenFile);
 if (p.gitAccess?.caPemFile) p.gitAccess.caPemFile = path(p.gitAccess.caPemFile);
 p.gitAccess = parseGitAccess(p);
 if (p.gitAccess) readPrivateFile(p.gitAccess.tokenFile, 'Git token');
 p.catalog = parseCatalogConfig(p);
 onlyKeys(p.secretStore,['name','backendAddress','workloadAddress','mount','authMount','tokenFile','tlsCaPemFile'],'secretStore');
 for (const k of ['name','mount','authMount']) requireConfig(text(p.secretStore[k]), `secretStore.${k} required`);
 for (const k of ['backendAddress','workloadAddress']) { const u = new URL(p.secretStore[k]); requireConfig(['http:','https:'].includes(u.protocol) && !u.username && !u.password && !u.search && !u.hash, `secretStore.${k} invalid`); }
 p.secretStore.tokenFile = path(p.secretStore.tokenFile); readPrivateFile(p.secretStore.tokenFile,'Vault token');
 if (p.secretStore.tlsCaPemFile) p.secretStore.tlsCaPemFile=path(p.secretStore.tlsCaPemFile);
 onlyKeys(p.delivery,['name','provider','usePlatformGit','config','writerUsername','writerPasswordFile','writerURL','setDefault','namespaceAccess'],'delivery');
 requireConfig(text(p.delivery.name) && text(p.delivery.provider) && obj(p.delivery.config) && Object.values(p.delivery.config).every(v=>typeof v==='string'), 'delivery config invalid');
 requireConfig(typeof p.delivery.usePlatformGit === 'boolean' && typeof p.delivery.setDefault === 'boolean','delivery flags must be boolean');
 if(p.delivery.namespaceAccess) {
  onlyKeys(p.delivery.namespaceAccess,['enabled','rulesFile'],'namespaceAccess');
  requireConfig(typeof p.delivery.namespaceAccess.enabled==='boolean','namespaceAccess.enabled invalid');
  p.delivery.namespaceAccess.rulesFile=path(p.delivery.namespaceAccess.rulesFile);
  const policy=JSON.parse(readFileSync(p.delivery.namespaceAccess.rulesFile,'utf8'));
  onlyKeys(policy,['rules'],'namespaceAccess policy');
  requireConfig(Array.isArray(policy.rules)&&policy.rules.every(r=>Array.isArray(r.apiGroups)&&r.apiGroups.every(v=>['','apps','batch','secrets.hashicorp.com','networking.k8s.io'].includes(v))&&Array.isArray(r.resources)&&r.resources.every(v=>['configmaps','secrets','services','serviceaccounts','pods','persistentvolumeclaims','deployments','replicasets','statefulsets','jobs','cronjobs','vaultauths','vaultconnections','vaultstaticsecrets','ingresses'].includes(v))&&Array.isArray(r.verbs)&&r.verbs.every(v=>['get','list','watch','create','update','patch','delete'].includes(v))), 'Only bounded namespaced workload rules allowed');
 }
 if (p.delivery.usePlatformGit) requireConfig(p.gitAccess, 'Shared CD requires gitAccess');
 else if(!p.delivery.config.gitConnectionKey) { requireConfig(typeof p.delivery.writerURL==='string' && p.delivery.writerURL.startsWith('https://'),'delivery.writerURL must name the Git account/group scope'); p.delivery.writerPasswordFile=path(p.delivery.writerPasswordFile); readPrivateFile(p.delivery.writerPasswordFile,'CD credential'); }
 onlyKeys(raw.application,['name','subdomain','expectedKey','environment','variables','secretFiles','scores','workloadClass','images','catalogTag','definitionRevisions'],'application');
 const app=config.application;
 requireConfig(text(app.name) && /^[a-z0-9][a-z0-9-]{0,62}$/.test(app.subdomain) && ['staging','production'].includes(app.environment),'application identity/environment invalid');
 requireConfig(obj(app.variables) && Object.values(app.variables).every(v=>typeof v==='string') && obj(app.secretFiles),'variables/secretFiles required');
 for (const [k,v] of Object.entries(app.secretFiles)) { app.secretFiles[k]=path(v); readPrivateFile(app.secretFiles[k],'application secret'); }
 requireConfig(obj(app.scores) && ['backend','worker','frontend'].every(k=>text(app.scores[k])), 'Three acceptance Score paths required');
 for (const [k,v] of Object.entries(app.scores)) { app.scores[k]=path(v); let score; try { score=parseYaml(readFileSync(app.scores[k],'utf8').replace(/@@(\w+)_IMAGE@@/g,(_,w)=>app.images?.[w.toLowerCase()]??'invalid:pin')); } catch { throw new BootstrapError(`Invalid Score: ${k}`); } requireConfig(score?.metadata?.name===k,`Score identity differs: ${k}`); }
 const version = (v,label) => { requireConfig(obj(v.images) && ['backend','worker','frontend'].every(k=>text(v.images[k]) && /(?::[A-Za-z0-9_][A-Za-z0-9._-]*|@sha256:[a-f0-9]{64})$/.test(v.images[k]) && !v.images[k].endsWith(':latest')), `${label}: explicit images required`); requireConfig(/^v?\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(v.catalogTag??''), `${label}: explicit SemVer Catalog tag required`); requireConfig(obj(v.definitionRevisions??{}) && Object.values(v.definitionRevisions??{}).every(text),`${label}: definition revisions invalid`); };
 version(app,'application');
 onlyKeys(raw.upgrade,['enabled','images','catalogTag','definitionRevisions'],'upgrade');
 config.upgrade.enabled = raw.upgrade.enabled ?? true; requireConfig(typeof config.upgrade.enabled==='boolean','upgrade.enabled must be boolean');
 version(config.upgrade,'upgrade');
 onlyKeys(raw.verification,['enabled','url','jobPayload','timeoutMs','forward'],'verification');
 requireConfig(typeof raw.verification.enabled==='boolean' && text(raw.verification.jobPayload) && Number.isInteger(raw.verification.timeoutMs) && raw.verification.timeoutMs>=1000,'verification invalid');
 if (raw.verification.forward) { onlyKeys(raw.verification.forward,['bindAddress','port'],'verification.forward'); requireConfig(text(raw.verification.forward.bindAddress) && Number.isInteger(raw.verification.forward.port) && raw.verification.forward.port>1024 && raw.verification.forward.port<65536,'forward address/port invalid'); }
 const access=new URL(raw.verification.url); requireConfig(['http:','https:'].includes(access.protocol) && !access.username && !access.password && !access.search && !access.hash,'verification URL invalid');
 const forbidden = ['localhost','127.0.0.1','0.0.0.0']; requireConfig(!forbidden.includes(access.hostname),'verification.url must be the Tailscale handoff URL');
 return config;
}
