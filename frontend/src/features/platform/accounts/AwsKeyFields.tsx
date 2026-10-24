import { useId } from 'react';
import { maxRegions, parseRegions } from './accountsApi';
import { t } from '../../../shared/i18n';

export type AwsFields = { accessKeyId: string; secretAccessKey: string; regions: string };
export const emptyAwsFields: AwsFields = { accessKeyId: '', secretAccessKey: '', regions: '' };

/**
 * Concealed AWS access-key inputs and the “Vùng kiểm tra” field. The regions
 * are verification evidence only; the deployment region is chosen later in the
 * Driver Inputs of a Definition.
 */
export function AwsKeyFields({ fields, disabled, onChange, prefix }: { fields: AwsFields; disabled: boolean; onChange(patch: Partial<AwsFields>): void; prefix: string }) {
  const id = useId();
  const count = parseRegions(fields.regions).length;
  return <>
    <label className="connection-field">Access key ID<input type="password" name={`${prefix}-access-key-id`} value={fields.accessKeyId} disabled={disabled} autoComplete="new-password" spellCheck={false} maxLength={20} aria-describedby={`${id}-keys`} onChange={(event) => onChange({ accessKeyId: event.target.value })} /></label>
    <label className="connection-field">Secret access key<input type="password" name={`${prefix}-secret-access-key`} value={fields.secretAccessKey} disabled={disabled} autoComplete="new-password" spellCheck={false} maxLength={40} aria-describedby={`${id}-keys`} onChange={(event) => onChange({ secretAccessKey: event.target.value })} /></label>
    <small id={`${id}-keys`} className="muted">{t('platform.accounts.awsKeyFields.theseTwoFieldsAreAlways')}</small>
    <label className="connection-field">{t('platform.accounts.awsKeyFields.checkRegions')}<input name={`${prefix}-verification-regions`} value={fields.regions} disabled={disabled} autoComplete="off" spellCheck={false} placeholder="us-east-1, eu-west-1" aria-describedby={`${id}-regions`} onChange={(event) => onChange({ regions: event.target.value })} /></label>
    <small id={`${id}-regions`} className="muted">{t('platform.accounts.awsKeyFields.regionsUsedToCheckRead', { maxRegions, value: count > 0 ? t('platform.accounts.awsKeyFields.countEntered', { count }) : '' })}</small>
  </>;
}
