import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI,outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureInfrastructureAccount} from '../../../scripts/bootstrap/lib/infrastructure-ui.mjs';
import {ensureDelivery} from '../../../scripts/bootstrap/lib/platform.mjs';
import {openDefinition} from '../../../scripts/bootstrap/lib/direct-platform.mjs';
process.umask(0o077);
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir);outsideRepo(dir);
const config=JSON.parse(readFileSync(dir+'/config.json','utf8')),file=dir+'/state.json',state=existsSync(file)?JSON.parse(readFileSync(file,'utf8')):{};
const rt={state,save(){writeFileSync(file,JSON.stringify(state,null,2))},record(key,status,value){state[key]=value??status;this.save();console.log(key,status)}};
const stage=process.argv[2];assert(['platform','definitions'].includes(stage));
const c=readConsoleConfig(),s=await openConsole(c,{role:'ADMIN'}),ui=makeUI(s.page,c.baseURL,180000),{page}=ui;
try{
 if(stage==='platform'){
  const git=(await ui.get('/platform/settings/git')).settings;
  if(!git.version){await page.goto(c.baseURL+'/ui/platform/settings');await page.getByLabel('URL Git / phạm vi',{exact:true}).fill(config.gitScope);await page.getByLabel('Username',{exact:true}).fill(config.gitUsername);await page.getByLabel('Token',{exact:true}).fill(readFileSync(config.gitTokenFile,'utf8').trim());await ui.submit('/platform/settings/git','PUT',page.getByRole('button',{name:'Lưu',exact:true}));}
  const account=await ensureInfrastructureAccount({...ui,account:{name:'complex-ui-kind',context:config.context,endpoint:config.kubernetesEndpoint},kubeconfigFile:config.kubeconfigFile});state.account=account;rt.save();
  await ensureDelivery(ui,{name:'complex-ui-fleet',provider:'fleet',config:{workspace:'fleet-local',targetCluster:'local'},usePlatformGit:true,setDefault:true},account,rt);
 }else{
  assert(state.account);const definitions=(await ui.get('/resource-definitions')).resourceDefinitions;
  for(const[key,type]of [['complex-cluster','k8s-cluster'],['complex-namespace','k8s-namespace'],['complex-workload','workload']]){
   const existing=definitions.find(d=>d.key===key);if(existing){assert.equal(existing.resourceType,type);continue;}
   await openDefinition(ui,type);
   if(type==='k8s-cluster'){await page.getByRole('combobox',{name:/^Cách cung cấp cluster/}).selectOption('existing-cluster');await page.getByRole('combobox',{name:/^Tài khoản/}).selectOption(state.account.key);}
   else{
    const template=type==='workload';await page.getByRole('textbox',{name:'Git HTTPS URL',exact:true}).fill(template?config.templateRepository:config.moduleRepository);await page.getByLabel(template?'Thư mục template':'Thư mục module',{exact:true}).fill(template?config.templatePath:config.namespacePath);await page.getByLabel('Tag hoặc commit',{exact:true}).filter({visible:true}).fill(template?config.templateRef:config.moduleRef);
    const prepared=await ui.submit(template?'/template-sources/inspect':'/terraform-modules/inspect','POST',page.getByRole('button',{name:template?'Kiểm tra template':'Kiểm tra module',exact:true}));
    if(template){state.source=prepared.templateSource;rt.save();await page.getByLabel('Tham chiếu namespace',{exact:true}).fill("${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}");}
    else{await page.getByRole('button',{name:'Tham số: JSON nâng cao',exact:true}).click();await page.getByLabel('Tham số cấu hình (JSON object)',{exact:true}).fill(JSON.stringify({name:'${context.env.namespace}'}));await page.locator('details.definition-advanced > summary').click();await page.getByRole('button',{name:'Nâng cao: JSON nâng cao',exact:true}).click();await page.getByLabel('Ánh xạ module JSON',{exact:true}).fill(JSON.stringify({input_mapping:{},output_mapping:{name:'name'},provider_bindings:{kubernetes:{target:'resource',resource_ref:'k8s-cluster.default#environments.@app.@env'}}}));}
   }
   await page.getByLabel('ID cấu hình (Definition ID)',{exact:true}).fill(key);await page.getByLabel('Tên hiển thị',{exact:true}).fill(key);await ui.submit('/resource-definitions','POST',page.getByRole('button',{name:'Đăng ký cấu hình tài nguyên',exact:true}).first());console.log('Definition saved via UI',key);
  }
 }
 console.log('PASS UI setup stage',stage);
}catch(error){await page.screenshot({path:dir+`/setup-${stage}-error.png`,fullPage:true});console.error('UI setup failed; inspect private evidence; status',error.status??'unknown');throw error;}finally{await closeConsole(s,c);}
