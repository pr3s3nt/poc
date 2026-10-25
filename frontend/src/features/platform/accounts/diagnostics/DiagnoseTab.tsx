import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError, describeError } from '../../../../shared/api/client';
import { Button } from '../../../../shared/ui/Button';
import { intlLocale, t } from '../../../../shared/i18n';
import type { Account } from '../accountsApi';
import { defaultDraft, DiagnosticSetup, type Draft } from './Setup';
import { Badge, Results, formatTime, symbols } from './Findings';
import { Icon } from './Icon';
import {
  active, cancelDiagnostic, getDiagnostic, limitText, listDiagnostics, overall, resourceStateText, scopeLabel, staleText, startDiagnostic, verdicts,
  type Purpose, type Report, type Summary, type Verdict,
} from './diagnosticsApi';

type Load<T> = { state: 'loading' } | { state: 'error'; message: string } | { state: 'ready'; value: T };
const pollMillis = 1000;
const lower: Record<Verdict, keyof Report['counts']> = { PASS: 'pass', FAIL: 'fail', BLOCKED: 'blocked', UNKNOWN: 'unknown', NA: 'na' };

/**
 * Diagnose tab of one Infrastructure Account. Every response is checked against
 * the scope (account, purpose) it was requested for, so a late answer for
 * another account, purpose or report can never be shown here. Progress comes
 * only from the persisted report; a lost start response is reconciled by
 * reading, never by starting again.
 */
export function DiagnoseTab({ account, purpose: purposeRoute, reportId, onRoute, onStale, onChanged }: {
  account: Account; purpose: Purpose; reportId?: string; onRoute(purpose: Purpose, reportId?: string): void; onStale(): void;
  /** A run started or ended: the parent re-reads the account list's latest summary. */
  onChanged?(): void;
}) {
  const purpose = purposeRoute;
  const scopeKey = `${account.key}/${purpose}`;
  const current = useRef(scopeKey);
  current.current = scopeKey;
  const [history, setHistory] = useState<Load<{ items: Summary[]; next: string }>>({ state: 'loading' });
  const [report, setReport] = useState<Load<Report | null>>({ state: 'loading' });
  const [view, setView] = useState<'results' | 'history'>('results');
  const [setup, setSetup] = useState(false);
  const [busy, setBusy] = useState<'start' | 'cancel' | null>(null);
  const [notice, setNotice] = useState<{ kind: 'error' | 'info'; text: string } | null>(null);
  const wanted = useRef(reportId);
  wanted.current = reportId;
  // Every report read takes a generation; only the newest read may answer, so
  // two reads of the same purpose (a poll, a history selection, a new start)
  // can never overwrite each other out of order, even when the scope returns to
  // a previous value.
  const generation = useRef(0);
  const shownId = useRef('');

  const stale = (key: string) => current.current !== key;

  const loadReport = useCallback(async (id: string, key: string, expected: Purpose = purpose) => {
    const mine = ++generation.current;
    const outdated = () => mine !== generation.current || stale(key);
    try {
      const value = await getDiagnostic(account.key, id);
      if (outdated()) return;
      if (value.scope.purpose !== expected || value.scope.accountKey !== account.key) {
        // An explicit report of another scope is never substituted by another one.
        setReport({ state: 'error', message: t('platform.diagnostics.reportMismatch') });
        return;
      }
      shownId.current = value.id;
      setReport({ state: 'ready', value });
    } catch (reason) {
      if (outdated()) return;
      setReport({ state: 'error', message: describeError(reason, t('platform.diagnostics.reportLoadFailed')) });
    }
  }, [account.key, purpose]);

  const loadHistory = useCallback(async (key: string, openLatest: boolean) => {
    try {
      const page = await listDiagnostics(account.key, purpose);
      if (stale(key)) return;
      setHistory({ state: 'ready', value: { items: page.reports, next: page.nextCursor } });
      if (openLatest) {
        const latest = page.reports[0]?.reportId;
        if (latest) void loadReport(latest, key); else { generation.current += 1; setReport({ state: 'ready', value: null }); }
      }
    } catch (reason) {
      if (stale(key)) return;
      const message = describeError(reason, t('platform.diagnostics.historyLoadFailed'));
      setHistory({ state: 'error', message });
      if (openLatest) { generation.current += 1; setReport({ state: 'error', message }); }
    }
  }, [account.key, purpose, loadReport]);

  // Switching account or purpose discards everything loaded for the old scope.
  useEffect(() => {
    generation.current += 1; shownId.current = '';
    setHistory({ state: 'loading' }); setReport({ state: 'loading' }); setView('results'); setNotice(null); setSetup(false); setBusy(null);
    const wantedId = wanted.current;
    if (wantedId) { void loadReport(wantedId, scopeKey); void loadHistory(scopeKey, false); } else void loadHistory(scopeKey, true);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scopeKey]);

  // A different report requested on the same open tab (for example from the
  // account list) is read directly, wherever it sits in the history.
  useEffect(() => {
    if (reportId && reportId !== shownId.current && current.current === scopeKey) {
      setReport({ state: 'loading' }); setView('results');
      void loadReport(reportId, scopeKey);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reportId]);

  // Poll the persisted report while it is in flight.
  const shown = report.state === 'ready' ? report.value : null;
  const inFlight = !!shown && active(shown.status);
  useEffect(() => {
    if (!shown || !inFlight) return;
    const key = scopeKey;
    const timer = window.setTimeout(() => { void loadReport(shown.id, key); }, pollMillis);
    return () => window.clearTimeout(timer);
  }, [shown, inFlight, scopeKey, loadReport]);
  // When a run ends, refresh the history so the list shows the final state.
  const finished = shown && !inFlight ? shown.id : '';
  useEffect(() => { if (finished) { void loadHistory(scopeKey, false); onChanged?.(); } // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [finished, scopeKey, loadHistory]);

  // Configure starts from the scope of the report on screen (what a rerun would
  // repeat), or the defaults when there is none; nothing unsaved outlives it.
  const draft = shown ? draftFrom(shown, account) : defaultDraft(account, purpose);

  async function start(next: Draft) {
    if (busy || inFlight) return;
    const key = scopeKey;
    setSetup(false); setNotice(null); setBusy('start');
    try {
      const created = await startDiagnostic(account.key, {
        version: account.version, purpose: next.purpose, mode: next.mode, ...(account.type === 'AWS' ? { region: next.region } : {}),
        ...(next.purpose === 'USE_CASE' ? { useCases: next.useCases, ...(next.application ? { application: next.application, environment: next.environment } : {}) } : {}),
      });
      if (stale(key)) return;
      generation.current += 1; shownId.current = created.id;
      setReport({ state: 'ready', value: created }); setView('results');
      onRoute(purpose, created.id);
      onChanged?.();
    } catch (reason) {
      if (stale(key)) return;
      if (reason instanceof ApiError) {
        if (reason.code === 'STALE_VERSION') { setNotice({ kind: 'error', text: t('platform.diagnostics.accountChanged') }); onStale(); }
        else if (reason.code === 'BUSY') { setNotice({ kind: 'info', text: t('platform.diagnostics.busy') }); void loadHistory(key, true); }
        else setNotice({ kind: 'error', text: reason.message });
      } else {
        // The outcome is unknown: read the history instead of starting again.
        setNotice({ kind: 'info', text: t('platform.diagnostics.startUnknown') });
        void loadHistory(key, true);
      }
    } finally { if (!stale(key)) setBusy(null); }
  }

  async function stop() {
    if (!shown || busy) return;
    const key = scopeKey;
    setBusy('cancel');
    try {
      const next = await cancelDiagnostic(account.key, shown.id);
      if (!stale(key) && shownId.current === next.id) { generation.current += 1; setReport({ state: 'ready', value: next }); }
    } catch (reason) {
      if (!stale(key)) setNotice({ kind: 'error', text: describeError(reason, t('platform.diagnostics.cancelFailed')) });
    } finally { if (!stale(key)) setBusy(null); }
  }

  function openReport(id: string) {
    setReport({ state: 'loading' }); setView('results'); setNotice(null);
    shownId.current = '';
    onRoute(purpose, id);
    void loadReport(id, scopeKey);
  }

  function download() {
    if (!shown) return;
    const body = { exportedAt: new Date().toISOString(), account: { key: account.key, name: account.name, type: account.type }, report: shown };
    const url = URL.createObjectURL(new Blob([JSON.stringify(body, null, 2)], { type: 'application/json' }));
    const anchor = document.createElement('a'); anchor.href = url; anchor.download = `diagnostic-${account.key}-${shown.id}.json`; anchor.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  const general = purpose === 'GENERAL';
  const items = history.state === 'ready' ? history.value.items : [];
  const hasReport = !!shown;
  return <section className="ac-diagnose" aria-label={t('platform.diagnostics.tab')}>
    <div className="ac-diagnose-toolbar">
      <div>
        <div className="ac-purpose-switch" role="group" aria-label={t('platform.diagnostics.scope')}>
          {(['GENERAL', 'USE_CASE'] as const).map((p) => <button key={p} type="button" aria-pressed={purpose === p} onClick={() => { if (p !== purpose) onRoute(p); }}>{p === 'GENERAL' ? t('platform.diagnostics.general') : t('platform.diagnostics.useCaseScope')}</button>)}
        </div>
        <p>{general ? t('platform.diagnostics.generalHelp') : t('platform.diagnostics.useCaseHelp')}</p>
      </div>
      <div className="dg-actions">
        <Button type="button" disabled={!!busy || inFlight} onClick={() => setSetup(true)}><Icon name="settings" size={15} />{t('platform.diagnostics.configure')}</Button>
        <Button type="button" tone="primary" disabled={!!busy || inFlight || report.state === 'loading'} onClick={() => (hasReport && shown ? void start(draftFrom(shown, account)) : general ? void start(draft) : setSetup(true))}>
          <Icon name={inFlight ? 'refresh' : 'play'} size={15} />{inFlight ? t('platform.diagnostics.running') : hasReport ? t('platform.diagnostics.runAgain') : t('platform.diagnostics.run')}
        </Button>
      </div>
    </div>
    {notice && <div className={notice.kind === 'error' ? 'form-error' : 'form-info'} role={notice.kind === 'error' ? 'alert' : 'status'}>{notice.text}</div>}
    {history.state === 'error' && report.state !== 'error' && <div className="form-error" role="alert">{history.message} <Button type="button" onClick={() => void loadHistory(scopeKey, false)}>{t('platform.diagnostics.retry')}</Button></div>}
    {report.state === 'loading' ? <section className="dg-state-panel" role="status" aria-busy="true"><h2>{t('platform.diagnostics.loading')}</h2></section>
      : report.state === 'error' ? <section className="dg-state-panel" role="alert"><span className="dg-state-icon dg-fail">!</span><h2>{t('platform.diagnostics.errorTitle')}</h2><p>{report.message}</p><Button type="button" onClick={() => { setReport({ state: 'loading' }); if (wanted.current) void loadReport(wanted.current, scopeKey); else void loadHistory(scopeKey, true); }}>{t('platform.diagnostics.retry')}</Button>{wanted.current && <Button type="button" onClick={() => { shownId.current = ''; onRoute(purpose); setReport({ state: 'loading' }); void loadHistory(scopeKey, true); }}>{t('platform.diagnostics.openLatestReport')}</Button>}</section>
      : !shown ? <section className="dg-state-panel"><span className="dg-state-icon"><Icon name="diagnose" size={30} /></span><h2>{general ? t('platform.diagnostics.generalEmpty') : t('platform.diagnostics.useCaseEmpty')}</h2><p>{general ? t('platform.diagnostics.generalEmptyHelp') : t('platform.diagnostics.useCaseEmptyHelp')}</p>
        <Button type="button" tone="primary" disabled={!!busy} onClick={() => (general ? void start(draft) : setSetup(true))}><Icon name="play" size={16} />{t('platform.diagnostics.run')}</Button></section>
      : <ReportView report={shown} view={view} onView={setView} history={history} items={items} onOpen={openReport} onStop={() => void stop()} onExport={download} busy={busy} onMore={async () => {
        if (history.state !== 'ready' || !history.value.next) return;
        const key = scopeKey;
        try { const page = await listDiagnostics(account.key, purpose, history.value.next); if (!stale(key)) setHistory({ state: 'ready', value: { items: [...history.value.items, ...page.reports], next: page.nextCursor } }); }
        catch (reason) { if (!stale(key)) setNotice({ kind: 'error', text: describeError(reason, t('platform.diagnostics.historyLoadFailed')) }); }
      }} />}
    {setup && <DiagnosticSetup account={account} draft={draft} onClose={() => setSetup(false)} onRun={(next) => void start(next)} />}
  </section>;
}

function draftFrom(report: Report, account: Account): Draft {
  return {
    purpose: report.scope.purpose, mode: report.scope.mode, region: report.scope.region ?? account.config?.verificationRegions?.[0] ?? '', useCases: [...report.scope.useCases],
    application: report.scope.context?.applicationKey ?? '', environment: report.scope.context?.environmentKey ?? '',
  };
}

function ReportView({ report, view, onView, history, items, onOpen, onStop, onExport, busy, onMore }: {
  report: Report; view: 'results' | 'history'; onView(v: 'results' | 'history'): void; history: Load<{ items: Summary[]; next: string }>; items: Summary[];
  onOpen(id: string): void; onStop(): void; onExport(): void; busy: string | null; onMore(): void;
}) {
  const running = active(report.status);
  const verdict = overall(report.counts, report.status);
  const general = report.scope.purpose === 'GENERAL';
  const bad = report.counts.fail + report.counts.blocked;
  const title = running ? t('platform.diagnostics.running') : report.status === 'CANCELLED' ? t('platform.diagnostics.summary.cancelled') : report.status === 'INTERRUPTED' ? t('platform.diagnostics.summary.interrupted')
    : bad ? (general ? t('platform.diagnostics.summary.generalFail') : t('platform.diagnostics.summary.needsFix')) : report.counts.unknown ? t('platform.diagnostics.summary.unknown') : report.ready ? t('platform.diagnostics.summary.ready') : t('platform.diagnostics.summary.incomplete');
  const help = running ? t('platform.diagnostics.progress', { done: report.progress.completed, total: report.progress.total })
    : bad || report.counts.unknown ? t('platform.diagnostics.failSummary', { fail: report.counts.fail, blocked: report.counts.blocked, unknown: report.counts.unknown })
      : report.ready ? t('platform.diagnostics.summary.readyHelp') : t('platform.diagnostics.summary.incompleteHelp');
  const tone = bad ? 'dg-summary-issues' : verdict === 'UNKNOWN' ? 'ac-summary-unknown' : 'dg-summary-ready';
  return <>
    <section className="ac-report-scope" aria-label={t('platform.diagnostics.reportContext')}>
      <span><Icon name="diagnose" size={14} />{general ? t('platform.diagnostics.general') : t('platform.diagnostics.useCaseScope')}</span>
      {report.scope.region && <span><strong>{t('platform.diagnostics.region')}</strong>{report.scope.region}</span>}
      {!general && <><span><strong>{t('platform.diagnostics.application')}</strong>{report.scope.context ? report.scope.context.applicationName ?? report.scope.context.applicationKey : t('platform.diagnostics.noAppContext')}{report.scope.context?.environmentKey && <small className="dg-env-label">{report.scope.context.environmentKey}</small>}</span><span>{t(`platform.diagnostics.mode.${report.scope.mode}`)}</span></>}
      <span className="ac-report-time"><Icon name="clock" size={12} />{t(`platform.diagnostics.status.${report.status}`)} · {formatTime(report.createdAt)} · v{report.scope.accountVersion}</span>
    </section>
    {report.stale && <div className="form-info dg-stale" role="status"><strong>{t('platform.diagnostics.stale')}</strong> {report.staleReasons.map(staleText).join(' ')}</div>}
    {running ? <section className="dg-running" aria-live="polite">
      <div className="dg-running-top"><span className="dg-spinner" /><div><h2>{title}</h2><p>{help}</p></div><Button type="button" disabled={report.status === 'CANCELLING' || !!busy} onClick={onStop}>{report.status === 'CANCELLING' ? t('platform.diagnostics.stopping') : t('platform.diagnostics.stop')}</Button></div>
      <progress max={report.progress.total || 1} value={report.progress.completed} aria-label={help} />
    </section> : null}
    <section className={`dg-summary ${tone}`} aria-label={title}>
      <div className="dg-summary-heading"><span className="dg-summary-icon" aria-hidden="true">{bad ? '!' : verdict === 'UNKNOWN' ? '?' : '✓'}</span><div><h2>{title}</h2><p>{help}</p></div>
        <Button tone="quiet" type="button" onClick={onExport}><Icon name="download" size={15} />{t('platform.diagnostics.export')}</Button></div>
      <div className="dg-counts">{verdicts.map((v) => <div key={v}><span className={`dg-count-symbol dg-${v.toLowerCase()}`} aria-hidden="true">{symbols[v]}</span><strong>{report.counts[lower[v]]}</strong><span>{t(`platform.diagnostics.verdict.${v}`)}</span></div>)}
        {report.counts.pending > 0 && <div><span className="dg-count-symbol" aria-hidden="true">…</span><strong>{report.counts.pending}</strong><span>{t('platform.diagnostics.pending')}</span></div>}</div>
    </section>
    {report.limits.length > 0 && <ul className="ac-limits" aria-label={t('platform.diagnostics.limits')}>{report.limits.map((code) => <li key={code}>{limitText(code)}</li>)}</ul>}
    <nav className="tabs dg-tabs ac-result-tabs" aria-label={t('platform.diagnostics.report')}>
      <button type="button" className={`tab${view === 'results' ? ' tab-active' : ''}`} onClick={() => onView('results')} aria-current={view === 'results' ? 'page' : undefined}>{t('platform.diagnostics.results')}</button>
      <button type="button" className={`tab${view === 'history' ? ' tab-active' : ''}`} onClick={() => onView('history')} aria-current={view === 'history' ? 'page' : undefined}>{t('platform.diagnostics.history')}{history.state === 'ready' && <span className="dg-history-count">{items.length}</span>}</button>
    </nav>
    {view === 'results' ? <Results key={`${report.id}:${running}`} report={report} /> : <HistoryPanel history={history} items={items} current={report.id} onOpen={onOpen} onMore={onMore} />}
    {report.cleanup.status !== 'NOT_REQUIRED' || report.cleanup.resources.length > 0 ? <div className="dg-cleanup-note" role="status">
      <strong>{t('platform.diagnostics.cleanup')}</strong> {t(`platform.diagnostics.cleanupStatus.${report.cleanup.status}`)}
      {report.cleanup.resources.length > 0 && <ul>{report.cleanup.resources.map((r) => <li key={`${r.kind}/${r.namespace ?? ''}/${r.name}`}><code>{r.kind}/{r.name}</code> {resourceStateText(r.state)}</li>)}</ul>}
    </div> : null}
  </>;
}

function HistoryPanel({ history, items, current, onOpen, onMore }: { history: Load<{ items: Summary[]; next: string }>; items: Summary[]; current: string; onOpen(id: string): void; onMore(): void }) {
  if (history.state === 'loading') return <section className="dg-history-panel" role="status" aria-busy="true">{t('platform.diagnostics.loading')}</section>;
  if (history.state === 'error') return <section className="dg-history-panel"><div className="form-error" role="alert">{history.message}</div></section>;
  return <section className="dg-history-panel">
    <table className="plain-table" aria-label={t('platform.diagnostics.history')}><thead><tr><th>{t('platform.diagnostics.report')}</th><th>{t('platform.diagnostics.scopeColumn')}</th><th>{t('platform.diagnostics.checkedAt')}</th><th>{t('platform.diagnostics.status')}</th><th /></tr></thead>
      <tbody>{items.map((entry) => <tr key={entry.reportId} data-current={entry.reportId === current}>
        <td><code>{entry.reportId.slice(0, 8)}</code></td><td>{scopeLabel(entry.scope)}{entry.stale && <small className="dg-stale-tag"> · {t('platform.diagnostics.stale')}</small>}</td>
        <td>{new Date(entry.createdAt).toLocaleString(intlLocale())}</td><td><Badge verdict={overall(entry.counts, entry.status)} /><small> {t(`platform.diagnostics.status.${entry.status}`)}</small></td>
        <td><Button type="button" aria-label={t('platform.diagnostics.viewReport', { id: entry.reportId.slice(0, 8) })} onClick={() => onOpen(entry.reportId)}>{t('platform.diagnostics.open')}</Button></td></tr>)}</tbody></table>
    {history.value.next && <Button type="button" onClick={onMore}>{t('platform.diagnostics.more')}</Button>}
  </section>;
}
