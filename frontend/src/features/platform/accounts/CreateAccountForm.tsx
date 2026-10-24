import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Button } from '../../../shared/ui/Button';
import { Ec2RoleFields } from './Ec2RoleFields';
import { AwsKeyFields, emptyAwsFields, type AwsFields } from './AwsKeyFields';
import { KubeconfigField, type KubeconfigDraft } from './KubeconfigField';
import { accountGuidance, createAws, createEc2Role, validateRoleArn, createInternal, outcomeUnknown, parseRegions, validateAwsKeys, validateName, validateRegions, maxNameLength, type Account } from './accountsApi';
import { t } from '../../../shared/i18n';

type Kind = 'internal' | 'aws';
const emptyKube: KubeconfigDraft = { kubeconfig: '', context: '' };

/**
 * Account registration: explicit “Kiểm tra và lưu” only. Secrets leave form
 * state when the request is sent (AWS keys) or when it succeeds (kubeconfig).
 * An unknown outcome never resubmits; the parent reloads the list instead.
 */
export function CreateAccountForm({ onCreated, onUnknownOutcome }: { onCreated(account: Account): Promise<void> | void; onUnknownOutcome(): Promise<boolean> }) {
  const [kind, setKind] = useState<Kind>('internal');
  const [awsAuth, setAwsAuth] = useState<'access-key' | 'ec2-role'>('access-key');
  const [roleArn, setRoleArn] = useState('');
  const [name, setName] = useState('');
  const [kube, setKube] = useState<KubeconfigDraft>(emptyKube);
  const [aws, setAws] = useState<AwsFields>(emptyAwsFields);
  const [resetSignal, setResetSignal] = useState(0);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  // After an unknown outcome registration stays blocked until a GET of the
  // list succeeded and the user explicitly reviewed it: a lost success must
  // never be answered by a second POST.
  const [recovery, setRecovery] = useState<'none' | 'checking' | 'failed' | 'review'>('none');
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const blocked = recovery !== 'none';

  async function recover() {
    setRecovery('checking');
    let loaded = false;
    try { loaded = await onUnknownOutcome(); } catch { loaded = false; }
    if (alive.current) setRecovery(loaded ? 'review' : 'failed');
  }

  function changeKind(next: Kind) {
    if (next === kind || saving || blocked) return;
    setKind(next); setRoleArn(''); setError(''); setNotice(''); setAws(emptyAwsFields); setKube(emptyKube); setResetSignal((n) => n + 1);
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving || blocked) return;
    setNotice('');
    const problem = validateName(name)
      || (kind === 'aws' ? (awsAuth === 'ec2-role' ? validateRoleArn(roleArn) : validateAwsKeys(aws.accessKeyId, aws.secretAccessKey)) || validateRegions(aws.regions) : (!kube.context ? t('platform.accounts.createAccountForm.uploadOrPasteAKubeconfig') : ''));
    if (problem) { setError(problem); return; }
    setError(''); setSaving(true);
    let created: Account;
    try {
      if (kind === 'aws' && awsAuth === 'ec2-role') {
        created = await createEc2Role({ name: name.trim(), roleArn, verificationRegions: parseRegions(aws.regions) });
      } else if (kind === 'aws') {
        const { accessKeyId, secretAccessKey, regions } = aws;
        // The keys leave form state before the request resolves.
        setAws((current) => ({ ...current, accessKeyId: '', secretAccessKey: '' }));
        created = await createAws({ name: name.trim(), accessKeyId, secretAccessKey, verificationRegions: parseRegions(regions) });
      } else {
        created = await createInternal({ name: name.trim(), kubeconfig: kube.kubeconfig, context: kube.context });
      }
    } catch (reason) {
      if (!alive.current) return;
      setError(accountGuidance('create', reason, kind === 'aws' && awsAuth === 'ec2-role')); setSaving(false);
      if (outcomeUnknown(reason)) void recover();
      return;
    }
    if (!alive.current) return;
    setNotice(t('platform.accounts.createAccountForm.registeredAccountNameKeyAt', { name: created.name, key: created.key, version: created.version }));
    setName(''); setRoleArn(''); setAws(emptyAwsFields); setKube(emptyKube); setResetSignal((n) => n + 1); setSaving(false);
    await onCreated(created);
  }

  return <form onSubmit={submit} autoComplete="off"><fieldset className="form-fieldset connection-form" disabled={saving} aria-busy={saving} aria-label={t('platform.accounts.createAccountForm.registerNewAccount')}>
    <div className="source-choices connection-kind" role="radiogroup" aria-label={t('platform.accounts.createAccountForm.infrastructureType')}>
      <label className="source-choice"><input type="radio" name="account-type" checked={kind === 'internal'} onChange={() => changeKind('internal')} /> {' '}{t('platform.accounts.createAccountForm.internalExistingKubernetesCluster')}</label>
      <label className="source-choice"><input type="radio" name="account-type" checked={kind === 'aws'} onChange={() => changeKind('aws')} /> AWS</label>
    </div>
    <p className="muted">{kind === 'aws'
      ? awsAuth === 'ec2-role' ? t('platform.accounts.createAccountForm.aWSInfrastructureTypeAuthenticatedWith') : t('platform.accounts.createAccountForm.aWSInfrastructureTypeAuthenticatedWith2')
      : t('platform.accounts.createAccountForm.internalInfrastructureTypeAuthenticatedWith')}</p>
    {kind === 'aws' ? <label className="connection-field">{t('platform.accounts.createAccountForm.authenticationMethod')}<select value={awsAuth} disabled={saving || blocked} onChange={(event) => {
      setAwsAuth(event.target.value as 'access-key' | 'ec2-role'); setAws(emptyAwsFields); setRoleArn(''); setError(''); setNotice('');
    }}><option value="access-key">Access key</option><option value="ec2-role">{t('platform.accounts.createAccountForm.eC2IAMRole')}</option></select></label> : null}
    <label className="connection-field">{t('platform.accounts.createAccountForm.accountName')}<input name="account-name" value={name} required maxLength={maxNameLength} autoComplete="off" onChange={(event) => setName(event.target.value)} /></label>
    {kind === 'internal'
      ? <KubeconfigField disabled={saving} resetSignal={resetSignal} onChange={setKube} />
      : awsAuth === 'ec2-role' ? <Ec2RoleFields roleArn={roleArn} regions={aws.regions} disabled={saving || blocked} onRoleArn={setRoleArn} onRegions={(regions) => setAws((current) => ({ ...current, regions }))} />
      : <AwsKeyFields prefix="account" fields={aws} disabled={saving} onChange={(patch) => setAws((current) => ({ ...current, ...patch }))} />}
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {notice ? <div className="form-success" role="status">{notice}</div> : null}
    {recovery === 'checking' ? <p className="muted" role="status">{t('platform.accounts.createAccountForm.reloadingTheListToCheck')}</p> : null}
    {recovery === 'failed' ? <div className="form-error" role="alert">{t('platform.accounts.createAccountForm.couldNotReloadTheList')}{' '} <Button type="button" onClick={() => void recover()}>{t('platform.accounts.createAccountForm.reloadList')}</Button></div> : null}
    {recovery === 'review' ? <div className="form-error" role="status">{t('platform.accounts.createAccountForm.theListWasReloadedReview')}{' '} <Button type="button" onClick={() => setRecovery('none')}>{t('platform.accounts.createAccountForm.iHaveReviewedTheList')}</Button></div> : null}
    <div className="connection-footer"><Button type="submit" tone="primary" disabled={saving || blocked}>{saving ? t('platform.accounts.createAccountForm.checkingAndSaving') : t('platform.accounts.createAccountForm.checkAndSave')}</Button></div>
  </fieldset></form>;
}
