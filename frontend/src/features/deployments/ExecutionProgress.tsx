import { useEffect, useState } from 'react';
import { t, intlLocale } from '../../shared/i18n';
import { Status } from '../../shared/ui/Status';

export type ProgressPod = { phase?: string; name: string; image?: string; reason: string; restarts: number };
export type ProgressEvent = { reason: string; object: string; at: string; count?: number };
export type ProgressStep = { stage?: string; logs?: {at:string;stage:string;status:string}[]; resourceKind?: string; resourceName?: string; namespace?: string; id: string; parent?: string; workload?: string; deploymentId?: string; kind: string; status: string; reason?: string; startedAt: string; updatedAt: string; finishedAt?: string; deadline?: string; pods?: ProgressPod[]; events?: ProgressEvent[]; commit?: string };
export type ExecutionRecord = { version: number; steps: ProgressStep[]; checks?: ProgressStep[]; priorHeartbeat?: string };
export function duration(start: string, end = new Date().toISOString()) {
  const seconds = Math.max(0, Math.floor((Date.parse(end) - Date.parse(start)) / 1000));
  return Number.isFinite(seconds) ? `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}` : '—';
}
const kinds = new Set(['APPLY', 'CLAIM', 'FINALIZE', 'VSO', 'JOB', 'WORKLOAD', 'TERRAFORM', 'PLAN', 'INFRASTRUCTURE', 'SECRETS', 'RENDER', 'PUBLISH', 'READINESS', 'RECOVERY', 'RECOVERY_DELIVERY', 'RECOVERY_RECORDS', 'RECOVERY_PUBLICATION', 'RECOVERY_CONTROLLER', 'RECOVERY_OBJECT', 'absence', 'receipt', 'submission', 'PRIOR_STOPPED', 'initialize', 'binding', 'publication-lease', 'rebind', 'artifact', 'recover-receipt', 'recover-submission', 'observe']);
export const stepLabel = (kind: string) => t(`execution.kind.${kinds.has(kind) ? kind : 'UNKNOWN'}`);
export const progressStatus = (status: string) => t(`execution.status.${['RUNNING', 'PENDING', 'SUCCEEDED', 'FAILED', 'INTERRUPTED'].includes(status) ? status : 'PENDING'}`);
const reasons = new Set(['FLEET_FORBIDDEN','FLEET_AUTH','FLEET_APPLY_FAILED','ABSENT','PUBLISHED','DELIVERY_CONFIRMED','SECRET_NOT_READY','JOB_FAILED','WAITING_DEPENDENCY','WAITING_FLEET','WAITING_READINESS','RECOVERY_REQUIRED','OWNERSHIP']);
const reasonText=(reason:string)=>t(`execution.reason.${reasons.has(reason)?reason:'UNKNOWN'}`);
export function reasonHint(reason: string) {
  const hints = { FLEET_FORBIDDEN:'fleetAccess', FLEET_AUTH:'fleetAuth', ImagePullBackOff: 'image', ErrImagePull: 'image', InvalidImageName: 'image', CrashLoopBackOff: 'crash', CreateContainerConfigError: 'config', FailedMount: 'config', FailedToRetrieveImagePullSecret: 'config', Pending: 'resources', FailedScheduling: 'resources', OOMKilled: 'oom', TERRAFORM: 'terraform', JOB_FAILED: 'job', OWNERSHIP: 'ownership' } as Record<string, string>;
 const category=Object.hasOwn(hints,reason) ? hints[reason] : undefined;
  return category ? t(`execution.hint.${category}`) : '';
}
function Events({ events }: { events: ProgressEvent[] }) {
  return <ul className="execution-events">{events.map((event, i) => <li key={`${event.object}-${event.reason}-${i}`}><time>{new Date(event.at).toLocaleString(intlLocale())}</time> · {event.object} · {event.reason}{event.count ? ` ×${event.count}` : ''}</li>)}</ul>;
}
function Step({ step, now, children = [] }: { step: ProgressStep; now: string; children?: ProgressStep[] }) {
  const [open, setOpen] = useState(step.status === 'RUNNING' || step.status === 'FAILED');
  useEffect(() => { if (step.status === 'RUNNING' || step.status === 'FAILED') setOpen(true); }, [step.status]);
  const pods = step.pods ?? []; const events = step.events ?? [];
  const title=step.kind==='TERRAFORM' && step.resourceName ? t('execution.terraformResource',{resource:step.resourceName.split('.')[0]}) : step.kind==='RECOVERY_OBJECT' && step.resourceName ? `${step.resourceKind ?? ''} ${step.resourceName}` : stepLabel(step.kind);
  return <li className={`execution-step execution-${step.status.toLowerCase()}`}><details open={open} onToggle={(event) => setOpen(event.currentTarget.open)}>
    <summary><strong>{title}</strong> <Status tone={step.status === 'SUCCEEDED' ? 'good' : step.status === 'FAILED' ? 'draft' : 'neutral'}>{progressStatus(step.status)}</Status> <span className="execution-time">{step.status === 'PENDING' ? '—' : duration(step.startedAt, step.finishedAt ?? now)}{step.deadline && step.status === 'RUNNING' ? ` · ${t('execution.remaining', { time: duration(now, step.deadline) })}` : ''}</span></summary>
    {step.reason ? <p>{reasonText(step.reason)}</p> : null}
    {step.resourceName ? <p>{step.resourceKind} <strong>{step.resourceName}</strong> · {step.namespace}</p> : null}
    {step.reason && reasonHint(step.reason) ? <p>{reasonHint(step.reason)}</p> : null}
    {step.logs?.length ? <details><summary>{t('execution.terraformLog')}</summary><p><small>{t('execution.structuredLog')}</small></p><ol>{step.logs.map((log,i)=><li key={i}>{new Date(log.at).toLocaleString(intlLocale())} · {log.stage} · {progressStatus(log.status)}</li>)}</ol></details> : null}
    {step.commit ? <p>Git · <code>{step.commit.slice(0, 12)}</code></p> : null}
    {pods.length ? <table className="plain-table"><thead><tr><th>Pod</th><th>{t('execution.podStatus')}</th><th>{t('execution.reason')}</th><th>Image</th><th>{t('execution.restarts')}</th></tr></thead><tbody>{pods.map((pod, i) => <tr key={`${pod.name}-${i}`}><td>{pod.name}</td><td>{t(`execution.podStatus.${['Pending','Running','Succeeded','Failed'].includes(pod.phase ?? '') ? pod.phase : 'Unknown'}`)}</td><td>{pod.reason}</td><td className="execution-image">{pod.image || '—'}</td><td>{pod.restarts}</td></tr>)}</tbody></table> : null}
    {events.length ? <><h4>{t('execution.events')}</h4><Events events={events.slice(0, 3)} />{events.length > 3 ? <details><summary>{t('execution.allEvents', { count: events.length })}</summary><Events events={events} /></details> : null}</> : null}
    {children.length ? <ol className="execution-timeline">{children.map((child)=><Step key={child.id} step={child} now={now} />)}</ol> : null}
  </details></li>;
}
export function RecoveryChecks({ record }: { record: ExecutionRecord }) {
 const failures=[...(record.checks ?? [])].reverse().filter((s)=>s.status==='FAILED');
 const check=failures.find((s)=>s.resourceName) ?? failures[0];
 return <>{check ? <div className="form-warning"><strong>{check.resourceKind} {check.resourceName || stepLabel(check.kind)}</strong>{check.namespace ? <span> · {check.namespace}</span> : null}<p>{reasonHint(check.reason ?? '') || t('execution.reason.RECOVERY_REQUIRED')}</p></div> : null}<details><summary>{t('execution.checks',{count:record.checks?.length ?? 0})}</summary><ol className="execution-timeline">{record.checks?.map((step)=><Step key={step.id} step={step} now={step.updatedAt} />)}</ol></details></>;
}
export function ExecutionProgress({ record, operationStatus }: { record: ExecutionRecord; operationStatus?: string }) {
  const [now, setNow] = useState(new Date().toISOString());
  const running = record.steps.some((s) => s.status === 'RUNNING');
  useEffect(() => { if (!running) return; const timer = window.setInterval(() => setNow(new Date().toISOString()), 1000); return () => window.clearInterval(timer); }, [running]);
  const roots=record.steps.filter((s)=>!s.parent || !record.steps.some((parent)=>parent.id===s.parent));
 const groups: {workload:string;steps:ProgressStep[]}[]=[];
 for (const step of roots) {const workload=step.workload || '';const last=groups.at(-1);if(last?.workload===workload) last.steps.push(step);else groups.push({workload,steps:[step]});}
  const failures=[...record.steps].reverse().filter((s)=>s.status==='FAILED');
 const failure=failures.find((s)=>!['FINALIZE','WORKLOAD'].includes(s.kind)) ?? failures[0];
 const pod=failure?.pods?.find((p)=>Boolean(reasonHint(p.reason)));
  return <section className="content-panel" aria-label={t('execution.title')}><h2>{t('execution.title')}</h2>
    {failure ? <div className="form-error" role="alert"><strong>{pod ? `${pod.name}: ${pod.reason}` : stepLabel(failure.kind)}</strong>{!pod && failure.reason ? <p>{reasonText(failure.reason)}</p> : null}{pod?.image ? <p className="execution-image">{pod.image}</p> : null}{reasonHint(pod?.reason ?? failure.reason ?? '') ? <p>{reasonHint(pod?.reason ?? failure.reason ?? '')}</p> : null}{operationStatus ? <p>{t(`execution.${['SUCCEEDED', 'FAILED', 'RECOVERED'].includes(operationStatus) ? 'unlocked' : 'held'}`)}</p> : null}</div> : null}
    {record.steps.length ? groups.map((group,index) => <div key={`${group.workload}-${index}`}><h3>{!group.workload || group.workload === '_environment' ? t('execution.environment') : group.workload}</h3><ol className="execution-timeline">{group.steps.map((step) => <Step key={step.id} step={step} now={now} children={record.steps.filter((s)=>s.parent===step.id)} />)}</ol></div>) : <p>{t('execution.noProgress')}</p>}
    {record.checks?.length ? <RecoveryChecks record={record} /> : null}
  </section>;
}
