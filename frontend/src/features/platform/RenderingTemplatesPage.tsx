import { useEffect, useState, type FormEvent } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { CriteriaEditor, draftToCriteria, emptyDraft, type ApplicationOption, type Criterion, type CriteriaDraft } from './CriteriaEditor';
import { useCatalogList } from './useCatalogList';

type APIApplication = { key: string; name?: string; environments?: { key: string; name?: string; environmentType?: string; executionProfile?: string }[] };
type Definition = { key: string; resourceType: string; executionProfile?: string; driverType: string; driverInputs?: { values?: { variables?: { render_bundle?: unknown } } }; criteria?: Partial<Criterion>[] };

type InstalledBundle = { id: string; version: string; status: string };
const idShape = /^[a-z0-9-]+$/;
const listFailure = () => 'Không tải được dữ liệu từ máy chủ.';
const loadDefinitions = async () => (await api<{ resourceDefinitions: Definition[] }>('/resource-definitions')).resourceDefinitions ?? [];
const loadBundles = async () => ((await api<{ renderBundles: InstalledBundle[] }>('/render-bundles')).renderBundles ?? []).filter((item) => item.status === 'AVAILABLE' && typeof item.id === 'string' && item.id !== '');
// Real application/environment IDs (keys) for the criteria editor; never derived from names.
const loadApplications = async (): Promise<ApplicationOption[]> => ((await api<{ applications: APIApplication[] }>('/applications')).applications ?? []).map((app) => ({ key: app.key, name: app.name ?? app.key, environments: (app.environments ?? []).map((env) => ({ key: env.key, name: env.name ?? env.key, environmentType: env.environmentType ?? '', executionProfile: env.executionProfile ?? '' })) }));

// Raw backend messages are English and unstable: map by HTTP status only.
function describeFailure(reason: unknown): string {
  if (!(reason instanceof ApiError)) return 'Chưa nhận được phản hồi từ máy chủ nên chưa rõ kết quả: mẫu có thể đã được đăng ký. Hãy tải lại danh sách trước khi thử lại.';
  switch (reason.status) {
    case 400: return 'Máy chủ từ chối mẫu này. Kiểm tra ID mẫu, bundle dựng ứng dụng đã chọn (bundle có thể không còn khả dụng; hãy làm mới danh sách bundle) và Điều kiện áp dụng rồi thử lại.';
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
  const bundleList = useCatalogList(loadBundles, () => 'Không tải được danh sách bundle đã cài.');
  const templates = definitionList.items.filter((definition) => definition.resourceType === 'workload');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [idError, setIdError] = useState('');
  const [bundleError, setBundleError] = useState('');
  const [notice, setNotice] = useState('');
  const [key, setKey] = useState('');
  const [bundle, setBundle] = useState('');
  const [criteriaDraft, setCriteriaDraft] = useState<CriteriaDraft>(emptyDraft);

  // One installed bundle is chosen for an unselected draft; several need an explicit choice.
  // The chosen ID is never replaced by a later or late list response.
  useEffect(() => {
    const [only] = bundleList.items;
    if (!bundleList.loading && !bundleList.loadError && bundle === '' && bundleList.items.length === 1 && only) setBundle(only.id);
  }, [bundleList.loading, bundleList.loadError, bundleList.items, bundle]);
  const bundleBlocked = bundleList.loading || bundleList.loadError !== '' || bundleList.items.length === 0;
  // The chosen ID stays visible even when the (stale, loading or failed) list does not hold it.
  const selectedMissing = bundle !== '' && !bundleList.items.some((item) => item.id === bundle);
  const bundleStale = selectedMissing && !bundleBlocked;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    setError(''); setIdError(''); setBundleError(''); setNotice('');
    // The backend owns the ID rule; the ID is checked, never changed.
    if (key === '') { setIdError('Nhập ID mẫu.'); return; }
    if (!idShape.test(key)) { setIdError('ID chỉ gồm chữ thường a-z, số 0-9 và dấu gạch ngang. ID không được tự sửa hộ.'); return; }
    if (bundleBlocked) { setBundleError('Chưa có danh sách bundle khả dụng nên chưa thể đăng ký. Xem hướng dẫn ở mục Bundle dựng ứng dụng.'); return; }
    if (bundle === '') { setBundleError('Chọn bundle dựng ứng dụng.'); return; }
    if (bundleStale) { setBundleError('Bundle đã chọn không còn khả dụng. Hãy chọn bundle khác trong danh sách.'); return; }
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
          <label>Bundle dựng ứng dụng<select value={bundle} disabled={bundleBlocked} aria-required="true" aria-invalid={bundleError || selectedMissing ? true : undefined} aria-describedby={bundleError ? 'template-bundle-hint template-bundle-error' : 'template-bundle-hint'} onChange={(event) => { setBundle(event.target.value); setBundleError(''); }}>
            {bundleList.items.length !== 1 ? <option value="">{bundleList.loading ? 'Đang tải bundle…' : 'Chọn bundle dựng ứng dụng'}</option> : null}
            {selectedMissing ? <option value={bundle}>{bundle} ({bundleBlocked ? 'chưa xác nhận khả dụng' : 'không còn khả dụng'})</option> : null}
            {bundleList.items.map((item) => <option key={item.id} value={item.id}>{item.id} (phiên bản {item.version})</option>)}
          </select></label></div>
        <p id="template-id-hint" className="feature-note">ID mẫu dùng chữ thường, số và dấu gạch ngang; ID không được tự sửa hộ và là tên kỹ thuật của mẫu.</p>
        {idError ? <div id="template-id-error" className="form-error" role="alert">{idError}</div> : null}
        <p id="template-bundle-hint" className="feature-note">Chọn một bundle đã được cài trên máy chủ. Một bundle duy nhất được chọn sẵn; nhiều bundle cần chọn tường minh. Máy chủ kiểm tra lại bundle khi đăng ký.</p>
        {bundleList.loadError ? <div className="form-error" role="alert" aria-label="Lỗi tải bundle">{bundleList.loadError} Dữ liệu đã nhập vẫn được giữ lại; chưa thể đăng ký cho đến khi tải được danh sách. <Button type="button" onClick={() => { void bundleList.reload(); }}>Thử lại tải bundle</Button></div> : bundleList.loading ? <p role="status">Đang tải danh sách bundle đã cài…</p> : bundleList.items.length === 0 ? <div className="form-error" role="alert" aria-label="Chưa có bundle">Chưa có bundle dựng ứng dụng nào được cài trên máy chủ nên chưa thể đăng ký mẫu. Hãy liên hệ Kỹ sư nền tảng để cài bundle, sau đó làm mới danh sách. <Button type="button" onClick={() => { void bundleList.reload(); }}>Làm mới danh sách bundle</Button></div> : <p><Button type="button" onClick={() => { void bundleList.reload(); }}>Làm mới danh sách bundle</Button></p>}
        {bundleStale ? <div className="form-error" role="alert" aria-label="Bundle không còn khả dụng">Bundle đã chọn ({bundle}) không còn khả dụng trong danh sách. Hãy chọn bundle khác; dữ liệu đã nhập được giữ lại.</div> : null}
        {bundleError ? <div id="template-bundle-error" className="form-error" role="alert">{bundleError}</div> : null}
        <CriteriaEditor draft={criteriaDraft} onChange={setCriteriaDraft} applications={{ items: applicationList.items, loading: applicationList.loading, error: applicationList.loadError, onRetry: () => { void applicationList.reload(); } }} sources={{ items: templates, loading: definitionList.loading, error: definitionList.loadError, onRetry: () => { void definitionList.reload(); } }} resourceType="workload" profile="internal-k8s" profileLabel="Cluster nội bộ" />
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saving || bundleBlocked || selectedMissing}>{saving ? 'Đang đăng ký…' : 'Đăng ký mẫu dựng ứng dụng'}</Button></div>
      </fieldset></form>
    </section>
  </section>;
}
