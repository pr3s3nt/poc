import { useEffect, useRef, useState } from 'react';
import { ApiError } from '../../shared/api/client';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { cleanupSource, executeTransition, fetchEnvironment, getTransition, listConnectionChoices, listTransitions, previewTransition, type Mode, type TransitionDetail, type TransitionPreview } from './api';

type Choice = { key: string; name: string; kind: string };
const stages = ['PREFLIGHT', 'QUIESCING', 'BACKUP', 'PROVISIONING', 'RESTORING', 'DEPLOYING', 'VERIFYING', 'CUTOVER'];

const actionLabel: Record<string, string> = {
  UNCHANGED: 'Redeployed unchanged',
  UPDATED: 'Redeployed with a pending draft change',
  ADDED: 'Added from a pending draft',
  REMOVED: 'Removed — absent at the destination (the source is not touched)',
};

// ImpactList shows what the destination will actually contain: each workload with
// its reviewed action and whether it picks up newer configuration. Removed
// workloads are never listed as redeployed.
function ImpactList({ workloads }: { workloads: TransitionPreview['workloads'] }) {
  return <>
    <h4>Workloads at the destination</h4>
    <ul aria-label="Workload impact">{workloads.map((item) => <li key={item.workloadId} data-action={item.action}><strong>{item.workloadId}</strong> — {actionLabel[item.action] ?? item.action}{item.configChanged && item.action !== 'REMOVED' ? ' · picks up newer configuration' : ''}</li>)}</ul>
    {workloads.some((item) => item.action === 'UPDATED' || item.action === 'ADDED' || item.action === 'REMOVED') ? <p className="warning-text">Pending draft changes marked above are applied by this transition and are consumed when it succeeds.</p> : null}
    {workloads.some((item) => item.configChanged) ? <p className="warning-text">The newest configuration revision is applied to the workloads marked above.</p> : null}
  </>;
}

// authorityText derives who is live from the persisted binding, not the status.
function authorityText(detail: TransitionDetail): string {
  if (detail.authority === 'DESTINATION') return 'The cutover had committed: the destination is live and authoritative.';
  if (detail.sourceState === 'NEEDS_ATTENTION') return 'The source generation is still the recorded target, but it was NOT restored. Operator action is needed.';
  return 'The source generation remains authoritative.';
}

// Explicit destination change of a deployed Environment (UC-01 ES-05, ADR-012):
// preview, acknowledged execution, persisted progress that survives a refresh,
// recovery results and explicit cleanup of the retained source generation.
export function TransitionPanel({ application, environment, initialDestination = '', onTargetChange, pollMs = 1500 }: {
  application: Application; environment: EnvironmentKey; initialDestination?: string;
  onTargetChange(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void; pollMs?: number;
}) {
  const target = application.environments[environment];
  const [choices, setChoices] = useState<readonly Choice[]>([]);
  const [destination, setDestination] = useState(initialDestination);
  const [mode, setMode] = useState<Mode>('MIGRATE_POSTGRES');
  const [preview, setPreview] = useState<TransitionPreview>();
  const [acknowledged, setAcknowledged] = useState(false);
  const [detail, setDetail] = useState<TransitionDetail>();
  const [history, setHistory] = useState<readonly TransitionDetail[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const generation = useRef(0);
  useEffect(() => () => { generation.current += 1; }, []);
  useEffect(() => { generation.current += 1; setPreview(undefined); setDetail(undefined); setError(''); setAcknowledged(false); setBusy(false); setDestination(initialDestination); }, [application.id, environment, initialDestination]);

  // Persisted progress: a refresh finds the latest transition again.
  useEffect(() => {
    const current = generation.current;
    listConnectionChoices().then((items) => { if (current === generation.current) setChoices(items.filter((item) => item.key !== target.connectionKey)); }).catch(() => undefined);
    listTransitions(application.id, environment).then((items) => {
      if (current !== generation.current) return;
      setHistory(items);
      const running = items.find((item) => item.status === 'RUNNING');
      if (running) setDetail(running);
    }).catch(() => undefined);
  }, [application.id, environment, target.connectionKey, target.targetGeneration]);

  const running = detail?.status === 'RUNNING';
  useEffect(() => {
    if (!detail || detail.status !== 'RUNNING') return;
    const current = generation.current;
    const timer = window.setInterval(() => {
      getTransition(application.id, environment, detail.id).then(async (latest) => {
        if (current !== generation.current) return;
        setDetail(latest);
        if (latest.status !== 'RUNNING') {
          const env = await fetchEnvironment(application.id, environment);
          if (env && current === generation.current) onTargetChange(application.id, environment, env);
          const items = await listTransitions(application.id, environment);
          if (current === generation.current) setHistory(items);
        }
      }).catch(() => undefined);
    }, pollMs);
    return () => window.clearInterval(timer);
  }, [detail?.id, detail?.status, application.id, environment, pollMs]); // eslint-disable-line react-hooks/exhaustive-deps

  function explain(err: unknown): string {
    if (err instanceof ApiError) {
      if (err.status === 409) return err.message;
      if (err.status === 422) return err.message;
    }
    return 'The request failed. Try again.';
  }

  async function runPreview() {
    if (!destination || busy) return;
    const current = ++generation.current;
    setBusy(true); setError(''); setPreview(undefined); setAcknowledged(false);
    try {
      const response = await previewTransition(application.id, environment, destination, mode);
      if (current === generation.current) setPreview(response.preview);
    } catch (err) {
      if (current === generation.current) setError(explain(err));
    } finally {
      if (current === generation.current) setBusy(false);
    }
  }

  async function execute() {
    if (!preview || busy) return;
    const current = ++generation.current;
    setBusy(true); setError('');
    try {
      const response = await executeTransition(application.id, environment, destination, mode, preview.token, acknowledged, preview.mappings);
      if (current !== generation.current) return;
      setDetail(response.transition);
      setPreview(undefined);
      const env = await fetchEnvironment(application.id, environment);
      if (env && current === generation.current) onTargetChange(application.id, environment, env);
    } catch (err) {
      if (current !== generation.current) return;
      if (err instanceof ApiError && err.status === 409) {
        // Stale preview or a busy environment: the old token is useless; review again.
        setPreview(undefined);
        const env = await fetchEnvironment(application.id, environment).catch(() => undefined);
        if (env && current === generation.current) onTargetChange(application.id, environment, env);
      }
      setError(explain(err));
    } finally {
      if (current === generation.current) setBusy(false);
    }
  }

  async function cleanup(id: string) {
    if (busy) return;
    const current = ++generation.current;
    setBusy(true); setError('');
    try {
      const response = await cleanupSource(application.id, environment, id);
      if (current === generation.current) { setDetail(response.transition); setHistory((items) => items.map((item) => item.id === id ? response.transition : item)); }
    } catch (err) {
      if (current === generation.current) setError(explain(err));
    } finally {
      if (current === generation.current) setBusy(false);
    }
  }

  const finished = detail && detail.status !== 'RUNNING' ? detail : undefined;
  const retained = history.find((item) => item.canCleanupSource);
  return <section className="content-panel transition-panel" aria-label="Connection transition">
    <div className="section-header"><div><h2>Change deployment destination</h2><p>This environment has runtime resources. A new destination gets a new, isolated generation. Nothing is overwritten and the old generation is kept until you clean it up.</p></div></div>
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {!detail || finished ? <>
      <label htmlFor="transition-destination">Destination connection</label>
      <select id="transition-destination" value={destination} disabled={busy || Boolean(target.activeOperation)} onChange={(event) => { setDestination(event.target.value); setPreview(undefined); }}>
        <option value="" disabled>Choose a destination</option>
        {choices.map((item) => <option key={item.key} value={item.key}>{item.name} ({item.key})</option>)}
      </select>
      <fieldset className="mode-choice"><legend>Transfer mode</legend>
        <label><input type="radio" name="transition-mode" checked={mode === 'DEPLOY_NEW'} onChange={() => { setMode('DEPLOY_NEW'); setPreview(undefined); }} /> Deploy new (empty resources, no data copied)</label>
        <label><input type="radio" name="transition-mode" checked={mode === 'MIGRATE_POSTGRES'} onChange={() => { setMode('MIGRATE_POSTGRES'); setPreview(undefined); }} /> Migrate PostgreSQL data (with downtime)</label>
      </fieldset>
      <div className="form-actions"><Button disabled={!destination || busy || Boolean(target.activeOperation)} onClick={() => void runPreview()}>{busy && !preview ? 'Checking…' : 'Preview transition'}</Button></div>
    </> : null}
    {preview ? <div className="transition-preview" aria-label="Transition preview">
      <h3>Impact</h3>
      <p><strong>{preview.source.connectionName || preview.source.connectionKey}</strong> (generation {preview.source.generation}, namespace {preview.source.namespace}) → <strong>{preview.destination.connectionName || preview.destination.connectionKey}</strong> (generation {preview.destination.generation}, namespace {preview.destination.namespace})</p>
      <ImpactList workloads={preview.workloads} />
      {preview.mappings.length ? <ul aria-label="Database mapping">{preview.mappings.map((item) => <li key={item.sourceDescriptor}>{item.sourceDescriptor} → {item.destinationDescriptor}</li>)}</ul> : null}
      <ul>{preview.notes.map((note) => <li key={note}>{note}</li>)}</ul>
      {preview.downtimeRequired ? <label className="checkbox"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /> I understand the application is stopped while its data is copied</label> : null}
      <div className="form-actions"><Button tone="primary" disabled={busy || (preview.downtimeRequired && !acknowledged)} onClick={() => void execute()}>{busy ? 'Starting…' : 'Start transition'}</Button></div>
    </div> : null}
    {detail ? <div className="transition-progress" aria-label="Transition progress">
      <h3>Transition {detail.status === 'RUNNING' ? 'in progress' : detail.status.toLowerCase()}</h3>
      <ol className="stage-list">{stages.filter((stage) => detail.mode === 'MIGRATE_POSTGRES' || !['QUIESCING', 'BACKUP', 'RESTORING'].includes(stage)).map((stage) => {
        const result = detail.stages.find((item) => item.stage === stage);
        return <li key={stage} data-status={result?.status ?? 'PENDING'}><strong>{stage}</strong> <span>{result?.status ?? 'PENDING'}</span>{result?.message ? <em> — {result.message}</em> : null}</li>;
      })}</ol>
      {running ? <p role="status" aria-busy="true">Working… progress is saved and survives a page refresh.</p> : null}
      {detail.status === 'SUCCEEDED' && detail.sourceState === 'RETAINED_QUIESCED' ? <div className="form-success" role="status">The destination is live. The source generation is retained and quiesced (data kept) until you clean it up.</div> : null}
      {detail.status === 'SUCCEEDED' && detail.sourceState === 'CLEANED' ? <div className="form-success" role="status">The destination is live and the source generation was cleaned up.</div> : null}
      {detail.status === 'SUCCEEDED' && detail.sourceState === 'NEEDS_ATTENTION' ? <div className="form-error" role="alert">The destination is live, but one or more source workloads could not be stopped. Stop them yourself before cleanup: the source is NOT quiesced.</div> : null}
      {detail.status === 'FAILED' || detail.status === 'INTERRUPTED' ? <div className="form-error" role="alert">{detail.failure || 'The transition did not complete.'} {authorityText(detail)}</div> : null}
      {detail.compensation?.length ? <ul aria-label="Recovery steps">{detail.compensation.map((item) => <li key={item}>{item}</li>)}</ul> : null}
      {detail.compensationFailed?.length ? <ul className="warning-text" aria-label="Recovery problems">{detail.compensationFailed.map((item) => <li key={item}>Needs attention: {item}</li>)}</ul> : null}
      {detail.backupRetained ? <p className="warning-text">A private backup archive is still on the source Pod and needs operator attention.</p> : null}
    </div> : null}
    {retained && !running ? <div className="retained-source" aria-label="Retained source">
      <h3>Retained source generation {retained.source.generation}</h3>
      <p>{retained.source.connectionName || retained.source.connectionKey} · namespace {retained.source.namespace} keeps its data. Delete it only after you reviewed the destination.</p>
      <Button tone="danger" disabled={busy} onClick={() => void cleanup(retained.id)}>Clean up source generation</Button>
    </div> : null}
  </section>;
}
