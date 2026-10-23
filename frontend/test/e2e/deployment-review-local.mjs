// Product HTTP/browser verification against an isolated fake-adapter backend.
// It creates disposable application/catalog fixtures; it never contacts a cluster or cloud.
import { chromium, expect } from '@playwright/test';
import {mkdir, writeFile} from 'node:fs/promises';
import {createHuman,createApplication,addWorkload} from './human.mjs';
const base=process.env.REVIEW_BASE_URL??'http://127.0.0.1:18092';
const evidence=process.env.REVIEW_EVIDENCE_DIR;
if(!evidence)throw Error('REVIEW_EVIDENCE_DIR required');
if(!/^http:\/\/127\.0\.0\.1:\d+$/.test(base))throw Error('Loopback fixture only');
await mkdir(evidence,{recursive:true});
const browser=await chromium.launch({channel:'chrome',headless:true});
const context=await browser.newContext({viewport:{width:1512,height:1100},permissions:['clipboard-read','clipboard-write']});
const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
const assertions=[];
const check=(name,value)=>{if(!value)throw Error(name);assertions.push(name);};
try{
 await page.goto(`${base}/ui/sign-in`);
 await page.getByLabel('Tên đăng nhập').fill('platform-engineer');await page.getByLabel('Mật khẩu').fill('test-password');await page.getByRole('button',{name:'Đăng nhập',exact:true}).click();
 await expect(page.getByRole('heading',{name:'Ứng dụng của bạn'})).toBeVisible();
 const h=createHuman(page,{fast:true});
 const app=await createApplication(h,`Deployment Review QA ${Date.now()}`, `review-${Date.now().toString().slice(-8)}`);
 const prefix=`/api/v1/applications/${app}/environments/staging`;
 const definitions=(await (await context.request.get(`${base}/api/v1/resource-definitions`)).json()).resourceDefinitions;
 for(const key of ['cluster-internal','namespace-k8s-module','workload-score-k8s']){
  const original=definitions.find(d=>d.key===key);if(!original)throw Error(`missing fixture definition ${key}`);
  const {version,sourceFingerprint,...draft}=original;void version;void sourceFingerprint;
  draft.key=`${key}-${app}`;draft.name=`QA ${original.name}`;draft.criteria=[{app_id:app,...(key==='cluster-internal'?{class:'internal'}:{})}];
  const res=await context.request.post(`${base}/api/v1/resource-definitions`,{data:draft});if(!res.ok())throw Error(`register ${key}: HTTP ${res.status()} ${JSON.stringify(await res.json())}`);check(`register ${key}`,true);
 }
 for(const name of ['api','web'])await addWorkload(h,{name,image:`example.invalid/${name}:v1`,bindings:[],port:{name:'http',port:'8080'}});
 const previewResponse=page.waitForResponse(r=>r.url().endsWith('/preview')&&r.request().method()==='POST');
 await page.getByRole('button',{name:'Xem trước thay đổi',exact:true}).click();
 const preview=await(await previewResponse).json();check('batch graph includes both workloads',preview.review.nodes.filter(n=>n.kind==='workload').length===2);
 check('existing cluster is only linked',preview.review.nodes.find(n=>n.resourceType==='k8s-cluster').intent==='BIND');
 check('new workloads compare against original empty base',preview.review.artifacts.filter(a=>a.kind==='kubernetes-manifest').every(a=>a.before.availability==='ABSENT'));
 const reviewPanel=page.getByLabel('Xem trước triển khai');
 await expect(reviewPanel.locator('.review-edges>path')).not.toHaveCount(0);
 await reviewPanel.locator('[data-node-id="workload.default#modules.api"]').click();await reviewPanel.getByRole('tab',{name:'Code',exact:true}).click();
 await expect(reviewPanel.locator('.review-diff-new')).not.toHaveCount(0);
 await reviewPanel.getByRole('button',{name:'Mở rộng',exact:true}).first().click();await expect(page.locator('.review-code-expanded')).toBeVisible();await page.getByRole('button',{name:'Thu gọn',exact:true}).first().click();await expect(page.locator('.review-code-expanded')).toHaveCount(0);
 check('code expands and collapses',true);
 await page.screenshot({path:`${evidence}/new-preview.png`,fullPage:true});
 const deployResponse=page.waitForResponse(r=>r.url().endsWith('/deploy')&&r.request().method()==='POST');
 await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();
 const response=await deployResponse;const report=await response.json();check('fake-adapter product deployment succeeds',response.ok()&&report.status==='SUCCEEDED');
 for(const workload of ['api','web'])await expect(page.getByLabel('Theo dõi triển khai').locator(`[data-node-id="workload.default#modules.${workload}"]`)).toHaveAttribute('data-state','READY');
 check('both batch workloads keep their own observed readiness',true);
 const requestId=response.request().postDataJSON().reviewRequestId;check('per-click UUID',/^[0-9a-f-]{36}$/.test(requestId));
 await expect(page.getByLabel('Kết quả triển khai')).toContainText('Triển khai thành công');
 const operations=(await(await context.request.get(`${base}${prefix}/operations?reviewRequestId=${requestId}`)).json()).operations;check('one exact correlated operation',operations.length===1&&operations[0].reviewRequestId===requestId&&operations[0].status==='SUCCEEDED');
 const deployments=(await(await context.request.get(`${base}${prefix}/deployments?reviewRequestId=${requestId}`)).json()).deployments;check('two exact correlated deployment records',deployments.length===2&&deployments.every(d=>d.reviewRequestId===requestId));
 const api=deployments.find(d=>d.workloadId==='api');
 const detail=await(await context.request.get(`${base}${prefix}/deployments/${api.id}`)).json();check('rendered review persisted',detail.workloads.some(w=>w.status==='READY'&&w.manifestReview?.version===1));
 check('observed node readiness is persisted',detail.nodeProgress.some(n=>n.nodeId==='workload.default#modules.api'&&n.state==='READY'));
 // Update via the product workload form; snapshot baseline comes from the prior execution.
 await page.locator('.workload-table .table-row').filter({hasText:/api/}).getByRole('button',{name:'Sửa',exact:true}).click();
 await page.getByLabel('Ảnh Container',{exact:true}).fill('example.invalid/api:v2');await page.getByRole('button',{name:'Lưu draft Workload',exact:true}).click();
 const updatedResponse=page.waitForResponse(r=>r.url().endsWith('/preview')&&r.request().method()==='POST');await page.getByRole('button',{name:'Xem trước thay đổi',exact:true}).click();
 const updated=await(await updatedResponse).json();const manifest=updated.review.artifacts.find(a=>a.nodeId==='workload.default#modules.api'&&a.path==='Deployment/api');
 check('update has exact applied baseline',manifest.before.provenance.baselineDeploymentId===api.id&&manifest.before.text.includes('api:v1')&&manifest.after.text.includes('api:v2'));
 await reviewPanel.locator('[data-node-id="workload.default#modules.api"]').click();await reviewPanel.getByRole('tab',{name:'Code',exact:true}).click();
 await expect(reviewPanel.locator('.review-diff-old').filter({hasText:'api:v1'})).toHaveCount(1);await expect(reviewPanel.locator('.review-diff-new').filter({hasText:'api:v2'})).toHaveCount(1);
 await reviewPanel.getByRole('button',{name:'Sao chép bản sau',exact:true}).first().click();const copied=await page.evaluate(()=>navigator.clipboard.readText());check('clipboard contains only new safe code',copied.includes('api:v2')&&!copied.includes('api:v1'));
 await page.screenshot({path:`${evidence}/update-diff.png`,fullPage:true});
 // Persisted deployment detail is independent of newer drafts/current catalog.
 await page.goto(`${base}/ui/applications/${app}/environments/staging/deployments/${api.id}`);
 await expect(page.getByRole('heading',{name:'Chi tiết triển khai',exact:true})).toBeVisible();await expect(page.locator('[data-node-id="workload.default#modules.api"]').first()).toHaveAttribute('data-state','READY');
 await page.screenshot({path:`${evidence}/deployment-detail.png`,fullPage:true});
 // Cloud is preview-only: the authored Definitions select the topology; no cloud executor is invoked.
 await page.goto(`${base}/ui/applications`);
 const cloudApp=await createApplication(h,`Cloud Review QA ${Date.now()}`,`cloud-${Date.now().toString().slice(-8)}`);
 for(const [key,klass] of [['vpc-aws','default'],['cluster-aws-eks','eks'],['postgres-aws-aurora','cloud-managed'],['namespace-k8s-module','default'],['workload-score-k8s','default']]){
  const original=definitions.find(d=>d.key===key);if(!original)throw Error(`missing cloud definition ${key}`);
  const {version,sourceFingerprint,...draft}=structuredClone(original);void version;void sourceFingerprint;
  draft.key=`${key}-${cloudApp}`;draft.name=`Cloud QA ${original.name}`;draft.criteria=[{app_id:cloudApp,class:klass}];
  draft.driverInputs=JSON.parse(JSON.stringify(draft.driverInputs).replaceAll('k8s-cluster.internal#','k8s-cluster.eks#').replaceAll('#@infra','#environments.@app.@env'));
  const res=await context.request.post(`${base}/api/v1/resource-definitions`,{data:draft});if(!res.ok())throw Error(`register cloud ${key}: ${res.status()} ${JSON.stringify(await res.json())}`);
 }
 await page.getByRole('button',{name:'Xem trước Score',exact:true}).click();
 await page.getByLabel('ID Workload',{exact:true}).fill('cloud-api');await page.getByLabel('ID lần chạy',{exact:true}).fill('cloud-review');
 const cloudScore={apiVersion:'score.dev/v1b1',metadata:{name:'cloud-api'},containers:{main:{image:'example.invalid/api:v1',variables:{DATABASE_HOST:'${resources.db.host}'}}},resources:{db:{type:'postgres',class:'cloud-managed',params:{database:'app',username:'app'}}}};
 await page.getByLabel('Score sau (YAML hoặc JSON)').fill(JSON.stringify(cloudScore));
 const cloudReply=page.waitForResponse(r=>r.url().endsWith('/score-preview')&&r.request().method()==='POST');await page.getByRole('button',{name:'Xem trước',exact:true}).click();
 const cloudResponse=await cloudReply;const cloud=await cloudResponse.json();if(!cloudResponse.ok())throw Error(`cloud preview HTTP ${cloudResponse.status()} ${JSON.stringify(cloud)}`);
 check('cloud graph comes from VPC EKS and Aurora Definitions',['vpc','k8s-cluster','postgres'].every(type=>cloud.review.nodes.some(n=>n.resourceType===type)));
 const db=cloud.review.nodes.find(n=>n.resourceType==='postgres');check('managed database has no Kubernetes placement',!db.placement?.cluster&&db.account==='aws-account');
 check('cloud source uses verified module and region',cloud.review.nodes.filter(n=>['vpc','k8s-cluster','postgres'].includes(n.resourceType)).every(n=>n.module?.contentSha256&&n.region==='us-east-1'));
 check('cloud workload uses selected manifest renderer',cloud.review.artifacts.some(a=>a.kind==='kubernetes-manifest'&&a.after.text?.includes('<namespace: chờ cấp phát>')));
 await expect(page.locator(`[data-node-id="${db.id}"]`)).toBeVisible();await page.locator(`[data-node-id="${db.id}"]`).click();
 await page.screenshot({path:`${evidence}/cloud-preview.png`,fullPage:true});
 check('no browser runtime errors',errors.length===0);
 await writeFile(`${evidence}/result.json`,JSON.stringify({status:'PASS',adapterMode:'fake',http:'real product server',applicationId:app,assertions},null,2));
 console.log(`PASS: ${assertions.length} assertions; real product HTTP/UI, fake execution adapters.`);
}catch(e){await page.screenshot({path:`${evidence}/failure.png`,fullPage:true});await writeFile(`${evidence}/result.json`,JSON.stringify({status:'FAIL',assertions,error:String(e),errors},null,2));throw e;}finally{await browser.close();}
