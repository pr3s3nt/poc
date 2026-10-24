import type { KeyKind } from '../configuration/api';
import { Button } from '../../shared/ui/Button';
import { selectKey, unselectKey, type Binding, type KeyCatalog } from './bindings';
import { t } from '../../shared/i18n';

type Props = { container: string; bindings: Binding[]; catalog: KeyCatalog; disabled: boolean; onChange: (bindings: Binding[]) => void };
type Mapping = { binding: Binding; index: number };

// Per-container checklist of existing Application variables and secrets. Only
// key names and container names are shown; catalog values are never rendered.
export function ApplicationKeyPicker({ container, bindings, catalog, disabled, onChange }: Props) {
  const mappings: Mapping[] = bindings.flatMap((binding, index) => binding.source === 'KEY' ? [{ binding, index }] : []);
  const update = (index: number, next: Binding) => onChange(bindings.map((item, i) => i === index ? next : item));
  const remove = (index: number) => onChange(bindings.filter((_, i) => i !== index));

  if (catalog.status !== 'ready') return <div className="key-picker">
    <p>{catalog.status === 'loading' ? t('workloads.applicationKeyPicker.loadingApplicationVariablesAndSecrets') : t('workloads.applicationKeyPicker.couldNotLoadApplicationVariables')}</p>
    {mappings.length ? <ul aria-label={t('workloads.applicationKeyPicker.applicationKeysSelectedForContainer', { container })}>{mappings.map(({ binding, index }) => <li key={index}><code>{binding.key}</code> {' '}{t('workloads.applicationKeyPicker.as')}{' '} <code>{binding.name}</code></li>)}</ul> : null}
  </div>;

  const known = new Set(catalog.keys.map((key) => key.name));
  const unavailable = mappings.filter(({ binding }) => !known.has(binding.key));

  const group = (kind: KeyKind, title: string, empty: string) => {
    const keys = catalog.keys.filter((key) => key.kind === kind);
    return <fieldset className="key-group" aria-label={`${title} cho ${container}`}>
      <legend>{title}</legend>
      {keys.length ? <ul>{keys.map((key) => {
        const selected = mappings.filter(({ binding }) => binding.key === key.name);
        return <li key={key.name}>
          <label className="key-option"><input type="checkbox" checked={selected.length > 0} disabled={disabled} onChange={(event) => onChange(event.target.checked ? selectKey(bindings, key.name) : unselectKey(bindings, key.name))} /><code>{key.name}</code></label>
          {key.configured ? null : <small className="muted">{' '}{t('workloads.applicationKeyPicker.noValueSetInThis')}</small>}
          {selected.map(({ binding, index }, n) => {
            const suffix = selected.length > 1 ? ` (${n + 1})` : '';
            return <div className="key-mapping" key={index}>
              {binding.custom ? <label>{t('workloads.applicationKeyPicker.nameInContainer')}<input aria-label={t('workloads.applicationKeyPicker.nameInContainerForName', { name: key.name, suffix })} value={binding.name} disabled={disabled} onChange={(event) => update(index, { ...binding, name: event.target.value })} /></label> : <span>{t('workloads.applicationKeyPicker.nameInContainer')}{' '} <code>{binding.name}</code></span>}
              <label className="key-option"><input type="checkbox" aria-label={t('workloads.applicationKeyPicker.useADifferentContainerName', { name: key.name, suffix })} checked={binding.custom} disabled={disabled} onChange={(event) => update(index, { ...binding, custom: event.target.checked, name: event.target.checked ? binding.name : key.name })} />{t('workloads.applicationKeyPicker.useADifferentContainerName2')}</label>
              {selected.length > 1 ? <Button tone="quiet" disabled={disabled} aria-label={t('workloads.applicationKeyPicker.removeContainerNameValueFor', { value: binding.name || t('workloads.applicationKeyPicker.empty'), name: key.name })} onClick={() => remove(index)}>{t('workloads.applicationKeyPicker.removeName')}</Button> : null}
            </div>;
          })}
        </li>;
      })}</ul> : <p>{empty}</p>}
    </fieldset>;
  };

  return <div className="key-picker">
    {group('VARIABLE', t('workloads.applicationKeyPicker.applicationVariables'), t('workloads.applicationKeyPicker.thisEnvironmentHasNoApplication'))}
    {group('SECRET', t('workloads.applicationKeyPicker.applicationSecrets'), t('workloads.applicationKeyPicker.thisEnvironmentHasNoApplication2'))}
    {unavailable.length ? <div className="form-error" role="group" aria-label={t('workloads.applicationKeyPicker.applicationKeysNoLongerExist', { container })}>
      <p>{t('workloads.applicationKeyPicker.theseKeysNoLongerExist')}</p>
      <ul>{unavailable.map(({ binding, index }) => <li key={index}><code>{binding.key}</code> {' '}{t('workloads.applicationKeyPicker.withContainerName')}{' '} <code>{binding.name}</code> <Button tone="quiet" disabled={disabled} aria-label={t('workloads.applicationKeyPicker.removeMissingKeyKeyWith', { key: binding.key, name: binding.name })} onClick={() => remove(index)}>{t('workloads.applicationKeyPicker.remove')}</Button></li>)}</ul>
    </div> : null}
  </div>;
}
