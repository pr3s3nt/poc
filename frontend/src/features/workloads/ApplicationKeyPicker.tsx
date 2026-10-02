import type { KeyKind } from '../configuration/api';
import { Button } from '../../shared/ui/Button';
import { selectKey, unselectKey, type Binding, type KeyCatalog } from './bindings';

type Props = { container: string; bindings: Binding[]; catalog: KeyCatalog; disabled: boolean; onChange: (bindings: Binding[]) => void };
type Mapping = { binding: Binding; index: number };

// Per-container checklist of existing Application variables and secrets. Only
// key names and container names are shown; catalog values are never rendered.
export function ApplicationKeyPicker({ container, bindings, catalog, disabled, onChange }: Props) {
  const mappings: Mapping[] = bindings.flatMap((binding, index) => binding.source === 'KEY' ? [{ binding, index }] : []);
  const update = (index: number, next: Binding) => onChange(bindings.map((item, i) => i === index ? next : item));
  const remove = (index: number) => onChange(bindings.filter((_, i) => i !== index));

  if (catalog.status !== 'ready') return <div className="key-picker">
    <p>{catalog.status === 'loading' ? 'Loading Application variables and secrets…' : 'Application variables and secrets could not be loaded. Use Retry above; selected keys are kept.'}</p>
    {mappings.length ? <ul aria-label={`Selected Application keys for ${container}`}>{mappings.map(({ binding, index }) => <li key={index}><code>{binding.key}</code> as <code>{binding.name}</code></li>)}</ul> : null}
  </div>;

  const known = new Set(catalog.keys.map((key) => key.name));
  const unavailable = mappings.filter(({ binding }) => !known.has(binding.key));

  const group = (kind: KeyKind, title: string) => {
    const keys = catalog.keys.filter((key) => key.kind === kind);
    return <fieldset className="key-group" aria-label={`${title} for ${container}`}>
      <legend>{title}</legend>
      {keys.length ? <ul>{keys.map((key) => {
        const selected = mappings.filter(({ binding }) => binding.key === key.name);
        return <li key={key.name}>
          <label className="key-option"><input type="checkbox" checked={selected.length > 0} disabled={disabled} onChange={(event) => onChange(event.target.checked ? selectKey(bindings, key.name) : unselectKey(bindings, key.name))} /><code>{key.name}</code></label>
          {key.configured ? null : <small className="muted"> · no value set in this environment</small>}
          {selected.map(({ binding, index }, n) => {
            const suffix = selected.length > 1 ? ` (${n + 1})` : '';
            return <div className="key-mapping" key={index}>
              {binding.custom ? <label>Container name<input aria-label={`Container name for ${key.name}${suffix}`} value={binding.name} disabled={disabled} onChange={(event) => update(index, { ...binding, name: event.target.value })} /></label> : <span>Container name <code>{binding.name}</code></span>}
              <label className="key-option"><input type="checkbox" aria-label={`Use a different container name for ${key.name}${suffix}`} checked={binding.custom} disabled={disabled} onChange={(event) => update(index, { ...binding, custom: event.target.checked, name: event.target.checked ? binding.name : key.name })} />Use a different container name</label>
              {selected.length > 1 ? <Button tone="quiet" disabled={disabled} aria-label={`Remove container name ${binding.name || '(empty)'} for ${key.name}`} onClick={() => remove(index)}>Remove name</Button> : null}
            </div>;
          })}
        </li>;
      })}</ul> : <p>No {title.toLowerCase()} in this environment yet. Add them in Variables &amp; Secrets settings.</p>}
    </fieldset>;
  };

  return <div className="key-picker">
    {group('VARIABLE', 'Application variables')}
    {group('SECRET', 'Application secrets')}
    {unavailable.length ? <div className="form-error" role="group" aria-label={`Unavailable Application keys for ${container}`}>
      <p>These keys no longer exist in this environment. Remove them or select an existing key before saving.</p>
      <ul>{unavailable.map(({ binding, index }) => <li key={index}><code>{binding.key}</code> as container name <code>{binding.name}</code> <Button tone="quiet" disabled={disabled} aria-label={`Remove unavailable ${binding.key} as ${binding.name}`} onClick={() => remove(index)}>Remove</Button></li>)}</ul>
    </div> : null}
  </div>;
}
