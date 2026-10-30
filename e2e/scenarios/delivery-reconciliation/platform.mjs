// Platform-only permission setup, after Terraform creates the app namespace.
// Never creates/adopts an application namespace or grants access outside it.
import { expect } from '@playwright/test';
export async function grantWorkloadAccess({ namespace, kubectl, state, saveState }) {
  let ns;
  await expect.poll(() => { try { ns = JSON.parse(kubectl('get','namespace',namespace,'-o','json')); return ns.metadata.labels?.['app.kubernetes.io/managed-by']; } catch { return ''; } }, { timeout: 10 * 60_000, intervals: [1000] }).toBe('orchestrator');
  if (state.namespaceUID && ns.metadata.uid !== state.namespaceUID) throw Error('Application namespace UID changed');
  state.namespaceUID = ns.metadata.uid; saveState();
  const name = 'orch-reconciliation-workloads';
  const labels = { 'orchestrator.test/run': state.runID };
  const resources = [
    { apiVersion:'rbac.authorization.k8s.io/v1',kind:'Role',metadata:{name,namespace,labels},rules:[
      {apiGroups:[''],resources:['configmaps','services','persistentvolumeclaims','serviceaccounts','pods'],verbs:['get','list','watch','create','update','patch','delete']},
      {apiGroups:['apps'],resources:['deployments','statefulsets','daemonsets','replicasets'],verbs:['get','list','watch','create','update','patch','delete']},
      {apiGroups:['autoscaling'],resources:['horizontalpodautoscalers'],verbs:['get','list','watch','create','update','patch','delete']},
      {apiGroups:['batch'],resources:['jobs','cronjobs'],verbs:['get','list','watch','create','update','patch','delete']},
      {apiGroups:['networking.k8s.io','gateway.networking.k8s.io'],resources:['ingresses','httproutes'],verbs:['get','list','watch','create','update','patch','delete']},
    ]},
    {apiVersion:'rbac.authorization.k8s.io/v1',kind:'RoleBinding',metadata:{name,namespace,labels},roleRef:{apiGroup:'rbac.authorization.k8s.io',kind:'Role',name},subjects:[{kind:'ServiceAccount',namespace:'cattle-fleet-system',name:'orch-recon-delivery'}]},
  ];
  for (const object of resources) {
    let current; try { current=JSON.parse(kubectl('get',object.kind,name,'-n',namespace,'-o','json')); } catch { /* Not yet installed. */ }
    if (current) {
      if (current.metadata.labels?.['orchestrator.test/run'] !== state.runID || current.metadata.uid !== state.platformRBAC?.[object.kind]) throw Error('Refusing preexisting platform RBAC');
      continue;
    }
    const created=JSON.parse(kubectl.withInput(JSON.stringify(object),'create','-f','-','-o','json'));
    state.platformRBAC ??= {}; state.platformRBAC[object.kind]=created.metadata.uid; saveState();
  }
}

// Creates an independent watched subtree and sibling as platform prerequisites.
// The product must later leave this GitRepo and sibling unchanged.
export async function existingGitRepoFixture({kubectl,state,saveState,run,ca}) {
  const name='orch-reconciliation-existing';
  if (state.existingGitRepoUID) {
    const current=JSON.parse(kubectl('get','gitrepo',name,'-n','fleet-local','-o','json'));
    expect(current.metadata.uid).toBe(state.existingGitRepoUID);return;
  }
  const {execFileSync}=await import('node:child_process');
  const {join}=await import('node:path');
  const {mkdirSync,writeFileSync,existsSync}=await import('node:fs');
  const writer=join(run,'existing-fixture-writer');
  if(existsSync(writer)) throw Error('Existing fixture writer requires inspection before retry');
  execFileSync('git',['clone','--branch','dev',join(run,'git','repo.git'),writer],{stdio:'pipe'});
  const dir=join(writer,'existing','sibling');mkdirSync(dir,{recursive:true,mode:0o700});
  writeFileSync(join(dir,'fleet.yaml'),JSON.stringify({defaultNamespace:'orch-recon-releases',deleteNamespace:false,serviceAccount:'orch-recon-delivery',helm:{releaseName:'orch-reconciliation-sibling'}}));
  writeFileSync(join(dir,'configmap.yaml'),JSON.stringify({apiVersion:'v1',kind:'ConfigMap',metadata:{name:'orch-reconciliation-sibling',namespace:'orch-recon-sibling',labels:{'orchestrator.test/run':state.runID}},data:{message:'retained company sibling'}}));
  execFileSync('git',['-C',writer,'add','existing'],{stdio:'pipe'});
  execFileSync('git',['-C',writer,'-c','user.name=Reconciliation fixture','-c','user.email=fixture@example.invalid','commit','-m','Prepare preexisting Fleet parent and sibling fixture'],{stdio:'pipe'});
  execFileSync('git',['-C',writer,'push','origin','HEAD:dev'],{stdio:'pipe'});
  const labels={'orchestrator.test/run':state.runID};
  const create=o=>JSON.parse(kubectl.withInput(JSON.stringify(o),'create','-f','-','-o','json'));
  create({apiVersion:'v1',kind:'Namespace',metadata:{name:'orch-recon-sibling',labels}});
  create({apiVersion:'rbac.authorization.k8s.io/v1',kind:'Role',metadata:{name,namespace:'orch-recon-sibling',labels},rules:[{apiGroups:[''],resources:['configmaps'],verbs:['get','list','watch','create','patch','update','delete']}]});
  create({apiVersion:'rbac.authorization.k8s.io/v1',kind:'RoleBinding',metadata:{name,namespace:'orch-recon-sibling',labels},roleRef:{apiGroup:'rbac.authorization.k8s.io',kind:'Role',name},subjects:[{kind:'ServiceAccount',namespace:'cattle-fleet-system',name:'orch-recon-delivery'}]});
  const repo=create({apiVersion:'fleet.cattle.io/v1alpha1',kind:'GitRepo',metadata:{name,namespace:'fleet-local',labels},spec:{repo:'https://172.18.0.1:19443/repo.git',branch:'dev',paths:['existing'],pollingInterval:'15s',caBundle:Buffer.from(ca).toString('base64'),targets:[{clusterName:'local'}]}});
  state.existingGitRepoUID=repo.metadata.uid;state.existingGitRepoSpec=repo.spec;saveState();
  await expect.poll(()=>{try{return JSON.parse(kubectl('get','configmap','orch-reconciliation-sibling','-n','orch-recon-sibling','-o','json')).metadata.uid;}catch{return '';}},{timeout:180_000,intervals:[2000]}).not.toBe('');
  state.siblingUID=JSON.parse(kubectl('get','configmap','orch-reconciliation-sibling','-n','orch-recon-sibling','-o','json')).metadata.uid;saveState();
}
