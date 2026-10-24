import { useState, type ReactNode } from 'react';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';

// Closing only hides the inputs; the form state lives in the page, so edited
// and imported overrides survive. The summary stays visible while closed.
export function Advanced({ label, summary, children }: { label: string; summary: ReactNode; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return <div className="advanced-section">
    <div role="note" aria-label={t('workloads.workloadEditor.advancedSummaryLabel', { label })} className="muted">{summary}</div>
    <Button tone="quiet" aria-expanded={open} aria-label={t('workloads.workloadEditor.valueAdvancedLabel', { value: open ? t('workloads.workloadEditor.hide') : t('workloads.workloadEditor.show'), label })} onClick={() => setOpen(!open)}>{open ? t('workloads.workloadEditor.hideAdvanced') : t('workloads.workloadEditor.showAdvanced')}</Button>
    {open ? children : null}
  </div>;
}
