import { useEffect, useId, useState, type ReactNode } from 'react';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';

export function AdvancedFields({ label, invalid = false, children }: { label: string; invalid?: boolean; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const [opened, setOpened] = useState(false);
  const id = useId();
  useEffect(() => { if (invalid) { setOpen(true); setOpened(true); } }, [invalid]);
  return <div className="workload-advanced-fields">
    <Button tone="quiet" data-workload-advanced aria-label={t('workloads.workloadEditor.valueAdvancedLabel', { value: open ? t('workloads.workloadEditor.hide') : t('workloads.workloadEditor.show'), label })} aria-expanded={open} aria-controls={id} onClick={() => { setOpen(!open); setOpened(true); }}>{open ? t('workloads.workloadEditor.hideAdvanced') : t('workloads.workloadEditor.showAdvanced')}</Button>
    <div id={id} hidden={!open} style={open ? undefined : { display: 'none' }}>{open || opened ? children : null}</div>
  </div>;
}
