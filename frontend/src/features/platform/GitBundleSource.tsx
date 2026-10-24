import { useEffect, useRef, useState } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { parseRenderBundle, type RenderBundle } from './metadata';
import { t } from '../../shared/i18n';

// Git render bundle source (ADR-021, UC-03 RB-01): HTTPS URL, directory and
// ref, then "Kiểm tra bundle". Every field change invalidates the inspected
// result; a late reply for an older request never replaces a newer draft.
export type GitSourceDraft = { url: string; path: string; ref: string };
export const emptyGitSource = (): GitSourceDraft => ({ url: '', path: '', ref: '' });

type Props = {
  draft: GitSourceDraft;
  onDraftChange: (draft: GitSourceDraft) => void;
  disabled: boolean;
  onResult: (bundle: RenderBundle | null) => void;
};

function inspectFailure(reason: unknown): string {
  if (reason instanceof ApiError && reason.code?.startsWith('RENDER_')) return reason.message;
  if (reason instanceof ApiError && reason.status === 403) return reason.message;
  return t('platform.gitBundleSource.couldNotCheckTheBundle');
}

export function GitBundleSource({ draft, onDraftChange, disabled, onResult }: Props) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const request = useRef(0);
  const callback = useRef(onResult);
  callback.current = onResult;
  useEffect(() => () => { request.current++; }, []);

  const change = (patch: Partial<GitSourceDraft>) => {
    request.current++; setBusy(false); setError(''); callback.current(null);
    onDraftChange({ ...draft, ...patch });
  };

  async function inspect() {
    const current = ++request.current;
    setBusy(true); setError(''); callback.current(null);
    try {
      const response = await api<{ renderBundle: unknown }>('/render-bundles/inspect', { method: 'POST', body: JSON.stringify({ url: draft.url.trim(), path: draft.path.trim() || '.', ref: draft.ref.trim() }) });
      const bundle = parseRenderBundle(response.renderBundle);
      if (!bundle.source) throw new Error('invalid metadata');
      if (current !== request.current) return;
      callback.current(bundle);
    } catch (reason) {
      if (current === request.current) setError(inspectFailure(reason));
    } finally {
      if (current === request.current) setBusy(false);
    }
  }

  return <div className="catalog-fields" role="group" aria-label={t('platform.gitBundleSource.addFromGit')}>
    <div className="field-grid">
      <label>Git HTTPS URL<input type="url" value={draft.url} disabled={disabled} spellCheck={false} placeholder="https://github.com/company/render-bundles.git" aria-describedby="git-bundle-help" onChange={(event) => change({ url: event.target.value })} /></label>
      <label>{t('platform.gitBundleSource.bundleDirectory')}<input value={draft.path} disabled={disabled} spellCheck={false} placeholder="." onChange={(event) => change({ path: event.target.value })} /></label>
      <label>{t('platform.gitBundleSource.tagOrCommit')}<input value={draft.ref} disabled={disabled} spellCheck={false} placeholder={t('platform.gitBundleSource.refPlaceholder')} onChange={(event) => change({ ref: event.target.value })} /></label>
    </div>
    <details className="feature-note" id="git-bundle-help"><summary>{t('platform.gitBundleSource.help')}</summary><p>{t('platform.gitBundleSource.helpText')}</p></details>
    <p><Button type="button" disabled={disabled || busy || !draft.url.trim() || !draft.ref.trim()} onClick={() => { void inspect(); }}>{busy ? t('platform.gitBundleSource.checking') : t('platform.gitBundleSource.checkBundle')}</Button></p>
    {busy ? <p role="status">{t('platform.gitBundleSource.downloadingAndChecking')}</p> : null}
    {error ? <p className="form-error" role="alert">{error}</p> : null}
  </div>;
}
