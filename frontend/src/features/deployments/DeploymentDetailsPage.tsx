import { useEffect, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { DependencyGraph } from '../../shared/ui/DependencyGraph';
import { getDeployment, type DeploymentDetail } from './api';
import { ApiError } from '../../shared/api/client';
import { DeploymentReview } from '../preview/DeploymentReview';
import { intlLocale, t } from '../../shared/i18n';

const actions: Record<string, string> = { DEPLOY: t('deployments.deploymentDetails.deploy'), UPDATE: t('deployments.deploymentDetails.update'), REMOVE: t('deployments.deploymentDetails.remove') };
const deploymentStatuses: Record<string, string> = { SUCCEEDED: t('deployments.deploymentDetails.succeeded'), FAILED: t('deployments.deploymentDetails.failed'), DEPLOYING: t('deployments.deploymentDetails.deploying'), PROVISIONING: t('deployments.deploymentDetails.provisioningResources'), PLANNING: t('deployments.deploymentDetails.planning') };
const itemStatuses: Record<string, string> = { READY: t('deployments.deploymentDetails.ready'), PENDING: t('deployments.deploymentDetails.pending'), FAILED: t('deployments.deploymentDetails.failed'), SUCCEEDED: t('deployments.deploymentDetails.succeeded'), RUNNING: t('deployments.deploymentDetails.running'), INTERRUPTED: t('deployments.deploymentDetails.interrupted'), ACTIVE: t('deployments.deploymentDetails.active'), PROVISIONING: t('deployments.deploymentDetails.provisioningResources'), DEPLOYING: t('deployments.deploymentDetails.deploying'), PLANNING: t('deployments.deploymentDetails.planning'), UNCONFIGURED: t('deployments.deploymentDetails.notConfigured'), UNREFERENCED: t('deployments.deploymentDetails.unreferenced'), APPLYING: t('deployments.deploymentDetails.applying'), REMOVING: t('deployments.deploymentDetails.removing'), REMOVED: t('deployments.deploymentDetails.removed') };
// Own-key lookup: inherited keys such as "constructor" must never resolve.
const lookup = (map: Record<string, string>, key: string) => (Object.hasOwn(map, key) ? map[key] ?? key : key);
// Progress values come from persisted strings: only known codes are shown, anything else is a generic label.
const known = (map: Record<string, string>, key: string, generic: string) => (Object.hasOwn(map, key) ? map[key] ?? generic : generic);
const redactedMarker = '***redacted***';
const stageLabels: Record<string, string> = {
  INTENT: t('deployments.deploymentDetails.intentRecorded'), INIT: t('deployments.deploymentDetails.moduleInitialization'), PLANNING: t('deployments.deploymentDetails.terraformPlanning'), PLANNED: t('deployments.deploymentDetails.planned'), APPLYING: t('deployments.deploymentDetails.applying'), APPLIED: t('deployments.deploymentDetails.applied'), OUTPUTS: t('deployments.deploymentDetails.readingOutputs'),
  READINESS: t('deployments.deploymentDetails.readinessCheck'), READY: t('deployments.deploymentDetails.ready'), NO_CHANGE: t('deployments.deploymentDetails.noChanges'), CHECKING: t('deployments.deploymentDetails.checking'), DESTROYING: t('deployments.deploymentDetails.destroying'), VERIFYING: t('deployments.deploymentDetails.verifying'), DESTROYED: t('deployments.deploymentDetails.destroyed'), RECOVERY: t('deployments.deploymentDetails.recovering'),
};
const scopeLabels: Record<string, string> = { APPLICATION: t('deployments.deploymentDetails.application'), ENVIRONMENT: t('deployments.deploymentDetails.environment'), WORKLOAD: 'Workload', SHARED: t('deployments.deploymentDetails.shared') };
const actionLabels: Record<string, string> = { CREATE: t('deployments.deploymentDetails.create'), UPDATE: t('deployments.deploymentDetails.update2'), DESTROY: t('deployments.deploymentDetails.destroy') };
const ownershipLabels: Record<string, string> = { CREATED_BY_RUN: t('deployments.deploymentDetails.createdByThisRun'), PREEXISTING: t('deployments.deploymentDetails.preexisting'), UPDATED_BY_RUN: t('deployments.deploymentDetails.updatedByThisRun') };
const attemptStatuses: Record<string, string> = { RUNNING: t('deployments.deploymentDetails.running'), SUCCEEDED: t('deployments.deploymentDetails.succeeded'), FAILED: t('deployments.deploymentDetails.failed'), INTERRUPTED: t('deployments.deploymentDetails.interrupted'), NEEDS_ATTENTION: t('deployments.deploymentDetails.needsAttention') };
const errorLabels: Record<string, string> = {
  DESTRUCTIVE_PLAN: t('deployments.deploymentDetails.planWouldDestroyResourcesSo'), OUTPUT_CONTRACT: t('deployments.deploymentDetails.moduleOutputsDoNotMatch'), NOT_READY: t('deployments.deploymentDetails.resourceNotReady'), READINESS_UNAVAILABLE: t('deployments.deploymentDetails.readinessCouldNotBeChecked'),
  OUTCOME_UNKNOWN: t('deployments.deploymentDetails.outcomeUnknownCheckBeforeRetrying'), STATE_NOT_ACKNOWLEDGED: t('deployments.deploymentDetails.stateNotConfirmedAsSaved'), STATE_BACKEND: t('deployments.deploymentDetails.stateStoreError'), TRANSIENT: t('deployments.deploymentDetails.transientError'), AUTH: t('deployments.deploymentDetails.providerAuthenticationError'),
  INPUT: t('deployments.deploymentDetails.invalidInput'), TIMEOUT: t('deployments.deploymentDetails.timedOut'), CANCELLED: t('deployments.deploymentDetails.destroyed'), FAILED: t('deployments.deploymentDetails.failed'), PHYSICAL_RESOURCE_REMAINS: t('deployments.deploymentDetails.physicalResourcesStillExist'), SECRET_UNAVAILABLE: t('deployments.deploymentDetails.couldNotReadARequired'),
};

const date = (value?: string) => {
  if (!value) return '—';
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? '—' : parsed.toLocaleString(intlLocale());
};

// Strings and numbers are shown as persisted; the backend marker never becomes data and
// nested values are never dumped, because they may carry secrets.
const outputValue = (value: unknown) => {
  if (value === redactedMarker) return t('deployments.deploymentDetails.masked');
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value);
  return t('deployments.deploymentDetails.valueHidden');
};

export function DeploymentDetailsPage({ application, environment, deploymentId }: { application: Application; environment: EnvironmentKey; deploymentId: string }) {
  const [detail, setDetail] = useState<DeploymentDetail>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [stale, setStale] = useState(false);
  const [notFound, setNotFound] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let cancelled = false; let timer: number | undefined; let hadDetail = false;
    setLoading(true); setError(''); setStale(false); setNotFound(false); setDetail(undefined);
    const refresh = async () => {
      let terminal = false;
      try {
        const value = await getDeployment(application.id, environment, deploymentId);
        if (cancelled) return;
        hadDetail = true; setDetail(value); setStale(false); setError('');
        terminal = value.deployment.status === 'SUCCEEDED' || value.deployment.status === 'FAILED';
      } catch (err) {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 404 && !hadDetail) { setNotFound(true); terminal = true; }
        else if (hadDetail) setStale(true);
        else setError(t('deployments.deploymentDetails.couldNotLoadTheDeployment'));
      } finally { if (!cancelled) setLoading(false); }
      if (!cancelled && !terminal) timer = window.setTimeout(() => void refresh(), 2000);
    };
    void refresh();
    return () => { cancelled = true; if (timer) window.clearTimeout(timer); };
  }, [application.id, environment, deploymentId, attempt]);
  const reload = () => setAttempt((value) => value + 1);
  const failed = detail && (detail.deployment.status === 'FAILED' || Boolean(detail.deployment.failureReason));
  const rendered = detail?.workloads.flatMap((w) => w.manifestReview?.version === 1 && w.manifestReview.planHash === detail.planHash ? w.manifestReview.artifacts : [] ) ?? [];
  const review = detail?.review ? { ...detail.review, artifacts: [...detail.review.artifacts.filter((artifact) => !rendered.some((actual) => actual.id === artifact.id)), ...rendered] } : undefined;
  return <section className="page deployment-details-page">
    <button className="back-link" onClick={() => navigate({ name: 'deployments', applicationId: application.id, environment })}>{t('deployments.deploymentDetails.deploymentHistoryEnvironment', { environment })}</button>
    <header className="page-header"><div><p className="eyebrow">{application.name} · {environment}</p><h1>{t('deployments.deploymentDetails.deploymentDetails')}</h1><p>{t('deployments.deploymentDetails.savedPlanAndExecutionState')}</p></div><Button onClick={reload}>{t('deployments.deploymentDetails.reload')}</Button></header>
    {loading ? <p role="status" aria-busy="true">{t('deployments.deploymentDetails.loadingDeploymentDetails')}</p> : notFound ? <section className="content-panel"><h2>{t('deployments.deploymentDetails.deploymentNotFound')}</h2><p>{t('deployments.deploymentDetails.thisDeploymentDoesNotExist')}</p></section> : error && !detail ? <div role="alert" className="form-error">{error} <Button onClick={reload}>{t('deployments.deploymentDetails.retry')}</Button></div> : detail ? <>
      {stale ? <div className="form-error" role="alert">{t('deployments.deploymentDetails.statusShownIsTheLast')}{' '} <Button onClick={reload}>{t('deployments.deploymentDetails.retry')}</Button></div> : null}
      <section className="content-panel"><p>{detail.workloads.some((w) => w.status === 'READY') ? t('deployments.deploymentDetails.manifestsOfReadyWorkloadsWere') : t('deployments.deploymentDetails.manifestsShownAreRenderedIntent')}</p><DeploymentReview review={review} progress={detail.nodeProgress} title={t('deployments.deploymentDetails.deploymentMapAndStatus')} /></section>
      <section className="content-panel"><div className="section-header"><div><h2>{detail.deployment.workloadId} · {lookup(actions, detail.deployment.action)}</h2><p>{detail.deployment.id}</p></div><Status tone={detail.deployment.status === 'SUCCEEDED' ? 'good' : detail.deployment.status === 'FAILED' ? 'draft' : 'neutral'}>{lookup(deploymentStatuses, detail.deployment.status)}</Status></div>
        {failed ? <div className="form-error" role="alert">{t('deployments.deploymentDetails.theDeploymentDidNotComplete')}</div> : null}
        <dl className="detail-grid"><div><dt>{t('deployments.deploymentDetails.started')}</dt><dd>{date(detail.deployment.startedAt)}</dd></div><div><dt>{t('deployments.deploymentDetails.finished')}</dt><dd>{date(detail.deployment.finishedAt)}</dd></div><div><dt>{t('deployments.deploymentDetails.performedBy')}</dt><dd>{detail.deployment.actorRef || t('deployments.deploymentDetails.unknownActor')}</dd></div><div><dt>{t('deployments.deploymentDetails.deploymentSet')}</dt><dd>{detail.deploymentSetId || '—'}</dd></div></dl></section>
      <section className="content-panel"><h2>Workload</h2>{detail.workloads?.length ? <div className="detail-list">{detail.workloads.map((item) => <div key={item.workloadId}><strong>{item.workloadId}</strong><span>{lookup(itemStatuses, item.status)}</span><small>{item.manifestDigest}</small></div>)}</div> : <p>{t('deployments.deploymentDetails.noWorkloadStatusRecordedYet')}</p>}</section>
      <section className="content-panel"><h2>{t('deployments.deploymentDetails.resources')}</h2>{detail.resources?.length ? <div className="detail-list">{detail.resources.map((item) => <div key={item.descriptor}><strong>{item.resourceType || item.descriptor}</strong><span>{lookup(itemStatuses, item.status)} · {item.definitionKey || t('deployments.deploymentDetails.noResourceDefinition')}</span><small>{item.descriptor}</small>{Object.keys(item.outputs ?? {}).length ? <small>{t('deployments.deploymentDetails.outputsValue', { value: Object.entries(item.outputs).map(([key, value]) => `${key}=${outputValue(value)}`).join(', ') })}</small> : null}</div>)}</div> : <p>{t('deployments.deploymentDetails.noResourceStatusRecordedYet')}</p>}</section>
      <section className="content-panel" aria-label={t('deployments.deploymentDetails.resourceProvisioningProgress')}><h2>{t('deployments.deploymentDetails.resourceProvisioningProgress')}</h2>{detail.resourceProgress === undefined
        ? <p className="form-warning" role="note">{t('deployments.deploymentDetails.theSavedExecutionProgressOf')}</p>
        : detail.resourceProgress.length === 0 ? <p>{t('deployments.deploymentDetails.noTerraformExecutionStepWas')}</p>
          : <table className="plain-table"><thead><tr><th>{t('deployments.deploymentDetails.resources')}</th><th>Module</th><th>{t('deployments.deploymentDetails.attempt')}</th><th>{t('deployments.deploymentDetails.action')}</th><th>{t('deployments.deploymentDetails.phase')}</th><th>{t('deployments.deploymentDetails.status')}</th><th>{t('deployments.deploymentDetails.ownership')}</th><th>{t('deployments.deploymentDetails.update2')}</th></tr></thead><tbody>{detail.resourceProgress.map((item) => <tr key={`${item.descriptor}|${item.scopeType}|${item.attempt}|${item.startedAt}`}>
            <td>{item.descriptor}<br /><small>{known(scopeLabels, item.scopeType, t('deployments.deploymentDetails.unknownScope'))}</small>{item.resourceInstanceId ? <><br /><a href="#" onClick={(event) => { event.preventDefault(); navigate({ name: 'resource', applicationId: application.id, environment, resourceId: item.resourceInstanceId as string }); }}>{t('deployments.deploymentDetails.resourceDetails')}</a></> : null}</td><td>{item.moduleId} · {item.moduleRevision}</td><td>{item.attempt}</td><td>{known(actionLabels, item.action, t('deployments.deploymentDetails.unknownAction'))}</td><td>{known(stageLabels, item.stage, t('deployments.deploymentDetails.unknownPhase'))}</td>
            <td>{known(attemptStatuses, item.status, t('deployments.deploymentDetails.unknownStatus'))}{item.safeErrorCode ? <><br /><small>{known(errorLabels, item.safeErrorCode, t('deployments.deploymentDetails.unknownError'))}</small></> : null}</td><td>{known(ownershipLabels, item.ownership, t('deployments.deploymentDetails.unknownOwnership'))}</td><td>{date(item.updatedAt)}</td></tr>)}</tbody></table>}
        <p><small>{t('deployments.deploymentDetails.onlyTheSavedPhaseStatus')}</small></p></section>
      <section className="content-panel"><h2>{t('deployments.deploymentDetails.resourceProvisioningPlan')}</h2>{!detail.planHash ? <p>{t('deployments.deploymentDetails.noResourceProvisioningPlanWas')}</p> : <><p>{t('deployments.deploymentDetails.planSlice', { slice: detail.planHash.slice(0, 12) })}</p>
        <DependencyGraph nodes={detail.graph?.nodes ?? []} edges={detail.graph?.edges ?? []} blocked={detail.blocked ?? []} definitionOf={(descriptor) => detail.matches?.[descriptor]?.definitionKey} />
        <h3>{t('deployments.deploymentDetails.resourceProvisioningWaves')}</h3>{detail.batches?.length ? <ol>{detail.batches.map((batch, index) => <li key={index}>{batch.join(', ')}</li>)}</ol> : <p>{t('deployments.deploymentDetails.noResourceProvisioningWaveRecorded')}</p>}</>}</section>
    </> : null}
  </section>;
}
