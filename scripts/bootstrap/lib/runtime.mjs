import { executionProfile } from './execution-profile.mjs';
import { mkdirSync, readFileSync, writeFileSync, renameSync, openSync, closeSync, unlinkSync, realpathSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { isDeepStrictEqual } from 'node:util';
import { BootstrapError, requireConfig } from './bootstrap-config.mjs';
export const equal = isDeepStrictEqual;
export const digest = v => createHash('sha256').update(v).digest('hex');
export function conflict(ok, label) { if (!ok) throw new BootstrapError(`BLOCKED: ${label}; existing configuration differs`); }
export function outsideRepo(path) {
 mkdirSync(path,{recursive:true,mode:0o700});
 const root=fileURLToPath(new URL('../../../',import.meta.url));
 const rel=relative(realpathSync(root),realpathSync(path));
 requireConfig(rel==='..'||rel.startsWith('../'),'State/report must be outside repository');
}
export function createRuntime(config) {
 const {stateDir,reportDir}=config.runtime;
 outsideRepo(stateDir); outsideRepo(reportDir);
 const lock=join(stateDir,'lock');
 try { const fd=openSync(lock,'wx',0o600); writeFileSync(fd,String(process.pid)); closeSync(fd); }
 catch {
  let stale=false;
  try { const pid=Number(readFileSync(lock,'utf8')); if(Number.isInteger(pid)&&pid>0) { try{process.kill(pid,0);}catch(e){stale=e.code==='ESRCH';} } } catch{}
  if(!stale)throw new BootstrapError('BLOCKED: stateDir is locked; inspect previous process before removing lock');
  unlinkSync(lock);const fd=openSync(lock,'wx',0o600);writeFileSync(fd,String(process.pid));closeSync(fd);
 }
 const file=join(stateDir,'identity.json');
 let state={};
 try { state=JSON.parse(readFileSync(file,'utf8')); } catch(e) { if(e.code!=='ENOENT'){unlinkSync(lock);throw new BootstrapError('Invalid state receipt');} }
 const identity=config.schemaVersion===3?{origin:config.console.baseURL,environment:config.environment,account:config.platform.account.name,context:config.platform.account.context,endpoint:config.platform.account.endpoint}:{origin:config.console.baseURL,environment:config.environment,subdomain:config.application?.subdomain??null};
 if(state.identity && !equal(state.identity,identity)){unlinkSync(lock);throw new BootstrapError('State directory belongs to another target');}
 if(config.schemaVersion===3 && config.application) {
  const acceptanceIdentity={name:config.application.name,subdomain:config.application.subdomain,environment:config.application.environment};
  if(state.acceptanceIdentity && !equal(state.acceptanceIdentity,acceptanceIdentity)){unlinkSync(lock);throw new BootstrapError('Acceptance receipt belongs to another Application/Environment');}
  state.acceptanceIdentity=acceptanceIdentity;
 }
 const profile=executionProfile(config);
 const previousProfile=state.executionProfile??(state.acceptanceSnapshot?.fleet||state.dataUids?'dev-test':undefined);
 if(previousProfile&&previousProfile!==profile){unlinkSync(lock);throw new BootstrapError('State receipt belongs to another execution profile; use separate state/report directories');}
 state.executionProfile=profile;
 state.identity=identity;
 const report={environment:config.environment,profile:executionProfile(config),startedAt:new Date().toISOString(),status:'RUNNING',steps:[]};
 const reportFile=join(reportDir,`${Date.now()}-${process.pid}.json`);
 const save=()=>{const temp=file+'.tmp';writeFileSync(temp,JSON.stringify(state,null,2),{mode:0o600});renameSync(temp,file);};
 const record=(step,status,details={})=>{const entry={step,status,...details};report.steps.push(entry);writeFileSync(reportFile,JSON.stringify(report,null,2),{mode:0o600});console.log(`${status}: ${step}`);};
 return {state,save,record,report,reportFile,close(status){report.status=status;report.finishedAt=new Date().toISOString();writeFileSync(reportFile,JSON.stringify(report,null,2),{mode:0o600});unlinkSync(lock);}};
}
export function makeUI(page,baseURL,timeoutMs) {
 const get=async path=>{const r=await page.request.get(baseURL+'/api/v1'+path);if(!r.ok())throw new BootstrapError(`GET ${path.split('?')[0]}: HTTP ${r.status()}`);return r.json();};
 const act=async(locator,method,...args)=>locator[method](...args);
 const responseFor=(path,method)=>page.waitForResponse(r=>r.request().method()===method && new URL(r.url()).pathname==='/api/v1'+path,{timeout:/\/(deploy|refresh|verify|recover)$/.test(path)?timeoutMs:Math.min(timeoutMs,180000)});
 const submit=async(path,method,button)=>{const pending=responseFor(path,method);const [r]=await Promise.all([pending,button.click()]);if(!r.ok()){const error=new BootstrapError(`Mutation ${method} ${path}: HTTP ${r.status()}; inspect server before retry`);error.status=r.status();throw error;}return r.json();};
 return {page,baseURL,get,act,responseFor,submit};
}
export async function waitIdle(ui,appKey,env,timeout) {
 const deadline=Date.now()+timeout;
 for (;;) {
  await ui.get(`/applications/${appKey}/environments/${env}/operations`);
  const app=(await ui.get(`/applications/${appKey}`)).application;
  const target=app.environments.find(v=>v.key===env);
  requireConfig(target,'Missing Environment');
  if(!target.activeOperation)return target;
  if(target.activeOperation.status==='INTERRUPTED')throw new BootstrapError('BLOCKED: operation requires explicit recovery');
  if(Date.now()>deadline)throw new BootstrapError('BLOCKED: operation still active; resume later');
  await ui.page.waitForTimeout(2000);
 }
}
