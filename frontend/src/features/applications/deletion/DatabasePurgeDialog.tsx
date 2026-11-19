import { useEffect, useState } from 'react';
import { api, describeError } from '../../../shared/api/client';
import { t } from '../../../shared/i18n';
import { Button } from '../../../shared/ui/Button';
import { Dialog } from '../../../shared/ui/Dialog';

type PurgePreview = {
  applicationId: string;
  displayName: string;
  busy: boolean;
  priorStopRequired: boolean;
  environments: { key: string; namespace: string }[];
  fleet: { gitRepo: string; release: string; namespace: string }[];
  resources: string[];
  gitRepositories: string[];
};

export function DatabasePurgeDialog({ applicationId, onClose, onDeleted }: {
  applicationId: string;
  onClose(): void;
  onDeleted(): void;
}) {
  const path = `/applications/${encodeURIComponent(applicationId)}/database-purge`;
  const [preview, setPreview] = useState<PurgePreview>();
  const [error, setError] = useState('');
  const [typedName, setTypedName] = useState('');
  const [priorStopped, setPriorStopped] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [reload, setReload] = useState(0);
  const title = t('applications.databasePurge.title');

  useEffect(() => {
    const controller = new AbortController();
    api<{ preview: PurgePreview }>(path, { signal: controller.signal }).then(({ preview }) => {
      if (!controller.signal.aborted) setPreview(preview);
    }).catch((err: unknown) => {
      if (!controller.signal.aborted) setError(describeError(err, t('applications.databasePurge.previewFailed')));
    });
    return () => controller.abort();
  }, [path, reload]);

  const canSubmit = preview && !preview.busy && typedName === preview.displayName && (!preview.priorStopRequired || priorStopped);
  function reviewAgain() {
    setError('');
    setTypedName('');
    setPriorStopped(false);
    setPreview(undefined);
    setReload(current => current + 1);
  }

  async function submit() {
    if (!preview || !canSubmit || submitting) return;
    setSubmitting(true);
    setError('');
    try {
      await api<void>(path, {
        method: 'POST',
        body: JSON.stringify({ applicationId: preview.applicationId, confirmationName: typedName, priorExecutionStopped: priorStopped }),
      });
      onDeleted();
    } catch (err) {
      setError(describeError(err, t('applications.databasePurge.unknownOutcome')));
      setTypedName('');
      setPriorStopped(false);
    } finally {
      setSubmitting(false);
    }
  }

  return <Dialog label={title} busy={submitting} onClose={onClose}>
    <h2>{title}</h2>
    <p>{t('applications.databasePurge.scope')}</p>
    {error ? <p className="form-error" role="alert">{error}</p> : null}
    {!preview && !error ? <p role="status">{t('applications.databasePurge.loading')}</p> : null}
    {preview ? <>
      <p><strong>{preview.displayName}</strong></p>
      <details>
        <summary>{t('applications.databasePurge.inventory')}</summary>
        <ul>
          {preview.environments.map(env => <li key={env.key}>{env.key} · <code>{env.namespace}</code></li>)}
          {preview.fleet.map(item => <li key={`${item.gitRepo}/${item.release}/${item.namespace}`}><code>{item.gitRepo} · {item.release} · {item.namespace}</code></li>)}
          {preview.gitRepositories.map(repo => <li key={repo}><code>{repo}</code></li>)}
          {preview.resources.map(resource => <li key={resource}><code>{resource}</code></li>)}
        </ul>
      </details>
      {preview.busy ? <p className="form-info" role="status">{t('applications.databasePurge.busy')}</p> : <>
        {preview.priorStopRequired ? <label className="checkbox-row">
          <input type="checkbox" checked={priorStopped} disabled={submitting} onChange={event => setPriorStopped(event.target.checked)} />
          {t('applications.databasePurge.priorStopped')}
        </label> : null}
        <label className="field">{t('applications.deletion.typeName', { name: preview.displayName })}
          <input value={typedName} disabled={submitting} autoComplete="off" spellCheck={false} onChange={event => setTypedName(event.target.value)} />
        </label>
      </>}
    </> : null}
    <div className="form-actions">
      <Button disabled={submitting} onClick={onClose}>{t('applications.deletion.close')}</Button>
      {error || preview?.busy ? <Button disabled={submitting} onClick={reviewAgain}>{t('applications.deletion.retryLoad')}</Button> : null}
      <Button tone="danger" disabled={submitting || !canSubmit || Boolean(error)} onClick={() => void submit()}>{submitting ? t('applications.deletion.sending') : title}</Button>
    </div>
  </Dialog>;
}
