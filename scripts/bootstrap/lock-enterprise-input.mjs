import {parseArgs} from 'node:util';
import {readFileSync,writeFileSync,realpathSync} from 'node:fs';
import {resolve,dirname,relative} from 'node:path';
import {fileURLToPath} from 'node:url';
import {lockEnterpriseInput} from './lib/enterprise-lock.mjs';
try {
 const {values}=parseArgs({options:{input:{type:'string'},output:{type:'string'},'inputs-checkout':{type:'string'},'modules-checkout':{type:'string'},'templates-checkout':{type:'string'}}});
 if(!values.input||!values.output)throw Error('--input and --output required');
 const root=realpathSync(fileURLToPath(new URL('../../',import.meta.url))),output=resolve(values.output);
 const rel=relative(root,realpathSync(dirname(output)));if(rel===''||(!rel.startsWith('../')&&rel!=='..'))throw Error('Locked input must be outside the repository');
 const raw=JSON.parse(readFileSync(resolve(values.input),'utf8'));
 const locked=lockEnterpriseInput(raw,{inputs:values['inputs-checkout'],modules:values['modules-checkout'],templates:values['templates-checkout']});
 // Keep relative credential/state paths meaningful by writing beside the input.
 if(realpathSync(dirname(resolve(values.input)))!==realpathSync(dirname(output)))throw Error('Input and locked output must share the same private directory');
 writeFileSync(output,JSON.stringify(locked,null,2)+'\n',{mode:0o600,flag:'wx'});
 console.log('LOCKED: exact Git file/tree digests computed offline; no product mutation. Review before running bootstrap.');
}catch{console.error('Lock failed: check private input, exact commits, matching HTTPS origins and regular source files. No network or deployment performed.');process.exitCode=1;}
