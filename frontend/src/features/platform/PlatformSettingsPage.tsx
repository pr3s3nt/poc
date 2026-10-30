import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { PasswordInput } from '../../shared/ui/PasswordInput';
import { t } from '../../shared/i18n';

type Settings = { url: string; username: string; authenticated: boolean; caPem: string; tokenConfigured: boolean; version: number; updatedAt?: string };
type Draft = Settings & { token: string; repositoryUrl: string; ref: string };
const empty: Draft = { url: '', username: '', authenticated: true, caPem: '', tokenConfigured: false, version: 0, token: '', repositoryUrl: '', ref: 'main' };
const endpoint = '/platform/settings/git';

export function PlatformSettingsPage() {
  const [draft, setDraft] = useState<Draft>(empty);
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading');
  const [pending, setPending] = useState<'save' | 'test' | null>(null);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  const [stale, setStale] = useState(false);
  const mounted = useRef(false);
  const busy = useRef(false);
  const loadGeneration = useRef(0);

  const load = useCallback(async () => {
    const generation = ++loadGeneration.current;
    setState('loading'); setError(''); setMessage(''); setStale(false);
    try {
      const result = await api<{ settings: Settings }>(endpoint);
      if (!mounted.current || generation !== loadGeneration.current) return;
      setDraft({ ...empty, ...result.settings, authenticated: result.settings.version === 0 || result.settings.authenticated });
      setState('ready');
    } catch {
      if (mounted.current && generation === loadGeneration.current) setState('error');
    }
  }, []);
  useEffect(() => {
    mounted.current = true;
    void load();
    return () => { mounted.current = false; ++loadGeneration.current; };
  }, [load]);

  function edit<K extends keyof Draft>(key: K, value: Draft[K]) {
    setDraft(current => ({ ...current, [key]: value })); setMessage(''); setError('');
  }
  function payload() {
    return { url: draft.url.trim(), username: draft.username, authenticated: draft.authenticated, token: draft.token, caPem: draft.caPem, expectedVersion: draft.version };
  }
  function fail(cause: unknown) {
    if (cause instanceof ApiError && cause.code === 'STALE_VERSION') {
      setStale(true); setError(t('platform.git.stale'));
    } else if (cause instanceof ApiError && cause.code === 'INVALID_REQUEST') {
      setError(t('platform.git.invalid'));
    } else if (cause instanceof ApiError && cause.code === 'VERIFICATION_FAILED') {
      setError(t('platform.git.failed'));
    } else if (cause instanceof ApiError && cause.status === 403) {
      setError(t('platform.git.forbidden'));
    } else {
      setError(t('platform.git.unavailable'));
    }
  }
  async function act(kind: 'save' | 'test') {
    if (busy.current || stale) return;
    busy.current = true; setPending(kind); setError(''); setMessage('');
    try {
      if (kind === 'save') {
        const result = await api<{ settings: Settings }>(endpoint, { method: 'PUT', body: JSON.stringify(payload()) });
        if (!mounted.current) return;
        setDraft(current => ({ ...current, ...result.settings, token: '' }));
        setMessage(t('platform.git.saved'));
      } else {
        await api<{ reachable: boolean }>(`${endpoint}/test`, { method: 'POST', body: JSON.stringify({ ...payload(), repositoryUrl: draft.repositoryUrl.trim() || draft.url.trim(), ref: draft.ref }) });
        if (mounted.current) setMessage(t('platform.git.reachable'));
      }
    } catch (cause) {
      if (mounted.current) { fail(cause); setDraft(current => ({ ...current, token: '' })); }
    } finally {
      busy.current = false;
      if (mounted.current) setPending(null);
    }
  }
  function submit(event: FormEvent) { event.preventDefault(); void act('save'); }

  return <section className="page">
    <header className="page-header"><div><p className="eyebrow">{t('platform.git.platform')}</p><h1>{t('platform.git.title')}</h1></div></header>
    {state === 'loading' ? <p role="status" aria-busy="true">{t('platform.git.loading')}</p> : state === 'error' ? <div className="form-error" role="alert">{t('platform.git.loadFailed')} <Button onClick={() => void load()}>{t('platform.git.retry')}</Button></div> :
      <form className="create-form" onSubmit={submit}>
        <h2>{t('platform.git.access')}</h2>
        <fieldset className="form-fieldset editor-grid git-settings-fields" disabled={pending !== null || stale}>
          <div className="field"><label htmlFor="git-url">{t('platform.git.url')}</label><input id="git-url" type="url" required maxLength={2048} value={draft.url} placeholder="https://git.example.com/platform" onChange={event => edit('url', event.target.value)} aria-describedby="git-scope-help" /><small id="git-scope-help">{t('platform.git.scopeHelp')}</small></div>
          <div className="field"><label htmlFor="git-auth">{t('platform.git.authentication')}</label><select id="git-auth" value={draft.authenticated ? 'token' : 'anonymous'} onChange={event => { edit('authenticated', event.target.value === 'token'); setDraft(current => ({ ...current, token: '' })); }}><option value="token">{t('platform.git.usernameToken')}</option><option value="anonymous">{t('platform.git.anonymous')}</option></select></div>
          {draft.authenticated ? <>
            <div className="field"><label htmlFor="git-user">{t('platform.git.username')}</label><input id="git-user" required maxLength={256} autoComplete="off" value={draft.username} onChange={event => edit('username', event.target.value)} /></div>
            <div className="field"><label htmlFor="git-token">{t('platform.git.token')}</label><PasswordInput id="git-token" maxLength={4096} autoComplete="new-password" value={draft.token} onChange={event => edit('token', event.target.value)} aria-describedby="git-token-help" /><small id="git-token-help">{t(draft.tokenConfigured ? 'platform.git.tokenSaved' : 'platform.git.tokenHelp')}</small></div>
          </> : null}
          <details><summary>{t('platform.git.advanced')}</summary><div className="editor-grid">
            <div className="field"><label htmlFor="git-ca">{t('platform.git.ca')}</label><textarea id="git-ca" rows={4} maxLength={65536} spellCheck={false} value={draft.caPem} onChange={event => edit('caPem', event.target.value)} /></div>
            <div className="field"><label htmlFor="git-test-repo">{t('platform.git.repository')}</label><input id="git-test-repo" type="url" maxLength={2048} placeholder={draft.url} value={draft.repositoryUrl} onChange={event => edit('repositoryUrl', event.target.value)} /><small>{t('platform.git.repositoryHelp')}</small></div>
            <div className="field"><label htmlFor="git-ref">{t('platform.git.ref')}</label><input id="git-ref" maxLength={200} value={draft.ref} onChange={event => edit('ref', event.target.value)} /></div>
          </div></details>
        </fieldset>
        {error ? <p className="form-error" role="alert">{error}</p> : null}
        {message ? <p className="form-info" role="status">{message}</p> : null}
        <div className="form-actions">
          {stale || error ? <Button type="button" onClick={() => void load()}>{t('platform.git.reload')}</Button> : null}
          <Button type="button" disabled={pending !== null || stale || !draft.url} onClick={() => void act('test')}>{t(pending === 'test' ? 'platform.git.testing' : 'platform.git.test')}</Button>
          <Button type="submit" tone="primary" disabled={pending !== null || stale}>{t(pending === 'save' ? 'platform.git.saving' : 'platform.git.save')}</Button>
        </div>
      </form>}
  </section>;
}
