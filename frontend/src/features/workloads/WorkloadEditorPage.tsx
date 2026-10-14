import { useEffect, useRef, useState, type ReactNode } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { ApiError, describeError, isBusy, isStaleConflict } from '../../shared/api/client';
import { useEnvironmentBusy } from '../environment/useEnvironmentBusy';
import { getConfiguration } from '../configuration/api';
import { getResourceTypes, getWorkloads, parseScoreImport, saveWorkload, type ResourceType, type Workload } from './api';
import { ApplicationKeyPicker } from './ApplicationKeyPicker';
import { ClassControls } from './ClassControls';
import { bindingProblem, emptyBinding, keyBinding, type Binding, type KeyCatalog, type Source } from './bindings';

type ContainerForm = { name: string; image: string; cpuRequest: string; memoryRequest: string; cpuLimit: string; memoryLimit: string; bindings: Binding[] };
// sharedId is the Score resource `id` (UC-06 shared dependency `<type>.<class>#shared.<id>`); className is the
// provisioning class. Both are kept exactly as chosen or imported: never defaulted, mapped or switched.
type Dependency = { alias: string; type: string; className: string; sharedId: string; params: Record<string, string> };
type ClassMetadata = 'ready' | 'loading' | 'error';
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
  if (Object.values(resources).some((resource) => resource.type === 'service' && (resource.class != null || Object.keys((resource.params ?? {}) as Record<string, unknown>).some((key) => !['workload', 'port'].includes(key))))) return true;
  if (raw.service && Object.keys(raw.service as Record<string, unknown>).some((key) => key !== 'ports' && key !== 'publicRoutes')) return true;
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
      resources.set(dependency.alias, { type: dependency.type, ...(dependency.className ? { class: dependency.className } : {}), ...(dependency.sharedId ? { id: dependency.sharedId } : {}), ...(params.length ? { params: Object.fromEntries(params) } : {}) });
    }
  }
  for (const container of form.containers) {
    const variables: [string, string][] = [];
    const names = new Set<string>();
    for (const binding of container.bindings) {
      // bindingProblem runs first; this guard only refuses a silent overwrite.
      if (names.has(binding.name)) throw new Error(`Container ${container.name}: tên ${binding.name} bị trùng.`);
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

const DEFAULT_CPU_REQUEST = '10m';
const DEFAULT_MEMORY_REQUEST = '32Mi';

// Effective request: explicit value, else the same-dimension limit, else the
// current default. Display only; the Score keeps exactly what the user entered.
function effectiveRequest(request: string, limit: string, fallback: string): string {
  if (request) return `${request} (tường minh)`;
  if (limit) return `${limit} (theo limit)`;
  return `${fallback} (mặc định)`;
}

const validPortNumber = (value: string): boolean => /^\d+$/.test(value.trim()) && Number(value) >= 1 && Number(value) <= 65535;

// Names of Service ports a public route may target: nonblank, unique among all
// rows, and with an integer port in 1..65535.
function validPortNames(ports: ServicePort[]): Set<string> {
  const counts = new Map<string, number>();
  for (const port of ports) if (port.name.trim()) counts.set(port.name, (counts.get(port.name) ?? 0) + 1);
  return new Set(ports.filter((port) => port.name.trim() && counts.get(port.name) === 1 && validPortNumber(port.port)).map((port) => port.name));
}

// A route keeps its recorded target; a missing or no longer valid target is
// reported, never replaced.
function routeTargetProblem(route: PublicRoute, valid: ReadonlySet<string>): string | undefined {
  if (!route.port) return 'Hãy chọn một cổng Service cho đường dẫn công khai này.';
  if (!valid.has(route.port)) return `Cổng Service "${route.port}" không còn hợp lệ (cần tên không trống, duy nhất và số cổng nguyên 1–65535). Đích cũ được giữ nguyên; hãy sửa cổng hoặc chọn cổng khác.`;
  return undefined;
}

// Closing only hides the inputs; the form state lives in the page, so edited
// and imported overrides survive. The summary stays visible while closed.
function Advanced({ label, summary, children }: { label: string; summary: ReactNode; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return <div className="advanced-section">
    <div role="note" aria-label={`Tóm tắt nâng cao ${label}`} className="muted">{summary}</div>
    <Button tone="quiet" aria-expanded={open} aria-label={`${open ? 'Ẩn' : 'Hiện'} nâng cao ${label}`} onClick={() => setOpen(!open)}>{open ? 'Ẩn nâng cao' : 'Hiện nâng cao'}</Button>
    {open ? children : null}
  </div>;
}

function readForm(score: Record<string, unknown>): Form {
  const form = emptyForm();
  form.name = String((score.metadata as { name?: string } | undefined)?.name ?? '');
  const resources = (score.resources ?? {}) as Record<string, { type?: string; class?: string; id?: unknown; params?: Record<string, unknown> }>;
  form.dependencies = Object.entries(resources).filter(([, spec]) => spec.type !== 'environment' && spec.type !== 'service').map(([alias, spec]) => ({ alias, type: spec.type ?? '', className: spec.class ?? '', sharedId: typeof spec.id === 'string' ? spec.id : '', params: Object.fromEntries(Object.entries(spec.params ?? {}).map(([key, value]) => [key, String(value)])) }));
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
  form.publicRoutes = [...((score.service as { publicRoutes?: PublicRoute[] } | undefined)?.publicRoutes ?? [])];
  return form;
}

// `service.publicPort` was removed; a draft that still has it is never opened
// as a form because the route it declared would be dropped or rewritten.
function hasRemovedPublicPort(score: Record<string, unknown>): boolean {
  const service = score.service;
  return typeof service === 'object' && service !== null && Object.prototype.hasOwnProperty.call(service, 'publicPort');
}

const removedPublicPortMessage = 'Draft Score còn khai báo publicPort, trường này không còn được hỗ trợ. Khai báo đường dẫn bằng publicRoutes; hệ thống không tự chuyển đổi.';

export function WorkloadEditorPage({ application, environment, workloadId, onTargetChange }: { application: Application; environment: EnvironmentKey; workloadId?: string; onTargetChange?(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void }) {
  // Environment operations hold draft writes: Save stays disabled, text is kept.
  const environmentBusy = useEnvironmentBusy(application, environment, onTargetChange);
  const [form, setForm] = useState<Form>(emptyForm);
  const [catalog, setCatalog] = useState<KeyCatalog>({ status: 'loading' });
  const [catalogAttempt, setCatalogAttempt] = useState(0);
  const [types, setTypes] = useState<ResourceType[]>([]);
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
  const [current, setCurrent] = useState<{ version: number; score?: Record<string, unknown>; state?: string }>();
  const [reloadError, setReloadError] = useState('');
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
    setForm(emptyForm()); setImported(undefined); setDirty(false); setMode('form'); setAdvancedScore(false); setBlocked(false); setSaving(false); setReloading(false); setWorkloads([]);
  }, [application.id, environment, workloadId]);

  useEffect(() => {
    let cancelled = false;
    setStale(false); setCurrent(undefined); setReloadError('');
    setLoading(true); setLoadFailed(false); setError('');
    Promise.all([getWorkloads(application.id, environment), getResourceTypes()]).then(([list, catalog]) => {
      if (cancelled) return;
      setWorkloads(list.workloads); setTypes(catalog.resourceTypes.filter((type) => type.key !== 'workload')); setClassMetadata('ready'); setVersion(list.draftVersion);
      const existing = list.workloads.find((item) => item.id === workloadId);
      if (existing?.score && hasRemovedPublicPort(existing.score)) { setForm({ ...emptyForm(), name: workloadId ?? '' }); setBlocked(true); setError(removedPublicPortMessage); }
      else if (existing?.score) { setForm(readForm(existing.score)); setImported(existing.score); setBlocked(false); const advanced = needsScoreEditor(existing.score, catalog.resourceTypes); setAdvancedScore(advanced); if (advanced) setMode('import'); }
      else if (workloadId) { setForm({ ...emptyForm(), name: workloadId }); setBlocked(true); setError(existing ? 'Workload đã triển khai này chưa có draft Score có thể sửa. Việc sửa bị tắt để tránh làm mất cấu hình hiện có.' : 'Không tìm thấy Workload trong môi trường này.'); }
    }).catch((err: unknown) => { if (!cancelled) { setError(describeError(err, 'Không tải được cấu hình Workload. Kiểm tra kết nối rồi thử lại.')); setLoadFailed(true); setClassMetadata('error'); } }).finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [application.id, environment, workloadId, loadAttempt]);

  // The Application key catalog loads on its own so a failure is shown as a
  // retryable state in the picker, never as an empty key list.
  useEffect(() => {
    let cancelled = false;
    setCatalog({ status: 'loading' });
    getConfiguration(application.id, environment).then((config) => { if (!cancelled) setCatalog({ status: 'ready', keys: config.keys ?? [] }); })
      .catch((err: unknown) => { if (!cancelled) setCatalog({ status: 'error', message: describeError(err, 'Chưa nhận được phản hồi từ máy chủ') }); });
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
      if (at === scope.current) setReloadError(`Không tải lại được trạng thái hiện tại: ${describeError(err, 'Chưa nhận được phản hồi từ máy chủ.')} Bản chỉnh sửa của bạn được giữ nguyên; hãy thử lại.`);
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
      setTypes(catalog.resourceTypes.filter((type) => type.key !== 'workload')); setClassMetadata('ready');
    } catch {
      if (at === scope.current && request === metadataRequest.current) setClassMetadata('error');
    }
  }

  function updateForm(next: Form) { setForm(next); setDirty(true); }
  function updateContainer(index: number, next: ContainerForm) { const containers = [...form.containers]; containers[index] = next; updateForm({ ...form, containers }); }
  function updateBinding(containerIndex: number, bindingIndex: number, next: Binding) { const container = form.containers[containerIndex]; if (!container) return; const bindings = [...container.bindings]; bindings[bindingIndex] = next; updateContainer(containerIndex, { ...container, bindings }); }
  function updateDependency(index: number, patch: Partial<Dependency>) { updateForm({ ...form, dependencies: form.dependencies.map((item, i) => i === index ? { ...item, ...patch } : item) }); }
  function updatePort(index: number, patch: Partial<ServicePort>) { updateForm({ ...form, servicePorts: form.servicePorts.map((item, i) => i === index ? { ...item, ...patch } : item) }); }
  function updateRoute(index: number, patch: Partial<PublicRoute>) { updateForm({ ...form, publicRoutes: form.publicRoutes.map((item, i) => i === index ? { ...item, ...patch } : item) }); }
  // Adding a route never happens on its own. The target is chosen here only when
  // exactly one Service port is valid; otherwise the user must choose.
  function addRoute() {
    const valid = [...validPortNames(form.servicePorts)];
    updateForm({ ...form, publicRoutes: [...form.publicRoutes, { path: '', port: valid.length === 1 ? valid[0]! : '' }] });
  }
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
    catch (err) { if (at === scope.current) setError(describeError(err, 'Không nhập được file Score. Kiểm tra file và kết nối rồi thử lại.')); }
  }
  async function save() {
    if (environmentBusy) return;
    if (blocked || stale) return;
    const seenPaths = new Set<string>();
    const validPorts = validPortNames(form.servicePorts);
    for (const route of form.publicRoutes) {
      if (!/^\/(?:[A-Za-z0-9._~-]+(?:\/[A-Za-z0-9._~-]+)*)?$/.test(route.path) || seenPaths.has(route.path)) { setError('Đường dẫn công khai phải là URL path duy nhất, ví dụ / hoặc /api.'); return; }
      seenPaths.add(route.path);
      const problem = routeTargetProblem(route, validPorts);
      if (problem) { setError(`Đường dẫn ${route.path}: ${problem}`); return; }
    }
    if (mode === 'form' || dirty) {
      if (classMetadata !== 'ready' && form.dependencies.length > 0) { setError(classMetadata === 'loading' ? 'Metadata class đang tải. Hãy chờ tải xong trước khi lưu.' : 'Chưa xác nhận được metadata class của tài nguyên. Hãy tải lại metadata class trước khi lưu; các giá trị đã nhập được giữ nguyên.'); return; }
      for (const dependency of form.dependencies) {
        const contract = types.find((item) => item.key === dependency.type);
        if (!contract) { setError(`Hãy chọn loại tài nguyên hợp lệ cho ${dependency.alias || 'tài nguyên'}.`); return; }
        for (const input of contract.inputs ?? []) {
          const value = paramValue(dependency.params, input.name);
          if (input.required && !value.trim()) { setError(`${dependency.alias || dependency.type}: ${input.name} là bắt buộc.`); return; }
          if (value && input.type === 'number' && !Number.isFinite(Number(value))) { setError(`${dependency.alias || dependency.type}: ${input.name} phải là số.`); return; }
        }
      }
      if (catalog.status !== 'ready') { setError(catalog.status === 'loading' ? 'Biến và bí mật của ứng dụng đang tải. Hãy chờ tải xong trước khi lưu.' : 'Không tải được biến và bí mật của ứng dụng. Hãy thử tải lại trước khi lưu.'); return; }
      const problem = bindingProblem(form.containers, new Set(catalog.keys.map((key) => key.name)));
      if (problem) { setError(problem); return; }
    }
    const score = mode === 'import' && imported && !dirty ? imported : !dirty && imported ? imported : buildScore(form, types);
    const id = String((score.metadata as { name?: string } | undefined)?.name ?? '');
    if (!id) { setError('Cần nhập tên Workload.'); return; }
    const at = scope.current;
    setSaving(true); setError('');
    try { await saveWorkload(application.id, environment, id, score, version); if (at === scope.current) navigate({ name: 'application', applicationId: application.id }); }
    catch (err) {
      if (at !== scope.current) return;
      if (isBusy(err)) setError((err as ApiError).message);
      else if (isStaleConflict(err)) { setStale(true); setCurrent(undefined); setReloadError(''); }
      else setError(err instanceof ApiError ? err.message : 'Chưa nhận được phản hồi từ máy chủ nên chưa rõ draft đã được lưu hay chưa. Hãy tải lại trạng thái hiện tại trước khi lưu lại; Console không tự gửi lại.');
    }
    finally { if (at === scope.current) setSaving(false); }
  }
  const candidateServices = workloads.filter((item) => item.id !== form.name && item.state !== 'PENDING_DELETE' && item.servicePorts?.length);
  const validPorts = validPortNames(form.servicePorts);
  const currentState = current?.state === 'absent' ? 'không có' : current?.state === 'deployed' ? 'đã triển khai, không có thay đổi chờ' : current?.state === 'PENDING_DELETE' ? 'đang chờ xóa' : 'có thay đổi đang chờ';

  return <section className="page workload-editor">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id })}>← {application.name} / {environment}</button>
    <header className="page-header application-header"><div><p className="eyebrow">{environment} · Cấu hình Workload</p><h1>{workloadId ? `Sửa ${workloadId}` : 'Thêm Workload'}</h1><p>Hãy lưu draft trước. Xem trước và Triển khai là các bước riêng.</p></div><Button onClick={() => navigate({ name: 'settings', applicationId: application.id, environment })}>Biến &amp; Bí mật</Button></header>
    <div className="tabs" role="tablist"><button className={mode === 'form' ? 'tab tab-active' : 'tab'} disabled={advancedScore} onClick={() => setMode('form')}>Nhập bằng form</button><button className={mode === 'import' ? 'tab tab-active' : 'tab'} onClick={() => setMode('import')}>Nhập Score</button></div>
    {advancedScore ? <p className="feature-note">Score này có các trường mà form không thể giữ nguyên. Hãy tải lên file Score đã cập nhật để sửa mà không làm mất các trường đó.</p> : null}
    {error ? <div className="form-error" role="alert">{error}{loadFailed ? <> <Button onClick={() => setLoadAttempt((value) => value + 1)}>Thử lại</Button></> : null}</div> : null}
    {stale ? <div className="form-error" role="alert" aria-label="Draft Workload đã cũ">Các Workload trong {environment} đã thay đổi sau khi trang này được tải. Bản chỉnh sửa của bạn được giữ nhưng chưa được lưu. Hãy tải lại trạng thái hiện tại, xem lại rồi lưu lại. <Button disabled={reloading} onClick={() => void reloadCurrent()}>{reloading ? 'Đang tải lại…' : 'Tải lại trạng thái hiện tại'}</Button>{reloadError ? <p>{reloadError}</p> : null}</div> : null}
    {current ? <div className="form-info" role="status">Đã tải lại draft phiên bản {current.version}. Trạng thái hiện tại của Workload này: {currentState}. Các chỉnh sửa bên dưới không đổi; hãy xem lại và lưu lại nếu vẫn áp dụng.{current.score ? <details><summary>Score hiện tại</summary><pre className="score-preview">{JSON.stringify(current.score, null, 2)}</pre></details> : null}</div> : null}
    {loading ? <p>Đang tải tùy chọn Workload…</p> : mode === 'import' ? <section className="content-panel"><h2>Nhập một file Score</h2><p>YAML hoặc JSON. Áp dụng cùng quy tắc tham chiếu như form; nhập file không lưu hay triển khai.</p><input aria-label="File Score" type="file" accept=".yaml,.yml,.json,text/yaml,application/json" onChange={(event) => { const file = event.target.files?.[0]; if (file) void importFile(file); }} />{imported ? <><p>Đã phân tích Workload: <strong>{String((imported.metadata as { name?: string } | undefined)?.name ?? '')}</strong></p><pre className="score-preview">{JSON.stringify(imported, null, 2)}</pre></> : null}</section> : <>
      <section className="content-panel editor-grid"><h2>Thông tin cơ bản</h2><label>Tên Workload<input value={form.name} disabled={Boolean(workloadId)} onChange={(event) => updateForm({ ...form, name: event.target.value })} placeholder="frontend" /></label></section>
      {catalog.status === 'error' ? <div className="form-error" role="alert">Không tải được biến và bí mật của ứng dụng. {catalog.message} Các lựa chọn của bạn được giữ nguyên. <Button onClick={() => setCatalogAttempt((value) => value + 1)}>Thử tải lại key</Button></div> : null}
      {form.containers.map((container, ci) => <section className="content-panel editor-grid" key={ci}><div className="section-header"><h2>Container {ci + 1}</h2>{form.containers.length > 1 ? <Button tone="danger" onClick={() => updateForm({ ...form, containers: form.containers.filter((_, index) => index !== ci) })}>Xóa</Button> : null}</div><div className="field-grid"><label>Tên Container<input value={container.name} onChange={(event) => updateContainer(ci, { ...container, name: event.target.value })} /></label><label>Ảnh Container<input value={container.image} onChange={(event) => updateContainer(ci, { ...container, image: event.target.value })} placeholder="registry.example/app:tag" /></label></div>
        <Advanced label={`Container ${ci + 1}`} summary={`Yêu cầu hiệu lực: CPU ${effectiveRequest(container.cpuRequest, container.cpuLimit, DEFAULT_CPU_REQUEST)} · Bộ nhớ ${effectiveRequest(container.memoryRequest, container.memoryLimit, DEFAULT_MEMORY_REQUEST)}. Giới hạn: CPU ${container.cpuLimit || 'chưa đặt'} · Bộ nhớ ${container.memoryLimit || 'chưa đặt'}.`}>
          <div className="field-grid field-grid-four"><label>Yêu cầu CPU<input value={container.cpuRequest} onChange={(event) => updateContainer(ci, { ...container, cpuRequest: event.target.value })} placeholder="100m" /></label><label>Yêu cầu bộ nhớ<input value={container.memoryRequest} onChange={(event) => updateContainer(ci, { ...container, memoryRequest: event.target.value })} placeholder="128Mi" /></label><label>Giới hạn CPU<input value={container.cpuLimit} onChange={(event) => updateContainer(ci, { ...container, cpuLimit: event.target.value })} /></label><label>Giới hạn bộ nhớ<input value={container.memoryLimit} onChange={(event) => updateContainer(ci, { ...container, memoryLimit: event.target.value })} /></label></div>
          <p>Để trống yêu cầu thì dùng giới hạn cùng loại, nếu không có thì dùng mặc định CPU {DEFAULT_CPU_REQUEST}, bộ nhớ {DEFAULT_MEMORY_REQUEST}. Giới hạn để trống nghĩa là chưa đặt; không có giới hạn nào được thêm tự động.</p>
        </Advanced>
        <h3>Biến &amp; Bí mật của ứng dụng</h3><p>Tích chọn các key có sẵn trong cài đặt {environment}. Mỗi key giữ tên riêng trong Container này trừ khi bạn chọn tên khác. Giá trị không bao giờ được hiển thị hoặc sao chép.</p>
        <ApplicationKeyPicker container={container.name || `Container ${ci + 1}`} bindings={container.bindings} catalog={catalog} disabled={saving} onChange={(bindings) => updateContainer(ci, { ...container, bindings })} />
        <h3>Nguồn khác</h3><p>Output của tài nguyên và Service của các Workload khác trong {environment}. Không cho phép nhập giá trị trực tiếp.</p>
        {container.bindings.map((binding, bi) => binding.source === 'KEY' ? null : <div className="binding-row" key={bi}><input aria-label="Tên biến Container" value={binding.name} disabled={saving} placeholder="API_URL" onChange={(event) => updateBinding(ci, bi, { ...binding, name: event.target.value })} /><select aria-label="Nguồn tham chiếu" value={binding.source} disabled={saving} onChange={(event) => updateBinding(ci, bi, { ...emptyBinding(), name: binding.name, source: event.target.value as Source })}><option value="RESOURCE">Output tài nguyên</option><option value="SERVICE">Service của Workload</option></select>
          {binding.source === 'RESOURCE' ? <><select aria-label="Tài nguyên được tham chiếu" value={binding.alias} disabled={saving} onChange={(event) => updateBinding(ci, bi, { ...binding, alias: event.target.value, output: '' })}><option value="">Chọn tài nguyên</option>{form.dependencies.map((dep) => <option key={dep.alias} value={dep.alias}>{dep.alias}</option>)}</select><select aria-label="Output của tài nguyên" value={binding.output} disabled={saving} onChange={(event) => updateBinding(ci, bi, { ...binding, output: event.target.value })}><option value="">Chọn output</option>{types.find((type) => type.key === form.dependencies.find((dep) => dep.alias === binding.alias)?.type)?.outputs.map((output) => <option key={output.name} value={output.name}>{output.name}{output.secret ? ' · bí mật' : ''}</option>)}</select></> : null}
          {binding.source === 'SERVICE' ? <><select aria-label="Workload cung cấp Service" value={binding.workload} disabled={saving} onChange={(event) => updateBinding(ci, bi, { ...binding, workload: event.target.value, port: '' })}><option value="">Chọn Workload</option>{candidateServices.map((item) => <option key={item.id} value={item.id}>{item.id}</option>)}</select><select aria-label="Cổng của Service" value={binding.port} disabled={saving} onChange={(event) => updateBinding(ci, bi, { ...binding, port: event.target.value })}><option value="">Chọn cổng</option>{candidateServices.find((item) => item.id === binding.workload)?.servicePorts?.map((port) => <option key={port} value={port}>{port}</option>)}</select></> : null}<Button tone="quiet" disabled={saving} onClick={() => updateContainer(ci, { ...container, bindings: container.bindings.filter((_, index) => index !== bi) })}>Xóa</Button></div>)}
        <Button disabled={saving} onClick={() => updateContainer(ci, { ...container, bindings: [...container.bindings, emptyBinding()] })}>+ Thêm nguồn khác</Button></section>)}
      <Button onClick={() => updateForm({ ...form, containers: [...form.containers, { ...emptyContainer(), name: `container-${form.containers.length + 1}` }] })}>+ Thêm Container</Button>
      <section className="content-panel editor-grid"><div className="section-header"><div><h2>Tài nguyên của Workload</h2><p>Chọn loại tài nguyên có sẵn, điền các input bắt buộc rồi dùng output của nó ở trên.</p></div><div className="form-actions"><Button tone="quiet" onClick={() => void reloadClassMetadata()}>Làm mới metadata class</Button><Button onClick={() => updateForm({ ...form, dependencies: [...form.dependencies, { alias: '', type: '', className: '', sharedId: '', params: {} }] })}>+ Thêm tài nguyên</Button></div></div>{form.dependencies.map((dep, index) => <div className="editor-grid" key={index}><div className="binding-row"><input aria-label="Alias tài nguyên" value={dep.alias} placeholder="db" onChange={(event) => updateDependency(index, { alias: event.target.value })} /><select aria-label="Loại tài nguyên" value={dep.type} onChange={(event) => updateDependency(index, { type: event.target.value, params: {} })}><option value="">Chọn loại</option>{types.map((type) => <option key={type.key} value={type.key}>{type.key}</option>)}</select><Button tone="quiet" onClick={() => updateForm({ ...form, dependencies: form.dependencies.filter((_, i) => i !== index) })}>Xóa</Button></div><ClassControls index={index} alias={dep.alias} type={types.find((type) => type.key === dep.type)} className={dep.className} sharedId={dep.sharedId} metadata={classMetadata} disabled={saving} onClass={(value) => updateDependency(index, { className: value })} onSharedId={(value) => updateDependency(index, { sharedId: value })} onReload={() => void reloadClassMetadata()} /><div className="field-grid">{types.find((type) => type.key === dep.type)?.inputs?.map((input) => <label key={input.name}>{dep.alias || dep.type} · {input.name}{input.required ? ' *' : ''}{input.type === 'bool' ? <select aria-label={`Tham số ${input.name}`} value={paramValue(dep.params, input.name)} onChange={(event) => updateDependency(index, { params: { ...dep.params, [input.name]: event.target.value } })}><option value="">Chọn</option><option value="true">Đúng</option><option value="false">Sai</option></select> : <input aria-label={`Tham số ${input.name}`} type={input.type === 'number' ? 'number' : 'text'} value={paramValue(dep.params, input.name)} onChange={(event) => updateDependency(index, { params: { ...dep.params, [input.name]: event.target.value } })} />}</label>)}</div>
        <Advanced label={`tài nguyên ${dep.alias || index + 1}`} summary={`Lớp hiệu lực: ${dep.className || 'default'} (${dep.className ? 'tường minh' : 'mặc định'}).`}>
          <label>Lớp tài nguyên<input aria-label="Lớp tài nguyên" value={dep.className} placeholder="default" onChange={(event) => updateDependency(index, { className: event.target.value })} /></label>
          <p>Để trống thì dùng class default; trường class không được ghi vào Score.</p>
        </Advanced></div>)}</section>
      <section className="content-panel editor-grid">
        <div className="section-header"><div><h2>Cổng Service</h2><p>Truy cập nội bộ cho các Workload khác trong {environment}.</p></div><Button onClick={() => updateForm({ ...form, servicePorts: [...form.servicePorts, { name: '', port: '', targetPort: '' }] })}>+ Thêm cổng</Button></div>
        {form.servicePorts.map((port, index) => <div className="editor-grid" key={index}><div className="binding-row"><input aria-label="Tên cổng Service" value={port.name} placeholder="http" onChange={(event) => updatePort(index, { name: event.target.value })} /><input aria-label="Cổng Service" type="number" value={port.port} placeholder="80" onChange={(event) => updatePort(index, { port: event.target.value })} /><Button tone="quiet" onClick={() => updateForm({ ...form, servicePorts: form.servicePorts.filter((_, i) => i !== index) })}>Xóa</Button></div>
          <Advanced label={`cổng ${port.name || index + 1}`} summary={`Cổng đích hiệu lực: ${port.targetPort || port.port || 'chưa có'} (${port.targetPort ? 'ghi đè' : 'theo cổng Service'}).`}>
            <label>Cổng đích của Container<input aria-label="Cổng đích của Container" type="number" value={port.targetPort} placeholder={port.port || '8080'} onChange={(event) => updatePort(index, { targetPort: event.target.value })} /></label>
            <p>Để trống thì target port bằng cổng Service.</p>
          </Advanced></div>)}
        <div className="section-header"><div><h3>Đường dẫn công khai</h3><p>Ví dụ: / cho frontend và /api cho backend. Đường dẫn chỉ có sau khi bạn thêm; không tự bật.</p></div><Button onClick={addRoute}>+ Thêm đường dẫn công khai</Button></div>
        {form.publicRoutes.map((route, index) => { const problem = routeTargetProblem(route, validPorts); const known = form.servicePorts.filter((port) => validPorts.has(port.name)); return <div className="editor-grid" key={index}><div className="binding-row"><input aria-label="Đường dẫn công khai" value={route.path} placeholder="/api" onChange={(event) => updateRoute(index, { path: event.target.value })} /><select aria-label="Cổng Service công khai" aria-invalid={problem ? true : undefined} value={route.port} onChange={(event) => updateRoute(index, { port: event.target.value })}><option value="">Chọn cổng</option>{route.port && !validPorts.has(route.port) ? <option value={route.port}>{route.port} · không hợp lệ</option> : null}{known.map((port) => <option key={port.name} value={port.name}>{port.name} · {port.port}</option>)}</select><Button tone="quiet" onClick={() => updateForm({ ...form, publicRoutes: form.publicRoutes.filter((_, i) => i !== index) })}>Xóa</Button></div>{problem ? <p className="form-error">{problem}</p> : null}</div>; })}
        <p>Truy cập công khai chỉ được áp dụng sau Xem trước → Triển khai. DNS và TLS không được cấu hình tại đây.</p>
      </section>
    </>}
    {!loading && environmentBusy ? <div className="form-info" role="status">Một thao tác môi trường đang chạy trong {environment}. Bản chỉnh sửa của bạn được giữ nguyên; hãy lưu lại khi thao tác kết thúc.</div> : null}
    {!loading ? <div className="form-actions editor-actions"><Button onClick={() => navigate({ name: 'application', applicationId: application.id })}>Hủy</Button><Button tone="primary" disabled={environmentBusy || blocked || saving || stale || loadFailed || (mode === 'import' && !imported)} onClick={() => void save()}>Lưu draft Workload</Button></div> : null}
  </section>;
}
