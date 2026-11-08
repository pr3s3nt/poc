import { authoringFeatures } from '../../shared/authoringFeatures';
import { DefaultSelect } from './DefaultSelect';
import { AdvancedFields } from './AdvancedFields';
import { ConfigGroup, fieldValue, RuntimePolicyFields, SecurityFields, wd, type SecurityContext, type Resolution } from './RuntimePolicyFields';
import { api } from '../../shared/api/client';
import { useCallback, useEffect, useRef, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { ApiError, describeError, isBusy, isStaleConflict } from '../../shared/api/client';
import { useEnvironmentBusy } from '../environment/useEnvironmentBusy';
import { getConfiguration, type Configuration, type KeyKind } from '../configuration/api';
import { getResourceTypes, getWorkloads, parseWorkloadImport, saveWorkload, type ResourceType, type Workload } from './api';
import { AddResourceDialog } from './AddResourceDialog';
import { CreateKeyDialog } from './CreateKeyDialog';
import { DependencyFields, dependencyProblem, dependencyAliasesProblem, emptyDependency, type ClassMetadata, type Dependency } from './DependencyFields';
import { EnvironmentVariablesTable, type FocusRequest } from './EnvironmentVariablesTable';
import { ExecutionFields } from './ExecutionFields';
import { ContainerVolumeFields, UnassignedVolumeMounts } from './ContainerVolumeFields';
import { getWorkloadTemplates, templateKey, customExecution, type Execution, type WorkloadTemplate } from './templates';
import { Advanced } from './Advanced';
import { validRuntimeIntent } from './runtimeIntent';
import { WorkloadClassControl } from './WorkloadClassControl';
import { bindingProblem, changeSource, escapeLiteral, isKeySource, readBinding, serviceAlias, type Binding, type KeyCatalog, type ResourceSpecs } from './bindings';
import { buildRowContext } from './rowContext';
import { t } from '../../shared/i18n';
import { sharedResources, sharedSelectionProblem } from './sharedResources';
import { ServiceEndpointFields, endpointPorts } from './ServiceEndpointFields';

import { ProbeFields, type Probe } from './ProbeFields';
type ContainerForm = { ports?: {name: string; containerPort: number; protocol?: string}[]; securityContext?: SecurityContext; role?: 'app' | 'init' | 'sidecar'; initOrder?: number; workingDir?: string; readinessProbe?: Probe; livenessProbe?: Probe; startupProbe?: Probe; command: string[]; args: string[]; name: string; image: string; cpuRequest: string; memoryRequest: string; cpuLimit: string; memoryLimit: string; bindings: Binding[] };
type ServicePort = { protocol?: string; name: string; port: string; targetPort: string };
// workloadClass is Workload metadata.class (ADR-016): empty means the default class, an explicit value is kept as written.
type Form = { annotations: {name: string; value: string}[]; execution?: Execution; replicas?: number; name: string; workloadClass: string; containers: ContainerForm[]; dependencies: Dependency[]; servicePorts: ServicePort[] };

const emptyContainer = (): ContainerForm => ({ command: [], args: [], name: 'main', image: '', cpuRequest: '', memoryRequest: '', cpuLimit: '', memoryLimit: '', bindings: [] });
const emptyForm = (): Form => ({ annotations: [], name: '', workloadClass: '', containers: [emptyContainer()], dependencies: [], servicePorts: [] });

function needsScoreEditor(raw: Record<string, unknown>, types: ResourceType[]): boolean {
  if (Object.keys(raw).some((key) => !['apiVersion', 'metadata', 'containers', 'resources', 'service', 'workloadExecution', 'replicas'].includes(key))) return true;
  if (Object.keys((raw.metadata ?? {}) as Record<string, unknown>).some((key) => key !== 'name' && key !== 'class' && key !== 'annotations')) return true;
  const declaredClass = (raw.metadata as { class?: unknown } | undefined)?.class;
  if (declaredClass !== undefined && (typeof declaredClass !== 'string' || !validWorkloadClass(declaredClass))) return true;
  const containers = (raw.containers ?? {}) as Record<string, Record<string, unknown>>;
  if (Object.values(containers).some((container) => Object.keys(container).some((key) => !['image', 'variables', 'resources', 'command', 'args', 'workingDir', 'readinessProbe', 'livenessProbe', 'startupProbe', 'role', 'initOrder', 'ports', 'securityContext'].includes(key)))) return true;
  if (Object.values(containers).some((container) => Object.keys((container.resources ?? {}) as Record<string, unknown>).some((key) => !['requests', 'limits'].includes(key)))) return true;
  if (Object.values(containers).some((container) => Object.values((container.resources ?? {}) as Record<string, Record<string, unknown>>).some((quantity) => Object.keys(quantity).some((key) => !['cpu', 'memory'].includes(key))))) return true;
  const resources = (raw.resources ?? {}) as Record<string, Record<string, unknown>>;
  if (Object.values(resources).some((resource) => (resource.id != null && typeof resource.id !== 'string') || (resource.type !== 'service' && resource.params != null && Object.values(resource.params as Record<string, unknown>).some((value) => value === null || typeof value === 'object')))) return true;
  if (Object.values(resources).some((resource) => {
    if (resource.type === 'service' || resource.type === 'environment') return false;
    const contract = types.find((item) => item.key === resource.type);
    if (!contract) return true;
    return Object.entries((resource.params ?? {}) as Record<string, unknown>).some(([name, value]) => {
      const input = contract.inputs?.find((item) => item.name === name);
      return !input || typeof value !== (input.type === 'number' ? 'number' : input.type === 'bool' ? 'boolean' : 'string');
    });
  })) return true;
  if (Object.values(resources).some((resource) => Object.keys(resource).some((key) => !['type', 'class', 'params', 'id'].includes(key)))) return true;
  if (Object.values(resources).some((resource) => resource.type === 'environment' && Object.keys(resource).length !== 1)) return true;
  // A variable the table cannot hold unchanged (text mixed with a reference, a malformed reference) or a virtual
  // resource no variable uses would be rewritten or dropped by the form: keep the advanced Workload path.
  const referenced = new Set<string>();
 if ((raw.workloadExecution as Execution | undefined)?.secretFiles?.length) referenced.add('env');
  for (const container of Object.values(containers)) {
    for (const [name, raw] of Object.entries((container.variables ?? {}) as Record<string, unknown>)) {
      const binding = readBinding(name, raw, resources as ResourceSpecs);
      if (!binding) return true;
      if (binding.source === 'KEY') referenced.add('env');
      if (binding.source === 'SERVICE') referenced.add(binding.alias);
    }
  }
  if (Object.entries(resources).some(([alias, resource]) => (resource.type === 'service' || resource.type === 'environment') && !referenced.has(alias))) return true;
  if (Object.values(resources).some((resource) => resource.type === 'service' && (resource.class != null || Object.keys((resource.params ?? {}) as Record<string, unknown>).some((key) => !['workload', 'port'].includes(key))))) return true;
  if (raw.service && Object.keys(raw.service as Record<string, unknown>).some((key) => key !== 'ports')) return true;
  const ports = ((raw.service as { ports?: Record<string, Record<string, unknown>> } | undefined)?.ports ?? {});
  return Object.values(ports).some((port) => Object.keys(port).some((key) => !['port', 'targetPort', 'protocol'].includes(key)));
}

// Workload class identifier: lowercase, no dot or space, at most 63 characters (backend resource.ValidClass).
export const validWorkloadClass = (value: string): boolean => /^[a-z0-9][a-z0-9_-]{0,62}$/.test(value);

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
      resources.set(dependency.alias, { type: dependency.type, ...(dependency.className ? { class: dependency.className } : {}), ...(dependency.sharedId ? { id: dependency.sharedId } : {}), ...(params.length ? { params: Object.fromEntries(params) } : {}) });
    }
  }
  for (const container of form.containers) {
    const variables: [string, string][] = [];
    const names = new Set<string>();
    for (const binding of container.bindings) {
      // bindingProblem runs first; this guard only refuses a silent overwrite.
      if (names.has(binding.name)) throw new Error(t('workloads.workloadEditor.containerNameNameName2Is', { name: container.name, name2: binding.name }));
      names.add(binding.name);
      // Only the payload of the current source is serialized; a changed source left nothing behind.
      if (binding.source === 'LITERAL') { variables.push([binding.name, escapeLiteral(binding.value)]); continue; }
      let alias = binding.alias;
      let output = binding.output;
      if (isKeySource(binding.source)) { alias = 'env'; output = binding.key; resources.set('env', { type: 'environment' }); }
      if (binding.source === 'SERVICE') { alias = binding.alias || serviceAlias(binding.workload, binding.port); output = 'url'; resources.set(alias, { type: 'service', params: { workload: binding.workload, port: binding.port } }); }
      variables.push([binding.name, `\${resources.${alias}.${output}}`]);
    }
    const requests: Record<string, string> = {};
    const limits: Record<string, string> = {};
    if (container.cpuRequest) requests.cpu = container.cpuRequest;
    if (container.memoryRequest) requests.memory = container.memoryRequest;
    if (container.cpuLimit) limits.cpu = container.cpuLimit;
    if (container.memoryLimit) limits.memory = container.memoryLimit;
    containers.set(container.name, { ...(container.securityContext ? {securityContext:container.securityContext} : {}), ...(container.ports?.length ? {ports: container.ports} : {}), ...(container.role ? {role: container.role} : {}), ...(container.initOrder !== undefined ? {initOrder: container.initOrder} : {}), ...(container.workingDir ? { workingDir: container.workingDir } : {}), ...(container.readinessProbe ? { readinessProbe: container.readinessProbe } : {}), ...(container.livenessProbe ? { livenessProbe: container.livenessProbe } : {}), ...(container.startupProbe ? { startupProbe: container.startupProbe } : {}), ...(container.command.length ? { command: container.command } : {}), ...(container.args.length ? { args: container.args } : {}), ...(container.image ? { image: container.image } : {}), ...(variables.length ? { variables: Object.fromEntries(variables) } : {}), ...(Object.keys(requests).length || Object.keys(limits).length ? { resources: { ...(Object.keys(requests).length ? { requests } : {}), ...(Object.keys(limits).length ? { limits } : {}) } } : {}) });
  }
  if (form.execution?.secretFiles?.length) resources.set('env', { type: 'environment' });
  const ports = new Map<string, unknown>();
  for (const port of form.servicePorts) if (port.name && Number(port.port) > 0) ports.set(port.name, { ...(port.protocol ? {protocol: port.protocol} : {}), port: Number(port.port), targetPort: Number(port.targetPort || port.port) });
  return { ...(form.execution ? { workloadExecution: form.execution } : {}), ...(form.replicas !== undefined ? { replicas: form.replicas } : {}), apiVersion: 'orchestrator.dev/workload/v1', metadata: { name: form.name, ...(form.annotations.length ? { annotations: Object.fromEntries(form.annotations.map(a => [a.name, a.value])) } : {}), ...(form.workloadClass ? { class: form.workloadClass } : {}) }, containers: Object.fromEntries(containers), ...(ports.size ? { service: { ports: Object.fromEntries(ports) } } : {}), ...(resources.size ? { resources: Object.fromEntries(resources) } : {}) };
}


// Effective request: explicit value, else the same-dimension limit, else the
// current default. Display only; the Workload keeps exactly what the user entered.
function effectiveValue(resolution:Resolution|undefined,path:string){const value=fieldValue(resolution,path);return value===undefined?wd('notConfigured'):String(value)}
function inheritedMounts(resolution: Resolution | undefined, container: string): string[] {
  const mounts = fieldValue(resolution, 'workloadExecution.volumeMounts');
  return Array.isArray(mounts) ? mounts.filter((m: {container?: string}) => m.container === container).map((m: {volume: string; path: string}) => `${m.volume} → ${m.path}`) : [];
}
function inheritedPorts(resolution: Resolution | undefined, container: string): string {
  const ports = fieldValue(resolution, `containers.${container}.ports`);
  return Array.isArray(ports) ? ports.map((p: {name: string; containerPort: number; protocol?: string}) => `${p.name}: ${p.containerPort}/${p.protocol ?? 'TCP'}`).join(', ') : '';
}

const validPortNumber = (value: string): boolean => /^\d+$/.test(value.trim()) && Number(value) >= 1 && Number(value) <= 65535;

// Names of Service ports a public route may target: nonblank, unique among all
// rows, and with an integer port in 1..65535.
function validPortNames(ports: ServicePort[]): Set<string> {
  const counts = new Map<string, number>();
  for (const port of ports) if (port.name.trim()) counts.set(port.name, (counts.get(port.name) ?? 0) + 1);
  return new Set(ports.filter((port) => port.name.trim() && counts.get(port.name) === 1 && validPortNumber(port.port)).map((port) => port.name));
}
function readForm(workload: Record<string, unknown>): Form {
  const form = emptyForm();
  form.execution = workload.workloadExecution as Execution | undefined;
  form.annotations = Object.entries((workload.metadata as { annotations?: Record<string,string> })?.annotations ?? {}).map(([name,value]) => ({name,value}));
  form.replicas = workload.replicas as number | undefined;
  form.name = String((workload.metadata as { name?: string } | undefined)?.name ?? '');
  form.workloadClass = String((workload.metadata as { class?: string } | undefined)?.class ?? '');
  const resources = (workload.resources ?? {}) as Record<string, { type?: string; class?: string; id?: unknown; params?: Record<string, unknown> }>;
  form.dependencies = Object.entries(resources).filter(([, spec]) => spec.type !== 'environment' && spec.type !== 'service').map(([alias, spec]) => ({ existing: true, alias, type: spec.type ?? '', className: spec.class ?? '', sharedId: typeof spec.id === 'string' ? spec.id : '', params: Object.fromEntries(Object.entries(spec.params ?? {}).map(([key, value]) => [key, String(value)])) }));
  const rawContainers = (workload.containers ?? {}) as Record<string, { ports?: {name: string; containerPort: number; protocol?: string}[]; securityContext?: SecurityContext; role?: 'app' | 'init' | 'sidecar'; initOrder?: number; workingDir?: string; readinessProbe?: Probe; livenessProbe?: Probe; startupProbe?: Probe; command?: string[]; args?: string[]; image?: string; variables?: Record<string, string>; resources?: { requests?: { cpu?: string; memory?: string }; limits?: { cpu?: string; memory?: string } } }>;
  form.containers = Object.entries(rawContainers).map(([name, value]) => ({ securityContext: value.securityContext, ports: value.ports, role: value.role, initOrder: value.initOrder, workingDir: value.workingDir, readinessProbe: value.readinessProbe, livenessProbe: value.livenessProbe, startupProbe: value.startupProbe, command: value.command ?? [], args: value.args ?? [], name, image: value.image ?? '', cpuRequest: value.resources?.requests?.cpu ?? '', memoryRequest: value.resources?.requests?.memory ?? '', cpuLimit: value.resources?.limits?.cpu ?? '', memoryLimit: value.resources?.limits?.memory ?? '', bindings: Object.entries(value.variables ?? {}).flatMap(([envName, raw]) => readBinding(envName, raw, resources as ResourceSpecs) ?? []) }));
  if (!form.containers.length) form.containers = [emptyContainer()];
  const ports = ((workload.service as { ports?: Record<string, { port: number; targetPort?: number; protocol?: string }> } | undefined)?.ports ?? {});
  form.servicePorts = Object.entries(ports).map(([name, port]) => ({ protocol: port.protocol, name, port: String(port.port), targetPort: String(port.targetPort ?? port.port) }));
  return form;
}

// `service.publicPort` was removed; a draft that still has it is never opened
// as a form because the route it declared would be dropped or rewritten.
function hasRemovedPublicPort(workload: Record<string, unknown>): boolean {
  const service = workload.service;
  return typeof service === 'object' && service !== null && Object.prototype.hasOwnProperty.call(service, 'publicPort');
}

const removedPublicPortMessage = t('workloads.workloadEditor.theDraftScoreStillDeclares');

const readyCatalog = (config: Configuration): KeyCatalog => ({ status: 'ready', keys: config.keys ?? [], version: config.version, secretStoreKey: config.secretStoreKey });

export function WorkloadEditorPage({ application, environment, workloadId, selectedTemplate, onTargetChange }: { application: Application; environment: EnvironmentKey; workloadId?: string; selectedTemplate?: string; onTargetChange?(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void }) {
  // Environment operations hold draft writes: Save stays disabled, text is kept.
  const environmentBusy = useEnvironmentBusy(application, environment, onTargetChange);
  const [resolution,setResolution]=useState<Resolution>();
  const [resolutionProblem,setResolutionProblem]=useState('');
  const [resolutionState,setResolutionState]=useState<'loading'|'ready'|'error'|'incomplete'>('loading');
  const [resolutionAttempt,setResolutionAttempt]=useState(0);
  const [templateMetadata, setTemplateMetadata] = useState<WorkloadTemplate>();
  const [templateInputProblems, setTemplateInputProblems] = useState(0);
  const [form, setForm] = useState<Form>(emptyForm);
  const [catalog, setCatalog] = useState<KeyCatalog>({ status: 'loading' });
  const [catalogAttempt, setCatalogAttempt] = useState(0);
  const [types, setTypes] = useState<ResourceType[]>([]);
  const [workloadClasses, setWorkloadClasses] = useState<string[]>([]);
  // Class choices come from GET /resource-types; while they reload or fail the class controls lock, edits stay and nothing is guessed.
  const [classMetadata, setClassMetadata] = useState<ClassMetadata>('ready');
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
  const [current, setCurrent] = useState<{ version: number; workload?: Record<string, unknown>; state?: string }>();
  const [reloadError, setReloadError] = useState('');
  // Inline creation (UC-16 BR-21/22): one dialog at a time, bound to the row (stable id) that opened it.
  const [dialog, setDialog] = useState<{ type: 'key'; kind: KeyKind; rowId: string; suggested: string; file?: boolean; container?: string; mount?: {path: string; mode: number} } | { type: 'resource'; rowId: string } | { type: 'storage'; initial: Dependency; container: string; mount: {path: string; subPath: string; readOnly: boolean} }>();
  const [focusRequest, setFocusRequest] = useState<FocusRequest>();
  // After a failed Save the table also reports rows that are not filled in yet.
  const [showIncomplete, setShowIncomplete] = useState(false);
  // Scope/unmount guard for import, Save and reload: a late reply for another
  // scope or after leaving the page is dropped.
  const scope = useRef(0);
  const metadataRequest = useRef(0);
  useEffect(() => () => { scope.current += 1; }, []);

  // A different Application, Environment or workload starts from its own
  // empty/draft state; nothing selected, imported, saving or reloading carries
  // over (a pending reload's finally ignores the new scope by design).
  useEffect(() => {
    scope.current += 1;
    setWorkloadClasses([]);
    setTemplateMetadata(undefined); setTemplateInputProblems(0); setForm(emptyForm()); setImported(undefined); setDirty(false); setMode('form'); setAdvancedScore(false); setBlocked(false); setSaving(false); setReloading(false); setWorkloads([]); setDialog(undefined); setFocusRequest(undefined); setShowIncomplete(false);
  }, [application.id, environment, workloadId, selectedTemplate]);

  useEffect(() => {
    let cancelled = false;
    setStale(false); setCurrent(undefined); setReloadError('');
    setLoading(true); setLoadFailed(false); setError('');
    Promise.all([getWorkloads(application.id, environment), getResourceTypes()]).then(async ([list, catalog]) => {
      if (cancelled) return;
      setWorkloads(list.workloads); setTypes(catalog.resourceTypes.filter((type) => type.key !== 'workload')); setWorkloadClasses((catalog.resourceTypes.find(type => type.key === 'workload')?.classes ?? []).filter(validWorkloadClass)); setClassMetadata('ready'); setVersion(list.draftVersion);
      const existing = list.workloads.find((item) => item.id === workloadId);
      if (existing?.workload && hasRemovedPublicPort(existing.workload)) { setForm({ ...emptyForm(), name: workloadId ?? '' }); setBlocked(true); setError(removedPublicPortMessage); }
      else if (existing?.workload) { setForm(readForm(existing.workload)); setImported(existing.workload); setBlocked(false); const advanced = needsScoreEditor(existing.workload, catalog.resourceTypes); setAdvancedScore(advanced); if (advanced) setMode('import'); }
      else if (selectedTemplate) {
        const templates = await getWorkloadTemplates(application.id, environment); if (cancelled) return;
        const chosen = templates.find(item => templateKey(item) === selectedTemplate);
        if (!chosen) { setBlocked(true); setError(t('workloads.templates.templateMissing')); return; }
        setTemplateMetadata(chosen);
        setForm({ ...emptyForm(), execution: { version: 1, kind: chosen.kind, template: { id: chosen.id, version: chosen.version, sha256: chosen.sha256 }, inputs: chosen.defaults, ...(chosen.kind === 'CronJob' ? { schedule: { expression: '', timeZone: '', concurrencyPolicy: '', suspend: false } } : {}) } });
      }
      else if (!workloadId) {setForm({...emptyForm(),execution:customExecution()});}
      else if (workloadId) { setForm({ ...emptyForm(), name: workloadId }); setBlocked(true); setError(existing ? t('workloads.workloadEditor.thisDeployedWorkloadHasNo') : t('workloads.workloadEditor.workloadNotFoundInThis')); }
    }).catch((err: unknown) => { if (!cancelled) { setError(describeError(err, t('workloads.workloadEditor.couldNotLoadTheWorkload'))); setLoadFailed(true); setClassMetadata('error'); } }).finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [application.id, environment, workloadId, selectedTemplate, loadAttempt]);

  const templateImage = form.execution?.templateDefaults?.profile.image ?? templateMetadata?.runtimeDefaults?.environments?.[environment]?.image ?? templateMetadata?.runtimeDefaults?.image;
  const missingTemplateImage = Boolean(form.execution?.template) && form.containers.some(container => !container.image && !((!container.role || container.role === 'app') && typeof templateImage === 'string' && templateImage.length > 0));

  // Resolution only changes display; no effective field is copied into desired overrides.
  useEffect(()=>{
    if(loading)return;
    if(missingTemplateImage){setResolution(undefined);setResolutionState('incomplete');return;}
    let cancelled=false;
    setResolutionProblem('');
    setResolutionState('loading');
    const timer=window.setTimeout(()=>{
      let document: Record<string,unknown>;
      try {document=buildScore({...form,name:form.name||'pending-workload',containers:form.containers.map(c=>({...c,name:c.name||'pending-container',image:c.image||(form.execution?.template?'':'pending.invalid/image')}))},types)}catch{setResolutionState('error');return}
      // Empty Cron schedule is an unfinished field, not a default schedule.
      const execution=document.workloadExecution as Execution|undefined;
      if(execution?.schedule&&!execution.schedule.expression)document={...document,workloadExecution:{...execution,schedule:{...execution.schedule,expression:'0 0 * * *'}}};
      api<Resolution>(`/applications/${encodeURIComponent(application.id)}/environments/${environment}/workloads/resolve-defaults`,{method:'POST',body:JSON.stringify({workload:document})})
       .then(result=>{if(!result.workload||!result.sources)throw new Error('Invalid resolution');if(!cancelled){setResolution(result);setResolutionState('ready')}})
       .catch((reason:unknown)=>{
         if(cancelled)return;
         setResolution(undefined);
         const profile=templateMetadata?.runtimeDefaults;
         const replicas=form.replicas??form.execution?.templateDefaults?.profile.replicas??profile?.environments?.[environment]?.replicas??profile?.replicas;
         const invalidRequest=reason instanceof ApiError&&reason.status===400;
         const pdbProblem=invalidRequest&&form.execution?.podDisruptionBudget?.enabled&&typeof replicas==='number'&&replicas<2;
         setResolutionProblem(pdbProblem?wd('pdbHelp'):reason instanceof ApiError&&reason.status<500?describeError(reason,wd('resolutionError')):wd('resolutionError'));
         setResolutionState('error');
       });
    },250);
    return ()=>{cancelled=true;window.clearTimeout(timer)};
  },[application.id,environment,form,types,loading,resolutionAttempt,missingTemplateImage,templateMetadata]);

  // The Application key catalog loads on its own so a failure is shown as a
  // retryable state in the picker, never as an empty key list.
  useEffect(() => {
    let cancelled = false;
    setCatalog({ status: 'loading' });
    getConfiguration(application.id, environment).then((config) => { if (!cancelled) setCatalog(readyCatalog(config)); })
      .catch((err: unknown) => { if (!cancelled) setCatalog({ status: 'error', message: describeError(err, t('workloads.workloadEditor.noResponseFromTheServer')) }); });
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
      setCatalog(readyCatalog(config)); setWorkloads(list.workloads); setVersion(list.draftVersion);
      setCurrent({ version: list.draftVersion, workload: item?.workload, state: item?.state || (item ? 'deployed' : 'absent') });
      setStale(false); setError('');
    } catch (err) {
      if (at === scope.current) setReloadError(t('workloads.workloadEditor.couldNotReloadTheCurrent', { describeError: describeError(err, t('workloads.workloadEditor.noResponseFromTheServer2')) }));
    } finally {
      if (at === scope.current) setReloading(false);
    }
  }

  // Explicit retry/refresh of the class metadata only: it never touches the form or saves anything.
  // A late older reply never replaces a newer one or another scope's state.
  async function reloadClassMetadata() {
    const at = scope.current;
    const request = ++metadataRequest.current;
    setClassMetadata('loading');
    try {
      const catalog = await getResourceTypes();
      if (at !== scope.current || request !== metadataRequest.current) return;
      setTypes(catalog.resourceTypes.filter((type) => type.key !== 'workload')); setWorkloadClasses((catalog.resourceTypes.find(type => type.key === 'workload')?.classes ?? []).filter(validWorkloadClass)); setClassMetadata('ready');
    } catch {
      if (at === scope.current && request === metadataRequest.current) setClassMetadata('error');
    }
  }

  function updateForm(next: Form) { setForm(next); setDirty(true); }
  function updateContainer(index: number, next: ContainerForm) { const containers = [...form.containers]; containers[index] = next; updateForm({ ...form, containers }); }
  // Applies a change to one row by its stable id, wherever that row now is. A row that is gone changes nothing.
  const changeRow = useCallback((rowId: string, change: (row: Binding) => Binding) => {
    setForm((prev) => {
      if (!prev.containers.some((container) => container.bindings.some((row) => row.id === rowId))) return prev;
      return { ...prev, containers: prev.containers.map((container) => ({ ...container, bindings: container.bindings.map((row) => (row.id === rowId ? change(row) : row)) })) };
    });
    setDirty(true);
  }, []);
  const clearFocus = useCallback(() => setFocusRequest(undefined), []);
  function openKeyDialog(kind: KeyKind, row: Binding) { setDialog({ type: 'key', kind, rowId: row.id, suggested: row.name }); }
  function closeDialog() { if (dialog && dialog.type !== 'storage' && dialog.rowId) setFocusRequest({ rowId: dialog.rowId, field: 'key' }); setDialog(undefined); }
  // A configuration reply of the scope the dialog opened in refreshes the catalog; any other scope drops it.
  function acceptConfiguration(at: number) { return (config: Configuration) => { if (at === scope.current) setCatalog(readyCatalog(config)); }; }
  function updateDependency(index: number, patch: Partial<Dependency>) {
    const previous = form.dependencies[index];
    const changed = patch.type !== undefined && patch.type !== previous?.type;
    updateForm({ ...form, dependencies: form.dependencies.map((item, i) => i === index ? { ...item, ...patch } : item),
      containers: form.containers.map((container) => ({ ...container, bindings: container.bindings.map((row) => changed && row.source === 'RESOURCE' && row.alias === previous?.alias ? { ...row, resourceChanged: true } : row) })) });
  }
  function updatePort(index: number, patch: Partial<ServicePort>) { updateForm({ ...form, servicePorts: form.servicePorts.map((item, i) => i === index ? { ...item, ...patch } : item) }); }
  async function importFile(file: File) {
    const at = scope.current;
    setError('');
    try {
      const content = await file.text();
      if (at !== scope.current) return;
      const result = await parseWorkloadImport(application.id, environment, content);
      if (at !== scope.current) return;
      setImported(result.workload); setForm(readForm(result.workload)); setAdvancedScore(needsScoreEditor(result.workload, types)); setDirty(false);
    }
    catch (err) { if (at === scope.current) setError(describeError(err, t('workloads.workloadEditor.couldNotImportTheScore'))); }
  }
  const resourceChoices = sharedResources(workloads, types);
  const storageTypes = types.filter(type => ['claimName', 'namespace'].every(name => (type.outputs ?? []).some(field => field.name === name && field.type === 'string' && !field.secret)));
  const storageKeys = new Set(storageTypes.map(type => type.key));
  function openStorageResource(container: string, mount: {path: string; subPath: string; readOnly: boolean}) {
    let alias = 'data';
    for (let suffix = 2; form.dependencies.some(dependency => dependency.alias === alias); suffix += 1) alias = `data-${suffix}`;
    setDialog({ type: 'storage', container, mount, initial: { ...emptyDependency(), alias, type: storageTypes.length === 1 ? storageTypes[0]!.key : '' } });
  }
  const reusableStorage = resourceChoices.filter(choice => choice.dependency && storageKeys.has(choice.dependency.type) && !form.dependencies.some(dep => dep.sharedId === choice.id));
  function attachSharedStorage(id: string, container: string, mount: {path:string;subPath:string;readOnly:boolean}) {
    const selected = reusableStorage.find(choice => choice.id === id)?.dependency;
    if (!selected) return;
    setForm(prev => {
      if (!prev.containers.some(c => c.name === container)) return prev;
      const existing=prev.dependencies.find(dep => dep.sharedId === selected.sharedId && dep.type === selected.type);
      let alias=existing?.alias ?? selected.alias;
      if (!existing) {const base=alias;for(let i=2;prev.dependencies.some(dep => dep.alias === alias);i++)alias=`${base}-${i}`;}
      const dependency={...selected,alias,reuseId:id,params:{...selected.params}};
      return {...prev,dependencies:existing ? prev.dependencies : [...prev.dependencies,dependency],execution:{...(prev.execution ?? customExecution()),mounts:[...(prev.execution?.mounts ?? []),{resource:alias,container,path:mount.path,readOnly:mount.readOnly,...(mount.subPath ? {subPath:mount.subPath} : {})}]}};
    });
    setDirty(true);
  }
  const rowContext = buildRowContext(catalog, form.dependencies, types, workloads, form.name);
  async function save() {
    if (environmentBusy) return;
    if (blocked || stale || templateInputProblems > 0) return;
    if (form.execution?.mounts?.some(mount => !form.dependencies.some(dep => dep.alias === mount.resource) || !form.containers.some(c => c.name === mount.container) || !mount.path.startsWith('/'))) { setError(t('workloads.templates.mountInvalid')); return; }
    if (mode === 'form' || dirty) {
      if(missingTemplateImage){setError(wd('imageRequiredForDefaults'));return;}
      if (new Set(form.annotations.map(a => a.name)).size !== form.annotations.length || form.annotations.some(a => !a.name)) { setError(t('workloads.volumes.invalid')); return; }
      if (!validRuntimeIntent(form.containers.map(c=>({...c,image:c.image||String(fieldValue(resolution,`containers.${c.name}.image`)??'')})), form.execution, catalog.status === 'ready' ? catalog.keys.filter(k => k.kind === 'SECRET' && k.configured).map(k => k.name) : undefined, Boolean(form.execution?.template))) { setError(t('workloads.volumes.invalid')); return; }
      if (form.workloadClass && !validWorkloadClass(form.workloadClass)) { setError(t('workloads.workloadEditor.invalidWorkloadClassUseOnly')); return; }
      if (classMetadata !== 'ready' && form.dependencies.length > 0) { setError(classMetadata === 'loading' ? t('workloads.workloadEditor.classMetadataIsLoadingWait') : t('workloads.workloadEditor.theResourceClassMetadataIs')); return; }
      const aliasesIssue = dependencyAliasesProblem(form.dependencies, form.containers.flatMap((container) => container.bindings.filter((row) => row.source === 'SERVICE').map((row) => row.alias || serviceAlias(row.workload, row.port))));
      if (aliasesIssue) { setError(aliasesIssue); return; }
      for (const dependency of form.dependencies) {
        if (dependency.type === 'endpoint' && (form.dependencies.filter(d => d.type === 'endpoint').length > 1 || form.execution?.serviceEnabled === false || !endpointPorts(form.servicePorts).some(p => p.name === dependency.params.port))) { setError(t('workloads.endpoint.invalidPort')); return; }
        const dependencyIssue = sharedSelectionProblem(dependency, resourceChoices) || dependencyProblem(dependency, types);
        if (dependencyIssue) { setError(dependencyIssue); return; }
      }
      // The key catalog is only needed when a row references an Application key.
      const usesKeys = !!form.execution?.secretFiles?.length || form.containers.some((container) => container.bindings.some((row) => isKeySource(row.source)));
      if (usesKeys && catalog.status !== 'ready') { setError(catalog.status === 'loading' ? t('workloads.workloadEditor.applicationVariablesAndSecretsAre') : t('workloads.workloadEditor.couldNotLoadApplicationVariables')); return; }
      const problem = bindingProblem(form.containers, rowContext);
      if (problem) { setError(problem); setShowIncomplete(true); return; }
    }
    const workload = mode === 'import' && imported && !dirty ? imported : !dirty && imported ? imported : buildScore(form, types);
    const id = String((workload.metadata as { name?: string } | undefined)?.name ?? '');
    if (!id) { setError(t('workloads.workloadEditor.enterAWorkloadName')); return; }
    const at = scope.current;
    setSaving(true); setError('');
    try { if ((workload.workloadExecution as Execution|undefined)?.template) {const resolved=await api<Resolution>(`/applications/${encodeURIComponent(application.id)}/environments/${environment}/workloads/resolve-defaults`,{method:'POST',body:JSON.stringify({workload})});if(!resolved.workload||!resolved.sources)throw new Error(wd('resolutionError'));if(at!==scope.current)return;} await saveWorkload(application.id, environment, id, workload, version); if (at === scope.current) navigate({ name: 'application', applicationId: application.id, environment }); }
    catch (err) {
      if (at !== scope.current) return;
      if (isBusy(err)) setError((err as ApiError).message);
      else if (isStaleConflict(err)) { setStale(true); setCurrent(undefined); setReloadError(''); }
      else setError(err instanceof ApiError ? err.message : t('workloads.workloadEditor.noResponseFromTheServer3'));
    }
    finally { if (at === scope.current) setSaving(false); }
  }
  const currentState = current?.state === 'absent' ? t('workloads.workloadEditor.none') : current?.state === 'deployed' ? t('workloads.workloadEditor.deployedNoPendingChanges') : current?.state === 'PENDING_DELETE' ? t('workloads.workloadEditor.pendingDeletion') : t('workloads.workloadEditor.hasPendingChanges');

  return <section className="page workload-editor">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id, environment })}>← {application.name} / {environment}</button>
    <header className="page-header application-header"><div><p className="eyebrow">{t('workloads.workloadEditor.environmentWorkloadConfiguration', { environment })}</p><h1>{workloadId ? t('workloads.workloadEditor.editWorkloadId', { workloadId }) : t('workloads.workloadEditor.addWorkload')}</h1><p>{t('workloads.workloadEditor.saveTheDraftFirstPreview')}</p></div><Button onClick={() => navigate({ name: 'settings', applicationId: application.id, environment })}>{t('workloads.workloadEditor.variablesSecrets')}</Button></header>
    {authoringFeatures.workloadImport ? <div className="tabs" role="tablist"><button className={mode === 'form' ? 'tab tab-active' : 'tab'} disabled={advancedScore} onClick={() => setMode('form')}>{t('workloads.workloadEditor.formInput')}</button><button className={mode === 'import' ? 'tab tab-active' : 'tab'} onClick={() => setMode('import')}>{t('workloads.workloadEditor.importScore')}</button></div> : null}
    {advancedScore ? <p className="feature-note">{t(authoringFeatures.workloadImport ? 'workloads.workloadEditor.thisScoreHasFieldsThe' : 'workloads.workloadEditor.advancedReadOnly')}</p> : null}
    {error ? <div className="form-error" role="alert">{error}{loadFailed ? <> <Button onClick={() => setLoadAttempt((value) => value + 1)}>{t('workloads.workloadEditor.retry')}</Button></> : null}</div> : null}
    {stale ? <div className="form-error" role="alert" aria-label={t('workloads.workloadEditor.workloadDraftIsStale')}>{t('workloads.workloadEditor.workloadsIn')}{' '} {environment} {' '}{t('workloads.workloadEditor.changedAfterThisPageWas')}{' '} <Button disabled={reloading} onClick={() => void reloadCurrent()}>{reloading ? t('workloads.workloadEditor.reloading') : t('workloads.workloadEditor.reloadCurrentState')}</Button>{reloadError ? <p>{reloadError}</p> : null}</div> : null}
    {current ? <div className="form-info" role="status">{t('workloads.workloadEditor.reloadedDraftVersion')}{' '} {current.version}{t('workloads.workloadEditor.currentStateOfThisWorkload')}{' '} {currentState}{t('workloads.workloadEditor.theEditsBelowAreUnchanged')}{current.workload ? <details><summary>{t('workloads.workloadEditor.currentScore')}</summary><pre className="workload-preview">{JSON.stringify(current.workload, null, 2)}</pre></details> : null}</div> : null}
    {loading ? <p>{t('workloads.workloadEditor.loadingWorkloadOptions')}</p> : mode === 'import' ? <section className="content-panel">{authoringFeatures.workloadImport ? <><h2>{t('workloads.workloadEditor.importAScoreFile')}</h2><p>{t('workloads.workloadEditor.yAMLOrJSONTheSame')}</p><input aria-label={t('workloads.workloadEditor.scoreFile')} type="file" accept=".yaml,.yml,.json,text/yaml,application/json" onChange={(event) => { const file = event.target.files?.[0]; if (file) void importFile(file); }} /></> : null}{imported ? <><p>{t('workloads.workloadEditor.parsedWorkload')}{' '} <strong>{String((imported.metadata as { name?: string } | undefined)?.name ?? '')}</strong></p><pre className="workload-preview">{JSON.stringify(imported, null, 2)}</pre></> : null}</section> : <>
      <section className="content-panel editor-grid"><h2>{t('workloads.workloadEditor.basicInformation')}</h2><label>{t('workloads.workloadEditor.workloadName')}<input value={form.name} disabled={Boolean(workloadId)} onChange={(event) => updateForm({ ...form, name: event.target.value })} placeholder="frontend" /></label>
      {resolutionState==='error'?<div role="alert" className="form-error">{resolutionProblem||wd('resolutionError')}<Button onClick={()=>setResolutionAttempt(v=>v+1)}>{t('workloads.workloadEditor.retry')}</Button></div>:resolutionState==='incomplete'?<p role="status">{wd('imageRequiredForDefaults')}</p>:resolutionState==='loading'?<p role="status">{wd('resolving')}</p>:null}
</section>
      {catalog.status === 'error' ? <div className="form-error" role="alert">{t('workloads.workloadEditor.couldNotLoadApplicationVariables2')}{' '} {catalog.message} {' '}{t('workloads.workloadEditor.yourSelectionsAreKept')}{' '} <Button onClick={() => setCatalogAttempt((value) => value + 1)}>{t('workloads.workloadEditor.retryLoadingKeys')}</Button></div> : null}
      {form.containers.map((container, ci) => <section className="content-panel editor-grid" key={ci}><div className="section-header"><h2>Container {ci + 1}</h2>{form.containers.length > 1 ? <Button tone="danger" onClick={() => updateForm({ ...form, containers: form.containers.filter((_, index) => index !== ci) })}>{t('workloads.workloadEditor.remove')}</Button> : null}</div><div className="field-grid"><label>{t('workloads.workloadEditor.containerName')}<input value={container.name} onChange={(event) => updateContainer(ci, { ...container, name: event.target.value })} /></label><label>{t('workloads.workloadEditor.containerImage')}<input value={container.image} onChange={(event) => updateContainer(ci, { ...container, image: event.target.value })} placeholder={String(fieldValue(resolution,`containers.${container.name}.image`)??'')} /></label></div>

        <ConfigGroup title={wd('environment')} initialOpen summary={container.bindings.length} invalid={Boolean(error)}><EnvironmentVariablesTable grouped container={container.name || String(ci + 1)} bindings={container.bindings} catalog={catalog} dependencies={form.dependencies} types={types} workloads={workloads} self={form.name} context={rowContext} disabled={saving} showIncomplete={showIncomplete} focus={focusRequest} onFocused={clearFocus}
          onChange={(bindings) => updateContainer(ci, { ...container, bindings })} onCreateKey={openKeyDialog} onAddResource={(row) => setDialog({ type: 'resource', rowId: row.id })} /></ConfigGroup>
        <AdvancedFields label={`Container ${container.name}`} invalid={Boolean(error)}>
        <ConfigGroup title={wd('compute')} summary={form.execution?.template ? resolutionState==='ready' ? `CPU ${effectiveValue(resolution,`containers.${container.name}.resources.requests.cpu`)} · RAM ${effectiveValue(resolution,`containers.${container.name}.resources.requests.memory`)}` : wd('notConfigured') : `CPU ${container.cpuRequest||container.cpuLimit||wd('notConfigured')} · RAM ${container.memoryRequest||container.memoryLimit||wd('notConfigured')}`} invalid={Boolean(error)}>          <div className="field-grid field-grid-four"><label>{t('workloads.workloadEditor.cPURequest')}<input value={container.cpuRequest} aria-label={t('workloads.workloadEditor.cPURequest')} onChange={(event) => updateContainer(ci, { ...container, cpuRequest: event.target.value })} placeholder={String(fieldValue(resolution,`containers.${container.name}.resources.requests.cpu`)??(!form.execution?.template?container.cpuLimit||'':''))} /></label><label>{t('workloads.workloadEditor.memoryRequest')}<input value={container.memoryRequest} aria-label={t('workloads.workloadEditor.memoryRequest')} onChange={(event) => updateContainer(ci, { ...container, memoryRequest: event.target.value })} placeholder={String(fieldValue(resolution,`containers.${container.name}.resources.requests.memory`)??(!form.execution?.template?container.memoryLimit||'':''))} /></label><label>{t('workloads.workloadEditor.cPULimit')}<input value={container.cpuLimit} aria-label={t('workloads.workloadEditor.cPULimit')} onChange={(event) => updateContainer(ci, { ...container, cpuLimit: event.target.value })} placeholder={String(fieldValue(resolution,`containers.${container.name}.resources.limits.cpu`)??'')} /></label><label>{t('workloads.workloadEditor.memoryLimit')}<input value={container.memoryLimit} aria-label={t('workloads.workloadEditor.memoryLimit')} onChange={(event) => updateContainer(ci, { ...container, memoryLimit: event.target.value })} placeholder={String(fieldValue(resolution,`containers.${container.name}.resources.limits.memory`)??'')} /></label></div>
<Button tone="quiet" onClick={()=>updateContainer(ci,{...container,cpuRequest:'',memoryRequest:'',cpuLimit:'',memoryLimit:''})}>{wd('reset')}</Button></ConfigGroup>
        <ConfigGroup title={wd('volumes')} summary={(form.execution?.volumeMounts?.filter(m=>m.container===container.name).length??inheritedMounts(resolution,container.name).length)+(form.execution?.mounts?.filter(m=>m.container===container.name).length??0)} invalid={Boolean(error)}>{!form.execution?.volumeMounts && inheritedMounts(resolution,container.name).length ? <p className="muted">{inheritedMounts(resolution,container.name).join(', ')}</p> : null}<ContainerVolumeFields grouped value={form.execution ?? customExecution()} onChange={execution => updateForm({...form,execution})} container={container.name} storage={form.dependencies.filter(dep => storageKeys.has(dep.type)).map(dep => dep.alias)} secrets={catalog.status === 'ready' ? catalog.keys.filter(k => k.kind === 'SECRET' && k.configured).map(k => k.name) : []} secretCatalogStatus={catalog.status} storageReady={!loadFailed && classMetadata === 'ready'} storageStatus={loadFailed ? 'error' : classMetadata} sharedStorage={reusableStorage} onUseStorage={(id,mount) => attachSharedStorage(id,container.name,mount)} disabled={saving} onAddStorage={storageTypes.length ? mount => openStorageResource(container.name,mount) : undefined} onCreateSecret={catalog.status === 'ready' ? mount => setDialog({type:'key',kind:'SECRET',rowId:'',suggested:'',file:true,container:container.name,mount}) : undefined} /></ConfigGroup>
        <ConfigGroup title={wd('ports')} summary={container.ports?.length??inheritedPorts(resolution,container.name)} invalid={Boolean(error)}>          <div className="section-header"><h3>{t('workloads.containerPorts.title')}</h3><Button onClick={() => updateContainer(ci, {...container, ports: [...(container.ports ?? []), {name: '', containerPort: 0}]})}>{t('workloads.containerPorts.add')}</Button></div>
          {!container.ports && inheritedPorts(resolution,container.name) ? <p className="muted">{inheritedPorts(resolution,container.name)}</p> : null}
          {(container.ports ?? []).map((port, pi) => <div className="field-grid" key={pi}>
            <label>{t('workloads.containerPorts.name')}<input value={port.name} onChange={e => updateContainer(ci, {...container, ports: container.ports!.map((p,i) => i === pi ? {...p, name: e.target.value} : p)})} /></label>
            <label>{t('workloads.containerPorts.port')}<input type="number" min="1" max="65535" value={port.containerPort||''} onChange={e => updateContainer(ci, {...container, ports: container.ports!.map((p,i) => i === pi ? {...p, containerPort: Number(e.target.value)} : p)})} /></label>
            <label>{t('workloads.containerPorts.protocol')}<select aria-label={t('workloads.containerPorts.protocol')} value={port.protocol ?? 'TCP'} onChange={e => updateContainer(ci, {...container, ports: container.ports!.map((p,i) => i === pi ? {...p, protocol: e.target.value || undefined} : p)})}><option value="TCP">TCP</option><option value="UDP">UDP</option><option value="SCTP">SCTP</option></select></label>
            <Button tone="quiet" onClick={() => updateContainer(ci, {...container, ports: container.ports!.filter((_,i) => i !== pi)})}>{t('workloads.containerPorts.remove')}</Button>
          </div>)}
</ConfigGroup>
        <ConfigGroup title={wd('health')} summary={(['readinessProbe','livenessProbe','startupProbe'] as const).filter(probe=>fieldValue(resolution,`containers.${container.name}.${probe}`)).map(probe=>t(`workloads.templates.${probe}`)).join(', ')} invalid={Boolean(error)}>          {(['readinessProbe', 'livenessProbe', 'startupProbe'] as const).map(probe => <ProbeFields key={probe} label={t(`workloads.templates.${probe}`)} value={container[probe]} inherited={fieldValue(resolution,`containers.${container.name}.${probe}`) as Probe | undefined} disabled={saving} onChange={value => updateContainer(ci, {...container, [probe]: value})} />)}
</ConfigGroup>
        <ConfigGroup title={wd('startup')} summary={t(`workloads.containers.${container.role??'app'}`)} invalid={Boolean(error)}>        <div className="field-grid"><label>{t('workloads.containers.role')}<select aria-label={t('workloads.containers.roleOf', {name: container.name || ci + 1})} value={container.role ?? 'app'} onChange={event => {
          const role = event.target.value as 'app' | 'init' | 'sidecar';
          const order = Math.max(0, ...form.containers.map(c => c.initOrder ?? 0)) + 1;
          updateContainer(ci, { ...container, role, initOrder: role === 'app' ? undefined : container.initOrder ?? order });
        }}>{(['app', 'init', 'sidecar'] as const).map(role => <option key={role} value={role}>{t(`workloads.containers.${role}`)}</option>)}</select></label>
        {container.role === 'init' || container.role === 'sidecar' ? <label>{t('workloads.containers.initOrder')}<input type="number" min="1" max="999" value={container.initOrder ?? ''} onChange={event => updateContainer(ci, { ...container, initOrder: Number(event.target.value) })} /></label> : null}</div>          <div className="field-grid"><label>{t('workloads.templates.command')}<textarea aria-label={t('workloads.templates.command')} value={container.command.join('\n')} onChange={e => updateContainer(ci, { ...container, command: e.target.value === '' ? [] : e.target.value.split('\n') })} /></label><label>{t('workloads.templates.args')}<textarea aria-label={t('workloads.templates.args')} value={container.args.join('\n')} onChange={e => updateContainer(ci, { ...container, args: e.target.value === '' ? [] : e.target.value.split('\n') })} /></label></div>
          <label>{t('workloads.templates.workingDir')}<input value={container.workingDir ?? ''} onChange={e => updateContainer(ci, { ...container, workingDir: e.target.value })} /></label>
</ConfigGroup>
        <ConfigGroup title={wd('containerSecurity')}  invalid={Boolean(error)}><SecurityFields value={container.securityContext} onChange={securityContext=>updateContainer(ci,{...container,securityContext})} resolution={resolution} path={`containers.${container.name}.securityContext`}/></ConfigGroup>
        </AdvancedFields>
        </section>)}
      {form.execution ? <UnassignedVolumeMounts value={form.execution} onChange={execution => updateForm({...form,execution})} containers={form.containers.map(c => c.name)} disabled={saving} /> : null}
      <Button onClick={() => updateForm({ ...form, containers: [...form.containers, { ...emptyContainer(), name: `container-${form.containers.length + 1}` }] })}>{t('workloads.workloadEditor.addContainer')}</Button>

      {form.execution ? <ExecutionFields section="schedule" value={form.execution} onChange={execution => updateForm({ ...form, execution })} metadata={templateMetadata} disabled={saving} onRawProblems={setTemplateInputProblems} resolution={resolution} /> : null}
      <section className="content-panel editor-grid"><ConfigGroup initialOpen title={wd('networking')} summary={form.servicePorts.map(p=>`${p.name}: ${p.port} → ${p.targetPort||p.port}`).join(', ')||wd('notConfigured')} invalid={Boolean(error)}>
        {form.execution && form.servicePorts.length ? <label className="checkbox-row"><input type="checkbox" checked={form.execution.headless ?? false} onChange={e => updateForm({ ...form, execution: { ...form.execution!, headless: e.target.checked } })} />{t('workloads.templates.headless')}</label> : null}
        <label>{wd('service')}<DefaultSelect defaultLabel={fieldValue(resolution,'service')?wd('enabled'):wd('disabled')} aria-label={wd('service')} value={form.execution?.serviceEnabled===undefined?'':String(form.execution.serviceEnabled)} onValueChange={selected=>{const execution={...(form.execution??customExecution())};if(selected==='')delete execution.serviceEnabled;else execution.serviceEnabled=selected==='true';updateForm({...form,execution})}}><option value="true">{wd('enabled')}</option><option value="false">{wd('disabled')}</option></DefaultSelect></label>
        <div className="section-header"><div><h2>{t('workloads.workloadEditor.servicePorts')}</h2><p>{t('workloads.workloadEditor.internalAccessForOtherWorkloads', { environment })}</p></div><Button onClick={() => updateForm({ ...form, servicePorts: [...form.servicePorts, { name: '', port: '', targetPort: '' }] })}>{t('workloads.workloadEditor.addPort')}</Button></div>
        {form.servicePorts.map((port, index) => <div className="editor-grid" key={index}><div className="binding-row"><input aria-label={t('workloads.workloadEditor.servicePortName')} value={port.name} placeholder="http" onChange={(event) => updatePort(index, { name: event.target.value })} /><input aria-label={t('workloads.workloadEditor.servicePorts')} type="number" value={port.port} placeholder="80" onChange={(event) => updatePort(index, { port: event.target.value })} /><label>{t('workloads.containerPorts.protocol')}<select aria-label={t('workloads.containerPorts.protocol')} value={port.protocol ?? 'TCP'} onChange={e => updatePort(index, {protocol: e.target.value || undefined})}><option value="TCP">TCP</option><option value="UDP">UDP</option><option value="SCTP">SCTP</option></select></label><Button tone="quiet" onClick={() => updateForm({ ...form, servicePorts: form.servicePorts.filter((_, i) => i !== index) })}>{t('workloads.workloadEditor.remove')}</Button></div>
          <Advanced label={t('workloads.workloadEditor.portValue', { value: port.name || index + 1 })} summary={t('workloads.workloadEditor.effectiveTargetPortValueValue2', { value: port.targetPort || port.port || t('workloads.workloadEditor.none2'), value2: port.targetPort ? t('workloads.workloadEditor.override') : t('workloads.workloadEditor.fromServicePort') })}>
            <label>{t('workloads.workloadEditor.containerTargetPort')}<input aria-label={t('workloads.workloadEditor.containerTargetPort')} type="number" value={port.targetPort} placeholder={port.port || '8080'} onChange={(event) => updatePort(index, { targetPort: event.target.value })} /></label>
            <p>{t('workloads.workloadEditor.emptyMakesTheTargetPort')}</p>
          </Advanced></div>)}
        <ServiceEndpointFields dependencies={form.dependencies} ports={form.servicePorts} available={types.some(type=>type.key==='endpoint')} serviceEnabled={form.execution?.serviceEnabled!==false} workloadName={form.name} disabled={saving || classMetadata!=='ready'} onChange={dependencies=>updateForm({...form,dependencies})}/>
      </ConfigGroup></section>
      <section className="content-panel editor-grid"><ConfigGroup initialOpen keepMounted title={wd('dependencies')} summary={form.dependencies.map(d=>d.alias||d.type).join(', ')||wd('notConfigured')} invalid={Boolean(error)}><div className="section-header"><div><h2>{t('workloads.workloadEditor.workloadResources')}</h2><p>{t('workloads.workloadEditor.chooseAnAvailableResourceType')}</p></div><div className="form-actions"><Button tone="quiet" onClick={() => void reloadClassMetadata()}>{t('workloads.workloadEditor.refreshClassMetadata')}</Button><Button onClick={() => updateForm({ ...form, dependencies: [...form.dependencies, emptyDependency()] })}>{t('workloads.workloadEditor.addResource')}</Button></div></div>{form.dependencies.map((dep, index) => dep.type === 'endpoint' ? null : <DependencyFields key={index} index={index} dependency={dep} types={types} choices={resourceChoices} classMetadata={classMetadata} disabled={saving} servicePorts={[...validPortNames(form.servicePorts)]} onChange={(patch) => updateDependency(index, patch)} onReload={() => void reloadClassMetadata()} onRemove={() => updateForm({ ...form, dependencies: form.dependencies.filter((_, i) => i !== index) })} />)}</ConfigGroup></section>

      <AdvancedFields label="Workload" invalid={Boolean(error)||templateInputProblems>0}>
      <RuntimePolicyFields value={form.execution??customExecution()} onChange={execution=>updateForm({...form,execution})} resolution={resolutionState==='ready'?resolution:undefined} replicas={form.replicas} onReplicas={replicas=>updateForm({...form,replicas})} containers={form.containers.map(c=>c.name)} invalid={Boolean(error)}/>
      <section className="content-panel editor-grid"><ConfigGroup keepMounted title={wd('infrequent')} summary={`${form.execution?.kind??'Deployment'} · ${form.workloadClass||'default'}`} invalid={Boolean(error)||templateInputProblems>0}>
      {form.execution ? <ExecutionFields value={form.execution} onChange={execution => updateForm({ ...form, execution })} metadata={templateMetadata} disabled={saving} onRawProblems={setTemplateInputProblems} resolution={resolution} /> : null}

      <WorkloadClassControl embedded value={form.workloadClass} classes={workloadClasses} metadata={classMetadata} disabled={saving} onChange={workloadClass => updateForm({ ...form, workloadClass })} onReload={() => void reloadClassMetadata()}>
        <div className="section-header"><h3>{t('workloads.annotations.title')}</h3><Button onClick={() => updateForm({ ...form, annotations: [...form.annotations, {name: '', value: ''}] })}>{t('workloads.annotations.add')}</Button></div>
        {form.annotations.map((annotation, index) => <div className="field-grid" key={index}><label>{t('workloads.annotations.name')}<input value={annotation.name} onChange={event => updateForm({ ...form, annotations: form.annotations.map((a, i) => i === index ? {...a, name: event.target.value} : a) })} /></label><label>{t('workloads.annotations.value')}<input value={annotation.value} onChange={event => updateForm({ ...form, annotations: form.annotations.map((a, i) => i === index ? {...a, value: event.target.value} : a) })} /></label><Button tone="quiet" onClick={() => updateForm({ ...form, annotations: form.annotations.filter((_,i) => i !== index) })}>{t('workloads.annotations.remove')}</Button></div>)}
      </WorkloadClassControl>
      </ConfigGroup></section>
      </AdvancedFields>
    </>}
    {dialog?.type === 'key' && catalog.status === 'ready' ? <CreateKeyDialog application={application} environment={environment} kind={dialog.kind} suggestedName={dialog.suggested} catalog={catalog} onTargetChange={onTargetChange}
      onConfiguration={acceptConfiguration(scope.current)} onClose={closeDialog}
      onCreated={((at, rowId, kind, file, targetContainer, targetMount) => (key: string) => {
        if (at !== scope.current) return;
        setDialog(undefined);
        if (file) {
          setForm(prev => !prev.containers.some(c => c.name === targetContainer) ? prev : ({ ...prev, execution: { ...(prev.execution ?? customExecution()), secretFiles: [...(prev.execution?.secretFiles ?? []), { ref: `\${resources.env.${key}}`, container: targetContainer!, path: targetMount?.path ?? '/keys/value', mode: targetMount?.mode ?? 256 }] } }));
          setDirty(true); return;
        }
        // Select the key on the row that opened the dialog; a row removed meanwhile is left alone. The name is only suggested while empty.
        changeRow(rowId, (row) => ({ ...changeSource(row, kind), key, ...(row.name === '' ? { name: key } : {}) }));
        setFocusRequest({ rowId, field: 'key' });
      })(scope.current, dialog.rowId, dialog.kind, dialog.file, dialog.container, dialog.mount)} /> : null}
    {dialog?.type === 'storage' ? <AddResourceDialog label={t('workloads.templates.addStorage')} initialDependency={dialog.initial} existing={form.dependencies} types={storageTypes} choices={resourceChoices.filter(choice => choice.dependency && storageKeys.has(choice.dependency.type))} classMetadata={classMetadata} onReload={() => void reloadClassMetadata()} onClose={closeDialog}
      onAdd={((at, targetContainer, targetMount) => (dependency: Dependency) => {
        if (at !== scope.current) return;
        setDialog(undefined);
        setForm(prev => !prev.containers.some(c => c.name === targetContainer) ? prev : ({ ...prev, dependencies: [...prev.dependencies, dependency], execution: { ...(prev.execution ?? customExecution()), mounts: [...(prev.execution?.mounts ?? []), { resource: dependency.alias, container: targetContainer, path: targetMount.path, readOnly: targetMount.readOnly, ...(targetMount.subPath ? {subPath:targetMount.subPath} : {}) }] } }));
        setDirty(true);
      })(scope.current, dialog.container, dialog.mount)} /> : null}
    {dialog?.type === 'resource' ? <AddResourceDialog existing={form.dependencies} servicePorts={[...validPortNames(form.servicePorts)]} types={types} choices={resourceChoices} classMetadata={classMetadata} onReload={() => void reloadClassMetadata()} onClose={closeDialog}
      onAdd={((at, rowId) => (dependency: Dependency) => {
        if (at !== scope.current) return;
        setDialog(undefined);
        // Only the draft changes: the dependency is added, the row points to its alias and the output stays to be chosen.
        setForm((prev) => ({ ...prev, dependencies: [...prev.dependencies, dependency] }));
        changeRow(rowId, (row) => ({ ...changeSource(row, 'RESOURCE'), alias: dependency.alias, output: '' }));
        setFocusRequest({ rowId, field: 'output' });
      })(scope.current, dialog.rowId)} /> : null}
    {!loading && environmentBusy ? <div className="form-info" role="status">{t('workloads.workloadEditor.anEnvironmentOperationIsRunning', { environment })}</div> : null}
    {!loading ? <div className="form-actions editor-actions"><Button onClick={() => navigate({ name: 'application', applicationId: application.id, environment })}>{t('workloads.workloadEditor.cancel')}</Button><Button tone="primary" disabled={environmentBusy || blocked || saving || stale || loadFailed || (mode === 'import' && !imported)} onClick={() => void save()}>{t('workloads.workloadEditor.saveWorkloadDraft')}</Button></div> : null}
  </section>;
}
