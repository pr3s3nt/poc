import { useId, useState, type FormEvent } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';

// Public, secret-free result of a registration (UC-04 VAR-02). Never holds a key.
export type AwsRegistered = { key: string; name?: string; config?: { accountId?: string; principalArn?: string; region?: string; allowedRegions?: string[] } };

// Same bounds as the backend (application/connection/aws.go); the server stays the final guard.
const maxNameLength = 100;
const maxRegions = 10;
const accessKeyShape = /^AKIA[A-Z0-9]{16}$/;
const secretKeyShape = /^[A-Za-z0-9/+]{40}$/;
const regionShape = /^[a-z]{2}(-[a-z]+)+-[0-9]$/;

/** Splits the region field on commas and white space, keeping the typed order (the first is the default). */
export function parseRegions(text: string): string[] {
  return text.split(/[\s,;]+/).filter(Boolean);
}

type Fields = { name: string; accessKeyId: string; secretAccessKey: string; regions: string };
const emptyFields: Fields = { name: '', accessKeyId: '', secretAccessKey: '', regions: '' };

// Messages never include a submitted value.
function validate(fields: Fields): string {
  const name = fields.name.trim();
  if (!name) return t('platform.awsConnectionForm.enterAConnectionName');
  if ([...name].length > maxNameLength) return t('platform.awsConnectionForm.theConnectionNameCanHave', { maxNameLength });
  if (!accessKeyShape.test(fields.accessKeyId)) return t('platform.awsConnectionForm.invalidAccessKeyID20');
  if (!secretKeyShape.test(fields.secretAccessKey)) return t('platform.awsConnectionForm.invalidSecretAccessKeyExactly');
  const regions = parseRegions(fields.regions);
  if (regions.length === 0) return t('platform.awsConnectionForm.enterAtLeastOneAllowed');
  if (regions.length > maxRegions) return t('platform.awsConnectionForm.atMostMaxRegionsRegionsAre', { maxRegions });
  if (regions.some((region) => !regionShape.test(region))) return t('platform.awsConnectionForm.invalidRegionNameUseA');
  if (new Set(regions).size !== regions.length) return t('platform.awsConnectionForm.eachRegionMayBeEntered');
  return '';
}

// Fixed Vietnamese guidance by status and stable code. Raw backend or AWS text never reaches the page.
function registerGuidance(reason: unknown): string {
  if (!(reason instanceof ApiError) || reason.code === 'COMMIT_UNCERTAIN') return t('platform.awsConnectionForm.noResponseFromTheServer');
  switch (reason.status) {
    case 400: return t('platform.awsConnectionForm.theServerRejectedTheRegistration');
    case 401: return t('platform.awsConnectionForm.yourSessionHasExpiredSign');
    case 403: return t('platform.awsConnectionForm.yourAccountIsNotAllowed');
    case 413: return t('platform.awsConnectionForm.theRegistrationDataIsToo');
    case 422:
      switch (reason.code) {
        case 'VERIFICATION_CREDENTIAL_REJECTED': return t('platform.awsConnectionForm.aWSRejectedThisAccessKey');
        case 'VERIFICATION_PERMISSION_DENIED': return t('platform.awsConnectionForm.theAccessKeyLacksThe');
        case 'VERIFICATION_UNREACHABLE': return t('platform.awsConnectionForm.couldNotReachAWSFor');
        case 'VERIFICATION_REGION_UNAVAILABLE':
        case 'VERIFICATION_CAPABILITY_UNVERIFIABLE':
        case 'VERIFICATION_PRINCIPAL_UNSUPPORTED': return t('platform.awsConnectionForm.messageTheConnectionWasNot', { message: reason.message });
        default: return t('platform.awsConnectionForm.couldNotVerifyTheAWS');
      }
    case 503: return t('platform.awsConnectionForm.thePlatformCredentialStoreIs');
    default: return reason.status >= 500 ? t('platform.awsConnectionForm.theServerFailedDuringRegistration') : t('platform.awsConnectionForm.couldNotRegisterTheConnection');
  }
}

/**
 * AWS access-key onboarding. The key pair lives only in two concealed controlled
 * inputs until the request is sent; both are cleared after every attempt that
 * reached the server and after success. Nothing is stored or logged client-side.
 */
export function AwsConnectionForm({ onRegistered }: { onRegistered: () => Promise<void> | void }) {
  const [fields, setFields] = useState<Fields>(emptyFields);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [result, setResult] = useState<AwsRegistered | null>(null);
  const id = useId();
  const set = (patch: Partial<Fields>) => setFields((current) => ({ ...current, ...patch }));

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    const problem = validate(fields);
    setNotice(''); setResult(null);
    if (problem) { setError(problem); return; }
    setError(''); setSaving(true);
    const { name, accessKeyId, secretAccessKey, regions } = fields;
    // The keys leave the form state before the request resolves; only the payload holds them.
    set({ accessKeyId: '', secretAccessKey: '' });
    let created: AwsRegistered;
    try {
      created = await api<AwsRegistered>('/connections/aws', { method: 'POST', body: JSON.stringify({ name: name.trim(), accessKeyId, secretAccessKey, allowedRegions: parseRegions(regions) }) });
    } catch (reason) {
      setError(registerGuidance(reason)); setSaving(false);
      // An unknown outcome is checked by reloading the list, never by resubmitting.
      if (!(reason instanceof ApiError) || reason.code === 'COMMIT_UNCERTAIN') void onRegistered();
      return;
    }
    setFields(emptyFields); setSaving(false);
    setResult(created);
    setNotice(t('platform.awsConnectionForm.registeredAWSAccountValueKey', { value: created.name ?? name.trim(), key: created.key }));
    await onRegistered();
  }

  const regionCount = parseRegions(fields.regions).length;
  return <form onSubmit={submit} autoComplete="off"><fieldset className="form-fieldset connection-form" disabled={saving} aria-busy={saving}>
    <label className="connection-field">{t('platform.awsConnectionForm.connectionName')}<input name="aws-connection-name" value={fields.name} required maxLength={maxNameLength} autoComplete="off" onChange={(event) => set({ name: event.target.value })} /></label>
    <label className="connection-field">Access key ID<input type="password" name="aws-access-key-id" value={fields.accessKeyId} autoComplete="new-password" spellCheck={false} maxLength={20} aria-describedby={`${id}-keys`} onChange={(event) => set({ accessKeyId: event.target.value })} /></label>
    <label className="connection-field">Secret access key<input type="password" name="aws-secret-access-key" value={fields.secretAccessKey} autoComplete="new-password" spellCheck={false} maxLength={40} aria-describedby={`${id}-keys`} onChange={(event) => set({ secretAccessKey: event.target.value })} /></label>
    <small id={`${id}-keys`} className="muted">{t('platform.awsConnectionForm.theseTwoFieldsAreAlways')}</small>
    <label className="connection-field">{t('platform.awsConnectionForm.allowedRegions')}<input name="aws-allowed-regions" value={fields.regions} autoComplete="off" spellCheck={false} placeholder="us-east-1, eu-west-1" aria-describedby={`${id}-regions`} onChange={(event) => set({ regions: event.target.value })} /></label>
    <small id={`${id}-regions`} className="muted">{t('platform.awsConnectionForm.separatedByCommasAtMost', { maxRegions, value: regionCount > 0 ? t('platform.awsConnectionForm.regionCountRegionsEntered', { regionCount }) : '' })}</small>
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {notice ? <div className="form-success" role="status">{notice}</div> : null}
    {result ? <dl className="destination-summary" aria-label={t('platform.awsConnectionForm.aWSVerificationResult')}>
      <div><dt>{t('platform.awsConnectionForm.account')}</dt> <dd>{result.config?.accountId ?? '—'}</dd></div>
      <div><dt>Principal</dt> <dd>{result.config?.principalArn ?? '—'}</dd></div>
      <div><dt>{t('platform.awsConnectionForm.regions')}</dt> <dd>{(result.config?.allowedRegions ?? []).join(', ') || result.config?.region || '—'}</dd></div>
      <div><dt>{t('platform.awsConnectionForm.identity')}</dt> <dd>{t('platform.awsConnectionForm.verifiedSTS')}</dd></div>
      <div><dt>{t('platform.awsConnectionForm.allowedRegions')}</dt> <dd>{t('platform.awsConnectionForm.verifiedPerRegion')}</dd></div>
      <div><dt>{t('platform.awsConnectionForm.provisioningPermissions')}</dt> <dd>{t('platform.awsConnectionForm.verifiedVPCEKSElastiCacheAurora')}</dd></div>
    </dl> : null}
    <div className="connection-footer"><Button type="submit" tone="primary" disabled={saving}>{saving ? t('platform.awsConnectionForm.checkingAndSaving') : t('platform.awsConnectionForm.checkAndSave')}</Button></div>
  </fieldset></form>;
}
