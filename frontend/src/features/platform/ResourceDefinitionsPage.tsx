import { useState, type FormEvent } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { CriteriaEditor, draftToCriteria, emptyDraft, type ApplicationOption, type Criterion, type CriteriaDraft } from './CriteriaEditor';
import { useCatalogList } from './useCatalogList';

type ResourceType = { key: string };
type APIApplication = { key: string; name?: string; environments?: { key: string; name?: string; environmentType?: string; executionProfile?: string }[] };
type Definition = { key: string; resourceType: string; executionProfile?: string; driverType: string; criteria?: Partial<Criterion>[] };
type Connection = { key: string; name?: string; kind: string; status: string };
type Driver = 'terraform' | 'kubernetes';

// Resource Types the runtime can create today, with the drivers it accepts and
// the embedded Terraform module each one maps to (backend catalog policy).
const supported: Record<string, { drivers: Driver[]; module: string }> = {
  vpc: { drivers: ['terraform'], module: 'vpc' },
  'k8s-cluster': { drivers: ['terraform'], module: 'eks' },
  'k8s-namespace': { drivers: ['kubernetes'], module: '' },
  postgres: { drivers: ['kubernetes', 'terraform'], module: 'aurora' },
};
const driverLabels: Record<string, string> = { terraform: 'Tạo trên AWS (Terraform)', kubernetes: 'Tạo trong cluster (Kubernetes)', 'existing-cluster': 'Cluster có sẵn (existing-cluster)' };
const scopeLabels: Record<string, string> = { 'aws-eks': 'AWS', 'internal-k8s': 'Cluster nội bộ', '': 'Dùng chung' };
// Keys come from users and the API ("constructor" is a valid ID): own-property lookups only.
const own = <T,>(table: Record<string, T>, key: string): T | undefined => (Object.hasOwn(table, key) ? table[key] : undefined);
const idShape = /^[a-z0-9-]+$/;

function objectJSON(value: string, label: string): Record<string, unknown> {
  let parsed: unknown;
  try { parsed = JSON.parse(value); } catch { throw new Error(`${label} phải là JSON hợp lệ.`); }
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error(`${label} phải là một JSON object.`);
  return parsed as Record<string, unknown>;
}

// Raw backend messages are English and unstable: map by HTTP status only.
function describeFailure(reason: unknown): string {
  if (!(reason instanceof ApiError)) return 'Chưa nhận được phản hồi từ máy chủ nên chưa rõ kết quả: cấu hình có thể đã được đăng ký. Hãy tải lại danh sách trước khi thử lại.';
  const field = reason.field ? ` Trường liên quan: ${reason.field}.` : '';
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

const loadTypes = async () => (await api<{ resourceTypes: ResourceType[] }>('/resource-types')).resourceTypes ?? [];
const loadDefinitions = async () => (await api<{ resourceDefinitions: Definition[] }>('/resource-definitions')).resourceDefinitions ?? [];
const loadConnections = async () => (await api<{ connections: Connection[] }>('/connections')).connections ?? [];
// Real application/environment IDs (keys) for the criteria editor; never derived from names.
const loadApplications = async (): Promise<ApplicationOption[]> => ((await api<{ applications: APIApplication[] }>('/applications')).applications ?? []).map((app) => ({ key: app.key, name: app.name ?? app.key, environments: (app.environments ?? []).map((env) => ({ key: env.key, name: env.name ?? env.key, environmentType: env.environmentType ?? '', executionProfile: env.executionProfile ?? '' })) }));
const connectionLabel = (connection: Connection) => (connection.name && connection.name !== connection.key ? `${connection.name} (${connection.key})` : connection.key);

export function ResourceDefinitionsPage() {
  const typeList = useCatalogList(loadTypes, listFailure);
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
  const [postgresDriver, setPostgresDriver] = useState<Driver>('kubernetes');
  const [kubeProfile, setKubeProfile] = useState('internal-k8s');
  const [awsConnection, setAwsConnection] = useState('');
  const [kubeConnection, setKubeConnection] = useState('');
  const [variables, setVariables] = useState('{}');
  const [provision, setProvision] = useState('{}');
  const [criteriaDraft, setCriteriaDraft] = useState<CriteriaDraft>(emptyDraft);

  const rule = own(supported, resourceType);
  const driver: Driver | '' = rule ? (rule.drivers.length === 1 ? (rule.drivers[0] ?? '') : postgresDriver) : '';
  const awsConnections = connectionList.items.filter((connection) => connection.status === 'READY' && connection.kind === 'AWS');
  const kubeConnections = connectionList.items.filter((connection) => connection.status === 'READY' && connection.kind === 'KUBERNETES');
  const connectionsPending = connectionList.loading && connectionList.items.length === 0;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    setError(''); setIdError(''); setNotice('');
    // The backend owns the ID rule (UC-02 BR-05); the ID is checked, never changed.
    if (key === '') { setIdError('Nhập ID cấu hình.'); return; }
    if (!idShape.test(key)) { setIdError('ID chỉ gồm chữ thường a-z, số 0-9 và dấu gạch ngang. ID không được tự sửa hộ.'); return; }
    if (!rule || !driver) { setError('Chọn một Loại tài nguyên được hỗ trợ.'); return; }
    let inputVariables: Record<string, unknown>, provisionRules: Record<string, unknown>;
    try { inputVariables = objectJSON(variables, 'Tham số cấu hình'); provisionRules = objectJSON(provision, 'Quy tắc tạo tài nguyên liên quan'); }
    catch (reason) { setAdvanced(true); setError((reason as Error).message); return; }
    let connectionKey = '';
    if (driver === 'terraform') {
      if (!awsConnections.some((connection) => connection.key === awsConnection)) { setError('Chọn một Kết nối AWS sẵn sàng. Terraform cần Kết nối AWS tường minh.'); return; }
      connectionKey = awsConnection;
    } else if (kubeConnection !== '') {
      if (!kubeConnections.some((connection) => connection.key === kubeConnection)) { setAdvanced(true); setError('Kết nối riêng đã chọn không còn sẵn sàng. Chọn lại hoặc dùng Kết nối của Môi trường.'); return; }
      connectionKey = kubeConnection;
    }
    const { criteria: cleanCriteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) { setError(problem); return; }
    const values: Record<string, unknown> = { variables: inputVariables };
    if (driver === 'terraform') values.source = { module: rule.module };
    const profile = driver === 'terraform' ? 'aws-eks' : kubeProfile;
    const submitted = key;
    setSaving(true);
    try {
      await api('/resource-definitions', { method: 'POST', body: JSON.stringify({ key: submitted, resourceType, executionProfile: profile, driverType: driver, connectionKey, driverInputs: { values }, provision: provisionRules, criteria: cleanCriteria }) });
    } catch (reason) { setError(describeFailure(reason)); setSaving(false); return; }
    // Committed: report it and reset the submitted form before reloading.
    setNotice(`Đã đăng ký cấu hình tài nguyên ${submitted}.`); setKey(''); setVariables('{}'); setProvision('{}'); setCriteriaDraft(emptyDraft()); setSaving(false);
    await definitionList.reload();
  }

  const listError = definitionList.loadError;
  return <section className="page"><header className="page-header"><div><p className="eyebrow">Nền tảng</p><h1>Cấu hình tài nguyên</h1><p>Mô tả cách tạo tài nguyên và điều kiện áp dụng. Chỉ Kỹ sư nền tảng hoặc Quản trị viên được đăng ký.</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>Cấu hình đã đăng ký</h2></div>{listError ? <div className="form-error" role="alert" aria-label="Lỗi tải danh sách">{listError} <Button type="button" onClick={() => { void definitionList.reload(); }}>Thử lại</Button></div> : definitionList.loading ? <p>Đang tải danh sách cấu hình tài nguyên…</p> : definitions.length === 0 ? <p>Chưa có cấu hình tài nguyên nào.</p> : <div className="catalog-list">{definitions.map((definition) => <div key={definition.key} className="catalog-entry"><strong>{definition.key}</strong><span>{definition.resourceType} · {own(scopeLabels, definition.executionProfile ?? '') ?? definition.executionProfile} · {own(driverLabels, definition.driverType) ?? definition.driverType} · {definition.criteria?.length ?? 0} điều kiện áp dụng{definition.driverType === 'existing-cluster' ? <> · <strong>Hệ thống</strong> (chỉ đọc)</> : null}</span></div>)}</div>}</section>
    <section className="content-panel"><div className="section-header"><div><h2>Đăng ký cấu hình tài nguyên</h2><p>Chỉ nhận cách tạo mà hệ thống thực thi được: Terraform hoặc Kubernetes. Việc quản lý Mẫu dựng ứng dụng sẽ có ở trang Mẫu dựng ứng dụng riêng.</p></div></div>
      <form onSubmit={submit} noValidate><fieldset className="editor-grid form-fieldset" disabled={saving} aria-busy={saving}>
        <div className="field-grid"><label>ID cấu hình<input value={key} aria-required="true" aria-invalid={idError ? true : undefined} aria-describedby={idError ? 'definition-id-hint definition-id-error' : 'definition-id-hint'} onChange={(event) => { setKey(event.target.value); setIdError(''); }} /></label>
          <label>Loại tài nguyên<select value={resourceType} aria-required="true" onChange={(event) => setResourceType(event.target.value)}><option value="">{typeList.loading && types.length === 0 ? 'Đang tải loại tài nguyên…' : 'Chọn loại tài nguyên'}</option>{types.map((type) => <option key={type.key} value={type.key} disabled={!own(supported, type.key)}>{own(supported, type.key) ? type.key : `${type.key} (chưa hỗ trợ đăng ký)`}</option>)}</select></label></div>
        <p id="definition-id-hint" className="feature-note">Dùng chữ thường, số và dấu gạch ngang. ID không được tự sửa hộ.</p>
        {idError ? <div id="definition-id-error" className="form-error" role="alert">{idError}</div> : null}
        {typeList.loadError ? <div className="form-error" role="alert" aria-label="Lỗi tải loại tài nguyên">Không tải được loại tài nguyên. Dữ liệu đã nhập được giữ nguyên. <Button type="button" onClick={() => { void typeList.reload(); }}>Thử lại</Button></div> : null}
        <div className="field-grid"><label>Cách tạo<select value={driver} disabled={!rule || rule.drivers.length === 1} onChange={(event) => setPostgresDriver(event.target.value as Driver)}>{!rule ? <option value="">Chưa chọn (cần chọn loại trước)</option> : rule.drivers.map((item) => <option key={item} value={item}>{own(driverLabels, item)}</option>)}</select></label>
          {driver === 'terraform' ? <label>Phạm vi triển khai<select value="aws-eks" disabled onChange={() => undefined}><option value="aws-eks">AWS (aws-eks)</option></select></label>
            : <label>Phạm vi triển khai<select value={kubeProfile} disabled={driver === ''} onChange={(event) => setKubeProfile(event.target.value)}><option value="internal-k8s">Cluster nội bộ (internal-k8s)</option><option value="">Dùng chung</option></select></label>}</div>
        {driver === 'terraform' ? <p className="feature-note">Module Terraform nhúng: <code>{rule?.module}</code>. Phạm vi triển khai được khóa là aws-eks.</p> : null}
        {driver === 'kubernetes' && kubeProfile === '' ? <div className="form-warning" role="note">Dùng chung được xét ở mọi phạm vi triển khai nhưng vẫn phải thỏa điều kiện áp dụng. Cấu hình này có thể trùng mức ưu tiên với cấu hình khác; hệ thống không phát hiện hết mọi trường hợp trùng.</div> : null}
        {driver === 'terraform' ? <><label>Kết nối AWS<select value={awsConnection} aria-required="true" aria-describedby="aws-connection-hint" onChange={(event) => setAwsConnection(event.target.value)}><option value="">{connectionsPending ? 'Đang tải kết nối…' : 'Chọn Kết nối AWS'}</option>{awsConnections.map((connection) => <option key={connection.key} value={connection.key}>{connectionLabel(connection)}</option>)}</select></label>
          <p id="aws-connection-hint" className="feature-note">Terraform cần một Kết nối AWS sẵn sàng. Form chỉ liệt kê Kết nối AWS sẵn sàng của Tổ chức và không tự chọn hộ.{!connectionList.loading && !connectionList.loadError && awsConnections.length === 0 ? ' Hiện chưa có Kết nối AWS sẵn sàng.' : ''}</p></> : null}
        {connectionList.loadError ? <div className="form-error" role="alert" aria-label="Lỗi tải kết nối">Không tải được danh sách kết nối. Dữ liệu đã nhập được giữ nguyên. <Button type="button" onClick={() => { void connectionList.reload(); }}>Thử lại</Button></div> : null}
        <CriteriaEditor draft={criteriaDraft} onChange={setCriteriaDraft} applications={{ items: applicationList.items, loading: applicationList.loading, error: applicationList.loadError, onRetry: () => { void applicationList.reload(); } }} sources={{ items: definitions, loading: definitionList.loading, error: definitionList.loadError, onRetry: () => { void definitionList.reload(); } }} resourceType={resourceType} profile={driver === 'terraform' ? 'aws-eks' : kubeProfile} profileLabel={own(scopeLabels, driver === 'terraform' ? 'aws-eks' : kubeProfile)} />
        <div className="section-header"><div><h3>Nâng cao</h3><p>Tham số cấu hình, Quy tắc tạo tài nguyên liên quan và Kết nối riêng cho cluster. Thu gọn không xóa dữ liệu đã nhập.</p></div><Button type="button" aria-expanded={advanced} aria-controls="definition-advanced" onClick={() => setAdvanced(!advanced)}>{advanced ? 'Ẩn nâng cao' : 'Hiện nâng cao'}</Button></div>
        <div id="definition-advanced" hidden={!advanced} className="editor-grid">
          <label>Tham số cấu hình (JSON object)<textarea value={variables} rows={6} spellCheck={false} aria-describedby="driver-variables-hint" onChange={(event) => setVariables(event.target.value)} /></label>
          <p id="driver-variables-hint" className="feature-note">Chỉ nhận tham số mà cách tạo đã chọn hỗ trợ, và giá trị literal phải đúng kiểu khai báo. Giá trị là một placeholder nguyên vẹn, ví dụ <code>{'${context.app.id}'}</code>, được kiểm tra khi lập kế hoạch. Không nhận credential hoặc tham chiếu bí mật.</p>
          <label>Quy tắc tạo tài nguyên liên quan (JSON object)<textarea value={provision} rows={4} spellCheck={false} onChange={(event) => setProvision(event.target.value)} /></label>
          {driver === 'kubernetes' ? <><label>Kết nối riêng cho cluster (tùy chọn)<select value={kubeConnection} aria-describedby="kube-connection-hint" onChange={(event) => setKubeConnection(event.target.value)}><option value="">Dùng Kết nối của Môi trường (mặc định)</option>{kubeConnections.map((connection) => <option key={connection.key} value={connection.key}>{connectionLabel(connection)}</option>)}</select></label>
            <p id="kube-connection-hint" className="feature-note">Để trống thì dùng Kết nối của Môi trường. Kết nối riêng không đổi cluster đích: khi lập kế hoạch, Kết nối này phải trùng với Kết nối của Môi trường, nếu không cấu hình sẽ bị từ chối.</p></> : null}
        </div>
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saving || types.length === 0}>{saving ? 'Đang đăng ký…' : 'Đăng ký cấu hình tài nguyên'}</Button></div>
      </fieldset></form>
    </section>
  </section>;
}
