import { useEffect, useState } from 'react';
import { api, describeError } from '../../shared/api/client';
import { navigate } from '../../app/routes';
import type { EnvironmentKey } from '../../shared/types/application';
import { intlLocale, t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import { Status } from '../../shared/ui/Status';

type Job = { name: string; uid: string; createdAt: string; startedAt?: string; finishedAt?: string; state: 'Pending' | 'Running' | 'Succeeded' | 'Failed' };
type History = { generatedAt: string; code: string; jobs: Job[]; truncated: boolean };
const text = (key: string) => t(`workloads.jobs.${key}`);
export async function readJobs(applicationId: string, environment: EnvironmentKey, workload: string, signal?: AbortSignal): Promise<History> {
  const result = await api<History>(`/applications/${encodeURIComponent(applicationId)}/environments/${environment}/workloads/${encodeURIComponent(workload)}/jobs`, { signal });
  const stamp = (value: unknown) => typeof value === 'string' && Number.isFinite(Date.parse(value));
  if (!result || !stamp(result.generatedAt) || typeof result.code !== 'string' || typeof result.truncated !== 'boolean' || !Array.isArray(result.jobs) || result.jobs.length > 100 || !result.jobs.every(row => typeof row.name === 'string' && typeof row.uid === 'string' && stamp(row.createdAt) && (row.startedAt === undefined || stamp(row.startedAt)) && (row.finishedAt === undefined || stamp(row.finishedAt)) && ['Pending', 'Running', 'Succeeded', 'Failed'].includes(row.state))) throw new Error('Job history unavailable');
  return result;
}
export function JobHistoryDialog({ applicationId, environment, workload, onClose }: { applicationId: string; environment: EnvironmentKey; workload: string; onClose(): void }) {
  const [history, setHistory] = useState<History>();
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let current = true; const controller = new AbortController(); setLoading(true); setError(''); setHistory(undefined);
    readJobs(applicationId, environment, workload, controller.signal).then(value => { if (current) { setHistory(value); if (value.code !== 'OK') setError(text('unavailable')); } }).catch(err => { if (current) setError(describeError(err, text('unavailable'))); }).finally(() => { if (current) setLoading(false); });
    return () => { current = false; controller.abort(); };
  }, [applicationId, environment, workload, attempt]);
  return <Dialog label={text('title')} onClose={onClose}>
    <h2>{text('title')} · {workload}</h2>
    {loading ? <p role="status">{text('loading')}</p> : null}
    {error ? <p role="alert">{error}</p> : null}
    {history?.code === 'OK' ? <>
      <p className="muted">{text('retention')}</p>
      {history.truncated ? <p role="status">{text('truncated')}</p> : null}
      {history.jobs.length ? <table><thead><tr><th>Job</th><th>{text('state')}</th><th>{text('created')}</th><th>{text('finished')}</th></tr></thead><tbody>{history.jobs.map(job => <tr key={job.uid}><td>{job.name}</td><td><Status tone={job.state === 'Succeeded' ? 'good' : 'neutral'}>{text(job.state)}</Status></td><td>{new Date(job.createdAt).toLocaleString(intlLocale())}</td><td>{job.finishedAt ? new Date(job.finishedAt).toLocaleString(intlLocale()) : '—'}</td></tr>)}</tbody></table> : <p>{text('empty')}</p>}
      <small>{text('observed')} {new Date(history.generatedAt).toLocaleString(intlLocale())}</small>
    </> : null}
    <div className="form-actions"><Button disabled={loading} onClick={() => setAttempt(value => value + 1)}>{text('refresh')}</Button><Button onClick={() => navigate({ name: 'observability', applicationId, environment, tab: 'logs', query: { workload } })}>{text('logs')}</Button><Button onClick={onClose}>{text('close')}</Button></div>
  </Dialog>;
}
