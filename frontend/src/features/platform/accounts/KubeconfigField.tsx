import { useEffect, useId, useRef, useState, type ChangeEvent } from 'react';
import { Button } from '../../../shared/ui/Button';
import { accountGuidance, inspectKubeconfig, maxKubeconfigBytes, type KubeconfigContext } from './accountsApi';
import { t } from '../../../shared/i18n';

export type KubeconfigDraft = { kubeconfig: string; context: string; summary?: KubeconfigContext };

const pasteDebounceMs = 500;

function readText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ''));
    reader.onerror = () => reject(new Error('read failed'));
    reader.readAsText(file);
  });
}

/**
 * Concealed kubeconfig input shared by account creation and credential
 * replacement: upload or paste, parse-only inspect, context choice. Every
 * document change invalidates the previous contexts and any pending file read
 * or inspect response. `resetSignal` clears the document, file and contexts.
 * The document is only handed to the parent through onChange; nothing is
 * stored or echoed here.
 */
export function KubeconfigField({ disabled, resetSignal, onChange, onContexts }: {
  disabled: boolean; resetSignal: number; onChange(draft: KubeconfigDraft): void; onContexts?(contexts: KubeconfigContext[]): void;
}) {
  const [source, setSource] = useState<'upload' | 'paste'>('upload');
  const [kubeconfig, setKubeconfig] = useState('');
  const [fileLabel, setFileLabel] = useState('');
  const [hidden, setHidden] = useState(true);
  const [contexts, setContexts] = useState<KubeconfigContext[]>([]);
  const [selected, setSelected] = useState('');
  const [reading, setReading] = useState(false);
  const [inspecting, setInspecting] = useState(false);
  const [canRetry, setCanRetry] = useState(false);
  const [error, setError] = useState('');
  const fileInput = useRef<HTMLInputElement>(null);
  const id = useId();
  const revision = useRef(0);
  const request = useRef(0);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const notify = useRef(onChange);
  notify.current = onChange;
  const notifyContexts = useRef(onContexts);
  notifyContexts.current = onContexts;

  function cancelTimer() { clearTimeout(timer.current); timer.current = undefined; }
  function publish(text: string, list: KubeconfigContext[], chosen: string) {
    notify.current({ kubeconfig: text, context: chosen, summary: list.find((context) => context.name === chosen) });
  }
  function replaceDocument(next: string) {
    revision.current += 1; request.current += 1; cancelTimer();
    setKubeconfig(next); setContexts([]); setSelected(''); setReading(false); setInspecting(false); setCanRetry(false); setError('');
    notifyContexts.current?.([]);
    publish(next, [], '');
  }

  useEffect(() => () => { revision.current += 1; request.current += 1; cancelTimer(); }, []);
  const lastReset = useRef(resetSignal);
  useEffect(() => {
    if (lastReset.current === resetSignal) return;
    lastReset.current = resetSignal;
    setFileLabel(''); setHidden(true);
    if (fileInput.current) fileInput.current.value = '';
    replaceDocument('');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetSignal]);

  function chooseSource(next: 'upload' | 'paste') {
    if (next === source) return;
    setSource(next); setFileLabel('');
    if (fileInput.current) fileInput.current.value = '';
    replaceDocument('');
  }

  async function inspect(text: string, pending: number) {
    if (new Blob([text]).size > maxKubeconfigBytes) { setError(t('platform.accounts.kubeconfigField.theKubeconfigIsLargerThan')); return; }
    const requestId = ++request.current;
    const current = () => pending === revision.current && requestId === request.current;
    setInspecting(true); setError(''); setCanRetry(false);
    try {
      const result = await inspectKubeconfig(text);
      if (!current()) return;
      const found = result.contexts ?? [];
      const chosen = found.length === 1 && found[0] ? found[0].name : '';
      setContexts(found); setSelected(chosen);
      notifyContexts.current?.(found);
      publish(text, found, chosen);
    } catch (reason) {
      if (current()) { setError(accountGuidance('inspect', reason)); setCanRetry(true); }
    } finally {
      if (current()) setInspecting(false);
    }
  }

  async function chooseFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    replaceDocument(''); setFileLabel('');
    if (!file) return;
    if (file.size > maxKubeconfigBytes) { setError(t('platform.accounts.kubeconfigField.theKubeconfigFileIsLarger')); return; }
    const pending = revision.current;
    setReading(true);
    let text: string;
    try { text = await readText(file); } catch { if (pending === revision.current) { setReading(false); setError(t('platform.accounts.kubeconfigField.couldNotReadTheKubeconfig')); } return; }
    if (pending !== revision.current) return;
    setReading(false); setKubeconfig(text); setFileLabel(file.name);
    publish(text, [], '');
    void inspect(text, pending);
  }

  function pasteDocument(next: string) {
    replaceDocument(next);
    if (!next.trim()) return;
    const pending = revision.current;
    timer.current = setTimeout(() => { timer.current = undefined; void inspect(next, pending); }, pasteDebounceMs);
  }

  function chooseContext(name: string) {
    setSelected(name);
    publish(kubeconfig, contexts, name);
  }

  const chosen = contexts.find((context) => context.name === selected);
  return <div className="connection-source">
    <div className="source-choices" role="radiogroup" aria-label={t('platform.accounts.kubeconfigField.kubeconfigSource')}>
      <label className="source-choice"><input type="radio" name={`${id}-source`} disabled={disabled} checked={source === 'upload'} onChange={() => chooseSource('upload')} /> {' '}{t('platform.accounts.kubeconfigField.uploadKubeconfigFile')}</label>
      <label className="source-choice"><input type="radio" name={`${id}-source`} disabled={disabled} checked={source === 'paste'} onChange={() => chooseSource('paste')} /> {' '}{t('platform.accounts.kubeconfigField.pasteKubeconfigContent')}</label>
    </div>
    {source === 'upload'
      ? <div className="connection-field"><span id={`${id}-file`}>{t('platform.accounts.kubeconfigField.kubeconfigFile')}</span><div className="file-picker">
          <span className="button file-picker-button">{t('platform.accounts.kubeconfigField.chooseFile')}<input ref={fileInput} type="file" disabled={disabled} aria-labelledby={`${id}-file`} aria-describedby={`${id}-file-name`} onChange={(event) => void chooseFile(event)} /></span>
          <small id={`${id}-file-name`} className="file-picker-name">{fileLabel ? t('platform.accounts.kubeconfigField.fileSelectedFileLabelTheFile', { fileLabel }) : t('platform.accounts.kubeconfigField.noFileSelected')}</small>
        </div></div>
      : <label className="connection-field">{t('platform.accounts.kubeconfigField.kubeconfigContent')}<textarea aria-label={t('platform.accounts.kubeconfigField.kubeconfigContent')} className={hidden ? 'masked-text' : undefined} rows={8} spellCheck={false} autoComplete="off" disabled={disabled} value={kubeconfig} onChange={(event) => pasteDocument(event.target.value)} /></label>}
    <div className="source-actions">
      {canRetry ? <Button type="button" disabled={disabled || !kubeconfig.trim()} onClick={() => void inspect(kubeconfig, revision.current)}>{t('platform.accounts.kubeconfigField.recheckKubeconfig')}</Button> : null}
      {source === 'paste' ? <label className="inline-check"><input type="checkbox" checked={!hidden} onChange={(event) => setHidden(!event.target.checked)} /> {' '}{t('platform.accounts.kubeconfigField.showContent')}</label> : null}
    </div>
    {reading || inspecting ? <p className="muted" role="status" aria-live="polite">{reading ? t('platform.accounts.kubeconfigField.readingKubeconfigFile') : t('platform.accounts.kubeconfigField.checkingKubeconfig')}</p> : null}
    {contexts.length > 1 ? <label className="connection-field">Context<select value={selected} disabled={disabled} required onChange={(event) => chooseContext(event.target.value)}><option value="">{t('platform.accounts.kubeconfigField.chooseContext')}</option>{contexts.map((context) => <option key={context.name} value={context.name}>{context.name}</option>)}</select></label> : null}
    {chosen ? <dl className="destination-summary" aria-label={t('platform.accounts.kubeconfigField.selectedTarget')}>
      <div><dt>Context</dt> <dd>{chosen.name}</dd></div>
      <div><dt>Cluster</dt> <dd>{chosen.cluster}</dd></div>
      <div><dt>Endpoint</dt> <dd>{chosen.endpoint}</dd></div>
    </dl> : null}
    {error ? <div className="form-error" role="alert">{error}</div> : null}
  </div>;
}
