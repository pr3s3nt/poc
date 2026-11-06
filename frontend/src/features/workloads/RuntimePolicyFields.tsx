import { useEffect, useState, type ReactNode } from 'react';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import type { Execution } from './templates';

export type Seccomp = {type: 'RuntimeDefault' | 'Localhost'; localhostProfile?: string};
export type SecurityContext = { runAsNonRoot?: boolean; runAsUser?: number; runAsGroup?: number; privileged?: boolean; allowPrivilegeEscalation?: boolean; readOnlyRootFilesystem?: boolean; capabilities?: {add?: string[]; drop?: string[]}; seccompProfile?: Seccomp };
export type PodSecurity = Pick<SecurityContext,'runAsNonRoot'|'runAsUser'|'runAsGroup'|'seccompProfile'> & {fsGroup?: number; fsGroupChangePolicy?: string; supplementalGroups?: number[]};
export type RuntimePolicy = {
 serviceEnabled?: boolean;
 defaults?: {version: 'workload-defaults-v1'; environment: string; template?: Record<string,unknown>};
 podSecurityContext?: PodSecurity; automountServiceAccountToken?: boolean; serviceAccountName?: string;
 revisionHistoryLimit?: number;
 updateStrategy?: {type: string; rollingUpdate?: {maxUnavailable?: number|string; maxSurge?: number|string; partition?: number}};
 podDisruptionBudget?: {enabled: boolean; minAvailable?: number|string; maxUnavailable?: number|string; unhealthyPodEvictionPolicy?: string};
 volumeClaimTemplates?: {name: string; size: string; accessModes: string[]; storageClassName?: string; volumeMode?: string}[];
 claimMounts?: {claim: string; container: string; path: string; subPath?: string; readOnly: boolean}[];
 persistentVolumeClaimRetentionPolicy?: {whenDeleted: 'Retain'; whenScaled: 'Retain'};
};
export type Resolution = {workload: Record<string, unknown>; sources: Record<string,string>};
export const wd = (key: string) => t(`workloads.defaults.${key}`);
export function ConfigGroup({title,summary,children,invalid=false,initialOpen=false,keepMounted=false}: {title: string; summary?: ReactNode; children: ReactNode; invalid?: boolean; initialOpen?: boolean; keepMounted?: boolean}) {
 const [open,setOpen]=useState(initialOpen);
 const [hasOpened,setHasOpened]=useState(initialOpen);
 useEffect(()=>{if(invalid){setOpen(true);setHasOpened(true)}},[invalid]);
 return <details className="runtime-config-group" open={open} onToggle={event=>{setOpen(event.currentTarget.open);if(event.currentTarget.open)setHasOpened(true)}}><summary><strong>{title}</strong>{summary ? <span className="muted"> · {summary}</span> : null}</summary><div className="editor-grid" hidden={!open} style={open?undefined:{display:'none'}}>{open||(keepMounted&&hasOpened)?children:null}</div></details>;
}
export function fieldValue(resolution: Resolution|undefined,path: string): unknown {return path.split('.').reduce<unknown>((value,key)=>value!==null&&typeof value==='object' ? (value as Record<string,unknown>)[key] : undefined,resolution?.workload)}
export function FieldSource({resolution,path}: {resolution?: Resolution;path: string}) {
 const source=resolution?.sources?.[path];
 return <small className="muted">{source ? wd(source==='environment'?'sourceEnvironment':source) : wd('notConfigured')}</small>;
}
export function OptionalNumber({label,value,onChange,resolution,path,min=0}: {label: string;value?: number;onChange(value?: number): void;resolution?: Resolution;path: string;min?: number}) {
 const inherited=fieldValue(resolution,path);
 return <label>{label}<input aria-label={label} type="number" min={min} value={value??''} placeholder={typeof inherited==='number'?String(inherited):''} onChange={e=>onChange(e.target.value===''?undefined:Number(e.target.value))}/><FieldSource resolution={resolution} path={path}/>{value!==undefined?<Button tone="quiet" onClick={()=>onChange(undefined)}>{wd('reset')}</Button>:null}</label>;
}
function OptionalBoolean({label,value,onChange,resolution,path}: {label: string;value?: boolean;onChange(value?: boolean):void;resolution?:Resolution;path:string}) {
 return <label>{label}<select aria-label={label} value={value===undefined?'':String(value)} onChange={e=>onChange(e.target.value===''?undefined:e.target.value==='true')}><option value="">{wd('inherit')}{typeof fieldValue(resolution,path)==='boolean'?` · ${String(fieldValue(resolution,path))}`:''}</option><option value="true">{wd('yes')}</option><option value="false">{wd('no')}</option></select><FieldSource resolution={resolution} path={path}/></label>;
}
export function SecurityFields({value,onChange,pod=false,resolution,path}: {value?: SecurityContext|PodSecurity;onChange(value?: SecurityContext|PodSecurity):void;pod?:boolean;resolution?:Resolution;path:string}) {
 const current=value??{};
 function set(key:string,v:unknown){const next={...current} as Record<string,unknown>;if(v===undefined||v==='')delete next[key];else next[key]=v;onChange(Object.keys(next).length?next:undefined)}
 return <><div className="field-grid">
 <OptionalBoolean label="runAsNonRoot" value={current.runAsNonRoot} onChange={v=>set('runAsNonRoot',v)} resolution={resolution} path={`${path}.runAsNonRoot`}/>
 {(['runAsUser','runAsGroup',...(pod?['fsGroup']:[])] as const).map(key=><OptionalNumber key={key} label={key} value={(current as Record<string,number|undefined>)[key]} onChange={v=>set(key,v)} resolution={resolution} path={`${path}.${key}`}/>)}
 {!pod?(['privileged','allowPrivilegeEscalation','readOnlyRootFilesystem'] as const).map(key=><OptionalBoolean key={key} label={key} value={(current as SecurityContext)[key]} onChange={v=>set(key,v)} resolution={resolution} path={`${path}.${key}`}/>):null}
 <label>seccompProfile<select value={current.seccompProfile?.type??''} onChange={e=>set('seccompProfile',e.target.value?{type:e.target.value}:undefined)}><option value="">{wd('inherit')}</option><option>RuntimeDefault</option><option>Localhost</option></select><FieldSource resolution={resolution} path={`${path}.seccompProfile.type`}/></label>
 {current.seccompProfile?.type==='Localhost'?<label>localhostProfile<input value={current.seccompProfile.localhostProfile??''} onChange={e=>set('seccompProfile',{...current.seccompProfile,localhostProfile:e.target.value})}/></label>:null}
 {pod?<><label>fsGroupChangePolicy<select value={(current as PodSecurity).fsGroupChangePolicy??''} onChange={e=>set('fsGroupChangePolicy',e.target.value||undefined)}><option value="">{wd('inherit')}</option><option>Always</option><option>OnRootMismatch</option></select></label><label>supplementalGroups<input value={(current as PodSecurity).supplementalGroups?.join(', ')??''} onChange={e=>set('supplementalGroups',e.target.value===''?undefined:e.target.value.split(',').map(Number))}/></label></>:(['add','drop'] as const).map(key=><label key={key}>capabilities.{key}<input value={(current as SecurityContext).capabilities?.[key]?.join(', ')??''} onChange={e=>{const next={...(current as SecurityContext).capabilities};if(e.target.value==='')delete next[key];else next[key]=e.target.value.split(',').map(s=>s.trim());set('capabilities',Object.keys(next).length?next:undefined)}}/></label>)}
 </div>{value?<Button tone="quiet" onClick={()=>onChange(undefined)}>{wd('reset')}</Button>:null}</>;
}
const quantity=(s:string)=>s.endsWith('%')?s:Number(s);
export function RuntimePolicyFields({value,onChange,resolution,replicas,onReplicas,containers,invalid}: {value: Execution;onChange(value:Execution):void;resolution?:Resolution;replicas?:number;onReplicas(value?:number):void;containers:string[];invalid:boolean}) {
 function set<K extends keyof RuntimePolicy>(key:K,v:RuntimePolicy[K]){const next={...value};if(v===undefined)delete next[key];else Object.assign(next,{[key]:v});onChange(next)}
 const strategy=value.updateStrategy;
 const pdb=value.podDisruptionBudget;
 return <>

 {value.kind!=='CronJob'?<section className="content-panel"><ConfigGroup title={wd('scaling')} summary={strategy?.type??String(fieldValue(resolution,'workloadExecution.updateStrategy.type')??wd('inherit'))} invalid={invalid}>
 <OptionalNumber label={t('workloads.templates.replicas')} value={replicas} onChange={onReplicas} resolution={resolution} path="replicas"/>
 <label>{wd('strategy')}<select value={strategy?.type??''} onChange={e=>set('updateStrategy',e.target.value?{type:e.target.value}:undefined)}><option value="">{wd('inherit')}</option><option>RollingUpdate</option><option>{value.kind==='StatefulSet'?'OnDelete':'Recreate'}</option></select><FieldSource resolution={resolution} path="workloadExecution.updateStrategy.type"/></label>
 {(strategy?.type??fieldValue(resolution,'workloadExecution.updateStrategy.type'))==='RollingUpdate'?<div className="field-grid">{value.kind==='Deployment'?(['maxUnavailable','maxSurge'] as const).map(key=><label key={key}>{key}<input value={strategy?.rollingUpdate?.[key]??''} placeholder={String(fieldValue(resolution,`workloadExecution.updateStrategy.rollingUpdate.${key}`)??'')} onChange={e=>{const rolling={...strategy?.rollingUpdate};if(e.target.value==='')delete rolling[key];else rolling[key]=quantity(e.target.value);set('updateStrategy',{type:'RollingUpdate',...(Object.keys(rolling).length?{rollingUpdate:rolling}:{})})}}/></label>):<OptionalNumber label="partition" value={strategy?.rollingUpdate?.partition} onChange={v=>{const rolling={...strategy?.rollingUpdate};if(v===undefined)delete rolling.partition;else rolling.partition=v;set('updateStrategy',{type:'RollingUpdate',rollingUpdate:rolling})}} resolution={resolution} path="workloadExecution.updateStrategy.rollingUpdate.partition"/>}</div>:null}
 <OptionalNumber label="revisionHistoryLimit" value={value.revisionHistoryLimit} onChange={v=>set('revisionHistoryLimit',v)} resolution={resolution} path="workloadExecution.revisionHistoryLimit"/>
 {value.kind==='Deployment'?<><label>{wd('pdb')}<select aria-label={wd('pdb')} value={pdb===undefined?'':pdb.enabled?'on':'off'} onChange={e=>set('podDisruptionBudget',e.target.value===''?undefined:e.target.value==='off'?{enabled:false}:{enabled:true,maxUnavailable:1,unhealthyPodEvictionPolicy:'AlwaysAllow'})}><option value="">{wd('notConfigured')}</option><option value="on">{wd('enabled')}</option><option value="off">{wd('disabled')}</option></select></label>
 {pdb?.enabled?<><label>{wd('budget')}<select value={pdb.minAvailable!==undefined?'minAvailable':'maxUnavailable'} onChange={e=>set('podDisruptionBudget',{enabled:true,[e.target.value]:1,...(pdb.unhealthyPodEvictionPolicy?{unhealthyPodEvictionPolicy:pdb.unhealthyPodEvictionPolicy}:{})})}><option>maxUnavailable</option><option>minAvailable</option></select><input aria-label={wd('budgetValue')} value={pdb.minAvailable??pdb.maxUnavailable??''} onChange={e=>set('podDisruptionBudget',{...pdb,[pdb.minAvailable!==undefined?'minAvailable':'maxUnavailable']:e.target.value===''?undefined:quantity(e.target.value)})}/></label><label>unhealthyPodEvictionPolicy<select value={pdb.unhealthyPodEvictionPolicy??''} onChange={e=>{const next={...pdb};if(e.target.value==='')delete next.unhealthyPodEvictionPolicy;else next.unhealthyPodEvictionPolicy=e.target.value;set('podDisruptionBudget',next)}}><option value="">{wd('inherit')}</option><option>AlwaysAllow</option><option>IfHealthyBudget</option></select></label><p className="muted">{wd('pdbHelp')}</p></>:null}</>:null}
 </ConfigGroup></section>:null}
 {value.kind==='StatefulSet'?<section className="content-panel"><ConfigGroup title={wd('statefulStorage')} summary={`${value.volumeClaimTemplates?.length??0} · Retain`} invalid={invalid}>
 <p className="muted">{wd('claimHelp')}</p>
 {(value.volumeClaimTemplates??[]).map((claim,index)=><fieldset className="editor-grid" key={index}><legend>{claim.name||wd('newClaim')}</legend><div className="field-grid"><label>{wd('claimName')}<input value={claim.name} onChange={e=>set('volumeClaimTemplates',value.volumeClaimTemplates!.map((c,i)=>i===index?{...c,name:e.target.value}:c))}/></label><label>{wd('size')}<input value={claim.size} onChange={e=>set('volumeClaimTemplates',value.volumeClaimTemplates!.map((c,i)=>i===index?{...c,size:e.target.value}:c))}/></label><label>StorageClass<input value={claim.storageClassName??''} onChange={e=>{const next={...claim};if(e.target.value==='')delete next.storageClassName;else next.storageClassName=e.target.value;set('volumeClaimTemplates',value.volumeClaimTemplates!.map((c,i)=>i===index?next:c))}}/></label><label>accessModes<select value={claim.accessModes[0]??'ReadWriteOnce'} onChange={e=>set('volumeClaimTemplates',value.volumeClaimTemplates!.map((c,i)=>i===index?{...c,accessModes:[e.target.value]}:c))}>{['ReadWriteOnce','ReadWriteMany','ReadOnlyMany','ReadWriteOncePod'].map(mode=><option key={mode}>{mode}</option>)}</select></label></div><Button tone="quiet" onClick={()=>set('volumeClaimTemplates',value.volumeClaimTemplates!.filter((_,i)=>i!==index))}>{wd('removeClaim')}</Button></fieldset>)}
 <Button onClick={()=>set('volumeClaimTemplates',[...(value.volumeClaimTemplates??[]),{name:'',size:'',accessModes:['ReadWriteOnce']}])}>{wd('addClaim')}</Button>
 {(value.claimMounts??[]).map((mount,index)=><div className="field-grid" key={index}><label>{wd('claimName')}<select value={mount.claim} onChange={e=>set('claimMounts',value.claimMounts!.map((m,i)=>i===index?{...m,claim:e.target.value}:m))}><option value="">—</option>{value.volumeClaimTemplates?.map(c=><option key={c.name}>{c.name}</option>)}</select></label><label>Container<select value={mount.container} onChange={e=>set('claimMounts',value.claimMounts!.map((m,i)=>i===index?{...m,container:e.target.value}:m))}>{containers.map(c=><option key={c}>{c}</option>)}</select></label>{(['path','subPath'] as const).map(key=><label key={key}>{key}<input value={mount[key]??''} onChange={e=>set('claimMounts',value.claimMounts!.map((m,i)=>i===index?{...m,[key]:e.target.value}:m))}/></label>)}<label><input type="checkbox" checked={mount.readOnly} onChange={e=>set('claimMounts',value.claimMounts!.map((m,i)=>i===index?{...m,readOnly:e.target.checked}:m))}/>{wd('readOnly')}</label><Button tone="quiet" onClick={()=>set('claimMounts',value.claimMounts!.filter((_,i)=>i!==index))}>{wd('removeMount')}</Button></div>)}
 <Button disabled={!value.volumeClaimTemplates?.length} onClick={()=>set('claimMounts',[...(value.claimMounts??[]),{claim:value.volumeClaimTemplates?.[0]?.name??'',container:containers[0]??'',path:'',readOnly:false}])}>{wd('addClaimMount')}</Button>
 </ConfigGroup></section>:null}
 <section className="content-panel"><ConfigGroup title={wd('podSecurity')} summary={value.serviceAccountName??wd('managed')} invalid={invalid}>
 <label>ServiceAccount<input readOnly value={value.serviceAccountName??wd('managed')}/><small className="muted">{wd('identityHelp')}</small></label>
 <OptionalBoolean label="automountServiceAccountToken" value={value.automountServiceAccountToken} onChange={v=>set('automountServiceAccountToken',v)} resolution={resolution} path="workloadExecution.automountServiceAccountToken"/>
 <SecurityFields pod value={value.podSecurityContext} onChange={v=>set('podSecurityContext',v)} resolution={resolution} path="workloadExecution.podSecurityContext"/>
 </ConfigGroup></section>
 </>;
}
