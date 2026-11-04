import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { parse } from 'yaml';
import { onlyKeys, requireConfig } from './bootstrap-config.mjs';

const object = v => v && typeof v === 'object' && !Array.isArray(v);
const hash = v => createHash('sha256').update(v).digest('hex');
const sha = v => /^[a-f0-9]{64}$/.test(v ?? '');
export function treeDigest(files) {
 const digest = createHash('sha256');
 for (const name of Object.keys(files).sort()) { digest.update(name); digest.update('\0'); digest.update(hash(files[name])); digest.update('\n'); }
 return digest.digest('hex');
}
export function parseSource(source, path, template = false) {
 const label = template ? 'template source' : 'module source';
 onlyKeys(source, ['name','kind','pin','url','path','ref','file','archiveSha256','filesFile'], label);
 requireConfig(['installed','git','zip','inline'].includes(source.kind) && (template || source.kind !== 'inline'), `${label}: unsupported kind`);
 if (template) requireConfig(/^[a-z][a-z0-9-]*$/.test(source.name ?? ''), 'Template source name required');
 if (template) { onlyKeys(source.pin,['contentSha256','revision'],label+' pin'); requireConfig(sha(source.pin.contentSha256) && typeof source.pin.revision === 'string' && source.pin.revision.length > 0, 'Template content/revision pin required'); }
 else { onlyKeys(source.pin,['sourceId','moduleId','revision','subdirectory','contentSha256'],label+' pin'); requireConfig(Object.values(source.pin).every(v=>typeof v==='string'&&v.length>0) && ['sourceId','moduleId','revision','subdirectory'].every(k=>source.pin[k]) && sha(source.pin.contentSha256), 'Complete Terraform source pin required'); }
 if (source.kind === 'git') {
  const url = new URL(source.url);
  requireConfig(url.protocol==='https:'&&!url.username&&!url.password&&!url.search&&!url.hash && /^[a-f0-9]{40}$/.test(source.ref??'') && source.ref===source.pin.revision, 'Git source requires HTTPS and exact pinned commit');
  requireConfig(typeof source.path==='string'&&!source.path.startsWith('/')&&!source.path.split('/').includes('..'),'Relative Git source path required');
 }
 if (source.kind === 'zip') {
  source.file = path(source.file);
  requireConfig(sha(source.archiveSha256)&&hash(readFileSync(source.file))===source.archiveSha256,'ZIP archive SHA256 mismatch');
 }
 if (source.kind === 'inline') {
  source.filesFile=path(source.filesFile);
  source.files=JSON.parse(readFileSync(source.filesFile,'utf8'));
  requireConfig(object(source.files)&&Object.entries(source.files).every(([k,v])=>!k.startsWith('/')&&!k.split('/').includes('..')&&typeof v==='string'),'Invalid Inline files');
  requireConfig(treeDigest(source.files)===source.pin.contentSha256&&source.pin.revision===source.pin.contentSha256,'Inline content pin mismatch');
  const bundle=parse(source.files['bundle.yaml']);
  const resources=Object.keys(source.files).filter(name=>name.endsWith('.tpl'));
  const expectedManifest='apiVersion: orchestrator.dev/template-source/v1\nkind: TemplateSource\nmetadata:\n  name: custom-workload\nrenderer:\n  type: template\n  version: 1.0.0\ninputContract: orchestrator.dev/template-input/v1\ninputsSchema: inputs.schema.json\ndefaults: defaults.yaml\nresources:\n'+['deployment.tpl','service.tpl',...resources.filter(name=>!['deployment.tpl','service.tpl'].includes(name))].map(name=>'  - '+name+'\n').join('');
  requireConfig(source.files['bundle.yaml']===expectedManifest&&Object.keys(source.files).every(name=>['bundle.yaml','inputs.schema.json','defaults.yaml',...resources].includes(name)), 'Inline manifest cannot be represented by Console; use Git/ZIP');
  requireConfig(bundle?.metadata?.name==='custom-workload' && Array.isArray(bundle.resources) && bundle.resources[0]==='deployment.tpl'&&bundle.resources[1]==='service.tpl'&&!bundle.normalize&&!bundle.patches,'Inline bootstrap uses the Console editable deployment/service source; use Git/ZIP for other manifests');
 }
 return source;
}
export function parseDirectPlatform(platform, path, businessFile=path, gitOnly=false) {
 requireConfig(Array.isArray(platform.templateSources)&&platform.templateSources.length>0,'templateSources required');
 for(const source of platform.templateSources){requireConfig(!gitOnly||source.kind==='git','gitOnly template sources must use Git');parseSource(source,path,true);}
 requireConfig(new Set(platform.templateSources.map(s=>s.name)).size===platform.templateSources.length,'Duplicate template source name');
 requireConfig(Array.isArray(platform.definitions)&&platform.definitions.length>0,'Definitions required');
 for(const entry of platform.definitions) {
  onlyKeys(entry,['file','git','source'],'Definition entry');
  requireConfig(Boolean(entry.file)!==Boolean(entry.git),'Definition requires exactly one file or Git document');
  entry.file=businessFile(entry.git?{git:entry.git}:entry.file);
  const definition=JSON.parse(readFileSync(entry.file,'utf8'));
  onlyKeys(definition,['key','name','resourceType','driverType','driverAccount','driverInputs','provision','criteria'],'Definition');
  requireConfig(/^[a-z0-9][a-z0-9-]*$/.test(definition.key??'')&&typeof definition.name==='string'&&definition.name.length>0&&typeof definition.resourceType==='string'&&object(definition.driverInputs?.values)&&Array.isArray(definition.criteria)&&definition.criteria.length>0,'Invalid Definition document');
  requireConfig(['terraform','existing-cluster','template'].includes(definition.driverType),'Unsupported Definition driver');
  requireConfig(definition.criteria.every(rule=>object(rule)&&Object.entries(rule).every(([k,v])=>['env_type','app_id','env_id','res_id','class'].includes(k)&&typeof v==='string'&&v.length>0)),'Invalid Definition criteria');
  if(definition.driverType==='existing-cluster')requireConfig(definition.resourceType==='k8s-cluster'&&typeof definition.driverAccount==='string','Existing cluster requires an account');
  if(definition.driverType==='terraform') {
   requireConfig(!gitOnly||entry.source?.kind==='git','gitOnly Terraform modules must use Git');
   parseSource(entry.source,path);
   const p=entry.source.pin;
   const expected={source_id:p.sourceId,module_id:p.moduleId,revision:p.revision,subdirectory:p.subdirectory,content_sha256:p.contentSha256};
   requireConfig(JSON.stringify(Object.entries(definition.driverInputs.values.source??{}).sort())===JSON.stringify(Object.entries(expected).sort()),'Definition differs from configured module pin');
  } else requireConfig(!entry.source,'Only Terraform Definitions use module source');
  if(definition.driverType==='template') {
   requireConfig(definition.resourceType==='workload','Template driver requires workload Resource Type');
   const name=definition.driverInputs.values.variables?.template_source?.match(/^\$\{templateSources\.([a-z0-9-]+)\}$/)?.[1];
   requireConfig(platform.templateSources.some(s=>s.name===name),'Definition must reference a configured template source');
  }
  entry.definition=definition;
 }
 requireConfig(new Set(platform.definitions.map(e=>e.definition.key)).size===platform.definitions.length,'Duplicate Definition key');
}
