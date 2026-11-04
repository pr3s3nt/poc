import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync, mkdirSync, mkdtempSync, existsSync, renameSync, rmSync, realpathSync } from 'node:fs';
import { join, resolve, relative } from 'node:path';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { gitURL, inGitScope, readGitToken } from './git-access-config.mjs';
import { BootstrapError, requireConfig, onlyKeys } from './bootstrap-config.mjs';
const sha = value => createHash('sha256').update(value).digest('hex');
const repositoryRoot = fileURLToPath(new URL('../../../', import.meta.url));
export function validateGitInput(input) {
 onlyKeys(input,['url','ref','path','sha256','preparedFile'],'Git input');
 requireConfig(gitURL(input.url)===input.url && /^[a-f0-9]{40}$/.test(input.ref??''),'Git input requires normalized HTTPS URL and full commit');
 requireConfig(typeof input.path==='string'&&input.path.length<512&&input.path.split('/').every(p=>p&&p!=='.'&&p!=='..')&&!/[\\\x00-\x1f\x7f:]/.test(input.path)&&!input.path.startsWith('-'),'Git input path must be a clean relative file path');
 requireConfig(/^[a-f0-9]{64}$/.test(input.sha256??''),'Git input file SHA256 required');
}
export function createGitInputs(cacheDir,access) {
 mkdirSync(cacheDir,{recursive:true,mode:0o700});
 const rel=relative(realpathSync(repositoryRoot),realpathSync(cacheDir));
 requireConfig(rel.startsWith('../'),'Git input cache must be outside repository');
 const receipts=[];
 const environment=Object.fromEntries(Object.entries(process.env).filter(([key])=>!key.startsWith('GIT_')&&!['DEBUG','NODE_DEBUG','PWDEBUG'].includes(key)));
 Object.assign(environment,{GIT_CONFIG_NOSYSTEM:'1',GIT_CONFIG_GLOBAL:'/dev/null',GIT_TERMINAL_PROMPT:'0'});
 const options={encoding:'utf8',timeout:180000,maxBuffer:2*1024*1024,stdio:['ignore','pipe','pipe'],env:environment};
 const git=(dir,args,extra={})=>execFileSync('git',['-c','credential.helper=','-c','core.hooksPath=/dev/null','-c','protocol.file.allow=never','-c','http.extraHeader=','-C',dir,...args],{...options,...extra});
 return {receipts,read(input) {
  validateGitInput(input);
  const identity=sha(input.url+'\0'+input.ref),repository=join(cacheDir,identity);
  if(!existsSync(repository)) {
   const temporary=mkdtempSync(join(cacheDir,'.prepare-'));
   try {
    git(temporary,['init','--bare','.']);
    let credentials={};
    if(access?.enabled!==false&&access?.tokenFile) {
     requireConfig(inGitScope(access.url,input.url),'Git input is outside configured credential scope');
     requireConfig(typeof access.username==='string'&&access.username.length>0&&!/[\x00-\x1f\x7f]/.test(access.username),'Git credential username invalid');
     readGitToken(access.tokenFile);
     const helper=join(temporary,'askpass');
     writeFileSync(helper,'#!/bin/sh\ncase "$1" in *Username*) printf "%s" "$ORCH_INPUT_GIT_USERNAME" ;; *) cat "$ORCH_INPUT_GIT_TOKEN_FILE" ;; esac\n',{mode:0o700,flag:'wx'});
     credentials={...(access.caPemFile?{GIT_SSL_CAINFO:access.caPemFile}:{}),GIT_ASKPASS:helper,ORCH_INPUT_GIT_USERNAME:access.username,ORCH_INPUT_GIT_TOKEN_FILE:access.tokenFile};
    }
    try {git(temporary,['fetch','--depth=1','--no-tags','--no-recurse-submodules',input.url,input.ref],{env:{...environment,...credentials}});}
    finally {rmSync(join(temporary,'askpass'),{force:true});}
    requireConfig(git(temporary,['rev-parse','FETCH_HEAD']).trim()===input.ref,'Git input commit mismatch');
    if(existsSync(repository))rmSync(temporary,{recursive:true,force:true});else renameSync(temporary,repository);
   }catch(error){rmSync(temporary,{recursive:true,force:true});if(error instanceof BootstrapError)throw error;throw new BootstrapError('Git input fetch failed; check repository, commit and private credential scope');}
  }
  let raw;
  try {
   const tree=git(repository,['ls-tree',input.ref,'--',input.path]).trim();
   requireConfig(/^100644 blob [a-f0-9]{40}\t/.test(tree)&&tree.split('\t')[1]===input.path,'Git input must be one regular nonexecutable file');
   raw=git(repository,['show',`${input.ref}:${input.path}`],{encoding:null,maxBuffer:1024*1024});
  }catch(error){if(error instanceof BootstrapError)throw error;throw new BootstrapError('Git input missing/oversized or cache invalid; no fallback');}
  requireConfig(sha(raw)===input.sha256,'Git input content SHA256 mismatch');
  const output=join(cacheDir,identity+'-'+input.sha256+'.input');
  if(existsSync(output))requireConfig(sha(readFileSync(output))===input.sha256,'Git materialized input drift');
  else writeFileSync(output,raw,{mode:0o600,flag:'wx'});
  receipts.push({...input});return resolve(output);
 }};
}
