// Recover the observed existing-GitRepo policy acknowledgement race through UI.
import { expect } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { execFileSync } from 'node:child_process';
import { openConsole, closeConsole, readConsoleConfig, readPrivateFile } from '../../lib/console.mjs';

const run = resolve(process.env.ORCH_RECON_RUN_DIR ?? '');
if (!process.env.ORCH_RECON_RUN_DIR || run.includes('/projects/thieu/poc/')) throw Error('Private run directory required');
const statePath = resolve(run, process.env.ORCH_RECON_STATE_FILE ?? 'ui-state-current.json');
if (!statePath.startsWith(run + '/')) throw Error('State escaped private run directory');
const state = JSON.parse(readFileSync(statePath));
const reverse = process.env.ORCH_RECON_RECOVERY_DIRECTION === 'fleet';
const semantic = process.env.ORCH_RECON_FOCUS === 'semantics';
const removal = process.env.ORCH_RECON_RECOVERY_KIND === 'removal';
const project = readPrivateFile(join(run, semantic ? 'drift-compose-project' : 'current-compose-project'));
expect(project).toBe(semantic ? 'orch-reconciliation-drift-20261029' : 'orch-reconciliation-current-20261029');
const prior = readPrivateFile(join(run, semantic ? (reverse ? 'semantic-reverse-failed-worker-container-id' : 'semantic-failed-worker-container-id') : removal ? 'removal-failed-worker-container-id' : 'transfer-failed-worker-container-id'));
expect(prior).toMatch(/^[a-f0-9]{64}$/);
let running = false;
try { running = execFileSync('docker', ['inspect', '--format', '{{.State.Running}}', prior], { encoding: 'utf8', stdio: ['ignore','pipe','pipe'] }).trim() === 'true'; } catch { /* Recreated owned worker is absent. */ }
if (running) throw Error('Prior executor is still running');
expect(execFileSync('docker', ['inspect', '--format', '{{.Id}}', `${project}-backend-1`], { encoding:'utf8' }).trim()).not.toBe(prior);
const cluster = readPrivateFile(join(run, 'cluster-name'));
expect(cluster).toMatch(/^orch-reconciliation-/);
expect(execFileSync('docker', ['inspect', '--format', '{{.Id}}', `${cluster}-control-plane`], { encoding:'utf8' }).trim()).toBe(readPrivateFile(join(run, 'container-id')));
const kube = (...args) => execFileSync('kubectl', ['--kubeconfig',join(run,'kubeconfig'),...args], {encoding:'utf8',stdio:['ignore','pipe','pipe']});
expect(kube('get','namespace','kube-system','-o','jsonpath={.metadata.uid}')).toBe(readPrivateFile(join(run,'kube-system-uid')));
const snapshot = () => JSON.parse(kube('get',semantic ? 'deployments,statefulsets,persistentvolumeclaims,services,jobs,cronjobs,hpa,ingresses' : 'deployments,statefulsets,persistentvolumeclaims,services','-n',state.namespace,'-o','json')).items.filter(o=>!o.metadata.ownerReferences?.some(ref=>ref.kind==='CronJob')).map(o=>({key:`${o.kind}/${o.metadata.name}`,uid:o.metadata.uid,template:o.spec.template})).sort((a,b)=>a.key.localeCompare(b.key));
expect(kube('get','namespace',state.namespace,'-o','jsonpath={.metadata.uid}')).toBe(state.namespaceUID);
if (!removal) expect(snapshot()).toEqual((semantic ? state.transferBaselines[reverse ? "fleetTransferPassed" : "directTransferPassed"] : state.existingTransferBaseline));
const before = snapshot();
const config = readConsoleConfig();
const session = await openConsole(config, {role:'PLATFORM_ENGINEER'});
const {page} = session;
try {
  await page.goto(`${config.baseURL}/ui/applications/${state.appID}/settings?environment=staging`);
  const banner = page.getByRole('region',{name:'Thao tác trên môi trường',exact:true});
  await expect(banner).toContainText(semantic ? 'ADOPTING' : removal ? 'DEPLOY backend' : 'VERIFYING');
  await page.screenshot({path:join(run,'ui-evidence',removal?'removal-held-recovery.png':'transfer-held-recovery.png'),fullPage:true});
  await banner.getByRole('checkbox',{name:'Tôi xác nhận thao tác bị gián đoạn đã dừng hẳn',exact:true}).check({timeout:90_000});
  const response = page.waitForResponse(r=>r.request().method()==='POST'&&/\/operations\/[^/]+\/recover$/.test(new URL(r.url()).pathname),{timeout:30*60_000});
  await banner.getByRole('button',{name:'Khôi phục môi trường',exact:true}).click();
  expect((await response).status()).toBe(200);
  await expect(page.getByText('Khôi phục đã hoàn tất.',{exact:false})).toBeVisible();
  if (!removal) expect(snapshot()).toEqual((semantic ? state.transferBaselines[reverse ? "fleetTransferPassed" : "directTransferPassed"] : state.existingTransferBaseline));
  else {
    const retained = before.filter(o=>!['Deployment/backend','Service/backend'].includes(o.key));
    expect(snapshot()).toEqual(retained);
  }
  const parent = JSON.parse(kube('get','gitrepo','orch-reconciliation-existing','-n','fleet-local','-o','json'));
  expect(parent.metadata.uid).toBe(state.existingGitRepoUID);
  expect(parent.spec).toEqual(state.existingGitRepoSpec);
  const sibling = JSON.parse(kube('get','configmap','orch-reconciliation-sibling','-n','orch-recon-sibling','-o','json'));
  expect(sibling.metadata.uid).toBe(state.siblingUID);
  expect(sibling.data.message).toBe('retained company sibling');
  const deliveryResponse = await page.request.get(`${config.baseURL}/api/v1/applications/${state.appID}/environments/staging/delivery`);
  expect(deliveryResponse.ok()).toBe(true);
  const delivery = await deliveryResponse.json();
  if(semantic){expect(delivery.applied.mode).toBe(reverse?'CD':'DIRECT');expect(delivery.applied.selfHeal).toBe(false);state[reverse?'fleetTransferPassed':'directTransferPassed']=true;}
  else {expect(delivery.applied.bindingId).toBe(state.existingBindingID);expect(delivery.applied.selfHeal).toBe(true);}
  expect(delivery.pending).toBe(false);
  if (semantic) state.semanticTransferRecoveryPassed=true;
  else if (removal) state.removalRecoveryPassed = true;
  else { state.existingTransferPassed = true; state.existingTransferRecoveryPassed = true; }
  writeFileSync(statePath,JSON.stringify(state,null,2),{mode:0o600});
  await page.screenshot({path:join(run,'ui-evidence',removal?'removal-recovery-pass.png':'existing-transfer-recovery-pass.png'),fullPage:true});
  console.log(removal ? 'PASS explicit UI removal recovery; original UID-guarded backend withdrawal finished; retained resources, parent and sibling preserved' : 'PASS explicit UI transfer recovery; exact receipt acknowledged without repeated adoption; UIDs, Pod templates, parent and sibling preserved');
} catch(error) {
  await page.screenshot({path:join(run,'ui-evidence',`transfer-recovery-failure-${Date.now()}.png`),fullPage:true}).catch(()=>{});
  console.error(error.message.split('\n')[0]); process.exitCode=1;
} finally { await closeConsole(session,config); }
