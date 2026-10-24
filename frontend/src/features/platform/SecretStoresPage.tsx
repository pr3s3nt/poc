import { useCallback, useEffect, useRef, useState } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';

type Store = { key: string; name: string; provider: string; backendAddress: string; workloadAddress: string; mount: string; authMount: string; tlsCustomCa: boolean; status: string; verification?: Record<string, unknown> | null };
type Form = { name: string; backendAddress: string; workloadAddress: string; sameAddress: boolean; mount: string; authMount: string; tlsCaPem: string; token: string };
type Field = 'name' | 'backendAddress' | 'workloadAddress' | 'mount' | 'authMount' | 'tlsCaPem' | 'token' | 'form';
type Problems = Partial<Record<Field, string>>;

// Defaults the backend applies to an empty mount (secretstore.DefaultKVMount / DefaultAuthMount).
export const DEFAULT_MOUNT = 'kv';
export const DEFAULT_AUTH_MOUNT = 'kubernetes';
// Limits mirror the backend registration guard (secretstores/service.go, stores.go).
export const MAX_NAME_CHARS = 100;
export const MAX_TOKEN_BYTES = 4096;
export const MAX_CA_BYTES = 64 << 10;
export const MAX_ENVELOPE_BYTES = 256 << 10;

const empty: Form = { name: '', backendAddress: '', workloadAddress: '', sameAddress: false, mount: '', authMount: '', tlsCaPem: '', token: '' };
const mountPattern = /^[A-Za-z0-9_-]{1,64}$/;
// eslint-disable-next-line no-control-regex -- control characters are exactly what is rejected.
const controlPattern = /[\u0000-\u001f\u007f-\u009f]/;
const base64Pattern = /^[A-Za-z0-9+/]+={0,2}$/;
const bytes = (text: string) => new TextEncoder().encode(text).length;

function addressProblem(raw: string, label: string): string {
  const text = raw.trim();
  if (!text) return t('platform.secretStores.labelIsRequired', { label });
  const fix = t('platform.secretStores.labelMustBeAnHttp', { label });
  // Reject what WHATWG URL would silently normalize (backslashes, control characters) but Go url.Parse refuses.
  if (/\s/.test(text) || /[?#\\]/.test(text) || controlPattern.test(text)) return fix;
  const authority = /^https?:\/\/([^/]*)/i.exec(text)?.[1];
  if (authority === undefined || authority === '' || authority.includes('@')) return fix;
  try {
    const url = new URL(text);
    if ((url.protocol !== 'http:' && url.protocol !== 'https:') || !url.hostname || url.username || url.password || url.search || url.hash) return fix;
  } catch { return fix; }
  return '';
}

function mountProblem(raw: string, label: string): string {
  const text = raw.trim();
  return text === '' || mountPattern.test(text) ? '' : t('platform.secretStores.labelMayContainOnly1', { label });
}

// Shape check only: CERTIFICATE PEM blocks with base64 bodies and nothing else.
// It proves nothing about X.509 validity, trust or reachability; the backend
// parses and verifies for real.
export function caShapeProblem(raw: string): string {
  const text = raw.trim();
  if (!text) return '';
  if (bytes(text) > MAX_CA_BYTES) return t('platform.secretStores.theCAPEMExceeds64');
  const block = /-----BEGIN CERTIFICATE-----\r?\n([A-Za-z0-9+/=\r\n]+?)\r?\n-----END CERTIFICATE-----/y;
  let index = 0;
  let count = 0;
  while (index < text.length) {
    block.lastIndex = index;
    const match = block.exec(text);
    if (!match) return t('platform.secretStores.theCAPEMAcceptsOnly');
    const body = (match[1] ?? '').replace(/\s+/g, '');
    if (body.length % 4 !== 0 || !base64Pattern.test(body)) return t('platform.secretStores.theBase64ContentOfThe');
    count += 1;
    index = block.lastIndex;
    while (index < text.length && /\s/.test(text[index] ?? '')) index += 1;
  }
  return count > 0 ? '' : t('platform.secretStores.theCAPEMNeedsAt');
}

export function buildPayload(form: Form) {
  const backendAddress = form.backendAddress.trim();
  const workloadAddress = form.sameAddress ? backendAddress : form.workloadAddress.trim();
  const tlsCaPem = form.tlsCaPem.trim();
  return { name: form.name.trim(), backendAddress, workloadAddress, mount: form.mount.trim() || DEFAULT_MOUNT, authMount: form.authMount.trim() || DEFAULT_AUTH_MOUNT, ...(tlsCaPem ? { tlsCaPem } : {}), token: form.token };
}

export function validate(form: Form): Problems {
  const problems: Problems = {};
  const name = form.name.trim();
  if (!name) problems.name = t('platform.secretStores.theStoreNameIsRequired');
  else if ([...name].length > MAX_NAME_CHARS || controlPattern.test(name)) problems.name = t('platform.secretStores.theStoreNameCanHave', { MAX_NAME_CHARS });
  const backend = addressProblem(form.backendAddress, t('platform.secretStores.backendAddress'));
  if (backend) problems.backendAddress = backend;
  if (!form.sameAddress) {
    const workload = addressProblem(form.workloadAddress, t('platform.secretStores.workloadAddress'));
    if (workload) problems.workloadAddress = workload;
  }
  const mount = mountProblem(form.mount, t('platform.secretStores.kvV2Mount'));
  if (mount) problems.mount = mount;
  const authMount = mountProblem(form.authMount, t('platform.secretStores.kubernetesAuthMount'));
  if (authMount) problems.authMount = authMount;
  const ca = caShapeProblem(form.tlsCaPem);
  if (ca) problems.tlsCaPem = ca;
  const token = form.token.trim();
  if (!token) problems.token = t('platform.secretStores.theTokenIsRequired');
  else if (bytes(token) > MAX_TOKEN_BYTES || controlPattern.test(token)) problems.token = t('platform.secretStores.theTokenCanHaveAt', { MAX_TOKEN_BYTES });
  if (Object.keys(problems).length === 0 && bytes(JSON.stringify(buildPayload(form))) > MAX_ENVELOPE_BYTES) problems.form = t('platform.secretStores.theSubmittedDataExceeds256');
  return problems;
}

// Fixed Vietnamese guidance by stable code or HTTP status. Raw backend text is never shown.
export function failureGuidance(error: unknown): string {
  if (!(error instanceof ApiError)) return t('platform.secretStores.noResponseFromTheServer');
  switch (error.status) {
    case 400: return t('platform.secretStores.theServerRejectedTheRegistration');
    case 401: return t('platform.secretStores.yourSessionHasExpiredSign');
    case 403: return t('platform.secretStores.yourAccountIsNotAllowed');
    case 413: return t('platform.secretStores.theRegistrationDataIsToo');
    case 422: return t('platform.secretStores.couldNotVerifyTheSecret');
    case 503: return t('platform.secretStores.thePlatformCredentialStoreIs');
    default: return error.status >= 500 ? t('platform.secretStores.theServerFailedDuringRegistration') : t('platform.secretStores.couldNotRegisterTheSecret');
  }
}

export function backendVerification(store: Store): string {
  const verified = store.verification?.verified;
  if (verified === true) return t('platform.secretStores.backendVerifiedAtRegistration');
  if (verified === false) return t('platform.secretStores.backendVerificationFailed');
  return t('platform.secretStores.noBackendVerificationResultYet');
}

export function kubernetesAuth(store: Store): string {
  switch (String(store.verification?.kubernetesAuth ?? '').toUpperCase()) {
    case 'CONFIGURED': return t('platform.secretStores.configured');
    case 'ABSENT': return t('platform.secretStores.notConfigured');
    default: return t('platform.secretStores.unknown');
  }
}

export function statusLabel(status: string): string {
  switch (status) {
    case 'READY': return t('platform.secretStores.ready');
    case 'REJECTED': return t('platform.secretStores.rejected');
    case '': return t('platform.secretStores.unknown');
    default: return t('platform.secretStores.unrecognizedStatusStatus', { status });
  }
}

// Platform Engineer registration of Organization-scoped Vault KV v2 stores
// (UC-04 SS-01..06, T05). The token is write-only: it is cleared after every
// server failure and after success, and never shown again; the list shows only
// safe metadata.
export function SecretStoresPage() {
  const [stores, setStores] = useState<readonly Store[]>([]);
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading');
  const [form, setForm] = useState<Form>(empty);
  const [advanced, setAdvanced] = useState(false);
  const [saving, setSaving] = useState(false);
  const [problems, setProblems] = useState<Problems>({});
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const submitting = useRef(false);
  const load = useCallback(async () => {
    setState('loading');
    try { const response = await api<{ secretStores: Store[] }>('/secret-stores'); setStores(response.secretStores ?? []); setState('ready'); } catch { setState('error'); }
  }, []);
  useEffect(() => { void load(); }, [load]);

  async function register() {
    if (submitting.current) return;
    const found = validate(form);
    setProblems(found); setSuccess('');
    if (Object.keys(found).length > 0) {
      setError('');
      // Advanced fields must not hide an actionable error.
      if (found.mount || found.authMount || found.tlsCaPem) setAdvanced(true);
      return;
    }
    submitting.current = true;
    setSaving(true); setError('');
    let created: Store;
    try {
      created = await api<Store>('/secret-stores', { method: 'POST', body: JSON.stringify(buildPayload(form)) });
    } catch (err) {
      // The token is cleared on every server failure; metadata and choices stay.
      setForm((value) => ({ ...value, token: '' }));
      setError(failureGuidance(err));
      submitting.current = false; setSaving(false);
      return;
    }
    setForm(empty); setAdvanced(false); setProblems({});
    setSuccess(t('platform.secretStores.secretStoreNameIsReady', { name: created.name }));
    submitting.current = false; setSaving(false);
    await load();
  }
  const set = (key: 'name' | 'backendAddress' | 'workloadAddress' | 'mount' | 'authMount' | 'tlsCaPem' | 'token') => (event: { target: { value: string } }) => setForm((value) => ({ ...value, [key]: event.target.value }));
  // Help and error text sit outside the <label> so the accessible name stays the bare field name.
  const describe = (field: Field) => ({ 'aria-describedby': `ss-${field}-help${problems[field] ? ` ss-${field}-error` : ''}`, ...(problems[field] ? { 'aria-invalid': true } : {}) });
  const note = (field: Field, help: string) => <><small id={`ss-${field}-help`} className="muted">{help}</small>{problems[field] ? <small id={`ss-${field}-error`} className="field-error" role="alert">{problems[field]}</small> : null}</>;
  const effectiveMount = form.mount.trim() || DEFAULT_MOUNT;
  const effectiveAuthMount = form.authMount.trim() || DEFAULT_AUTH_MOUNT;
  const workloadValue = form.sameAddress ? form.backendAddress : form.workloadAddress;
  return <section className="page">
    <header className="page-header"><div><p className="eyebrow">{t('platform.secretStores.platform')}</p><h1>{t('platform.secretStores.secretStores')}</h1><p>{t('platform.secretStores.registerVaultKVV2Stores')}</p></div></header>
    {success ? <div className="form-success" role="status">{success}</div> : null}
    <section className="content-panel" aria-label={t('platform.secretStores.registeredSecretStores')}>
      <div className="section-header"><div><h2>{t('platform.secretStores.registeredStores')}</h2><p>{t('platform.secretStores.readyMeansTheBackendWas')}</p></div></div>
      {state === 'loading' ? <p role="status" aria-busy="true">{t('platform.secretStores.loadingSecretStores')}</p> : null}
      {state === 'error' ? <div className="form-error" role="alert">{t('platform.secretStores.couldNotLoadTheSecret')}<Button type="button" onClick={() => void load()}>{t('platform.secretStores.retry')}</Button></div> : null}
      {state === 'ready' && stores.length === 0 ? <div className="section-empty">{t('platform.secretStores.noSecretStoresRegisteredYet')}</div> : null}
      {stores.length > 0 ? <div className="settings-table"><div className="settings-table-head"><span>{t('platform.secretStores.store')}</span><span>{t('platform.secretStores.address')}</span><span>{t('platform.secretStores.mountAndCA')}</span><span>{t('platform.secretStores.statusAndVerification')}</span></div>
        {stores.map((store) => <div className="settings-table-row" key={store.key}>
          <strong>{store.name}<small className="muted"> {store.key} · {store.provider}</small></strong>
          <span>Backend: {store.backendAddress}<br />Workload: {store.workloadAddress}</span>
          <span>KV v2: {store.mount}<br />{t('platform.secretStores.kubernetesAuth')}{' '} {store.authMount}<br />CA: {store.tlsCustomCa ? t('platform.secretStores.custom') : t('platform.secretStores.default')}</span>
          <span>{t('platform.secretStores.status')}{' '} {statusLabel(store.status)}<br />{t('platform.secretStores.backendVerification')}{' '} {backendVerification(store)}<br />{t('platform.secretStores.kubernetesAuth')}{' '} {kubernetesAuth(store)}</span></div>)}</div> : null}
    </section>
    <form className="create-form secret-store-form" aria-label={t('platform.secretStores.registerSecretStore')} noValidate onSubmit={(event) => { event.preventDefault(); void register(); }}>
      <h2>{t('platform.secretStores.registerVaultKVV2Store')}</h2>
      {error ? <div className="form-error" role="alert">{error}</div> : null}
      {problems.form ? <div className="form-error" role="alert">{problems.form}</div> : null}
      <fieldset className="form-fieldset" disabled={saving} aria-busy={saving}>
        <div className="editor-grid">
          <div className="field"><label>{t('platform.secretStores.storeName')}<input value={form.name} onChange={set('name')} placeholder={t('platform.secretStores.teamVault')} {...describe('name')} /></label>{note('name', t('platform.secretStores.atMost100Characters'))}</div>
          <div className="field"><label>{t('platform.secretStores.backendAddress')}<input value={form.backendAddress} onChange={set('backendAddress')} placeholder="https://vault.example.com:8200" {...describe('backendAddress')} /></label>{note('backendAddress', t('platform.secretStores.orchestratorUsesThisAddressTo'))}</div>
          <label className="checkbox"><input type="checkbox" checked={form.sameAddress} onChange={(event) => setForm((value) => ({ ...value, sameAddress: event.target.checked }))} />{t('platform.secretStores.useTheSameBackendAddress')}</label>
          <div className="field"><label>{t('platform.secretStores.workloadAddress')}<input value={workloadValue} onChange={set('workloadAddress')} readOnly={form.sameAddress} aria-readonly={form.sameAddress} placeholder="http://vault.vault.svc:8200" {...describe('workloadAddress')} /></label>{note('workloadAddress', form.sameAddress ? t('platform.secretStores.usingTheBackendAddressUncheck') : t('platform.secretStores.theAddressWorkloadPodsAnd'))}</div>
          <div className="field"><label>{t('platform.secretStores.vaultToken')}<input type="password" autoComplete="off" value={form.token} onChange={set('token')} {...describe('token')} /></label>{note('token', t('platform.secretStores.requiresKVV2ValuePermissions'))}</div>
        </div>
        <section className="form-info" aria-label={t('platform.secretStores.effectiveConfigurationSummary')}>
          <strong>{t('platform.secretStores.effectiveConfiguration')}</strong>
          <ul>
            <li>{t('platform.secretStores.kvV2MountLabel')}{' '} <code>{effectiveMount}</code>{form.mount.trim() ? '' : t('platform.secretStores.default2')}</li>
            <li>{t('platform.secretStores.kubernetesAuthMount2')}{' '} <code>{effectiveAuthMount}</code>{form.authMount.trim() ? '' : t('platform.secretStores.default2')}</li>
            <li>{t('platform.secretStores.cAValueTLSIsAlways', { value: form.tlsCaPem.trim() ? t('platform.secretStores.custom') : t('platform.secretStores.systemDefault') })}</li>
          </ul>
        </section>
        <div className="section-header"><div><h3>{t('platform.secretStores.advanced')}</h3><p>{t('platform.secretStores.optionalMountAndCAPEM')}</p></div><Button type="button" aria-expanded={advanced} aria-controls="secret-store-advanced" onClick={() => setAdvanced(!advanced)}>{advanced ? t('platform.secretStores.hideAdvanced') : t('platform.secretStores.showAdvanced')}</Button></div>
        <div id="secret-store-advanced" hidden={!advanced} className="editor-grid">
          <div className="field"><label>{t('platform.secretStores.kvV2Mount')}<input value={form.mount} onChange={set('mount')} placeholder={DEFAULT_MOUNT} {...describe('mount')} /></label>{note('mount', t('platform.secretStores.leaveEmptyToUseThe', { DEFAULT_MOUNT }))}</div>
          <div className="field"><label>{t('platform.secretStores.kubernetesAuthMount')}<input value={form.authMount} onChange={set('authMount')} placeholder={DEFAULT_AUTH_MOUNT} {...describe('authMount')} /></label>{note('authMount', t('platform.secretStores.leaveEmptyToUseThe2', { DEFAULT_AUTH_MOUNT }))}</div>
          <div className="field"><label>{t('platform.secretStores.customCACertificatePEMOptional')}<textarea value={form.tlsCaPem} onChange={set('tlsCaPem')} rows={4} spellCheck={false} {...describe('tlsCaPem')} /></label>{note('tlsCaPem', t('platform.secretStores.theConsoleOnlyChecksThe'))}</div>
        </div>
        <div className="form-actions"><Button tone="primary" type="submit" disabled={saving}>{saving ? t('platform.secretStores.verifying') : t('platform.secretStores.verifyAndRegister')}</Button></div>
      </fieldset>
    </form>
  </section>;
}
