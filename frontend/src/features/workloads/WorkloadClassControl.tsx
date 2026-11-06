import type { ReactNode } from 'react';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import { Advanced } from './Advanced';

export function WorkloadClassControl({ value, classes, metadata, disabled, onChange, onReload, children }: {
  children?: ReactNode;
  value: string;
  classes: string[];
  metadata: 'ready' | 'loading' | 'error';
  disabled: boolean;
  onChange(value: string): void;
  onReload(): void;
}) {
  const choices = [...new Set(classes)].filter(name => name !== 'default').sort();
  const retained = value !== '' && !choices.includes(value);
  return <Advanced label="Workload" summary={t('workloads.workloadClass.summary', { value: value || 'default' })}>
    <label>{t('workloads.workloadEditor.workloadClass')}
      <select aria-label={t('workloads.workloadEditor.workloadClass')} value={value} disabled={disabled || metadata !== 'ready'} onChange={event => onChange(event.target.value)}>
        <option value="">{t('workloads.workloadClass.default')}</option>
        {choices.map(name => <option key={name} value={name}>{name}</option>)}
        {retained ? <option value={value}>{t('workloads.workloadClass.retained', { value })}</option> : null}
      </select>
    </label>
    {retained && value !== 'default' && metadata === 'ready' ? <small className="muted">{t('workloads.workloadClass.unpublished')}</small> : null}
    {metadata === 'loading' ? <small role="status">{t('workloads.workloadClass.loading')}</small> : null}
    {metadata === 'error' ? <div className="form-error" role="alert">{t('workloads.workloadClass.unavailable')}{' '}<Button disabled={disabled} type="button" onClick={onReload}>{t('workloads.classControls.retryLoadingClassMetadata')}</Button></div> : null}
    {children}
  </Advanced>;
}
