import { useId } from 'react';
import { t } from '../../../shared/i18n';

export function Ec2RoleFields({ roleArn, regions, disabled, onRoleArn, onRegions }: {
  roleArn: string; regions: string; disabled: boolean; onRoleArn(value: string): void; onRegions(value: string): void;
}) {
  const id = useId();
  return <>
    <p className="muted">{t('platform.accounts.ec2RoleFields.theBackendAndExecutorMust')}</p>
    <label className="connection-field">IAM Role ARN<input value={roleArn} disabled={disabled} required autoComplete="off" spellCheck={false} maxLength={2048} placeholder="arn:aws:iam::123456789012:role/OrchestratorRole" aria-describedby={`${id}-role`} onChange={(event) => onRoleArn(event.target.value)} /></label>
    <small id={`${id}-role`} className="muted">{t('platform.accounts.ec2RoleFields.enterTheARNOfThe')}</small>
    <label className="connection-field">{t('platform.accounts.ec2RoleFields.checkRegions')}<input value={regions} disabled={disabled} required autoComplete="off" spellCheck={false} placeholder="us-east-1, eu-west-1" aria-describedby={`${id}-regions`} onChange={(event) => onRegions(event.target.value)} /></label>
    <small id={`${id}-regions`} className="muted">{t('platform.accounts.ec2RoleFields.atMost10RegionsTo')}</small>
  </>;
}
