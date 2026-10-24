import { useEffect, useRef, useState } from 'react';
import { navigate } from '../../app/routes';
import { ApplicationTarget } from '../../shared/ui/ApplicationTarget';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { ApiError } from '../../shared/api/client';
import { previewScore, type PreviewAction, type ScorePreview, type ScorePreviewRequest } from './api';
import { parseScoreText } from './parseScore';
import { ResourceIntents, rendererText } from './ResourceIntents';
import { DeploymentReview } from './DeploymentReview';
import { DependencyGraph } from '../../shared/ui/DependencyGraph';
import { Placements } from './Placements';
import { t } from '../../shared/i18n';

const actions: { value: PreviewAction; label: string; hint: string }[] = [
  { value: 'deploy', label: t('preview.scorePreview.addWorkload'), hint: t('preview.scorePreview.onlyTheAfterScoreIs') },
  { value: 'update', label: t('preview.scorePreview.updateWorkload'), hint: t('preview.scorePreview.theBeforeScoreMustMatch') },
  { value: 'remove', label: t('preview.scorePreview.deleteWorkload'), hint: t('preview.scorePreview.onlyTheBeforeScoreIs') },
];

const actionLabel = (action: string) => actions.find((item) => item.value === action)?.label ?? action;

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
    if (!workloadId.trim()) problems.push(t('preview.scorePreview.workloadIDIsRequired'));
    if (!runId.trim()) problems.push(t('preview.scorePreview.runIDIsRequired'));
    const request: ScorePreviewRequest = { workloadId, action, runId };
    if (needsBefore) {
      const parsed = parseScoreText(before);
      if (parsed.ok) request.scoreBefore = parsed.score; else problems.push(t('preview.scorePreview.beforeScoreError', { error: parsed.error }));
    }
    if (needsAfter) {
      const parsed = parseScoreText(after);
      if (parsed.ok) request.scoreAfter = parsed.score; else problems.push(`Score sau ${parsed.error}`);
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
      // Only the stable code selects text; raw backend/parser text is never shown.
      if (err instanceof ApiError && (err.status === 400 || err.status === 413)) setFailure({ kind: 'validation', message: err.message });
      else if (err instanceof ApiError) setFailure({ kind: 'error', message: err.message });
      else setFailure({ kind: 'error', message: t('preview.scorePreview.couldNotPreviewTheScore') });
    } finally {
      if (current === generation.current) setPreviewing(false);
    }
  }

  const hint = actions.find((item) => item.value === action)?.hint;
  return <section className="page preview-page">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id })}>← {application.name}</button>
    <header className="page-header"><div><p className="eyebrow">{application.name} · {environment}</p><h1>{t('preview.scorePreview.previewScore')}</h1><p>{t('preview.scorePreview.checkTheScoreAndSee')}{' '} <strong>{application.name}</strong> {' '}{t('preview.scorePreview.in')}{' '} <strong>{environment}</strong>{t('preview.scorePreview.nothingIsSavedOrDeployed')}</p><ApplicationTarget application={application} environment={environment} /></div></header>
    <div className="tabs" role="tablist" aria-label={t('preview.scorePreview.environment')}>{(['staging', 'production'] as const).map((env) => <button key={env} role="tab" aria-selected={environment === env} className={environment === env ? 'tab tab-active' : 'tab'} onClick={() => navigate({ name: 'score-preview', applicationId: application.id, environment: env })}>{env}</button>)}</div>
    <form className="content-panel preview-form" onSubmit={(event) => { event.preventDefault(); void submit(); }}>
      <p className="form-note" role="note">{t('preview.scorePreview.doNotPasteSecretValues')}{' '} <code>{'${resources.env.API_TOKEN}'}</code>.</p>
      <div className="form-grid">
        <label>{t('preview.scorePreview.action')}<select value={action} aria-describedby="preview-action-hint" onChange={(event) => changed(setAction)(event.target.value as PreviewAction)}>{actions.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select></label>
        <label>{t('preview.scorePreview.workloadId')}<input value={workloadId} onChange={(event) => changed(setWorkloadId)(event.target.value)} placeholder={t('preview.scorePreview.mustMatchMetadataName')} autoComplete="off" /></label>
        <label>{t('preview.scorePreview.runID')}<input value={runId} aria-describedby="preview-run-hint" onChange={(event) => changed(setRunId)(event.target.value)} placeholder={t('preview.scorePreview.forExamplePreview1')} autoComplete="off" /></label>
      </div>
      <p className="form-hints"><small id="preview-action-hint">{hint}</small> <small id="preview-run-hint">{t('preview.scorePreview.theRunIDIsPlanning')}</small></p>
      {needsBefore ? <label className="score-editor">{t('preview.scorePreview.beforeScoreYAMLOrJSON')}<textarea value={before} onChange={(event) => changed(setBefore)(event.target.value)} rows={12} spellCheck={false} /></label> : null}
      {needsAfter ? <label className="score-editor">{t('preview.scorePreview.afterScoreYAMLOrJSON')}<textarea value={after} onChange={(event) => changed(setAfter)(event.target.value)} rows={12} spellCheck={false} /></label> : null}
      {failure ? <div className="form-error" role="alert">{failure.kind === 'validation' ? t('preview.scorePreview.couldNotPreview') : ''}{failure.message}{failure.kind === 'error' ? <> <Button type="button" onClick={() => void submit()}>{t('preview.scorePreview.retry')}</Button></> : null}</div> : null}
      <div className="form-actions"><Button tone="primary" type="submit" disabled={previewing}>{previewing ? t('preview.scorePreview.computingPreviewResult') : t('preview.scorePreview.preview')}</Button></div>
    </form>
    {previewing ? <p role="status" aria-busy="true">{t('preview.scorePreview.computingPreviewResult')}</p> : null}
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
    return <div className="form-success" role="status">{t('preview.scorePreview.noWorkloadChangesTheScore')}</div>;
  }
  return <ul className="delta-summary">
    {added.map((id) => <li key={`add-${id}`}><strong>{t('preview.scorePreview.add')}</strong> module {id}</li>)}
    {updated.map(([id, patches]) => <li key={`update-${id}`}><strong>{t('preview.scorePreview.update')}</strong> module {id} · {patches.length} {' '}{t('preview.scorePreview.changes')}{' '} {patches.map((patch) => `${patch.op} ${patch.path}`).join(', ')}</li>)}
    {removed.map((id) => <li key={`remove-${id}`}><strong>{t('preview.scorePreview.delete')}</strong> module {id}</li>)}
    {shared.length ? <li><strong>{t('preview.scorePreview.shared')}</strong> · {shared.length} {' '}{t('preview.scorePreview.changes')}{' '} {shared.map((patch) => `${patch.op} ${patch.path}`).join(', ')}</li> : null}
  </ul>;
}

function List({ items, empty }: { items: string[]; empty: string }) {
  return items.length ? <ul>{items.map((item) => <li key={item}>{item}</li>)}</ul> : <p>{empty}</p>;
}

function PreviewResult({ result }: { result: ScorePreview }) {
  const renderer = result.rendering?.[result.workloadId];
  return <section className="preview-result" aria-label={t('preview.scorePreview.scorePreviewResult')}>
    <section className="content-panel"><DeploymentReview review={result.review} title={t('preview.scorePreview.scorePreviewMap')} /></section>
    <section className="content-panel"><div className="section-header"><div><h2>{t('preview.scorePreview.previewWorkloadIdActionLabel', { workloadId: result.workloadId, actionLabel: actionLabel(result.action) })}</h2><p>{t('preview.scorePreview.readOnlyResultNothingIs')}</p></div></div>
      <dl className="detail-grid"><div><dt>{t('preview.scorePreview.environment')}</dt><dd>{result.applicationKey} / {result.environmentKey}</dd></div><div><dt>{t('preview.scorePreview.baseDeploymentSet')}</dt><dd>{result.baseSetId || t('preview.scorePreview.empty')}</dd></div><div><dt>{t('preview.scorePreview.environmentVersion')}</dt><dd>{result.baseVersion}</dd></div><div><dt>{t('preview.scorePreview.runID')}</dt><dd>{result.runId}</dd></div><div><dt>{t('preview.scorePreview.workloadRenderer')}</dt><dd>{rendererText(renderer)}</dd></div><div><dt>{t('preview.scorePreview.planHash')}</dt><dd><code className="plan-hash" title={result.planHash}>{result.planHash}</code></dd></div></dl></section>
    <section className="content-panel"><h2>Delta</h2><DeltaSummary result={result} /></section>
    <details open={result.review ? undefined : true}><summary>{t('preview.scorePreview.technicalPlanDetails')}</summary><section className="content-panel"><h2>{t('preview.scorePreview.resourceChanges')}</h2><div className="detail-grid">
      <div><h3>{t('preview.scorePreview.newCount', { count: result.classification.new.length })}</h3><List items={result.classification.new} empty={t('preview.scorePreview.none')} /></div>
      <div><h3>{t('preview.scorePreview.existingCount', { count: result.classification.existing.length })}</h3><List items={result.classification.existing} empty={t('preview.scorePreview.none')} /></div>
      <div><h3>{t('preview.scorePreview.unreferencedCount', { count: result.classification.unreferenced.length })}</h3><List items={result.classification.unreferenced} empty={t('preview.scorePreview.none')} /></div></div></section>
    <section className="content-panel"><h2>{t('preview.scorePreview.infrastructureResources')}</h2><Placements placements={result.placements} />{}<ResourceIntents intents={result.resourceIntents} /></section>
    <section className="content-panel"><h2>{t('preview.scorePreview.resourceCreationOrder')}</h2>{result.batches.length ? <ol className="batch-list">{result.batches.map((batch, index) => <li key={index}>{t('preview.scorePreview.batchValueBatch', { value: index + 1, batch: batch.join(', ') })}</li>)}</ol> : <p>{t('preview.scorePreview.noResourcesToCreate')}</p>}<p><small>{t('preview.scorePreview.providersAreCreatedBeforeConsumers')}</small></p></section>
    <section className="content-panel"><h2>{t('preview.scorePreview.resourceExecutionLinks')}</h2>{result.matches.length ? <table className="plain-table"><thead><tr><th>{t('preview.scorePreview.resource')}</th><th>{t('preview.scorePreview.definition')}</th><th>Driver</th></tr></thead><tbody>{result.matches.map((match) => <tr key={match.descriptor}><td>{match.descriptor}</td><td>{match.definitionKey}</td><td>{match.driverType}</td></tr>)}</tbody></table> : <p>{t('preview.scorePreview.noResourceLinks')}</p>}</section>
    <section className="content-panel"><h2>{t('preview.scorePreview.resourceGraph')}</h2>
      <DependencyGraph nodes={result.graph.nodes} edges={result.graph.edges} /></section></details>
    <section className="content-panel"><h2>{t('preview.scorePreview.documentsSensitiveDataFiltered')}</h2><p><small>{t('preview.scorePreview.sensitiveDataFilteredLiteralContainer')}{' '} <code>***redacted***</code>{t('preview.scorePreview.andParameterValuesInThe')}</small></p>
      <details><summary>{t('preview.scorePreview.deltaDocument')}</summary><pre className="score-preview">{JSON.stringify(result.delta, null, 2)}</pre></details>
      <details><summary>{t('preview.scorePreview.candidateDeploymentSetSensitiveData')}</summary><pre className="score-preview">{JSON.stringify(result.candidateSet, null, 2)}</pre></details></section>
  </section>;
}
