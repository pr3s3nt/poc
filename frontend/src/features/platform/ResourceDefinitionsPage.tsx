import { useEffect, useRef, useState, type FormEvent } from 'react';
import { specificGuidance, fieldNote } from '../../shared/api/errors';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { snapshotKey, type ApiEnvironment } from './environmentSnapshot';
import { CriteriaEditor, draftToCriteria, emptyDraft, type ApplicationOption, type Criterion, type CriteriaDraft } from './CriteriaEditor';
import type { MatchingCandidateInput } from './MatchingPreviewPanel';
import { useCatalogList } from './useCatalogList';
import { own } from './resourceSupport';
import { capabilitiesForType, capabilityKey, isVerifiedPin, loadCapabilities, type Capability } from './capabilities';
import { TypedVariablesEditor } from './TypedVariablesEditor';
import { ProvisionEditor } from './ProvisionEditor';
import { MappingEditor } from './MappingEditor';
import { clusterType, emptyMapping, mappingProblems, mappingToObject, needsAwsConnection, objectToMapping, profileChoices, type MappingDraft, type TypeContract } from './moduleMapping';
import { draftToVariables, emptyVariableDraft, missingRequired, parseObjectText, variablesToDraft, type JsonObject, type VariableDraft } from './typedVariables';
import { provisionToRows, rowsToProvision, type ProvisionRow } from './typedProvision';

type APIApplication = { key: string; name?: string; environments?: ApiEnvironment[] };
type Definition = { key: string; resourceType: string; executionProfile?: string; driverType: string; criteria?: Partial<Criterion>[] };
const driverLabels: Record<string, string> = { terraform: 'Terraform (module đã kiểm chứng)', kubernetes: 'Tạo trong cluster (Kubernetes)', 'existing-cluster': 'Cluster có sẵn (existing-cluster)' };
const scopeLabels: Record<string, string> = { 'aws-eks': 'AWS', 'internal-k8s': 'Cluster nội bộ', '': 'Dùng chung' };
const idShape = /^[a-z0-9-]+$/;
const unsetProfile = '__unset';

// Draft of one verified module pin: switching pin keeps each pin's own edits
// (UC-03 BR-24); only the submitted pin is reset after a commit.
type Mode = 'typed' | 'json';
type PairDraft = {
  profile: string | null;
  varMode: Mode; vars: VariableDraft; varsJSON: string; varsNote: string;
  mapMode: Mode; map: MappingDraft; mapJSON: string; mapNote: string;
  provMode: Mode; rows: ProvisionRow[]; provJSON: string; provNote: string;
};
const initialDraft = (): PairDraft => ({ profile: null, varMode: 'typed', vars: emptyVariableDraft(), varsJSON: '{}', varsNote: '', mapMode: 'typed', map: emptyMapping(), mapJSON: '{}', mapNote: '', provMode: 'typed', rows: [], provJSON: '{}', provNote: '' });

// Specific guidance comes from the stable error code; raw backend text is never shown.
function describeFailure(reason: unknown): string {
  if (!(reason instanceof ApiError)) return 'Chưa nhận được phản hồi từ máy chủ nên chưa rõ kết quả: cấu hình có thể đã được đăng ký. Hãy tải lại danh sách trước khi thử lại.';
  const field = fieldNote(reason.field);
  const specific = specificGuidance(reason.code, reason.message, reason.field);
  if (specific) return `Máy chủ từ chối cấu hình. ${specific}`;
  switch (reason.status) {
    case 400: return `Máy chủ từ chối cấu hình này. Kiểm tra ID, module đã chọn, ánh xạ, Tham số cấu hình và Quy tắc tạo tài nguyên liên quan rồi thử lại.${field}`;
    case 401: return 'Phiên đăng nhập đã hết hạn. Hãy đăng nhập lại.';
    case 403: return 'Bạn không có quyền đăng ký cấu hình tài nguyên. Chỉ Kỹ sư nền tảng hoặc Quản trị viên được đăng ký.';
    case 409: return 'ID cấu hình đã tồn tại. Hãy đổi ID rồi thử lại.';
    case 413: return 'Nội dung cấu hình quá lớn. Hãy rút gọn rồi thử lại.';
    default: return 'Máy chủ gặp lỗi khi đăng ký cấu hình. Dữ liệu đã nhập được giữ lại; hãy thử lại sau.';
  }
}
const listFailure = () => 'Không tải được dữ liệu từ máy chủ.';
const capabilityFailure = () => 'Không tải được metadata module đã kiểm chứng từ máy chủ.';

type RawType = Partial<TypeContract> & { key: string };
const loadTypes = async (): Promise<TypeContract[]> => ((await api<{ resourceTypes: RawType[] }>('/resource-types')).resourceTypes ?? []).map((type) => ({ key: type.key, inputs: type.inputs ?? [], outputs: type.outputs ?? [], classes: type.classes ?? [] }));
const loadDefinitions = async () => (await api<{ resourceDefinitions: Definition[] }>('/resource-definitions')).resourceDefinitions ?? [];
// Real application/environment IDs (keys) for the criteria editor; never derived from names.
const loadApplications = async (): Promise<ApplicationOption[]> => ((await api<{ applications: APIApplication[] }>('/applications')).applications ?? []).map((app) => ({ key: app.key, name: app.name ?? app.key, environments: (app.environments ?? []).map((env) => ({ key: env.key, name: env.name ?? env.key, environmentType: env.environmentType ?? '', executionProfile: env.executionProfile ?? '', snapshotKey: snapshotKey(env) })) }));
const shortDigest = (digest: string) => digest.slice(0, 12);
const pinLabel = (capability: Capability) => `${capability.module} · ${capability.source?.revision ?? ''} · sha256:${shortDigest(capability.source?.contentSha256 ?? '')}`;

// initialResourceType comes from the Resource Types CTA query (UC-03 UI): it is
// applied once, after the catalog and the registry metadata confirm a verified
// pair, and never over a choice.
export function ResourceDefinitionsPage({ initialResourceType = '' }: { initialResourceType?: string } = {}) {
  const typeList = useCatalogList(loadTypes, listFailure);
  const capabilityList = useCatalogList(loadCapabilities, capabilityFailure);
  const definitionList = useCatalogList(loadDefinitions, listFailure);
  const applicationList = useCatalogList(loadApplications, listFailure);
  const types = typeList.items.filter((type) => type.key !== 'workload');
  const definitions = definitionList.items.filter((definition) => definition.resourceType !== 'workload' && definition.driverType !== 'score-k8s');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [idError, setIdError] = useState('');
  const [notice, setNotice] = useState('');
  const [key, setKey] = useState('');
  const [resourceType, setResourceType] = useState('');
  const [pinChoice, setPinChoice] = useState('');
  const [drafts, setDrafts] = useState<Record<string, PairDraft>>({});
  const [criteriaDraft, setCriteriaDraft] = useState<CriteriaDraft>(emptyDraft);

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
  const pin: Capability | undefined = typePins.length === 1 ? typePins[0] : typePins.find((candidate) => capabilityKey(candidate) === pinChoice);
  const draftKey = pin ? capabilityKey(pin) : '';
  const draft = drafts[draftKey] ?? initialDraft();
  const patchDraft = (change: Partial<PairDraft>) => { if (draftKey) setDrafts((current) => ({ ...current, [draftKey]: { ...initialDraft(), ...current[draftKey], ...change } })); };
  const choices = pin ? profileChoices(pin) : [];
  const profile: string | null = choices.length === 1 ? (choices[0] ?? null) : draft.profile;

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
      const built = draftToVariables(draft.vars, pin.variables);
      if (!built.value) { patchDraft({ varsNote: built.problem ?? '' }); return; }
      patchDraft({ varMode: 'json', varsJSON: JSON.stringify(built.value, null, 2), varsNote: '' });
      return;
    }
    const parsed = parseObjectText(draft.varsJSON, 'Tham số cấu hình');
    if (!parsed.value) { patchDraft({ varsNote: `${parsed.error ?? ''} Sửa JSON trước khi chuyển sang biểu mẫu.` }); return; }
    const converted = variablesToDraft(parsed.value, pin.variables);
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
      const built = draftToVariables(draft.vars, pin.variables);
      if (!built.value) return { problem: built.problem };
      variables = built.value;
    } else {
      const parsed = parseObjectText(draft.varsJSON, 'Tham số cấu hình');
      if (!parsed.value) return { problem: parsed.error };
      variables = parsed.value;
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

  // Guidance before the POST; the server checks the same rules again.
  function guidance(): string {
    if (!pin) return metadataReady ? 'Chọn một Loại tài nguyên và module đã kiểm chứng.' : 'Chưa xác nhận được module đã kiểm chứng từ máy chủ. Tải lại metadata rồi thử lại.';
    if (!typesReady || !typeContract) return 'Chưa tải được hợp đồng của Loại tài nguyên. Tải lại rồi thử lại.';
    if (profile === null) return 'Chọn phạm vi triển khai của cấu hình.';
    if (draft.mapMode === 'typed') {
      const problem = mappingProblems(draft.map, pin, typeContract)[0];
      if (problem) return problem.message;
    }
    const uncovered = missingRequired(draft.vars, pin.variables, covered).map((variable) => variable.name);
    if (draft.varMode === 'typed' && uncovered.length > 0) return `Cần nhập trước khi đăng ký: ${uncovered.join(', ')} (bắt buộc, không có mặc định và không được tham số của Loại cung cấp).`;
    return '';
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    setError(''); setIdError(''); setNotice('');
    // The backend owns the ID rule (UC-02 BR-05); the ID is checked, never changed.
    if (key === '') { setIdError('Nhập ID cấu hình.'); return; }
    if (!idShape.test(key)) { setIdError('ID chỉ gồm chữ thường a-z, số 0-9 và dấu gạch ngang. ID không được tự sửa hộ.'); return; }
    const blocked = guidance();
    if (blocked) { setError(blocked); return; }
    const inputs = buildInputs();
    if (!inputs.values || !inputs.provision) { setError(inputs.problem ?? ''); return; }
    const { criteria: cleanCriteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) { setError(problem); return; }
    const submitted = key;
    const submittedDraft = draftKey;
    setSaving(true);
    try {
      await api('/resource-definitions', { method: 'POST', body: JSON.stringify({ key: submitted, resourceType, executionProfile: profile ?? '', driverType: 'terraform', driverInputs: { values: inputs.values }, provision: inputs.provision, criteria: cleanCriteria }) });
    } catch (reason) { setError(describeFailure(reason)); setSaving(false); return; }
    // Committed: report it and reset the submitted pin's draft before reloading.
    setNotice(`Đã đăng ký cấu hình tài nguyên ${submitted}.`); setKey(''); setDrafts((current) => { const next = { ...current }; delete next[submittedDraft]; return next; }); setCriteriaDraft(emptyDraft()); setSaving(false);
    await definitionList.reload();
  }

  // Full unsaved candidate for the exact matching preview: same shape as registration, built without side effects.
  const matching = ((): MatchingCandidateInput => {
    if (key === '' || !idShape.test(key)) return { definition: null, problem: 'Nhập ID cấu hình hợp lệ để kiểm tra matching chính xác.' };
    if (!pin) return { definition: null, problem: 'Chọn Loại tài nguyên và module đã kiểm chứng để kiểm tra matching chính xác.' };
    if (profile === null) return { definition: null, problem: 'Chọn phạm vi triển khai để kiểm tra matching chính xác.' };
    const inputs = buildInputs();
    if (!inputs.values || !inputs.provision) return { definition: null, problem: inputs.problem ?? '' };
    const { criteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) return { definition: null, problem };
    return { definition: { key, resourceType, executionProfile: profile, driverType: 'terraform', connectionKey: '', driverInputs: { values: inputs.values }, provision: inputs.provision, criteria }, problem: '' };
  })();

  const listError = definitionList.loadError;
  const typeOptionLabel = (typeKey: string) => (capabilityList.loading ? `${typeKey} (đang kiểm tra hỗ trợ…)` : capabilityList.loadError ? `${typeKey} (chưa xác nhận hỗ trợ)` : supportedType(typeKey) ? typeKey : `${typeKey} (chưa có module đã kiểm chứng)`);
  const problems = pin && draft.mapMode === 'typed' ? mappingProblems(draft.map, pin, typeContract) : [];
  return <section className="page"><header className="page-header"><div><p className="eyebrow">Nền tảng</p><h1>Cấu hình tài nguyên</h1><p>Mô tả cách tạo tài nguyên bằng module Terraform đã kiểm chứng và điều kiện áp dụng. Chỉ Kỹ sư nền tảng hoặc Quản trị viên được đăng ký.</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>Cấu hình đã đăng ký</h2></div>{listError ? <div className="form-error" role="alert" aria-label="Lỗi tải danh sách">{listError} <Button type="button" onClick={() => { void definitionList.reload(); }}>Thử lại</Button></div> : definitionList.loading ? <p>Đang tải danh sách cấu hình tài nguyên…</p> : definitions.length === 0 ? <p>Chưa có cấu hình tài nguyên nào.</p> : <div className="catalog-list">{definitions.map((definition) => <div key={definition.key} className="catalog-entry"><strong>{definition.key}</strong><span>{definition.resourceType} · {own(scopeLabels, definition.executionProfile ?? '') ?? definition.executionProfile} · {own(driverLabels, definition.driverType) ?? definition.driverType} · {definition.criteria?.length ?? 0} điều kiện áp dụng{definition.driverType === 'existing-cluster' ? <> · <strong>Hệ thống</strong> (chỉ đọc)</> : null}</span></div>)}</div>}</section>
    <section className="content-panel"><div className="section-header"><div><h2>Đăng ký cấu hình tài nguyên</h2><p>Chỉ nhận module Terraform đã được kiểm chứng trong registry. Module, phiên bản và digest lấy từ metadata của máy chủ; form không có ô nhập nguồn, URL hay digest.</p></div></div>
      <form onSubmit={submit} noValidate><fieldset className="editor-grid form-fieldset" disabled={saving} aria-busy={saving}>
        <div className="field-grid"><label>ID cấu hình<input value={key} aria-required="true" aria-invalid={idError ? true : undefined} aria-describedby={idError ? 'definition-id-hint definition-id-error' : 'definition-id-hint'} onChange={(event) => { setKey(event.target.value); setIdError(''); }} /></label>
          <label>Loại tài nguyên<select value={resourceType} aria-required="true" onChange={(event) => { setResourceType(event.target.value); setPinChoice(''); }}><option value="">{typeList.loading && types.length === 0 ? 'Đang tải loại tài nguyên…' : 'Chọn loại tài nguyên'}</option>{types.map((type) => <option key={type.key} value={type.key} disabled={!supportedType(type.key)}>{typeOptionLabel(type.key)}</option>)}</select></label></div>
        <p id="definition-id-hint" className="feature-note">Dùng chữ thường, số và dấu gạch ngang. ID không được tự sửa hộ.</p>
        {idError ? <div id="definition-id-error" className="form-error" role="alert">{idError}</div> : null}
        {typeList.loadError ? <div className="form-error" role="alert" aria-label="Lỗi tải loại tài nguyên">Không tải được loại tài nguyên. Dữ liệu đã nhập được giữ nguyên. <Button type="button" onClick={() => { void typeList.reload(); }}>Thử lại</Button></div> : null}
        {capabilityList.loading ? <p role="status" className="feature-note">Đang tải metadata module đã kiểm chứng…</p> : null}
        {capabilityList.loadError ? <div className="form-error" role="alert" aria-label="Lỗi tải metadata module">{capabilityList.loadError} Chưa chọn được module và chưa đăng ký được; dữ liệu đã nhập được giữ nguyên. <Button type="button" onClick={() => { void capabilityList.reload(); }}>Thử lại</Button></div> : null}
        <div className="field-grid"><label>Module đã kiểm chứng<select value={pin ? capabilityKey(pin) : ''} aria-required="true" disabled={typePins.length <= 1} onChange={(event) => setPinChoice(event.target.value)}>
            {typePins.length === 0 ? <option value="">Chưa chọn (cần chọn loại có module đã kiểm chứng)</option> : typePins.length > 1 ? <option value="">Chọn module và phiên bản</option> : null}
            {typePins.map((candidate) => <option key={capabilityKey(candidate)} value={capabilityKey(candidate)}>{pinLabel(candidate)}</option>)}</select></label>
          <label>Phạm vi triển khai<select value={profile === null ? unsetProfile : profile} disabled={!pin || choices.length === 1} onChange={(event) => { if (event.target.value !== unsetProfile) patchDraft({ profile: event.target.value }); }}>
            {!pin ? <option value={unsetProfile}>Chưa chọn (cần chọn module trước)</option> : choices.length === 1 ? <option value={choices[0]}>{own(scopeLabels, choices[0] ?? '')} ({choices[0]})</option> : <>
              <option value={unsetProfile} disabled>Chọn phạm vi triển khai</option><option value="internal-k8s">Cluster nội bộ (internal-k8s)</option><option value="aws-eks">AWS (aws-eks)</option><option value="">Dùng chung (mọi phạm vi)</option></>}</select></label></div>
        {pin?.source ? <dl className="pin-panel" aria-label="Pin module đã kiểm chứng">
          <div><dt>Module</dt><dd><code>{pin.source.moduleId}</code></dd></div><div><dt>Nguồn</dt><dd><code>{pin.source.sourceId}</code></dd></div>
          <div><dt>Revision</dt><dd><code>{pin.source.revision}</code></dd></div><div><dt>Thư mục con</dt><dd><code>{pin.source.subdirectory}</code></dd></div>
          <div><dt>SHA-256 nội dung</dt><dd><code>{pin.source.contentSha256}</code></dd></div>
          <div><dt>Terraform</dt><dd>{pin.terraformVersion ?? 'không rõ'}</dd></div>
          <div><dt>Provider</dt><dd>{pin.providerRequirements.map((provider) => `${provider.name} ${provider.version}`).join(', ') || 'không có'}</dd></div></dl> : null}
        {pin && needsAwsConnection(pin) ? <p className="feature-note" role="note">Tài nguyên này dùng Kết nối AWS của Môi trường khi Xem trước hoặc Triển khai (cùng tài khoản và vùng); cấu hình không chọn Kết nối AWS riêng, kể cả Aurora. Đăng ký không cần Môi trường.</p> : null}
        {profile === '' && pin ? <div className="form-warning" role="note">Dùng chung được xét ở mọi phạm vi triển khai nhưng vẫn phải thỏa điều kiện áp dụng. Cấu hình này có thể trùng mức ưu tiên với cấu hình khác; hệ thống không phát hiện hết mọi trường hợp trùng.</div> : null}
        {pin ? <section className="catalog-fields" aria-label="Ánh xạ module">
          <div className="section-header"><div><h3>Ánh xạ module</h3><p>Đầu vào/đầu ra giữa Loại tài nguyên và module, cùng provider binding. Chỉ chọn trong những gì module đã kiểm chứng công bố.</p></div>
            <div className="form-actions"><Button type="button" aria-label="Ánh xạ: biểu mẫu" aria-pressed={draft.mapMode === 'typed'} onClick={() => switchMapping('typed')}>Biểu mẫu</Button><Button type="button" aria-label="Ánh xạ: JSON nâng cao" aria-pressed={draft.mapMode === 'json'} onClick={() => switchMapping('json')}>JSON nâng cao</Button></div></div>
          {draft.mapNote ? <div className="form-warning" role="note">{draft.mapNote}</div> : null}
          {!typeContract ? <p className="feature-note" role="status">Đang chờ hợp đồng của Loại tài nguyên.</p> : draft.mapMode === 'typed'
            ? <MappingEditor capability={pin} type={typeContract} clusterClasses={clusterClasses} draft={draft.map} onChange={(map) => patchDraft({ map })} problems={problems} disabled={saving} />
            : <><label>Ánh xạ module (JSON object: input_mapping, output_mapping, provider_bindings)<textarea aria-label="Ánh xạ module JSON" value={draft.mapJSON} rows={8} spellCheck={false} onChange={(event) => patchDraft({ mapJSON: event.target.value, mapNote: '' })} /></label>
              <p className="feature-note">JSON hợp lệ vẫn được máy chủ kiểm tra nghiêm ngặt: biến, output, kiểu và provider phải có trong module đã kiểm chứng; không nhận tài khoản, vùng hay credential.</p></>}
        </section> : null}
        {pin ? <section className="catalog-fields" aria-label="Tham số cấu hình">
          <div className="section-header"><div><h3>Tham số cấu hình</h3><p>Mỗi tham số chỉ được gửi khi bạn chọn ghi đè; tham số bỏ trống giữ mặc định của module hoặc giá trị từ Loại tài nguyên. Giá trị là một placeholder nguyên vẹn, ví dụ <code>{'${context.env.namespace}'}</code>, được kiểm tra khi lập kế hoạch. Không nhận credential hoặc tham chiếu bí mật.</p></div>
            <div className="form-actions"><Button type="button" aria-label="Tham số: biểu mẫu" aria-pressed={draft.varMode === 'typed'} onClick={() => switchVariables('typed')}>Biểu mẫu</Button><Button type="button" aria-label="Tham số: JSON nâng cao" aria-pressed={draft.varMode === 'json'} onClick={() => switchVariables('json')}>JSON nâng cao</Button></div></div>
          {pin.executorOwned.length > 0 ? <p className="feature-note">Hệ thống tự cung cấp, không có ô nhập: {pin.executorOwned.map((name) => <code key={name}>{name} </code>)}</p> : null}
          {draft.varsNote ? <div className="form-warning" role="note">{draft.varsNote}</div> : null}
          {draft.varMode === 'typed' && missingRequired(draft.vars, pin.variables, covered).length > 0 ? <p className="feature-note">Cần nhập trước khi đăng ký (không có mặc định và không có tham số của Loại cung cấp): {missingRequired(draft.vars, pin.variables, covered).map((variable) => variable.name).join(', ')}. Máy chủ vẫn kiểm tra khi đăng ký.</p> : null}
          {draft.varMode === 'typed' ? <TypedVariablesEditor variables={pin.variables} draft={draft.vars} onChange={(vars) => patchDraft({ vars })} disabled={saving} mapped={mapped} covered={covered} />
            : <><label>Tham số cấu hình (JSON object)<textarea value={draft.varsJSON} rows={6} spellCheck={false} aria-describedby="driver-variables-hint" onChange={(event) => patchDraft({ varsJSON: event.target.value, varsNote: '' })} /></label>
              <p id="driver-variables-hint" className="feature-note">Chỉ nhận tham số mà module đã chọn khai báo, và giá trị literal phải đúng kiểu khai báo. JSON hợp lệ vẫn được máy chủ kiểm tra nghiêm ngặt.</p></>}
        </section> : null}
        {pin ? <section className="catalog-fields" aria-label="Quy tắc tạo tài nguyên liên quan">
          <div className="section-header"><div><h3>Quy tắc tạo tài nguyên liên quan</h3></div>
            <div className="form-actions"><Button type="button" aria-label="Quy tắc: biểu mẫu" aria-pressed={draft.provMode === 'typed'} onClick={() => switchProvision('typed')}>Biểu mẫu</Button><Button type="button" aria-label="Quy tắc: JSON nâng cao" aria-pressed={draft.provMode === 'json'} onClick={() => switchProvision('json')}>JSON nâng cao</Button></div></div>
          {draft.provNote ? <div className="form-warning" role="note">{draft.provNote}</div> : null}
          {draft.provMode === 'typed' ? <ProvisionEditor rows={draft.rows} onChange={(rows) => patchDraft({ rows })} disabled={saving} />
            : <label>Quy tắc tạo tài nguyên liên quan (JSON object)<textarea value={draft.provJSON} rows={4} spellCheck={false} onChange={(event) => patchDraft({ provJSON: event.target.value, provNote: '' })} /></label>}
        </section> : null}
        <CriteriaEditor draft={criteriaDraft} onChange={setCriteriaDraft} applications={{ items: applicationList.items, loading: applicationList.loading, error: applicationList.loadError, onRetry: () => { void applicationList.reload(); } }} sources={{ items: definitions, loading: definitionList.loading, error: definitionList.loadError, onRetry: () => { void definitionList.reload(); } }} resourceType={resourceType} profile={profile ?? ''} profileLabel={own(scopeLabels, profile ?? '')} matching={matching} />
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saving || types.length === 0 || !metadataReady}>{saving ? 'Đang đăng ký…' : 'Đăng ký cấu hình tài nguyên'}</Button></div>
      </fieldset></form>
    </section>
  </section>;
}
