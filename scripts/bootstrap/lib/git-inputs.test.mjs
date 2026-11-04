import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdtempSync, mkdirSync, writeFileSync, chmodSync, symlinkSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createGitInputs, validateGitInput } from './git-inputs.mjs';
const hash=v=>createHash('sha256').update(v).digest('hex');
function fixture() {
 const dir=mkdtempSync(join(tmpdir(),'git-input-test-')),work=join(dir,'work'),cache=join(dir,'cache');mkdirSync(work);mkdirSync(cache);
 const git=args=>execFileSync('git',['-C',work,...args],{encoding:'utf8',stdio:['ignore','pipe','pipe']});
 git(['init','.']);writeFileSync(join(work,'definition.json'),'{"key":"test"}\n');writeFileSync(join(work,'executable'),'not allowed');chmodSync(join(work,'executable'),0o755);symlinkSync('definition.json',join(work,'link'));
 git(['add','.']);git(['-c','user.name=Test','-c','user.email=test@example.invalid','commit','-m','fixture']);const ref=git(['rev-parse','HEAD']).trim(),url='https://example.invalid/repo.git';
 execFileSync('git',['clone','--bare',work,join(cache,hash(url+'\0'+ref))],{stdio:'ignore'});
 return {dir,cache,input:{url,ref,path:'definition.json',sha256:hash('{"key":"test"}\n')},close:()=>rmSync(dir,{recursive:true,force:true})};
}
test('Git input pins file bytes and commit; unchanged cache read remains identical',()=>{
 const f=fixture();try{const reader=createGitInputs(f.cache);const first=reader.read(f.input);assert.equal(reader.read(f.input),first);assert.equal(reader.receipts.length,2);assert.equal(reader.receipts[0].ref,f.input.ref);assert.throws(()=>reader.read({...f.input,sha256:'0'.repeat(64)}),/SHA256 mismatch/);}finally{f.close();}
});
test('Git inputs reject mutable refs, credential URLs, traversal and unsafe file modes',()=>{
 const f=fixture();try{for(const change of [{ref:'main'},{url:'https://secret@example.invalid/repo.git'},{path:'../definition.json'},{path:'/definition.json'},{path:'a\\b'},{path:'-bad'}])assert.throws(()=>validateGitInput({...f.input,...change}));const reader=createGitInputs(f.cache);for(const path of ['link','executable'])assert.throws(()=>reader.read({...f.input,path}),/regular nonexecutable/);}finally{f.close();}
});
test('Git file cache drift is blocked instead of repairing or reading the worktree',()=>{
 const f=fixture();try{const reader=createGitInputs(f.cache),path=reader.read(f.input);writeFileSync(path,'drift');assert.throws(()=>reader.read(f.input),/materialized input drift/);}finally{f.close();}
});

test('Git-only rejects inline Resource Types before any product mutation',async()=>{
 const {loadBootstrapConfig}=await import('./bootstrap-config.mjs');
 const f=fixture();try {
  const raw=JSON.parse((await import('node:fs')).readFileSync(new URL('../direct-targets/dev.json',import.meta.url)));
  const privateFile=join(f.dir,'private');writeFileSync(privateFile,'fixture',{mode:0o600});
  raw.gitOnly=true;raw.runtime.mode='bootstrap';raw.runtime.stateDir=join(f.dir,'state');raw.runtime.reportDir=join(f.dir,'reports');raw.runtime.clusterKubeconfigFile=privateFile;
  raw.console.passwordFile=raw.developer.passwordFile=raw.platform.account.kubeconfigFile=privateFile;
  const file=join(f.dir,'config.json');writeFileSync(file,JSON.stringify(raw));
  assert.throws(()=>loadBootstrapConfig(file),/Resource Types must come from Git/);
 }finally{f.close();}
});
