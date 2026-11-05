import assert from 'node:assert/strict';
import {parseArgs} from 'node:util';
import {join} from 'node:path';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {loadBootstrapConfig} from '../../../scripts/bootstrap/lib/bootstrap-config.mjs';
import {openConsole,closeConsole} from '../../lib/console.mjs';
import {makeUI} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {definitionDocument,readDefinitionPin,samePin} from '../../../scripts/bootstrap/lib/direct-platform.mjs';
const {values:a}=parseArgs({options:{'staging-config':{type:'string'},'production-config':{type:'string'}}});assert(a['staging-config']&&a['production-config']);
const inputs={staging:a['staging-config'],production:a['production-config']};
const c=loadBootstrapConfig(inputs.staging),production=loadBootstrapConfig(inputs.production);assert.equal(c.runtime.profile,'dev-test');assert.equal(production.console.baseURL,c.console.baseURL);assert.equal(production.application.name,c.application.name);
const p=await openConsole(c.console);
try{
 const ui=makeUI(p.page,c.console.baseURL,120000);
 const mutate=async(path,method,data)=>{const r=await p.page.request.fetch(c.console.baseURL+'/api/v1'+path,{method,data});assert(r.ok(),`Setup ${method} ${path}: HTTP ${r.status()}`);return r.json();};
 const spec=c.platform.templateSources.find(s=>s.name==='acceptance');assert(spec.kind==='git');
 const source=(await mutate('/template-sources/inspect','POST',{url:spec.url,path:spec.path,ref:spec.ref})).templateSource;
 assert(samePin(source.source,spec.pin),'Exact source content pins required');
 const entries=c.platform.definitions.filter(e=>e.definition.driverType==='template');assert.equal(entries.length,1);const key=entries[0].definition.key;let d=(await ui.get('/resource-definitions')).resourceDefinitions.find(d=>d.key===key);assert(d&&d.driverType==='template');
 if(d.driverInputs.values.variables.template_source!==source.id){
  writeFileSync(join(c.runtime.reportDir,Date.now()+'-pre-runtime-definition.json'),JSON.stringify(d,null,2),{mode:0o600});
  const want=definitionDocument(d);want.driverInputs.values.variables.template_source=source.id;
  await mutate('/resource-definitions/'+key,'PUT',{...want,version:d.version});
  d=(await ui.get('/resource-definitions')).resourceDefinitions.find(d=>d.key===key);
 }
 let history=(await ui.get('/resource-definitions/'+key+'/versions')).history;
 const target=history.versions.find(v=>v.definition.driverInputs.values.variables.template_source===source.id&&!v.archived);assert(target);
 if(history.activeId!==target.id)await mutate('/resource-definitions/'+key+'/versions/'+target.id+'/promote','POST',{expectedGeneration:history.generation});
 d=(await ui.get('/resource-definitions')).resourceDefinitions.find(d=>d.key===key);
 const pin=await readDefinitionPin(ui,d);
 for(const env of ['staging','production']){
  const cfg=loadBootstrapConfig(inputs[env]),file=cfg.runtime.stateDir+'/identity.json',state=existsSync(file)?JSON.parse(readFileSync(file)):{};
  if(!state.identity)continue;assert(state.identity.origin===c.console.baseURL);state.templateSources??={};state.templateSources.acceptance={id:source.id,pin:spec.pin};state.definitionPins??={};state.definitionPins[key]=pin;
  writeFileSync(file,JSON.stringify(state,null,2),{mode:0o600});
  if(state.appKey){const base=`/applications/${state.appKey}/environments/${env}`;const versions=await ui.get(base+'/definition-versions');const row=versions.definitions.find(d=>d.key===key);
   if(row&&row.desiredRevisionId!==pin.revisionId){await mutate(base+'/definition-versions','PUT',{definitionKey:key,revisionId:pin.revisionId,expectedRevision:versions.revision,expectedEnvironmentVersion:versions.environmentVersion});console.log('UPDATED: exact workload Definition selection '+env);}
  }
 }
 console.log(JSON.stringify({templateSourceId:source.id,definitionPin:pin}));
}finally{await closeConsole(p,c.console);}
