import type { ReactNode } from 'react';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import { Advanced } from './Advanced';
import { offeredClasses } from '../../shared/classChoices';

export function WorkloadClassControl({ value, classes, defaultAvailable = false, saved = false, metadata, disabled, onChange, onReload, children, embedded = false }: {
  children?: ReactNode;
  embedded?: boolean;
  value: string;
  classes: string[];
  /** The catalog serves the default class (GET /resource-types `defaultAvailable`). */
  defaultAvailable?: boolean;
  /** `value` is the class stored in the loaded Workload. */
  saved?: boolean;
  metadata: 'ready' | 'loading' | 'error';
  disabled: boolean;
  onChange(value: string): void;
  onReload(): void;
}) {
  // UC-16 BR-33: Default only when served; an unset new value asks for a choice.
  const offered = offeredClasses({ classes, defaultAvailable }, '');
  const choices = offered.filter(name => name !== '');
  const retained = value !== '' && !choices.includes(value);
  const content = <>
    <label>{t('workloads.workloadEditor.workloadClass')}
      <select aria-label={t('workloads.workloadEditor.workloadClass')} value={value} disabled={disabled || metadata !== 'ready'} onChange={event => onChange(event.target.value)}>
        {offered.includes('') ? <option value="">{t('workloads.workloadClass.default')}</option> : value === '' ? <option value="" disabled={!saved}>{saved ? t('shared.classChoices.defaultUnavailable') : t('shared.classChoices.choose')}</option> : null}
        {choices.map(name => <option key={name} value={name}>{name}</option>)}
        {retained ? <option value={value}>{t('workloads.workloadClass.retained', { value })}</option> : null}
      </select>
    </label>
    {retained && value !== 'default' && metadata === 'ready' ? <small className="muted">{t('workloads.workloadClass.unpublished')}</small> : null}
    {metadata === 'loading' ? <small role="status">{t('workloads.workloadClass.loading')}</small> : null}
    {metadata === 'error' ? <div className="form-error" role="alert">{t('workloads.workloadClass.unavailable')}{' '}<Button disabled={disabled} type="button" onClick={onReload}>{t('workloads.classControls.retryLoadingClassMetadata')}</Button></div> : null}
    {children}
  </>;
  return embedded ? content : <Advanced label="Workload" summary={t('workloads.workloadClass.summary', { value: value || 'default' })}>{content}</Advanced>;
}
