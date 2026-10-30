import {expect} from '@playwright/test';
import {existingGitRepoFixture} from './platform.mjs';

export async function runExisting({page,baseURL,run,state,saveState,step,business,ca,writer,accountName}) {
 const {kubectl,home,deploy}=business;
 await step('Prepare preexisting watched GitRepo and retained sibling',()=>existingGitRepoFixture({kubectl,state,saveState,run,ca}));
 await step('Discover and verify existing GitRepo through CD configuration form',async()=>{
  if(state.existingBindingID) return;
  await page.goto(`${baseURL}/ui/platform/cd-providers`);await page.getByRole('button',{name:'Thêm cấu hình CD',exact:true}).click();
  await page.getByLabel('Tên cấu hình',{exact:true}).fill('Fleet existing reconciliation');
  await page.getByLabel('Tài khoản quản lý CD',{exact:true}).selectOption({label:accountName});
  await page.getByLabel('Tài khoản cluster chạy workload',{exact:true}).selectOption({label:accountName});
  await page.getByLabel('Quản lý GitRepo',{exact:true}).selectOption('EXISTING');
  await page.getByRole('button',{name:'Tìm cấu hình trên cluster',exact:true}).click();
  await page.getByLabel('GitRepo có sẵn',{exact:true}).selectOption('orch-reconciliation-existing');
  await expect(page.getByLabel('Định danh GitRepo đã xác minh',{exact:true})).toHaveValue(state.existingGitRepoUID);
  await page.getByLabel('Thư mục manifest được quản lý',{exact:true}).fill('existing/orchestrator');
  await page.getByText('Chi tiết kết nối',{exact:true}).click();
  await page.getByLabel('Namespace agent',{exact:true}).fill('cattle-fleet-system');
  await page.getByLabel('ServiceAccount triển khai',{exact:true}).fill('orch-recon-delivery');
  await page.getByLabel('Namespace lưu Helm release',{exact:true}).fill('orch-recon-releases');
  await page.getByLabel('CA của Git (PEM)',{exact:true}).fill(ca);
  await page.getByText('Thông tin ghi Git',{exact:true}).click();
  await page.getByLabel('Tên đăng nhập Git',{exact:true}).fill(writer[0]);await page.getByLabel('Token hoặc mật khẩu Git',{exact:true}).fill(writer[1]);
  const reply=page.waitForResponse(r=>r.request().method()==='POST'&&new URL(r.url()).pathname==='/api/v1/delivery-bindings');
  await page.getByRole('button',{name:'Lưu cấu hình',exact:true}).click();const response=await reply;expect(response.status()).toBe(201);const binding=await response.json();state.existingBindingID=binding.id;state.existingBindingVersion=binding.version;saveState();
 });
 await page.goto(`${baseURL}/ui/platform/cd-providers`);
 const row=page.getByRole('row').filter({has:page.getByRole('cell',{name:'Fleet existing reconciliation',exact:true})});
 await expect(row).toBeVisible();
 if(!(await row.getByRole('cell',{name:'Sẵn sàng',exact:true}).count())) {await row.getByRole('button',{name:'Xác minh',exact:true}).click();await expect(row.getByRole('cell',{name:'Sẵn sàng',exact:true})).toBeVisible({timeout:120_000});}
 await step('Transfer to existing GitRepo through Environment Settings',async()=>{
  if(state.existingTransferPassed) return;
  const snapshot=()=>JSON.parse(kubectl('get','deployments,statefulsets,persistentvolumeclaims,services','-n',state.namespace,'-o','json')).items.map(o=>({key:`${o.kind}/${o.metadata.name}`,uid:o.metadata.uid,template:o.spec.template})).sort((a,b)=>a.key.localeCompare(b.key));
  state.existingTransferBaseline??=snapshot();saveState();
  await page.goto(`${baseURL}/ui/applications/${state.appID}/settings?environment=staging`);
  await page.getByLabel('CD provider',{exact:true}).selectOption(`${state.existingBindingID}:${state.existingBindingVersion}`);
  await page.getByRole('button',{name:'Lưu thay đổi',exact:true}).click();await expect(page.getByText('Đã lưu. Xem trước để áp dụng.',{exact:false})).toBeVisible();
  await home();await deploy();expect(snapshot()).toEqual(state.existingTransferBaseline);
  const parent=JSON.parse(kubectl('get','gitrepo','orch-reconciliation-existing','-n','fleet-local','-o','json'));expect(parent.metadata.uid).toBe(state.existingGitRepoUID);expect(parent.spec).toEqual(state.existingGitRepoSpec);
  const sibling=JSON.parse(kubectl('get','configmap','orch-reconciliation-sibling','-n','orch-recon-sibling','-o','json'));expect(sibling.metadata.uid).toBe(state.siblingUID);expect(sibling.data.message).toBe('retained company sibling');
  state.existingTransferPassed=true;saveState();
 });
 await step('Retained data and new queued job after provider transfer',()=>business.checkBusiness({afterTransfer:true}));
}
