import { useState, type FormEvent } from 'react';
import { api } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { useCatalogList } from './useCatalogList';

type FieldType = 'string' | 'number' | 'bool' | 'any';
type InputField = { name: string; type: FieldType; required: boolean };
type OutputField = InputField & { secret: boolean };
type ResourceType = { key: string; inputs: InputField[]; outputs: OutputField[] };

const newInput = (): InputField => ({ name: '', type: 'string', required: false });
const newOutput = (): OutputField => ({ name: '', type: 'string', required: false, secret: false });

const loadTypes = async () => (await api<{ resourceTypes: ResourceType[] }>('/resource-types')).resourceTypes ?? [];

export function ResourceTypesPage() {
  const { items: types, loading, loadError, reload } = useCatalogList(loadTypes);
  const [key, setKey] = useState('');
  const [inputs, setInputs] = useState<InputField[]>([]);
  const [outputs, setOutputs] = useState<OutputField[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    setError(''); setNotice(''); setSaving(true);
    // The backend owns the ID rule (UC-02 BR-05); send the ID exactly as typed.
    const submitted = key;
    try {
      await api('/resource-types', { method: 'POST', body: JSON.stringify({ key: submitted, inputs: inputs.map((field) => ({ ...field, name: field.name.trim() })), outputs: outputs.map((field) => ({ ...field, name: field.name.trim() })) }) });
    } catch (reason) { setError((reason as Error).message); setSaving(false); return; }
    // Committed: report it and reset the form before reloading the list.
    setNotice(`Registered resource type ${submitted}.`); setKey(''); setInputs([]); setOutputs([]); setSaving(false);
    await reload();
  }
  function renderFields<T extends InputField>(label: string, fields: T[], update: (fields: T[]) => void, create: () => T, secret: boolean) {
    return <section className="catalog-fields"><div className="section-header"><h3>{label}</h3><Button type="button" onClick={() => update([...fields, create()])}>+ Add {label.toLowerCase().slice(0, -1)}</Button></div>
      {fields.length === 0 ? <p className="muted">No {label.toLowerCase()} declared.</p> : null}
      {fields.map((field, index) => <div className="catalog-field-row" key={index}>
        <label>Name<input aria-label={`${label} ${index + 1} name`} value={field.name} required onChange={(event) => update(fields.map((item, position) => position === index ? { ...item, name: event.target.value } : item))} /></label>
        <label>Type<select aria-label={`${label} ${index + 1} type`} value={field.type} onChange={(event) => update(fields.map((item, position) => position === index ? { ...item, type: event.target.value as FieldType } : item))}><option>string</option><option>number</option><option>bool</option><option>any</option></select></label>
        <label className="catalog-check"><input type="checkbox" checked={field.required} onChange={(event) => update(fields.map((item, position) => position === index ? { ...item, required: event.target.checked } : item))} /> Required</label>
        {secret ? <label className="catalog-check"><input type="checkbox" checked={'secret' in field && field.secret === true} onChange={(event) => update(fields.map((item, position) => position === index ? { ...item, secret: event.target.checked } : item))} /> Secret</label> : null}
        <Button type="button" tone="quiet" aria-label={`Remove ${label.toLowerCase()} ${index + 1}`} onClick={() => update(fields.filter((_, position) => position !== index))}>Remove</Button>
      </div>)}
    </section>;
  }
  return <section className="page"><header className="page-header"><div><p className="eyebrow">Platform</p><h1>Resource types</h1><p>Define reusable resource contracts for this organization.</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>Registered types</h2></div>{loadError ? <div className="form-error" role="alert" aria-label="List error">{loadError} <Button type="button" onClick={() => void reload()}>Retry</Button></div> : loading ? <p>Loading resource types…</p> : types.length === 0 ? <p>No resource types registered yet.</p> : <div className="catalog-list">{types.map((type) => <div key={type.key} className="catalog-entry"><strong>{type.key}</strong><span>{type.inputs?.length ?? 0} inputs · {type.outputs?.length ?? 0} outputs</span></div>)}</div>}</section>
    <section className="content-panel"><div className="section-header"><div><h2>Register resource type</h2><p>Contracts are independent of the infrastructure provider.</p></div></div>
      <form onSubmit={submit}><fieldset className="editor-grid form-fieldset" disabled={saving} aria-busy={saving}><label>Resource type ID<input value={key} required aria-describedby="resource-type-id-hint" onChange={(event) => setKey(event.target.value)} /></label>
        <p id="resource-type-id-hint" className="feature-note">Use lowercase letters, digits and hyphens only, for example <code>postgres-ha</code>. The ID is not changed for you. <code>environment</code> and <code>service</code> are reserved.</p>
        {renderFields('Inputs', inputs, setInputs, newInput, false)}{renderFields('Outputs', outputs, setOutputs, newOutput, true)}
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saving}>{saving ? 'Registering…' : 'Register resource type'}</Button></div>
      </fieldset></form>
    </section>
  </section>;
}
