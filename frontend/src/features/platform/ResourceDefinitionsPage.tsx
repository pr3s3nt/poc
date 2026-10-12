import { useEffect, useRef, useState, type FormEvent } from 'react';
import { specificGuidance, fieldNote } from '../../shared/api/errors';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { snapshotKey, type ApiEnvironment } from './environmentSnapshot';
import { CriteriaEditor, draftToCriteria, emptyDraft, type ApplicationOption, type Criterion, type CriteriaDraft } from './CriteriaEditor';
import type { MatchingCandidateInput } from './MatchingPreviewPanel';
import { useCatalogList } from './useCatalogList';
import { own } from './resourceSupport';
import { capabilitiesForType, loadCapabilities, pairKey, type Capability, type Driver } from './capabilities';
import { TypedVariablesEditor } from './TypedVariablesEditor';
import { ProvisionEditor } from './ProvisionEditor';
import { draftToVariables, emptyVariableDraft, missingRequired, parseObjectText, variablesToDraft, type VariableDraft } from './typedVariables';
import { provisionToRows, rowsToProvision, type ProvisionRow } from './typedProvision';

type ResourceType = { key: string };
type APIApplication = { key: string; name?: string; environments?: ApiEnvironment[] };
type Definition = { key: string; resourceType: string; executionProfile?: string; driverType: string; criteria?: Partial<Criterion>[] };
type Connection = { key: string; name?: string; kind: string; status: string };
const driverLabels: Record<string, string> = { terraform: 'Tạo trên AWS (Terraform)', kubernetes: 'Tạo trong cluster (Kubernetes)', 'existing-cluster': 'Cluster có sẵn (existing-cluster)' };
const scopeLabels: Record<string, string> = { 'aws-eks': 'AWS', 'internal-k8s': 'Cluster nội bộ', '': 'Dùng chung' };
const idShape = /^[a-z0-9-]+$/;

// Draft of one Resource Type/driver pair: switching pair keeps each pair's own
// edits (UC-03 BR-24); only the submitted pair is reset after a commit.
type Mode = 'typed' | 'json';
type PairDraft = { varMode: Mode; vars: VariableDraft; varsJSON: string; varsNote: string; provMode: Mode; rows: ProvisionRow[]; provJSON: string; provNote: string };
const initialDraft = (): PairDraft => ({ varMode: 'typed', vars: emptyVariableDraft(), varsJSON: '{}', varsNote: '', provMode: 'typed', rows: [], provJSON: '{}', provNote: '' });

// Specific guidance comes from the stable error code; raw backend text is never shown.
function describeFailure(reason: unknown): string {
  if (!(reason instanceof ApiError)) return 'Chưa nhận được phản hồi từ máy chủ nên chưa rõ kết quả: cấu hình có thể đã được đăng ký. Hãy tải lại danh sách trước khi thử lại.';
  const field = fieldNote(reason.field);
  const specific = specificGuidance(reason.code, reason.message, reason.field);
  if (specific) return `Máy chủ từ chối cấu hình. ${specific}`;
  switch (reason.status) {
    case 400: return `Máy chủ từ chối cấu hình này. Kiểm tra ID, Tham số cấu hình, Quy tắc tạo tài nguyên liên quan và Kết nối rồi thử lại.${field}`;
    case 401: return 'Phiên đăng nhập đã hết hạn. Hãy đăng nhập lại.';
    case 403: return 'Bạn không có quyền đăng ký cấu hình tài nguyên. Chỉ Kỹ sư nền tảng hoặc Quản trị viên được đăng ký.';
    case 409: return 'ID cấu hình đã tồn tại. Hãy đổi ID rồi thử lại.';
    case 413: return 'Nội dung cấu hình quá lớn. Hãy rút gọn rồi thử lại.';
    default: return 'Máy chủ gặp lỗi khi đăng ký cấu hình. Dữ liệu đã nhập được giữ lại; hãy thử lại sau.';
  }
}
const listFailure = () => 'Không tải được dữ liệu từ máy chủ.';
const capabilityFailure = () => 'Không tải được metadata cách tạo được hỗ trợ từ máy chủ.';

const loadTypes = async () => (await api<{ resourceTypes: ResourceType[] }>('/resource-types')).resourceTypes ?? [];
const loadDefinitions = async () => (await api<{ resourceDefinitions: Definition[] }>('/resource-definitions')).resourceDefinitions ?? [];
const loadConnections = async () => (await api<{ connections: Connection[] }>('/connections')).connections ?? [];
// Real application/environment IDs (keys) for the criteria editor; never derived from names.
const loadApplications = async (): Promise<ApplicationOption[]> => ((await api<{ applications: APIApplication[] }>('/applications')).applications ?? []).map((app) => ({ key: app.key, name: app.name ?? app.key, environments: (app.environments ?? []).map((env) => ({ key: env.key, name: env.name ?? env.key, environmentType: env.environmentType ?? '', executionProfile: env.executionProfile ?? '', snapshotKey: snapshotKey(env) })) }));
const connectionLabel = (connection: Connection) => (connection.name && connection.name !== connection.key ? `${connection.name} (${connection.key})` : connection.key);

// initialResourceType comes from the Resource Types CTA query (UC-03 UI): it is
// applied once, after the catalog and the capability metadata confirm a supported
// pair, and never over a choice.
export function ResourceDefinitionsPage({ initialResourceType = '' }: { initialResourceType?: string } = {}) {
  const typeList = useCatalogList(loadTypes, listFailure);
  const capabilityList = useCatalogList(loadCapabilities, capabilityFailure);
  const definitionList = useCatalogList(loadDefinitions, listFailure);
  const connectionList = useCatalogList(loadConnections, listFailure);
  const applicationList = useCatalogList(loadApplications, listFailure);
  const types = typeList.items.filter((type) => type.key !== 'workload');
  const definitions = definitionList.items.filter((definition) => definition.resourceType !== 'workload' && definition.driverType !== 'score-k8s');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [idError, setIdError] = useState('');
  const [notice, setNotice] = useState('');
  const [advanced, setAdvanced] = useState(false);
  const [key, setKey] = useState('');
  const [resourceType, setResourceType] = useState('');
  // Per-driver choices survive a type/driver change; only the effective driver's are sent.
  const [driverChoice, setDriverChoice] = useState<Driver>('kubernetes');
  const [kubeProfile, setKubeProfile] = useState('internal-k8s');
  const [awsConnection, setAwsConnection] = useState('');
  const [kubeConnection, setKubeConnection] = useState('');
  const [drafts, setDrafts] = useState<Record<string, PairDraft>>({});
  const [criteriaDraft, setCriteriaDraft] = useState<CriteriaDraft>(emptyDraft);

  // The metadata is authoritative: while it loads or fails nothing is "supported" and nothing is guessed.
  const metadataReady = !capabilityList.loading && !capabilityList.loadError;
  const typePairs = metadataReady ? capabilitiesForType(capabilityList.items, resourceType) : [];
  const supportedType = (typeKey: string) => metadataReady && capabilitiesForType(capabilityList.items, typeKey).length > 0;

  const prefilled = useRef(false);
  useEffect(() => {
    if (prefilled.current || typeList.loading || typeList.loadError || capabilityList.loading || capabilityList.loadError) return;
    prefilled.current = true;
    if (initialResourceType !== '' && typeList.items.some((type) => type.key === initialResourceType) && capabilitiesForType(capabilityList.items, initialResourceType).length > 0) setResourceType((current) => (current === '' ? initialResourceType : current));
  }, [typeList.loading, typeList.loadError, typeList.items, capabilityList.loading, capabilityList.loadError, capabilityList.items, initialResourceType]);

  const pair: Capability | undefined = typePairs.length === 0 ? undefined : typePairs.length === 1 ? typePairs[0] : (typePairs.find((candidate) => candidate.driverType === driverChoice) ?? typePairs[0]);
  const driver: Driver | '' = pair?.driverType ?? '';
  const module = pair?.module ?? '';
  const draftKey = pair ? pairKey(pair.resourceType, pair.driverType) : '';
  const draft = drafts[draftKey] ?? initialDraft();
  const patchDraft = (change: Partial<PairDraft>) => { if (draftKey) setDrafts((current) => ({ ...current, [draftKey]: { ...initialDraft(), ...current[draftKey], ...change } })); };
  // Terraform VPC/EKS always run on the Environment's AWS Connection (UC-03 BR-08); only Aurora takes its own account.
  const ownsAwsAccount = driver === 'terraform' && module === 'aurora';
  const inheritsEnvironmentAccount = driver === 'terraform' && !ownsAwsAccount;
  const awsConnections = connectionList.items.filter((connection) => connection.status === 'READY' && connection.kind === 'AWS');
  const kubeConnections = connectionList.items.filter((connection) => connection.status === 'READY' && connection.kind === 'KUBERNETES');
  const connectionsPending = connectionList.loading && connectionList.items.length === 0;

  function switchVariables(target: Mode) {
    if (!pair || target === draft.varMode) return;
    if (target === 'json') {
      const built = draftToVariables(draft.vars, pair.variables);
      if (!built.value) { patchDraft({ varsNote: built.problem ?? '' }); return; }
      patchDraft({ varMode: 'json', varsJSON: JSON.stringify(built.value, null, 2), varsNote: '' });
      return;
    }
    const parsed = parseObjectText(draft.varsJSON, 'Tham số cấu hình');
    if (!parsed.value) { patchDraft({ varsNote: `${parsed.error ?? ''} Sửa JSON trước khi chuyển sang biểu mẫu.` }); return; }
    const converted = variablesToDraft(parsed.value, pair.variables);
    if (!converted.draft) { patchDraft({ varsNote: converted.reason ?? '' }); return; }
    patchDraft({ varMode: 'typed', vars: converted.draft, varsNote: '' });
  }
  function switchProvision(target: Mode) {
    if (!pair || target === draft.provMode) return;
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

  // Both modes serialize to the same driverInputs.values.variables / provision objects.
  function buildInputs(): { variables?: Record<string, unknown>; provision?: Record<string, unknown>; problem?: string } {
    if (!pair) return { problem: 'Chọn một Loại tài nguyên được hỗ trợ.' };
    let variables: Record<string, unknown> | undefined, provision: Record<string, unknown> | undefined;
    if (draft.varMode === 'typed') {
      const built = draftToVariables(draft.vars, pair.variables);
      if (!built.value) return { problem: built.problem };
      variables = built.value;
    } else {
      const parsed = parseObjectText(draft.varsJSON, 'Tham số cấu hình');
      if (!parsed.value) return { problem: parsed.error };
      variables = parsed.value;
    }
    if (draft.provMode === 'typed') {
      const built = rowsToProvision(draft.rows);
      if (!built.value) return { problem: built.problem };
      provision = built.value;
    } else {
      const parsed = parseObjectText(draft.provJSON, 'Quy tắc tạo tài nguyên liên quan');
      if (!parsed.value) return { problem: parsed.error };
      provision = parsed.value;
    }
    return { variables, provision };
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    setError(''); setIdError(''); setNotice('');
    // The backend owns the ID rule (UC-02 BR-05); the ID is checked, never changed.
    if (key === '') { setIdError('Nhập ID cấu hình.'); return; }
    if (!idShape.test(key)) { setIdError('ID chỉ gồm chữ thường a-z, số 0-9 và dấu gạch ngang. ID không được tự sửa hộ.'); return; }
    if (!pair || !driver) { setError(metadataReady ? 'Chọn một Loại tài nguyên được hỗ trợ.' : 'Chưa xác nhận được cách tạo được hỗ trợ từ máy chủ. Tải lại metadata rồi thử lại.'); return; }
    const inputs = buildInputs();
    if (!inputs.variables || !inputs.provision) { setError(inputs.problem ?? ''); return; }
    let connectionKey = '';
    if (ownsAwsAccount) {
      if (!awsConnections.some((connection) => connection.key === awsConnection)) { setError('Chọn một Kết nối AWS sẵn sàng. Aurora cần Kết nối AWS riêng, tường minh.'); return; }
      connectionKey = awsConnection;
    } else if (driver === 'kubernetes' && kubeConnection !== '') {
      if (!kubeConnections.some((connection) => connection.key === kubeConnection)) { setAdvanced(true); setError('Kết nối riêng đã chọn không còn sẵn sàng. Chọn lại hoặc dùng Kết nối của Môi trường.'); return; }
      connectionKey = kubeConnection;
    }
    const { criteria: cleanCriteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) { setError(problem); return; }
    const values: Record<string, unknown> = { variables: inputs.variables };
    if (driver === 'terraform') values.source = { module };
    const profile = driver === 'terraform' ? 'aws-eks' : kubeProfile;
    const submitted = key;
    const submittedDraft = draftKey;
    setSaving(true);
    try {
      await api('/resource-definitions', { method: 'POST', body: JSON.stringify({ key: submitted, resourceType, executionProfile: profile, driverType: driver, connectionKey, driverInputs: { values }, provision: inputs.provision, criteria: cleanCriteria }) });
    } catch (reason) { setError(describeFailure(reason)); setSaving(false); return; }
    // Committed: report it and reset the submitted pair's draft before reloading.
    setNotice(`Đã đăng ký cấu hình tài nguyên ${submitted}.`); setKey(''); setDrafts((current) => { const next = { ...current }; delete next[submittedDraft]; return next; }); setCriteriaDraft(emptyDraft()); setSaving(false);
    await definitionList.reload();
  }

  // Full unsaved candidate for the exact matching preview: same shape as registration, built without side effects.
  const matching = ((): MatchingCandidateInput => {
    if (key === '' || !idShape.test(key)) return { definition: null, problem: 'Nhập ID cấu hình hợp lệ để kiểm tra matching chính xác.' };
    if (!pair || !driver) return { definition: null, problem: 'Chọn Loại tài nguyên được hỗ trợ để kiểm tra matching chính xác.' };
    const inputs = buildInputs();
    if (!inputs.variables || !inputs.provision) return { definition: null, problem: inputs.problem ?? '' };
    const { criteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) return { definition: null, problem };
    const values: Record<string, unknown> = { variables: inputs.variables };
    if (driver === 'terraform') values.source = { module };
    const connectionKey = ownsAwsAccount ? awsConnection : driver === 'kubernetes' ? kubeConnection : '';
    return { definition: { key, resourceType, executionProfile: driver === 'terraform' ? 'aws-eks' : kubeProfile, driverType: driver, connectionKey, driverInputs: { values }, provision: inputs.provision, criteria }, problem: '' };
  })();

  const listError = definitionList.loadError;
  const typeOptionLabel = (typeKey: string) => (capabilityList.loading ? `${typeKey} (đang kiểm tra hỗ trợ…)` : capabilityList.loadError ? `${typeKey} (chưa xác nhận hỗ trợ)` : supportedType(typeKey) ? typeKey : `${typeKey} (chưa hỗ trợ đăng ký)`);
  return <section className="page"><header className="page-header"><div><p className="eyebrow">Nền tảng</p><h1>Cấu hình tài nguyên</h1><p>Mô tả cách tạo tài nguyên và điều kiện áp dụng. Chỉ Kỹ sư nền tảng hoặc Quản trị viên được đăng ký.</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>Cấu hình đã đăng ký</h2></div>{listError ? <div className="form-error" role="alert" aria-label="Lỗi tải danh sách">{listError} <Button type="button" onClick={() => { void definitionList.reload(); }}>Thử lại</Button></div> : definitionList.loading ? <p>Đang tải danh sách cấu hình tài nguyên…</p> : definitions.length === 0 ? <p>Chưa có cấu hình tài nguyên nào.</p> : <div className="catalog-list">{definitions.map((definition) => <div key={definition.key} className="catalog-entry"><strong>{definition.key}</strong><span>{definition.resourceType} · {own(scopeLabels, definition.executionProfile ?? '') ?? definition.executionProfile} · {own(driverLabels, definition.driverType) ?? definition.driverType} · {definition.criteria?.length ?? 0} điều kiện áp dụng{definition.driverType === 'existing-cluster' ? <> · <strong>Hệ thống</strong> (chỉ đọc)</> : null}</span></div>)}</div>}</section>
    <section className="content-panel"><div className="section-header"><div><h2>Đăng ký cấu hình tài nguyên</h2><p>Chỉ nhận cách tạo mà hệ thống thực thi được: Terraform hoặc Kubernetes.</p></div></div>
      <form onSubmit={submit} noValidate><fieldset className="editor-grid form-fieldset" disabled={saving} aria-busy={saving}>
        <div className="field-grid"><label>ID cấu hình<input value={key} aria-required="true" aria-invalid={idError ? true : undefined} aria-describedby={idError ? 'definition-id-hint definition-id-error' : 'definition-id-hint'} onChange={(event) => { setKey(event.target.value); setIdError(''); }} /></label>
          <label>Loại tài nguyên<select value={resourceType} aria-required="true" onChange={(event) => setResourceType(event.target.value)}><option value="">{typeList.loading && types.length === 0 ? 'Đang tải loại tài nguyên…' : 'Chọn loại tài nguyên'}</option>{types.map((type) => <option key={type.key} value={type.key} disabled={!supportedType(type.key)}>{typeOptionLabel(type.key)}</option>)}</select></label></div>
        <p id="definition-id-hint" className="feature-note">Dùng chữ thường, số và dấu gạch ngang. ID không được tự sửa hộ.</p>
        {idError ? <div id="definition-id-error" className="form-error" role="alert">{idError}</div> : null}
        {typeList.loadError ? <div className="form-error" role="alert" aria-label="Lỗi tải loại tài nguyên">Không tải được loại tài nguyên. Dữ liệu đã nhập được giữ nguyên. <Button type="button" onClick={() => { void typeList.reload(); }}>Thử lại</Button></div> : null}
        {capabilityList.loading ? <p role="status" className="feature-note">Đang tải metadata cách tạo được hỗ trợ…</p> : null}
        {capabilityList.loadError ? <div className="form-error" role="alert" aria-label="Lỗi tải metadata cách tạo">{capabilityList.loadError} Chưa chọn được loại tài nguyên và chưa đăng ký được; dữ liệu đã nhập được giữ nguyên. <Button type="button" onClick={() => { void capabilityList.reload(); }}>Thử lại</Button></div> : null}
        <div className="field-grid"><label>Cách tạo<select value={driver} disabled={!pair || typePairs.length === 1} onChange={(event) => setDriverChoice(event.target.value as Driver)}>{!pair ? <option value="">Chưa chọn (cần chọn loại trước)</option> : typePairs.map((item) => <option key={item.driverType} value={item.driverType}>{own(driverLabels, item.driverType)}</option>)}</select></label>
          {driver === 'terraform' ? <label>Phạm vi triển khai<select value="aws-eks" disabled onChange={() => undefined}><option value="aws-eks">AWS (aws-eks)</option></select></label>
            : <label>Phạm vi triển khai<select value={kubeProfile} disabled={driver === ''} onChange={(event) => setKubeProfile(event.target.value)}><option value="internal-k8s">Cluster nội bộ (internal-k8s)</option><option value="">Dùng chung</option></select></label>}</div>
        {driver === 'terraform' ? <p className="feature-note">Module Terraform nhúng: <code>{module}</code>. Phạm vi triển khai được khóa là aws-eks.</p> : null}
        {driver === 'kubernetes' && kubeProfile === '' ? <div className="form-warning" role="note">Dùng chung được xét ở mọi phạm vi triển khai nhưng vẫn phải thỏa điều kiện áp dụng. Cấu hình này có thể trùng mức ưu tiên với cấu hình khác; hệ thống không phát hiện hết mọi trường hợp trùng.</div> : null}
        {inheritsEnvironmentAccount ? <p className="feature-note" role="note">Tài nguyên này dùng Kết nối AWS của Môi trường khi Xem trước hoặc Triển khai; cấu hình không chọn Kết nối AWS riêng. Đăng ký không cần Môi trường.</p> : null}
        {ownsAwsAccount ? <><label>Kết nối AWS<select value={awsConnection} aria-required="true" aria-describedby="aws-connection-hint" onChange={(event) => setAwsConnection(event.target.value)}><option value="">{connectionsPending ? 'Đang tải kết nối…' : 'Chọn Kết nối AWS'}</option>{awsConnections.map((connection) => <option key={connection.key} value={connection.key}>{connectionLabel(connection)}</option>)}</select></label>
          <p id="aws-connection-hint" className="feature-note">Aurora cần một Kết nối AWS riêng, sẵn sàng; VPC và EKS không dùng ô này. Form chỉ liệt kê Kết nối AWS sẵn sàng của Tổ chức và không tự chọn hộ.{!connectionList.loading && !connectionList.loadError && awsConnections.length === 0 ? ' Hiện chưa có Kết nối AWS sẵn sàng.' : ''}</p></> : null}
        {connectionList.loadError && !inheritsEnvironmentAccount ? <div className="form-error" role="alert" aria-label="Lỗi tải kết nối">Không tải được danh sách kết nối. Dữ liệu đã nhập được giữ nguyên. <Button type="button" onClick={() => { void connectionList.reload(); }}>Thử lại</Button></div> : null}
        {pair ? <section className="catalog-fields" aria-label="Tham số cấu hình">
          <div className="section-header"><div><h3>Tham số cấu hình</h3><p>Mỗi tham số chỉ được gửi khi bạn chọn ghi đè; tham số bỏ trống giữ mặc định của cách tạo hoặc giá trị từ Loại tài nguyên. Giá trị là một placeholder nguyên vẹn, ví dụ <code>{'${context.env.namespace}'}</code>, được kiểm tra khi lập kế hoạch. Không nhận credential hoặc tham chiếu bí mật.</p></div>
            <div className="form-actions"><Button type="button" aria-label="Tham số: biểu mẫu" aria-pressed={draft.varMode === 'typed'} onClick={() => switchVariables('typed')}>Biểu mẫu</Button><Button type="button" aria-label="Tham số: JSON nâng cao" aria-pressed={draft.varMode === 'json'} onClick={() => switchVariables('json')}>JSON nâng cao</Button></div></div>
          {pair.executorOwned.length > 0 ? <p className="feature-note">Hệ thống tự cung cấp, không có ô nhập: {pair.executorOwned.map((name) => <code key={name}>{name} </code>)}</p> : null}
          {draft.varsNote ? <div className="form-warning" role="note">{draft.varsNote}</div> : null}
          {draft.varMode === 'typed' && missingRequired(draft.vars, pair.variables).length > 0 ? <p className="feature-note">Cần nhập trước khi đăng ký (không có mặc định và Loại tài nguyên không cung cấp): {missingRequired(draft.vars, pair.variables).map((variable) => variable.name).join(', ')}. Máy chủ vẫn kiểm tra khi đăng ký.</p> : null}
          {draft.varMode === 'typed' ? <TypedVariablesEditor variables={pair.variables} draft={draft.vars} onChange={(vars) => patchDraft({ vars })} disabled={saving} />
            : <><label>Tham số cấu hình (JSON object)<textarea value={draft.varsJSON} rows={6} spellCheck={false} aria-describedby="driver-variables-hint" onChange={(event) => patchDraft({ varsJSON: event.target.value, varsNote: '' })} /></label>
              <p id="driver-variables-hint" className="feature-note">Chỉ nhận tham số mà cách tạo đã chọn hỗ trợ, và giá trị literal phải đúng kiểu khai báo. JSON hợp lệ vẫn được máy chủ kiểm tra nghiêm ngặt.</p></>}
        </section> : null}
        {pair ? <section className="catalog-fields" aria-label="Quy tắc tạo tài nguyên liên quan">
          <div className="section-header"><div><h3>Quy tắc tạo tài nguyên liên quan</h3></div>
            <div className="form-actions"><Button type="button" aria-label="Quy tắc: biểu mẫu" aria-pressed={draft.provMode === 'typed'} onClick={() => switchProvision('typed')}>Biểu mẫu</Button><Button type="button" aria-label="Quy tắc: JSON nâng cao" aria-pressed={draft.provMode === 'json'} onClick={() => switchProvision('json')}>JSON nâng cao</Button></div></div>
          {draft.provNote ? <div className="form-warning" role="note">{draft.provNote}</div> : null}
          {draft.provMode === 'typed' ? <ProvisionEditor rows={draft.rows} onChange={(rows) => patchDraft({ rows })} disabled={saving} />
            : <label>Quy tắc tạo tài nguyên liên quan (JSON object)<textarea value={draft.provJSON} rows={4} spellCheck={false} onChange={(event) => patchDraft({ provJSON: event.target.value, provNote: '' })} /></label>}
        </section> : null}
        <CriteriaEditor draft={criteriaDraft} onChange={setCriteriaDraft} applications={{ items: applicationList.items, loading: applicationList.loading, error: applicationList.loadError, onRetry: () => { void applicationList.reload(); } }} sources={{ items: definitions, loading: definitionList.loading, error: definitionList.loadError, onRetry: () => { void definitionList.reload(); } }} resourceType={resourceType} profile={driver === 'terraform' ? 'aws-eks' : kubeProfile} profileLabel={own(scopeLabels, driver === 'terraform' ? 'aws-eks' : kubeProfile)} matching={matching} />
        <div className="section-header"><div><h3>Nâng cao</h3><p>Kết nối riêng cho cluster (chỉ Kubernetes). Thu gọn không xóa dữ liệu đã nhập.</p></div><Button type="button" aria-expanded={advanced} aria-controls="definition-advanced" onClick={() => setAdvanced(!advanced)}>{advanced ? 'Ẩn nâng cao' : 'Hiện nâng cao'}</Button></div>
        <div id="definition-advanced" hidden={!advanced} className="editor-grid">
          {driver === 'kubernetes' ? <><label>Kết nối riêng cho cluster (tùy chọn)<select value={kubeConnection} aria-describedby="kube-connection-hint" onChange={(event) => setKubeConnection(event.target.value)}><option value="">Dùng Kết nối của Môi trường (mặc định)</option>{kubeConnections.map((connection) => <option key={connection.key} value={connection.key}>{connectionLabel(connection)}</option>)}</select></label>
            <p id="kube-connection-hint" className="feature-note">Để trống thì dùng Kết nối của Môi trường. Kết nối riêng không đổi cluster đích: khi lập kế hoạch, Kết nối này phải trùng với Kết nối của Môi trường, nếu không cấu hình sẽ bị từ chối.</p></> : <p className="feature-note">Không có tùy chọn nâng cao cho cách tạo này.</p>}
        </div>
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saving || types.length === 0 || !metadataReady}>{saving ? 'Đang đăng ký…' : 'Đăng ký cấu hình tài nguyên'}</Button></div>
      </fieldset></form>
    </section>
  </section>;
}
