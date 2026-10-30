import { OverviewPanel } from '../observability/OverviewPanel';
import { ReconciliationPanel } from '../delivery/ReconciliationPanel';
import { InfrastructureDriftPanel } from '../delivery/InfrastructureDriftPanel';
import { useEffect, useRef, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { endpointFor } from '../../shared/types/application';
import { ApiError, isBusy, isStaleConflict } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { ApplicationTarget } from '../../shared/ui/ApplicationTarget';
import { ResourceIntents, rendererText } from '../preview/ResourceIntents';
import { Placements } from '../preview/Placements';
import { deleteWorkload, deployChanges, getWorkloads, previewChanges, undoWorkloadDelete, type DeployReport, type PendingPreview, type WorkloadList } from '../workloads/api';
import { useEnvironmentBusy } from '../environment/useEnvironmentBusy';
import { RecentDeployments } from '../deployments/RecentDeployments';
import { EnvironmentResources } from '../resources/EnvironmentResources';
import { correlateReviewProgress } from '../preview/reviewObservation';
import { newReviewRequestId } from '../preview/reviewRequestId';
import { DeploymentReview, type DeploymentReviewData } from '../preview/DeploymentReview';
import { getDeployment, listDeployments, listReviewOperations, type DeploymentDetail, type ReviewOperation } from '../deployments/api';
import { t } from '../../shared/i18n';

const actions: Record<string, string> = { DEPLOY: t('applications.applicationHome.deploy'), UPDATE: t('applications.applicationHome.update'), REMOVE: t('applications.applicationHome.remove') };
const reports: Record<string, string> = { SUCCEEDED: t('applications.applicationHome.succeeded'), PARTIAL: t('applications.applicationHome.partial'), FAILED: t('applications.applicationHome.failed') };
const results: Record<string, string> = { SUCCEEDED: t('applications.applicationHome.succeeded'), FAILED: t('applications.applicationHome.failed'), SKIPPED: t('applications.applicationHome.skipped') };
// Own-key lookup: inherited keys such as "constructor" must never resolve.
const lookup = (map: Record<string, string>, key: string) => (Object.hasOwn(map, key) ? map[key] ?? key : key);
const actionLabel = (action: string) => lookup(actions, action);
const reportLabel = (status: string) => lookup(reports, status);
const resultLabel = (status: string) => lookup(results, status);

// Raw API/provider errors may carry values that must not be shown; every
// failure uses a fixed Vietnamese message chosen by the attempted action.
function failureMessage(kind: Busy): string {
  if (kind === 'preview') return t('applications.applicationHome.couldNotPreviewTheChanges');
  if (kind === 'deploy') return t('applications.applicationHome.couldNotDeployPreviewAgain');
  if (kind.startsWith('undo:')) return t('applications.applicationHome.couldNotUndoTheDeletion');
  return t('applications.applicationHome.couldNotMarkTheWorkload');
}

// One mutation or read that must not overlap another (UC-07 UI states).
type Busy = '' | 'preview' | 'deploy' | `delete:${string}` | `undo:${string}`;

export function ApplicationHomePage({ application, initialEnvironment = 'staging', created = false, onTargetChange, canManageInfrastructure = false }: { application: Application; initialEnvironment?: EnvironmentKey; created?: boolean; canManageInfrastructure?: boolean; onTargetChange?(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void }) {
  const [environment, setEnvironment] = useState<EnvironmentKey>(initialEnvironment);
  // Draft Delete/Undo are Environment writes: held while an operation owns it.
  const environmentBusy = useEnvironmentBusy(application, environment, onTargetChange);
  const [data, setData] = useState<WorkloadList>();
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [loading, setLoading] = useState(true);
  const [preview, setPreview] = useState<PendingPreview>();
  const [busy, setBusy] = useState<Busy>('');
  const [deployReport, setDeployReport] = useState<DeployReport>();
  const [reviewRequestId, setReviewRequestId] = useState('');
  const [activeReview, setActiveReview] = useState<DeploymentReviewData>();
  const [observed, setObserved] = useState<DeploymentDetail[]>([]);
  const [reviewOperation, setReviewOperation] = useState<ReviewOperation>();
  const [observationStale, setObservationStale] = useState(false);
  const [reload, setReload] = useState(0);
  // Bumped on every Environment switch: a response started for an earlier
  // scope is ignored instead of being shown under the new Environment.
  const scope = useRef(0);

  useEffect(() => {
    const current = ++scope.current;
    setData(undefined); setPreview(undefined); setDeployReport(undefined); setActiveReview(undefined); setReviewRequestId(''); setObserved([]); setReviewOperation(undefined); setObservationStale(false); setLoading(true); setError(''); setNotice(''); setBusy('');
    getWorkloads(application.id, environment)
      .then((result) => { if (current === scope.current) setData(result); })
      .catch(() => { if (current === scope.current) setError(t('applications.applicationHome.couldNotLoadTheWorkload', { environment })); })
      .finally(() => { if (current === scope.current) setLoading(false); });
  }, [application.id, environment]);

  useEffect(() => {
    if (!reviewRequestId) return;
    let cancelled = false; let timer: number | undefined;
    const refresh = async () => {
      let terminal = false;
      try {
        const [list, operations] = await Promise.all([listDeployments(application.id, environment, undefined, reviewRequestId), listReviewOperations(application.id, environment, reviewRequestId)]);
        const exact = await Promise.all(list.deployments.map((entry) => getDeployment(application.id, environment, entry.id)));
        if (cancelled) return;
        const operation = operations.operations.find((entry) => entry.reviewRequestId === reviewRequestId);
        setObserved(exact); setReviewOperation(operation); setObservationStale(false);
        terminal = Boolean(operation && ['SUCCEEDED', 'FAILED', 'RECOVERED'].includes(operation.status) && exact.every((entry) => ['SUCCEEDED', 'FAILED'].includes(entry.deployment.status)));
      } catch {
        if (cancelled) return;
        setObservationStale(true);
      }
      if (!cancelled && !terminal) timer = window.setTimeout(() => void refresh(), 2000);
    };
    void refresh();
    return () => { cancelled = true; if (timer) window.clearTimeout(timer); };
  }, [application.id, environment, reviewRequestId]);

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
      if (kind === 'deploy' && err instanceof ApiError && err.status >= 400 && err.status < 500) { setReviewRequestId(''); setActiveReview(undefined); }
      if (isBusy(err)) {
        setError((err as ApiError).message);
      } else if (isStaleConflict(err)) {
        const reloaded = await reloadList(current);
        if (reloaded && current === scope.current) setNotice(t('applications.applicationHome.workloadsInEnvironmentChangedSince', { environment }));
      } else {
        setError(err instanceof ApiError && err.code ? `${failureMessage(kind)} ${err.message}` : failureMessage(kind));
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
      if (current === scope.current) { setData(undefined); setError(t('applications.applicationHome.couldNotReloadTheWorkload', { environment })); }
      return false;
    }
  }

  function changeDeletion(id: string, undo: boolean) {
    if (!data || environmentBusy) return;
    if (!undo && !window.confirm(t('applications.applicationHome.markIdInEnvironmentFor', { id, environment }))) return;
    const version = data.draftVersion;
    setDeployReport(undefined);
    void run(undo ? `undo:${id}` : `delete:${id}`, () => undo ? undoWorkloadDelete(application.id, environment, id, version) : deleteWorkload(application.id, environment, id, version), (list) => { setPreview(undefined); setData(list); });
  }
  function loadPreview() {
    setReviewRequestId(''); setActiveReview(undefined); setObserved([]); setReviewOperation(undefined);
    setPreview(undefined); setDeployReport(undefined);
    void run('preview', () => previewChanges(application.id, environment), setPreview);
  }
  function deployPreview() {
    if (!preview || (preview.changes.length === 0 && !preview.routePending && !preview.deliveryPending)) return;
    const token = preview.token;
    const requestId = newReviewRequestId();
    setActiveReview(preview.review); setObserved([]); setReviewOperation(undefined); setReviewRequestId(requestId); setObservationStale(false);
    const current = scope.current;
    void run('deploy', () => deployChanges(application.id, environment, token, requestId), async (report) => {
      // The outcome is shown at once; the list reload is a separate guarded
      // step whose failure never hides the outcome.
      setDeployReport(report); setPreview(undefined); setReload((value) => value + 1);
      await reloadList(current);
    });
  }

  const workloads = data?.workloads ?? [];
  const progress = activeReview ? correlateReviewProgress(activeReview, observed, reviewOperation?.status) : undefined;
  const locked = busy !== '' || loading || !data || application.status === 'DELETING';
  return <section className="page application-home"><button className="back-link" disabled={busy === 'deploy'} onClick={() => navigate({ name: 'applications' })}>{t('applications.applicationHome.applications')}</button>
    {application.status === 'DELETING' ? <div className="form-error" role="status">{t('applications.deletion.deletingBanner')} <Button onClick={() => navigate({ name: 'settings', applicationId: application.id, environment })}>{t('applications.deletion.viewProgress')}</Button></div> : null}
    <header className="page-header application-header"><div><p className="eyebrow">{t('applications.applicationHome.application')}</p><h1>{application.name}</h1><p>{endpointFor(application, 'production')}</p><ApplicationTarget application={application} environment={environment} /></div><Button disabled={busy === 'deploy'} onClick={() => navigate({ name: 'settings', applicationId: application.id, environment })}>{t('applications.applicationHome.environmentSettings')}</Button></header>
    {created ? <div className="form-success" role="status">{t('applications.applicationHome.applicationCreatedTheDeploymentTarget')}</div> : null}
    <div className="tabs" role="tablist"><button className={environment === 'staging' ? 'tab tab-active' : 'tab'} disabled={busy === 'deploy'} onClick={() => setEnvironment('staging')}>staging<span>{endpointFor(application, 'staging')}</span></button><button className={environment === 'production' ? 'tab tab-active' : 'tab'} disabled={busy === 'deploy'} onClick={() => setEnvironment('production')}>production<span>{endpointFor(application, 'production')}</span></button></div>
    {application.environments[environment].reconciliationEnabled ? <ReconciliationPanel key={`${application.id}/${environment}`} applicationId={application.id} environment={environment} refreshKey={String(reload)} /> : null}
    {application.environments[environment].infrastructureDriftEnabled ? <InfrastructureDriftPanel key={`infrastructure/${application.id}/${environment}`} applicationId={application.id} environment={environment} canManage={canManageInfrastructure} busy={environmentBusy} refreshKey={String(reload)} /> : null}
    <OverviewPanel applicationId={application.id} environment={environment} refreshKey={String(reload)} />
    <section className="content-panel"><div className="section-header"><div><h2>Workload</h2><p>{t('applications.applicationHome.configurationOfEnvironmentSavingHere', { environment })}</p></div><span><Button disabled={busy === 'deploy'} onClick={() => navigate({ name: 'score-preview', applicationId: application.id, environment })}>{t('applications.applicationHome.previewScore')}</Button><Button disabled={locked} onClick={() => navigate({ name: 'workload', applicationId: application.id, environment })}>{t('applications.applicationHome.addWorkload')}</Button></span></div>
      {error ? <div className="form-error" role="alert">{error}{!data && !loading ? <> <Button onClick={() => void reloadList(scope.current)}>{t('applications.applicationHome.retry')}</Button></> : null}</div> : null}
      {environmentBusy ? <div className="form-info" role="status">{t('applications.applicationHome.anEnvironmentOperationIsRunning', { environment })}</div> : null}
      {notice ? <div className="form-info" role="status">{notice}</div> : null}
      {loading ? <p role="status" aria-busy="true">{t('applications.applicationHome.loadingWorkloads')}</p> : workloads.length ? <div className="workload-table"><div className="table-head"><span>{t('applications.applicationHome.name')}</span><span>{t('applications.applicationHome.status')}</span><span>{t('applications.applicationHome.actions')}</span></div>{workloads.map((workload) => <div className="table-row" key={workload.id}><span className="workload-name"><span className="workload-icon">◫</span>{workload.id}</span><Status tone={workload.state ? 'draft' : 'good'}>{workload.state === 'PENDING_DELETE' ? t('applications.applicationHome.pendingDeletion') : workload.state === 'PENDING_UPSERT' ? t('applications.applicationHome.pendingChange') : t('applications.applicationHome.ready')}</Status><span>{workload.state === 'PENDING_DELETE' ? <Button tone="quiet" disabled={locked || environmentBusy} onClick={() => changeDeletion(workload.id, true)}>{busy === `undo:${workload.id}` ? t('applications.applicationHome.undoing') : t('applications.applicationHome.undo')}</Button> : <><Button tone="quiet" disabled={locked || !workload.score} onClick={() => navigate({ name: 'workload', applicationId: application.id, environment, workloadId: workload.id })}>{t('applications.applicationHome.edit')}</Button><Button tone="danger" disabled={locked || environmentBusy} onClick={() => changeDeletion(workload.id, false)}>{busy === `delete:${workload.id}` ? t('applications.applicationHome.marking') : t('applications.applicationHome.delete')}</Button></>}</span></div>)}</div> : data ? <div className="section-empty">{t('applications.applicationHome.thisEnvironmentHasNoWorkloads')}</div> : null}
      <div className="form-actions"><Button tone="primary" disabled={locked} onClick={loadPreview}>{busy === 'preview' ? t('applications.applicationHome.computingPreview') : t('applications.applicationHome.previewChanges')}</Button></div>
    </section>
      {preview && !reviewRequestId ? <div className="content-panel preview-container" aria-label={t('applications.applicationHome.deploymentPreview')}><header className="review-section-heading"><div><h2>{t('applications.applicationHome.previewChanges')}</h2><ApplicationTarget application={application} environment={environment} /></div><span className="review-section-status">{t('applications.applicationHome.pendingDeployment')}</span></header><DeploymentReview review={preview.review} />{preview.deliveryPending ? <div className="form-info" role="status">{preview.deliveryPolicyOnly ? t('delivery.previewPolicy', { policy: preview.desiredDelivery?.selfHeal ? t('delivery.policyOn') : t('delivery.policyOff') }) : t('delivery.previewChange', { source: preview.appliedDelivery?.mode === 'CD' ? t('delivery.provider') : t('delivery.direct'), destination: preview.desiredDelivery?.mode === 'CD' ? t('delivery.provider') : t('delivery.direct') })}</div> : null}{preview.deliveryTransfer || preview.deliveryPolicyOnly ? <div className="form-info"><strong>{t('delivery.retainedInfrastructure')}</strong><ul>{preview.deliveryUnits?.map(unit => <li key={`${unit.workloadId}/${unit.phase}`}>{unit.workloadId === '_routes' ? t('reconciliation.routes') : unit.workloadId} · {t(`delivery.phase.${unit.phase}`)} · {t('delivery.objectCount', { count: unit.objects })}</li>)}</ul></div> : null}{preview.changes.length ? <details className="review-plan-details" open={preview.review ? undefined : true}><summary>{t('applications.applicationHome.technicalPlanDetails')}</summary><ul className="preview-changes">{preview.changes.map((change) => <li key={change.workloadId}><strong>{change.workloadId}</strong> · {actionLabel(change.action)} {' '}{t('applications.applicationHome.renderer')}{' '} {rendererText(change.rendering)} · {change.resources.new.length} {' '}{t('applications.applicationHome.newResources')}{change.resources.unreferenced.length ? t('applications.applicationHome.countResourcesWillNoLonger', { count: change.resources.unreferenced.length }) : ''}<Placements placements={change.placements} label={t('applications.applicationHome.deploymentTargetOfWorkloadId', { workloadId: change.workloadId })} /><ResourceIntents intents={change.resourceIntents} label={t('applications.applicationHome.resourcesOfWorkloadId', { workloadId: change.workloadId })} /></li>)}</ul></details> : <p>{preview.routePending ? t('applications.applicationHome.publicRoutesNeedToBe') : t('applications.applicationHome.noWorkloadChangesToDeploy')}</p>}<footer className="review-action-bar"><div><strong>{t('applications.applicationHome.readyForReview')}</strong><small>{t('applications.applicationHome.countWorkloadsAffectedReviewThe', { count: preview.changes.length })}</small></div><Button tone="primary" disabled={busy !== '' || (preview.changes.length === 0 && !preview.routePending && !preview.deliveryPending)} onClick={deployPreview}>{busy === 'deploy' ? t('applications.applicationHome.deploying') : preview.routePending && preview.changes.length === 0 ? t('applications.applicationHome.retryPublicRoutes') : t('applications.applicationHome.deployTheseChanges')}</Button></footer></div> : null}
      {reviewRequestId ? <div className="content-panel preview-container" aria-label={t('applications.applicationHome.trackDeployment')}><header className="review-section-heading"><div><h2>{t('applications.applicationHome.deploymentStatus')}</h2><p>{t('applications.applicationHome.environmentTrackTheReadinessOf', { environment })}</p></div><span className="review-section-status">{reviewOperation ? `${reviewOperation.status} · ${reviewOperation.stage || t('applications.applicationHome.waitingForRecord')}` : t('applications.applicationHome.waitingForTheExecutionRecord')}</span></header>{observationStale ? <div className="form-error" role="alert">{t('applications.applicationHome.couldNotRefreshTheStatus')}</div> : null}<DeploymentReview review={activeReview} progress={progress} title={t('applications.applicationHome.mapAndActualProgress')} />{busy === 'deploy' ? <footer className="review-action-bar"><div><strong>{t('applications.applicationHome.deploymentInProgress')}</strong><small>{t('applications.applicationHome.statusUpdatesPerComponent')}</small></div><Button tone="primary" disabled>{t('applications.applicationHome.deploying')}</Button></footer> : null}{observed.length ? <ul>{observed.map((item) => <li key={item.deployment.id}><a href="#" onClick={(event) => { event.preventDefault(); navigate({ name: 'deployment', applicationId: application.id, environment, deploymentId: item.deployment.id }); }}>{item.deployment.workloadId} · {item.deployment.status}</a></li>)}</ul> : <p>{t('applications.applicationHome.noDeploymentForThisRequest')}</p>}</div> : null}
      {deployReport ? <div className="content-panel" aria-label={t('applications.applicationHome.deploymentResult')}><h3>{t('applications.applicationHome.deploymentReportLabel', { reportLabel: reportLabel(deployReport.status) })}</h3><ApplicationTarget application={application} environment={environment} /><ul>{deployReport.results.map((result) => <li key={result.workloadId}>{result.workloadId} · {actionLabel(result.action)}: {resultLabel(result.status)}{result.deploymentId ? <> · <a href="#" onClick={(event) => { event.preventDefault(); navigate({ name: 'deployment', applicationId: application.id, environment, deploymentId: result.deploymentId as string }); }}>{t('applications.applicationHome.viewDeployment')}</a></> : null}{result.error ? t('applications.applicationHome.thisWorkloadCouldNotBe') : ''}</li>)}</ul>{deployReport.status !== 'SUCCEEDED' ? <p>{t('applications.applicationHome.incompleteChangesStayPendingPreview')}</p> : null}</div> : null}
    <EnvironmentResources applicationId={application.id} environment={environment} refreshKey={`${reload}`} />
    <RecentDeployments applicationId={application.id} environment={environment} refreshKey={`${reload}`} />
  </section>;
}
