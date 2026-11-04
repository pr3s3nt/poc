import { requireDevTest } from './execution-profile.mjs';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { conflict, equal } from './runtime.mjs';
import { BootstrapError, requireConfig } from './bootstrap-config.mjs';
// Technical setup only: a Role/RoleBinding scoped to the product-created app namespace.
// Never creates/adopts the Namespace or changes shared controller/cluster permissions.
export async function ensureNamespaceAccess(ui,config,app,rt,{wait=true}={}) {
 const access=config.platform.delivery.namespaceAccess;
 if(!access?.enabled)return;
 requireDevTest(config);
 const ns=`app-${app.key}-${config.application.environment}`;
 const args=['--kubeconfig',config.runtime.clusterKubeconfigFile,'--context',config.platform.account.context];
 const run=(more,input)=>execFileSync('kubectl',[...args,...more],{encoding:'utf8',input,timeout:15000,stdio:['pipe','pipe','pipe']});
 const deadline=Date.now()+config.runtime.timeoutMs;let namespace;
 for(;;){
  try{namespace=JSON.parse(run(['get','namespace',ns,'-o','json']));break;}catch{}
  requireConfig(wait&&Date.now()<deadline,'Product Namespace unavailable for scoped access setup');await ui.page.waitForTimeout(1000);
 }
 conflict(namespace.metadata.labels?.['app.kubernetes.io/managed-by']==='orchestrator'&&namespace.metadata.labels?.['orchestrator-managed']==='true','Namespace ownership');
 rt.state.namespaceUid??=namespace.metadata.uid;conflict(rt.state.namespaceUid===namespace.metadata.uid,'Namespace UID');rt.save();
 const subject=rt.state.deliverySubject;requireConfig(subject?.name&&subject?.namespace,'CD subject unavailable');
 const name='bootstrap-fleet-access';
 const rules=JSON.parse(readFileSync(access.rulesFile,'utf8')).rules;
 const role={apiVersion:'rbac.authorization.k8s.io/v1',kind:'Role',metadata:{name,namespace:ns,labels:{'bootstrap.orchestrator.dev/application':app.key}},rules};
 const rb={apiVersion:'rbac.authorization.k8s.io/v1',kind:'RoleBinding',metadata:role.metadata,subjects:[{kind:'ServiceAccount',name:subject.name,namespace:subject.namespace}],roleRef:{apiGroup:'rbac.authorization.k8s.io',kind:'Role',name}};
 for(const desired of [role,rb]){
  let existing;
  try{existing=JSON.parse(run(['-n',ns,'get',desired.kind,name,'--ignore-not-found','-o','json'])||'null');}catch{throw new BootstrapError('Cannot inspect namespace access');}
  if(existing){
   conflict(existing.metadata.labels?.['bootstrap.orchestrator.dev/application']===app.key,'Namespace access owner');
   conflict(desired.kind==='Role'?equal(existing.rules,desired.rules):equal(existing.subjects,desired.subjects)&&equal(existing.roleRef,desired.roleRef),'Namespace access rules');
   rt.record(`namespace-access:${desired.kind}`,'REUSED');
  }else{
   try{run(['create','-f','-'],JSON.stringify(desired));}catch{throw new BootstrapError('Namespace access create unconfirmed; inspect before rerun');}
   rt.record(`namespace-access:${desired.kind}`,'CREATED');
  }
 }
}
