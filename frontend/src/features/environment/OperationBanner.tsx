import { duration, stepLabel, RecoveryChecks } from "../deployments/ExecutionProgress";
import { navigate } from "../../app/routes";
import { useEffect, useRef, useState } from 'react';
import { ApiError } from '../../shared/api/client';
import type { Application, EnvironmentKey, EnvironmentTarget, EnvironmentOperation } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { statusLabel } from './labels';
import { fetchEnvironment, fetchOperationEnvironment, recoverOperation, listOperations, type RecoveryResult } from './api';
import { t } from '../../shared/i18n';

// Shows the operation that owns the Environment, keeps it fresh while it runs
// and offers explicit recovery of an interrupted one. Recovery is never
// automatic: the user must confirm the interrupted work has stopped.
export function OperationBanner({ application, environment, onTargetChange, pollMs = 3000 }: {
  application: Application; environment: EnvironmentKey;
  onTargetChange(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void; pollMs?: number;
}) {
  const operation = application.environments[environment].activeOperation;
 const steps=operation?.progress?.steps ?? [];
 const active=[...steps].reverse().find((s)=>s.status==='RUNNING') ?? steps.at(-1);
 const warning=active?.pods?.find((p)=>!['Running','Completed'].includes(p.reason));
 const deploymentId=[...steps].reverse().find((s)=>s.deploymentId)?.deploymentId;

  const [history, setHistory] = useState<EnvironmentOperation>();
 const [confirmed, setConfirmed] = useState(false);
  const [recovering, setRecovering] = useState(false);
  const [result, setResult] = useState<RecoveryResult>();
  const [error, setError] = useState('');
  // scope changes only when the Application/Environment changes or the banner unmounts;
  // recovery attempts never touch it, so polling keeps working after a failed recovery.
  const scope = useRef(0);
  useEffect(() => { scope.current += 1; setHistory(undefined); setConfirmed(false); setResult(undefined); setError(''); setRecovering(false); }, [application.id, environment]);
  useEffect(() => () => { scope.current += 1; }, []);
  useEffect(() => {
    if (!operation) return;
    const current = scope.current;
    const timer = window.setInterval(() => {
      fetchOperationEnvironment(application.id, environment).then((latest) => { if (latest && current === scope.current) onTargetChange(application.id, environment, latest); }).catch(() => undefined);
    }, pollMs);
    return () => window.clearInterval(timer);
  }, [operation?.id, application.id, environment, pollMs]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    let cancelled=false;
    void listOperations(application.id,environment).then(({operations})=>{if (!cancelled) setHistory(operations.find((op)=>Boolean(op.progress?.checks?.length)));}).catch(()=>undefined);
    return ()=>{cancelled=true;};
  },[application.id,environment,operation?.status,recovering]);

  async function recover() {
    if (!operation || !confirmed || recovering) return;
    const current = scope.current;
    setRecovering(true); setError('');
    try {
      const response = await recoverOperation(application.id, environment, operation.id);
      if (current !== scope.current) return;
      setResult(response.recovery);
      const latest = await fetchEnvironment(application.id, environment);
      if (current !== scope.current) return;
      if (latest) onTargetChange(application.id, environment, latest);
    } catch (err) {
      if (current !== scope.current) return;
      setError(err instanceof ApiError ? err.message : t('environment.operationBanner.noResponseFromTheServer'));
    } finally {
      if (current === scope.current) setRecovering(false);
    }
  }

  const recovery=operation?.progress?.checks?.length ? operation : !operation ? history : undefined;
  if (!operation && !result && !recovery) return null;
  return <section className="content-panel operation-banner" aria-label={t('environment.operationBanner.environmentOperation')}>
    {operation ? <>
      <div className="section-header"><div><h2>{operation.status === 'INTERRUPTED' ? t('environment.operationBanner.operationInterrupted') : t('environment.operationBanner.operationRunning')}</h2><p>{t('environment.operationBanner.toLowerCaseStatusLabelValueSettingsConfiguration', { toLowerCase: operation.kind.replace('_', ' ').toLowerCase(), statusLabel: statusLabel(operation.status), value: operation.stage ? ` · ${operation.stage}` : '' })}</p></div><span className="pending-pill">{statusLabel(operation.status)}</span></div>
      <p>{t('execution.elapsed', { time: duration(operation.startedAt) })}</p>
      {active ? <p>{t('execution.current', { step: steps.indexOf(active)+1, total: steps.length, workload: active.workload === '_environment' ? environment : active.workload ?? '', label: stepLabel(active.kind) })}</p> : null}
      {warning ? <p className="form-warning">{warning.name}: {warning.reason} {warning.image}</p> : null}
      {deploymentId ? <Button onClick={()=>navigate({name:'deployment',applicationId:application.id,environment,deploymentId})}>{t('execution.details')}</Button> : null}
      {operation.status === 'INTERRUPTED' ? <div className="recovery">
        <p>{t('environment.operationBanner.theProcessThatOwnsThis')}</p><p>{t('execution.heartbeat', { time: new Date(operation.progress?.priorHeartbeat ?? operation.heartbeatAt).toLocaleString() })}</p>
        <label className="checkbox"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} /> {' '}{t('environment.operationBanner.iConfirmTheInterruptedOperation')}</label>
        <div className="form-actions"><Button tone="primary" disabled={!confirmed || recovering} onClick={() => void recover()}>{recovering ? t('environment.operationBanner.recovering') : t('environment.operationBanner.recoverEnvironment')}</Button></div>
      </div> : null}
    </> : null}
    {recovery?.progress?.checks?.length ? <><p role="status">{t(recovery.status === 'RECOVERED' ? 'execution.recoveryComplete' : recovery.status === 'RECOVERING' ? 'execution.recoveryChecking' : 'execution.recoveryIncomplete')}</p><RecoveryChecks record={recovery.progress} /></> : null}
 {error ? <div className="form-error" role="alert">{error}</div> : null}
    {result && !recovery ? <div className="form-info" role="status"><strong>{t('environment.operationBanner.recoveryResult')}</strong> {result.compensationFailed?.length ? t('environment.operationBanner.recoveryIncompleteCountStepsStill', { count: result.compensationFailed.length }) : t('environment.operationBanner.recoveryCompleted')}</div> : null}
  </section>;
}
