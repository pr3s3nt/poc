import { outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureInfrastructureAccount} from '../../../scripts/bootstrap/lib/infrastructure-ui.mjs';
import {ensureDelivery,ensureSecretStore} from '../../../scripts/bootstrap/lib/platform.mjs';
process.umask(0o077);
const directory=process.env.ORCH_TEST_RUN_DIR;if(!directory)throw Error('ORCH_TEST_RUN_DIR required');
outsideRepo(directory);
const config=JSON.parse(readFileSync(directory+'/config.json'));const file=directory+'/state.json';const state=existsSync(file)?JSON.parse(readFileSync(file,'utf8')):{};
const rt={state,save(){writeFileSync(file,JSON.stringify(state,null,2))},record(key,status,value){state[key]=value??status;this.save();console.log(key,status)}};
const c=readConsoleConfig(),session=await openConsole(c,{role:'ADMIN'}),ui=makeUI(session.page,c.baseURL,180000),{page}=ui;
try {
const git=(await ui.get('/platform/settings/git')).settings;
if(!git.version){await page.goto(c.baseURL+'/ui/platform/settings');await page.getByLabel('URL Git / phạm vi',{exact:true}).fill(config.gitScope);await page.getByLabel('Username',{exact:true}).fill(config.gitUsername);await page.getByLabel('Token',{exact:true}).fill(readFileSync(config.gitTokenFile,'utf8').trim());await ui.submit('/platform/settings/git','PUT',page.getByRole('button',{name:'Lưu',exact:true}));console.log('Git configured via UI')}
const account=await ensureInfrastructureAccount({...ui,account:{name:'boutique-kind',context:'kind-k8s-4f',endpoint:config.kubernetesEndpoint},kubeconfigFile:directory+'/kubeconfig'});state.account=account;rt.save();
await ensureSecretStore(ui,{name:'Boutique runtime Vault',backendAddress:'http://vault:8200',workloadAddress:config.workloadVaultAddress,mount:'kv',authMount:'kubernetes',tokenFile:directory+'/vault-token'},rt);
await ensureDelivery(ui,{name:'boutique-fleet',provider:'fleet',config:{workspace:'fleet-local',targetCluster:'local'},usePlatformGit:true,setDefault:true},account,rt);
console.log('PASS platform setup through Console UI');
} catch(e){await page.screenshot({path:directory+'/setup-error.png',fullPage:true});console.error('UI setup failed; inspect private screenshot. HTTP status:',e.status??'unavailable');process.exitCode=1}finally{await closeConsole(session,c)}
