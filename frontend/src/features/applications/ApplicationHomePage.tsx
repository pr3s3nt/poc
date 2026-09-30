import { useEffect, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { endpointFor } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { deleteWorkload, deployChanges, getWorkloads, previewChanges, undoWorkloadDelete, type DeployReport, type PendingPreview, type WorkloadList } from '../workloads/api';
import { RecentDeployments } from '../deployments/RecentDeployments';

export function ApplicationHomePage({ application, created = false }: { application: Application; created?: boolean }) {
  const [environment, setEnvironment] = useState<EnvironmentKey>('staging');
  const [data, setData] = useState<WorkloadList>();
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [preview, setPreview] = useState<PendingPreview>();
  const [previewing, setPreviewing] = useState(false);
  const [deploying, setDeploying] = useState(false);
  const [deployReport, setDeployReport] = useState<DeployReport>();
  useEffect(() => {
    let cancelled = false;
    setData(undefined); setPreview(undefined); setDeployReport(undefined); setLoading(true); setError('');
    getWorkloads(application.id, environment).then((result) => { if (!cancelled) setData(result); }).catch((err: Error) => { if (!cancelled) setError(err.message); }).finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [application.id, environment]);
  async function changeDeletion(id: string, undo: boolean) {
    if (!data) return;
    if (!undo && !window.confirm(`Mark ${id} for deletion in ${environment}? Running workloads will not change until Deploy.`)) return;
    try { setError(''); setPreview(undefined); setDeployReport(undefined); setData(undo ? await undoWorkloadDelete(application.id, environment, id, data.draftVersion) : await deleteWorkload(application.id, environment, id, data.draftVersion)); }
    catch (err) { setError((err as Error).message); }
  }
  async function loadPreview() {
    setPreviewing(true); setError(''); setPreview(undefined); setDeployReport(undefined);
    try { setPreview(await previewChanges(application.id, environment)); }
    catch (err) { setError((err as Error).message); }
    finally { setPreviewing(false); }
  }
  async function deployPreview() {
    if (!preview || (preview.changes.length === 0 && !preview.routePending)) return;
    setDeploying(true); setError('');
    try {
      const report = await deployChanges(application.id, environment, preview.token);
      setDeployReport(report); setPreview(undefined);
      setData(await getWorkloads(application.id, environment));
    } catch (err) { setError((err as Error).message); setPreview(undefined); }
    finally { setDeploying(false); }
  }
  const workloads = data?.workloads ?? [];
  return <section className="page application-home"><button className="back-link" onClick={() => navigate({ name: 'applications' })}>← Applications</button>
    <header className="page-header application-header"><div><p className="eyebrow">Application</p><h1>{application.name}</h1><p>{endpointFor(application, 'production')}</p></div><Button onClick={() => navigate({ name: 'settings', applicationId: application.id })}>Variables &amp; Secrets</Button></header>
    {created ? <div className="form-success" role="status">Application created. Staging and production are ready; nothing has been deployed yet.</div> : null}
    <div className="tabs" role="tablist"><button className={environment === 'staging' ? 'tab tab-active' : 'tab'} onClick={() => setEnvironment('staging')}>Staging<span>{endpointFor(application, 'staging')}</span></button><button className={environment === 'production' ? 'tab tab-active' : 'tab'} onClick={() => setEnvironment('production')}>Production<span>{endpointFor(application, 'production')}</span></button></div>
    <section className="content-panel"><div className="section-header"><div><h2>Workloads</h2><p>Configuration for {environment}; saving here does not deploy.</p></div><Button onClick={() => navigate({ name: 'workload', applicationId: application.id, environment })}>+ Add workload</Button></div>
      {error ? <div className="form-error" role="alert">{error}</div> : null}
      {loading ? <p>Loading workloads…</p> : workloads.length ? <div className="workload-table"><div className="table-head"><span>Name</span><span>Status</span><span>Actions</span></div>{workloads.map((workload) => <div className="table-row" key={workload.id}><span className="workload-name"><span className="workload-icon">◫</span>{workload.id}</span><Status tone={workload.state ? 'draft' : 'good'}>{workload.state === 'PENDING_DELETE' ? 'Pending deletion' : workload.state === 'PENDING_UPSERT' ? 'Pending change' : 'Ready'}</Status><span>{workload.state === 'PENDING_DELETE' ? <Button tone="quiet" onClick={() => void changeDeletion(workload.id, true)}>Undo</Button> : <><Button tone="quiet" disabled={!workload.score} onClick={() => navigate({ name: 'workload', applicationId: application.id, environment, workloadId: workload.id })}>Edit</Button><Button tone="danger" onClick={() => void changeDeletion(workload.id, false)}>Delete</Button></>}</span></div>)}</div> : <div className="section-empty">No workloads in this environment yet.</div>}
      <div className="form-actions"><Button tone="primary" disabled={loading || previewing} onClick={() => void loadPreview()}>{previewing ? 'Calculating preview…' : 'Preview changes'}</Button></div>
      {preview ? <div className="content-panel" aria-label="Deployment preview"><h3>Preview for {environment}</h3><p>{preview.changes.length} workload(s) affected · draft v{preview.draftVersion} · configuration {preview.configRevisionId ? preview.configRevisionId.slice(0, 8) : 'empty'}</p>{preview.changes.length ? <ul>{preview.changes.map((change) => <li key={change.workloadId}><strong>{change.workloadId}</strong> · {change.action.toLowerCase()} · {change.resources.new.length} new resource(s)</li>)}</ul> : <p>{preview.routePending ? 'Public routes need reconciliation; workloads will not restart.' : 'No workload changes to deploy.'}</p>}<Button tone="primary" disabled={deploying || (preview.changes.length === 0 && !preview.routePending)} onClick={() => void deployPreview()}>{deploying ? 'Deploying…' : preview.routePending && preview.changes.length === 0 ? 'Retry public routes' : 'Deploy these changes'}</Button></div> : null}
      {deployReport ? <div className="content-panel" aria-label="Deployment result"><h3>Deploy {deployReport.status.toLowerCase()}</h3><ul>{deployReport.results.map((result) => <li key={result.workloadId}>{result.workloadId}: {result.status.toLowerCase()}{result.error ? ` — ${result.error}` : ''}</li>)}</ul>{deployReport.status !== 'SUCCEEDED' ? <p>Preview again to retry workloads that did not finish.</p> : null}</div> : null}
    </section>
    <RecentDeployments applicationId={application.id} environment={environment} refreshKey={deployReport?.results[0]?.deploymentId} />
  </section>;
}
