import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readdirSync, readFileSync, statSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { chromium } from '@playwright/test';
import { openWorkloadEvidence } from './video.mjs';

test('video publishes only latest success and discards failed recording',async()=>{
 const root=mkdtempSync(join(tmpdir(),'bootstrap-video-'));
 const browser=await chromium.launch({headless:true});
 try {
  const config={runtime:{recordVideo:true,reportDir:root}};
  const record=async status=>{
   const evidence=await openWorkloadEvidence(browser,config,'pvc');
   try{await evidence.page.setContent('<h1>Public test page</h1>');await evidence.page.waitForTimeout(350);}
   finally{await evidence.close(status);}
  };
  await record('FAILED');assert.deepEqual(readdirSync(join(root,'videos')),[]);
  await record('SUCCEEDED');
  const file=join(root,'videos','pvc.webm');assert.ok(statSync(file).size>100);
  const first=readFileSync(file);
  await record('FAILED');assert.deepEqual(readFileSync(file),first);
  await record('SUCCEEDED');
  assert.deepEqual(readdirSync(join(root,'videos')).sort(),['pvc.json','pvc.webm']);
  assert.equal(JSON.parse(readFileSync(join(root,'videos','pvc.json'))).status,'SUCCEEDED');
  assert.equal(statSync(file).mode&0o077,0);
 }finally{await browser.close();rmSync(root,{recursive:true,force:true});}
});

test('disabled video creates no artifacts',async()=>{
 const root=mkdtempSync(join(tmpdir(),'bootstrap-no-video-'));
 const browser=await chromium.launch({headless:true});
 try {
  const evidence=await openWorkloadEvidence(browser,{runtime:{recordVideo:false,reportDir:root}},'pvc');
  assert.equal(evidence.page.video(),null);await evidence.close('SUCCEEDED');
  assert.deepEqual(readdirSync(root),[]);
 }finally{await browser.close();rmSync(root,{recursive:true,force:true});}
});
