import { useEffect, useRef, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { ApiError } from '../../shared/api/client';
import { getConfiguration } from '../configuration/api';
import { getResourceTypes, getWorkloads, parseScoreImport, saveWorkload, type ResourceType, type Workload } from './api';
import { ApplicationKeyPicker } from './ApplicationKeyPicker';
import { bindingProblem, emptyBinding, keyBinding, type Binding, type KeyCatalog, type Source } from './bindings';

type ContainerForm = { name: string; image: string; cpuRequest: string; memoryRequest: string; cpuLimit: string; memoryLimit: string; bindings: Binding[] };
type Dependency = { alias: string; type: string; className: string; params: Record<string, string> };
type ServicePort = { name: string; port: string; targetPort: string };
type PublicRoute = { path: string; port: string };
type Form = { name: string; containers: ContainerForm[]; dependencies: Dependency[]; servicePorts: ServicePort[]; publicRoutes: PublicRoute[] };

const emptyContainer = (): ContainerForm => ({ name: 'main', image: '', cpuRequest: '', memoryRequest: '', cpuLimit: '', memoryLimit: '', bindings: [] });
const emptyForm = (): Form => ({ name: '', containers: [emptyContainer()], dependencies: [], servicePorts: [], publicRoutes: [] });

function needsScoreEditor(raw: Record<string, unknown>, types: ResourceType[]): boolean {
  if (Object.keys(raw).some((key) => !['apiVersion', 'metadata', 'containers', 'resources', 'service'].includes(key))) return true;
  if (Object.keys((raw.metadata ?? {}) as Record<string, unknown>).some((key) => key !== 'name')) return true;
  const containers = (raw.containers ?? {}) as Record<string, Record<string, unknown>>;
  if (Object.values(containers).some((container) => Object.keys(container).some((key) => !['image', 'variables', 'resources'].includes(key)))) return true;
  if (Object.values(containers).some((container) => Object.keys((container.resources ?? {}) as Record<string, unknown>).some((key) => !['requests', 'limits'].includes(key)))) return true;
  if (Object.values(containers).some((container) => Object.values((container.resources ?? {}) as Record<string, Record<string, unknown>>).some((quantity) => Object.keys(quantity).some((key) => !['cpu', 'memory'].includes(key))))) return true;
  const resources = (raw.resources ?? {}) as Record<string, Record<string, unknown>>;
  if (Object.values(resources).some((resource) => resource.id != null || (resource.type !== 'service' && resource.params != null && Object.values(resource.params as Record<string, unknown>).some((value) => value === null || typeof value === 'object')))) return true;
  if (Object.values(resources).some((resource) => {
    if (resource.type === 'service' || resource.type === 'environment') return false;
    const contract = types.find((item) => item.key === resource.type);
    if (!contract) return true;
    return Object.entries((resource.params ?? {}) as Record<string, unknown>).some(([name, value]) => {
      const input = contract.inputs?.find((item) => item.name === name);
      return !input || typeof value !== (input.type === 'number' ? 'number' : input.type === 'bool' ? 'boolean' : 'string');
    });
  })) return true;
  if (Object.values(resources).some((resource) => Object.keys(resource).some((key) => !['type', 'class', 'params'].includes(key)))) return true;
  if (Object.values(resources).some((resource) => resource.type === 'environment' && Object.keys(resource).length !== 1)) return true;
  if (Object.values(resources).some((resource) => resource.type === 'service' && (resource.class != null || Object.keys((resource.params ?? {}) as Record<string, unknown>).some((key) => !['workload', 'port'].includes(key))))) return true;
  if (raw.service && Object.keys(raw.service as Record<string, unknown>).some((key) => key !== 'ports' && key !== 'publicPort' && key !== 'publicRoutes')) return true;
  const ports = ((raw.service as { ports?: Record<string, Record<string, unknown>> } | undefined)?.ports ?? {});
  return Object.values(ports).some((port) => Object.keys(port).some((key) => !['port', 'targetPort'].includes(key)));
}

// Reads a resource input the user set; an unset name such as __proto__ is empty,
// not an inherited Object member.
const paramValue = (params: Record<string, string>, name: string): string => (Object.hasOwn(params, name) ? params[name] ?? '' : '');

// Every dictionary keyed by a user-chosen name is collected in a Map and turned
// into an object with Object.fromEntries, so a name such as __proto__ stays an
// own property instead of replacing the prototype and being dropped from JSON.
function buildScore(form: Form, types: ResourceType[]): Record<string, unknown> {
  const resources = new Map<string, unknown>();
  const containers = new Map<string, unknown>();
  for (const dependency of form.dependencies) {
    if (dependency.alias && dependency.type) {
      const inputs = types.find((item) => item.key === dependency.type)?.inputs ?? [];
      const params: [string, string | number | boolean][] = [];
      for (const input of inputs) {
        const value = Object.hasOwn(dependency.params, input.name) ? dependency.params[input.name] : undefined;
        if (value === undefined || value === '') continue;
        params.push([input.name, input.type === 'number' ? Number(value) : input.type === 'bool' ? value === 'true' : value]);
      }
      resources.set(dependency.alias, { type: dependency.type, ...(dependency.className ? { class: dependency.className } : {}), ...(params.length ? { params: Object.fromEntries(params) } : {}) });
    }
  }
  for (const container of form.containers) {
    const variables: [string, string][] = [];
    const names = new Set<string>();
    for (const binding of container.bindings) {
      // bindingProblem runs first; this guard only refuses a silent overwrite.
      if (names.has(binding.name)) throw new Error(`Container ${container.name}: duplicate name ${binding.name}.`);
      names.add(binding.name);
      let alias = binding.alias;
      let output = binding.output;
      if (binding.source === 'KEY') { alias = 'env'; output = binding.key; resources.set('env', { type: 'environment' }); }
      if (binding.source === 'SERVICE') { alias = `svc_${binding.workload}_${binding.port}`.replace(/[^A-Za-z0-9_-]/g, '_'); output = 'url'; resources.set(alias, { type: 'service', params: { workload: binding.workload, port: binding.port } }); }
      variables.push([binding.name, `\${resources.${alias}.${output}}`]);
    }
    const requests: Record<string, string> = {};
    const limits: Record<string, string> = {};
    if (container.cpuRequest) requests.cpu = container.cpuRequest;
    if (container.memoryRequest) requests.memory = container.memoryRequest;
    if (container.cpuLimit) limits.cpu = container.cpuLimit;
    if (container.memoryLimit) limits.memory = container.memoryLimit;
    containers.set(container.name, { image: container.image, ...(variables.length ? { variables: Object.fromEntries(variables) } : {}), ...(Object.keys(requests).length || Object.keys(limits).length ? { resources: { ...(Object.keys(requests).length ? { requests } : {}), ...(Object.keys(limits).length ? { limits } : {}) } } : {}) });
  }
  const ports = new Map<string, unknown>();
  for (const port of form.servicePorts) if (port.name && Number(port.port) > 0) ports.set(port.name, { port: Number(port.port), targetPort: Number(port.targetPort || port.port) });
  return { apiVersion: 'score.dev/v1b1', metadata: { name: form.name }, containers: Object.fromEntries(containers), ...(ports.size ? { service: { ports: Object.fromEntries(ports), ...(form.publicRoutes.length ? { publicRoutes: form.publicRoutes } : {}) } } : {}), ...(resources.size ? { resources: Object.fromEntries(resources) } : {}) };
}

function readForm(score: Record<string, unknown>): Form {
  const form = emptyForm();
  form.name = String((score.metadata as { name?: string } | undefined)?.name ?? '');
  const resources = (score.resources ?? {}) as Record<string, { type?: string; class?: string; params?: Record<string, unknown> }>;
  form.dependencies = Object.entries(resources).filter(([, spec]) => spec.type !== 'environment' && spec.type !== 'service').map(([alias, spec]) => ({ alias, type: spec.type ?? '', className: spec.class ?? '', params: Object.fromEntries(Object.entries(spec.params ?? {}).map(([key, value]) => [key, String(value)])) }));
  const rawContainers = (score.containers ?? {}) as Record<string, { image?: string; variables?: Record<string, string>; resources?: { requests?: { cpu?: string; memory?: string }; limits?: { cpu?: string; memory?: string } } }>;
  form.containers = Object.entries(rawContainers).map(([name, value]) => ({ name, image: value.image ?? '', cpuRequest: value.resources?.requests?.cpu ?? '', memoryRequest: value.resources?.requests?.memory ?? '', cpuLimit: value.resources?.limits?.cpu ?? '', memoryLimit: value.resources?.limits?.memory ?? '', bindings: Object.entries(value.variables ?? {}).map(([envName, raw]) => {
    const match = raw.match(/^\$\{resources\.([A-Za-z0-9_-]+)\.([A-Za-z0-9_-]+)\}$/);
    const alias = match?.[1] ?? ''; const output = match?.[2] ?? '';
    if (alias === 'env') return keyBinding(output, envName);
    if (resources[alias]?.type === 'service') return { ...emptyBinding(), name: envName, source: 'SERVICE' as const, workload: String(resources[alias].params?.workload ?? ''), port: String(resources[alias].params?.port ?? '') };
    return { ...emptyBinding(), name: envName, source: 'RESOURCE' as const, alias, output };
  }) }));
  if (!form.containers.length) form.containers = [emptyContainer()];
  const ports = ((score.service as { ports?: Record<string, { port: number; targetPort?: number }> } | undefined)?.ports ?? {});
  form.servicePorts = Object.entries(ports).map(([name, port]) => ({ name, port: String(port.port), targetPort: String(port.targetPort ?? port.port) }));
  const service = score.service as { publicPort?: string; publicRoutes?: PublicRoute[] } | undefined;
  form.publicRoutes = [...(service?.publicRoutes ?? []), ...(service?.publicPort ? [{ path: '/', port: service.publicPort }] : [])];
  return form;
}

export function WorkloadEditorPage({ application, environment, workloadId }: { application: Application; environment: EnvironmentKey; workloadId?: string }) {
  const [form, setForm] = useState<Form>(emptyForm);
  const [catalog, setCatalog] = useState<KeyCatalog>({ status: 'loading' });
  const [catalogAttempt, setCatalogAttempt] = useState(0);
  const [types, setTypes] = useState<ResourceType[]>([]);
  const [workloads, setWorkloads] = useState<Workload[]>([]);
  const [version, setVersion] = useState(0);
  const [mode, setMode] = useState<'form' | 'import'>('form');
  const [imported, setImported] = useState<Record<string, unknown>>();
  const [dirty, setDirty] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [blocked, setBlocked] = useState(false);
  const [advancedScore, setAdvancedScore] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const [loadAttempt, setLoadAttempt] = useState(0);
  // A 409 on Save marks the page stale: input is kept, Save is disabled until
  // the user reloads and reviews the current state; nothing is re-sent.
  const [stale, setStale] = useState(false);
  const [reloading, setReloading] = useState(false);
  const [current, setCurrent] = useState<{ version: number; score?: Record<string, unknown>; state?: string }>();
  const [reloadError, setReloadError] = useState('');
  // Scope/unmount guard for import, Save and reload: a late reply for another
  // scope or after leaving the page is dropped.
  const scope = useRef(0);
  useEffect(() => () => { scope.current += 1; }, []);

  // A different Application, Environment or workload starts from its own
  // empty/draft state; nothing selected, imported, saving or reloading carries
  // over (a pending reload's finally ignores the new scope by design).
  useEffect(() => {
    scope.current += 1;
    setForm(emptyForm()); setImported(undefined); setDirty(false); setMode('form'); setAdvancedScore(false); setBlocked(false); setSaving(false); setReloading(false); setWorkloads([]);
  }, [application.id, environment, workloadId]);

  useEffect(() => {
    let cancelled = false;
    setStale(false); setCurrent(undefined); setReloadError('');
    setLoading(true); setLoadFailed(false); setError('');
    Promise.all([getWorkloads(application.id, environment), getResourceTypes()]).then(([list, catalog]) => {
      if (cancelled) return;
      setWorkloads(list.workloads); setTypes(catalog.resourceTypes.filter((type) => type.key !== 'workload')); setVersion(list.draftVersion);
      const existing = list.workloads.find((item) => item.id === workloadId);
      if (existing?.score) { setForm(readForm(existing.score)); setImported(existing.score); setBlocked(false); const advanced = needsScoreEditor(existing.score, catalog.resourceTypes); setAdvancedScore(advanced); if (advanced) setMode('import'); }
      else if (workloadId) { setForm({ ...emptyForm(), name: workloadId }); setBlocked(true); setError(existing ? 'This deployed workload has no editable Score draft yet. Editing is disabled to avoid losing its existing configuration.' : 'Workload not found in this environment.'); }
    }).catch((err: Error) => { if (!cancelled) { setError(err.message); setLoadFailed(true); } }).finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [application.id, environment, workloadId, loadAttempt]);

  // The Application key catalog loads on its own so a failure is shown as a
  // retryable state in the picker, never as an empty key list.
  useEffect(() => {
    let cancelled = false;
    setCatalog({ status: 'loading' });
    getConfiguration(application.id, environment).then((config) => { if (!cancelled) setCatalog({ status: 'ready', keys: config.keys ?? [] }); })
      .catch((err: Error) => { if (!cancelled) setCatalog({ status: 'error', message: err.message }); });
    return () => { cancelled = true; };
  }, [application.id, environment, loadAttempt, catalogAttempt]);

  // Reloads the Environment's current drafts without touching the form.
  async function reloadCurrent() {
    const at = scope.current;
    setReloading(true); setReloadError('');
    try {
      const [config, list] = await Promise.all([getConfiguration(application.id, environment), getWorkloads(application.id, environment)]);
      if (at !== scope.current) return;
      const id = workloadId ?? form.name;
      const item = list.workloads.find((entry) => entry.id === id);
      setCatalog({ status: 'ready', keys: config.keys ?? [] }); setWorkloads(list.workloads); setVersion(list.draftVersion);
      setCurrent({ version: list.draftVersion, score: item?.score, state: item?.state || (item ? 'deployed' : 'absent') });
      setStale(false); setError('');
    } catch (err) {
      if (at === scope.current) setReloadError(`Could not reload the current state: ${(err as Error).message}. Your edits are kept; try again.`);
    } finally {
      if (at === scope.current) setReloading(false);
    }
  }

  function updateForm(next: Form) { setForm(next); setDirty(true); }
  function updateContainer(index: number, next: ContainerForm) { const containers = [...form.containers]; containers[index] = next; updateForm({ ...form, containers }); }
  function updateBinding(containerIndex: number, bindingIndex: number, next: Binding) { const container = form.containers[containerIndex]; if (!container) return; const bindings = [...container.bindings]; bindings[bindingIndex] = next; updateContainer(containerIndex, { ...container, bindings }); }
  async function importFile(file: File) {
    const at = scope.current;
    setError('');
    try {
      const content = await file.text();
      if (at !== scope.current) return;
      const result = await parseScoreImport(application.id, environment, content);
      if (at !== scope.current) return;
      setImported(result.score); setForm(readForm(result.score)); setAdvancedScore(needsScoreEditor(result.score, types)); setDirty(false);
    }
    catch (err) { if (at === scope.current) setError((err as Error).message); }
  }
  async function save() {
    if (blocked || stale) return;
    const seenPaths = new Set<string>();
    for (const route of form.publicRoutes) {
      if (!/^\/(?:[A-Za-z0-9._~-]+(?:\/[A-Za-z0-9._~-]+)*)?$/.test(route.path) || seenPaths.has(route.path)) { setError('Public paths must be unique URL paths such as / or /api.'); return; }
      seenPaths.add(route.path);
      if (!form.servicePorts.some((port) => port.name === route.port && Number(port.port) > 0)) { setError('Choose a declared Service port for every public path.'); return; }
    }
    if (mode === 'form' || dirty) {
      for (const dependency of form.dependencies) {
        const contract = types.find((item) => item.key === dependency.type);
        if (!contract) { setError(`Choose a valid resource type for ${dependency.alias || 'resource'}.`); return; }
        for (const input of contract.inputs ?? []) {
          const value = paramValue(dependency.params, input.name);
          if (input.required && !value.trim()) { setError(`${dependency.alias || dependency.type}: ${input.name} is required.`); return; }
          if (value && input.type === 'number' && !Number.isFinite(Number(value))) { setError(`${dependency.alias || dependency.type}: ${input.name} must be a number.`); return; }
        }
      }
      if (catalog.status !== 'ready') { setError(catalog.status === 'loading' ? 'Application variables and secrets are still loading. Wait for them before saving.' : 'Application variables and secrets could not be loaded. Retry loading them before saving.'); return; }
      const problem = bindingProblem(form.containers, new Set(catalog.keys.map((key) => key.name)));
      if (problem) { setError(problem); return; }
    }
    const score = mode === 'import' && imported && !dirty ? imported : !dirty && imported ? imported : buildScore(form, types);
    const id = String((score.metadata as { name?: string } | undefined)?.name ?? '');
    if (!id) { setError('Workload name is required.'); return; }
    const at = scope.current;
    setSaving(true); setError('');
    try { await saveWorkload(application.id, environment, id, score, version); if (at === scope.current) navigate({ name: 'application', applicationId: application.id }); }
    catch (err) {
      if (at !== scope.current) return;
      if (err instanceof ApiError && err.status === 409) { setStale(true); setCurrent(undefined); setReloadError(''); }
      else setError((err as Error).message);
    }
    finally { if (at === scope.current) setSaving(false); }
  }
  const candidateServices = workloads.filter((item) => item.id !== form.name && item.state !== 'PENDING_DELETE' && item.servicePorts?.length);

  return <section className="page workload-editor">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id })}>← {application.name} / {environment}</button>
    <header className="page-header application-header"><div><p className="eyebrow">{environment} · Workload configuration</p><h1>{workloadId ? `Edit ${workloadId}` : 'Add workload'}</h1><p>Save a draft first. Preview and Deploy happen separately.</p></div><Button onClick={() => navigate({ name: 'settings', applicationId: application.id })}>Variables &amp; Secrets</Button></header>
    <div className="tabs" role="tablist"><button className={mode === 'form' ? 'tab tab-active' : 'tab'} disabled={advancedScore} onClick={() => setMode('form')}>Enter on form</button><button className={mode === 'import' ? 'tab tab-active' : 'tab'} onClick={() => setMode('import')}>Import Score</button></div>
    {advancedScore ? <p className="feature-note">This Score contains fields the form cannot preserve. Upload an updated Score file to edit it without losing those fields.</p> : null}
    {error ? <div className="form-error" role="alert">{error}{loadFailed ? <> <Button onClick={() => setLoadAttempt((value) => value + 1)}>Retry</Button></> : null}</div> : null}
    {stale ? <div className="form-error" role="alert" aria-label="Stale workload drafts">Workloads in {environment} changed after this page loaded. Your edits are kept but were not saved. Reload the current state, review it, then save again. <Button disabled={reloading} onClick={() => void reloadCurrent()}>{reloading ? 'Reloading…' : 'Reload current state'}</Button>{reloadError ? <p>{reloadError}</p> : null}</div> : null}
    {current ? <div className="form-info" role="status">Reloaded draft version {current.version}. Current state of this workload: {current.state === 'absent' ? 'not present' : current.state === 'deployed' ? 'deployed, no pending change' : current.state === 'PENDING_DELETE' ? 'pending deletion' : 'pending change'}. Your edits below are unchanged; review them and save again if they still apply.{current.score ? <details><summary>Current Score</summary><pre className="score-preview">{JSON.stringify(current.score, null, 2)}</pre></details> : null}</div> : null}
    {loading ? <p>Loading workload options…</p> : mode === 'import' ? <section className="content-panel"><h2>Import one Score file</h2><p>YAML or JSON. The same reference rules apply as the form; importing does not save or deploy.</p><input aria-label="Score file" type="file" accept=".yaml,.yml,.json,text/yaml,application/json" onChange={(event) => { const file = event.target.files?.[0]; if (file) void importFile(file); }} />{imported ? <><p>Parsed workload: <strong>{String((imported.metadata as { name?: string } | undefined)?.name ?? '')}</strong></p><pre className="score-preview">{JSON.stringify(imported, null, 2)}</pre></> : null}</section> : <>
      <section className="content-panel editor-grid"><h2>Basic information</h2><label>Workload name<input value={form.name} disabled={Boolean(workloadId)} onChange={(event) => updateForm({ ...form, name: event.target.value })} placeholder="frontend" /></label></section>
      {catalog.status === 'error' ? <div className="form-error" role="alert">Application variables and secrets could not be loaded: {catalog.message}. Your selections are kept. <Button onClick={() => setCatalogAttempt((value) => value + 1)}>Retry loading keys</Button></div> : null}
      {form.containers.map((container, ci) => <section className="content-panel editor-grid" key={ci}><div className="section-header"><h2>Container {ci + 1}</h2>{form.containers.length > 1 ? <Button tone="danger" onClick={() => updateForm({ ...form, containers: form.containers.filter((_, index) => index !== ci) })}>Remove</Button> : null}</div><div className="field-grid"><label>Name<input value={container.name} onChange={(event) => updateContainer(ci, { ...container, name: event.target.value })} /></label><label>Image<input value={container.image} onChange={(event) => updateContainer(ci, { ...container, image: event.target.value })} placeholder="registry.example/app:tag" /></label></div><div className="field-grid field-grid-four"><label>CPU request<input value={container.cpuRequest} onChange={(event) => updateContainer(ci, { ...container, cpuRequest: event.target.value })} placeholder="100m" /></label><label>Memory request<input value={container.memoryRequest} onChange={(event) => updateContainer(ci, { ...container, memoryRequest: event.target.value })} placeholder="128Mi" /></label><label>CPU limit<input value={container.cpuLimit} onChange={(event) => updateContainer(ci, { ...container, cpuLimit: event.target.value })} /></label><label>Memory limit<input value={container.memoryLimit} onChange={(event) => updateContainer(ci, { ...container, memoryLimit: event.target.value })} /></label></div>
        <h3>Application variables &amp; secrets</h3><p>Tick existing keys from {environment} settings. Each key keeps its own name in this container unless you choose a different one. Values are never shown or copied.</p>
        <ApplicationKeyPicker container={container.name || `container ${ci + 1}`} bindings={container.bindings} catalog={catalog} disabled={saving} onChange={(bindings) => updateContainer(ci, { ...container, bindings })} />
        <h3>Other sources</h3><p>Resource outputs and Services of other workloads in {environment}. Direct values are not allowed.</p>
        {container.bindings.map((binding, bi) => binding.source === 'KEY' ? null : <div className="binding-row" key={bi}><input aria-label="Container variable name" value={binding.name} disabled={saving} placeholder="API_URL" onChange={(event) => updateBinding(ci, bi, { ...binding, name: event.target.value })} /><select aria-label="Reference source" value={binding.source} disabled={saving} onChange={(event) => updateBinding(ci, bi, { ...emptyBinding(), name: binding.name, source: event.target.value as Source })}><option value="RESOURCE">Resource output</option><option value="SERVICE">Workload Service</option></select>
          {binding.source === 'RESOURCE' ? <><select aria-label="Resource dependency" value={binding.alias} disabled={saving} onChange={(event) => updateBinding(ci, bi, { ...binding, alias: event.target.value, output: '' })}><option value="">Choose resource</option>{form.dependencies.map((dep) => <option key={dep.alias} value={dep.alias}>{dep.alias}</option>)}</select><select aria-label="Resource output" value={binding.output} disabled={saving} onChange={(event) => updateBinding(ci, bi, { ...binding, output: event.target.value })}><option value="">Choose output</option>{types.find((type) => type.key === form.dependencies.find((dep) => dep.alias === binding.alias)?.type)?.outputs.map((output) => <option key={output.name} value={output.name}>{output.name}{output.secret ? ' · secret' : ''}</option>)}</select></> : null}
          {binding.source === 'SERVICE' ? <><select aria-label="Service workload" value={binding.workload} disabled={saving} onChange={(event) => updateBinding(ci, bi, { ...binding, workload: event.target.value, port: '' })}><option value="">Choose workload</option>{candidateServices.map((item) => <option key={item.id} value={item.id}>{item.id}</option>)}</select><select aria-label="Service port" value={binding.port} disabled={saving} onChange={(event) => updateBinding(ci, bi, { ...binding, port: event.target.value })}><option value="">Choose port</option>{candidateServices.find((item) => item.id === binding.workload)?.servicePorts?.map((port) => <option key={port} value={port}>{port}</option>)}</select></> : null}<Button tone="quiet" disabled={saving} onClick={() => updateContainer(ci, { ...container, bindings: container.bindings.filter((_, index) => index !== bi) })}>Remove</Button></div>)}
        <Button disabled={saving} onClick={() => updateContainer(ci, { ...container, bindings: [...container.bindings, emptyBinding()] })}>+ Add other source</Button></section>)}
      <Button onClick={() => updateForm({ ...form, containers: [...form.containers, { ...emptyContainer(), name: `container-${form.containers.length + 1}` }] })}>+ Add container</Button>
      <section className="content-panel editor-grid"><div className="section-header"><div><h2>Resource dependencies</h2><p>Choose an existing resource type, then fill its required inputs and use its outputs above.</p></div><Button onClick={() => updateForm({ ...form, dependencies: [...form.dependencies, { alias: '', type: '', className: '', params: {} }] })}>+ Add resource</Button></div>{form.dependencies.map((dep, index) => <div className="editor-grid" key={index}><div className="binding-row"><input aria-label="Resource alias" value={dep.alias} placeholder="db" onChange={(event) => updateForm({ ...form, dependencies: form.dependencies.map((item, i) => i === index ? { ...item, alias: event.target.value } : item) })} /><select aria-label="Resource type" value={dep.type} onChange={(event) => updateForm({ ...form, dependencies: form.dependencies.map((item, i) => i === index ? { ...item, type: event.target.value, params: {} } : item) })}><option value="">Choose type</option>{types.map((type) => <option key={type.key} value={type.key}>{type.key}</option>)}</select><input aria-label="Resource class" value={dep.className} placeholder="default" onChange={(event) => updateForm({ ...form, dependencies: form.dependencies.map((item, i) => i === index ? { ...item, className: event.target.value } : item) })} /><Button tone="quiet" onClick={() => updateForm({ ...form, dependencies: form.dependencies.filter((_, i) => i !== index) })}>Remove</Button></div><div className="field-grid">{types.find((type) => type.key === dep.type)?.inputs?.map((input) => <label key={input.name}>{dep.alias || dep.type} · {input.name}{input.required ? ' *' : ''}{input.type === 'bool' ? <select aria-label={`Resource ${input.name}`} value={paramValue(dep.params, input.name)} onChange={(event) => updateForm({ ...form, dependencies: form.dependencies.map((item, i) => i === index ? { ...item, params: { ...item.params, [input.name]: event.target.value } } : item) })}><option value="">Choose</option><option value="true">True</option><option value="false">False</option></select> : <input aria-label={`Resource ${input.name}`} type={input.type === 'number' ? 'number' : 'text'} value={paramValue(dep.params, input.name)} onChange={(event) => updateForm({ ...form, dependencies: form.dependencies.map((item, i) => i === index ? { ...item, params: { ...item.params, [input.name]: event.target.value } } : item) })} />}</label>)}</div></div>)}</section>
      <section className="content-panel editor-grid">
        <div className="section-header"><div><h2>Service ports</h2><p>Internal access for other workloads in {environment}.</p></div><Button onClick={() => updateForm({ ...form, servicePorts: [...form.servicePorts, { name: '', port: '', targetPort: '' }] })}>+ Add port</Button></div>
        {form.servicePorts.map((port, index) => <div className="binding-row" key={index}><input aria-label="Service port name" value={port.name} placeholder="http" onChange={(event) => updateForm({ ...form, servicePorts: form.servicePorts.map((item, i) => i === index ? { ...item, name: event.target.value } : item) })} /><input aria-label="Service port" type="number" value={port.port} placeholder="80" onChange={(event) => updateForm({ ...form, servicePorts: form.servicePorts.map((item, i) => i === index ? { ...item, port: event.target.value } : item) })} /><input aria-label="Container target port" type="number" value={port.targetPort} placeholder="8080" onChange={(event) => updateForm({ ...form, servicePorts: form.servicePorts.map((item, i) => i === index ? { ...item, targetPort: event.target.value } : item) })} /><Button tone="quiet" onClick={() => updateForm({ ...form, servicePorts: form.servicePorts.filter((_, i) => i !== index) })}>Remove</Button></div>)}
        <div className="section-header"><div><h3>Public paths</h3><p>For example, / for frontend and /api for backend.</p></div><Button onClick={() => updateForm({ ...form, publicRoutes: [...form.publicRoutes, { path: '', port: '' }] })}>+ Add public path</Button></div>
        {form.publicRoutes.map((route, index) => <div className="binding-row" key={index}><input aria-label="Public path" value={route.path} placeholder="/api" onChange={(event) => updateForm({ ...form, publicRoutes: form.publicRoutes.map((item, i) => i === index ? { ...item, path: event.target.value } : item) })} /><select aria-label="Public Service port" value={route.port} onChange={(event) => updateForm({ ...form, publicRoutes: form.publicRoutes.map((item, i) => i === index ? { ...item, port: event.target.value } : item) })}><option value="">Choose port</option>{form.servicePorts.filter((port) => port.name && Number(port.port) > 0).map((port) => <option key={port.name} value={port.name}>{port.name} · {port.port}</option>)}</select><Button tone="quiet" onClick={() => updateForm({ ...form, publicRoutes: form.publicRoutes.filter((_, i) => i !== index) })}>Remove</Button></div>)}
        <p>Public access is applied only after Preview → Deploy. DNS and TLS are not configured here.</p>
      </section>
    </>}
    {!loading ? <div className="form-actions editor-actions"><Button onClick={() => navigate({ name: 'application', applicationId: application.id })}>Cancel</Button><Button tone="primary" disabled={blocked || saving || stale || loadFailed || (mode === 'import' && !imported)} onClick={() => void save()}>Save pending workload</Button></div> : null}
  </section>;
}
