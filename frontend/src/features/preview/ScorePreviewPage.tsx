import { useEffect, useRef, useState } from 'react';
import { navigate } from '../../app/routes';
import { ApplicationTarget } from '../../shared/ui/ApplicationTarget';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { ApiError } from '../../shared/api/client';
import { previewScore, type PreviewAction, type ScorePreview, type ScorePreviewRequest } from './api';
import { parseScoreText } from './parseScore';

const actions: { value: PreviewAction; label: string; hint: string }[] = [
  { value: 'deploy', label: 'Add workload (deploy)', hint: 'Score after only; the workload must not exist yet.' },
  { value: 'update', label: 'Update workload', hint: 'Score before must equal the current deployed Score.' },
  { value: 'remove', label: 'Remove workload', hint: 'Score before only; must equal the current deployed Score.' },
];

type Failure = { kind: 'validation' | 'error'; message: string };

export function ScorePreviewPage({ application, environment }: { application: Application; environment: EnvironmentKey }) {
  const [action, setAction] = useState<PreviewAction>('deploy');
  const [workloadId, setWorkloadId] = useState('');
  const [runId, setRunId] = useState('');
  const [before, setBefore] = useState('');
  const [after, setAfter] = useState('');
  const [result, setResult] = useState<ScorePreview>();
  const [failure, setFailure] = useState<Failure>();
  const [previewing, setPreviewing] = useState(false);
  // Every input or scope change bumps the generation, so an obsolete
  // response can never be shown as the preview of the current input.
  const generation = useRef(0);
  const needsBefore = action !== 'deploy';
  const needsAfter = action !== 'remove';

  useEffect(() => { generation.current += 1; setResult(undefined); setFailure(undefined); setPreviewing(false); }, [application.id, environment]);

  function changed<T>(set: (value: T) => void) {
    return (value: T) => { generation.current += 1; set(value); setResult(undefined); setFailure(undefined); setPreviewing(false); };
  }

  async function submit() {
    const problems: string[] = [];
    if (!workloadId.trim()) problems.push('Workload ID is required.');
    if (!runId.trim()) problems.push('Run ID is required.');
    const request: ScorePreviewRequest = { workloadId, action, runId };
    if (needsBefore) {
      const parsed = parseScoreText(before);
      if (parsed.ok) request.scoreBefore = parsed.score; else problems.push(`Score before ${parsed.error}`);
    }
    if (needsAfter) {
      const parsed = parseScoreText(after);
      if (parsed.ok) request.scoreAfter = parsed.score; else problems.push(`Score after ${parsed.error}`);
    }
    setResult(undefined);
    if (problems.length) { setFailure({ kind: 'validation', message: problems.join(' ') }); return; }
    const current = ++generation.current;
    setFailure(undefined); setPreviewing(true);
    try {
      const response = await previewScore(application.id, environment, request);
      if (current === generation.current) setResult(response);
    } catch (err) {
      if (current !== generation.current) return;
      if (err instanceof ApiError && err.status === 401) return;
      if (err instanceof ApiError && (err.status === 400 || err.status === 413)) setFailure({ kind: 'validation', message: err.message });
      else if (err instanceof ApiError && err.status === 404) setFailure({ kind: 'error', message: 'This Application or Environment is not available.' });
      else setFailure({ kind: 'error', message: 'Preview failed. Try again.' });
    } finally {
      if (current === generation.current) setPreviewing(false);
    }
  }

  const hint = actions.find((item) => item.value === action)?.hint;
  return <section className="page preview-page">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id })}>← {application.name}</button>
    <header className="page-header"><div><p className="eyebrow">{application.name} · {environment}</p><h1>Preview Score</h1><p>Validate a Score and see the planned changes for <strong>{application.name}</strong> in <strong>{environment}</strong>. Nothing is saved or deployed.</p><ApplicationTarget application={application} /></div></header>
    <div className="tabs" role="tablist" aria-label="Environment">{(['staging', 'production'] as const).map((env) => <button key={env} role="tab" aria-selected={environment === env} className={environment === env ? 'tab tab-active' : 'tab'} onClick={() => navigate({ name: 'score-preview', applicationId: application.id, environment: env })}>{env === 'staging' ? 'Staging' : 'Production'}</button>)}</div>
    <form className="content-panel preview-form" onSubmit={(event) => { event.preventDefault(); void submit(); }}>
      <p className="form-note" role="note">Do not paste secret values. Reference configuration with placeholders such as <code>{'${resources.env.API_TOKEN}'}</code>.</p>
      <div className="form-grid">
        <label>Action<select value={action} aria-describedby="preview-action-hint" onChange={(event) => changed(setAction)(event.target.value as PreviewAction)}>{actions.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select></label>
        <label>Workload ID<input value={workloadId} onChange={(event) => changed(setWorkloadId)(event.target.value)} placeholder="must equal metadata.name" autoComplete="off" /></label>
        <label>Run ID<input value={runId} aria-describedby="preview-run-hint" onChange={(event) => changed(setRunId)(event.target.value)} placeholder="for example preview-1" autoComplete="off" /></label>
      </div>
      <p className="form-hints"><small id="preview-action-hint">{hint}</small> <small id="preview-run-hint">Run ID is the planning context used in generated names; reuse it to compare runs.</small></p>
      {needsBefore ? <label className="score-editor">Score before (YAML or JSON)<textarea value={before} onChange={(event) => changed(setBefore)(event.target.value)} rows={12} spellCheck={false} /></label> : null}
      {needsAfter ? <label className="score-editor">Score after (YAML or JSON)<textarea value={after} onChange={(event) => changed(setAfter)(event.target.value)} rows={12} spellCheck={false} /></label> : null}
      {failure ? <div className="form-error" role="alert">{failure.kind === 'validation' ? 'Cannot preview: ' : ''}{failure.message}{failure.kind === 'error' ? <> <Button type="button" onClick={() => void submit()}>Retry</Button></> : null}</div> : null}
      <div className="form-actions"><Button tone="primary" type="submit" disabled={previewing}>{previewing ? 'Calculating preview…' : 'Preview'}</Button></div>
    </form>
    {previewing ? <p role="status" aria-busy="true">Calculating preview…</p> : null}
    {result ? <PreviewResult result={result} /> : null}
  </section>;
}

function DeltaSummary({ result }: { result: ScorePreview }) {
  const modules = result.delta.modules;
  const added = Object.keys(modules?.add ?? {});
  const updated = Object.entries(modules?.update ?? {});
  const removed = modules?.remove ?? [];
  const shared = result.delta.shared ?? [];
  if (!added.length && !updated.length && !removed.length && !shared.length) {
    return <div className="form-success" role="status">No workload change: the Score produces the current Deployment Set. The planning details below still describe the Environment graph.</div>;
  }
  return <ul className="delta-summary">
    {added.map((id) => <li key={`add-${id}`}><strong>add</strong> module {id}</li>)}
    {updated.map(([id, patches]) => <li key={`update-${id}`}><strong>update</strong> module {id} · {patches.length} change(s): {patches.map((patch) => `${patch.op} ${patch.path}`).join(', ')}</li>)}
    {removed.map((id) => <li key={`remove-${id}`}><strong>remove</strong> module {id}</li>)}
    {shared.length ? <li><strong>shared</strong> · {shared.length} change(s): {shared.map((patch) => `${patch.op} ${patch.path}`).join(', ')}</li> : null}
  </ul>;
}

function List({ items, empty }: { items: string[]; empty: string }) {
  return items.length ? <ul>{items.map((item) => <li key={item}>{item}</li>)}</ul> : <p>{empty}</p>;
}

function PreviewResult({ result }: { result: ScorePreview }) {
  const renderer = result.rendering?.[result.workloadId];
  return <section className="preview-result" aria-label="Score preview result">
    <section className="content-panel"><div className="section-header"><div><h2>Preview · {result.workloadId} · {result.action}</h2><p>Read-only result. Nothing was saved or deployed; deploy through the normal workload flow.</p></div></div>
      <dl className="detail-grid"><div><dt>Environment</dt><dd>{result.applicationKey} / {result.environmentKey}</dd></div><div><dt>Base Deployment Set</dt><dd>{result.baseSetId || 'empty'}</dd></div><div><dt>Environment version</dt><dd>{result.baseVersion}</dd></div><div><dt>Run ID</dt><dd>{result.runId}</dd></div><div><dt>Workload renderer</dt><dd>{renderer ? `${renderer.driverType} ${renderer.bundle.version} · ${renderer.definitionKey}` : 'Built-in Kubernetes'}</dd></div><div><dt>Plan hash</dt><dd><code className="plan-hash" title={result.planHash}>{result.planHash}</code></dd></div></dl></section>
    <section className="content-panel"><h2>Delta</h2><DeltaSummary result={result} /></section>
    <section className="content-panel"><h2>Resource changes</h2><div className="detail-grid">
      <div><h3>New ({result.classification.new.length})</h3><List items={result.classification.new} empty="None" /></div>
      <div><h3>Existing ({result.classification.existing.length})</h3><List items={result.classification.existing} empty="None" /></div>
      <div><h3>Unreferenced ({result.classification.unreferenced.length})</h3><List items={result.classification.unreferenced} empty="None" /></div></div></section>
    <section className="content-panel"><h2>Provision order</h2>{result.batches.length ? <ol className="batch-list">{result.batches.map((batch, index) => <li key={index}>Batch {index + 1}: {batch.join(', ')}</li>)}</ol> : <p>No resources to provision.</p>}<p><small>Providers are provisioned before their consumers.</small></p></section>
    <section className="content-panel"><h2>Matched Resource Definitions</h2>{result.matches.length ? <table className="plain-table"><thead><tr><th>Resource</th><th>Definition</th><th>Driver</th></tr></thead><tbody>{result.matches.map((match) => <tr key={match.descriptor}><td>{match.descriptor}</td><td>{match.definitionKey}</td><td>{match.driverType}</td></tr>)}</tbody></table> : <p>No resource matched.</p>}</section>
    <section className="content-panel"><h2>Resource Graph</h2><p>{result.graph.nodes.length} node(s) · {result.graph.edges.length} dependency edge(s)</p>
      <h3>Nodes</h3><ul>{result.graph.nodes.map((node) => <li key={node.descriptor}>{node.descriptor} · {node.kind}{node.paramKeys.length ? ` · params: ${node.paramKeys.join(', ')}` : ''}</li>)}</ul>
      {result.graph.edges.length ? <><h3>Dependencies (consumer → provider)</h3><ul>{result.graph.edges.map((edge, index) => <li key={`${edge.consumer}-${edge.provider}-${index}`}>{edge.consumer} → {edge.provider} <small>({edge.reason})</small></li>)}</ul></> : null}</section>
    <section className="content-panel"><h2>Documents (sanitized view)</h2><p><small>Sanitized view: literal container variable values of other workloads show as <code>***redacted***</code>, and Resource Graph parameter values are omitted (only parameter names are listed). This is not an executable Deployment Set.</small></p>
      <details><summary>Delta document</summary><pre className="score-preview">{JSON.stringify(result.delta, null, 2)}</pre></details>
      <details><summary>Candidate Deployment Set — sanitized view, not executable</summary><pre className="score-preview">{JSON.stringify(result.candidateSet, null, 2)}</pre></details></section>
  </section>;
}
