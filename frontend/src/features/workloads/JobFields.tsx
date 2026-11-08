import { t } from '../../shared/i18n';
import type { Execution } from './templates';
import type { Workload } from './api';

export function JobFields({value,onChange}:{value:Execution;onChange(value:Execution):void}) {
 if(value.kind!=='Job')return null;
 const set=(key:'backoffLimit'|'activeDeadlineSeconds',raw:string)=>{const job={...value.job};if(raw==='')delete job[key];else job[key]=Number(raw);const next={...value};if(Object.keys(job).length)next.job=job;else delete next.job;onChange(next)};
 return <section className="content-panel editor-grid"><h2>{t('workloads.jobs.kind')}</h2><div className="field-grid">
 <label>{t('workloads.jobs.backoff')}<input aria-label={t('workloads.jobs.backoff')} type="number" min="0" step="1" placeholder="0" value={value.job?.backoffLimit??''} onChange={e=>set('backoffLimit',e.target.value)}/></label>
 <label>{t('workloads.jobs.deadline')}<input aria-label={t('workloads.jobs.deadline')} type="number" min="1" step="1" value={value.job?.activeDeadlineSeconds??''} onChange={e=>set('activeDeadlineSeconds',e.target.value)}/></label>
 </div><p className="muted">{t('workloads.jobs.immutable')}</p></section>;
}

export function WaitForJobsFields({value,onChange,workloads,name}:{value:Execution;onChange(value:Execution):void;workloads:Workload[];name:string}) {
 const available=workloads.filter(w=>w.id!==name&&w.state!=='PENDING_DELETE'&&(w.runtime==='Job'||(w.workload?.workloadExecution as Execution|undefined)?.kind==='Job'));
 const selected=value.waitForJobs??[];
 const missing=selected.filter(id=>!available.some(w=>w.id===id));
 function toggle(id:string,checked:boolean){const next={...value},wait=checked?[...selected,id]:selected.filter(n=>n!==id);if(wait.length)next.waitForJobs=wait;else delete next.waitForJobs;onChange(next)}
 return <fieldset className="editor-grid"><legend>{t('workloads.jobs.wait')}</legend>{available.length||missing.length?<>{available.map(w=><label className="checkbox-row" key={w.id}><input type="checkbox" checked={selected.includes(w.id)} onChange={e=>toggle(w.id,e.target.checked)}/>{w.id}</label>)}{missing.map(id=><label className="checkbox-row" key={id}><input type="checkbox" checked onChange={()=>toggle(id,false)}/>{id} · {t('workloads.jobs.missing')}</label>)}</>:<p className="muted">{t('workloads.jobs.empty')}</p>}</fieldset>;
}
