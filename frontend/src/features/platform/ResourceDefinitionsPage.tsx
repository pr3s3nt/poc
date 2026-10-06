import { useState, type FormEvent } from 'react';
import { api } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { useCatalogList } from './useCatalogList';

type ResourceType = { key: string };
type Criterion = { env_type: string; app_id: string; env_id: string; res_id: string; class: string };
type Definition = { key: string; resourceType: string; executionProfile?: string; driverType: string; criteria: Criterion[] };
const emptyCriterion = (): Criterion => ({ env_type: '', app_id: '', env_id: '', res_id: '', class: '' });
const criterionFields: { key: keyof Criterion; label: string }[] = [
  { key: 'env_type', label: 'Environment type' }, { key: 'app_id', label: 'Application ID' },
  { key: 'env_id', label: 'Environment ID' }, { key: 'res_id', label: 'Resource ID' }, { key: 'class', label: 'Class' },
];
function objectJSON(value: string, label: string): Record<string, unknown> {
  let parsed: unknown;
  try { parsed = JSON.parse(value); } catch { throw new Error(`${label} must be valid JSON.`); }
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error(`${label} must be a JSON object.`);
  return parsed as Record<string, unknown>;
}

const loadTypes = async () => (await api<{ resourceTypes: ResourceType[] }>('/resource-types')).resourceTypes ?? [];
const loadDefinitions = async () => (await api<{ resourceDefinitions: Definition[] }>('/resource-definitions')).resourceDefinitions ?? [];

export function ResourceDefinitionsPage() {
  const typeList = useCatalogList(loadTypes);
  const { items: definitions, loading, loadError, reload } = useCatalogList(loadDefinitions);
  const types = typeList.items;
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [key, setKey] = useState('');
  const [resourceType, setResourceType] = useState('');
  const [profile, setProfile] = useState('internal-k8s');
  const [driver, setDriver] = useState('kubernetes');
  const [renderBundle, setRenderBundle] = useState('score-k8s-internal-v1');
  const isRenderer = driver === 'score-k8s';
  const [connectionKey, setConnectionKey] = useState('');
  const [module, setModule] = useState('');
  const [variables, setVariables] = useState('{}');
  const [provision, setProvision] = useState('{}');
  const [criteria, setCriteria] = useState<Criterion[]>([emptyCriterion()]);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    setError(''); setNotice('');
    let inputVariables: Record<string, unknown>, provisionRules: Record<string, unknown>;
    try { inputVariables = isRenderer ? { render_bundle: renderBundle } : objectJSON(variables, 'Driver variables'); provisionRules = isRenderer ? {} : objectJSON(provision, 'Provision rules'); }
    catch (reason) { setError((reason as Error).message); return; }
    setSaving(true);
    const cleanCriteria = criteria.map((criterion) => Object.fromEntries(Object.entries(criterion).filter(([, value]) => value.trim() !== '')));
    const values: Record<string, unknown> = { variables: inputVariables };
    if (driver === 'terraform') values.source = { module };
    // The backend owns the ID rule (UC-03 BR-10); send the ID exactly as typed.
    const submitted = key;
    try {
      await api('/resource-definitions', { method: 'POST', body: JSON.stringify({ key: submitted, resourceType, executionProfile: profile, driverType: driver, connectionKey: isRenderer ? '' : connectionKey.trim(), driverInputs: { values }, provision: provisionRules, criteria: cleanCriteria }) });
    } catch (reason) { setError((reason as Error).message); setSaving(false); return; }
    // Committed: report it and reset the submitted form before reloading.
    setNotice(`Registered resource definition ${submitted}.`); setKey(''); setVariables('{}'); setProvision('{}'); setCriteria([emptyCriterion()]); setSaving(false);
    await reload();
  }
  return <section className="page"><header className="page-header"><div><p className="eyebrow">Platform</p><h1>Resource definitions</h1><p>Map a Resource Type to an executable driver and matching criteria.</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>Registered definitions</h2></div>{loadError || typeList.loadError ? <div className="form-error" role="alert" aria-label="List error">{loadError || typeList.loadError} <Button type="button" onClick={() => { void reload(); void typeList.reload(); }}>Retry</Button></div> : loading || typeList.loading ? <p>Loading definitions…</p> : definitions.length === 0 ? <p>No resource definitions registered yet.</p> : <div className="catalog-list">{definitions.map((definition) => <div key={definition.key} className="catalog-entry"><strong>{definition.key}</strong><span>{definition.resourceType} · {definition.executionProfile || 'all profiles'} · {definition.driverType} · {definition.criteria.length} criteria</span></div>)}</div>}</section>
    <section className="content-panel"><div className="section-header"><div><h2>Register resource definition</h2><p>Only runtime-supported drivers and embedded Terraform modules are accepted.</p></div></div>
      <form onSubmit={submit}><fieldset className="editor-grid form-fieldset" disabled={saving} aria-busy={saving}>
        <div className="field-grid"><label>Definition ID<input value={key} required aria-describedby="definition-id-hint" onChange={(event) => setKey(event.target.value)} /></label><label>Resource Type<select value={resourceType} disabled={isRenderer} required onChange={(event) => setResourceType(event.target.value)}><option value="">Select a Resource Type</option>{types.map((type) => <option key={type.key} value={type.key}>{type.key}</option>)}</select></label></div>
        <p id="definition-id-hint" className="feature-note">Use lowercase letters, digits and hyphens only. The ID is not changed for you.</p>
        <div className="field-grid"><label>Execution profile<select value={profile} disabled={isRenderer} onChange={(event) => setProfile(event.target.value)}><option value="internal-k8s">internal-k8s</option><option value="aws-eks">aws-eks</option></select></label><label>Driver<select value={driver} onChange={(event) => { const selected = event.target.value; setDriver(selected); if (selected === 'score-k8s') { setResourceType('workload'); setProfile('internal-k8s'); } }}><option value="kubernetes">Kubernetes</option><option value="terraform">Terraform</option><option value="existing-cluster">Existing cluster</option><option value="score-k8s">score-k8s workload renderer</option></select></label></div>
        {!isRenderer ? <div className="field-grid"><label>Connection key<input value={connectionKey} onChange={(event) => setConnectionKey(event.target.value)} placeholder="Required for Terraform/existing cluster" /></label>{driver === 'terraform' ? <label>Embedded Terraform module<select value={module} required onChange={(event) => setModule(event.target.value)}><option value="">Select a module</option><option value="vpc">vpc</option><option value="eks">eks</option><option value="aurora">aurora</option></select></label> : null}</div> : <label>Rendering bundle<input value={renderBundle} required onChange={(event) => setRenderBundle(event.target.value)} /><span className="feature-note">Use a bundle installed by your platform engineer. Database and namespace provisioning use their own Definitions.</span></label>}
        <section className="catalog-fields"><div className="section-header"><div><h3>Matching criteria</h3><p>An empty row is an explicit wildcard.</p></div><Button type="button" onClick={() => setCriteria([...criteria, emptyCriterion()])}>+ Add criterion</Button></div>{criteria.map((criterion, index) => <div className="catalog-criterion" key={index}>{criterionFields.map(({ key: field, label }) => <label key={field}>{label}<input aria-label={`Criterion ${index + 1} ${label}`} value={criterion[field]} onChange={(event) => setCriteria(criteria.map((item, position) => position === index ? { ...item, [field]: event.target.value } : item))} /></label>)}<Button type="button" tone="quiet" disabled={criteria.length === 1} onClick={() => setCriteria(criteria.filter((_, position) => position !== index))}>Remove</Button></div>)}</section>
        {!isRenderer ? <><label>Driver variables (JSON object)<textarea value={variables} rows={6} spellCheck={false} aria-describedby="driver-variables-hint" onChange={(event) => setVariables(event.target.value)} /></label>
        <p id="driver-variables-hint" className="feature-note">Only variables the selected driver supports are accepted, and literal values must match their declared types. A value that is one whole placeholder, such as <code>{'${context.app.id}'}</code>, is checked when planning. Credentials and secret references are not accepted.</p>
        <label>Provision rules (JSON object)<textarea value={provision} rows={4} spellCheck={false} onChange={(event) => setProvision(event.target.value)} /></label></> : null}
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saving || types.length === 0}>{saving ? 'Registering…' : 'Register resource definition'}</Button></div>
      </fieldset></form>
    </section>
  </section>;
}
