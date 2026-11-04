import {mkdtempSync,writeFileSync,readFileSync,existsSync,rmSync} from 'node:fs';
import {join,basename} from 'node:path';
import {createHash} from 'node:crypto';
import {lockEnterpriseInput} from './enterprise-lock.mjs';
import {expandParameters} from './input-parameters.mjs';
import {outsideRepo} from './runtime.mjs';
import {requireConfig} from './bootstrap-config.mjs';
const sha=value=>createHash('sha256').update(value).digest('hex');
// Preparation consumes an already supplied asset snapshot, without Git or Kubernetes CLIs.
export function prepareEnterpriseInput(raw,{inputs,modules=inputs,templates=inputs,outputDir}) {
 requireConfig(raw.schemaVersion===3&&['check','bootstrap','all'].includes(raw.runtime?.mode),'Preparation requires a schema-3 enterprise input');
 requireConfig(!raw.platform?.delivery?.namespaceAccess?.enabled&&!raw.verification?.forward,'Remove dev namespaceAccess/forward flags before preparing Console input');
 outsideRepo(outputDir);
 const prepared=lockEnterpriseInput(raw,{inputs,modules,templates,localSnapshot:true});
 prepared.runtime.profile='console';delete prepared.runtime.clusterKubeconfigFile;delete prepared.runtime.gitCacheDir;
 const directory=mkdtempSync(join(outputDir,'prepared-inputs-'));
 try {
  const visit=value=>{
   if(!value||typeof value!=='object')return;
   if(value.git){
    const spec=expandParameters(value.git,prepared.parameters);
    const bytes=readFileSync(join(inputs,spec.path));
    requireConfig(sha(bytes)===spec.sha256,'Snapshot document changed during preparation');
    const name=sha(spec.url+'\0'+spec.ref+'\0'+spec.path)+'-'+spec.sha256+'.input';
    const target=join(directory,name);if(!existsSync(target))writeFileSync(target,bytes,{mode:0o600,flag:'wx'});
    value.git.preparedFile='./'+basename(directory)+'/'+name;
   }
   for(const child of Object.values(value))visit(child);
  };
  visit(prepared);return prepared;
 }catch(error){rmSync(directory,{recursive:true,force:true});throw error;}
}
