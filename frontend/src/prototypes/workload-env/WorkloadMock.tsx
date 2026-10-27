import { useEffect, useRef, useState } from 'react';
import { AppShell } from '../../app/AppShell';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';
import './mock.css';

type Source = { key: string; group: 'application' | 'resource' | 'service'; secret: boolean; suggestedName?: string };
type Row = { id: number; name: string; value: string; secret: boolean; reference?: Source; secretSaved?: boolean };
const sources: Source[] = [
  { key: 'LOG_LEVEL', group: 'application', secret: false, suggestedName: 'LOG_LEVEL' },
  { key: 'APP_ENV', group: 'application', secret: false, suggestedName: 'APP_ENV' },
  { key: 'API_TOKEN', group: 'application', secret: true, suggestedName: 'API_TOKEN' },
  { key: 'DB_PASSWORD', group: 'application', secret: true, suggestedName: 'DB_PASSWORD' },
  ...['db.host', 'db.port', 'db.database', 'cache.host', 'cache.port'].map((key): Source => ({ key, group: 'resource', secret: false })),
  ...['db.password', 'cache.password'].map((key): Source => ({ key, group: 'resource', secret: true })),
  { key: 'frontend / http · 80', group: 'service', secret: false },
  { key: 'gateway / http · 8080', group: 'service', secret: false },
];
const initialRows: Row[] = [
  { id: 1, name: 'LOG_LEVEL', value: 'debug', secret: false },
  { id: 2, name: 'DB_HOST', value: '', secret: false, reference: sources.find((source) => source.key === 'db.host')! },
];
const validName = (name: string) => /^[A-Za-z_][A-Za-z0-9_]*$/.test(name);

export function WorkloadMock() {
  const [rows, setRows] = useState(initialRows);
  const [nextId, setNextId] = useState(3);
  const [focusId, setFocusId] = useState<number>();
  const [selectedRow, setSelectedRow] = useState<number | null>(null);
  const [search, setSearch] = useState('');
  const [dirty, setDirty] = useState(false);
  const [notice, setNotice] = useState('');
  const dialog = useRef<HTMLDialogElement>(null);
  const nameInputs = useRef(new Map<number, HTMLInputElement>());

  useEffect(() => {
    if (selectedRow !== null) dialog.current?.showModal();
    else dialog.current?.close();
  }, [selectedRow]);
  useEffect(() => { if (focusId !== undefined) nameInputs.current.get(focusId)?.focus(); }, [focusId]);

  function update(id: number, patch: Partial<Row>) {
    setRows((current) => current.map((row) => row.id === id ? { ...row, ...patch } : row));
    setDirty(true);
    setNotice('');
  }
  function addRow() {
    setRows([...rows, { id: nextId, name: '', value: '', secret: false }]);
    setFocusId(nextId);
    setNextId(nextId + 1);
    setDirty(true);
    setNotice('');
  }
  function choose(reference?: Source) {
    const row = rows.find((item) => item.id === selectedRow);
    if (!row) return;
    // Selecting a source replaces only this row. A name already entered stays intact.
    update(row.id, {
      name: row.name || reference?.suggestedName || '',
      value: reference || row.reference ? '' : row.value,
      reference,
      secret: reference?.secret ?? (row.reference ? false : row.secret),
      secretSaved: false,
    });
    setSelectedRow(null);
    if (!row.name && !reference?.suggestedName) setFocusId(row.id);
  }
  function problem(row: Row): string {
    if (!row.name) return '';
    if (!validName(row.name)) return t('mock.invalid');
    if (rows.some((other) => other.id !== row.id && other.name === row.name)) return t('mock.duplicate');
    return '';
  }
  const invalid = rows.some((row) => !row.name || !!problem(row));
  const matches = sources.filter((source) => source.key.toLowerCase().includes(search.toLowerCase()));

  return <div onClickCapture={(event) => {
    if ((event.target as HTMLElement).closest('a')) { event.preventDefault(); event.stopPropagation(); }
  }}>
    <AppShell username="demo" role="ADMIN" activeRoute="workload" onSignOut={() => setNotice(t('mock.signout'))}>
      <section className="page workload-editor mock-page">
        <div className="mock-banner">
          <strong>{t('mock.badge')}</strong><span>{t('mock.scope')}</span>
          <Button tone="quiet" onClick={() => {
            setRows(initialRows); setNextId(3); setFocusId(undefined); setSelectedRow(null); setSearch(''); setDirty(false); setNotice('');
          }}>{t('mock.reset')}</Button>
        </div>
        <span className="back-link">{t('mock.back')}</span>
        <header className="page-header">
          <div><h1>{t('mock.title')}</h1><p>acceptance-1 / backend</p></div>
          <span className="status-badge">{t(dirty ? 'mock.pending' : 'mock.clean')}</span>
        </header>
        <div className="tabs mock-environments" role="tablist" aria-label="Environment">
          <button type="button" role="tab" className="tab tab-active" aria-selected="true">staging</button>
          <button type="button" role="tab" className="tab" aria-selected="false" disabled title={t('mock.production')}>production</button>
        </div>
        <section className="content-panel mock-context">
          <div><strong>{t('mock.workload')}</strong><code>backend</code></div>
          <div><strong>{t('mock.container')}</strong><code>main</code></div>
          <div><strong>{t('mock.image')}</strong><code>registry.example/acceptance-backend:v1</code></div>
        </section>
        <section className="content-panel mock-env">
          <div className="section-header"><div><h2>{t('mock.variables')}</h2><p className="muted">{t('mock.localScope')}</p></div></div>
          <div className="mock-table-wrap">
            <table className="mock-table">
              <thead><tr><th scope="col">{t('mock.name')}</th><th scope="col">{t('mock.value')}</th><th scope="col"><span className="mock-sr-only">{t('mock.actions')}</span></th></tr></thead>
              <tbody>{rows.map((row, index) => {
                const error = problem(row);
                const rowLabel = row.name || String(index + 1);
                const openSources = () => { setSearch(''); setSelectedRow(row.id); };
                return <tr key={row.id}>
                  <td>
                    <input ref={(element) => { if (element) nameInputs.current.set(row.id, element); else nameInputs.current.delete(row.id); }}
                      aria-label={`${t('mock.name')} ${index + 1}`} aria-invalid={!!error} aria-describedby={error ? `mock-error-${row.id}` : undefined}
                      value={row.name} placeholder="VARIABLE_NAME" autoComplete="off" spellCheck={false} onChange={(event) => update(row.id, { name: event.target.value })} />
                    {error && <small className="mock-error" id={`mock-error-${row.id}`} role="alert">{error}</small>}
                  </td>
                  <td>
                    <div className="mock-value-line">
                      {row.reference ? <button type="button" className="mock-reference" onClick={openSources} aria-label={`${t('mock.changeSource')} ${rowLabel}`}>
                        <span aria-hidden="true">↗</span><code>{row.reference.key}</code><small>{t(`mock.${row.reference.group}`)}</small>
                      </button> : <>
                        <input aria-label={`${t('mock.value')} ${rowLabel}`} type={row.secret ? 'password' : 'text'} autoComplete="off" spellCheck={false}
                          value={row.value} placeholder={row.secretSaved ? t('mock.secretSaved') : t('mock.enterValue')}
                          onChange={(event) => update(row.id, { value: event.target.value, secretSaved: false })} />
                        <Button tone="quiet" aria-label={`${t('mock.choose')} ${rowLabel}`} onClick={openSources}>{t('mock.choose')}</Button>
                      </>}
                    </div>
                  </td>
                  <td className="mock-row-actions">
                    <label className="mock-secret"><input type="checkbox" checked={row.secret} disabled={!!row.reference}
                      aria-label={`${t('mock.secret')} ${rowLabel}`} onChange={(event) => update(row.id, { secret: event.target.checked, secretSaved: false })} />{t('mock.secret')}</label>
                    <Button tone="quiet" aria-label={`${t('mock.remove')} ${rowLabel}`} onClick={() => {
                      setRows(rows.filter((item) => item.id !== row.id)); setDirty(true); setNotice('');
                    }}>{t('mock.remove')}</Button>
                  </td>
                </tr>;
              })}</tbody>
            </table>
            {!rows.length && <p className="mock-empty">{t('mock.empty')}</p>}
          </div>
          <div className="mock-add"><Button onClick={addRow}>+ {t('mock.add')}</Button></div>
        </section>
        {notice && <p className="mock-notice" role="status">{notice}</p>}
        <footer className="mock-footer">
          <span className="muted">{t('mock.footer')}</span>
          <Button tone="primary" disabled={invalid || !dirty} onClick={() => {
            // This mock discards entered secret bytes after saving; it has no secret backend.
            setRows(rows.map((row) => row.secret && !row.reference ? { ...row, value: '', secretSaved: true } : row));
            setDirty(false); setNotice(t('mock.saved'));
          }}>{t('mock.save')}</Button>
        </footer>
      </section>
      <dialog ref={dialog} className="mock-source-dialog" aria-labelledby="mock-source-title" onClose={() => setSelectedRow(null)}>
        <header><h2 id="mock-source-title">{t('mock.chooseTitle')}</h2><Button tone="quiet" onClick={() => setSelectedRow(null)}>{t('mock.close')}</Button></header>
        <label className="mock-search">{t('mock.search')}<input type="search" value={search} onChange={(event) => setSearch(event.target.value)} /></label>
        <button type="button" className="mock-source-option mock-direct" onClick={() => choose()}>{t('mock.direct')}</button>
        <div className="mock-source-list">{(['application', 'resource', 'service'] as const).map((group) => {
          const options = matches.filter((source) => source.group === group);
          return options.length ? <section key={group}><h3>{t(`mock.${group}`)}</h3>{options.map((source) =>
            <button type="button" className="mock-source-option" key={source.key} onClick={() => choose(source)}>
              <code>{source.key}</code>{source.secret && <small>{t('mock.secret')}</small>}
            </button>)}</section> : null;
        })}{!matches.length && <p className="muted">{t('mock.none')}</p>}</div>
      </dialog>
    </AppShell>
  </div>;
}
