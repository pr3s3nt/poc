import { chmodSync, renameSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { randomUUID } from 'node:crypto';
import { outsideRepo } from './runtime.mjs';

// Only public workload pages are recorded. No authentication or secret-entry
// Console context, cookies or browser storage state is persisted.
export async function openWorkloadEvidence(browser,config,label) {
 if(!/^[a-z0-9-]+$/.test(label))throw new Error('Invalid video label');
 const root=join(config.runtime.reportDir,'videos');
 const directory=join(root,`.recording-${randomUUID()}`);
 if(config.runtime.recordVideo){outsideRepo(directory);chmodSync(root,0o700);chmodSync(directory,0o700);}
 let context,page;
 try {
  context=await browser.newContext({viewport:{width:1440,height:1000},
   ...(config.runtime.recordVideo?{recordVideo:{dir:directory,size:{width:1440,height:1000}}}:{})});
  page=await context.newPage();
 }catch(error){try{if(context)await context.close();}finally{if(config.runtime.recordVideo)rmSync(directory,{recursive:true,force:true});}throw error;}
 const video=page.video(),startedAt=new Date().toISOString();let closed=false;
 return {page,async close(status='FAILED') {
  if(closed)return;closed=true;
  try {
   await context.close();
   if(video&&status==='SUCCEEDED'){
    const file=join(root,`${label}.webm`);
    // Publish only a completely successful run; overwrite the previous success.
    renameSync(await video.path(),file);chmodSync(file,0o600);
    writeFileSync(join(root,`${label}.json`),JSON.stringify({label,status,startedAt,finishedAt:new Date().toISOString(),file},null,2),{mode:0o600});
    console.log(`VIDEO: ${file}`);
   }
  }finally{if(config.runtime.recordVideo)rmSync(directory,{recursive:true,force:true});}
 }};
}
