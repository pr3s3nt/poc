import { expandParameters, validateParameters } from './input-parameters.mjs';
import { readFileSync } from 'node:fs';
import { dirname, resolve, join } from 'node:path';
import { homedir } from 'node:os';
import { parse as parseYaml } from 'yaml';
import { parseResourceTypes, parseCatalogConfig } from './catalog-config.mjs';
import { parseInfrastructure } from './infrastructure-config.mjs';
import { parseGitAccess } from './git-access-config.mjs';
import { readConsoleConfig, readPrivateFile } from './console.mjs';
import { createPreparedInputs } from './prepared-inputs.mjs';
import { createGitInputs } from './git-inputs.mjs';
import { parseDirectPlatform } from './direct-config.mjs';

export class BootstrapError extends Error { constructor(message) { super(message); this.name = 'BootstrapError'; } }
export function requireConfig(ok, message) { if (!ok) throw new BootstrapError(message); }
const obj = v => v && typeof v === 'object' && !Array.isArray(v);
const text = v => typeof v === 'string' && v.trim() === v && v.length > 0;
export function onlyKeys(value, keys, label) { requireConfig(obj(value) && Object.keys(value).every(k => keys.includes(k)), `${label}: unknown field or invalid object`); }
export function loadBootstrapConfig(file) {
 const absolute = resolve(file), root = dirname(absolute);
 let raw; try { raw = JSON.parse(readFileSync(absolute, 'utf8')); } catch { throw new BootstrapError('Cannot read JSON config'); }
 onlyKeys(raw, ['schemaVersion','environment','console','developer','platform','application','upgrade','verification','runtime','gitOnly','parameters'], 'config');
 const parameters=validateParameters(raw.parameters??{});
 raw=expandParameters(raw,parameters);
 const direct = raw.schemaVersion === 3;
 requireConfig(raw.gitOnly===undefined||direct&&typeof raw.gitOnly==='boolean','gitOnly requires schemaVersion 3 and boolean');
 requireConfig((direct || raw.schemaVersion === 2) && (direct ? /^[a-z][a-z0-9-]{0,62}$/.test(raw.environment ?? '') : ['dev','staging','production'].includes(raw.environment)), 'schemaVersion must be 2 or 3; explicit environment identity required');
 const platformOnly = direct && ['check','bootstrap'].includes(raw.runtime?.mode);
 if (platformOnly && !raw.developer) raw.developer = raw.console;
 const path = v => { requireConfig(text(v), 'File path required'); return v.startsWith('~/') ? join(homedir(), v.slice(2)) : resolve(root, v); };
 const auth = v => { onlyKeys(v,['url','username','passwordFile'],'console account'); return readConsoleConfig({ORCH_PLATFORM_URL:v.url ?? raw.console.url, ORCH_PLATFORM_USERNAME:v.username, ORCH_PLATFORM_PASSWORD_FILE:path(v.passwordFile), ORCH_HEADLESS:'1'}); };
 onlyKeys(raw.runtime, ['mode','headless','timeoutMs','stateDir','reportDir','clusterKubeconfigFile','recoverCompletedDeploy','gitCacheDir','recordVideo','profile'], 'runtime');
 requireConfig((direct?['all','check','bootstrap']:['all','check','bootstrap','acceptance','upgrade','verify']).includes(raw.runtime.mode), 'runtime.mode invalid');
 requireConfig(typeof raw.runtime.headless === 'boolean' && Number.isInteger(raw.runtime.timeoutMs) && raw.runtime.timeoutMs >= 1000 && raw.runtime.timeoutMs <= 3600000,'runtime headless/timeoutMs invalid');
 requireConfig(obj(raw.console)&&obj(raw.developer),'console/developer account input required');
 requireConfig((raw.developer.url??raw.console.url)===raw.console.url,'Developer and platform Console must be the same target');
 const config = structuredClone(raw);
 config.runtime.profile=raw.runtime.profile??(direct?'console':'dev-test');
 requireConfig(['console','dev-test'].includes(config.runtime.profile)&&(direct||config.runtime.profile==='dev-test'),'Console profile requires schemaVersion 3; profile must be console or dev-test');
 const native=config.runtime.profile==='dev-test';
 if(!native){requireConfig(!raw.platform?.delivery?.namespaceAccess?.enabled,'namespaceAccess is dev-test setup; provision Fleet RBAC outside staging bootstrap');requireConfig(!raw.verification?.forward,'port-forward is dev-test only; configure an accessible workload URL');}
 config.runtime.recordVideo=raw.runtime.recordVideo??false;
 requireConfig(typeof config.runtime.recordVideo==='boolean','runtime.recordVideo must be boolean');
 config.console = auth(raw.console); config.developer = auth(raw.developer);
 config.console.headless = config.developer.headless = raw.runtime.headless;
 if(native){config.runtime.clusterKubeconfigFile = path(raw.runtime.clusterKubeconfigFile); readPrivateFile(config.runtime.clusterKubeconfigFile,'cluster kubeconfig');}else requireConfig(raw.runtime.clusterKubeconfigFile===undefined,'Console profile does not use a runtime cluster kubeconfig');
 config.runtime.recoverCompletedDeploy=raw.runtime.recoverCompletedDeploy??!direct;requireConfig(typeof config.runtime.recoverCompletedDeploy==='boolean','recoverCompletedDeploy invalid');
 config.runtime.stateDir = path(raw.runtime.stateDir); config.runtime.reportDir = path(raw.runtime.reportDir);
 requireConfig(config.runtime.stateDir !== config.runtime.reportDir,'stateDir and reportDir must differ');
 onlyKeys(raw.platform, ['gitAccess','account','resourceTypes','catalog','secretStore','delivery',...(direct?['templateSources','definitions']:[])], 'platform');
 const p = config.platform;
 p.account = parseInfrastructure({account:p.account}).account;
 requireConfig(p.account && text(p.account.context) && text(p.account.endpoint), 'Explicit account context and endpoint required');
 p.account.kubeconfigFile = path(raw.platform.account.kubeconfigFile);
 readPrivateFile(p.account.kubeconfigFile, 'kubeconfig');
 // Business input files may come from a pinned Git object; private credential files stay local.
 let gitInputs;
 const gitAccess=raw.platform.gitAccess?{...raw.platform.gitAccess,tokenFile:raw.platform.gitAccess.tokenFile?path(raw.platform.gitAccess.tokenFile):undefined,caPemFile:raw.platform.gitAccess.caPemFile?path(raw.platform.gitAccess.caPemFile):undefined}:undefined;
 const businessFile=value=>{if(value&&typeof value==='object'&&!Array.isArray(value)){onlyKeys(value,['git'],'Git file reference');gitInputs??=native?createGitInputs(path(raw.runtime.gitCacheDir??join(config.runtime.stateDir,'git-inputs')),gitAccess):createPreparedInputs(path);return gitInputs.read(value.git);}requireConfig(!raw.gitOnly,'gitOnly business inputs must use pinned Git files');return path(value);};
 if(!Array.isArray(p.resourceTypes))p.resourceTypes=JSON.parse(readFileSync(businessFile(p.resourceTypes),'utf8'));
 else requireConfig(!raw.gitOnly,'gitOnly Resource Types must come from Git');
 p.resourceTypes = parseResourceTypes(p.resourceTypes);
 if (p.gitAccess?.tokenFile) p.gitAccess.tokenFile = path(p.gitAccess.tokenFile);
 if (p.gitAccess?.caPemFile) p.gitAccess.caPemFile = path(p.gitAccess.caPemFile);
 p.gitAccess = parseGitAccess(p);
 if (p.gitAccess) readPrivateFile(p.gitAccess.tokenFile, 'Git token');
 if (direct) { requireConfig(p.catalog === undefined, 'schemaVersion 3 does not select a Catalog'); parseDirectPlatform(p, path, businessFile,raw.gitOnly===true,value=>expandParameters(value,parameters)); }
 else p.catalog = parseCatalogConfig(p);
 onlyKeys(p.secretStore,['name','backendAddress','workloadAddress','mount','authMount','tokenFile','tlsCaPemFile'],'secretStore');
 for (const k of ['name','mount','authMount']) requireConfig(text(p.secretStore[k]), `secretStore.${k} required`);
 for (const k of ['backendAddress','workloadAddress']) { const u = new URL(p.secretStore[k]); requireConfig(['http:','https:'].includes(u.protocol) && !u.username && !u.password && !u.search && !u.hash, `secretStore.${k} invalid`); }
 p.secretStore.tokenFile = path(p.secretStore.tokenFile); readPrivateFile(p.secretStore.tokenFile,'Vault token');
 if (p.secretStore.tlsCaPemFile) p.secretStore.tlsCaPemFile=path(p.secretStore.tlsCaPemFile);
 onlyKeys(p.delivery,['name','provider','usePlatformGit','config','writerUsername','writerPasswordFile','writerURL','setDefault','namespaceAccess'],'delivery');
 requireConfig(text(p.delivery.name) && text(p.delivery.provider) && obj(p.delivery.config) && Object.values(p.delivery.config).every(v=>typeof v==='string'), 'delivery config invalid');
 requireConfig(typeof p.delivery.usePlatformGit === 'boolean' && typeof p.delivery.setDefault === 'boolean','delivery flags must be boolean');
 if(p.delivery.namespaceAccess && (native || p.delivery.namespaceAccess.enabled)) {
  onlyKeys(p.delivery.namespaceAccess,['enabled','rulesFile'],'namespaceAccess');
  requireConfig(typeof p.delivery.namespaceAccess.enabled==='boolean','namespaceAccess.enabled invalid');
  p.delivery.namespaceAccess.rulesFile=businessFile(p.delivery.namespaceAccess.rulesFile);
  const policy=JSON.parse(readFileSync(p.delivery.namespaceAccess.rulesFile,'utf8'));
  onlyKeys(policy,['rules'],'namespaceAccess policy');
  requireConfig(Array.isArray(policy.rules)&&policy.rules.every(r=>Array.isArray(r.apiGroups)&&r.apiGroups.every(v=>['','apps','batch','secrets.hashicorp.com','networking.k8s.io'].includes(v))&&Array.isArray(r.resources)&&r.resources.every(v=>['configmaps','secrets','services','serviceaccounts','pods','persistentvolumeclaims','deployments','replicasets','statefulsets','jobs','cronjobs','vaultauths','vaultconnections','vaultstaticsecrets','ingresses'].includes(v))&&Array.isArray(r.verbs)&&r.verbs.every(v=>['get','list','watch','create','update','patch','delete'].includes(v))), 'Only bounded namespaced workload rules allowed');
 }
 if (p.delivery.usePlatformGit) requireConfig(p.gitAccess, 'Shared CD requires gitAccess');
 else if(!p.delivery.config.gitConnectionKey) { requireConfig(typeof p.delivery.writerURL==='string' && p.delivery.writerURL.startsWith('https://'),'delivery.writerURL must name the Git account/group scope'); p.delivery.writerPasswordFile=path(p.delivery.writerPasswordFile); readPrivateFile(p.delivery.writerPasswordFile,'CD credential'); }
 if (platformOnly) { requireConfig(!raw.upgrade, 'Direct bootstrap does not perform upgrades'); config.gitInputReceipts=gitInputs?.receipts??[]; return config; }
 onlyKeys(raw.application,['name','subdomain','expectedKey','environment','variables','secretFiles','scores','workloadClass','images','catalogTag','definitionRevisions',...(direct?['workloads']:[])],'application');
 const app=config.application;
 if (direct) { requireConfig(app.catalogTag === undefined && app.scores === undefined && !raw.upgrade, 'Direct acceptance uses workloads and no Catalog/upgrade'); requireConfig(Object.keys(app.definitionRevisions??{}).length===0,'Direct bootstrap does not select upgrade revisions'); app.scores = app.workloads; }
 requireConfig(text(app.name) && /^[a-z0-9][a-z0-9-]{0,62}$/.test(app.subdomain) && ['staging','production'].includes(app.environment),'application identity/environment invalid');
 requireConfig(obj(app.variables) && Object.values(app.variables).every(v=>typeof v==='string') && obj(app.secretFiles),'variables/secretFiles required');
 for (const [k,v] of Object.entries(app.secretFiles)) { app.secretFiles[k]=path(v); readPrivateFile(app.secretFiles[k],'application secret'); }
 requireConfig(obj(app.scores) && ['backend','worker','frontend'].every(k=>text(app.scores[k])||direct&&obj(app.scores[k])), 'Three acceptance Workload paths required');
 for (const [k,v] of Object.entries(app.scores)) { app.scores[k]=direct?businessFile(v):path(v); let workload; try { workload=expandParameters(parseYaml(readFileSync(app.scores[k],'utf8').replace(/@@(\w+)_IMAGE@@/g,(_,w)=>app.images?.[w.toLowerCase()]??'invalid:pin')),parameters); } catch { throw new BootstrapError(`Invalid Workload: ${k}`); } requireConfig(workload?.metadata?.name===k,`Workload identity differs: ${k}`); }
 const version = (v,label) => { requireConfig(obj(v.images) && ['backend','worker','frontend'].every(k=>text(v.images[k]) && /(?::[A-Za-z0-9_][A-Za-z0-9._-]*|@sha256:[a-f0-9]{64})$/.test(v.images[k]) && !v.images[k].endsWith(':latest')), `${label}: explicit images required`); if (!direct) requireConfig(/^v?\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(v.catalogTag??''), `${label}: explicit SemVer Catalog tag required`); requireConfig(obj(v.definitionRevisions??{}) && Object.values(v.definitionRevisions??{}).every(text),`${label}: definition revisions invalid`); };
 version(app,'application');
 if (!direct) {
 onlyKeys(raw.upgrade,['enabled','images','catalogTag','definitionRevisions'],'upgrade');
 config.upgrade.enabled = raw.upgrade.enabled ?? true; requireConfig(typeof config.upgrade.enabled==='boolean','upgrade.enabled must be boolean');
 version(config.upgrade,'upgrade');
 } else { config.upgrade = { ...app, enabled: false }; requireConfig(raw.runtime.mode !== 'upgrade', 'Direct bootstrap does not perform upgrades'); requireConfig(raw.verification?.enabled === true, 'Direct full bootstrap requires acceptance verification'); }
 onlyKeys(raw.verification,['enabled','url','jobPayload','timeoutMs','forward',...(direct?['storage']:[])],'verification');
 requireConfig(typeof raw.verification.enabled==='boolean' && text(raw.verification.jobPayload) && Number.isInteger(raw.verification.timeoutMs) && raw.verification.timeoutMs>=1000,'verification invalid');
 if (raw.verification.forward) { onlyKeys(raw.verification.forward,['bindAddress','port'],'verification.forward'); requireConfig(text(raw.verification.forward.bindAddress) && Number.isInteger(raw.verification.forward.port) && raw.verification.forward.port>1024 && raw.verification.forward.port<65536,'forward address/port invalid'); }
 if(raw.verification.storage){onlyKeys(raw.verification.storage,['enabled','value'],'verification.storage');requireConfig(typeof raw.verification.storage.enabled==='boolean'&&text(raw.verification.storage.value)&&Buffer.byteLength(raw.verification.storage.value)<=4096,'Storage verification needs a bounded public test value');}
 const access=new URL(raw.verification.url); requireConfig(['http:','https:'].includes(access.protocol) && !access.username && !access.password && !access.search && !access.hash,'verification URL invalid');
 const forbidden = ['localhost','127.0.0.1','0.0.0.0']; requireConfig(!forbidden.includes(access.hostname),'verification.url must be the Tailscale handoff URL');
 config.gitInputReceipts=gitInputs?.receipts??[];
 return config;
}
