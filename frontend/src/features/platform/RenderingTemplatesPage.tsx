import { useState, type FormEvent } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { CriteriaEditor, draftToCriteria, emptyDraft, type ApplicationOption, type Criterion, type CriteriaDraft } from './CriteriaEditor';
import { useCatalogList } from './useCatalogList';

type APIApplication = { key: string; name?: string; environments?: { key: string; name?: string; environmentType?: string; executionProfile?: string }[] };
type Definition = { key: string; resourceType: string; executionProfile?: string; driverType: string; driverInputs?: { values?: { variables?: { render_bundle?: unknown } } }; criteria?: Partial<Criterion>[] };

const idShape = /^[a-z0-9-]+$/;
const listFailure = () => 'Không tải được dữ liệu từ máy chủ.';
const loadDefinitions = async () => (await api<{ resourceDefinitions: Definition[] }>('/resource-definitions')).resourceDefinitions ?? [];
// Real application/environment IDs (keys) for the criteria editor; never derived from names.
const loadApplications = async (): Promise<ApplicationOption[]> => ((await api<{ applications: APIApplication[] }>('/applications')).applications ?? []).map((app) => ({ key: app.key, name: app.name ?? app.key, environments: (app.environments ?? []).map((env) => ({ key: env.key, name: env.name ?? env.key, environmentType: env.environmentType ?? '', executionProfile: env.executionProfile ?? '' })) }));

// Raw backend messages are English and unstable: map by HTTP status only.
function describeFailure(reason: unknown): string {
  if (!(reason instanceof ApiError)) return 'Chưa nhận được phản hồi từ máy chủ nên chưa rõ kết quả: mẫu có thể đã được đăng ký. Hãy tải lại danh sách trước khi thử lại.';
  switch (reason.status) {
    case 400: return 'Máy chủ từ chối mẫu này. Kiểm tra ID mẫu, ID bundle dựng ứng dụng (bundle có thể chưa được cài hoặc không khả dụng) và Điều kiện áp dụng rồi thử lại.';
    case 401: return 'Phiên đăng nhập đã hết hạn. Hãy đăng nhập lại.';
    case 403: return 'Bạn không có quyền đăng ký mẫu dựng ứng dụng. Chỉ Kỹ sư nền tảng hoặc Quản trị viên được đăng ký.';
    case 409: return 'ID mẫu đã tồn tại. Hãy đổi ID rồi thử lại.';
    case 413: return 'Nội dung mẫu quá lớn. Hãy rút gọn rồi thử lại.';
    default: return 'Máy chủ gặp lỗi khi đăng ký mẫu. Dữ liệu đã nhập được giữ lại; hãy thử lại sau.';
  }
}

const bundleOf = (definition: Definition) => {
  const bundle = definition.driverInputs?.values?.variables?.render_bundle;
  return typeof bundle === 'string' && bundle !== '' ? bundle : '';
};

export function RenderingTemplatesPage() {
  const definitionList = useCatalogList(loadDefinitions, listFailure);
  const applicationList = useCatalogList(loadApplications, listFailure);
  const templates = definitionList.items.filter((definition) => definition.resourceType === 'workload');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [idError, setIdError] = useState('');
  const [bundleError, setBundleError] = useState('');
  const [notice, setNotice] = useState('');
  const [key, setKey] = useState('');
  const [bundle, setBundle] = useState('');
  const [criteriaDraft, setCriteriaDraft] = useState<CriteriaDraft>(emptyDraft);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    setError(''); setIdError(''); setBundleError(''); setNotice('');
    // The backend owns the ID rule; the ID is checked, never changed.
    if (key === '') { setIdError('Nhập ID mẫu.'); return; }
    if (!idShape.test(key)) { setIdError('ID chỉ gồm chữ thường a-z, số 0-9 và dấu gạch ngang. ID không được tự sửa hộ.'); return; }
    if (bundle.trim() === '') { setBundleError('Nhập ID bundle dựng ứng dụng.'); return; }
    const { criteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) { setError(problem); return; }
    const submitted = key;
    setSaving(true);
    try {
      await api('/resource-definitions', { method: 'POST', body: JSON.stringify({ key: submitted, resourceType: 'workload', executionProfile: 'internal-k8s', driverType: 'score-k8s', driverInputs: { values: { variables: { render_bundle: bundle } } }, criteria }) });
    } catch (reason) { setError(describeFailure(reason)); setSaving(false); return; }
    // Committed: report it and reset the submitted form before reloading.
    setNotice(`Đã đăng ký mẫu dựng ứng dụng ${submitted}.`); setKey(''); setBundle(''); setCriteriaDraft(emptyDraft()); setSaving(false);
    await definitionList.reload();
  }

  const listError = definitionList.loadError;
  return <section className="page"><header className="page-header"><div><p className="eyebrow">Nền tảng</p><h1>Mẫu dựng ứng dụng</h1><p>Mẫu là tùy chọn và chỉ dùng cho cluster nội bộ. Hệ thống tự chọn mẫu theo Điều kiện áp dụng; không có mẫu khớp thì dùng renderer mặc định. Mẫu đã chọn mà dựng lỗi sẽ báo lỗi, không chuyển âm thầm sang renderer mặc định. Chỉ Kỹ sư nền tảng hoặc Quản trị viên được đăng ký.</p></div></header>
    <section className="content-panel" aria-label="Mẫu dựng ứng dụng đã đăng ký"><div className="section-header"><h2>Mẫu đã đăng ký</h2></div>{listError ? <div className="form-error" role="alert" aria-label="Lỗi tải danh sách">{listError} <Button type="button" onClick={() => { void definitionList.reload(); }}>Thử lại</Button></div> : definitionList.loading ? <p role="status">Đang tải danh sách mẫu dựng ứng dụng…</p> : templates.length === 0 ? <p>Chưa có mẫu dựng ứng dụng nào.</p> : <div className="catalog-list">{templates.map((template) => <div key={template.key} className="catalog-entry"><strong>{template.key}</strong><span>Bundle: {bundleOf(template) || 'không rõ'} · {template.criteria?.length ?? 0} điều kiện áp dụng</span></div>)}</div>}</section>
    <section className="content-panel"><div className="section-header"><div><h2>Đăng ký mẫu dựng ứng dụng</h2><p>Loại tài nguyên, driver và phạm vi triển khai được hệ thống đặt sẵn cho cluster nội bộ. Lưu mẫu không tự Xem trước hoặc Triển khai.</p></div></div>
      <form onSubmit={submit} noValidate aria-label="Đăng ký mẫu dựng ứng dụng"><fieldset className="editor-grid form-fieldset" disabled={saving} aria-busy={saving}>
        <div className="field-grid"><label>ID mẫu<input value={key} aria-required="true" aria-invalid={idError ? true : undefined} aria-describedby={idError ? 'template-id-hint template-id-error' : 'template-id-hint'} onChange={(event) => { setKey(event.target.value); setIdError(''); }} /></label>
          <label>ID bundle dựng ứng dụng<input value={bundle} aria-required="true" aria-invalid={bundleError ? true : undefined} aria-describedby={bundleError ? 'template-bundle-hint template-bundle-error' : 'template-bundle-hint'} onChange={(event) => { setBundle(event.target.value); setBundleError(''); }} /></label></div>
        <p id="template-id-hint" className="feature-note">ID mẫu dùng chữ thường, số và dấu gạch ngang; ID không được tự sửa hộ và là tên kỹ thuật của mẫu.</p>
        {idError ? <div id="template-id-error" className="form-error" role="alert">{idError}</div> : null}
        <p id="template-bundle-hint" className="feature-note">Nhập tường minh ID của bundle đã được cài. Đây chưa phải bộ chọn từ danh sách bundle đã cài; danh sách sẽ có ở bước sau. Máy chủ kiểm tra bundle khi đăng ký.</p>
        {bundleError ? <div id="template-bundle-error" className="form-error" role="alert">{bundleError}</div> : null}
        <CriteriaEditor draft={criteriaDraft} onChange={setCriteriaDraft} applications={{ items: applicationList.items, loading: applicationList.loading, error: applicationList.loadError, onRetry: () => { void applicationList.reload(); } }} sources={{ items: templates, loading: definitionList.loading, error: definitionList.loadError, onRetry: () => { void definitionList.reload(); } }} resourceType="workload" profile="internal-k8s" profileLabel="Cluster nội bộ" />
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saving}>{saving ? 'Đang đăng ký…' : 'Đăng ký mẫu dựng ứng dụng'}</Button></div>
      </fieldset></form>
    </section>
  </section>;
}
