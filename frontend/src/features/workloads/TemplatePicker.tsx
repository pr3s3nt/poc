import { useEffect, useState } from 'react';
import { Dialog } from '../../shared/ui/Dialog';
import { Button } from '../../shared/ui/Button';
import { currentLocale, t } from '../../shared/i18n';
import { getWorkloadTemplates, type WorkloadTemplate } from './templates';

export function TemplatePicker({ app, environment, onClose, onSelect, onCustom }: { app: string; environment: string; onClose(): void; onSelect(template: WorkloadTemplate): void; onCustom(): void }) {
  const [templates, setTemplates] = useState<WorkloadTemplate[]>();
  const [error, setError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [search, setSearch] = useState('');
  useEffect(() => { let cancelled = false; setTemplates(undefined); setError(false); getWorkloadTemplates(app, environment).then(items => { if (!cancelled) setTemplates(items); }).catch(() => { if (!cancelled) setError(true); }); return () => { cancelled = true; }; }, [app, environment, attempt]);
  const label = (item: WorkloadTemplate) => item.labels[currentLocale()] ?? item.labels.vi ?? item.labels.en ?? item.id;
  const filtered = templates?.filter(item => `${label(item)} ${item.kind}`.toLowerCase().includes(search.toLowerCase()));
  return <Dialog label={t('workloads.templates.choose')} onClose={onClose}><div className="section-header"><h2>{t('workloads.templates.choose')}</h2><Button onClick={onClose}>{t('workloads.templates.cancel')}</Button></div><div className="editor-grid"><label>{t('workloads.templates.search')}<input value={search} onChange={e => setSearch(e.target.value)} /></label></div>
    {error ? <div className="form-error" role="alert">{t('workloads.templates.unavailable')} <Button onClick={() => setAttempt(a => a + 1)}>{t('workloads.templates.retry')}</Button></div> : !templates ? <p role="status">{t('workloads.templates.loading')}</p> : !filtered?.length ? <p>{t('workloads.templates.empty')}</p> : <div className="template-picker-list">{filtered.map(item => <button className="application-card" key={`${item.id}@${item.version}`} onClick={() => onSelect(item)}><strong>{label(item)}</strong><small>{item.kind} · {item.version}</small>{item.descriptions?.[currentLocale()] ? <p>{item.descriptions[currentLocale()]}</p> : null}</button>)}</div>}
    <div className="form-actions"><Button tone="quiet" onClick={onCustom}>{t('workloads.templates.custom')}</Button></div>
  </Dialog>;
}
