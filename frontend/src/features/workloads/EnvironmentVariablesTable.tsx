import { useEffect, useRef, useState } from 'react';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import type { KeyKind } from '../configuration/api';
import type { ResourceType, Workload } from './api';
import { changeSource, emptyBinding, isKeySource, rowProblem, sourceOf, type Binding, type KeyCatalog, type RowContext } from './bindings';
import type { Dependency } from './DependencyFields';
import { serviceCandidates } from './rowContext';
import { t } from '../../shared/i18n';

export type FocusField = 'name' | 'key' | 'output';
export type FocusRequest = { rowId: string; field: FocusField };
type Props = {
  container: string; bindings: Binding[]; catalog: KeyCatalog; dependencies: Dependency[];
  types: ResourceType[]; workloads: Workload[]; self: string; context: RowContext;
  disabled: boolean; showIncomplete: boolean; focus?: FocusRequest; onFocused(): void;
  onChange(bindings: Binding[]): void; onCreateKey(kind: KeyKind, row: Binding): void;
  onAddResource(row: Binding): void;
};
type Choice = { id: string; label: string; group: 'shared' | 'resources' | 'services'; secret: boolean; binding: Binding };

// Direct values stay in the workload. Only scoped Application keys, declared
// outputs and other Services are candidates: local rows are never source data.
export function EnvironmentVariablesTable({ container, bindings, catalog, dependencies, types, workloads, self, context, disabled, showIncomplete, focus, onFocused, onChange, onCreateKey, onAddResource }: Props) {
  const fields = useRef(new Map<string, HTMLElement>());
  const [added, setAdded] = useState<string>();
  const [choosing, setChoosing] = useState<string>();
  const [query, setQuery] = useState('');
  useEffect(() => {
    if (!added) return;
    const element = fields.current.get(`${added}:name`);
    if (element) { element.focus(); setAdded(undefined); }
  }, [added, bindings]);
  useEffect(() => {
    if (!focus) return;
    const element = fields.current.get(`${focus.rowId}:${focus.field}`);
    if (element) {
      element.focus();
      if (focus.field === 'output') { setQuery(''); setChoosing(focus.rowId); }
      onFocused();
    }
  }, [focus, bindings, onFocused]);
  const row = bindings.find((item) => item.id === choosing);
  const track = (binding: Binding, field: FocusField) => (element: HTMLElement | null) => {
    for (const key of field === 'key' ? ['key', 'output'] : [field]) {
      if (element) fields.current.set(`${binding.id}:${key}`, element); else fields.current.delete(`${binding.id}:${key}`);
    }
  };
  const update = (binding: Binding, patch: Partial<Binding>) => onChange(bindings.map((item) => item.id === binding.id ? { ...item, ...patch } : item));
  const choose = (next: Binding) => {
    onChange(bindings.map((item) => item.id === next.id ? next : item));
    setChoosing(undefined);
  };
  const open = (binding: Binding) => { setQuery(''); setChoosing(binding.id); };
  function secret(binding: Binding) {
    if (isKeySource(binding.source)) return sourceOf(binding, catalog) === 'SECRET';
    return binding.source === 'RESOURCE' && !!types.find((type) => type.key === dependencies.find((item) => item.alias === binding.alias)?.type)?.outputs.find((output) => output.name === binding.output)?.secret;
  }
  function referenceLabel(binding: Binding) {
    if (isKeySource(binding.source)) return t('workloads.envEntry.applicationKey', { key: binding.key });
    if (binding.source === 'RESOURCE') return `${binding.alias}.${binding.output}`;
    return `${binding.workload} / ${binding.port}`;
  }
  const choices: Choice[] = row ? [
    ...(catalog.status === 'ready' ? catalog.keys.map((key): Choice => ({
      id: `key:${key.name}`, label: t('workloads.envEntry.applicationKey', { key: key.name }), group: 'shared', secret: key.kind === 'SECRET',
      binding: { ...changeSource(row, key.kind), key: key.name, name: row.name || key.name },
    })) : []),
    ...dependencies.flatMap((dependency) => (types.find((type) => type.key === dependency.type)?.outputs ?? []).map((output): Choice => ({
      id: `resource:${dependency.alias}:${output.name}`, label: `${dependency.alias}.${output.name}`, group: 'resources', secret: !!output.secret,
      binding: { ...changeSource(row, 'RESOURCE'), alias: dependency.alias, output: output.name, resourceChanged: false },
    }))),
    ...serviceCandidates(workloads, self).flatMap((workload) => (workload.servicePorts ?? []).map((port): Choice => ({
      id: `service:${workload.id}:${port}`, label: `${workload.id} / ${port}`, group: 'services', secret: false,
      binding: { ...changeSource(row, 'SERVICE'), workload: workload.id, port },
    }))),
  ].filter((item) => item.label.toLocaleLowerCase().includes(query.toLocaleLowerCase())) : [];

  function create(kind: KeyKind) {
    if (!row) return;
    setChoosing(undefined);
    onCreateKey(kind, row);
  }
  return <section className="env-variables" aria-label={t('workloads.envTable.region', { container })}>
    <div className="section-header"><h3>{t('workloads.envTable.title')}</h3></div>
    {bindings.length === 0 ? <p className="muted">{t('workloads.envTable.empty')}</p> : <div className="env-table-scroll"><table className="env-table env-entry-table">
      <thead><tr><th scope="col">{t('workloads.envTable.columnName')}</th><th scope="col">{t('workloads.envEntry.value')}</th><th scope="col"><span className="definition-sr-only">{t('workloads.envTable.columnActions')}</span></th></tr></thead>
      <tbody>{bindings.map((binding, index) => {
        const position = index + 1;
        const source = sourceOf(binding, catalog);
        const found = rowProblem(binding, position, container, bindings, context);
        const shown = found && (showIncomplete || !found.incomplete) ? found.message : undefined;
        return <tr key={binding.id} data-row={binding.id} aria-invalid={shown ? true : undefined}>
          <td><input ref={track(binding, 'name')} aria-label={t('workloads.envTable.name', { row: position })} value={binding.name} disabled={disabled} placeholder="API_URL" spellCheck={false} aria-invalid={shown ? true : undefined} aria-describedby={shown ? `error-${binding.id}` : undefined} onChange={(event) => update(binding, { name: event.target.value })} /></td>
          <td><div className="env-entry-value">
            {source === 'LITERAL' ? <><input aria-label={t('workloads.envTable.value', { row: position })} value={binding.value} disabled={disabled} autoComplete="off" spellCheck={false} onChange={(event) => update(binding, { value: event.target.value })} />
              <button ref={track(binding, 'key')} type="button" className="button button-quiet" disabled={disabled} aria-label={t('workloads.envEntry.chooseFor', { row: position })} onClick={() => open(binding)}>{t('workloads.envEntry.choose')}</button></>
              : <button ref={track(binding, 'key')} type="button" className="env-entry-reference" disabled={disabled} aria-label={t('workloads.envEntry.changeFor', { row: position })} onClick={() => open(binding)}><span aria-hidden="true">↗</span><code>{referenceLabel(binding)}</code>{secret(binding) ? <small>{t('workloads.envEntry.secret')}</small> : null}</button>}
          </div>{shown ? <p id={`error-${binding.id}`} className="field-error" role="alert">{shown}</p> : null}</td>
          <td><div className="env-entry-actions"><label><input type="checkbox" aria-label={t('workloads.envEntry.secretFor', { row: position })} checked={!!secret(binding)} disabled={disabled || source !== 'LITERAL' || catalog.status !== 'ready'} onChange={() => onCreateKey('SECRET', binding)} />{t('workloads.envEntry.secret')}</label><Button tone="quiet" disabled={disabled} aria-label={t('workloads.envTable.remove', { row: position, name: binding.name || position })} onClick={() => onChange(bindings.filter((item) => item.id !== binding.id))}>{t('workloads.envTable.removeShort')}</Button></div></td>
        </tr>;
      })}</tbody>
    </table></div>}
    <div><Button disabled={disabled} onClick={() => { const binding = emptyBinding(); onChange([...bindings, binding]); setAdded(binding.id); }}>{t('workloads.envTable.add')}</Button></div>
    {bindings.some((binding) => binding.source === 'LITERAL') ? <details className="env-entry-help"><summary>{t('workloads.envEntry.help')}</summary><small>{t('workloads.envTable.literalNote')} {t('workloads.envEntry.secretHelp')}</small></details> : null}
    {row && !disabled ? <Dialog label={t('workloads.envEntry.pickerTitle')} onClose={() => setChoosing(undefined)}>
      <h2>{t('workloads.envEntry.pickerTitle')}</h2>
      <label className="field">{t('workloads.envEntry.search')}<input type="search" value={query} onChange={(event) => setQuery(event.target.value)} /></label>
      <Button tone="quiet" onClick={() => choose(changeSource(row, 'LITERAL'))}>{t('workloads.envEntry.direct')}</Button>
      <div className="env-entry-options">{(['shared', 'resources', 'services'] as const).map((group) => <section key={group} aria-label={t(`workloads.envEntry.group.${group}`)}>
        <h3>{t(`workloads.envEntry.group.${group}`)}</h3>
        {group === 'shared' && catalog.status !== 'ready' ? <p role="status">{catalog.status === 'loading' ? t('workloads.envTable.keysLoading') : t('workloads.envTable.keysError')}</p> : null}
        {choices.filter((choice) => choice.group === group).map((choice) => <button type="button" className="env-entry-option" key={choice.id} onClick={() => choose(choice.binding)}><code>{choice.label}</code>{choice.secret ? <small>{t('workloads.envEntry.secret')}</small> : null}</button>)}
      </section>)}{choices.length === 0 && catalog.status === 'ready' ? <p className="muted">{t('workloads.envEntry.noMatches')}</p> : null}</div>
      <div className="env-entry-create"><Button tone="quiet" disabled={catalog.status !== 'ready'} onClick={() => create('VARIABLE')}>{t('workloads.envTable.createVariable')}</Button><Button tone="quiet" disabled={catalog.status !== 'ready'} onClick={() => create('SECRET')}>{t('workloads.envTable.createSecret')}</Button><Button tone="quiet" onClick={() => { setChoosing(undefined); onAddResource(row); }}>{t('workloads.envTable.addResource')}</Button></div>
      <div className="form-actions"><Button onClick={() => setChoosing(undefined)}>{t('workloads.envEntry.close')}</Button></div>
    </Dialog> : null}
  </section>;
}
