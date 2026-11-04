import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync,mkdirSync,readFileSync,writeFileSync,rmSync} from 'node:fs';
import {join,dirname} from 'node:path';
import {tmpdir} from 'node:os';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {lockEnterpriseInput} from './enterprise-lock.mjs';
import {expandParameters} from './input-parameters.mjs';
import {loadBootstrapConfig} from './bootstrap-config.mjs';
const sha=x=>createHash('sha256').update(x).digest('hex');
test('enterprise locking reads exact local commits, derives pins, stays idempotent and rejects wrong origins',()=>{
 const root=mkdtempSync(join(tmpdir(),'enterprise-lock-'));
 const raw=JSON.parse(readFileSync(new URL('../enterprise/staging.input.json',import.meta.url)));
 raw.runtime.profile='dev-test';
 const git=args=>execFileSync('git',['-C',root,...args],{encoding:'utf8',stdio:['ignore','pipe','pipe']}).trim();
 try{
  const copy=(name,content)=>{mkdirSync(dirname(join(root,name)),{recursive:true});writeFileSync(join(root,name),content);};
  const visit=value=>{if(!value||typeof value!=='object')return;if(value.git){const name=value.git.path;copy(name,readFileSync(new URL('../../../projects/platform-resources/'+name,import.meta.url)));}for(const v of Object.values(value))visit(v);};visit(raw);
  for(const kind of ['namespace','postgres','redis','storage','mysql','mariadb','mongodb'])copy(raw.parameters[kind+'ModulePath']+'/main.tf',`variable "namespace" {type=string}\n# ${kind}\n`);
  for(const kind of ['mysql','mariadb','mongodb'])copy(raw.parameters[kind+'ModulePath']+'/main.tf',readFileSync(new URL('../../../projects/platform-resources/terraform/'+kind+'/main.tf',import.meta.url)));
  copy(raw.platform.templateSources[0].path+'/bundle.yaml','kind: TemplateSource\n');
  git(['init','--quiet']);git(['config','user.email','test@localhost']);git(['config','user.name','Bootstrap Test']);git(['remote','add','origin',raw.parameters.inputsRepository]);git(['add','.']);git(['commit','--quiet','-m','Fixture sources']);
  for(const kind of ['postgres','redis','mysql','mariadb','mongodb'])raw.parameters[kind+'Image']='registry.company.example:5000/platform/'+kind+'@sha256:'+'a'.repeat(64);
  const commit=git(['rev-parse','HEAD']);for(const k of ['inputsCommit','modulesCommit','templatesCommit'])raw.parameters[k]=commit;
  const checkouts={inputs:root,modules:root,templates:root};const locked=lockEnterpriseInput(raw,checkouts);
  assert.deepEqual(lockEnterpriseInput(raw,checkouts),locked);
  assert.equal(locked.parameters.moduleSourceId,('git-'+sha(raw.parameters.modulesRepository)).slice(0,60));
  assert.equal(locked.platform.resourceTypes.git.sha256,sha(readFileSync(join(root,raw.platform.resourceTypes.git.path))));
  assert.equal(locked.parameters.namespaceModuleId,('module-'+sha('k8s-namespace\0'+raw.parameters.namespaceModulePath)).slice(0,40));
  assert.match(locked.parameters.templateTreeSHA256,/^[a-f0-9]{64}$/);
  const resolved=expandParameters(locked,locked.parameters);assert.equal(resolved.platform.definitions[1].source.pin.sourceId,locked.parameters.moduleSourceId);
  const privateDir=join(root,'private');mkdirSync(privateDir);
  const credential=join(privateDir,'credential');writeFileSync(credential,'private-test-value',{mode:0o600});
  delete locked.platform.gitAccess.caPemFile;delete locked.platform.secretStore.tlsCaPemFile;
  locked.console.passwordFile=locked.developer.passwordFile=locked.platform.gitAccess.tokenFile=locked.platform.account.kubeconfigFile=locked.platform.secretStore.tokenFile=locked.runtime.clusterKubeconfigFile=locked.application.secretFiles.ACCEPTANCE_SECRET=credential;
  locked.runtime.stateDir=join(privateDir,'state');locked.runtime.reportDir=join(privateDir,'reports');locked.runtime.gitCacheDir=join(privateDir,'git-cache');mkdirSync(locked.runtime.gitCacheDir);
  const cached=join(locked.runtime.gitCacheDir,sha(raw.parameters.inputsRepository+'\0'+commit));git(['clone','--bare',root,cached]);
  locked.runtime.mode='all';
  const configFile=join(privateDir,'locked.json');writeFileSync(configFile,JSON.stringify(locked));
  const config=loadBootstrapConfig(configFile);
  assert.equal(config.gitInputReceipts.length,13);assert.equal(config.platform.definitions.length,9);
  assert.equal(config.platform.definitions[0].definition.key,'company-staging-cluster');
  assert.equal(config.platform.definitions[1].definition.driverInputs.values.source.source_id,locked.parameters.moduleSourceId);
  assert.equal(config.platform.definitions[2].source.url,raw.parameters.modulesRepository);
  for(const kind of ['postgres','redis','mysql','mariadb','mongodb']){const item=config.platform.definitions.find(d=>d.definition.resourceType===kind);assert.equal(item.definition.driverInputs.values.variables.image,raw.parameters[kind+'Image']);}
  for(const kind of ['mysql','mariadb','mongodb']){const item=config.platform.definitions.find(d=>d.definition.resourceType===kind);const vars=item.definition.driverInputs.values.variables;assert.equal(vars.auth_secret_name,kind+'-auth');assert.equal(vars.user_password_key,'user-password');assert.equal(vars.image_pull_secret_name,'');assert.equal(item.definition.driverInputs.values.output_mapping.claimName,'claimName');assert.match(locked.parameters[kind+'ModuleSHA256'],/^[a-f0-9]{64}$/);}
  const types=JSON.parse(readFileSync(join(root,raw.platform.resourceTypes.git.path)));for(const kind of ['mysql','mariadb','mongodb']){const type=types.find(t=>t.key===kind),definition=config.platform.definitions.find(d=>d.definition.resourceType===kind).definition;assert.ok(type);assert.ok(type.outputs.every(o=>Object.hasOwn(definition.driverInputs.values.output_mapping,o.name)));assert.ok(type.inputs.filter(i=>i.required).every(i=>Object.hasOwn(definition.driverInputs.values.input_mapping,i.name)));assert.ok(!type.outputs.some(o=>o.name==='password'));}
  assert.equal(config.application.workloadClass,'company-staging');
  assert.ok(config.gitInputReceipts.every(r=>r.url.startsWith('https://github.company.example/')));
  git(['remote','set-url','origin','https://github.company.example/other/repo.git']);assert.throws(()=>lockEnterpriseInput(raw,checkouts),/origin/);
  assert.throws(()=>lockEnterpriseInput({...raw,parameters:{...raw.parameters,inputsCommit:'main'}},checkouts),/full commits/);
 }finally{rmSync(root,{recursive:true,force:true});}
});
