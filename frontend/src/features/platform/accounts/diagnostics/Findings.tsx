import { useState } from 'react';
import { Button } from '../../../../shared/ui/Button';
import { intlLocale, t } from '../../../../shared/i18n';
import { Icon } from './Icon';
import { checkExpected, checkTitle, factLabel, fixSteps, groupText, reasonText, verdicts, type Finding, type Report, type Verdict } from './diagnosticsApi';

const symbols: Record<Verdict, string> = { PASS: '✓', FAIL: '×', BLOCKED: '−', UNKNOWN: '?', NA: '—' };
export const verdictLabel = (v: Verdict) => t(`platform.diagnostics.verdict.${v}`);

export function Badge({ verdict }: { verdict: Verdict }) {
  return <span className={`dg-badge dg-${verdict.toLowerCase()}`}><span aria-hidden="true">{symbols[verdict]}</span>{verdictLabel(verdict)}</span>;
}

export const formatTime = (value: string) => new Date(value).toLocaleString(intlLocale(), { hour: '2-digit', minute: '2-digit', second: '2-digit', day: '2-digit', month: '2-digit' });
export { symbols };

function Inspector({ finding, report, onSelect }: { finding?: Finding; report: Report; onSelect(id: string): void }) {
  if (!finding) return <aside className="dg-inspector dg-inspector-empty">{t('platform.diagnostics.selectCheck')}</aside>;
  const steps = finding.verdict === 'PASS' || finding.verdict === 'NA' ? [] : fixSteps(finding.remediation);
  const facts = Object.entries(finding.facts).sort(([a], [b]) => a.localeCompare(b));
  return <aside className="dg-inspector" aria-label={t('platform.diagnostics.detail')}>
    <div className="dg-inspector-heading"><span className="dg-overline">{groupText(finding.group)}</span><Badge verdict={finding.verdict} /><h3>{checkTitle(finding.id)}</h3><code>{finding.id}</code></div>
    <div className="dg-inspector-body">
      <section><h4>{t('platform.diagnostics.observed')}</h4><p>{reasonText(finding.reason)}</p></section>
      {finding.verdict !== 'PASS' && finding.verdict !== 'NA' && <section><h4>{t('platform.diagnostics.expected')}</h4><p>{checkExpected(finding.id)}</p></section>}
      <section><h4>{report.scope.purpose === 'GENERAL' ? t('platform.diagnostics.reportContext') : t('platform.diagnostics.affected')}</h4>
        <div className="dg-tags">{report.scope.purpose === 'GENERAL' ? <span>{t('platform.diagnostics.general')}</span> : finding.affectedUseCases.length ? finding.affectedUseCases.map((uc) => <span key={uc}>{t(`platform.diagnostics.useCase.${uc}`)}</span>) : <span>{t('platform.diagnostics.noUseCase')}</span>}</div></section>
      {finding.verdict === 'BLOCKED' && finding.dependsOn.length > 0 && <section><h4>{t('platform.diagnostics.dependencies')}</h4>{finding.dependsOn.map((id) => {
        const dependency = report.findings.find((entry) => entry.id === id);
        return dependency ? <button className="dg-dependency" key={id} type="button" onClick={() => onSelect(id)}><span className="dg-fail-dot" />{checkTitle(id)}<Icon name="arrow" size={13} /></button> : <span key={id} className="dg-dependency">{checkTitle(id)}</span>;
      })}</section>}
      {steps.length > 0 && <section><h4>{t('platform.diagnostics.remediation')}</h4><ol className="dg-steps">{steps.map((step) => <li key={step}>{step}</li>)}</ol></section>}
      {finding.verdict === 'PASS' && <p className="dg-pass-help">{t('platform.diagnostics.passHelp')}</p>}
      <details className="dg-technical"><summary>{t('platform.diagnostics.technical')}</summary>
        <dl><dt>{t('platform.diagnostics.code')}</dt><dd><code>{finding.reason}</code></dd><dt>{t('platform.diagnostics.checkedAt')}</dt><dd>{formatTime(finding.observedAt)}</dd>
          {facts.map(([key, value]) => <div key={key}><dt>{factLabel(key)}</dt><dd><code>{value}</code></dd></div>)}</dl>
      </details>
    </div>
  </aside>;
}

export function Results({ report }: { report: Report }) {
  const actionable = report.findings.filter((f) => !['PASS', 'NA'].includes(f.verdict));
  const [filter, setFilter] = useState<'all' | 'actionable'>(actionable.length ? 'actionable' : 'all');
  const [query, setQuery] = useState('');
  const first = actionable.find((f) => f.verdict === 'FAIL') ?? actionable[0] ?? report.findings[0];
  const [selected, setSelected] = useState(first?.id ?? '');
  const needle = query.toLocaleLowerCase(intlLocale());
  const visible = report.findings.filter((f) => (filter === 'all' || !['PASS', 'NA'].includes(f.verdict)) && `${checkTitle(f.id)} ${groupText(f.group)} ${f.id}`.toLocaleLowerCase(intlLocale()).includes(needle));
  const item = report.findings.find((f) => f.id === selected);
  const pending = report.progress.total - report.findings.length;
  return <div className="dg-results-grid">
    <section className="dg-checks-panel" aria-labelledby="checks-heading">
      <div className="dg-panel-heading"><h2 id="checks-heading">{t('platform.diagnostics.results')}</h2><span>{t('platform.diagnostics.countChecks', { count: report.progress.total })}</span></div>
      <div className="dg-table-toolbar">
        <div className="dg-filter-toggle" role="group" aria-label={t('platform.diagnostics.results')}>
          <button type="button" aria-pressed={filter === 'actionable'} onClick={() => setFilter('actionable')}>{t('platform.diagnostics.actionable')}<span>{actionable.length}</span></button>
          <button type="button" aria-pressed={filter === 'all'} onClick={() => setFilter('all')}>{t('platform.diagnostics.all')}<span>{report.findings.length}</span></button>
        </div>
        <label className="dg-search"><Icon name="search" size={16} /><input aria-label={t('platform.diagnostics.search')} placeholder={t('platform.diagnostics.searchPlaceholder')} value={query} onChange={(event) => setQuery(event.target.value)} /></label>
      </div>
      <div className="dg-table-wrap"><table className="dg-check-table"><thead><tr><th scope="col">{t('platform.diagnostics.checkName')}</th><th scope="col">{t('platform.diagnostics.status')}</th><th scope="col">{t('platform.diagnostics.elapsed')}</th><th scope="col"><span className="dg-sr-only">{t('platform.diagnostics.detail')}</span></th></tr></thead>
        <tbody>{visible.map((f) => <tr key={f.id} data-selected={selected === f.id}>
          <td><button className="dg-check-link" type="button" onClick={() => setSelected(f.id)} aria-pressed={selected === f.id}><strong>{checkTitle(f.id)}</strong><small>{groupText(f.group)}</small></button></td>
          <td><Badge verdict={f.verdict} /></td>
          <td className="dg-duration">{f.verdict === 'BLOCKED' || f.verdict === 'NA' || !f.durationMs ? '—' : `${(f.durationMs / 1000).toFixed(1)} s`}</td>
          <td><button className="dg-row-open" type="button" aria-label={t('platform.diagnostics.viewCheck', { name: checkTitle(f.id) })} onClick={() => setSelected(f.id)}><Icon name="arrow" size={14} /></button></td>
        </tr>)}</tbody></table></div>
      {!visible.length && <div className="dg-no-results"><p>{t('platform.diagnostics.noMatches')}</p><Button onClick={() => { setQuery(''); setFilter('all'); }}>{t('platform.diagnostics.clearFilter')}</Button></div>}
      {pending > 0 && <p className="dg-pending-note" role="status">{t('platform.diagnostics.pendingChecks', { count: pending })}</p>}
    </section>
    <Inspector finding={item} report={report} onSelect={setSelected} />
  </div>;
}

export { verdicts };
