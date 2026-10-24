import { useEffect, useRef, useState, type FormEvent } from 'react';
import { isStaleConflict } from '../../../shared/api/client';
import { Button } from '../../../shared/ui/Button';
import { AwsKeyFields, emptyAwsFields, type AwsFields } from './AwsKeyFields';
import { KubeconfigField, type KubeconfigDraft } from './KubeconfigField';
import {
  accountGuidance, authLabel, getAccount, maxNameLength, outcomeUnknown, parseRegions, renameAccount, replaceCredentials, statusLabel, typeLabel,
  validateAwsKeys, validateName, validateRegions, verifyAccount, type Account, type Operation,
} from './accountsApi';
import { intlLocale, t } from '../../../shared/i18n';

const emptyKube: KubeconfigDraft = { kubeconfig: '', context: '' };
type Busy = null | 'rename' | 'verify' | 'replace';

function when(value?: string): string {
  const date = value ? new Date(value) : undefined;
  return date && !Number.isNaN(date.getTime()) ? date.toLocaleString(intlLocale()) : '';
}

/**
 * One account: safe metadata, explicit rename, explicit read-only connection
 * check and a separate concealed same-target credential replacement. Rendered
 * with key={account.key}, so switching account discards every draft and secret,
 * and unmounting makes late responses no-ops. Mutations share one busy flag
 * (no duplicates), carry the loaded version and are never replayed: after a
 * stale or unknown outcome the user reloads (GET) and saves again explicitly.
 */
export function AccountDetail({ account, onChanged, onRefreshed }: { account: Account; onChanged(account: Account): void; onRefreshed(account: Account): void }) {
  const [busy, setBusy] = useState<Busy>(null);
  const [nameDraft, setNameDraft] = useState(account.name);
  const [error, setError] = useState('');
  const [stale, setStale] = useState(false);
  const [unknown, setUnknown] = useState(false);
  // After an unknown outcome the loaded result must be reviewed explicitly.
  const [review, setReview] = useState(false);
  const [reloading, setReloading] = useState(false);
  const [notice, setNotice] = useState('');
  const [verifyFailure, setVerifyFailure] = useState('');
  const [verifyRegions, setVerifyRegions] = useState((account.config?.verificationRegions ?? []).join(', '));
  const [kube, setKube] = useState<KubeconfigDraft>(emptyKube);
  const [aws, setAws] = useState<AwsFields>({ ...emptyAwsFields, regions: (account.config?.verificationRegions ?? []).join(', ') });
  const [resetSignal, setResetSignal] = useState(0);
  const [replaceError, setReplaceError] = useState('');
  const [replaceNotice, setReplaceNotice] = useState('');
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);

  const isAws = account.type === 'AWS';
  const isEc2Role = account.authenticationType === 'AWS_EC2_ROLE';
  const blocked = busy !== null || unknown || reloading || review;

  async function reload() {
    if (reloading) return;
    setReloading(true);
    try {
      const fresh = await getAccount(account.key);
      if (!alive.current) return;
      // Metadata only: the user's drafts are never replaced by a loaded copy.
      onRefreshed(fresh);
      if (unknown) setReview(true);
      setStale(false); setUnknown(false); setError('');
      setNotice(t('platform.accounts.accountDetail.accountReloadedCurrentVersionVersion', { version: fresh.version, value: fresh.name !== nameDraft ? t('platform.accounts.accountDetail.theSavedNameIsName', { name: fresh.name }) : '' }));
    } catch {
      if (alive.current) setError(t('platform.accounts.accountDetail.couldNotReloadTheAccount'));
    } finally {
      if (alive.current) setReloading(false);
    }
  }

  function failure(operation: Operation, reason: unknown) {
    const text = accountGuidance(operation, reason, isEc2Role);
    if (isStaleConflict(reason)) setStale(true);
    if (outcomeUnknown(reason)) { setUnknown(true); void reloadAfterUnknown(); }
    return text;
  }
  async function reloadAfterUnknown() {
    // GET only: finding out what happened never repeats the mutation.
    setReloading(true);
    try {
      const fresh = await getAccount(account.key);
      if (!alive.current) return;
      onRefreshed(fresh); setUnknown(false); setReview(true);
      setNotice(t('platform.accounts.accountDetail.accountReloadedToCheckThe', { version: fresh.version, name: fresh.name }));
    } catch { /* stays blocked until an explicit reload succeeds */ } finally {
      if (alive.current) setReloading(false);
    }
  }

  async function rename(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (blocked) return;
    const problem = validateName(nameDraft);
    setNotice(''); setVerifyFailure('');
    if (problem) { setError(problem); return; }
    setError(''); setBusy('rename');
    try {
      const updated = await renameAccount(account.key, { name: nameDraft.trim(), version: account.version });
      if (!alive.current) return;
      setNameDraft(updated.name); setStale(false);
      setNotice(t('platform.accounts.accountDetail.renamedToNameVersionVersion', { name: updated.name, version: updated.version }));
      onChanged(updated);
    } catch (reason) {
      if (alive.current) setError(failure('rename', reason));
    } finally {
      if (alive.current) setBusy(null);
    }
  }

  async function verify() {
    if (blocked) return;
    let regions: string[] | undefined;
    if (isAws) {
      const problem = validateRegions(verifyRegions);
      if (problem) { setError(problem); return; }
      regions = parseRegions(verifyRegions);
    }
    setError(''); setNotice(''); setVerifyFailure(''); setBusy('verify');
    try {
      const updated = await verifyAccount(account.key, { version: account.version, ...(regions ? { verificationRegions: regions } : {}) });
      if (!alive.current) return;
      setStale(false);
      setNotice(t('platform.accounts.accountDetail.connectionCheckSucceededAtValue', { value: when(updated.verification?.verifiedAt) || t('platform.accounts.accountDetail.justNow'), version: updated.version }));
      onChanged(updated);
    } catch (reason) {
      if (!alive.current) return;
      const text = failure('verify', reason);
      // A fresh check failure is shown apart from the last successful check.
      if (isStaleConflict(reason) || outcomeUnknown(reason)) setError(text); else setVerifyFailure(text);
    } finally {
      if (alive.current) setBusy(null);
    }
  }

  function cancelReplace() {
    setAws({ ...emptyAwsFields, regions: aws.regions }); setKube(emptyKube); setResetSignal((n) => n + 1); setReplaceError('');
  }

  async function replace(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (blocked) return;
    setReplaceNotice(''); setNotice(''); setVerifyFailure('');
    const problem = isAws
      ? validateAwsKeys(aws.accessKeyId, aws.secretAccessKey) || validateRegions(aws.regions)
      : (!kube.context ? t('platform.accounts.accountDetail.uploadOrPasteAKubeconfig') : '');
    if (problem) { setReplaceError(problem); return; }
    setReplaceError(''); setBusy('replace');
    let body: Record<string, unknown>;
    if (isAws) {
      body = { version: account.version, accessKeyId: aws.accessKeyId, secretAccessKey: aws.secretAccessKey, verificationRegions: parseRegions(aws.regions) };
      // Keys leave form state before the request resolves.
      setAws((current) => ({ ...current, accessKeyId: '', secretAccessKey: '' }));
    } else {
      body = { version: account.version, kubeconfig: kube.kubeconfig, context: kube.context };
    }
    try {
      const updated = await replaceCredentials(account.key, body);
      if (!alive.current) return;
      setStale(false); cancelReplace();
      setReplaceNotice(t('platform.accounts.accountDetail.credentialReplacedAfterVerifyingThe', { version: updated.version }));
      onChanged(updated);
    } catch (reason) {
      if (alive.current) setReplaceError(failure('replace', reason));
    } finally {
      if (alive.current) setBusy(null);
    }
  }

  const config = account.config ?? {};
  const verification = account.verification ?? {};
  const checkedAt = when(verification.verifiedAt);
  return <section className="content-panel account-detail" aria-label={t('platform.accounts.accountDetail.accountDetailsName', { name: account.name })} aria-busy={busy !== null}>
    <div className="section-header"><div><h2>{account.name}</h2><p className="muted"><code>{account.key}</code> {' '}{t('platform.accounts.accountDetail.version')}{' '} {account.version}</p></div></div>
    <dl className="destination-summary" aria-label={t('platform.accounts.accountDetail.accountInformation')}>
      <div><dt>{t('platform.accounts.accountDetail.infrastructureType')}</dt> <dd>{typeLabel(account.type)}</dd></div>
      <div><dt>{t('platform.accounts.accountDetail.authenticationMethod')}</dt> <dd>{authLabel(account.authenticationType)}</dd></div>
      <div><dt>{t('platform.accounts.accountDetail.status')}</dt> <dd>{statusLabel(account.status)}</dd></div>
      {isAws ? <>
        <div><dt>{t('platform.accounts.accountDetail.aWSAccount')}</dt> <dd>{config.accountId ?? '—'}</dd></div>
        {isEc2Role ? <><div><dt>IAM Role ARN</dt> <dd>{config.roleArn ?? '—'}</dd></div><div><dt>Role ID</dt> <dd>{config.roleId ?? '—'}</dd></div></> : null}
        <div><dt>Principal</dt> <dd>{config.principalArn ?? '—'}</dd></div>
        <div><dt>{t('platform.accounts.accountDetail.checkRegions')}</dt> <dd>{(config.verificationRegions ?? []).join(', ') || '—'}</dd></div>
      </> : <>
        <div><dt>Cluster</dt> <dd>{config.cluster ?? '—'}</dd></div>
        <div><dt>Context</dt> <dd>{config.kubeContext ?? '—'}</dd></div>
        <div><dt>Endpoint</dt> <dd>{config.endpoint ?? '—'}</dd></div>
        <div><dt>TLS</dt> <dd>{config.tls?.mode === 'CUSTOM_CA' ? t('platform.accounts.accountDetail.customCA') : config.tls?.mode === 'INSECURE_SKIP_VERIFY' ? t('platform.accounts.accountDetail.skipTLSVerification') : config.tls?.mode === 'SYSTEM_ROOTS' ? t('platform.accounts.accountDetail.systemCA') : '—'}{config.tls?.serverName ? ` (${config.tls.serverName})` : ''}</dd></div>
      </>}
      <div><dt>{t('platform.accounts.accountDetail.lastSuccessfulCheck')}</dt> <dd>{checkedAt || t('platform.accounts.accountDetail.noneYet')}</dd></div>
    </dl>
    <p className="muted">{t('platform.accounts.accountDetail.readyOnlyRecordsTheLast')}</p>

    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {review ? <div className="form-error" role="status">{t('platform.accounts.accountDetail.reloadedToCheckTheResult')}{' '} <Button type="button" disabled={reloading} onClick={() => setReview(false)}>{t('platform.accounts.accountDetail.iHaveReviewedTheResult')}</Button></div> : null}
    {stale || unknown ? <div className="form-error"><Button type="button" disabled={reloading} onClick={() => void reload()}>{t('platform.accounts.accountDetail.reloadLatestVersion')}</Button></div> : null}
    {notice ? <div className="form-success" role="status">{notice}</div> : null}

    <form onSubmit={rename} autoComplete="off"><fieldset className="form-fieldset connection-form" disabled={blocked}>
      <label className="connection-field">{t('platform.accounts.accountDetail.accountName')}<input name="rename" value={nameDraft} required maxLength={maxNameLength} onChange={(event) => setNameDraft(event.target.value)} /></label>
      <div className="connection-footer"><Button type="submit" disabled={blocked}>{busy === 'rename' ? t('platform.accounts.accountDetail.saving') : t('platform.accounts.accountDetail.saveName')}</Button></div>
    </fieldset></form>

    <fieldset className="form-fieldset connection-form" disabled={blocked}>
      <legend>{t('platform.accounts.accountDetail.checkConnection')}</legend>
      {isAws ? <label className="connection-field">{t('platform.accounts.accountDetail.checkRegions')}<input name="verify-regions" value={verifyRegions} autoComplete="off" spellCheck={false} onChange={(event) => setVerifyRegions(event.target.value)} /></label> : null}
      <div className="connection-footer"><Button type="button" disabled={blocked} onClick={() => void verify()}>{busy === 'verify' ? t('platform.accounts.accountDetail.checking') : t('platform.accounts.accountDetail.checkConnection')}</Button></div>
      {verifyFailure ? <div className="form-error" role="alert" aria-label={t('platform.accounts.accountDetail.theLastCheckFailed')}><strong>{t('platform.accounts.accountDetail.theLastCheckFailed2')}</strong> {verifyFailure} {' '}{t('platform.accounts.accountDetail.theStatus')}{statusLabel(account.status)}{t('platform.accounts.accountDetail.aboveIsStillTheLast')}{checkedAt ? ` (${checkedAt})` : ''}{t('platform.accounts.accountDetail.notTheResultOfThis')}</div> : null}
    </fieldset>

    {isEc2Role ? <p className="muted">{t('platform.accounts.accountDetail.temporaryCredentialsAreTakenFrom')}</p> : <form onSubmit={replace} autoComplete="off"><fieldset className="form-fieldset connection-form" disabled={blocked} aria-label={t('platform.accounts.accountDetail.replaceCredential')}>
      <legend>{t('platform.accounts.accountDetail.replaceCredential')}</legend>
      <p className="muted">{t('platform.accounts.accountDetail.valueTheCurrentCredentialIs', { value: isAws
        ? t('platform.accounts.accountDetail.enterANewAccessKey')
        : t('platform.accounts.accountDetail.provideANewKubeconfigFor') })}</p>
      {isAws
        ? <AwsKeyFields prefix="replace" fields={aws} disabled={blocked} onChange={(patch) => setAws((current) => ({ ...current, ...patch }))} />
        : <KubeconfigField disabled={blocked} resetSignal={resetSignal} onChange={setKube} />}
      {replaceError ? <div className="form-error" role="alert">{replaceError}</div> : null}
      {replaceNotice ? <div className="form-success" role="status">{replaceNotice}</div> : null}
      <div className="connection-footer">
        <Button type="submit" tone="primary" disabled={blocked}>{busy === 'replace' ? t('platform.accounts.accountDetail.verifyingAndReplacing') : t('platform.accounts.accountDetail.verifyAndReplaceCredential')}</Button>
        <Button type="button" disabled={busy !== null} onClick={cancelReplace}>{t('platform.accounts.accountDetail.cancel')}</Button>
      </div>
    </fieldset></form>}
  </section>;
}
