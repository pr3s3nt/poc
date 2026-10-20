import { useEffect, useRef, useState, type FormEvent } from 'react';
import { specificGuidance, fieldNote } from '../../shared/api/errors';
import { api, ApiError, isStaleConflict } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { snapshotKey, type ApiEnvironment } from './environmentSnapshot';
import { CriteriaEditor, draftFromCriteria, draftToCriteria, emptyDraft, type ApplicationOption, type Criterion, type CriteriaDraft } from './CriteriaEditor';
import type { MatchingCandidateInput } from './MatchingPreviewPanel';
import { useCatalogList } from './useCatalogList';
import { own } from './resourceSupport';
import { capabilitiesForType, capabilityKey, isVerifiedPin, loadCapabilities, type Capability } from './capabilities';
import { TypedVariablesEditor } from './TypedVariablesEditor';
import { ProvisionEditor } from './ProvisionEditor';
import { parseTypeMetadata } from './metadata';
import { MappingEditor } from './MappingEditor';
import { clusterType, emptyMapping, inheritsCluster, mappingProblems, mappingToObject, needsAccount, objectToMapping, bindingProviders, type MappingDraft, type TypeContract } from './moduleMapping';
import { draftToVariables, emptyVariableDraft, missingRequired, parseObjectText, variablesToDraft, type JsonObject, type VariableDraft } from './typedVariables';
import { provisionToRows, rowsToProvision, type ProvisionRow } from './typedProvision';
import { ExistingClusterSection } from './ExistingClusterSection';
import { listAccounts, statusLabel, typeLabel, type Account } from './accounts/accountsApi';

type APIApplication = { key: string; name?: string; environments?: ApiEnvironment[] };
// The public Definition DTO (UC-03 V02-01): Name, version and Driver Account; no profile or Connection field.
type Definition = {
  key: string; name?: string; version?: number; resourceType: string; driverType: string; driverAccount?: string;
  driverInputs?: { values?: Record<string, unknown> }; provision?: Record<string, unknown>; criteria?: Partial<Criterion>[];
};
const driverLabels: Record<string, string> = { terraform: 'Terraform (module đã kiểm chứng)', kubernetes: 'Tạo trong cluster (Kubernetes)', 'existing-cluster': 'Cluster có sẵn (existing-cluster)' };
const idShape = /^[a-z0-9-]+$/;
const regionShape = /^[a-z]{2}(-[a-z]+)+-[0-9]{1,2}$/;
const maxNameLength = 253;

// Draft of one verified module pin: switching pin keeps each pin's own edits
// (UC-03 BR-24); only the submitted pin is reset after a commit.
type Mode = 'typed' | 'json';
type PairDraft = {
  // Driver Account key and literal AWS region of a pair that needs an account (UC-03 V02-03).
  account: string; region: string;
  varMode: Mode; vars: VariableDraft; varsJSON: string; varsNote: string;
  mapMode: Mode; map: MappingDraft; mapJSON: string; mapNote: string;
  provMode: Mode; rows: ProvisionRow[]; provJSON: string; provNote: string;
};
const initialDraft = (): PairDraft => ({ account: '', region: '', varMode: 'typed', vars: emptyVariableDraft(), varsJSON: '{}', varsNote: '', mapMode: 'typed', map: emptyMapping(), mapJSON: '{}', mapNote: '', provMode: 'typed', rows: [], provJSON: '{}', provNote: '' });
type Editing = { key: string; version: number };
// A stale (409) or unknown-outcome save never resubmits by itself: the form keeps the draft and Save stays off until the author reloads explicitly.
// A create whose outcome is unknown (`created` names the submitted ID) is held the same way: no automatic POST replay until the stored key is read back.
type Review = { kind: 'stale' | 'unknown'; created?: string } | null;

// Specific guidance comes from the stable error code; raw backend text is never shown.
function describeFailure(reason: unknown, editing: boolean): string {
  if (!(reason instanceof ApiError)) return editing
    ? 'Chưa nhận được phản hồi từ máy chủ nên chưa rõ thay đổi đã được lưu hay chưa. Console không tự gửi lại; hãy tải lại cấu hình để xem kết quả thực tế.'
    : 'Chưa nhận được phản hồi từ máy chủ nên chưa rõ kết quả: cấu hình có thể đã được đăng ký. Hãy tải lại danh sách trước khi thử lại.';
  const field = fieldNote(reason.field);
  const specific = specificGuidance(reason.code, reason.message, reason.field);
  if (specific) return `Máy chủ từ chối cấu hình. ${specific}`;
  switch (reason.status) {
    case 400: return `Máy chủ từ chối cấu hình này. Kiểm tra ID, Tên, module đã chọn, tài khoản, vùng, ánh xạ, Tham số cấu hình và Quy tắc tạo tài nguyên liên quan rồi thử lại. Dữ liệu đã nhập được giữ lại.${field}`;
    case 401: return 'Phiên đăng nhập đã hết hạn. Hãy đăng nhập lại.';
    case 403: return 'Bạn không có quyền đăng ký hoặc sửa cấu hình tài nguyên. Chỉ Kỹ sư nền tảng hoặc Quản trị viên được phép.';
    case 404: return 'Không tìm thấy cấu hình này trong Tổ chức của bạn. Tải lại danh sách.';
    case 409: return 'ID cấu hình đã tồn tại. Hãy đổi ID rồi thử lại.';
    case 413: return 'Nội dung cấu hình quá lớn. Hãy rút gọn rồi thử lại.';
    default: return 'Máy chủ gặp lỗi khi lưu cấu hình. Dữ liệu đã nhập được giữ lại; hãy tải lại trước khi thử lại.';
  }
}
const listFailure = () => 'Không tải được dữ liệu từ máy chủ.';
const accountFailure = () => 'Không tải được danh sách tài khoản hạ tầng.';
const capabilityFailure = () => 'Không tải được metadata module đã kiểm chứng từ máy chủ.';

const loadTypes = async (): Promise<TypeContract[]> => parseTypeMetadata(await api<unknown>('/resource-types'));
const loadDefinitions = async () => (await api<{ resourceDefinitions: Definition[] }>('/resource-definitions')).resourceDefinitions ?? [];
// Real application/environment IDs (keys) for the criteria editor; never derived from names.
const loadApplications = async (): Promise<ApplicationOption[]> => ((await api<{ applications: APIApplication[] }>('/applications')).applications ?? []).map((app) => ({ key: app.key, name: app.name ?? app.key, environments: (app.environments ?? []).map((env) => ({ key: env.key, name: env.name ?? env.key, environmentType: env.environmentType ?? '', snapshotKey: snapshotKey(env) })) }));
const shortDigest = (digest: string) => digest.slice(0, 12);
const pinLabel = (capability: Capability) => `${capability.module} · ${capability.source?.revision ?? ''} · sha256:${shortDigest(capability.source?.contentSha256 ?? '')}`;
const accountLabel = (account: Account) => (account.name && account.name !== account.key ? `${account.name} (${account.key})` : account.key);
const isObject = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value);

// initialResourceType comes from the Resource Types CTA query (UC-03 UI): it is
// applied once, after the catalog and the registry metadata confirm a verified
// pair, and never over a choice.
export function ResourceDefinitionsPage({ initialResourceType = '' }: { initialResourceType?: string } = {}) {
  const typeList = useCatalogList(loadTypes, listFailure);
  const capabilityList = useCatalogList(loadCapabilities, capabilityFailure);
  const definitionList = useCatalogList(loadDefinitions, listFailure);
  const applicationList = useCatalogList(loadApplications, listFailure);
  // The account list names the account of every listed Definition and fills the selector, so it always loads.
  const accountList = useCatalogList(listAccounts, accountFailure);
  const types = typeList.items.filter((type) => type.key !== 'workload');
  const definitions = definitionList.items.filter((definition) => definition.resourceType !== 'workload' && definition.driverType !== 'score-k8s' && definition.driverType !== 'existing-cluster');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [idError, setIdError] = useState('');
  const [nameError, setNameError] = useState('');
  const [notice, setNotice] = useState('');
  const [key, setKey] = useState('');
  const [name, setName] = useState('');
  const [resourceType, setResourceType] = useState('');
  const [pinChoice, setPinChoice] = useState('');
  const [drafts, setDrafts] = useState<Record<string, PairDraft>>({});
  const [criteriaDraft, setCriteriaDraft] = useState<CriteriaDraft>(emptyDraft);
  const [editing, setEditing] = useState<Editing | null>(null);
  const [review, setReview] = useState<Review>(null);
  // The stored module of an edited Definition is no longer verified: the replacement pin must then be chosen explicitly, never inferred.
  const [sourceLost, setSourceLost] = useState(false);
  const formRef = useRef<HTMLElement | null>(null);

  // The registry metadata is authoritative: while it loads or fails nothing is "supported" and nothing is guessed.
  const metadataReady = !capabilityList.loading && !capabilityList.loadError;
  const typesReady = !typeList.loading && !typeList.loadError;
  const verifiedPins = (typeKey: string) => (metadataReady ? capabilitiesForType(capabilityList.items, typeKey).filter(isVerifiedPin) : []);
  const supportedType = (typeKey: string) => verifiedPins(typeKey).length > 0;
  const typePins = verifiedPins(resourceType);
  const typeContract: TypeContract | undefined = types.find((type) => type.key === resourceType);
  const clusterClasses = typeList.items.find((type) => type.key === clusterType)?.classes ?? [];

  const prefilled = useRef(false);
  useEffect(() => {
    if (prefilled.current || typeList.loading || typeList.loadError || capabilityList.loading || capabilityList.loadError) return;
    prefilled.current = true;
    if (initialResourceType !== '' && typeList.items.some((type) => type.key === initialResourceType) && capabilitiesForType(capabilityList.items, initialResourceType).some(isVerifiedPin)) setResourceType((current) => (current === '' ? initialResourceType : current));
  }, [typeList.loading, typeList.loadError, typeList.items, capabilityList.loading, capabilityList.loadError, capabilityList.items, initialResourceType]);

  // One verified pin is chosen for the user; several need an explicit choice.
  const pin: Capability | undefined = typePins.length === 1 && !sourceLost ? typePins[0] : typePins.find((candidate) => capabilityKey(candidate) === pinChoice);
  const draftKey = pin ? capabilityKey(pin) : '';
  const draft = drafts[draftKey] ?? initialDraft();
  const patchDraft = (change: Partial<PairDraft>) => { if (draftKey) setDrafts((current) => ({ ...current, [draftKey]: { ...initialDraft(), ...current[draftKey], ...change } })); };
  // The account requirement is published by the server per pair; the form never infers a driver's needs.
  const accountRequired = pin !== undefined && needsAccount(pin);
  const regionName = pin?.account.regionVariable ?? 'region';
  // The region has its own control, so it is not a generic variable of the typed form.
  const variableList = pin ? pin.variables.filter((variable) => !(accountRequired && variable.name === regionName)) : [];
  const compatibleAccounts = accountList.items.filter((account) => account.type === pin?.account.type && account.status === 'READY');
  const chosenAccount = accountRequired ? draft.account : '';
  const chosenRegion = accountRequired ? draft.region.trim() : '';

  // The mapping as the server will receive it, from whichever mode is active.
  const mappingObject = (): { value?: { input_mapping: JsonObject; output_mapping: JsonObject; provider_bindings: JsonObject }; problem?: string } => {
    if (!pin) return { problem: 'Chọn một module đã kiểm chứng.' };
    if (draft.mapMode === 'typed') return { value: mappingToObject(draft.map, pin) };
    const parsed = parseObjectText(draft.mapJSON, 'Ánh xạ module');
    if (!parsed.value) return { problem: parsed.error };
    const extra = Object.keys(parsed.value).find((name) => !['input_mapping', 'output_mapping', 'provider_bindings'].includes(name));
    if (extra) return { problem: `Ánh xạ module chỉ nhận input_mapping, output_mapping và provider_bindings; không nhận "${extra}".` };
    if (typeof parsed.value.provider_bindings !== 'object' || parsed.value.provider_bindings === null || Array.isArray(parsed.value.provider_bindings)) return { problem: 'Ánh xạ module cần provider_bindings là một JSON object.' };
    return { value: { input_mapping: (parsed.value.input_mapping ?? {}) as JsonObject, output_mapping: (parsed.value.output_mapping ?? {}) as JsonObject, provider_bindings: parsed.value.provider_bindings as JsonObject } };
  };
  const currentMapping = pin ? mappingObject() : {};
  // Variables a required Resource Type input feeds (the param then outranks the fallback and the module default).
  const mapped: Record<string, string> = {};
  const covered = new Set<string>();
  for (const [variable, entry] of Object.entries(currentMapping.value?.input_mapping ?? {})) {
    const param = typeof entry === 'object' && entry !== null ? (entry as { param?: unknown }).param : undefined;
    if (typeof param !== 'string') continue;
    mapped[variable] = param;
    if (typeContract?.inputs.some((input) => input.name === param && input.required)) covered.add(variable);
  }

  function switchVariables(target: Mode) {
    if (!pin || target === draft.varMode) return;
    if (target === 'json') {
      const built = draftToVariables(draft.vars, variableList);
      if (!built.value) { patchDraft({ varsNote: built.problem ?? '' }); return; }
      patchDraft({ varMode: 'json', varsJSON: JSON.stringify(built.value, null, 2), varsNote: '' });
      return;
    }
    const parsed = parseObjectText(draft.varsJSON, 'Tham số cấu hình');
    if (!parsed.value) { patchDraft({ varsNote: `${parsed.error ?? ''} Sửa JSON trước khi chuyển sang biểu mẫu.` }); return; }
    const converted = variablesToDraft(parsed.value, variableList);
    if (!converted.draft) { patchDraft({ varsNote: converted.reason ?? '' }); return; }
    patchDraft({ varMode: 'typed', vars: converted.draft, varsNote: '' });
  }
  function switchMapping(target: Mode) {
    if (!pin || target === draft.mapMode) return;
    if (target === 'json') {
      patchDraft({ mapMode: 'json', mapJSON: JSON.stringify(mappingToObject(draft.map, pin), null, 2), mapNote: '' });
      return;
    }
    const parsed = parseObjectText(draft.mapJSON, 'Ánh xạ module');
    if (!parsed.value) { patchDraft({ mapNote: `${parsed.error ?? ''} Sửa JSON trước khi chuyển sang biểu mẫu.` }); return; }
    const converted = objectToMapping(parsed.value, pin, typeContract, clusterClasses);
    if (!converted.draft) { patchDraft({ mapNote: converted.reason ?? '' }); return; }
    patchDraft({ mapMode: 'typed', map: converted.draft, mapNote: '' });
  }
  function switchProvision(target: Mode) {
    if (!pin || target === draft.provMode) return;
    if (target === 'json') {
      const built = rowsToProvision(draft.rows);
      if (!built.value) { patchDraft({ provNote: built.problem ?? '' }); return; }
      patchDraft({ provMode: 'json', provJSON: JSON.stringify(built.value, null, 2), provNote: '' });
      return;
    }
    const converted = provisionToRows(draft.provJSON);
    if (!converted.rows) { patchDraft({ provNote: converted.reason ?? '' }); return; }
    patchDraft({ provMode: 'typed', rows: converted.rows, provNote: '' });
  }

  // Every mode serializes to the same driverInputs.values / provision objects.
  function buildInputs(): { values?: Record<string, unknown>; provision?: Record<string, unknown>; problem?: string } {
    if (!pin?.source) return { problem: 'Chọn một Loại tài nguyên có module đã kiểm chứng.' };
    let variables: Record<string, unknown>, provision: Record<string, unknown>;
    if (draft.varMode === 'typed') {
      const built = draftToVariables(draft.vars, variableList);
      if (!built.value) return { problem: built.problem };
      variables = built.value;
    } else {
      const parsed = parseObjectText(draft.varsJSON, 'Tham số cấu hình');
      if (!parsed.value) return { problem: parsed.error };
      variables = parsed.value;
    }
    if (accountRequired) {
      if (Object.hasOwn(variables, regionName)) return { problem: `Vùng AWS nhập ở ô “Vùng AWS”, không nhập trong Tham số cấu hình (${regionName}).` };
      if (chosenRegion !== '') variables = { ...variables, [regionName]: chosenRegion };
    }
    const mapping = mappingObject();
    if (!mapping.value) return { problem: mapping.problem };
    if (draft.provMode === 'typed') {
      const built = rowsToProvision(draft.rows);
      if (!built.value) return { problem: built.problem };
      provision = built.value;
    } else {
      const parsed = parseObjectText(draft.provJSON, 'Quy tắc tạo tài nguyên liên quan');
      if (!parsed.value) return { problem: parsed.error };
      provision = parsed.value;
    }
    const { sourceId, moduleId, revision, subdirectory, contentSha256 } = pin.source;
    return { values: { source: { source_id: sourceId, module_id: moduleId, revision, subdirectory, content_sha256: contentSha256 }, variables, ...mapping.value }, provision };
  }

  // Guidance before the request; the server checks the same rules again.
  function guidance(): string {
    if (!pin) return metadataReady ? 'Chọn một Loại tài nguyên và module đã kiểm chứng.' : 'Chưa xác nhận được module đã kiểm chứng từ máy chủ. Tải lại metadata rồi thử lại.';
    if (!typesReady || !typeContract) return 'Chưa tải được hợp đồng của Loại tài nguyên. Tải lại rồi thử lại.';
    if (accountRequired && chosenAccount === '') return `Chọn tài khoản ${typeLabel(pin.account.type ?? '')} sẵn sàng cho cấu hình này.`;
    if (accountRequired && !regionShape.test(chosenRegion)) return 'Nhập vùng AWS là một chuỗi literal hợp lệ, ví dụ ap-southeast-1 (không dùng placeholder).';
    if (draft.mapMode === 'typed') {
      const problem = mappingProblems(draft.map, pin, typeContract)[0];
      if (problem) return problem.message;
    }
    if (draft.mapMode === 'json') {
      const parsed = parseObjectText(draft.mapJSON, 'Ánh xạ module');
      const bindings = parsed.value && isObject(parsed.value.provider_bindings) ? parsed.value.provider_bindings : {};
      for (const binding of Object.values(bindings)) {
        if (isObject(binding) && binding.target === 'environment') return 'Binding Kubernetes tới Môi trường đã ngừng hỗ trợ. Chọn tài nguyên cluster làm đích provider.';
      }
    }
    const uncovered = missingRequired(draft.vars, variableList, covered).map((variable) => variable.name);
    if (draft.varMode === 'typed' && uncovered.length > 0) return `Cần nhập trước khi lưu: ${uncovered.join(', ')} (bắt buộc, không có mặc định và không được tham số của Loại cung cấp).`;
    return '';
  }

  function resetForm() {
    setKey(''); setName(''); setIdError(''); setNameError(''); setError(''); setReview(null); setEditing(null); setSourceLost(false);
    setDrafts({}); setCriteriaDraft(emptyDraft()); setPinChoice('');
  }

  // Loads a stored Definition into the form for editing (UC-03 V02-06): the ID and Resource Type stay fixed.
  function startEdit(definition: Definition) {
    const values = isObject(definition.driverInputs?.values) ? definition.driverInputs.values : {};
    const source = isObject(values.source) ? values.source : {};
    const capability = capabilitiesForType(capabilityList.items, definition.resourceType).filter(isVerifiedPin).find((candidate) =>
      candidate.source?.sourceId === source.source_id && candidate.source?.moduleId === source.module_id && candidate.source?.revision === source.revision
      && candidate.source?.subdirectory === source.subdirectory && candidate.source?.contentSha256 === source.content_sha256);
    setNotice(''); setError(''); setIdError(''); setNameError(''); setReview(null);
    if (!capability) {
      // Nothing of the old module configuration is guessed or carried over: the author must pick a verified module explicitly before Save.
      setKey(definition.key); setName(definition.name ?? ''); setResourceType(definition.resourceType); setPinChoice(''); setDrafts({}); setCriteriaDraft(draftFromCriteria(definition.criteria));
      setEditing({ key: definition.key, version: definition.version ?? 0 }); setSourceLost(true);
      setError('Module Terraform của cấu hình này không còn nằm trong metadata đã kiểm chứng của máy chủ. Cấu hình module cũ không được giữ lại; hãy chọn module đã kiểm chứng và nhập lại ánh xạ trước khi lưu.');
      formRef.current?.scrollIntoView?.({ block: 'start' });
      return;
    }
    setSourceLost(false);
    const contract = types.find((type) => type.key === definition.resourceType);
    const accountBound = needsAccount(capability);
    const variables = isObject(values.variables) ? { ...values.variables } : {};
    const regionVariable = capability.account.regionVariable ?? 'region';
    const region = accountBound && typeof variables[regionVariable] === 'string' ? (variables[regionVariable] as string) : '';
    if (accountBound) delete variables[regionVariable];
    const editable = capability.variables.filter((variable) => !(accountBound && variable.name === regionVariable));
    const next = initialDraft();
    next.account = accountBound ? (definition.driverAccount ?? '') : '';
    next.region = region;
    const typedVars = variablesToDraft(variables, editable);
    if (typedVars.draft) next.vars = typedVars.draft; else { next.varMode = 'json'; next.varsJSON = JSON.stringify(variables, null, 2); next.varsNote = typedVars.reason ?? ''; }
    const mappingValue: JsonObject = { input_mapping: isObject(values.input_mapping) ? values.input_mapping : {}, output_mapping: isObject(values.output_mapping) ? values.output_mapping : {}, provider_bindings: isObject(values.provider_bindings) ? values.provider_bindings : {} };
    const typedMap = objectToMapping(mappingValue, capability, contract, clusterClasses);
    if (typedMap.draft) next.map = typedMap.draft; else { next.mapMode = 'json'; next.mapJSON = JSON.stringify(mappingValue, null, 2); next.mapNote = typedMap.reason ?? ''; }
    const provisionText = JSON.stringify(definition.provision ?? {}, null, 2);
    const typedProvision = provisionToRows(provisionText);
    if (typedProvision.rows) next.rows = typedProvision.rows; else { next.provMode = 'json'; next.provJSON = provisionText; next.provNote = typedProvision.reason ?? ''; }
    setKey(definition.key); setName(definition.name ?? ''); setResourceType(definition.resourceType); setPinChoice(capabilityKey(capability));
    setDrafts({ [capabilityKey(capability)]: next }); setCriteriaDraft(draftFromCriteria(definition.criteria));
    setEditing({ key: definition.key, version: definition.version ?? 0 });
    formRef.current?.scrollIntoView?.({ block: 'start' });
  }

  // Explicit reload after a stale or unknown-outcome save: one GET of the stored Definition replaces the draft.
  // After an unknown CREATE the same GET decides: found means the earlier POST committed (the stored version opens for editing, nothing is
  // re-sent); not found means it rolled back (the draft stays and the hold is released); a failed GET keeps both draft and hold.
  async function reloadLatest() {
    const target = review?.created ?? editing?.key;
    if (!target) return;
    let latest: Definition | undefined;
    try { latest = (await loadDefinitions()).find((definition) => definition.key === target); } catch { setError('Chưa tải lại được cấu hình từ máy chủ. Bản nháp của bạn vẫn được giữ; hãy thử lại.'); return; }
    void definitionList.reload();
    if (review?.created) {
      if (!latest) { setReview(null); setError(''); setNotice(`Máy chủ không có cấu hình ${target}: lần đăng ký trước không được lưu. Bản nháp được giữ; bạn có thể đăng ký lại.`); return; }
      startEdit(latest);
      setNotice(`Cấu hình ${target} đã có trên máy chủ (lần đăng ký trước đã được lưu). Đang mở bản đã lưu để xem lại; Console không gửi lại.`);
      return;
    }
    if (!latest) { setError('Cấu hình này không còn trong danh sách của máy chủ. Bản nháp của bạn vẫn được giữ.'); return; }
    startEdit(latest);
    setNotice('Đã tải lại bản mới nhất từ máy chủ; bản nháp trước đó đã được thay thế.');
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving || review) return;
    setError(''); setIdError(''); setNameError(''); setNotice('');
    // The backend owns the ID rule (UC-02 BR-05); the ID is checked, never changed.
    if (key === '') { setIdError('Nhập ID cấu hình.'); return; }
    if (!idShape.test(key)) { setIdError('ID chỉ gồm chữ thường a-z, số 0-9 và dấu gạch ngang. ID không được tự sửa hộ.'); return; }
    if (name.trim() === '') { setNameError('Nhập Tên hiển thị của cấu hình.'); return; }
    if ([...name].length > maxNameLength) { setNameError(`Tên tối đa ${maxNameLength} ký tự.`); return; }
    const blocked = guidance();
    if (blocked) { setError(blocked); return; }
    const inputs = buildInputs();
    if (!inputs.values || !inputs.provision) { setError(inputs.problem ?? ''); return; }
    const { criteria: cleanCriteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) { setError(problem); return; }
    const submitted = key;
    const submittedDraft = draftKey;
    const body = { key: submitted, name, resourceType, driverType: pin?.driverType ?? 'terraform', ...(accountRequired ? { driverAccount: chosenAccount } : {}), driverInputs: { values: inputs.values }, provision: inputs.provision, criteria: cleanCriteria };
    setSaving(true);
    if (editing) {
      let saved: Definition;
      try {
        saved = await api<Definition>(`/resource-definitions/${encodeURIComponent(editing.key)}`, { method: 'PUT', body: JSON.stringify({ ...body, version: editing.version }) });
      } catch (reason) {
        setSaving(false);
        if (reason instanceof ApiError && isStaleConflict(reason)) { setReview({ kind: 'stale' }); setError(''); return; }
        const unknown = !(reason instanceof ApiError) || reason.code === 'COMMIT_UNCERTAIN';
        if (unknown) { setReview({ kind: 'unknown' }); void definitionList.reload(); }
        setError(describeFailure(reason, true));
        return;
      }
      // Committed: stay on the edited Definition at its new version; the list reload is a separate, retryable GET.
      setEditing({ key: saved.key, version: saved.version ?? editing.version + 1 });
      setNotice(`Đã lưu cấu hình ${saved.key} (phiên bản ${saved.version ?? editing.version + 1}). Chỉ Xem trước và Triển khai mới dùng cấu hình mới; tài nguyên đã triển khai và snapshot cũ không đổi.`);
      setSaving(false);
      // Definition criteria change the server-computed Resource Type classes.
      await Promise.all([definitionList.reload(), typeList.reload()]);
      return;
    }
    try {
      await api('/resource-definitions', { method: 'POST', body: JSON.stringify(body) });
    } catch (reason) {
      setSaving(false);
      if (!(reason instanceof ApiError) || reason.code === 'COMMIT_UNCERTAIN') { setReview({ kind: 'unknown', created: submitted }); void definitionList.reload(); }
      setError(describeFailure(reason, false));
      return;
    }
    // Committed: report it and reset the submitted pin's draft before reloading.
    setNotice(`Đã đăng ký cấu hình tài nguyên ${submitted}.`); setKey(''); setName(''); setDrafts((current) => { const next = { ...current }; delete next[submittedDraft]; return next; }); setCriteriaDraft(emptyDraft()); setSaving(false);
    await Promise.all([definitionList.reload(), typeList.reload()]);
  }

  // Full unsaved candidate for the exact matching preview: same shape as the save request, built without side effects.
  const matching = ((): MatchingCandidateInput => {
    if (key === '' || !idShape.test(key)) return { definition: null, problem: 'Nhập ID cấu hình hợp lệ để kiểm tra matching chính xác.' };
    if (!pin) return { definition: null, problem: 'Chọn Loại tài nguyên và module đã kiểm chứng để kiểm tra matching chính xác.' };
    const inputs = buildInputs();
    if (!inputs.values || !inputs.provision) return { definition: null, problem: inputs.problem ?? '' };
    const { criteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) return { definition: null, problem };
    return { definition: { key, name, resourceType, driverType: pin.driverType, ...(accountRequired && chosenAccount !== '' ? { driverAccount: chosenAccount } : {}), driverInputs: { values: inputs.values }, provision: inputs.provision, criteria }, problem: '' };
  })();
  // Account metadata and the stored catalog version invalidate an earlier preview result.
  const previewScope = `accounts:${accountList.loading ? 'loading' : accountList.loadError ? 'error' : JSON.stringify(accountList.items.map((account) => [account.key, account.status, account.version]))}|editing:${editing ? `${editing.key}@${editing.version}` : ''}`;

  const listError = definitionList.loadError;
  const typeOptionLabel = (typeKey: string) => (capabilityList.loading ? `${typeKey} (đang kiểm tra hỗ trợ…)` : capabilityList.loadError ? `${typeKey} (chưa xác nhận hỗ trợ)` : supportedType(typeKey) ? typeKey : `${typeKey} (chưa có module đã kiểm chứng)`);
  const problems = pin && draft.mapMode === 'typed' ? mappingProblems(draft.map, pin, typeContract) : [];
  const accountName = (accountKey: string) => { const account = accountList.items.find((item) => item.key === accountKey); return account ? accountLabel(account) : accountKey; };
  const clusterSources = pin ? bindingProviders(pin).filter((provider) => provider.binding === 'kubernetes').map((provider) => {
    const binding = draft.map.bindings[provider.name];
    const label = binding?.target === 'resource' ? `cluster do node EKS cung cấp${binding.clusterClass ? ` (class ${binding.clusterClass})` : ''}` : 'chưa chọn';
    return { provider: provider.name, label };
  }) : [];
  const selectedMissing = accountRequired && chosenAccount !== '' && !compatibleAccounts.some((account) => account.key === chosenAccount);
  const saveDisabled = saving || types.length === 0 || !metadataReady || review !== null;

  return <section className="page"><header className="page-header"><div><p className="eyebrow">Nền tảng</p><h1>Cấu hình tài nguyên</h1><p>Mô tả cách tạo tài nguyên bằng module Terraform đã kiểm chứng, tài khoản hạ tầng và điều kiện áp dụng. Chỉ Kỹ sư nền tảng hoặc Quản trị viên được đăng ký và sửa. Không có thao tác xóa.</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>Cấu hình đã đăng ký</h2></div>{listError ? <div className="form-error" role="alert" aria-label="Lỗi tải danh sách">{listError} <Button type="button" onClick={() => { void definitionList.reload(); }}>Thử lại</Button></div> : definitionList.loading && definitionList.items.length === 0 ? <p>Đang tải danh sách cấu hình tài nguyên…</p> : definitions.length === 0 ? <p>Chưa có cấu hình tài nguyên nào.</p> : <div className="catalog-list">{definitions.map((definition) => <div key={definition.key} className="catalog-entry" data-definition={definition.key}><strong>{definition.name || definition.key}</strong><span>ID <code>{definition.key}</code> · {definition.resourceType} · {own(driverLabels, definition.driverType) ?? definition.driverType}{definition.driverAccount ? ` · tài khoản ${accountName(definition.driverAccount)}` : ''} · phiên bản {definition.version ?? '?'} · {definition.criteria?.length ?? 0} điều kiện áp dụng{definition.driverType === 'existing-cluster' ? <> · <strong>Hệ thống</strong> (chỉ đọc)</> : null}</span>
      {definition.driverType !== 'existing-cluster' ? <Button type="button" aria-label={`Sửa cấu hình ${definition.key}`} disabled={saving || !metadataReady} onClick={() => startEdit(definition)}>Sửa</Button> : null}</div>)}</div>}</section>
    <ExistingClusterSection definitions={definitionList.items} reload={async () => { void typeList.reload(); return definitionList.reload(); }} />
    <section className="content-panel" ref={formRef} aria-label={editing ? 'Sửa cấu hình tài nguyên' : 'Đăng ký cấu hình tài nguyên'}><div className="section-header"><div><h2>{editing ? `Sửa cấu hình ${editing.key}` : 'Đăng ký cấu hình tài nguyên'}</h2><p>{editing ? 'ID và Loại tài nguyên không đổi. Các thay đổi khác được lưu một lần theo phiên bản; Lưu không tạo hay thay đổi tài nguyên. Chỉ lần Xem trước và Triển khai mới dùng cấu hình mới.' : 'Chỉ nhận module Terraform đã được kiểm chứng trong registry. Module, phiên bản và digest lấy từ metadata của máy chủ; form không có ô nhập nguồn, URL hay digest.'}</p></div>{editing ? <Button type="button" onClick={resetForm} disabled={saving}>Hủy sửa</Button> : null}</div>
      {review ? <div className="form-warning" role="alert" aria-label="Cần tải lại cấu hình">
        <p>{review.kind === 'stale' ? 'Cấu hình đã được người khác thay đổi kể từ lần bạn tải (phiên bản không còn khớp). Chưa có gì bị ghi đè. Bản nháp của bạn vẫn được giữ trên màn hình.' : review.created ? 'Chưa biết lần đăng ký đã được lưu hay chưa. Console không tự gửi lại.' : 'Chưa biết thay đổi đã được lưu hay chưa. Console không tự gửi lại.'}</p>
        <p>{review.created ? 'Hãy kiểm tra trên máy chủ: nếu cấu hình đã có thì mở bản đã lưu, nếu chưa có thì bản nháp được giữ để bạn đăng ký lại. Nút Đăng ký tạm tắt cho đến khi kiểm tra.' : 'Hãy tải lại bản mới nhất, xem lại rồi lưu lại tường minh. Nút Lưu tạm tắt cho đến khi tải lại.'}</p>
        <div className="form-actions"><Button type="button" onClick={() => { void definitionList.reload(); }}>Tải lại danh sách</Button><Button type="button" tone="primary" onClick={() => { void reloadLatest(); }}>{review.created ? 'Kiểm tra trên máy chủ' : 'Tải lại bản mới nhất'}</Button></div></div> : null}
      <form onSubmit={submit} noValidate><fieldset className="editor-grid form-fieldset" disabled={saving} aria-busy={saving}>
        <div className="field-grid"><label>ID cấu hình (Definition ID)<input value={key} readOnly={editing !== null} aria-readonly={editing !== null ? true : undefined} aria-required="true" aria-invalid={idError ? true : undefined} aria-describedby={idError ? 'definition-id-hint definition-id-error' : 'definition-id-hint'} onChange={(event) => { setKey(event.target.value); setIdError(''); }} /></label>
          <label>Tên hiển thị<input value={name} maxLength={maxNameLength} aria-required="true" aria-invalid={nameError ? true : undefined} aria-describedby={nameError ? 'definition-name-error' : undefined} onChange={(event) => { setName(event.target.value); setNameError(''); }} /></label>
          <label>Loại tài nguyên<select value={resourceType} aria-required="true" disabled={editing !== null} onChange={(event) => { setResourceType(event.target.value); setPinChoice(''); }}><option value="">{typeList.loading && types.length === 0 ? 'Đang tải loại tài nguyên…' : 'Chọn loại tài nguyên'}</option>{types.map((type) => <option key={type.key} value={type.key} disabled={!supportedType(type.key)}>{typeOptionLabel(type.key)}</option>)}</select></label></div>
        <p id="definition-id-hint" className="feature-note">ID cấu hình là tên kỹ thuật của cấu hình này: chữ thường, số và dấu gạch ngang, không đổi sau khi tạo và không được tự sửa hộ. Đây khác với ID tài nguyên (<code>res_id</code>) dùng trong điều kiện áp dụng bên dưới. Tên hiển thị là chữ tự do (tối đa {maxNameLength} ký tự) và sửa được.</p>
        {idError ? <div id="definition-id-error" className="form-error" role="alert">{idError}</div> : null}
        {nameError ? <div id="definition-name-error" className="form-error" role="alert">{nameError}</div> : null}
        {typeList.loadError ? <div className="form-error" role="alert" aria-label="Lỗi tải loại tài nguyên">Không tải được loại tài nguyên. Dữ liệu đã nhập được giữ nguyên. <Button type="button" onClick={() => { void typeList.reload(); }}>Thử lại</Button></div> : null}
        {capabilityList.loading ? <p role="status" className="feature-note">Đang tải metadata module đã kiểm chứng…</p> : null}
        {capabilityList.loadError ? <div className="form-error" role="alert" aria-label="Lỗi tải metadata module">{capabilityList.loadError} Chưa chọn được module và chưa lưu được; dữ liệu đã nhập được giữ nguyên. <Button type="button" onClick={() => { void capabilityList.reload(); }}>Thử lại</Button></div> : null}
        <div className="field-grid"><label>Module đã kiểm chứng<select value={pin ? capabilityKey(pin) : ''} aria-required="true" disabled={typePins.length <= 1 && !sourceLost} onChange={(event) => setPinChoice(event.target.value)}>
            {typePins.length === 0 ? <option value="">Chưa chọn (cần chọn loại có module đã kiểm chứng)</option> : typePins.length > 1 || sourceLost ? <option value="">Chọn module và phiên bản</option> : null}
            {typePins.map((candidate) => <option key={capabilityKey(candidate)} value={capabilityKey(candidate)}>{pinLabel(candidate)}</option>)}</select></label>
          <label>Driver Type<input aria-label="Driver Type" readOnly value={pin ? own(driverLabels, pin.driverType) ?? pin.driverType : ''} placeholder="Theo module đã chọn" aria-describedby="driver-type-hint" /></label></div>
        <p id="driver-type-hint" className="feature-note">Driver Type lấy từ metadata của máy chủ theo module đã kiểm chứng; không tự suy từ tên Loại tài nguyên.</p>
        {pin?.source ? <dl className="pin-panel" aria-label="Pin module đã kiểm chứng">
          <div><dt>Module</dt><dd><code>{pin.source.moduleId}</code></dd></div><div><dt>Nguồn</dt><dd><code>{pin.source.sourceId}</code></dd></div>
          <div><dt>Revision</dt><dd><code>{pin.source.revision}</code></dd></div><div><dt>Thư mục con</dt><dd><code>{pin.source.subdirectory}</code></dd></div>
          <div><dt>SHA-256 nội dung</dt><dd><code>{pin.source.contentSha256}</code></dd></div>
          <div><dt>Terraform</dt><dd>{pin.terraformVersion ?? 'không rõ'}</dd></div>
          <div><dt>Provider</dt><dd>{pin.providerRequirements.map((provider) => `${provider.name} ${provider.version}`).join(', ') || 'không có'}</dd></div></dl> : null}
        {pin ? <section className="catalog-fields" aria-label="Tài khoản và nguồn phụ thuộc">
          <div className="section-header"><div><h3>Tài khoản và nguồn phụ thuộc</h3></div></div>
          {accountRequired ? <>
            <p className="feature-note">Module này chạy bằng tài khoản {typeLabel(pin.account.type ?? '')} và vùng ghi ngay trong cấu hình. Cấu hình không lấy tài khoản hay vùng từ Môi trường; mỗi cấu hình có thể dùng tài khoản và vùng khác nhau. Vùng xác minh của tài khoản chỉ là bằng chứng kiểm tra, không phải danh sách vùng được phép.</p>
            <div className="field-grid">
              <label>Tài khoản {typeLabel(pin.account.type ?? '')}<select aria-label="Tài khoản hạ tầng" aria-required="true" value={chosenAccount} disabled={accountList.loading || accountList.loadError !== ''} onChange={(event) => patchDraft({ account: event.target.value })}>
                <option value="">{accountList.loading ? 'Đang tải tài khoản…' : accountList.loadError ? 'Chưa tải được tài khoản' : compatibleAccounts.length === 0 ? 'Chưa có tài khoản sẵn sàng' : 'Chọn tài khoản'}</option>
                {selectedMissing ? <option value={chosenAccount}>{accountName(chosenAccount)} (không còn tương thích hoặc chưa sẵn sàng)</option> : null}
                {compatibleAccounts.map((account) => <option key={account.key} value={account.key}>{accountLabel(account)} · {statusLabel(account.status)} · phiên bản {account.version}</option>)}</select></label>
              <label>Vùng AWS<input aria-label="Vùng AWS" aria-required="true" value={draft.region} spellCheck={false} placeholder="ap-southeast-1" aria-describedby="definition-region-hint" onChange={(event) => patchDraft({ region: event.target.value })} /></label></div>
            <p id="definition-region-hint" className="feature-note">Vùng là chuỗi literal (không dùng placeholder) và được lưu trong Driver Inputs của cấu hình.</p>
            {accountList.loadError ? <div className="form-error" role="alert" aria-label="Lỗi tải tài khoản">{accountList.loadError} Chưa chọn được tài khoản; dữ liệu đã nhập được giữ nguyên. <Button type="button" onClick={() => { void accountList.reload(); }}>Thử lại</Button></div> : !accountList.loading && compatibleAccounts.length === 0 ? <div className="form-warning" role="note" aria-label="Chưa có tài khoản sẵn sàng">Chưa có tài khoản {typeLabel(pin.account.type ?? '')} ở trạng thái Sẵn sàng trong Tổ chức. Tạo và xác minh tài khoản ở trang Tài khoản hạ tầng trước.</div> : null}
            {selectedMissing && !accountList.loading ? <div className="form-warning" role="alert">Tài khoản đã chọn ({chosenAccount}) không còn tương thích hoặc chưa Sẵn sàng. Chọn tài khoản khác; máy chủ cũng kiểm tra lại khi lưu và khi lập kế hoạch.</div> : null}
          </> : <p className="feature-note" role="note">Module này không chọn tài khoản. {inheritsCluster(pin) ? 'Truy cập cluster được kế thừa từ tài nguyên phụ thuộc, không chọn lại tài khoản.' : 'Không dùng tài khoản hạ tầng.'}</p>}
          {clusterSources.map((source) => <p key={source.provider} className="feature-note" data-cluster-source={source.provider}>Nguồn cluster của provider <code>{source.provider}</code>: <strong>{source.label}</strong>.</p>)}
        </section> : null}
        {pin ? <section className="catalog-fields" aria-label="Ánh xạ module">
          <div className="section-header"><div><h3>Ánh xạ module</h3><p>Đầu vào/đầu ra giữa Loại tài nguyên và module, cùng provider binding. Chỉ chọn trong những gì module đã kiểm chứng công bố.</p></div>
            <div className="form-actions"><Button type="button" aria-label="Ánh xạ: biểu mẫu" aria-pressed={draft.mapMode === 'typed'} onClick={() => switchMapping('typed')}>Biểu mẫu</Button><Button type="button" aria-label="Ánh xạ: JSON nâng cao" aria-pressed={draft.mapMode === 'json'} onClick={() => switchMapping('json')}>JSON nâng cao</Button></div></div>
          {draft.mapNote ? <div className="form-warning" role="note">{draft.mapNote}</div> : null}
          {!typeContract ? <p className="feature-note" role="status">Đang chờ hợp đồng của Loại tài nguyên.</p> : draft.mapMode === 'typed'
            ? <MappingEditor capability={pin} type={typeContract} clusterClasses={clusterClasses} draft={draft.map} onChange={(map) => patchDraft({ map })} problems={problems} disabled={saving} />
            : <><label>Ánh xạ module (JSON object: input_mapping, output_mapping, provider_bindings)<textarea aria-label="Ánh xạ module JSON" value={draft.mapJSON} rows={8} spellCheck={false} onChange={(event) => patchDraft({ mapJSON: event.target.value, mapNote: '' })} /></label>
              <p className="feature-note">JSON hợp lệ vẫn được máy chủ kiểm tra nghiêm ngặt: biến, output, kiểu và provider phải có trong module đã kiểm chứng; provider AWS dùng <code>{'{"target": "account"}'}</code>; không nhận endpoint hay credential.</p></>}
        </section> : null}
        {pin ? <section className="catalog-fields" aria-label="Tham số cấu hình">
          <div className="section-header"><div><h3>Tham số cấu hình</h3><p>Mỗi tham số chỉ được gửi khi bạn chọn ghi đè; tham số bỏ trống giữ mặc định của module hoặc giá trị từ Loại tài nguyên. Giá trị là một placeholder nguyên vẹn, ví dụ <code>{'${context.env.namespace}'}</code>, được kiểm tra khi lập kế hoạch. Không nhận credential hoặc tham chiếu bí mật.</p></div>
            <div className="form-actions"><Button type="button" aria-label="Tham số: biểu mẫu" aria-pressed={draft.varMode === 'typed'} onClick={() => switchVariables('typed')}>Biểu mẫu</Button><Button type="button" aria-label="Tham số: JSON nâng cao" aria-pressed={draft.varMode === 'json'} onClick={() => switchVariables('json')}>JSON nâng cao</Button></div></div>
          {pin.executorOwned.length > 0 ? <p className="feature-note">Hệ thống tự cung cấp, không có ô nhập: {pin.executorOwned.map((name) => <code key={name}>{name} </code>)}</p> : null}
          {draft.varsNote ? <div className="form-warning" role="note">{draft.varsNote}</div> : null}
          {draft.varMode === 'typed' && missingRequired(draft.vars, variableList, covered).length > 0 ? <p className="feature-note">Cần nhập trước khi lưu (không có mặc định và không có tham số của Loại cung cấp): {missingRequired(draft.vars, variableList, covered).map((variable) => variable.name).join(', ')}. Máy chủ vẫn kiểm tra khi lưu.</p> : null}
          {draft.varMode === 'typed' ? <TypedVariablesEditor variables={variableList} draft={draft.vars} onChange={(vars) => patchDraft({ vars })} disabled={saving} mapped={mapped} covered={covered} />
            : <><label>Tham số cấu hình (JSON object)<textarea value={draft.varsJSON} rows={6} spellCheck={false} aria-describedby="driver-variables-hint" onChange={(event) => patchDraft({ varsJSON: event.target.value, varsNote: '' })} /></label>
              <p id="driver-variables-hint" className="feature-note">Chỉ nhận tham số mà module đã chọn khai báo, và giá trị literal phải đúng kiểu khai báo.{accountRequired ? ` Vùng AWS (${regionName}) nhập ở ô riêng, không nhập ở đây.` : ''} JSON hợp lệ vẫn được máy chủ kiểm tra nghiêm ngặt.</p></>}
        </section> : null}
        {pin ? <section className="catalog-fields" aria-label="Quy tắc tạo tài nguyên liên quan">
          <div className="section-header"><div><h3>Quy tắc tạo tài nguyên liên quan</h3></div>
            <div className="form-actions"><Button type="button" aria-label="Quy tắc: biểu mẫu" aria-pressed={draft.provMode === 'typed'} onClick={() => switchProvision('typed')}>Biểu mẫu</Button><Button type="button" aria-label="Quy tắc: JSON nâng cao" aria-pressed={draft.provMode === 'json'} onClick={() => switchProvision('json')}>JSON nâng cao</Button></div></div>
          {draft.provNote ? <div className="form-warning" role="note">{draft.provNote}</div> : null}
          {draft.provMode === 'typed' ? <ProvisionEditor rows={draft.rows} onChange={(rows) => patchDraft({ rows })} disabled={saving} />
            : <label>Quy tắc tạo tài nguyên liên quan (JSON object)<textarea value={draft.provJSON} rows={4} spellCheck={false} onChange={(event) => patchDraft({ provJSON: event.target.value, provNote: '' })} /></label>}
        </section> : null}
        <CriteriaEditor draft={criteriaDraft} onChange={setCriteriaDraft} applications={{ items: applicationList.items, loading: applicationList.loading, error: applicationList.loadError, onRetry: () => { void applicationList.reload(); } }} sources={{ items: definitions, loading: definitionList.loading, error: definitionList.loadError, onRetry: () => { void definitionList.reload(); } }} resourceType={resourceType} matching={matching} scopeKey={previewScope} excludeKey={editing?.key ?? ''} />
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saveDisabled}>{saving ? (editing ? 'Đang lưu…' : 'Đang đăng ký…') : editing ? 'Lưu thay đổi' : 'Đăng ký cấu hình tài nguyên'}</Button></div>
      </fieldset></form>
    </section>
  </section>;
}
