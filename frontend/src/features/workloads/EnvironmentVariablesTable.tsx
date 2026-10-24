import { useEffect, useRef, useState } from 'react';
import { Button } from '../../shared/ui/Button';
import type { KeyKind } from '../configuration/api';
import type { ResourceType, Workload } from './api';
import { changeSource, emptyBinding, rowProblem, sourceOf, type Binding, type KeyCatalog, type RowContext, type Source } from './bindings';
import type { Dependency } from './DependencyFields';
import { serviceCandidates } from './rowContext';
import { t } from '../../shared/i18n';

export type FocusField = 'name' | 'key' | 'output';
export type FocusRequest = { rowId: string; field: FocusField };

type Props = {
  container: string;
  bindings: Binding[];
  catalog: KeyCatalog;
  dependencies: Dependency[];
  types: ResourceType[];
  workloads: Workload[];
  self: string;
  context: RowContext;
  disabled: boolean;
  // showIncomplete: Save was tried, so rows that are not filled in report it too.
  showIncomplete: boolean;
  focus?: FocusRequest;
  onFocused(): void;
  onChange(bindings: Binding[]): void;
  onCreateKey(kind: KeyKind, row: Binding): void;
  onAddResource(row: Binding): void;
};

const SOURCES: Exclude<Source, 'KEY'>[] = ['LITERAL', 'VARIABLE', 'SECRET', 'RESOURCE', 'SERVICE'];
const sourceLabel = (source: Exclude<Source, 'KEY'>): string => t(`workloads.envTable.source.${source.toLowerCase()}`);

// One table per container: Tên biến → Lấy giá trị từ → Giá trị / Nguồn (UC-16 BR-15).
// It only edits form state. Choosing, changing or removing a row never creates, changes
// or deletes an Application key or a dependency; those have their own explicit actions.
export function EnvironmentVariablesTable({ container, bindings, catalog, dependencies, types, workloads, self, context, disabled, showIncomplete, focus, onFocused, onChange, onCreateKey, onAddResource }: Props) {
  const fields = useRef(new Map<string, HTMLElement>());
  // A row added here takes focus in its name field.
  const [added, setAdded] = useState<string>();
  useEffect(() => {
    if (!added) return;
    const element = fields.current.get(`${added}:name`);
    if (element) { element.focus(); setAdded(undefined); }
  }, [added, bindings]);
  useEffect(() => {
    if (!focus) return;
    const element = fields.current.get(`${focus.rowId}:${focus.field}`);
    if (element) { element.focus(); onFocused(); }
  }, [focus, bindings, onFocused]);

  const track = (row: Binding, field: FocusField) => (element: HTMLElement | null) => { if (element) fields.current.set(`${row.id}:${field}`, element); else fields.current.delete(`${row.id}:${field}`); };
  const update = (row: Binding, patch: Partial<Binding>) => onChange(bindings.map((item) => (item.id === row.id ? { ...item, ...patch } : item)));
  const hasLiteral = bindings.some((row) => sourceOf(row, catalog) === 'LITERAL');
  const candidates = serviceCandidates(workloads, self);

  function keyCells(row: Binding, position: number, source: 'VARIABLE' | 'SECRET') {
    const kind: KeyKind = source;
    const label = t(source === 'SECRET' ? 'workloads.envTable.secretKey' : 'workloads.envTable.variableKey', { row: position });
    const keys = catalog.status === 'ready' ? catalog.keys.filter((key) => key.kind === kind) : [];
    const known = keys.some((key) => key.name === row.key);
    const chosen = catalog.status === 'ready' ? catalog.keys.find((key) => key.name === row.key) : undefined;
    // Choosing a key suggests the variable name only while it is still empty.
    const choose = (key: string) => update(row, { key, source, ...(row.name === '' ? { name: key } : {}) });
    return <>
      <select ref={track(row, 'key')} aria-label={label} value={row.key} disabled={disabled || catalog.status !== 'ready'} onChange={(event) => choose(event.target.value)}>
        <option value="">{catalog.status === 'loading' ? t('workloads.envTable.keysLoading') : catalog.status === 'error' ? t('workloads.envTable.keysError') : t('workloads.envTable.chooseKey')}</option>
        {row.key && !known ? <option value={row.key}>{t('workloads.envTable.keyUnavailable', { key: row.key })}</option> : null}
        {keys.map((key) => <option key={key.name} value={key.name}>{key.name}</option>)}
      </select>
      {source === 'SECRET' && chosen ? <small className="muted">{chosen.configured ? t('workloads.envTable.secretConfigured') : t('workloads.envTable.secretNotSet')}</small> : null}
      <Button tone="quiet" disabled={disabled || catalog.status !== 'ready'} onClick={() => onCreateKey(kind, row)}>{source === 'SECRET' ? t('workloads.envTable.createSecret') : t('workloads.envTable.createVariable')}</Button>
    </>;
  }

  function valueCells(row: Binding, position: number) {
    const source = sourceOf(row, catalog);
    if (source === 'LITERAL') return <input aria-label={t('workloads.envTable.value', { row: position })} value={row.value} disabled={disabled} autoComplete="off" spellCheck={false} onChange={(event) => update(row, { value: event.target.value })} />;
    if (source === 'VARIABLE' || source === 'SECRET') return keyCells(row, position, source);
    if (source === 'RESOURCE') {
      const dependency = dependencies.find((item) => item.alias === row.alias);
      const outputs = types.find((type) => type.key === dependency?.type)?.outputs ?? [];
      const known = !row.output || outputs.some((output) => output.name === row.output);
      return <>
        <select aria-label={t('workloads.envTable.resource', { row: position })} value={row.alias} disabled={disabled} onChange={(event) => update(row, { alias: event.target.value, output: '' })}>
          <option value="">{t('workloads.envTable.chooseResource')}</option>
          {row.alias && !dependency ? <option value={row.alias}>{t('workloads.envTable.aliasUnavailable', { alias: row.alias })}</option> : null}
          {dependencies.filter((item) => item.alias).map((item) => <option key={item.alias} value={item.alias}>{item.alias}</option>)}
        </select>
        <select ref={track(row, 'output')} aria-label={t('workloads.envTable.output', { row: position })} value={row.output} disabled={disabled} onChange={(event) => update(row, { output: event.target.value })}>
          <option value="">{t('workloads.envTable.chooseOutput')}</option>
          {!known ? <option value={row.output}>{t('workloads.envTable.outputUnavailable', { output: row.output })}</option> : null}
          {outputs.map((output) => <option key={output.name} value={output.name}>{output.secret ? t('workloads.envTable.outputSecret', { output: output.name }) : output.name}</option>)}
        </select>
        <Button tone="quiet" disabled={disabled} onClick={() => onAddResource(row)}>{t('workloads.envTable.addResource')}</Button>
      </>;
    }
    const target = candidates.find((item) => item.id === row.workload);
    return <>
      <select aria-label={t('workloads.envTable.workload', { row: position })} value={row.workload} disabled={disabled} onChange={(event) => update(row, { workload: event.target.value, port: '', alias: '' })}>
        <option value="">{t('workloads.envTable.chooseWorkload')}</option>
        {row.workload && !target ? <option value={row.workload}>{t('workloads.envTable.workloadUnavailable', { workload: row.workload })}</option> : null}
        {candidates.map((item) => <option key={item.id} value={item.id}>{item.id}</option>)}
      </select>
      <select aria-label={t('workloads.envTable.port', { row: position })} value={row.port} disabled={disabled} onChange={(event) => update(row, { port: event.target.value, alias: '' })}>
        <option value="">{t('workloads.envTable.choosePort')}</option>
        {row.port && !target?.servicePorts?.includes(row.port) ? <option value={row.port}>{t('workloads.envTable.portUnavailable', { port: row.port })}</option> : null}
        {target?.servicePorts?.map((port) => <option key={port} value={port}>{port}</option>)}
      </select>
    </>;
  }

  return <section className="env-variables" aria-label={t('workloads.envTable.region', { container })}>
    <div className="section-header">
      <h3>{t('workloads.envTable.title')}</h3>
      <Button disabled={disabled} onClick={() => { const row = emptyBinding(); onChange([...bindings, row]); setAdded(row.id); }}>{t('workloads.envTable.add')}</Button>
    </div>
    {bindings.length === 0 ? <p className="muted">{t('workloads.envTable.empty')}</p> : <table className="env-table">
      <thead><tr><th scope="col">{t('workloads.envTable.columnName')}</th><th scope="col">{t('workloads.envTable.columnSource')}</th><th scope="col">{t('workloads.envTable.columnValue')}</th><th scope="col"><span className="definition-sr-only">{t('workloads.envTable.columnActions')}</span></th></tr></thead>
      <tbody>{bindings.map((row, index) => {
        const position = index + 1;
        const source = sourceOf(row, catalog);
        const found = rowProblem(row, position, container, bindings, context);
        const shown = found && (showIncomplete || !found.incomplete) ? found.message : undefined;
        return <tr key={row.id} data-row={row.id} aria-invalid={shown ? true : undefined}>
          <td><input ref={track(row, 'name')} aria-label={t('workloads.envTable.name', { row: position })} value={row.name} disabled={disabled} placeholder="API_URL" spellCheck={false} aria-invalid={shown ? true : undefined} onChange={(event) => update(row, { name: event.target.value })} /></td>
          <td><select aria-label={t('workloads.envTable.sourceOf', { row: position })} value={source} disabled={disabled} onChange={(event) => onChange(bindings.map((item) => (item.id === row.id ? changeSource(item, event.target.value as Source) : item)))}>{SOURCES.map((item) => <option key={item} value={item}>{sourceLabel(item)}</option>)}</select></td>
          <td><div className="env-value">{valueCells(row, position)}</div>{shown ? <p className="field-error" role="alert">{shown}</p> : null}</td>
          <td><Button tone="quiet" disabled={disabled} aria-label={t('workloads.envTable.remove', { row: position, name: row.name || position })} onClick={() => onChange(bindings.filter((item) => item.id !== row.id))}>{t('workloads.envTable.removeShort')}</Button></td>
        </tr>;
      })}</tbody>
    </table>}
    {hasLiteral ? <small className="muted" role="note">{t('workloads.envTable.literalNote')}</small> : null}
  </section>;
}
