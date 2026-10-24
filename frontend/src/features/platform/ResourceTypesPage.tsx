import { useRef, useState, type FormEvent } from 'react';
import { specificGuidance, fieldNote } from '../../shared/api/errors';
import { href, navigate } from '../../app/routes';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { capabilitiesForType, isVerifiedPin, loadCapabilities, type Capability } from './capabilities';
import { useCatalogList } from './useCatalogList';
import { t } from '../../shared/i18n';

type InputField = { name: string; type: string; required: boolean };
type OutputField = InputField & { secret: boolean };
type ResourceType = { key: string; inputs?: Partial<InputField>[] | null; outputs?: Partial<OutputField>[] | null; classes?: string[] | null };

const fieldTypes = ['string', 'number', 'bool', 'any'];
const newInput = (): InputField => ({ name: '', type: 'string', required: false });
const newOutput = (): OutputField => ({ name: '', type: 'string', required: false, secret: false });
const yesNo = (value: boolean | undefined) => (value === true ? t('platform.resourceTypes.yes') : t('platform.resourceTypes.no'));
const driverText = (pair: Capability) => `Terraform/${pair.module}`;

// Flags and types are copied exactly as the GET returned them; nothing is
// inferred from field names. A missing flag is false (the API omits false).
const copyInputs = (fields: Partial<InputField>[] | null | undefined): InputField[] => (fields ?? []).map((field) => ({ name: field.name ?? '', type: field.type ?? 'string', required: field.required === true }));
const copyOutputs = (fields: Partial<OutputField>[] | null | undefined): OutputField[] => (fields ?? []).map((field) => ({ name: field.name ?? '', type: field.type ?? 'string', required: field.required === true, secret: field.secret === true }));

// Specific guidance comes from the stable error code; raw backend text is never shown.
function describeFailure(reason: unknown): string {
  if (!(reason instanceof ApiError)) return t('platform.resourceTypes.noResponseFromTheServer');
  const field = fieldNote(reason.field);
  const specific = specificGuidance(reason.code, reason.message, reason.field);
  if (specific) return t('platform.resourceTypes.theServerRejectedTheResource', { specific });
  switch (reason.status) {
    case 400: return t('platform.resourceTypes.theServerRejectedThisResource', { field });
    case 401: return t('platform.resourceTypes.yourSessionHasExpiredPlease');
    case 403: return t('platform.resourceTypes.youAreNotAllowedTo');
    case 409: return t('platform.resourceTypes.theResourceTypeIDAlready');
    case 413: return t('platform.resourceTypes.theResourceTypeContentIs');
    default: return t('platform.resourceTypes.theServerFailedWhileRegistering');
  }
}
const listFailure = () => t('platform.resourceTypes.couldNotLoadTheResource');

const loadTypes = async () => (await api<{ resourceTypes: ResourceType[] }>('/resource-types')).resourceTypes ?? [];

function SchemaTable({ label, fields, secret }: { label: string; fields: Partial<OutputField>[]; secret: boolean }) {
  return <table className="catalog-schema" aria-label={label}><thead><tr><th scope="col">{t('platform.resourceTypes.name')}</th><th scope="col">{t('platform.resourceTypes.type')}</th><th scope="col">{t('platform.resourceTypes.required')}</th>{secret ? <th scope="col">{t('platform.resourceTypes.secret')}</th> : null}</tr></thead>
    <tbody>{fields.map((field, index) => <tr key={index}><td><code>{field.name}</code></td><td><code>{field.type}</code></td><td>{yesNo(field.required)}</td>{secret ? <td>{yesNo(field.secret)}</td> : null}</tr>)}</tbody></table>;
}

export function ResourceTypesPage() {
  const { items: types, loading, loadError, reload } = useCatalogList(loadTypes, listFailure);
  const capabilityList = useCatalogList(loadCapabilities, () => t('platform.resourceTypes.couldNotLoadTheSupported'));
  const [key, setKey] = useState('');
  const [inputs, setInputs] = useState<InputField[]>([]);
  const [outputs, setOutputs] = useState<OutputField[]>([]);
  const [copiedFrom, setCopiedFrom] = useState('');
  const [expanded, setExpanded] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const idField = useRef<HTMLInputElement>(null);

  // Copy never sends a request: it only fills the independent draft; the ID stays blank.
  function copySchema(source: ResourceType) {
    if (saving) return;
    setKey(''); setInputs(copyInputs(source.inputs)); setOutputs(copyOutputs(source.outputs));
    setCopiedFrom(source.key); setError(''); setNotice('');
    idField.current?.focus();
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    setError(''); setNotice(''); setSaving(true);
    // The backend owns the ID rule (UC-02 BR-05); send the ID exactly as typed.
    const submitted = key;
    try {
      await api('/resource-types', { method: 'POST', body: JSON.stringify({ key: submitted, inputs: inputs.map((field) => ({ ...field, name: field.name.trim() })), outputs: outputs.map((field) => ({ ...field, name: field.name.trim() })) }) });
    } catch (reason) { setError(describeFailure(reason)); setSaving(false); return; }
    // Committed: report it and reset the form before reloading the list.
    setNotice(t('platform.resourceTypes.registeredResourceTypeSubmitted', { submitted })); setKey(''); setInputs([]); setOutputs([]); setCopiedFrom(''); setSaving(false);
    await reload();
  }
  function typeOptions(value: string) {
    return [...fieldTypes, ...(fieldTypes.includes(value) ? [] : [value])].map((option) => <option key={option}>{option}</option>);
  }
  function renderFields<T extends InputField>(label: string, noun: string, fields: T[], update: (fields: T[]) => void, create: () => T, secret: boolean) {
    const patch = (index: number, change: Partial<OutputField>) => update(fields.map((item, position) => position === index ? { ...item, ...change } : item));
    return <section className="catalog-fields"><div className="section-header"><h3>{label}</h3><Button type="button" onClick={() => update([...fields, create()])}>{t('platform.resourceTypes.addNoun', { noun })}</Button></div>
      {fields.length === 0 ? <p className="muted">{t('platform.resourceTypes.noNounDeclared', { noun })}</p> : null}
      {fields.map((field, index) => <div className="catalog-field-row" key={index}>
        <label>{t('platform.resourceTypes.name')}<input aria-label={t('platform.resourceTypes.labelValueName', { label, value: index + 1 })} value={field.name} required onChange={(event) => patch(index, { name: event.target.value })} /></label>
        <label>{t('platform.resourceTypes.type')}<select aria-label={t('platform.resourceTypes.labelValueType', { label, value: index + 1 })} value={field.type} onChange={(event) => patch(index, { type: event.target.value })}>{typeOptions(field.type)}</select></label>
        <label className="catalog-check"><input type="checkbox" aria-label={t('platform.resourceTypes.labelValueRequired', { label, value: index + 1 })} checked={field.required} onChange={(event) => patch(index, { required: event.target.checked })} /> {' '}{t('platform.resourceTypes.required')}</label>
        {secret ? <label className="catalog-check"><input type="checkbox" aria-label={t('platform.resourceTypes.labelValueSecret', { label, value: index + 1 })} checked={'secret' in field && field.secret === true} onChange={(event) => patch(index, { secret: event.target.checked })} /> {' '}{t('platform.resourceTypes.secret')}</label> : null}
        <Button type="button" tone="quiet" aria-label={t('platform.resourceTypes.removeToLowerCaseValue', { toLowerCase: label.toLowerCase(), value: index + 1 })} onClick={() => update(fields.filter((_, position) => position !== index))}>{t('platform.resourceTypes.remove')}</Button>
      </div>)}
    </section>;
  }
  function renderEntry(type: ResourceType) {
    // Support comes only from the authoritative metadata; loading/error never reads as "contract only" or "supported".
    const metadataPending = capabilityList.loading || capabilityList.loadError !== '';
    // Only registry-verified module pins count as a way to create the Type.
    const pairs = metadataPending ? [] : capabilitiesForType(capabilityList.items, type.key).filter(isVerifiedPin);
    const rule = pairs.length > 0;
    const supportLabel = capabilityList.loading ? t('platform.resourceTypes.checkingSupport') : capabilityList.loadError ? t('platform.resourceTypes.supportNotConfirmed') : rule ? t('platform.resourceTypes.provisioningSupported') : t('platform.resourceTypes.contractOnly');
    const open = expanded === type.key;
    const inputList = type.inputs ?? [];
    const outputList = type.outputs ?? [];
    return <div key={type.key} className="catalog-entry catalog-entry-rich"><div className="section-header"><div><strong>{type.key}</strong> <span className="status-pill">{supportLabel}</span><br />
      <span>{t('platform.resourceTypes.inputListCountInputsOutputListCountOutputs', { inputListCount: inputList.length, outputListCount: outputList.length })}</span>{rule ? <span>{' '}{t('platform.resourceTypes.provisioningValue', { value: pairs.map(driverText).join(t('platform.resourceTypes.or')) })}</span> : null}{type.classes && type.classes.length > 0 ? <span>{' '}{t('platform.resourceTypes.publishedClassesValue', { value: type.classes.join(', ') })}</span> : null}</div>
      <div className="form-actions">
        <Button type="button" aria-expanded={open} aria-label={`${open ? t('platform.resourceTypes.hide') : t('platform.resourceTypes.view')} schema ${type.key}`} onClick={() => setExpanded(open ? '' : type.key)}>{open ? t('platform.resourceTypes.hideSchema') : 'Xem schema'}</Button>
        {rule ? <a className="button" href={href({ name: 'resource-definitions', resourceType: type.key })} aria-label={t('platform.resourceTypes.createAResourceDefinitionFor', { key: type.key })} onClick={(event) => { event.preventDefault(); navigate({ name: 'resource-definitions', resourceType: type.key }); }}>{t('platform.resourceTypes.createResourceDefinition')}</a> : null}
      </div></div>
      {type.key === 'workload' ? <p className="feature-note">{t('platform.resourceTypes.thisTypeDescribesTheApplications')}{' '} <a href={href({ name: 'rendering-templates' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'rendering-templates' }); }}>{t('platform.resourceTypes.renderingTemplate')}</a>{t('platform.resourceTypes.doNotCreateAResource')}</p> : null}
      {open ? <section className="catalog-details" aria-label={t('platform.resourceTypes.schemaOfKey', { key: type.key })}>
        <h3>{t('platform.resourceTypes.inputs')}</h3>{inputList.length === 0 ? <p className="muted">{t('platform.resourceTypes.noInputParametersDeclared')}</p> : <SchemaTable label={t('platform.resourceTypes.inputsOfKey', { key: type.key })} fields={inputList} secret={false} />}
        <h3>{t('platform.resourceTypes.outputs')}</h3>{outputList.length === 0 ? <p className="muted">{t('platform.resourceTypes.noOutputParametersDeclared')}</p> : <SchemaTable label={t('platform.resourceTypes.outputsOfKey', { key: type.key })} fields={outputList} secret />}
        <div className="form-actions"><Button type="button" disabled={saving} aria-label={t('platform.resourceTypes.copySchemaKey', { key: type.key })} onClick={() => copySchema(type)}>{t('platform.resourceTypes.copySchema')}</Button></div>
      </section> : null}
    </div>;
  }
  return <section className="page"><header className="page-header"><div><p className="eyebrow">{t('platform.resourceTypes.platform')}</p><h1>{t('platform.resourceTypes.resourceTypes')}</h1><p>{t('platform.resourceTypes.declareReusableContractsInputsOutputs')}</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>{t('platform.resourceTypes.registeredResourceTypes')}</h2></div>
      <p className="feature-note">{t('platform.resourceTypes.provisioningSupportedMeansTheSystem')}</p>
      {capabilityList.loadError ? <div className="form-error" role="alert" aria-label={t('platform.resourceTypes.supportMetadataLoadError')}>{capabilityList.loadError} {' '}{t('platform.resourceTypes.itCouldNotBeConfirmed')}{' '} <Button type="button" onClick={() => void capabilityList.reload()}>{t('platform.resourceTypes.retry')}</Button></div> : null}
      {loadError ? <div className="form-error" role="alert" aria-label={t('platform.resourceTypes.listLoadError')}>{loadError} <Button type="button" onClick={() => void reload()}>{t('platform.resourceTypes.retry')}</Button></div> : loading ? <p role="status">{t('platform.resourceTypes.loadingResourceTypes')}</p> : types.length === 0 ? <p>{t('platform.resourceTypes.noResourceTypesYet')}</p> : <div className="catalog-list">{types.map(renderEntry)}</div>}</section>
    <section className="content-panel"><div className="section-header"><div><h2>{t('platform.resourceTypes.registerResourceType')}</h2><p>{t('platform.resourceTypes.theContractIsIndependentOf')}</p></div></div>
      <form onSubmit={submit}><fieldset className="editor-grid form-fieldset" disabled={saving} aria-busy={saving}><label>{t('platform.resourceTypes.resourceTypeID')}<input ref={idField} value={key} required aria-describedby="resource-type-id-hint" onChange={(event) => setKey(event.target.value)} /></label>
        <p id="resource-type-id-hint" className="feature-note">{t('platform.resourceTypes.useLowercaseLettersDigitsAnd')}{' '} <code>postgres-ha</code>{t('platform.resourceTypes.theIDIsNeverCorrected')}{' '} <code>environment</code> {' '}{t('platform.resourceTypes.and')}{' '} <code>service</code> {' '}{t('platform.resourceTypes.areReservedIDs')}</p>
        {copiedFrom ? <p className="feature-note">{t('platform.resourceTypes.draftCopiedFrom')}{' '} <code>{copiedFrom}</code>{t('platform.resourceTypes.enterANewIDCheck')}</p> : null}
        {renderFields(t('platform.resourceTypes.inputs'), t('platform.resourceTypes.inputParameters'), inputs, setInputs, newInput, false)}{renderFields(t('platform.resourceTypes.outputs'), t('platform.resourceTypes.outputParameters'), outputs, setOutputs, newOutput, true)}
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saving}>{saving ? t('platform.resourceTypes.registering') : t('platform.resourceTypes.registerResourceType')}</Button></div>
      </fieldset></form>
    </section>
  </section>;
}
