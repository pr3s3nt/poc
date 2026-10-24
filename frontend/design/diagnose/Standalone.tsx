import { useEffect, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { AppShell } from '../../src/app/AppShell';
import { ApplicationsPage } from '../../src/features/applications/ApplicationsPage';
import { Button } from '../../src/shared/ui/Button';
import { emptyEnvironment } from '../../src/shared/types/application';
import { setCatalogs, t } from '../../src/shared/i18n';
import vi from '../../public/locales/vi.json';
import messages from './messages.json';
import { countVerdicts, executorIdentity, initialScope, makeReport, targetName, useCases, type Check, type Report, type Scenario, type Scope, type UseCase, type Verdict } from './fixtures';
import '../../src/styles/index.css';
import './style.css';

setCatalogs('vi', { ...vi, ...messages }, undefined, [{ code: 'vi', name: 'Tiếng Việt', intl: 'vi-VN' }]);
const text = (key: string, params?: Record<string, string | number>) => t(`diagnose.${key}`, params);
const verdicts: Verdict[] = ['PASS', 'FAIL', 'BLOCKED', 'UNKNOWN', 'NA'];
const verdictKeys: Record<Verdict, string> = { PASS: 'pass', FAIL: 'fail', BLOCKED: 'blocked', UNKNOWN: 'unknown', NA: 'na' };
const verdictSymbols: Record<Verdict, string> = { PASS: '✓', FAIL: '×', BLOCKED: '−', UNKNOWN: '?', NA: '—' };
const formatTime = (value: string) => new Date(value).toLocaleString('vi-VN', { hour: '2-digit', minute: '2-digit', second: '2-digit', day: '2-digit', month: '2-digit' });

export function Icon({ name, size = 18 }: { name: 'diagnose' | 'play' | 'settings' | 'refresh' | 'arrow' | 'search' | 'clock' | 'download' | 'cluster' | 'cloud'; size?: number }) {
  const paths: Record<string, ReactNode> = {
    diagnose: <><path d="M4 4h16v16H4zM8 2v4m8-4v4M8 18v4m8-4v4M2 8h4m12 0h4M2 16h4m12 0h4" /><path d="m7 12 3 0 2-4 2 8 2-4h2" /></>,
    play: <path d="m9 5 11 7-11 7V5Z" />,
    settings: <><path d="M4 6h16M4 12h16M4 18h16" /><circle cx="9" cy="6" r="2" /><circle cx="15" cy="12" r="2" /><circle cx="9" cy="18" r="2" /></>,
    refresh: <path d="M20 11a8 8 0 1 0-2 6M20 4v7h-7" />,
    arrow: <path d="m9 5 7 7-7 7" />,
    search: <><circle cx="10" cy="10" r="6" /><path d="m15 15 5 5" /></>,
    clock: <><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></>,
    download: <path d="M12 3v12m-4-4 4 4 4-4M4 16v5h16v-5" />,
    cluster: <><rect x="8" y="2" width="8" height="6" rx="1" /><rect x="2" y="16" width="8" height="6" rx="1" /><rect x="14" y="16" width="8" height="6" rx="1" /><path d="M12 8v4M6 16v-4h12v4" /></>,
    cloud: <path d="M6 18a5 5 0 0 1-1-10 7 7 0 0 1 13 0 5 5 0 0 1 0 10H6Z" />,
  };
  return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{paths[name]}</svg>;
}

export function Badge({ status }: { status: Verdict }) {
  return <span className={`dg-badge dg-${status.toLowerCase()}`}><span aria-hidden="true">{verdictSymbols[status]}</span>{text(verdictKeys[status])}</span>;
}

export function EnvironmentTabs({ value, onChange }: { value: Scope['environment']; onChange(value: Scope['environment']): void }) {
  return <div className="dg-environments" role="group" aria-label={text('environment')}>{(['staging', 'production'] as const).map((env) => <button key={env} type="button" aria-pressed={value === env} onClick={() => onChange(env)}>{env}</button>)}</div>;
}

function SetupDialog({ scope, onClose, onRun }: { scope: Scope; onClose(): void; onRun(scope: Scope): void }) {
  const [draft, setDraft] = useState<Scope>({ ...scope, useCases: [...scope.useCases] });
  const [validation, setValidation] = useState(false);
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const returnFocus = document.activeElement as HTMLElement | null;
    dialog.current?.showModal();
    return () => { returnFocus?.focus(); };
  }, []);
  function toggle(uc: UseCase) {
    setValidation(false);
    setDraft({ ...draft, useCases: draft.useCases.includes(uc) ? draft.useCases.filter((item) => item !== uc) : [...draft.useCases, uc] });
  }
  return <dialog className="dg-dialog" ref={dialog} aria-labelledby="setup-title" onCancel={onClose} onClick={(event) => { if (event.target === event.currentTarget) onClose(); }}>
    <form onSubmit={(event) => { event.preventDefault(); if (!draft.useCases.length) { setValidation(true); return; } onRun(draft); }}>
      <header><div><p className="eyebrow">{text('mock')}</p><h2 id="setup-title">{text('configure')}</h2></div><button type="button" className="dg-close" aria-label={text('cancel')} onClick={onClose}>×</button></header>
      <div className="dg-dialog-body">
        <div className="dg-form-grid">
          <label>{text('account')}<select value={draft.target} onChange={(event) => setDraft({ ...draft, target: event.target.value as Scope['target'] })}><option value="kubernetes">K8S · staging-cluster</option><option value="aws">AWS · Sandbox</option></select></label>
          {draft.target === 'aws' ? <label>{text('region')}<select value={draft.region} onChange={(event) => setDraft({ ...draft, region: event.target.value })}><option>ap-southeast-1</option><option>us-east-1</option><option>eu-west-1</option></select></label> : <div className="dg-field-summary"><span>{text('cluster')}</span><strong>staging-cluster</strong></div>}
          <label>{text('application')}<select value={draft.application} onChange={(event) => setDraft({ ...draft, application: event.target.value })}><option>storefront</option><option>payments</option></select></label>
          <div className="dg-field-summary"><span>{text('environment')}</span><EnvironmentTabs value={draft.environment} onChange={(environment) => setDraft({ ...draft, environment })} /></div>
        </div>
        <fieldset className="dg-usecase-picker"><legend>{text('useCases')}</legend><div>{useCases.map((uc) => <label key={uc}><input type="checkbox" checked={draft.useCases.includes(uc)} onChange={() => toggle(uc)} /><span>{text(uc)}</span></label>)}</div></fieldset>
        {validation && <p className="form-error" role="alert">{text('validation')}</p>}
        <fieldset className="dg-mode-picker"><legend>{text('mode')}</legend>{(['conditions', 'execution'] as const).map((mode) => <label key={mode} data-selected={draft.mode === mode}><input type="radio" name="mode" value={mode} checked={draft.mode === mode} onChange={() => setDraft({ ...draft, mode })} /><span><strong>{text(mode)}</strong><small>{text(`${mode}Help`)}</small></span></label>)}</fieldset>
        {draft.mode === 'execution' && <p className="dg-mode-note">{text('executionNote')}</p>}
        <p className="dg-runtime-note"><Icon name="diagnose" size={15} />{text('scopeHelp')}</p>
      </div>
      <footer><Button type="button" onClick={onClose}>{text('cancel')}</Button><Button type="submit" tone="primary"><Icon name="play" size={15} />{text('startMock')}</Button></footer>
    </form>
  </dialog>;
}

function Inspector({ check: item, report, onRecheck, onSelect }: { check?: Check; report: Report; onRecheck(): void; onSelect(id: string): void }) {
  if (!item) return <aside className="dg-inspector dg-inspector-empty">{text('selectCheck')}</aside>;
  const affected = item.useCases.filter((uc) => report.scope.useCases.includes(uc));
  return <aside className="dg-inspector" aria-label={text('detail')}>
    <div className="dg-inspector-heading"><span className="dg-overline">{item.group}</span><Badge status={item.status} /><h3>{item.name}</h3><code>{item.id}</code></div>
    <div className="dg-inspector-body">
      <section><h4>{text('observed')}</h4><p>{item.observed}</p></section>
      {item.status !== 'PASS' && item.status !== 'NA' && <section><h4>{text('expected')}</h4><p>{item.expected}</p></section>}
      {report.scope.purpose === 'general' ? <section><h4>{text('reportContext')}</h4><div className="dg-tags"><span>{text('general')}</span></div></section> : <section><h4>{text('affected')}</h4><div className="dg-tags">{affected.map((uc) => <span key={uc}>{text(uc)}</span>)}</div></section>}
      {item.status === 'BLOCKED' && <section><h4>{text('dependencies')}</h4>{item.dependencies?.map((id) => { const dependency = report.checks.find((entry) => entry.id === id); return dependency ? <button className="dg-dependency" key={id} type="button" onClick={() => onSelect(id)}><span className="dg-fail-dot" />{dependency.name}<Icon name="arrow" size={13} /></button> : null; })}</section>}
      {item.status !== 'PASS' && item.status !== 'NA' && item.steps.length > 0 && <section><h4>{text('remediation')}</h4><ol className="dg-steps">{item.steps.map((step) => <li key={step}>{step}</li>)}</ol></section>}
      {item.status === 'PASS' && <p className="dg-pass-help">{text('passHelp')}</p>}
      <details className="dg-technical"><summary>{text('technical')}</summary><dl><dt>{text('identity')}</dt><dd><code>{executorIdentity(report.scope.target)}</code></dd><dt>{text('origin')}</dt><dd><code>platform / executor</code></dd><dt>{text('checkedAt')}</dt><dd>{formatTime(report.checkedAt)}</dd></dl><pre>{item.technical}</pre></details>
    </div>
    <footer><Button onClick={onRecheck} title={text('withDependencies')}><Icon name="refresh" size={15} />{text('recheckItem')}</Button><small>{text('recheckNote')}</small></footer>
  </aside>;
}

export function Results({ report, initialCheck, onRecheck }: { report: Report; initialCheck: string; onRecheck(): void }) {
  const counts = countVerdicts(report.checks);
  const actionable = counts.FAIL + counts.BLOCKED + counts.UNKNOWN;
  const initialStatus = report.checks.find((entry) => entry.id === initialCheck)?.status;
  const [filter, setFilter] = useState<'all' | 'actionable'>(actionable && initialStatus !== 'PASS' && initialStatus !== 'NA' ? 'actionable' : 'all');
  const [query, setQuery] = useState('');
  const [selected, setSelected] = useState(initialCheck || report.checks.find((item) => item.id === 'vault-tls' && item.status === 'FAIL')?.id || report.checks.find((item) => item.status === 'FAIL')?.id || report.checks.find((item) => !['PASS', 'NA'].includes(item.status))?.id || report.checks[0]?.id);
  const visible = report.checks.filter((item) => (filter === 'all' || !['PASS', 'NA'].includes(item.status)) && `${item.name} ${item.group} ${item.id}`.toLocaleLowerCase('vi').includes(query.toLocaleLowerCase('vi')));
  const item = report.checks.find((entry) => entry.id === selected);
  return <div className="dg-results-grid">
    <section className="dg-checks-panel" aria-labelledby="checks-heading">
      <div className="dg-panel-heading"><h2 id="checks-heading">{text('results')}</h2><span>{text('countChecks', { count: report.checks.length })}</span></div>
      <div className="dg-table-toolbar"><div className="dg-filter-toggle" role="group" aria-label={text('results')}><button type="button" aria-pressed={filter === 'actionable'} onClick={() => setFilter('actionable')}>{text('actionable')}<span>{actionable}</span></button><button type="button" aria-pressed={filter === 'all'} onClick={() => setFilter('all')}>{text('all')}<span>{report.checks.length}</span></button></div><label className="dg-search"><Icon name="search" size={16} /><input aria-label={text('search')} placeholder={text('searchPlaceholder')} value={query} onChange={(event) => setQuery(event.target.value)} /></label></div>
      <div className="dg-table-wrap"><table className="dg-check-table"><thead><tr><th scope="col">{text('checkName')}</th><th scope="col">{text('status')}</th><th scope="col">{text('elapsed')}</th><th scope="col"><span className="dg-sr-only">{text('detail')}</span></th></tr></thead><tbody>{visible.map((entry) => <tr key={entry.id} data-selected={selected === entry.id}><td><button className="dg-check-link" type="button" onClick={() => setSelected(entry.id)} aria-pressed={selected === entry.id}><strong>{entry.name}</strong><small>{entry.group}</small></button></td><td><Badge status={entry.status} /></td><td className="dg-duration">{entry.status === 'BLOCKED' || entry.status === 'NA' ? '—' : `${(entry.milliseconds / 1000).toFixed(1)} s`}</td><td><button className="dg-row-open" type="button" aria-label={text('viewCheck', { name: entry.name })} onClick={() => setSelected(entry.id)}><Icon name="arrow" size={14} /></button></td></tr>)}</tbody></table></div>
      {!visible.length && <div className="dg-no-results"><p>{text('noMatches')}</p><Button onClick={() => { setQuery(''); setFilter('all'); }}>{text('clearFilter')}</Button></div>}
      <div className="dg-table-footer"><Icon name="diagnose" size={14} /><span>{text('conditionLimit')}</span></div>
    </section>
    <Inspector check={item} report={report} onRecheck={onRecheck} onSelect={setSelected} />
  </div>;
}

function UseCaseResults({ report, onOpen }: { report: Report; onOpen(id: string): void }) {
  return <div className="dg-usecase-grid">{report.scope.useCases.map((uc) => {
    const checks = report.checks.filter((item) => item.useCases.includes(uc));
    const counts = countVerdicts(checks);
    const status: Verdict = counts.FAIL ? 'FAIL' : counts.BLOCKED ? 'BLOCKED' : counts.UNKNOWN ? 'UNKNOWN' : 'PASS';
    return <section className="dg-usecase-card" key={uc}><header><h2>{text(uc)}</h2><Badge status={status} /></header><p>{text('useCaseCount', { count: checks.length })}</p><ul>{checks.filter((item) => item.status !== 'PASS' && item.status !== 'NA').map((item) => <li key={item.id}><span className={`dg-mini-status dg-${item.status.toLowerCase()}`}>{verdictSymbols[item.status]}</span>{item.name}</li>)}</ul><Button onClick={() => onOpen(checks.find((item) => item.status === 'FAIL')?.id ?? checks[0]?.id ?? '')}>{text('open')}<Icon name="arrow" size={13} /></Button></section>;
  })}</div>;
}

export function Standalone() {
  const [report, setReport] = useState<Report | null>(() => makeReport(initialScope, 'issues', 1));
  const [history, setHistory] = useState<Report[]>(() => report ? [report] : []);
  const [scope, setScope] = useState(initialScope);
  const [scenario, setScenario] = useState<Scenario>('issues');
  const [setup, setSetup] = useState(false);
  const [tab, setTab] = useState<'results' | 'useCases' | 'history'>('results');
  const [notice, setNotice] = useState('');
  const [run, setRun] = useState<{ candidate: Report; done: number } | null>(null);
  const [resultsKey, setResultsKey] = useState(0);
  const [initialCheck, setInitialCheck] = useState('');
  const sequence = useRef(1);
  const shell = useRef<HTMLDivElement>(null);
  const [navSlot] = useState(() => document.createElement('div'));
  useEffect(() => {
    navSlot.className = 'dg-nav-slot';
    const nav = shell.current?.querySelector('.nav');
    nav?.insertBefore(navSlot, nav.querySelector('.nav-item-disabled'));
    return () => navSlot.remove();
  }, [navSlot]);
  useEffect(() => {
    if (!run) return;
    const timeout = window.setTimeout(() => {
      if (run.done + 1 >= run.candidate.checks.length) {
        setReport(run.candidate); setHistory((items) => [run.candidate, ...items]);
        setRun(null); setResultsKey((key) => key + 1);
      } else setRun({ ...run, done: run.done + 1 });
    }, 130);
    return () => window.clearTimeout(timeout);
  }, [run]);

  function start(nextScope = scope) {
    setScope(nextScope); setSetup(false); setNotice(''); setTab('results'); setInitialCheck('');
    const candidate = makeReport(nextScope, scenario, ++sequence.current);
    setRun({ candidate, done: 0 }); setScenario(candidate.scenario);
  }
  function changeScenario(next: Scenario) {
    setScenario(next); setNotice(''); setTab('results'); setInitialCheck('');
    if (next === 'empty' || next === 'error') { setReport(null); return; }
    const candidate = makeReport(scope, next, ++sequence.current);
    setReport(candidate); setHistory((items) => [candidate, ...items]); setResultsKey((key) => key + 1);
  }
  function exportReport() {
    if (!report) return;
    const contents = JSON.stringify({ mock: true, execution: 'SIMULATED', ...report, identity: executorIdentity(report.scope.target) }, null, 2);
    const url = URL.createObjectURL(new Blob([contents], { type: 'application/json' }));
    const anchor = document.createElement('a'); anchor.href = url; anchor.download = `MOCK-diagnose-${report.id}.json`; anchor.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  const displayed = run?.candidate ?? report;
  const counts = report ? countVerdicts(report.checks) : null;
  const failed = !!counts && (counts.FAIL > 0 || counts.BLOCKED > 0);
  const allDone = !!counts && !failed && counts.UNKNOWN === 0 && report?.scope.mode === 'execution';
  const title = failed ? 'needsFix' : allDone ? 'complete' : 'preliminary';
  const activeScope = displayed?.scope ?? scope;
  return <div ref={shell} onClickCapture={(event) => {
    if ((event.target as HTMLElement).closest('.sidebar a:not(.dg-nav-link)')) { event.preventDefault(); event.stopPropagation(); setNotice(text('demoNavigation')); }
  }}>
    <AppShell username="platform.demo" role="PLATFORM_ENGINEER" activeRoute="diagnose" onSignOut={() => setNotice(text('demoNavigation'))}>
      {createPortal(<a className="nav-item nav-item-active dg-nav-link" aria-current="page" href="#diagnose" title={text('title')} onClick={(event) => { event.preventDefault(); setTab('results'); }}><span className="nav-icon" data-tone="teal"><Icon name="diagnose" size={20} /></span><span className="sidebar-label">{text('title')}</span></a>, navSlot)}
      <section className="page dg-page">
        <header className="page-header dg-page-header"><div><p className="eyebrow">{text('platform')}</p><h1>{text('title')}</h1><p>{text('subtitle')}</p></div><div className="dg-actions"><Button onClick={() => setSetup(true)} disabled={!!run}><Icon name="settings" size={16} />{text('configure')}</Button><Button tone="primary" disabled={!!run} onClick={() => report ? start() : setSetup(true)}><Icon name={run ? 'refresh' : 'play'} size={16} />{text(run ? 'running' : report ? 'runAgain' : 'run')}</Button></div></header>
        <div className="dg-demo-bar"><div><span className="dg-mock-label">{text('mock')}</span><span>{text('mockNotice')}</span></div><label>{text('scenario')}<select value={scenario} disabled={!!run} onChange={(event) => changeScenario(event.target.value as Scenario)}><option value="issues">{text('issues')}</option><option value="ready">{text('ready')}</option><option value="empty">{text('empty')}</option><option value="error">{text('apiError')}</option></select></label></div>
        {notice && <div className="dg-notice" role="status">{notice}<button aria-label={text('dismiss')} onClick={() => setNotice('')}>×</button></div>}
        <section className="dg-scope-bar" aria-label={text('reportContext')}><div className="dg-target-icon"><Icon name={activeScope.target === 'aws' ? 'cloud' : 'cluster'} size={22} /></div><div className="dg-target"><span>{text('target')}</span><strong>{targetName(activeScope.target)}</strong></div><div className="dg-scope-app"><span>{text('application')}</span><strong>{activeScope.application}<span className="dg-env-label">{activeScope.environment}</span></strong></div>{activeScope.target === 'aws' && <div><span>{text('region')}</span><strong>{activeScope.region}</strong></div>}<div className="dg-scope-mode"><span>{text('mode')}</span><strong>{text(activeScope.mode)}</strong></div>{displayed && <div className="dg-scope-time"><span><Icon name="clock" size={12} />{displayed.id}</span><strong>{formatTime(displayed.checkedAt)}</strong></div>}</section>
        {run ? <section className="dg-running" aria-live="polite"><div className="dg-running-top"><span className="dg-spinner" /><div><h2>{text('running')}</h2><p>{text('progress', { done: run.done, total: run.candidate.checks.length })}</p></div><Button onClick={() => { setRun(null); setScope(report?.scope ?? initialScope); setScenario(report?.scenario ?? 'empty'); setNotice(text('stopped')); }}>{text('stop')}</Button></div><progress max={run.candidate.checks.length} value={run.done} aria-label={text('progress', { done: run.done, total: run.candidate.checks.length })} /><ol>{run.candidate.checks.map((item, index) => <li key={item.id}><span>{item.name}</span>{index < run.done ? <Badge status={item.status} /> : <span className="dg-waiting">{index === run.done ? text('checking') : text('pending')}</span>}</li>)}</ol></section> : scenario === 'error' ? <section className="dg-state-panel" role="alert"><span className="dg-state-icon dg-fail">!</span><h2>{text('errorTitle')}</h2><p>{text('errorHelp')}</p><Button onClick={() => changeScenario('issues')}>{text('retry')}</Button></section> : !report || !counts ? <section className="dg-state-panel"><span className="dg-state-icon"><Icon name="diagnose" size={30} /></span><h2>{text('emptyTitle')}</h2><p>{text('emptyHelp')}</p><Button tone="primary" onClick={() => setSetup(true)}><Icon name="play" size={16} />{text('run')}</Button></section> : <>
          <section className={`dg-summary${failed ? ' dg-summary-issues' : ' dg-summary-ready'}`} aria-label={text(title)}><div className="dg-summary-heading"><span className="dg-summary-icon" aria-hidden="true">{failed ? '!' : '✓'}</span><div><h2>{text(title)}</h2><p>{failed ? text('failSummary', { fail: counts.FAIL, blocked: counts.BLOCKED, unknown: counts.UNKNOWN }) : text(allDone ? 'completeHelp' : 'preliminaryHelp')}</p></div><Button tone="quiet" onClick={exportReport}><Icon name="download" size={15} />{text('export')}</Button></div><div className="dg-counts">{verdicts.map((status) => <div key={status}><span className={`dg-count-symbol dg-${status.toLowerCase()}`} aria-hidden="true">{verdictSymbols[status]}</span><strong>{counts[status]}</strong><span>{text(verdictKeys[status])}</span></div>)}</div></section>
          <nav className="tabs dg-tabs" aria-label={text('report')}><button className={`tab${tab === 'results' ? ' tab-active' : ''}`} aria-current={tab === 'results' ? 'page' : undefined} onClick={() => setTab('results')}>{text('results')}</button><button className={`tab${tab === 'useCases' ? ' tab-active' : ''}`} aria-current={tab === 'useCases' ? 'page' : undefined} onClick={() => setTab('useCases')}>{text('byUseCase')}</button><button className={`tab${tab === 'history' ? ' tab-active' : ''}`} aria-current={tab === 'history' ? 'page' : undefined} onClick={() => setTab('history')}>{text('history')}<span className="dg-history-count">{history.length}</span></button><span className="dg-snapshot">{text('snapshot')}</span></nav>
          {tab === 'results' && <Results key={`${report.id}-${resultsKey}-${initialCheck}`} report={report} initialCheck={initialCheck} onRecheck={() => start()} />}
          {tab === 'useCases' && <UseCaseResults report={report} onOpen={(id) => { setInitialCheck(id); setTab('results'); }} />}
          {tab === 'history' && <section className="dg-history-panel"><table className="plain-table"><thead><tr><th>{text('report')}</th><th>{text('target')}</th><th>{text('application')}</th><th>{text('checkedAt')}</th><th>{text('status')}</th><th /></tr></thead><tbody>{history.map((entry) => { const counts = countVerdicts(entry.checks); return <tr key={entry.id}><td><code>{entry.id}</code></td><td>{targetName(entry.scope.target)}</td><td>{entry.scope.application}<small>{entry.scope.environment}</small></td><td>{formatTime(entry.checkedAt)}</td><td><Badge status={counts.FAIL ? 'FAIL' : counts.UNKNOWN ? 'UNKNOWN' : 'PASS'} /></td><td><Button aria-label={text('viewReport', { id: entry.id })} onClick={() => { setReport(entry); setScope(entry.scope); setScenario(entry.scenario); setInitialCheck(''); setResultsKey((key) => key + 1); setTab('results'); }}>{text('open')}</Button></td></tr>; })}</tbody></table></section>}
          {report.scope.mode === 'execution' && <div className="dg-cleanup-note"><span className="dg-pass">✓</span><strong>{text('cleanup')}</strong>{text(report.scope.target === 'aws' ? 'awsCleanupResult' : 'cleanupResult')}</div>}
        </>}
      </section>
      {setup && <SetupDialog scope={scope} onClose={() => setSetup(false)} onRun={start} />}
    </AppShell>
  </div>;
}

export function ReferenceApplications() {
  return <AppShell username="platform.demo" role="PLATFORM_ENGINEER" activeRoute="applications" onSignOut={() => undefined}><ApplicationsPage applications={['storefront', 'payments'].map((name) => ({ id: name, name, subdomain: name, environments: { staging: emptyEnvironment, production: emptyEnvironment }, workloads: { staging: [], production: [] } }))} /></AppShell>;
}

