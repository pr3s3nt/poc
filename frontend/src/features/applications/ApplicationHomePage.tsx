import { useEffect, useRef, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { endpointFor } from '../../shared/types/application';
import { ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { deleteWorkload, deployChanges, getWorkloads, previewChanges, undoWorkloadDelete, type DeployReport, type PendingPreview, type WorkloadList } from '../workloads/api';
import { RecentDeployments } from '../deployments/RecentDeployments';

// One mutation or read that must not overlap another (UC-07 UI states).
type Busy = '' | 'preview' | 'deploy' | `delete:${string}` | `undo:${string}`;

export function ApplicationHomePage({ application, created = false }: { application: Application; created?: boolean }) {
  const [environment, setEnvironment] = useState<EnvironmentKey>('staging');
  const [data, setData] = useState<WorkloadList>();
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [loading, setLoading] = useState(true);
  const [preview, setPreview] = useState<PendingPreview>();
  const [busy, setBusy] = useState<Busy>('');
  const [deployReport, setDeployReport] = useState<DeployReport>();
  const [reload, setReload] = useState(0);
  // Bumped on every Environment switch: a response started for an earlier
  // scope is ignored instead of being shown under the new Environment.
  const scope = useRef(0);

  useEffect(() => {
    const current = ++scope.current;
    setData(undefined); setPreview(undefined); setDeployReport(undefined); setLoading(true); setError(''); setNotice(''); setBusy('');
    getWorkloads(application.id, environment)
      .then((result) => { if (current === scope.current) setData(result); })
      .catch((err: Error) => { if (current === scope.current) setError(err.message); })
      .finally(() => { if (current === scope.current) setLoading(false); });
  }, [application.id, environment]);

  // Runs one guarded request for the current scope. A 409 explains the stale
  // state and reloads the list; the mutation is never retried silently.
  // `apply` may reload the list; controls stay busy until it finishes, so no
  // mutation can run against the pre-change list or race the reload.
  async function run<T>(kind: Busy, request: () => Promise<T>, apply: (value: T) => void | Promise<void>) {
    const current = scope.current;
    setBusy(kind); setError(''); setNotice('');
    try {
      const value = await request();
      if (current === scope.current) await apply(value);
    } catch (err) {
      if (current !== scope.current) return;
      setPreview(undefined);
      if (err instanceof ApiError && err.status === 409) {
        const reloaded = await reloadList(current);
        if (reloaded && current === scope.current) setNotice(`Workloads in ${environment} changed since this page loaded or was previewed. The list was reloaded; review it and preview again.`);
      } else {
        setError((err as Error).message);
      }
    } finally {
      if (current === scope.current) setBusy('');
    }
  }

  // Reloads the list after a mutation outcome. On failure the stale list is
  // dropped, so no further mutation uses an old version, and Retry reloads.
  async function reloadList(current: number): Promise<boolean> {
    try {
      const fresh = await getWorkloads(application.id, environment);
      if (current === scope.current) { setData(fresh); setError(''); setLoading(false); }
      return true;
    } catch {
      if (current === scope.current) { setData(undefined); setError(`The workload list for ${environment} could not be reloaded. Retry before making more changes.`); }
      return false;
    }
  }

  function changeDeletion(id: string, undo: boolean) {
    if (!data) return;
    if (!undo && !window.confirm(`Mark ${id} for deletion in ${environment}? Running workloads will not change until Deploy.`)) return;
    const version = data.draftVersion;
    setDeployReport(undefined);
    void run(undo ? `undo:${id}` : `delete:${id}`, () => undo ? undoWorkloadDelete(application.id, environment, id, version) : deleteWorkload(application.id, environment, id, version), (list) => { setPreview(undefined); setData(list); });
  }
  function loadPreview() {
    setPreview(undefined); setDeployReport(undefined);
    void run('preview', () => previewChanges(application.id, environment), setPreview);
  }
  function deployPreview() {
    if (!preview || (preview.changes.length === 0 && !preview.routePending)) return;
    const token = preview.token;
    const current = scope.current;
    void run('deploy', () => deployChanges(application.id, environment, token), async (report) => {
      // The outcome is shown at once; the list reload is a separate guarded
      // step whose failure never hides the outcome.
      setDeployReport(report); setPreview(undefined); setReload((value) => value + 1);
      await reloadList(current);
    });
  }

  const workloads = data?.workloads ?? [];
  const locked = busy !== '' || loading || !data;
  return <section className="page application-home"><button className="back-link" disabled={busy === 'deploy'} onClick={() => navigate({ name: 'applications' })}>← Applications</button>
    <header className="page-header application-header"><div><p className="eyebrow">Application</p><h1>{application.name}</h1><p>{endpointFor(application, 'production')}</p></div><Button disabled={busy === 'deploy'} onClick={() => navigate({ name: 'settings', applicationId: application.id })}>Variables &amp; Secrets</Button></header>
    {created ? <div className="form-success" role="status">Application created. Staging and production are ready; nothing has been deployed yet.</div> : null}
    <div className="tabs" role="tablist"><button className={environment === 'staging' ? 'tab tab-active' : 'tab'} disabled={busy === 'deploy'} onClick={() => setEnvironment('staging')}>Staging<span>{endpointFor(application, 'staging')}</span></button><button className={environment === 'production' ? 'tab tab-active' : 'tab'} disabled={busy === 'deploy'} onClick={() => setEnvironment('production')}>Production<span>{endpointFor(application, 'production')}</span></button></div>
    <section className="content-panel"><div className="section-header"><div><h2>Workloads</h2><p>Configuration for {environment}; saving here does not deploy.</p></div><span><Button disabled={busy === 'deploy'} onClick={() => navigate({ name: 'score-preview', applicationId: application.id, environment })}>Preview Score</Button><Button disabled={locked} onClick={() => navigate({ name: 'workload', applicationId: application.id, environment })}>+ Add workload</Button></span></div>
      {error ? <div className="form-error" role="alert">{error}{!data && !loading ? <> <Button onClick={() => void reloadList(scope.current)}>Retry</Button></> : null}</div> : null}
      {notice ? <div className="form-info" role="status">{notice}</div> : null}
      {loading ? <p>Loading workloads…</p> : workloads.length ? <div className="workload-table"><div className="table-head"><span>Name</span><span>Status</span><span>Actions</span></div>{workloads.map((workload) => <div className="table-row" key={workload.id}><span className="workload-name"><span className="workload-icon">◫</span>{workload.id}</span><Status tone={workload.state ? 'draft' : 'good'}>{workload.state === 'PENDING_DELETE' ? 'Pending deletion' : workload.state === 'PENDING_UPSERT' ? 'Pending change' : 'Ready'}</Status><span>{workload.state === 'PENDING_DELETE' ? <Button tone="quiet" disabled={locked} onClick={() => changeDeletion(workload.id, true)}>{busy === `undo:${workload.id}` ? 'Restoring…' : 'Undo'}</Button> : <><Button tone="quiet" disabled={locked || !workload.score} onClick={() => navigate({ name: 'workload', applicationId: application.id, environment, workloadId: workload.id })}>Edit</Button><Button tone="danger" disabled={locked} onClick={() => changeDeletion(workload.id, false)}>{busy === `delete:${workload.id}` ? 'Marking…' : 'Delete'}</Button></>}</span></div>)}</div> : data ? <div className="section-empty">No workloads in this environment yet.</div> : null}
      <div className="form-actions"><Button tone="primary" disabled={locked} onClick={loadPreview}>{busy === 'preview' ? 'Calculating preview…' : 'Preview changes'}</Button></div>
      {preview ? <div className="content-panel" aria-label="Deployment preview"><h3>Preview for {environment}</h3><p>{preview.changes.length} workload(s) affected · draft v{preview.draftVersion} · configuration {preview.configRevisionId ? preview.configRevisionId.slice(0, 8) : 'empty'}</p>{preview.changes.length ? <ul>{preview.changes.map((change) => <li key={change.workloadId}><strong>{change.workloadId}</strong> · {change.action.toLowerCase()} · {change.resources.new.length} new resource(s){change.resources.unreferenced.length ? ` · ${change.resources.unreferenced.length} resource(s) will become unreferenced (not destroyed)` : ''}</li>)}</ul> : <p>{preview.routePending ? 'Public routes need reconciliation; workloads will not restart.' : 'No workload changes to deploy.'}</p>}<Button tone="primary" disabled={busy !== '' || (preview.changes.length === 0 && !preview.routePending)} onClick={deployPreview}>{busy === 'deploy' ? 'Deploying…' : preview.routePending && preview.changes.length === 0 ? 'Retry public routes' : 'Deploy these changes'}</Button></div> : null}
      {deployReport ? <div className="content-panel" aria-label="Deployment result"><h3>Deploy {deployReport.status.toLowerCase()}</h3><ul>{deployReport.results.map((result) => <li key={result.workloadId}>{result.workloadId} · {result.action.toLowerCase()}: {result.status.toLowerCase()}{result.deploymentId ? <> · <a href="#" onClick={(event) => { event.preventDefault(); navigate({ name: 'deployment', applicationId: application.id, environment, deploymentId: result.deploymentId as string }); }}>view deployment</a></> : null}{result.error ? ` — ${result.error}` : ''}</li>)}</ul>{deployReport.status !== 'SUCCEEDED' ? <p>Unfinished changes stay pending. Preview again to retry them; nothing is rolled back automatically.</p> : null}</div> : null}
    </section>
    <RecentDeployments applicationId={application.id} environment={environment} refreshKey={`${reload}`} />
  </section>;
}
