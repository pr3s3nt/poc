import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync,readFileSync,writeFileSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {prepareEnterpriseInput} from './enterprise-prepare.mjs';
import {loadBootstrapConfig} from './bootstrap-config.mjs';
import {preflight} from './runner.mjs';
import {kubectl,verifyBackendStorage} from './verification.mjs';
import {verifyDirectAcceptance} from './direct-runner.mjs';
import {desiredScores} from './workloads.mjs';
import {createRuntime} from './runtime.mjs';
const resources=fileURLToPath(new URL('../../../projects/platform-resources/',import.meta.url));
function fixture(){
 const dir=mkdtempSync(join(tmpdir(),'console-bootstrap-'));
 const raw=JSON.parse(readFileSync(new URL('../enterprise/staging.input.json',import.meta.url)));
 for(const key of ['inputsCommit','modulesCommit','templatesCommit'])raw.parameters[key]='a'.repeat(40);
 const credential=join(dir,'credential');writeFileSync(credential,'private-test-value',{mode:0o600});
 raw.console.passwordFile=raw.developer.passwordFile=raw.platform.gitAccess.tokenFile=raw.platform.account.kubeconfigFile=raw.platform.secretStore.tokenFile=raw.application.secretFiles.ACCEPTANCE_SECRET=credential;
 delete raw.platform.gitAccess.caPemFile;delete raw.platform.secretStore.tlsCaPemFile;
 raw.runtime.mode='all';raw.runtime.stateDir=join(dir,'state');raw.runtime.reportDir=join(dir,'reports');
 const file=join(dir,'prepared.json'),load=value=>{writeFileSync(file,JSON.stringify(value),{mode:0o600});return loadBootstrapConfig(file);};
 return {dir,raw,load,close(){rmSync(dir,{force:true,recursive:true});}};
}
test('Console preparation, load and preflight need no CLI on PATH; hashes fail closed',async()=>{
 const f=fixture(),previous=process.env.PATH;process.env.PATH='';
 try{
  const prepared=prepareEnterpriseInput(f.raw,{inputs:resources,outputDir:f.dir});
  const config=f.load(prepared);assert.equal(config.runtime.profile,'console');assert.equal(config.runtime.clusterKubeconfigFile,undefined);
  assert.equal(config.platform.definitions.length,9);assert.equal(config.gitInputReceipts.length,13);
  const template=config.platform.definitions.find(d=>d.definition.resourceType==='workload').definition.driverInputs.values.variables.template_inputs;assert.equal(template.httpRouteGatewayName,config.parameters.httprouteGatewayName);assert.equal(template.httpRouteEnabled,true);assert.equal(desiredScores(config,config.application).frontend.metadata.annotations['routing.orchestrator.dev/hostname'],config.parameters.httprouteHostname);assert.ok(!config.platform.definitions.some(d=>d.definition.resourceType==='httproute'));assert.equal(desiredScores(config,config.application).frontend.resources.public_http,undefined);
  assert.ok(config.gitInputReceipts.every(r=>r.verification==='local-sha256'&&r.gitCommitVerified===false));
  const responses={'/healthz':{},'/driver-accounts':{accounts:[]},'/resource-types':{resourceTypes:[]},'/secret-stores':{secretStores:[]},'/delivery-bindings':{bindings:[]},'/delivery-providers':{providers:[{kind:'fleet'}]},'/resource-definition-capabilities':{capabilities:[{resourceType:'workload',driverType:'template'}]},'/template-sources':{templateSources:[]},'/resource-definitions':{resourceDefinitions:[]},'/delivery-default':{}};
  const steps=[];await preflight({get:async path=>{assert.ok(Object.hasOwn(responses,path),path);return responses[path];}},config,{state:{},record:(...args)=>steps.push(args)});
  assert.ok(!steps.some(([name])=>name==='cluster-capabilities'));
  const key='company-staging-workload',workloads=desiredScores(config,config.application),rendering=Object.fromEntries(Object.keys(workloads).map(name=>[name,{driverType:'template',definitionKey:key,bundle:{id:'template-id',source:{contentSha256:'b'.repeat(64)}}}]));
  const base='/applications/app/environments/'+config.application.environment;
  const observed={[base+'/catalog']:{catalog:{}},[base+'/definition-versions']:{pending:false,definitions:[{key,desiredRevisionId:'r1',appliedRevisionId:'r1',actualRevisionIds:['r1'],inUse:true}]},[base+'/workloads']:{workloads:Object.entries(workloads).map(([id,workload])=>({id,workload,ready:true}))},[base+'/deployments']:{deployments:[{id:'d1',status:'SUCCEEDED'}]},[base+'/deployments/d1']:{rendering}};
  const receipt={state:{appKey:'app',definitionPins:{[key]:{revisionId:'r1'}},templateSources:{acceptance:{id:'template-id',pin:{contentSha256:'b'.repeat(64)}}}},save(){},record:(...args)=>steps.push(args)};
  await verifyDirectAcceptance({get:async path=>{assert.ok(Object.hasOwn(observed,path),path);return observed[path];}},config,{key:'app'},receipt);
  assert.deepEqual(receipt.state.acceptanceSnapshot,{deploymentIds:['d1']});
  const value=config.verification.storage.value,hash=(await import('node:crypto')).createHash('sha256').update(value).digest('hex');
  await verifyBackendStorage({locator:selector=>({count:async()=>1,textContent:async()=>selector==='#storage-hash'?hash:value})},config,receipt);
  assert.deepEqual(receipt.state.backendStorage,{verification:'browser-file',hash});assert.ok(steps.some(([name])=>name==='backend-browser-file'));

  const ref=prepared.platform.resourceTypes.git,bytes=readFileSync(join(f.dir,ref.preparedFile));
  writeFileSync(join(f.dir,ref.preparedFile),'[]');assert.throws(()=>f.load(prepared),/SHA256/);writeFileSync(join(f.dir,ref.preparedFile),bytes);
  delete ref.preparedFile;assert.throws(()=>f.load(prepared),/preparedFile/);
 }finally{process.env.PATH=previous;f.close();}
});
test('Console rejects native setup and cannot reuse native evidence',()=>{
 const f=fixture();try{
  const prepared=prepareEnterpriseInput(f.raw,{inputs:resources,outputDir:f.dir});
  const config=f.load(prepared);assert.throws(()=>kubectl(config,'get','nodes'),/dev-test/);
  prepared.platform.delivery.namespaceAccess={enabled:true};assert.throws(()=>f.load(prepared),/namespaceAccess/);delete prepared.platform.delivery.namespaceAccess;
  prepared.verification.forward={bindAddress:'127.0.0.1',port:9000};assert.throws(()=>f.load(prepared),/port-forward/);delete prepared.verification.forward;
  const rt=createRuntime(config);rt.save();rt.close('CHECKED');
  config.runtime.profile='dev-test';assert.throws(()=>createRuntime(config),/execution profile/);
 }finally{f.close();}
});
