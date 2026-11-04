import {execFileSync} from 'node:child_process';
import {readFileSync,lstatSync,readdirSync} from 'node:fs';
import {resolve,relative,join} from 'node:path';
import {createHash} from 'node:crypto';
import {expandParameters,validateParameters} from './input-parameters.mjs';
import {treeDigest} from './direct-config.mjs';
import {gitURL} from './git-access-config.mjs';
const sha=value=>createHash('sha256').update(value).digest('hex');
const requireValue=(ok,message)=>{if(!ok)throw Error(message);};
const clean=path=>typeof path==='string'&&path.length>0&&!path.startsWith('/')&&path.split('/').every(p=>p&&p!=='.'&&p!=='..')&&!/[\\\x00-\x1f:]/.test(path);
export function lockEnterpriseInput(raw,{inputs,modules,templates,localSnapshot=false}) {
 raw=structuredClone(raw);const params=validateParameters(raw.parameters);
 const resolved=()=>expandParameters(raw,params);
 const git=(dir,args,options={})=>execFileSync('git',['-c','core.hooksPath=/dev/null','-C',dir,...args],{encoding:'utf8',timeout:30000,maxBuffer:8*1024*1024,stdio:['ignore','pipe','pipe'],...options});
 const verify=(dir,url,commit)=>{
  requireValue(typeof dir==='string'&&dir.length>0,'Source checkout required');
  requireValue(/^[a-f0-9]{40}$/.test(commit),'Supply exact full commits before locking');
  if(localSnapshot){requireValue(gitURL(url)===url,'Normalized HTTPS source URL required');return;}
  const actual=git(dir,['remote','get-url','origin']).trim();
  requireValue(gitURL(actual).replace(/\.git$/,'')===gitURL(url).replace(/\.git$/,''),'Checkout HTTPS origin does not match input');
  requireValue(git(dir,['rev-parse',commit+'^{commit}']).trim()===commit,'Commit missing from local checkout');
 };
 verify(inputs,params.inputsRepository,params.inputsCommit);verify(modules,params.modulesRepository,params.modulesCommit);verify(templates,params.templatesRepository,params.templatesCommit);
 const files=(dir,commit,path)=>{
  requireValue(clean(path),'Clean relative source path required');
  if(localSnapshot)return snapshotFiles(dir,path);
  const entries=git(dir,['ls-tree','-r','-z',commit,'--',path]).split('\0').filter(Boolean);
  requireValue(entries.length>0,'Git source path is empty/missing');
  const result={};
  for(const entry of entries){const [meta,name]=entry.split('\t');requireValue(/^100644 blob [a-f0-9]{40}$/.test(meta),'Only regular nonexecutable source files supported by locker');const rel=name===path?name:name.slice(path.length+1);requireValue(name===path||name.startsWith(path+'/'),'Source tree mismatch');result[rel]=git(dir,['cat-file','blob',meta.split(' ')[2]],{encoding:null});}
  return result;
 };
 const blob=(commit,path)=>{const found=files(inputs,commit,path);requireValue(Object.keys(found).length===1&&found[path],'Expected one exact Git input file');return found[path];};
 const set=(expression,value)=>{const key=/^\$\{bootstrap\.([A-Za-z][A-Za-z0-9_]*)\}$/.exec(expression)?.[1];requireValue(key,'Enterprise pin fields must reference explicit bootstrap parameters');params[key]=value;};
 const release=JSON.parse(readFileSync(new URL('../../../backend/internal/adapters/terraform/registry/release.json',import.meta.url),'utf8'));
 for(const entry of raw.platform.definitions){
  if(!entry.source)continue;
  const spec=expandParameters(entry,params),definition=JSON.parse(blob(spec.git.ref,spec.git.path));
  const digest=treeDigest(files(modules,spec.source.ref,spec.source.path));
  const packaged=release.packaged.modules.find(m=>m.contentSha256===digest&&m.resourceTypes.includes(definition.resourceType));
  const moduleId=packaged?.id??('module-'+sha(definition.resourceType+'\0'+spec.source.path)).slice(0,40);
  set(entry.source.pin.sourceId,('git-'+sha(spec.source.url)).slice(0,60));set(entry.source.pin.moduleId,moduleId);set(entry.source.pin.contentSha256,digest);
 }
 for(const entry of raw.platform.templateSources){const spec=expandParameters(entry,params);set(entry.pin.contentSha256,treeDigest(files(templates,spec.ref,spec.path)));}
 const visit=value=>{if(!value||typeof value!=='object')return;if(value.git){const spec=expandParameters(value.git,params);value.git.sha256=sha(blob(spec.ref,spec.path));}for(const v of Object.values(value))visit(v);};
 visit(raw);resolved();return raw;
}

// Local snapshots verify bytes only; they do not prove Git origin or commit membership.
function snapshotFiles(dir,path) {
 const root=resolve(dir),parts=path.split('/');let current=root;
 requireValue(lstatSync(root).isDirectory()&&!lstatSync(root).isSymbolicLink(),'Snapshot root must be a regular directory');
 for(const part of parts){current=join(current,part);requireValue(!lstatSync(current).isSymbolicLink(),'Snapshot paths may not follow symlinks');}
 const result={};let total=0;
 const visit=(file,name)=>{
  const stat=lstatSync(file);requireValue(!stat.isSymbolicLink(),'Snapshot symlink refused');
  if(stat.isDirectory()) {for(const child of readdirSync(file).sort()){requireValue(!['.git','.terraform','node_modules'].includes(child),'Remove runtime caches from snapshot sources');visit(join(file,child),name?name+'/'+child:child);}return;}
  requireValue(stat.isFile()&&!(stat.mode&0o111)&&stat.size<=1024*1024,'Only bounded regular nonexecutable snapshot files supported');
  total+=stat.size;requireValue(total<=8*1024*1024&&Object.keys(result).length<1000,'Snapshot exceeds source limits');result[name]=readFileSync(file);
 };
 const selected=resolve(root,path),rel=relative(root,selected);requireValue(rel&&!rel.startsWith('..'),'Snapshot source outside root');
 visit(selected,lstatSync(selected).isFile()?path:'');requireValue(Object.keys(result).length>0,'Snapshot source empty');return result;
}
