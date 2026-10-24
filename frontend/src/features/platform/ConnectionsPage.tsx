import { useEffect, useId, useRef, useState, type ChangeEvent, type FormEvent } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { specificGuidance } from '../../shared/api/errors';
import { Button } from '../../shared/ui/Button';
import { AwsConnectionForm } from './AwsConnectionForm';
import { useCatalogList } from './useCatalogList';
import { t } from '../../shared/i18n';

type Connection = {
  key: string; name?: string; kind: string; authenticationType?: string; status: string;
  config?: { cluster?: string; kubeContext?: string; endpoint?: string; region?: string; accountId?: string; allowedRegions?: string[] };
  verification?: { identity?: string; regionCheck?: string; capabilityCheck?: string };
};
type ConnectionKind = 'kubernetes' | 'aws';
type KubeconfigContext = { name: string; cluster: string; endpoint: string };
type Source = 'upload' | 'paste';

// Same bound as the backend document limit (UC-04 API mapping).
const maxKubeconfigBytes = 1 << 20;
const maxNameLength = 100;
// A pasted document is inspected once it has stopped changing for this long.
const pasteDebounceMs = 500;

function readText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ''));
    reader.onerror = () => reject(new Error('read failed'));
    reader.readAsText(file);
  });
}

const loadConnections = async () => (await api<{ connections: Connection[] }>('/connections')).connections ?? [];

// Fixed Vietnamese guidance by operation and HTTP status. Raw backend or
// provider text never reaches the page; specific guidance comes from stable error codes.
const listFailure = () => t('platform.connections.couldNotLoadTheConnection');

function inspectGuidance(reason: unknown): string {
  if (!(reason instanceof ApiError)) return t('platform.connections.theKubeconfigWasNotChecked');
  switch (reason.status) {
    case 400: case 422: return t('platform.connections.couldNotReadTheKubeconfig', { value: specificGuidance(reason.code, reason.message) ?? t('platform.connections.checkThatTheFileIs') });
    case 401: return t('platform.connections.yourSessionHasExpiredSign');
    case 403: return t('platform.connections.yourAccountIsNotAllowed');
    case 413: return t('platform.connections.theKubeconfigIsLargerThan');
    default: return reason.status >= 500 ? t('platform.connections.theServerFailedWhileReading') : t('platform.connections.couldNotCheckTheKubeconfig');
  }
}

function registerGuidance(reason: unknown): string {
  // No HTTP status: the request may or may not have reached the backend.
  if (!(reason instanceof ApiError) || reason.code === 'COMMIT_UNCERTAIN') return t('platform.connections.noResponseFromTheServer');
  switch (reason.status) {
    case 400: return t('platform.connections.theServerRejectedTheRegistration', { value: reason.code && reason.code !== 'INVALID_REQUEST' ? specificGuidance(reason.code, reason.message) ?? '' : t('platform.connections.checkTheConnectionNameKubeconfig') });
    case 401: return t('platform.connections.yourSessionHasExpiredSign2');
    case 403: return t('platform.connections.yourAccountIsNotAllowed2');
    case 409: return t('platform.connections.couldNotRegisterBecauseThe');
    case 413: return t('platform.connections.theRegistrationDataIsToo');
    case 422: return t('platform.connections.couldNotVerifyTheCluster', { value: reason.code && reason.code !== 'VERIFICATION_FAILED' ? reason.message : t('platform.connections.checkTheEndpointTheCredentials') });
    case 503: return t('platform.connections.thePlatformCredentialStoreIs');
    default: return reason.status >= 500 ? t('platform.connections.theServerFailedDuringRegistration') : t('platform.connections.couldNotRegisterTheConnection');
  }
}

// Name suggested from the selected context; the cluster is the clearer label.
function suggestName(context: KubeconfigContext | undefined): string {
  return context ? (context.cluster || context.name).slice(0, maxNameLength) : '';
}

export function ConnectionsPage() {
  const { items, loading, loadError, reload } = useCatalogList(loadConnections, listFailure);
  // The Console shows READY Connections only (UC-04 UI states).
  const connections = items.filter((connection) => connection.status === 'READY');
  // Once the user edits the name (even to blank) the suggestion never applies
  // again until the form is reset.
  const [typedName, setTypedName] = useState<string | null>(null);
  const [kind, setKind] = useState<ConnectionKind>('kubernetes');
  const [source, setSource] = useState<Source>('upload');
  const [kubeconfig, setKubeconfig] = useState('');
  const [fileLabel, setFileLabel] = useState('');
  const [hidden, setHidden] = useState(true);
  const [contexts, setContexts] = useState<KubeconfigContext[]>([]);
  const [selected, setSelected] = useState('');
  const [reading, setReading] = useState(false);
  const [inspecting, setInspecting] = useState(false);
  const [canRetry, setCanRetry] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const fileInput = useRef<HTMLInputElement>(null);
  const formId = useId();
  // Every document change bumps the revision and every inspection takes a new
  // request id, so a late file read or inspection response (also a retry of
  // the same document) can never restore an older document's contexts.
  const revision = useRef(0);
  const request = useRef(0);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  function cancelTimer() { clearTimeout(timer.current); timer.current = undefined; }
  useEffect(() => () => { revision.current += 1; request.current += 1; cancelTimer(); }, []);

  function replaceDocument(next: string) {
    revision.current += 1; request.current += 1; cancelTimer();
    setKubeconfig(next); setContexts([]); setSelected(''); setReading(false); setInspecting(false); setCanRetry(false); setError('');
  }

  function chooseSource(next: Source) {
    if (next === source) return;
    setSource(next); setFileLabel('');
    if (fileInput.current) fileInput.current.value = '';
    replaceDocument('');
  }

  async function inspect(text: string, pending: number) {
    if (new Blob([text]).size > maxKubeconfigBytes) { setError(t('platform.connections.theKubeconfigIsLargerThan2')); return; }
    const id = ++request.current;
    const current = () => pending === revision.current && id === request.current;
    setInspecting(true); setError(''); setCanRetry(false); setNotice('');
    try {
      const result = await api<{ contexts: KubeconfigContext[] }>('/connections/kubernetes/inspect', { method: 'POST', body: JSON.stringify({ kubeconfig: text }) });
      if (!current()) return;
      const found = result.contexts ?? [];
      setContexts(found);
      // One context is selected for the user; several never guess from current-context.
      setSelected(found.length === 1 && found[0] ? found[0].name : '');
    } catch (reason) {
      if (current()) { setError(inspectGuidance(reason)); setCanRetry(true); }
    } finally {
      if (current()) setInspecting(false);
    }
  }

  async function chooseFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    replaceDocument('');
    setFileLabel('');
    if (!file) return;
    if (file.size > maxKubeconfigBytes) { setError(t('platform.connections.theKubeconfigFileIsLarger')); return; }
    const pending = revision.current;
    setReading(true);
    let text: string;
    try { text = await readText(file); } catch { if (pending === revision.current) { setReading(false); setError(t('platform.connections.couldNotReadTheKubeconfig2')); } return; }
    if (pending !== revision.current) return;
    setReading(false); setKubeconfig(text); setFileLabel(file.name);
    void inspect(text, pending);
  }

  function pasteDocument(next: string) {
    replaceDocument(next);
    if (!next.trim()) return;
    const pending = revision.current;
    timer.current = setTimeout(() => { timer.current = undefined; void inspect(next, pending); }, pasteDebounceMs);
  }

  const chosen = contexts.find((context) => context.name === selected);
  const name = typedName ?? suggestName(chosen);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving || inspecting || reading) return;
    if (!name.trim()) { setError(t('platform.connections.enterAConnectionName')); return; }
    if (!chosen) { setError(t('platform.connections.uploadOrPasteAKubeconfig')); return; }
    cancelTimer();
    setError(''); setNotice(''); setSaving(true);
    let created: Connection;
    try {
      created = await api<Connection>('/connections/kubernetes', { method: 'POST', body: JSON.stringify({ name, kubeconfig, context: selected }) });
    } catch (reason) {
      setError(registerGuidance(reason)); setSaving(false);
      // An unknown outcome is checked by reloading the list, never by resubmitting.
      if (!(reason instanceof ApiError) || reason.code === 'COMMIT_UNCERTAIN') void reload();
      return;
    }
    // Committed: report it, clear every credential and the form, then reload.
    setNotice(t('platform.connections.registeredConnectionValueKey', { value: created.name ?? name.trim(), key: created.key }));
    setTypedName(null); setFileLabel(''); setHidden(true);
    if (fileInput.current) fileInput.current.value = '';
    replaceDocument('');
    setSaving(false);
    await reload();
  }

  const busy = saving || inspecting || reading;
  return <section className="page"><header className="page-header"><div><p className="eyebrow">{t('platform.connections.platform')}</p><h1>{t('platform.connections.connections')}</h1><p>{t('platform.connections.registerAnExistingKubernetesCluster')}</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>{t('platform.connections.registeredConnections')}</h2></div>{loadError ? <div className="form-error" role="alert" aria-label={t('platform.connections.listLoadError')}>{loadError} <Button type="button" onClick={() => void reload()}>{t('platform.connections.retry')}</Button></div> : loading ? <p>{t('platform.connections.loadingConnections')}</p> : connections.length === 0 ? <p>{t('platform.connections.noConnectionsRegisteredYet')}</p> : <ConnectionTable connections={connections} />}</section>
    <section className="content-panel"><div className="connection-register">
      <div className="source-choices connection-kind" role="radiogroup" aria-label={t('platform.connections.connectionType')}>
        <label className="source-choice"><input type="radio" name="connection-kind" checked={kind === 'kubernetes'} onChange={() => setKind('kubernetes')} /> {' '}{t('platform.connections.kubernetesCluster')}</label>
        <label className="source-choice"><input type="radio" name="connection-kind" checked={kind === 'aws'} onChange={() => setKind('aws')} /> {' '}{t('platform.connections.aWSAccount')}</label>
      </div>
      {kind === 'aws' ? <>
        <div className="section-header"><div><h2>{t('platform.connections.registerAWSAccount')}</h2><p>{t('platform.connections.enterTheLongTermAccess')}</p></div></div>
        <AwsConnectionForm onRegistered={async () => { await reload(); }} />
      </> : <>
      <div className="section-header"><div><h2>{t('platform.connections.registerKubernetesCluster')}</h2><p>{t('platform.connections.uploadOrPasteAKubeconfig2')}</p></div></div>
      <form onSubmit={submit}><fieldset className="form-fieldset connection-form" disabled={saving} aria-busy={busy}>
        <label className="connection-field">{t('platform.connections.connectionName')}<input value={name} required maxLength={maxNameLength} onChange={(event) => setTypedName(event.target.value)} /></label>
        <div className="connection-source">
          <div className="source-choices" role="radiogroup" aria-label={t('platform.connections.kubeconfigSource')}>
            <label className="source-choice"><input type="radio" name="kubeconfig-source" checked={source === 'upload'} onChange={() => chooseSource('upload')} /> {' '}{t('platform.connections.uploadKubeconfigFile')}</label>
            <label className="source-choice"><input type="radio" name="kubeconfig-source" checked={source === 'paste'} onChange={() => chooseSource('paste')} /> {' '}{t('platform.connections.pasteKubeconfigContent')}</label>
          </div>
          {source === 'upload'
            ? <div className="connection-field"><span id={`${formId}-file`}>{t('platform.connections.kubeconfigFile')}</span><div className="file-picker">
                <span className="button file-picker-button">{t('platform.connections.chooseFile')}<input ref={fileInput} type="file" aria-labelledby={`${formId}-file`} aria-describedby={`${formId}-file-name`} onChange={(event) => void chooseFile(event)} /></span>
                <small id={`${formId}-file-name`} className="file-picker-name">{fileLabel ? t('platform.connections.fileSelectedFileLabelTheFile', { fileLabel }) : t('platform.connections.noFileSelected')}</small>
              </div></div>
            : <label className="connection-field">{t('platform.connections.kubeconfigContent')}<textarea aria-label={t('platform.connections.kubeconfigContent')} className={hidden ? 'masked-text' : undefined} rows={8} spellCheck={false} autoComplete="off" value={kubeconfig} onChange={(event) => pasteDocument(event.target.value)} /></label>}
          <div className="source-actions">
            {canRetry ? <Button type="button" disabled={busy || !kubeconfig.trim()} onClick={() => void inspect(kubeconfig, revision.current)}>{t('platform.connections.recheckKubeconfig')}</Button> : null}
            {source === 'paste' ? <label className="inline-check"><input type="checkbox" checked={!hidden} onChange={(event) => setHidden(!event.target.checked)} /> {' '}{t('platform.connections.showContent')}</label> : null}</div>
          {reading || inspecting ? <p className="muted" role="status" aria-live="polite">{reading ? t('platform.connections.readingKubeconfigFile') : t('platform.connections.checkingKubeconfig')}</p> : null}
        </div>
        {contexts.length > 0 ? <div className="connection-destination">
          {contexts.length > 1 ? <label className="connection-field">Context<select value={selected} required onChange={(event) => setSelected(event.target.value)}><option value="">{t('platform.connections.chooseContext')}</option>{contexts.map((context) => <option key={context.name} value={context.name}>{context.name}</option>)}</select></label> : null}
          {chosen ? <dl className="destination-summary" aria-label={t('platform.connections.selectedTarget')}>
            <div><dt>Context</dt> <dd>{chosen.name}</dd></div>
            <div><dt>Cluster</dt> <dd>{chosen.cluster}</dd></div>
            <div><dt>Endpoint</dt> <dd>{chosen.endpoint}</dd></div>
          </dl> : null}
        </div> : null}
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}
        <div className="connection-footer"><Button type="submit" tone="primary" disabled={busy}>{saving ? t('platform.connections.checkingAndSaving') : t('platform.connections.checkAndSave')}</Button></div>
      </fieldset></form>
      </>}
    </div></section>
  </section>;
}

// Destination shows only the public fields that apply to the kind, so AWS
// records never carry empty Kubernetes columns.
function destination(connection: Connection): [string, string][] {
  const config = connection.config ?? {};
  const fields: [string, string | undefined][] = connection.kind === 'AWS'
    ? [[t('platform.connections.regions'), config.allowedRegions?.length ? config.allowedRegions.join(', ') : config.region], [t('platform.connections.account'), config.accountId]]
    : [['Cluster', config.cluster], ['Endpoint', config.endpoint]];
  return fields.filter((field): field is [string, string] => Boolean(field[1]));
}

// Truthful AWS evidence: only a record that carries all three registration
// checks claims them; anything else says no provisioning evidence exists.
function awsEvidence(connection: Connection): string {
  const v = connection.verification;
  return v?.identity === 'VERIFIED' && v.regionCheck === 'VERIFIED' && v.capabilityCheck === 'VERIFIED'
    ? t('platform.connections.verifiedAtRegistrationIdentityEach')
    : t('platform.connections.noEvidenceOfProvisioningPermission');
}

function ConnectionTable({ connections }: { connections: Connection[] }) {
  return <table className="connection-table" aria-label={t('platform.connections.registeredConnections')}>
    <thead><tr><th scope="col">{t('platform.connections.name')}</th><th scope="col">{t('platform.connections.type')}</th><th scope="col">{t('platform.connections.target')}</th><th scope="col">{t('platform.connections.status')}</th></tr></thead>
    <tbody>{connections.map((connection) => {
      const label = connection.name || connection.key;
      const fields = destination(connection);
      return <tr key={connection.key}>
        <td data-label={t('platform.connections.name')}><strong>{label}</strong>{label !== connection.key ? <small className="connection-key">{connection.key}</small> : null}</td>
        <td data-label={t('platform.connections.type')}>{connection.kind}</td>
        <td data-label={t('platform.connections.target')}>{fields.length ? fields.map(([field, value]) => <span className="destination-line" key={field}><span className="muted">{field}</span> {value}</span>) : <span className="muted">—</span>}</td>
        <td data-label={t('platform.connections.status')}><span className={`status-badge${connection.status === 'READY' ? ' status-badge-ready' : ''}`}>{connection.status === 'READY' ? t('platform.connections.ready') : connection.status}</span>{connection.kind === 'AWS' ? <small className="connection-key">{awsEvidence(connection)}</small> : null}</td>
      </tr>;
    })}</tbody>
  </table>;
}
