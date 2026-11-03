import { useState, type FormEvent } from 'react';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import { ApiError } from '../../shared/api/client';
import { KubeconfigField, type KubeconfigDraft } from '../platform/accounts/KubeconfigField';
import { accountGuidance, createInternal, maxNameLength, outcomeUnknown, validateName, type Account } from '../platform/accounts/accountsApi';

export function CDKubernetesConnectionDialog({ onClose, onCreated, onUncertain }: {
  onClose(): void;
  onCreated(account: Account): void;
  onUncertain(): void;
}) {
  const [name, setName] = useState('');
  const [kube, setKube] = useState<KubeconfigDraft>({ kubeconfig: '', context: '' });
  const [reset, setReset] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [uncertain, setUncertain] = useState(false);
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (busy || uncertain) return;
    const problem = validateName(name) || (!kube.context ? t('platform.accounts.createAccountForm.uploadOrPasteAKubeconfig') : '');
    if (problem) { setError(problem); return; }
    setBusy(true); setError('');
    try {
      const created = await createInternal({ name: name.trim(), kubeconfig: kube.kubeconfig, context: kube.context });
      onCreated(created);
    } catch (reason) {
      setError(accountGuidance('create', reason));
      if (outcomeUnknown(reason) || (reason instanceof ApiError && reason.status >= 500)) {
        setKube({ kubeconfig: '', context: '' }); setReset(n => n + 1);
      }
      if (outcomeUnknown(reason)) { setUncertain(true); onUncertain(); }
    } finally { setBusy(false); }
  }
  return <Dialog label={t('cd.addKubernetesConnection')} busy={busy} onClose={onClose}>
    <form className="create-form" onSubmit={event => void submit(event)} autoComplete="off">
      <h2>{t('cd.addKubernetesConnection')}</h2>
      <label>{t('platform.accounts.createAccountForm.accountName')}<input required value={name} maxLength={maxNameLength} disabled={busy || uncertain} onChange={e => setName(e.target.value)} /></label>
      <KubeconfigField disabled={busy || uncertain} resetSignal={reset} onChange={setKube} />
      {error ? <p className="form-error" role="alert">{error}</p> : null}
      <div className="form-actions"><Button type="button" disabled={busy} onClick={onClose}>{t('cd.cancel')}</Button><Button type="submit" tone="primary" disabled={busy || uncertain}>{t(busy ? 'platform.accounts.createAccountForm.checkingAndSaving' : 'platform.accounts.createAccountForm.checkAndSave')}</Button></div>
    </form>
  </Dialog>;
}
