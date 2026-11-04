import {parseArgs} from 'node:util';
import {readFileSync,writeFileSync,realpathSync,existsSync} from 'node:fs';
import {resolve,dirname} from 'node:path';
import {prepareEnterpriseInput} from './lib/enterprise-prepare.mjs';
try {
 const {values}=parseArgs({options:{input:{type:'string'},output:{type:'string'},resources:{type:'string'},'inputs-dir':{type:'string'},'modules-dir':{type:'string'},'templates-dir':{type:'string'}}});
 if(!values.input||!values.output||!(values.resources||values['inputs-dir']))throw Error('Input, output and resource directory required');
 const input=resolve(values.input),output=resolve(values.output),outputDir=realpathSync(dirname(output));
 if(realpathSync(dirname(input))!==outputDir||existsSync(output))throw Error('Use a new output beside the private input');
 const prepared=prepareEnterpriseInput(JSON.parse(readFileSync(input,'utf8')),{inputs:values['inputs-dir']??values.resources,modules:values['modules-dir']??values.resources??values['inputs-dir'],templates:values['templates-dir']??values.resources??values['inputs-dir'],outputDir});
 writeFileSync(output,JSON.stringify(prepared,null,2)+'\n',{mode:0o600,flag:'wx'});
 console.log('PREPARED: local snapshot hashes computed; no Git/kubectl or product mutation. Git commit attribution is unverified locally; backend verifies module/template pins through Console.');
}catch{console.error('Preparation failed: check private input, full commit values, source paths and regular files. No network or deployment performed.');process.exitCode=1;}
