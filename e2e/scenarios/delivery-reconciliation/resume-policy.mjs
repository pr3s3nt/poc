// Remediation of an observed runtime defect through the existing recovery UI.
// No product mutation API or manual workload metadata patch substitutes for it.
import{expect}from'@playwright/test';
import{readFileSync,writeFileSync}from'node:fs';
import{join,resolve}from'node:path';
import{execFileSync}from'node:child_process';
import{openConsole,closeConsole,readConsoleConfig,readPrivateFile}from'../../lib/console.mjs';
const run=resolve(process.env.ORCH_RECON_RUN_DIR??'');if(!process.env.ORCH_RECON_RUN_DIR||run.includes('/projects/thieu/poc/'))throw Error('Private run directory required');
const statePath=resolve(run,process.env.ORCH_RECON_STATE_FILE??'ui-state-current.json');if(!statePath.startsWith(run+'/'))throw Error('State path escaped run');const state=JSON.parse(readFileSync(statePath));expect(state.appID).toMatch(/^[a-f0-9-]{36}$/);
const project=readPrivateFile(join(run,'current-compose-project'));expect(project).toBe('orch-reconciliation-current-20261029');
const prior=readPrivateFile(join(run,'policy-failed-worker-container-id'));expect(prior).toMatch(/^[a-f0-9]{64}$/);
let oldRunning=false;try{oldRunning=execFileSync('docker',['inspect','--format','{{.State.Running}}',prior],{encoding:'utf8',stdio:['ignore','pipe','pipe']}).trim()==='true';}catch{/* Original container removed after verified recreation. */}
if(oldRunning)throw Error('Prior executor is still running; recovery confirmation is refused');
const current=execFileSync('docker',['inspect','--format','{{.Id}}',`${project}-backend-1`],{encoding:'utf8'}).trim();expect(current).not.toBe(prior);
const kube=(...args)=>execFileSync('kubectl',['--kubeconfig',join(run,'kubeconfig'),...args],{encoding:'utf8',stdio:['ignore','pipe','pipe']});
expect(kube('get','namespace','kube-system','-o','jsonpath={.metadata.uid}')).toBe(readPrivateFile(join(run,'kube-system-uid')));
const before=()=>JSON.parse(kube('get','deployments,statefulsets,persistentvolumeclaims,services','-n',state.namespace,'-o','json')).items.map(o=>({key:`${o.kind}/${o.metadata.name}`,uid:o.metadata.uid,template:o.spec.template})).sort((a,b)=>a.key.localeCompare(b.key));
const inventory=before();
const offDriftCount=Number(execFileSync('docker',['exec',`${project}-postgres-1`,'psql','-U','orchestrator','-d','orchestrator','-Atc',`SELECT count(*) FROM delivery_observations o JOIN delivery_revisions r ON r.id=o.revision_id JOIN delivery_units u ON u.id=o.unit_id JOIN environments e ON e.id=u.environment_id JOIN applications a ON a.id=e.application_id WHERE a.application_key='${state.appID}' AND o.observation->>'drift'='DETECTED' AND o.observation->>'health'='HEALTHY' AND (r.selection->>'selfHeal')::boolean=false;`],{encoding:'utf8',stdio:['ignore','pipe','pipe']}).trim());expect(offDriftCount).toBeGreaterThan(0);
const config=readConsoleConfig(),session=await openConsole(config,{role:'PLATFORM_ENGINEER'}),{page}=session;
try{
 await page.goto(`${config.baseURL}/ui/applications/${state.appID}/settings?environment=staging`);
 const banner=page.getByRole('region',{name:'Thao tác trên môi trường',exact:true});
 await expect(banner).toBeVisible();await expect(banner).toContainText('DELIVERY_POLICY');
 await banner.getByRole('checkbox',{name:'Tôi xác nhận thao tác bị gián đoạn đã dừng hẳn',exact:true}).check({timeout:90_000});
 const response=page.waitForResponse(r=>r.request().method()==='POST'&&/\/operations\/[^/]+\/recover$/.test(new URL(r.url()).pathname),{timeout:30*60_000});
 await banner.getByRole('button',{name:'Khôi phục môi trường',exact:true}).click();const reply=await response;expect(reply.status()).toBe(200);
 await expect(page.getByText('Khôi phục đã hoàn tất.',{exact:false})).toBeVisible({timeout:30_000});
 expect(before()).toEqual(inventory);
 const live=()=>JSON.parse(kube('get','deployment','backend','-n',state.namespace,'-o','json'));const original=live();
 await expect.poll(()=>live().spec.replicas,{timeout:120_000,intervals:[2000]}).toBe(1);
 kube('patch','deployment','backend','-n',state.namespace,'--type=json','--patch',JSON.stringify([{op:'test',path:'/metadata/uid',value:original.metadata.uid},{op:'replace',path:'/spec/replicas',value:2}]));
 await expect.poll(()=>live().spec.replicas,{timeout:180_000,intervals:[2000]}).toBe(1);expect(live().metadata.uid).toBe(original.metadata.uid);
 const get=await page.request.get(`${config.baseURL}/api/v1/applications/${state.appID}/environments/staging/delivery`);expect(get.ok()).toBe(true);const selection=await get.json();expect(selection.desired.selfHeal).toBe(true);expect(selection.applied.selfHeal).toBe(true);expect(selection.pending).toBe(false);
 await page.goto(`${config.baseURL}/ui/applications/${state.appID}?environment=staging`);const panel=page.getByRole('region',{name:'Đối soát workload',exact:true});await expect(panel.getByRole('cell',{name:'Không lệch',exact:true})).toHaveCount(5,{timeout:150_000});
 state.selfHealPassed=true;state.policyRecoveryPassed=true;state.offDriftObservationCount=offDriftCount;writeFileSync(statePath,JSON.stringify(state,null,2),{mode:0o600});
 await page.screenshot({path:join(run,'ui-evidence','policy-recovery-self-heal-pass.png'),fullPage:true});console.log('PASS explicit UI recovery after proven worker stop; no UID/Pod-template changes; off-policy drift retained; opt-in Fleet healing restores replicas');
}catch(error){await page.screenshot({path:join(run,'ui-evidence',`policy-recovery-failure-${Date.now()}.png`),fullPage:true}).catch(()=>{});console.error(error.message.split('\n')[0]);process.exitCode=1;}finally{await closeConsole(session,config)}
