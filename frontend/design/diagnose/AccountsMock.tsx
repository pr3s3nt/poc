import { useEffect, useRef, useState } from 'react';
import { AppShell } from '../../src/app/AppShell';
import { Button } from '../../src/shared/ui/Button';
import { setCatalogs, t } from '../../src/shared/i18n';
import vi from '../../public/locales/vi.json';
import messages from './messages.json';
import accountMessages from './account-messages.json';
import { Badge, EnvironmentTabs, Icon, Results } from './Standalone';
import { countVerdicts, executorIdentity, useCases, type Report, type Scenario, type Scope, type Verdict } from './fixtures';
import { accounts, defaultScope, initialAccountReports, makeAccountReport, type MockAccount, type Purpose } from './accountFixtures';
import './accounts.css';

setCatalogs('vi', { ...vi, ...messages, ...accountMessages }, undefined, [{ code: 'vi', name: 'Tiếng Việt', intl: 'vi-VN' }]);
const a = (key: string, params?: Record<string, string | number>) => t(`accounts.${key}`, params);
const d = (key: string, params?: Record<string, string | number>) => t(`diagnose.${key}`, params);
const when = (value: string) => new Date(value).toLocaleString('vi-VN', { hour: '2-digit', minute: '2-digit', day: '2-digit', month: '2-digit' });
const statuses: Verdict[] = ['PASS', 'FAIL', 'BLOCKED', 'UNKNOWN', 'NA'];
const statusKeys: Record<Verdict, string> = { PASS: 'pass', FAIL: 'fail', BLOCKED: 'blocked', UNKNOWN: 'unknown', NA: 'na' };
const symbols: Record<Verdict, string> = { PASS: '✓', FAIL: '×', BLOCKED: '−', UNKNOWN: '?', NA: '—' };
type Route = { key?: string; tab: 'information' | 'diagnose'; purpose: Purpose };
function readRoute(): Route {
  const parts = window.location.hash.replace(/^#/, '').split('/');
  return { key: accounts.some((item) => item.key === parts[1]) ? parts[1] : undefined,
    tab: parts[2] === 'diagnose' ? 'diagnose' : 'information', purpose: parts[3] === 'usecase' ? 'usecase' : 'general' };
}
function scopeLabel(scope: Scope) {
  return [a(scope.purpose === 'general' ? 'general' : 'usecase'), scope.target === 'aws' ? scope.region : '',
    scope.purpose === 'usecase' && scope.application ? `${scope.application} / ${scope.environment}` : ''].filter(Boolean).join(' · ');
}
function reportStatus(report: Report): Verdict {
  const counts = countVerdicts(report.checks);
  return counts.FAIL ? 'FAIL' : counts.BLOCKED ? 'BLOCKED' : counts.UNKNOWN ? 'UNKNOWN' : 'PASS';
}
function ConnectionBadge() {
  return <span className="ac-connection-badge" title={a('verifiedHint')}><span aria-hidden="true">✓</span>{a('connectionVerified')}</span>;
}

function AccountSetup({ account, scope, onClose, onRun }: { account: MockAccount; scope: Scope; onClose(): void; onRun(scope: Scope): void }) {
  const [draft, setDraft] = useState<Scope>({ ...scope, useCases: [...scope.useCases] });
  const [validation, setValidation] = useState(false);
  const ref = useRef<HTMLDialogElement>(null);
  const general = draft.purpose === 'general';
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    ref.current?.showModal();
    return () => previous?.focus();
  }, []);
  return <dialog ref={ref} className="dg-dialog ac-setup" aria-labelledby="ac-setup-title" onCancel={onClose} onClick={(event) => { if (event.target === event.currentTarget) onClose(); }}>
    <form onSubmit={(event) => { event.preventDefault(); if (!general && !draft.useCases.length) { setValidation(true); return; } onRun(draft); }}>
      <header><div><p className="eyebrow">{a('fixedAccount')}</p><h2 id="ac-setup-title">{a(general ? 'configureGeneral' : 'configureUsecase')}</h2></div><button type="button" className="dg-close" aria-label={d('cancel')} onClick={onClose}>×</button></header>
      <div className="dg-dialog-body">
        <div className="ac-locked-target"><Icon name={account.target === 'aws' ? 'cloud' : 'cluster'} size={22} /><div><strong>{account.name}</strong><small>{account.destination}</small></div><ConnectionBadge /></div>
        {account.target === 'aws' && <label className="ac-region-field">{d('region')}<select value={draft.region} onChange={(event) => setDraft({ ...draft, region: event.target.value })}><option>ap-southeast-1</option><option>us-east-1</option><option>eu-west-1</option></select></label>}
        {!general && <>
          <fieldset className="dg-usecase-picker"><legend>{d('useCases')}</legend><div>{useCases.map((uc) => <label key={uc}><input type="checkbox" checked={draft.useCases.includes(uc)} onChange={() => { setValidation(false); setDraft({ ...draft, useCases: draft.useCases.includes(uc) ? draft.useCases.filter((item) => item !== uc) : [...draft.useCases, uc] }); }} /><span>{d(uc)}</span></label>)}</div></fieldset>
          {validation && <p className="form-error" role="alert">{d('validation')}</p>}
          <div className="dg-form-grid ac-application-fields"><label>{a('applicationOptional')}<select value={draft.application} onChange={(event) => setDraft({ ...draft, application: event.target.value })}><option value="">{a('noApp')}</option><option>storefront</option><option>payments</option></select></label>{draft.application && <div className="dg-field-summary"><span>{d('environment')}</span><EnvironmentTabs value={draft.environment} onChange={(environment) => setDraft({ ...draft, environment })} /></div>}</div>
          <p className="ac-field-help">{a('applicationHelp')}</p>
          <fieldset className="dg-mode-picker"><legend>{d('mode')}</legend>{(['conditions', 'execution'] as const).map((mode) => <label key={mode} data-selected={draft.mode === mode}><input type="radio" name="mode" checked={draft.mode === mode} onChange={() => setDraft({ ...draft, mode })} /><span><strong>{d(mode)}</strong><small>{d(`${mode}Help`)}</small></span></label>)}</fieldset>
          {draft.mode === 'execution' && <p className="dg-mode-note">{d('executionNote')}</p>}
        </>}
        {general && <p className="ac-general-readonly">{a('readOnlyScope')}</p>}
        <p className="dg-runtime-note"><Icon name="diagnose" size={15} />{d('scopeHelp')}</p>
      </div>
      <footer><Button type="button" onClick={onClose}>{d('cancel')}</Button><Button type="submit" tone="primary"><Icon name="play" size={15} />{d('startMock')}</Button></footer>
    </form>
  </dialog>;
}

function AccountList({ history, runningAccount, onOpen }: { history: Report[]; runningAccount?: string; onOpen(key: string, tab: Route['tab'], purpose?: Purpose): void }) {
  return <>
    <section className="content-panel ac-account-list"><div className="section-header"><h2>{a('registered')}</h2><span className="ac-total">{accounts.length}</span></div>
      <div className="ac-account-table-wrap"><table className="connection-table ac-account-table" aria-label={a('title')}><thead><tr><th scope="col">{a('name')}</th><th scope="col">{a('type')}</th><th scope="col">{a('auth')}</th><th scope="col">{a('destination')}</th><th scope="col">{a('connection')}</th><th scope="col">{a('lastDiagnosis')}</th><th scope="col"><span className="dg-sr-only">{a('diagnose')}</span></th></tr></thead><tbody>
        {accounts.map((account) => {
          const last = history.find((report) => report.scope.accountKey === account.key);
          const counts = last ? countVerdicts(last.checks) : null;
          return <tr key={account.key}><td data-label={a('name')}><button className="link-button ac-account-name" aria-label={a('viewAccount', { name: account.name })} onClick={() => onOpen(account.key, 'information')}><strong>{account.name}</strong></button><small className="connection-key">{account.key}</small></td><td data-label={a('type')}><span className={`ac-provider ac-provider-${account.target}`}><Icon name={account.target === 'aws' ? 'cloud' : 'cluster'} size={15} />{account.target === 'aws' ? 'AWS' : a('cluster')}</span></td><td data-label={a('auth')}>{account.authentication}</td><td data-label={a('destination')}><code>{account.destination}</code></td><td data-label={a('connection')}><span className="ac-connection-state">✓ {a('verified')}</span><small>{a('connectionTime', { time: '16:10 · 24/10' })}</small></td><td data-label={a('lastDiagnosis')}>{runningAccount === account.key ? <span className="ac-latest-running">{a('running')}</span> : last && counts ? <button className="ac-latest-result" onClick={() => onOpen(account.key, 'diagnose', last.scope.purpose ?? 'general')}><span className={`ac-result-label dg-${reportStatus(last).toLowerCase()}`}>{counts.FAIL ? a('summaryFailure', { count: counts.FAIL }) : counts.UNKNOWN ? a('summaryUnknown', { count: counts.UNKNOWN }) : a('summaryPass')}</span><small>{scopeLabel(last.scope)}</small><small>{when(last.checkedAt)}</small></button> : <span className="ac-not-checked">{a('noDiagnosis')}</span>}</td><td><Button aria-label={a('inspect', { name: account.name })} onClick={() => onOpen(account.key, 'diagnose')}>{a('diagnose')}</Button></td></tr>;
        })}
      </tbody></table></div>
    </section>
  </>;
}

function AccountInformation({ account, onDiagnose, onVerify }: { account: MockAccount; onDiagnose(): void; onVerify(): void }) {
  const aws = account.target === 'aws';
  return <section className="content-panel account-detail ac-information" aria-label={a('information')}>
    <div className="section-header"><h2>{a('information')}</h2><Button onClick={onVerify}><Icon name="refresh" size={15} />{a('checkConnection')}</Button></div>
    <dl className="destination-summary"><div><dt>{a('type')}</dt><dd>{aws ? 'AWS' : a('cluster')}</dd></div><div><dt>{a('auth')}</dt><dd>{account.authentication}</dd></div><div><dt>{a(aws ? 'accountId' : 'clusterName')}</dt><dd><code>{account.destination}</code></dd></div>
      <div><dt>{a('identity')}</dt><dd><code>{executorIdentity(account.target)}</code></dd></div>
      {aws ? <div><dt>{a('region')}</dt><dd>ap-southeast-1, us-east-1</dd></div> : <><div><dt>{a('context')}</dt><dd>platform-{account.destination}</dd></div><div><dt>{a('endpoint')}</dt><dd>https://kubernetes.{account.destination}.example:6443</dd></div><div><dt>TLS</dt><dd>{t('platform.accounts.accountDetail.customCA')}</dd></div></>}
      <div><dt>{a('lastConnection')}</dt><dd>16:10 · 24/10/2026</dd></div>
    </dl>
    <div className="ac-information-footer"><p>{a('connectionScope')}</p><Button onClick={onDiagnose}>{a('openDiagnosis')}<Icon name="arrow" size={14} /></Button></div>
  </section>;
}

export function AccountsMock() {
  const [route, setRoute] = useState<Route>(readRoute);
  const [history, setHistory] = useState(initialAccountReports);
  const [scenarios, setScenarios] = useState<Record<string, Scenario>>({});
  const [configs, setConfigs] = useState<Record<string, Scope>>({});
  const [viewedId, setViewedId] = useState<string | null>(null);
  const [view, setView] = useState<'results' | 'history'>('results');
  const [notice, setNotice] = useState('');
  const [setup, setSetup] = useState(false);
  const [run, setRun] = useState<{ candidate: Report; done: number } | null>(null);
  const sequence = useRef(4);
  useEffect(() => {
    function changed() { setRoute(readRoute()); setSetup(false); }
    window.addEventListener('hashchange', changed);
    return () => window.removeEventListener('hashchange', changed);
  }, []);
  useEffect(() => {
    if (!run) return;
    const id = window.setTimeout(() => {
      if (run.done + 1 >= run.candidate.checks.length) {
        setHistory((items) => [run.candidate, ...items]); setRun(null);
      } else setRun({ ...run, done: run.done + 1 });
    }, 150);
    return () => window.clearTimeout(id);
  }, [run]);
  const account = accounts.find((item) => item.key === route.key);
  const scopeKey = `${account?.key ?? ''}/${route.purpose}`;
  const accountHistory = history.filter((entry) => entry.scope.accountKey === account?.key);
  const scopedHistory = accountHistory.filter((entry) => entry.scope.purpose === route.purpose);
  const selectedReport = accountHistory.find((entry) => entry.id === viewedId && entry.scope.purpose === route.purpose);
  const report = selectedReport ?? scopedHistory[0] ?? null;
  const scope = report?.scope ?? configs[scopeKey] ?? (account ? defaultScope(account, route.purpose) : defaultScope(accounts[0]!, 'general'));
  const scenario = scenarios[scopeKey] ?? report?.scenario ?? 'issues';
  const activeRun = run?.candidate.scope.accountKey === account?.key && run?.candidate.scope.purpose === route.purpose ? run : null;
  function go(key?: string, tab: Route['tab'] = 'information', purpose: Purpose = 'general', reportId: string | null = null) {
    setViewedId(reportId); setView('results'); setSetup(false); setNotice('');
    const next = { key, tab, purpose }; setRoute(next);
    window.location.hash = key ? `accounts/${key}/${tab}/${purpose}` : 'accounts';
  }
  function start(nextScope = scope) {
    if (run) return;
    const candidate = makeAccountReport(nextScope, scenario, ++sequence.current);
    setConfigs((current) => ({ ...current, [scopeKey]: candidate.scope }));
    setScenarios((current) => ({ ...current, [scopeKey]: candidate.scenario }));
    setSetup(false); setViewedId(null); setView('results'); setNotice(''); setRun({ candidate, done: 0 });
  }
  function changeScenario(next: Scenario) {
    setScenarios((current) => ({ ...current, [scopeKey]: next })); setViewedId(null); setView('results');
    if (next === 'empty') { setHistory((items) => items.filter((entry) => !(entry.scope.accountKey === account?.key && entry.scope.purpose === route.purpose))); return; }
    if (next === 'error') return;
    setHistory((items) => [makeAccountReport(scope, next, ++sequence.current), ...items]);
  }
  function download() {
    if (!report || !account) return;
    const url = URL.createObjectURL(new Blob([JSON.stringify({ mock: true, execution: 'SIMULATED', account: account.name, ...report }, null, 2)], { type: 'application/json' }));
    const anchor = document.createElement('a'); anchor.href = url; anchor.download = `MOCK-${account.key}-${report.id}.json`; anchor.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  const general = route.purpose === 'general';
  const counts = report ? countVerdicts(report.checks) : null;
  const failures = counts ? counts.FAIL + counts.BLOCKED : 0;
  const summaryTitle = failures ? (general ? a('generalFailure') : d('needsFix')) : counts?.UNKNOWN ? a('unknownTitle') : general ? a('generalPass') : d(scope.mode === 'execution' ? 'complete' : 'preliminary');
  const currentScope = activeRun?.candidate.scope ?? scope;
  return <div onClickCapture={(event) => {
    const link = (event.target as HTMLElement).closest<HTMLAnchorElement>('.sidebar a');
    if (!link) return;
    event.preventDefault(); event.stopPropagation();
    if (link.getAttribute('href')?.includes('driver-accounts')) go(); else setNotice(a('mockNavigation'));
  }}>
    <AppShell username="platform.demo" role="PLATFORM_ENGINEER" activeRoute="driver-accounts" onSignOut={() => setNotice(a('mockNavigation'))}>
      <section className="page dg-page ac-page">
        {account ? <>
          <nav className="ac-breadcrumb" aria-label={a('back')}><button onClick={() => go()}>{a('title')}</button><Icon name="arrow" size={12} /><span>{account.name}</span></nav>
          <header className="ac-account-heading"><div className="dg-target-icon"><Icon name={account.target === 'aws' ? 'cloud' : 'cluster'} size={24} /></div><div><h1>{account.name}</h1><p><code>{account.key}</code><span>·</span>{account.authentication}<span>·</span>{a('version', { version: account.version })}</p></div><ConnectionBadge /></header>
        </> : null}
        {!account && <header className="page-header ac-list-heading"><div><p className="eyebrow">{d('platform')}</p><h1>{a('title')}</h1><p>{a('subtitle')}</p></div></header>}
        <div className="dg-demo-bar ac-demo-bar"><div><span className="dg-mock-label">{d('mock')}</span><span>{d('mockNotice')}</span></div>{account && route.tab === 'diagnose' && <label>{d('scenario')}<select value={scenario} disabled={!!run} onChange={(event) => changeScenario(event.target.value as Scenario)}><option value="issues">{d('issues')}</option><option value="ready">{d('ready')}</option><option value="empty">{d('empty')}</option><option value="error">{d('apiError')}</option></select></label>}</div>
        {notice && <div className="dg-notice" role="status">{notice}<button aria-label={d('dismiss')} onClick={() => setNotice('')}>×</button></div>}
        {!account ? <AccountList history={history} runningAccount={run?.candidate.scope.accountKey} onOpen={go} /> : <>
          <nav className="tabs ac-account-tabs" aria-label={account.name}><button className={`tab${route.tab === 'information' ? ' tab-active' : ''}`} aria-current={route.tab === 'information' ? 'page' : undefined} onClick={() => go(account.key, 'information', route.purpose)}>{a('information')}</button><button className={`tab${route.tab === 'diagnose' ? ' tab-active' : ''}`} aria-current={route.tab === 'diagnose' ? 'page' : undefined} onClick={() => go(account.key, 'diagnose', route.purpose)}>{a('diagnose')}</button></nav>
          {route.tab === 'information' ? <AccountInformation account={account} onVerify={() => setNotice(a('connectionSuccess'))} onDiagnose={() => go(account.key, 'diagnose')} /> : <>
            <div className="ac-diagnose-toolbar"><div><div className="ac-purpose-switch" role="group" aria-label={a('scope')}>{(['general', 'usecase'] as const).map((purpose) => <button key={purpose} aria-pressed={route.purpose === purpose} onClick={() => go(account.key, 'diagnose', purpose)}>{a(purpose)}</button>)}</div><p>{a(general ? 'generalHelp' : 'usecaseHelp')}</p></div><div className="dg-actions"><Button disabled={!!run} onClick={() => setSetup(true)}><Icon name="settings" size={15} />{d('configure')}</Button><Button tone="primary" disabled={!!run} onClick={() => report && scenario !== 'empty' ? start() : setSetup(true)}><Icon name={activeRun ? 'refresh' : 'play'} size={15} />{d(activeRun ? 'running' : report && scenario !== 'empty' ? 'runAgain' : 'run')}</Button></div></div>
            <section className="ac-report-scope" aria-label={d('reportContext')}><span><Icon name="diagnose" size={14} />{a(general ? 'general' : 'usecase')}</span>{currentScope.target === 'aws' && <span><strong>{d('region')}</strong>{currentScope.region}</span>}{!general && <><span><strong>{d('application')}</strong>{currentScope.application || a('noAppContext')}{currentScope.application && <small className="dg-env-label">{currentScope.environment}</small>}</span><span>{d(currentScope.mode)}</span></>}{report && <span className="ac-report-time"><Icon name="clock" size={12} />{report.id} · {when(report.checkedAt)}</span>}</section>
            {activeRun ? <section className="dg-running" aria-live="polite"><div className="dg-running-top"><span className="dg-spinner" /><div><h2>{d('running')}</h2><p>{d('progress', { done: activeRun.done, total: activeRun.candidate.checks.length })}</p></div><Button onClick={() => { setRun(null); setNotice(d('stopped')); }}>{d('stop')}</Button></div><progress max={activeRun.candidate.checks.length} value={activeRun.done} aria-label={d('running')} /><ol>{activeRun.candidate.checks.map((item, index) => <li key={item.id}><span>{item.name}</span>{index < activeRun.done ? <Badge status={item.status} /> : <span className="dg-waiting">{d(index === activeRun.done ? 'checking' : 'pending')}</span>}</li>)}</ol></section>
              : scenario === 'error' ? <section className="dg-state-panel" role="alert"><span className="dg-state-icon dg-fail">!</span><h2>{d('errorTitle')}</h2><p>{d('errorHelp')}</p><Button onClick={() => setScenarios((current) => ({ ...current, [scopeKey]: report?.scenario ?? 'issues' }))}>{d('retry')}</Button></section>
              : !report || scenario === 'empty' || !counts ? <section className="dg-state-panel"><span className="dg-state-icon"><Icon name="diagnose" size={30} /></span><h2>{a(general ? 'generalEmpty' : 'usecaseEmpty')}</h2><p>{a(general ? 'generalEmptyHelp' : 'usecaseEmptyHelp')}</p><Button tone="primary" onClick={() => general ? start() : setSetup(true)} disabled={!!run}><Icon name="play" size={16} />{d('run')}</Button></section>
              : <>
                <section className={`dg-summary ${failures ? 'dg-summary-issues' : counts.UNKNOWN ? 'ac-summary-unknown' : 'dg-summary-ready'}`} aria-label={summaryTitle}><div className="dg-summary-heading"><span className="dg-summary-icon" aria-hidden="true">{failures ? '!' : counts.UNKNOWN ? '?' : '✓'}</span><div><h2>{summaryTitle}</h2><p>{failures ? d('failSummary', { fail: counts.FAIL, blocked: counts.BLOCKED, unknown: counts.UNKNOWN }) : counts.UNKNOWN ? a('remainingUnknown', { count: counts.UNKNOWN }) : a(general ? 'generalLimit' : 'usecaseLimit')}</p></div><Button tone="quiet" onClick={download}><Icon name="download" size={15} />{d('export')}</Button></div><div className="dg-counts">{statuses.map((status) => <div key={status}><span className={`dg-count-symbol dg-${status.toLowerCase()}`} aria-hidden="true">{symbols[status]}</span><strong>{counts[status]}</strong><span>{d(statusKeys[status])}</span></div>)}</div></section>
                <nav className="tabs dg-tabs ac-result-tabs" aria-label={d('report')}><button className={`tab${view === 'results' ? ' tab-active' : ''}`} onClick={() => setView('results')} aria-current={view === 'results' ? 'page' : undefined}>{a('results')}</button><button className={`tab${view === 'history' ? ' tab-active' : ''}`} onClick={() => setView('history')} aria-current={view === 'history' ? 'page' : undefined}>{d('history')}<span className="dg-history-count">{accountHistory.length}</span></button><span className="dg-snapshot">{d('snapshot')}</span></nav>
                {view === 'results' ? <Results key={report.id} report={report} initialCheck="" onRecheck={() => start(report.scope)} /> : <section className="dg-history-panel"><table className="plain-table" aria-label={a('allHistory')}><thead><tr><th>{d('report')}</th><th>{a('scopeColumn')}</th><th>{d('checkedAt')}</th><th>{d('status')}</th><th /></tr></thead><tbody>{accountHistory.map((entry) => <tr key={entry.id}><td><code>{entry.id}</code></td><td>{scopeLabel(entry.scope)}</td><td>{when(entry.checkedAt)}</td><td><Badge status={reportStatus(entry)} /></td><td><Button aria-label={a('historyOpen', { id: entry.id })} onClick={() => { setScenarios((current) => ({ ...current, [`${account.key}/${entry.scope.purpose}`]: entry.scenario })); go(account.key, 'diagnose', entry.scope.purpose, entry.id); }}>{d('open')}</Button></td></tr>)}</tbody></table></section>}
                {!general && scope.mode === 'execution' && <div className="dg-cleanup-note"><span className="dg-pass">✓</span><strong>{d('cleanup')}</strong>{d(account.target === 'aws' ? 'awsCleanupResult' : 'cleanupResult')}</div>}
              </>}
          </>}
        </>}
      </section>
      {account && setup && <AccountSetup account={account} scope={scope} onClose={() => setSetup(false)} onRun={start} />}
    </AppShell>
  </div>;
}
