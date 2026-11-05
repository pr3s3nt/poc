import { requireConfig } from './bootstrap-config.mjs';

// Application-scoped Fleet proof. Metadata is read from the actual controller;
// this helper neither fetches credentials nor mutates Git/Kubernetes.
export function verifyApplicationFleet(repositories, bundles, applicationId, environmentId) {
 const repos=repositories.filter(r=>r.metadata.labels?.['orchestrator.io/application-id']===applicationId);
 requireConfig(repos.length===1,'Acceptance requires exactly one Application Fleet GitRepo');
 const repo=repos[0];
 requireConfig(/^[a-f0-9]{40}$/.test(repo.spec.revision)&&repo.status?.commit===repo.spec.revision,'Fleet commit does not match application delivery pin');
 let pins;try{pins=JSON.parse(repo.metadata.annotations?.['orchestrator.io/unit-pins']);}catch{requireConfig(false,'Missing application delivery unit pins');}
 requireConfig(pins&&typeof pins==='object'&&!Array.isArray(pins),'Invalid application delivery unit pins');
 const active=Object.entries(pins).filter(([,pin])=>!pin.removed);
 const scoped=active.filter(([,pin])=>pin.environmentId===environmentId);
 requireConfig(scoped.length>0,'No admitted acceptance Environment units');
 const owned=bundles.filter(b=>b.metadata.namespace===repo.metadata.namespace&&b.metadata.labels?.['fleet.cattle.io/repo-name']===repo.metadata.name);
 requireConfig(owned.length===active.length,'Fleet Bundle inventory differs from admitted units');
 for(const [release,pin]of active){
  requireConfig(repo.spec.paths?.includes(pin.path),'Admitted unit is outside watched paths');
  const matched=owned.filter(b=>b.spec?.helm?.releaseName===release);
  requireConfig(matched.length===1,'Admitted unit does not have exactly one Fleet Bundle');
  const bundle=matched[0];
  requireConfig(bundle.metadata.labels['fleet.cattle.io/commit']===repo.spec.revision&&bundle.status?.summary?.desiredReady===1&&bundle.status.summary.ready===1,'Fleet Bundle not ready at application pinned revision');
 }
 return {repos,bundles:owned,units:scoped.map(([release,pin])=>({release,path:pin.path,revision:pin.revision,digest:pin.digest})).sort((a,b)=>a.path.localeCompare(b.path))};
}
