import { useEffect, useId, useRef, useState } from 'react';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import { t } from '../../shared/i18n';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { getConfiguration, type Configuration } from '../configuration/api';
import { emptyBinding, type Binding } from './bindings';
import { EnvironmentVariablesGrid, type EnvironmentVariablesProps } from './EnvironmentVariablesTable';
import { BulkSecretsDialog } from './BulkSecretsDialog';
import { mergeEnvfile, parseEnvfile, serializeEnvfile, type EnvfileProblem } from './envfile';

type Props = EnvironmentVariablesProps & {
  application: Application; environment: EnvironmentKey;
  onConfiguration(value: Configuration): void;
  onTargetChange?(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void;
  editorId: number; onBlocked(id: number, value: boolean): void; onBusy(value: boolean): void;
  onReplaceSecret(id: string, binding: Binding): void;
};
export function EnvironmentVariablesEditor(props: Props) {
  const { bindings, context, disabled, onChange, onBlocked } = props;
  const id = useId();
  const [tab, setTab] = useState<'ui' | 'env'>('ui');
  const [text, setText] = useState<string>();
  const [problems, setProblems] = useState<EnvfileProblem[]>([]);
  const [fileError, setFileError] = useState(false), [reading, setReading] = useState(false);
  const [importText, setImportText] = useState<string>();
  const [incoming, setIncoming] = useState<Binding[]>();
  const [replace, setReplace] = useState(new Set<string>());
  const [selected, setSelected] = useState(new Set<string>());
  const [confirming, setConfirming] = useState(false);
  const [bulkBusy, setBulkBusy] = useState(false);
  const emitted = useRef<Binding[] | undefined>(undefined);
  const observed = useRef(bindings);
  const alive = useRef(true), fileVersion = useRef(0);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  useEffect(() => {
    if (bindings !== observed.current) {
      observed.current = bindings;
      if (bindings !== emitted.current && !problems.length) { emitted.current = undefined; setText(undefined); setProblems([]); }
    }
  }, [bindings, problems.length]);
  let canonical = '', serializationError = false;
  try { canonical = serializeEnvfile(bindings, context); } catch { serializationError = true; }
  // Revalidate preserved text when metadata changes. Never commit it partially.
  const parsed = importText !== undefined ? parseEnvfile(importText, context, bindings) : text === undefined ? undefined : parseEnvfile(text, context, bindings);
  const currentProblems = parsed?.problems ?? problems;
  // Metadata can make preserved text valid without another keystroke (for
  // example, adding its missing resource). Publish that whole draft before
  // unlocking Save; otherwise the old table could silently be saved instead.
  const needsReconciliation = importText === undefined && text !== undefined && parsed !== undefined && parsed.problems.length === 0 && serializeEnvfile(parsed.bindings, context) !== canonical;
  useEffect(() => {
    if (importText !== undefined || text === undefined || !parsed || parsed.problems.length) return;
    if (problems.length) setProblems([]);
    if (needsReconciliation) { emitted.current = parsed.bindings; onChange(parsed.bindings); }
  }, [importText, text, parsed, problems.length, needsReconciliation, onChange]);
  const blocked = currentProblems.length > 0 || needsReconciliation || reading || !!incoming || importText !== undefined || serializationError;
  useEffect(() => { onBlocked(props.editorId, blocked); }, [blocked, onBlocked, props.editorId]);
  useEffect(() => () => onBlocked(props.editorId, false), [onBlocked, props.editorId]);
  function publish(next: Binding[]) { emitted.current = next; onChange(next); }
  function edit(value: string) {
    fileVersion.current++; setReading(false); setFileError(false);
    if (importText !== undefined) {
      const result = parseEnvfile(value, context, bindings); setImportText(value); setProblems(result.problems);
      if (!result.problems.length) { setIncoming(result.bindings); setReplace(new Set()); }
      return;
    }
    setText(value);
    const result = parseEnvfile(value, context, bindings);
    setProblems(result.problems);
    if (!result.problems.length) publish(result.bindings);
  }
  async function readFile(file?: File) {
    if (!file || disabled || bulkBusy) return;
    const version = ++fileVersion.current;
    setReading(true); setFileError(false);
    try {
      if (file.size > 1024 * 1024) throw new Error('size');
      const value = new TextDecoder('utf-8', { fatal: true }).decode(await file.arrayBuffer());
      if (!alive.current || version !== fileVersion.current) return;
      const result = parseEnvfile(value, context, bindings);
      if (result.problems.length) { setImportText(value); setFileError(true); setProblems(result.problems); }
      else { setIncoming(result.bindings); setReplace(new Set()); setProblems([]); }
    } catch { if (alive.current && version === fileVersion.current) setFileError(true); }
    finally { if (alive.current && version === fileVersion.current) setReading(false); }
  }
  function download(value: string, name: string) {
    const url = URL.createObjectURL(new Blob([value], { type: 'text/plain;charset=utf-8' }));
    const link = document.createElement('a'); link.href = url; link.download = name; link.click(); URL.revokeObjectURL(url);
  }
  function sample() {
    let sequence = 0;
    const rows = [{ ...emptyBinding(), name: 'APP_ENV', value: 'production' }, ...context.dependencies.flatMap(dep => (context.outputs(dep.alias) ?? []).map((output) => ({ ...emptyBinding('RESOURCE'), name: `${dep.alias}_${output}_${++sequence}`.replace(/[^A-Za-z0-9_]/g, '_').replace(/^[^A-Za-z_]/, '_'), alias: dep.alias, output })))];
    try { download(serializeEnvfile(rows, context), 'example.env'); } catch { setFileError(true); }
  }
  const locked = disabled || bulkBusy || reading;
  return <section className="envfile-editor">
    <div className="envfile-toggle">
      <Button tone="quiet" aria-controls={`${id}-env-panel`} aria-expanded={tab === 'env'} disabled={locked || tab === 'env' && currentProblems.length > 0} onClick={() => setTab(tab === 'ui' ? 'env' : 'ui')}>{t(tab === 'ui' ? 'workloads.envfile.env' : 'workloads.envfile.ui')}</Button>
    </div>
    <div id={`${id}-ui-panel`} hidden={tab !== 'ui'}>
      <EnvironmentVariablesGrid {...props} disabled={locked || blocked} selected={selected} onSelect={(row, checked) => setSelected(prev => { const next = new Set(prev); if (checked) next.add(row); else next.delete(row); return next; })} onChange={next => { fileVersion.current++; setText(undefined); setProblems([]); publish(next); }} />
      {selected.size > 0 ? <div className="form-actions"><Button disabled={locked || blocked || !bindings.some(row => selected.has(row.id) && row.source === 'LITERAL')} onClick={() => publish(bindings.map(row => selected.has(row.id) && row.source === 'LITERAL' ? { ...row, secretDraft: true } : row))}>{t('workloads.envfile.markSecrets')}</Button></div> : null}
    </div>
    <div id={`${id}-env-panel`} hidden={tab !== 'env'}>
      <label className="field">{t('workloads.envfile.content')}<textarea aria-label={t('workloads.envfile.content')} className="envfile-text" rows={12} value={importText ?? text ?? canonical} disabled={locked || serializationError} autoComplete="off" autoCorrect="off" autoCapitalize="off" spellCheck={false} aria-invalid={currentProblems.length > 0} aria-describedby={`${id}-errors`} onChange={event => edit(event.target.value)} /></label>
      <div className="envfile-actions"><label className="field">{t('workloads.envfile.file')}<input type="file" accept=".env,text/plain" disabled={locked || currentProblems.length > 0} onChange={event => { void readFile(event.target.files?.[0]); event.target.value = ''; }} /></label><Button disabled={locked || serializationError} onClick={sample}>{t('workloads.envfile.sample')}</Button><Button disabled={locked || serializationError || currentProblems.length > 0} onClick={() => download(text ?? canonical, '.env')}>{t('workloads.envfile.download')}</Button></div>
      <details><summary>{t('workloads.envEntry.help')}</summary><p>{t('workloads.envfile.help')}</p></details>
    </div>
    <div id={`${id}-errors`}>{currentProblems.map((problem, index) => <p className="field-error" role="alert" key={index}>{t(`workloads.envfile.error.${problem.code}`, { line: problem.line, first: problem.first ?? '' })}</p>)}</div>
    {serializationError ? <div><p className="field-error" role="alert">{t('workloads.envfile.serializationError')}</p><Button disabled={locked} onClick={async () => { try { const config = await getConfiguration(props.application.id, props.environment); if (alive.current) props.onConfiguration(config); } catch { if (alive.current) setFileError(true); } }}>{t('workloads.createKey.reviewConfig')}</Button></div> : null}
    {fileError ? <p className="field-error" role="alert">{t('workloads.envfile.fileError')}</p> : null}
    {bindings.some(row => row.secretDraft) ? <div className="form-actions"><Button disabled={locked || blocked || props.catalog.status !== 'ready'} onClick={() => setConfirming(true)}>{t('workloads.envfile.confirmSecrets')}</Button></div> : null}
    {incoming ? <Dialog label={t('workloads.envfile.reviewImport')} onClose={() => { setIncoming(undefined); setImportText(undefined); setProblems([]); }}><h2>{t('workloads.envfile.reviewImport')}</h2>
      {incoming.filter(row => bindings.some(old => old.name === row.name)).map(row => <label key={row.name} className="field">{row.name}<select value={replace.has(row.name) ? 'replace' : 'keep'} onChange={event => setReplace(prev => { const next = new Set(prev); if (event.target.value === 'replace') next.add(row.name); else next.delete(row.name); return next; })}><option value="keep">{t('workloads.envfile.keep')}</option><option value="replace">{t('workloads.envfile.replace')}</option></select></label>)}
      <p>{t('workloads.envfile.mergeNote')}</p><div className="form-actions"><Button onClick={() => { setIncoming(undefined); setImportText(undefined); setProblems([]); }}>{t('workloads.createKey.cancel')}</Button><Button tone="primary" onClick={() => { publish(mergeEnvfile(bindings, incoming, replace)); setText(undefined); setProblems([]); setIncoming(undefined); setImportText(undefined); }}>{t('workloads.envfile.import')}</Button></div>
    </Dialog> : null}
    {confirming && props.catalog.status === 'ready' ? <BulkSecretsDialog application={props.application} environment={props.environment} rows={bindings.filter(row => row.secretDraft)} catalog={props.catalog} onConfiguration={props.onConfiguration} onTargetChange={props.onTargetChange} onUncertain={(rowId, value) => { const row = bindings.find(item => item.id === rowId); if (row) props.onReplaceSecret(rowId, { ...row, secretUncertain: value }); }} onReplace={(rowId, next) => { setText(undefined); props.onReplaceSecret(rowId, next); }} onBusy={value => { setBulkBusy(value); props.onBusy(value); }} onClose={() => setConfirming(false)} /> : null}
  </section>;
}
