import type { ReactNode, SelectHTMLAttributes } from 'react';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';

type DefaultSelectProps = Omit<SelectHTMLAttributes<HTMLSelectElement>, 'value' | 'defaultValue' | 'onChange' | 'placeholder'> & {
  value: string;
  defaultLabel: ReactNode;
  onValueChange(value: string): void;
};

// The inherited hint is shown by the closed control, never as a duplicate choice.
export function DefaultSelect({ value, defaultLabel, onValueChange, children, className, disabled, ...props }: DefaultSelectProps) {
  return <>
    <select {...props} disabled={disabled} className={[className, value === '' ? 'inherited-default' : ''].filter(Boolean).join(' ')} value={value} onChange={event => onValueChange(event.target.value)}>
      <option value="" hidden disabled>{defaultLabel}</option>
      {children}
    </select>
    {value !== '' ? <Button type="button" tone="quiet" disabled={disabled} onClick={() => onValueChange('')}>{t('workloads.defaults.reset')}</Button> : null}
  </>;
}
