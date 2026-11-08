import { useEffect, useState } from 'react';
import { href, navigate } from '../../app/routes';
import type { EnvironmentKey } from '../../shared/types/application';
import { ApiError } from '../../shared/api/client';
import { Dialog } from '../../shared/ui/Dialog';
import { Button } from '../../shared/ui/Button';
import { currentLocale, t } from '../../shared/i18n';
import { getWorkloadTemplates, type WorkloadTemplate } from './templates';

const label = (item: WorkloadTemplate) => item.labels[currentLocale()] ?? item.labels.vi ?? item.labels.en ?? item.id;

// The contract accepts numeric major.minor.patch only; compare components rather
// than lexical version strings (1.10.0 must sort before 1.9.0).
function newestFirst(a: WorkloadTemplate, b: WorkloadTemplate) {
  const left = a.version.split('.').map(BigInt), right = b.version.split('.').map(BigInt);
  for (let i = 0; i < 3; i++) { const l = left[i] ?? 0n, r = right[i] ?? 0n; if (l !== r) return l > r ? -1 : 1; }
  return 0;
}

function TemplateCard({ versions, onSelect }: { versions: WorkloadTemplate[]; onSelect(template: WorkloadTemplate): void }) {
  const [version, setVersion] = useState(versions[0]?.version);
  const selected = versions.find(item => item.version === version) ?? versions[0];
  if (!selected) return null;
  return <div className="application-card">
    <button className="template-picker-choice" onClick={() => onSelect(selected)}>
      <strong>{label(selected)}</strong><small>{selected.kind} · {selected.version}</small>
      {selected.descriptions?.[currentLocale()] ? <p>{selected.descriptions[currentLocale()]}</p> : null}
    </button>
    {versions.length > 1 ? <details><summary>{t('workloads.templates.versions')}</summary>
      <label className="editor-grid">{t('workloads.templates.version')}<select aria-label={`${t('workloads.templates.version')} · ${label(selected)}`} value={selected.version} onChange={event => setVersion(event.target.value)}>
        {versions.map(item => <option key={item.version} value={item.version}>{item.version}</option>)}
      </select></label>
    </details> : null}
  </div>;
}

export function TemplatePicker({ app, environment, onClose, onSelect, onCustom }: { app: string; environment: EnvironmentKey; onClose(): void; onSelect(template: WorkloadTemplate): void; onCustom(): void }) {
  const [templates, setTemplates] = useState<WorkloadTemplate[]>();
  const [error, setError] = useState<unknown>();
  const [attempt, setAttempt] = useState(0);
  const [search, setSearch] = useState('');
  useEffect(() => { let cancelled = false; setTemplates(undefined); setError(undefined); getWorkloadTemplates(app, environment).then(items => { if (!cancelled) setTemplates(items); }).catch(error => { if (!cancelled) setError(error); }); return () => { cancelled = true; }; }, [app, environment, attempt]);
  const catalogRequired = error instanceof ApiError && error.code === 'CATALOG_REQUIRED';
  const settings = { name: 'settings' as const, applicationId: app, environment };
  const groups = new Map<string, WorkloadTemplate[]>();
  for (const item of templates ?? []) groups.set(item.id, [...(groups.get(item.id) ?? []), item]);
  const filtered = [...groups.values()].map(items => items.sort(newestFirst)).filter(items => items.some(item => `${label(item)} ${item.kind}`.toLowerCase().includes(search.toLowerCase())));
  return <Dialog label={t('workloads.templates.choose')} onClose={onClose}><div className="section-header"><h2>{t('workloads.templates.choose')}</h2><Button onClick={onClose}>{t('workloads.templates.cancel')}</Button></div><div className="editor-grid"><label>{t('workloads.templates.search')}<input value={search} onChange={e => setSearch(e.target.value)} /></label></div>
    {error ? <div className="form-error" role="alert">{catalogRequired ? t('workloads.templates.catalogRequired') : t('workloads.templates.unavailable')} {catalogRequired ? <a href={href(settings)} onClick={event => { event.preventDefault(); onClose(); navigate(settings); }}>{t('applications.applicationHome.environmentSettings')}</a> : <Button onClick={() => setAttempt(a => a + 1)}>{t('workloads.templates.retry')}</Button>}</div> : !templates ? <p role="status">{t('workloads.templates.loading')}</p> : !filtered?.length ? <p>{t('workloads.templates.empty')}</p> : <div className="template-picker-list">{filtered.map(versions => <TemplateCard key={`${app}/${environment}/${versions[0]?.id}`} versions={versions} onSelect={onSelect} />)}</div>}
    <div className="form-actions"><Button tone="quiet" onClick={onCustom}>{t('workloads.templates.custom')}</Button></div>
  </Dialog>;
}
