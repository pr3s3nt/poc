import { useEffect, useRef, useState, type FormEvent } from 'react';
import { navigate } from '../../app/routes';
import { api, ApiError, describeError } from '../../shared/api/client';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import { listAccounts, type Account } from '../platform/accounts/accountsApi';
import { createBinding, inspectProvider, type Discovery, getBinding, getBindings, getProviders, getPlatformDefault, setPlatformDefault, type PlatformDefault, verifyBinding, type BindingDetails, type BindingInput, type BindingSummary, type ConfigField, type Provider } from './platformApi';

type Form = { usePlatformGit?: boolean; name: string; provider: string; account: string; config: Record<string, string>; username: string; password: string; retainCredential: boolean; previous?: BindingDetails };
const empty: Form = { name: '', provider: '', account: '', config: {}, username: '', password: '', retainCredential: false };
const key = (b: BindingSummary) => `${b.id}:${b.version}`;
const defaults = (p?: Provider) => Object.fromEntries((p?.fields ?? []).map(f => [f.key, f.default ?? '']));
const statusText = (status: string) => t(`cd.status.${['READY', 'UNVERIFIED', 'REJECTED', 'DISABLED'].includes(status) ? status : 'UNKNOWN'}`);

export function CDProvidersPage() {
  const [platformGit, setPlatformGit] = useState<{ tokenConfigured: boolean; url: string; username: string; repositoryOwner?: string }>();
  const [providers, setProviders] = useState<Provider[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [bindings, setBindings] = useState<BindingSummary[]>([]);
  const [platformDefault, setDefault] = useState<PlatformDefault>({ bindingId: '', bindingVersion: 0, revision: 0 });
  const [form, setForm] = useState<Form>();
  const [inspecting, setInspecting] = useState(false);
  const [inspectError, setInspectError] = useState('');
  const [discovery, setDiscovery] = useState<Discovery>();
  const [details, setDetails] = useState<BindingDetails>();
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [unknown, setUnknown] = useState(false);
  const [defaultUncertain, setDefaultUncertain] = useState(false);
  const [reload, setReload] = useState(0);
  const generation = useRef(0);
  useEffect(() => {
    const current = ++generation.current;
    setLoading(true);
    void api<{ settings: { tokenConfigured: boolean; url: string; username: string; repositoryOwner?: string } }>('/platform/settings/git').then(r => { if (generation.current === current) setPlatformGit(r.settings); }).catch(() => {});
    Promise.all([getProviders(), listAccounts(), getBindings(), getPlatformDefault()]).then(([p, a, b, d]) => {
      if (generation.current !== current) return;
      setProviders(p); setAccounts(a); setBindings(b); setDefault(d); setDefaultUncertain(false); setError('');
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
      if (editing) setForm({ usePlatformGit: d.usePlatformGit, name: d.binding.name, provider: d.binding.provider, account: d.managementAccountKey, config: { ...defaults(providers.find(p => p.kind === d.binding.provider)), ...d.config }, username: '', password: '', retainCredential: d.credentialConfigured && !d.usePlatformGit, previous: d });
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
      setNotice(next.status === 'READY' ? t('cd.verified') : t(`cd.reason.${next.reasonCode ?? 'VERIFICATION_FAILED'}`));
    } catch (err) { if (generation.current === current) setError(describeError(err, t('cd.verifyFailed'))); }
    finally { if (generation.current === current) setBusy(false); }
  }
  async function selectDefault(binding: BindingSummary) {
    const current = generation.current; setBusy(true); setError(''); setNotice('');
    try {
      const next = await setPlatformDefault(binding, platformDefault);
      if (generation.current !== current) return;
      setDefault(next); setNotice(t('cd.defaultSaved'));
    } catch (err) {
      if (generation.current === current) { setError(describeError(err, t('cd.defaultFailed'))); setDefaultUncertain(true); }
    } finally { if (generation.current === current) setBusy(false); }
  }
  async function inspect(nextForm = form, nextAccount = account) {
    if (!nextForm?.account || !nextForm.provider || !nextAccount || nextAccount.key !== nextForm.account) return;
    const current = generation.current; setBusy(true); setInspecting(true); setInspectError(''); setError('');
    try {
      const found = await inspectProvider(nextForm.provider, nextAccount, nextForm.config);
      if (generation.current !== current) return;
      setDiscovery(found);
      const only = found.options.targetCluster?.length === 1 ? found.options.targetCluster[0] : undefined;
      if (only) setForm(old => old ? { ...old, config: { ...old.config, ...only.values } } : old);
    } catch { if (generation.current === current) setInspectError(t('cd.inspectFailed')); }
    finally { if (generation.current === current) { setBusy(false); setInspecting(false); } }
  }
  async function save(event: FormEvent) {
    event.preventDefault();
    if (!form || !provider || !account || busy || unknown) return;
    const current = generation.current;
    const input: BindingInput = { usePlatformGit: Boolean(form.usePlatformGit), name: form.name.trim(), provider: form.provider, managementAccountKey: account.key, managementAccountVersion: account.version, config: form.config, writerUsername: form.retainCredential || form.usePlatformGit ? '' : form.username, writerPassword: form.retainCredential || form.usePlatformGit ? '' : form.password };
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
  const locked = busy || loading || unknown || defaultUncertain;
  function field(f: ConfigField) {
    if (!form || (f.key === 'repositoryOwner' && form.usePlatformGit) || (f.visibleWhenKey && form.config[f.visibleWhenKey] !== f.visibleWhenValue)) return null;
    const value = form.config[f.key] ?? '';
    const props = { id: `cd-${f.key}`, value, required: f.required, disabled: locked, readOnly: f.readOnly, onChange: (event: { target: { value: string } }) => edit({ ...form, config: { ...form.config, ...(f.key === 'gitRepoMode' && event.target.value === 'MANAGED' ? { existingGitRepo: '', existingGitRepoUID: '' } : {}), [f.key]: event.target.value } }) };
    return <div key={f.key} className="editor-grid"><label htmlFor={props.id}>{t(f.labelKey)}</label>{f.choices ? <select {...props}>{f.choices.map(c => <option key={c.value} value={c.value}>{t(c.labelKey)}</option>)}</select> : discovery?.options[f.key] ? <select {...props} onChange={e => { const option = discovery.options[f.key]?.find(item => item.value === e.target.value); edit({ ...form, config: { ...form.config, ...option?.values, [f.key]: e.target.value } }); }}><option value="">{t('cd.choose')}</option>{discovery.options[f.key]?.map(o => <option key={o.value} value={o.value}>{o.name}</option>)}</select> : f.kind === 'account' ? <select {...props}><option value="">{t('cd.chooseAccount')}</option>{eligibleAccounts.map(a => <option key={a.key} value={a.key}>{a.name}</option>)}</select> : f.kind === 'textarea' ? <textarea {...props} rows={4} /> : <input {...props} type={f.kind === 'url' ? 'url' : 'text'} autoComplete="off" />}</div>;
  }
  return <section className="page">
    <header className="page-header"><div><p className="eyebrow">{t('cd.platform')}</p><h1>{t('cd.title')}</h1></div><Button tone="primary" disabled={locked || providers.length === 0} onClick={() => { const p = providers.length === 1 ? providers[0] : undefined; setDiscovery(undefined); setInspectError(''); setForm({ ...empty, usePlatformGit: Boolean(platformGit?.tokenConfigured), provider: p?.kind ?? '', config: defaults(p) }); setDetails(undefined); setError(''); setNotice(''); }}>{t('cd.add')}</Button></header>
    {loading ? <p role="status">{t('cd.loading')}</p> : null}
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {notice ? <div className="form-info" role="status">{notice}</div> : null}
    {unknown ? <p className="form-info" role="status">{t('cd.saveUnknown')}</p> : null}
    {!loading ? <section className="content-panel"><div className="section-header"><h2>{t('cd.registered')}</h2><Button disabled={busy} onClick={() => setReload(n => n + 1)}>{t('cd.reload')}</Button></div>
      {providers.length === 0 ? <p className="form-info">{t('cd.notConfigured')}</p> : null}
      {bindings.length === 0 ? <div className="section-empty">{t('cd.empty')}</div> : <table className="plain-table"><thead><tr><th>{t('cd.name')}</th><th>{t('cd.provider')}</th><th>{t('cd.version')}</th><th>{t('cd.status')}</th><th>{t('cd.actions')}</th></tr></thead><tbody>{bindings.map(b => <tr key={key(b)}><td>{b.name}{platformDefault.bindingId === b.id && platformDefault.bindingVersion === b.version ? <span className="pending-pill">{t('cd.default')}</span> : null}</td><td>{b.provider}</td><td>{b.version}</td><td>{statusText(b.status)}</td><td><div className="row-actions"><Button disabled={busy} onClick={() => void open(b, false)}>{t('cd.view')}</Button><Button disabled={locked} onClick={() => void open(b, true)}>{t('cd.edit')}</Button><Button disabled={locked} onClick={() => void verify(b)}>{t('cd.verify')}</Button>{b.status === 'READY' ? <Button disabled={locked || (platformDefault.bindingId === b.id && platformDefault.bindingVersion === b.version)} onClick={() => void selectDefault(b)}>{t('cd.useDefault')}</Button> : null}</div></td></tr>)}</tbody></table>}
    </section> : null}
    {form ? <form className="content-panel create-form secret-store-form" onSubmit={event => void save(event)}><h2>{form.previous ? t('cd.newVersion') : t('cd.add')}</h2>
      <label>{t('cd.name')}<input value={form.name} required maxLength={120} disabled={locked} onChange={e => edit({ ...form, name: e.target.value })} /></label>
      <label htmlFor="cd-provider-kind">{t('cd.provider')}</label><select id="cd-provider-kind" required value={form.provider} disabled={locked || Boolean(form.previous)} onChange={e => { const p = providers.find(item => item.kind === e.target.value); edit({ ...form, provider: e.target.value, config: defaults(p) }); }}><option value="">{t('cd.chooseProvider')}</option>{providers.map(p => <option key={p.kind} value={p.kind}>{p.kind}</option>)}</select>
      <label htmlFor="cd-management-account">{t('cd.managementAccount')}</label><select id="cd-management-account" required value={form.account} disabled={locked} onChange={e => { setDiscovery(undefined); setInspectError(''); const next = { ...form, account: e.target.value, config: { ...form.config, targetAccountKey: e.target.value, targetClusterUID: '' } }; edit(next); void inspect(next, eligibleAccounts.find(a => a.key === e.target.value)); }}><option value="">{t('cd.chooseAccount')}</option>{eligibleAccounts.map(a => <option key={a.key} value={a.key}>{a.name}</option>)}</select>
      {form.config.targetClusterUID ? <p className="form-info" role="status">{t('cd.discovered')} {form.config.workspace} / {form.config.targetCluster} · {form.config.controllerNamespace}/{form.config.controllerName}</p> : null}
      {eligibleAccounts.length === 0 ? <p className="form-info">{t('cd.noAccount')}</p> : null}
      <Button type="button" disabled={locked || !account} onClick={() => void inspect()}>{t(inspecting ? 'cd.inspecting' : 'cd.inspect')}</Button>
      {inspecting ? <p role="status">{t('cd.inspecting')}</p> : null}
      {inspectError ? <p className="form-error" role="alert">{inspectError}</p> : null}
      {(provider?.fields ?? []).filter(f => !f.advanced).map(field)}
      <details><summary>{t('cd.connectionDetails')}</summary><div className="editor-grid">{(provider?.fields ?? []).filter(f => f.advanced).map(field)}</div></details>
      {!form.usePlatformGit && !form.previous?.usePlatformGit && form.previous?.credentialConfigured ? <label className="checkbox"><input type="checkbox" checked={form.retainCredential} disabled={locked} onChange={e => edit({ ...form, retainCredential: e.target.checked, username: '', password: '' })} />{t('cd.retainCredential')}</label> : null}
      <label className="checkbox"><input type="checkbox" checked={Boolean(form.usePlatformGit)} disabled={locked} onChange={e => edit({ ...form, usePlatformGit: e.target.checked, retainCredential: false, username: '', password: '' })} />{t('cd.usePlatformGit')}</label>
      {form.usePlatformGit ? <p className="form-info">{platformGit?.tokenConfigured ? `${platformGit.url} · ${platformGit.username}` : t('cd.platformGitMissing')} {t('platform.git.repositoryOwner')}: {platformGit?.repositoryOwner || t('platform.git.ownerMissing')} <a href="/ui/platform/settings">{t('cd.platformGitSettings')}</a></p> : null}
      {!form.retainCredential && !form.usePlatformGit ? <div className="editor-grid"><label>{t('cd.gitUsername')}<input required value={form.username} disabled={locked} autoComplete="off" onChange={e => edit({ ...form, username: e.target.value })} /></label><label>{t('cd.gitToken')}<input type="password" required value={form.password} disabled={locked} autoComplete="new-password" onChange={e => edit({ ...form, password: e.target.value })} /></label></div> : null}
      <div className="form-actions"><Button type="button" disabled={busy} onClick={() => setForm(undefined)}>{t('cd.cancel')}</Button><Button type="submit" tone="primary" disabled={locked || !provider || !account || (Boolean(form.usePlatformGit) && !platformGit?.repositoryOwner)}>{t('cd.save')}</Button></div>
    </form> : null}
    {details && !form ? <section className="content-panel"><h2>{details.binding.name} · {t('cd.version')} {details.binding.version}</h2><dl>{Object.entries(details.config).filter(([k]) => k !== 'caBundle').map(([k, v]) => <div key={k}><dt>{t(`cd.fields.${k}`)}</dt><dd>{v || '—'}</dd></div>)}</dl><h3>{t('cd.usage')}</h3>{details.usage.length ? <ul>{details.usage.map(u => <li key={`${u.applicationKey}:${u.environmentKey}`}><Button onClick={() => navigate({ name: 'settings', applicationId: u.applicationKey, environment: u.environmentKey === 'production' ? 'production' : 'staging' })}>{u.applicationKey} / {u.environmentKey}</Button> {u.applied ? t('cd.applied') : t('cd.desired')}</li>)}</ul> : <p>{t('cd.unused')}</p>}</section> : null}
  </section>;
}
