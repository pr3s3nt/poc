import { useEffect, useRef, useState } from 'react';
import { ApiError } from '../../shared/api/client';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { fetchEnvironment, recoverOperation, type RecoveryResult } from './api';

// Shows the operation that owns the Environment, keeps it fresh while it runs
// and offers explicit recovery of an interrupted one. Recovery is never
// automatic: the user must confirm the interrupted work has stopped.
export function OperationBanner({ application, environment, onTargetChange, pollMs = 2000 }: {
  application: Application; environment: EnvironmentKey;
  onTargetChange(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void; pollMs?: number;
}) {
  const operation = application.environments[environment].activeOperation;
  const [confirmed, setConfirmed] = useState(false);
  const [recovering, setRecovering] = useState(false);
  const [result, setResult] = useState<RecoveryResult>();
  const [error, setError] = useState('');
  const generation = useRef(0);
  useEffect(() => { generation.current += 1; setConfirmed(false); setResult(undefined); setError(''); setRecovering(false); }, [application.id, environment]);
  useEffect(() => () => { generation.current += 1; }, []);
  useEffect(() => {
    if (!operation) return;
    const current = generation.current;
    const timer = window.setInterval(() => {
      fetchEnvironment(application.id, environment).then((latest) => { if (latest && current === generation.current) onTargetChange(application.id, environment, latest); }).catch(() => undefined);
    }, pollMs);
    return () => window.clearInterval(timer);
  }, [operation?.id, application.id, environment, pollMs]); // eslint-disable-line react-hooks/exhaustive-deps

  async function recover() {
    if (!operation || !confirmed || recovering) return;
    const current = ++generation.current;
    setRecovering(true); setError('');
    try {
      const response = await recoverOperation(application.id, environment, operation.id);
      if (current !== generation.current) return;
      setResult(response.recovery);
      const latest = await fetchEnvironment(application.id, environment);
      if (latest) onTargetChange(application.id, environment, latest);
    } catch (err) {
      if (current !== generation.current) return;
      setError(err instanceof ApiError ? err.message : 'Recovery could not be started. Try again.');
    } finally {
      if (current === generation.current) setRecovering(false);
    }
  }

  if (!operation && !result) return null;
  return <section className="content-panel operation-banner" aria-label="Environment operation">
    {operation ? <>
      <div className="section-header"><div><h2>{operation.status === 'INTERRUPTED' ? 'Operation interrupted' : 'Operation in progress'}</h2><p>{operation.kind.replace('_', ' ').toLowerCase()} · {operation.status.toLowerCase()}{operation.stage ? ` · ${operation.stage}` : ''}. Settings, configuration and draft changes are blocked until it ends.</p></div><span className="pending-pill">{operation.status}</span></div>
      {operation.status === 'INTERRUPTED' ? <div className="recovery">
        <p>The process that owned this operation stopped sending heartbeats. A stale heartbeat does not prove it stopped working: confirm that before recovery restores the source and releases the environment.</p>
        <label className="checkbox"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} /> I confirm the interrupted operation has stopped</label>
        <div className="form-actions"><Button tone="primary" disabled={!confirmed || recovering} onClick={() => void recover()}>{recovering ? 'Recovering…' : 'Recover environment'}</Button></div>
      </div> : null}
    </> : null}
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {result ? <div className="form-info" role="status"><strong>Recovery result:</strong> {result.outcome}{result.compensationFailed?.length ? <ul>{result.compensationFailed.map((item) => <li key={item}>Needs attention: {item}</li>)}</ul> : null}</div> : null}
  </section>;
}
