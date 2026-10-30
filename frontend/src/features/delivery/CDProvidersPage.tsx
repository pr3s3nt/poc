import { useEffect, useRef, useState, type FormEvent } from 'react';
import { navigate } from '../../app/routes';
import { ApiError, describeError } from '../../shared/api/client';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import { listAccounts, type Account } from '../platform/accounts/accountsApi';
import { createBinding, inspectProvider, type Discovery, getBinding, getBindings, getProviders, verifyBinding, type BindingDetails, type BindingInput, type BindingSummary, type ConfigField, type Provider } from './platformApi';

type Form = { name: string; provider: string; account: string; config: Record<string, string>; username: string; password: string; retainCredential: boolean; previous?: BindingDetails };
const empty: Form = { name: '', provider: '', account: '', config: {}, username: '', password: '', retainCredential: false };
const key = (b: BindingSummary) => `${b.id}:${b.version}`;
const defaults = (p?: Provider) => Object.fromEntries((p?.fields ?? []).map(f => [f.key, f.default ?? '']));
const statusText = (status: string) => t(`cd.status.${['READY', 'UNVERIFIED', 'REJECTED', 'DISABLED'].includes(status) ? status : 'UNKNOWN'}`);

export function CDProvidersPage() {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [bindings, setBindings] = useState<BindingSummary[]>([]);
  const [form, setForm] = useState<Form>();
  const [discovery, setDiscovery] = useState<Discovery>();
  const [details, setDetails] = useState<BindingDetails>();
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [unknown, setUnknown] = useState(false);
  const [reload, setReload] = useState(0);
  const generation = useRef(0);
  useEffect(() => {
    const current = ++generation.current;
    setLoading(true);
    Promise.all([getProviders(), listAccounts(), getBindings()]).then(([p, a, b]) => {
      if (generation.current !== current) return;
      setProviders(p); setAccounts(a); setBindings(b); setError('');
    }).catch(() => { if (generation.current === current) setError(t('cd.loadFailed')); }).finally(() => { if (generation.current === current) setLoading(false); });
    return () => { generation.current += 1; };
  }, [reload]);
  const eligibleAccounts = accounts.filter(a => a.type === 'INTERNAL' && a.status === 'READY');
  const provider = providers.find(p => p.kind === form?.provider);
  const account = eligibleAccounts.find(a => a.key === form?.account);
  function edit(next: Form) { setForm(next); setError(''); setNotice(''); }
  async function open(b: BindingSummary, editing: boolean) {
    const current = generation.current; setBusy(true); setError(''); setNotice('');
    try {
      const d = await getBinding(b.id, b.version);
      if (generation.current !== current) return;
      setDetails(d);
      if (editing) setForm({ name: d.binding.name, provider: d.binding.provider, account: d.managementAccountKey, config: { ...defaults(providers.find(p => p.kind === d.binding.provider)), ...d.config }, username: '', password: '', retainCredential: d.credentialConfigured, previous: d });
      else setForm(undefined);
    } catch (err) { if (generation.current === current) setError(describeError(err, t('cd.loadFailed'))); }
    finally { if (generation.current === current) setBusy(false); }
  }
  async function verify(b: BindingSummary) {
    const current = generation.current; setBusy(true); setError(''); setNotice('');
    try {
      const next = await verifyBinding(b);
      if (generation.current !== current) return;
      setBindings(old => old.map(item => key(item) === key(next) ? next : item));
      setDetails(undefined);
      setNotice(next.status === 'READY' ? t('cd.verified') : t('cd.verifyFailed'));
    } catch (err) { if (generation.current === current) setError(describeError(err, t('cd.verifyFailed'))); }
    finally { if (generation.current === current) setBusy(false); }
  }
  async function inspect() {
    if (!form || !account) return;
    const current = generation.current; setBusy(true); setError('');
    try {
      const found = await inspectProvider(form.provider, account, form.config);
      if (generation.current !== current) return;
      setDiscovery(found);
      const only = found.options.targetCluster?.length === 1 ? found.options.targetCluster[0] : undefined;
      if (only) setForm(old => old ? { ...old, config: { ...old.config, ...only.values } } : old);
    } catch (err) { if (generation.current === current) setError(describeError(err, t('cd.inspectFailed'))); }
    finally { if (generation.current === current) setBusy(false); }
  }
  async function save(event: FormEvent) {
    event.preventDefault();
    if (!form || !provider || !account || busy || unknown) return;
    const current = generation.current;
    const input: BindingInput = { name: form.name.trim(), provider: form.provider, managementAccountKey: account.key, managementAccountVersion: account.version, config: form.config, writerUsername: form.retainCredential ? '' : form.username, writerPassword: form.retainCredential ? '' : form.password };
    if (form.previous) Object.assign(input, { previousVersion: form.previous.binding.version, expectedRevision: form.previous.binding.revision, retainWriterCredential: form.retainCredential });
    setBusy(true); setError(''); setNotice('');
    // Credential controls clear for every submitted outcome. Unknown mutations
    // stay held for explicit review instead of enabling another POST.
    setForm(old => old ? { ...old, username: '', password: '' } : old);
    try {
      const saved = await createBinding(input, form.previous?.binding.id);
      if (generation.current !== current) return;
      setBindings(old => [...old, saved]); setForm(undefined); setDetails(undefined); setNotice(t('cd.saved'));
    } catch (err) {
      if (generation.current !== current) return;
      setError(describeError(err, t('cd.saveUnknown')));
      if (!(err instanceof ApiError) || err.status >= 500) setUnknown(true);
    } finally { if (generation.current === current) setBusy(false); }
  }
  const locked = busy || loading || unknown;
  function field(f: ConfigField) {
    if (!form || (f.visibleWhenKey && form.config[f.visibleWhenKey] !== f.visibleWhenValue)) return null;
    const value = form.config[f.key] ?? '';
    const props = { id: `cd-${f.key}`, value, required: f.required, disabled: locked, readOnly: f.readOnly, onChange: (event: { target: { value: string } }) => edit({ ...form, config: { ...form.config, ...(f.key === 'gitRepoMode' && event.target.value === 'MANAGED' ? { existingGitRepo: '', existingGitRepoUID: '' } : {}), [f.key]: event.target.value } }) };
    return <div key={f.key} className="editor-grid"><label htmlFor={props.id}>{t(f.labelKey)}</label>{f.choices ? <select {...props}>{f.choices.map(c => <option key={c.value} value={c.value}>{t(c.labelKey)}</option>)}</select> : discovery?.options[f.key] ? <select {...props} onChange={e => { const option = discovery.options[f.key]?.find(item => item.value === e.target.value); edit({ ...form, config: { ...form.config, ...option?.values, [f.key]: e.target.value } }); }}><option value="">{t('cd.choose')}</option>{discovery.options[f.key]?.map(o => <option key={o.value} value={o.value}>{o.name}</option>)}</select> : f.kind === 'account' ? <select {...props}><option value="">{t('cd.chooseAccount')}</option>{eligibleAccounts.map(a => <option key={a.key} value={a.key}>{a.name}</option>)}</select> : f.kind === 'textarea' ? <textarea {...props} rows={4} /> : <input {...props} type={f.kind === 'url' ? 'url' : 'text'} autoComplete="off" />}</div>;
  }
  return <section className="page">
    <header className="page-header"><div><p className="eyebrow">{t('cd.platform')}</p><h1>{t('cd.title')}</h1></div><Button tone="primary" disabled={locked || providers.length === 0} onClick={() => { const p = providers.length === 1 ? providers[0] : undefined; setDiscovery(undefined); setForm({ ...empty, provider: p?.kind ?? '', config: defaults(p) }); setDetails(undefined); setError(''); setNotice(''); }}>{t('cd.add')}</Button></header>
    {loading ? <p role="status">{t('cd.loading')}</p> : null}
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {notice ? <div className="form-info" role="status">{notice}</div> : null}
    {unknown ? <p className="form-info" role="status">{t('cd.saveUnknown')}</p> : null}
    {!loading ? <section className="content-panel"><div className="section-header"><h2>{t('cd.registered')}</h2><Button disabled={busy} onClick={() => setReload(n => n + 1)}>{t('cd.reload')}</Button></div>
      {providers.length === 0 ? <p className="form-info">{t('cd.notConfigured')}</p> : null}
      {bindings.length === 0 ? <div className="section-empty">{t('cd.empty')}</div> : <table className="plain-table"><thead><tr><th>{t('cd.name')}</th><th>{t('cd.provider')}</th><th>{t('cd.version')}</th><th>{t('cd.status')}</th><th>{t('cd.actions')}</th></tr></thead><tbody>{bindings.map(b => <tr key={key(b)}><td>{b.name}</td><td>{b.provider}</td><td>{b.version}</td><td>{statusText(b.status)}</td><td><div className="row-actions"><Button disabled={busy} onClick={() => void open(b, false)}>{t('cd.view')}</Button><Button disabled={locked} onClick={() => void open(b, true)}>{t('cd.edit')}</Button><Button disabled={locked} onClick={() => void verify(b)}>{t('cd.verify')}</Button></div></td></tr>)}</tbody></table>}
    </section> : null}
    {form ? <form className="content-panel create-form secret-store-form" onSubmit={event => void save(event)}><h2>{form.previous ? t('cd.newVersion') : t('cd.add')}</h2>
      <label>{t('cd.name')}<input value={form.name} required maxLength={120} disabled={locked} onChange={e => edit({ ...form, name: e.target.value })} /></label>
      <label htmlFor="cd-provider-kind">{t('cd.provider')}</label><select id="cd-provider-kind" required value={form.provider} disabled={locked || Boolean(form.previous)} onChange={e => { const p = providers.find(item => item.kind === e.target.value); edit({ ...form, provider: e.target.value, config: defaults(p) }); }}><option value="">{t('cd.chooseProvider')}</option>{providers.map(p => <option key={p.kind} value={p.kind}>{p.kind}</option>)}</select>
      <label htmlFor="cd-management-account">{t('cd.managementAccount')}</label><select id="cd-management-account" required value={form.account} disabled={locked} onChange={e => (setDiscovery(undefined), edit({ ...form, account: e.target.value, config: { ...form.config, targetClusterUID: '' } }))}><option value="">{t('cd.chooseAccount')}</option>{eligibleAccounts.map(a => <option key={a.key} value={a.key}>{a.name}</option>)}</select>
      {eligibleAccounts.length === 0 ? <p className="form-info">{t('cd.noAccount')}</p> : null}
      <Button type="button" disabled={locked || !account || !form.config.workspace} onClick={() => void inspect()}>{t('cd.inspect')}</Button>
      {(provider?.fields ?? []).filter(f => !f.advanced).map(field)}
      <details><summary>{t('cd.connectionDetails')}</summary><div className="editor-grid">{(provider?.fields ?? []).filter(f => f.advanced).map(field)}</div></details>
      {form.previous?.credentialConfigured ? <label className="checkbox"><input type="checkbox" checked={form.retainCredential} disabled={locked} onChange={e => edit({ ...form, retainCredential: e.target.checked, username: '', password: '' })} />{t('cd.retainCredential')}</label> : null}
      {!form.retainCredential ? <details><summary>{t('cd.gitCredentials')}</summary><div className="editor-grid"><label>{t('cd.gitUsername')}<input value={form.username} disabled={locked} autoComplete="off" onChange={e => edit({ ...form, username: e.target.value })} /></label><label>{t('cd.gitToken')}<input type="password" value={form.password} disabled={locked} autoComplete="new-password" onChange={e => edit({ ...form, password: e.target.value })} /></label></div></details> : null}
      <div className="form-actions"><Button type="button" disabled={busy} onClick={() => setForm(undefined)}>{t('cd.cancel')}</Button><Button type="submit" tone="primary" disabled={locked || !provider || !account}>{t('cd.save')}</Button></div>
    </form> : null}
    {details && !form ? <section className="content-panel"><h2>{details.binding.name} · {t('cd.version')} {details.binding.version}</h2><dl>{Object.entries(details.config).filter(([k]) => k !== 'caBundle').map(([k, v]) => <div key={k}><dt>{t(`cd.fields.${k}`)}</dt><dd>{v || '—'}</dd></div>)}</dl><h3>{t('cd.usage')}</h3>{details.usage.length ? <ul>{details.usage.map(u => <li key={`${u.applicationKey}:${u.environmentKey}`}><Button onClick={() => navigate({ name: 'settings', applicationId: u.applicationKey, environment: u.environmentKey === 'production' ? 'production' : 'staging' })}>{u.applicationKey} / {u.environmentKey}</Button> {u.applied ? t('cd.applied') : t('cd.desired')}</li>)}</ul> : <p>{t('cd.unused')}</p>}</section> : null}
  </section>;
}
