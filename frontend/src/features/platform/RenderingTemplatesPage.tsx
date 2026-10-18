import { useEffect, useState, type FormEvent } from 'react';
import { specificGuidance } from '../../shared/api/errors';
import { parseRenderBundles } from './metadata';
import { api, ApiError, isStaleConflict } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { snapshotKey, type ApiEnvironment } from './environmentSnapshot';
import { CriteriaEditor, draftFromCriteria, draftToCriteria, emptyDraft, type ApplicationOption, type Criterion, type CriteriaDraft } from './CriteriaEditor';
import type { MatchingCandidateInput } from './MatchingPreviewPanel';
import { useCatalogList } from './useCatalogList';
import { loadCapabilities } from './capabilities';
import { ProvisionEditor } from './ProvisionEditor';
import type { ProvisionRow } from './typedProvision';
import { bodyToTemplate, emptyTemplateForm, jsonToBody, templateToBody, type TemplateForm } from './workloadTemplate';

type APIApplication = { key: string; name?: string; environments?: ApiEnvironment[] };
// The same profile-free Definition contract as the Resource Definition page (UC-03 V02-01): Name, version, five-field criteria.
type Definition = { key: string; name?: string; version?: number; resourceType: string; driverType: string; driverInputs?: { values?: { variables?: Record<string, unknown> } }; provision?: Record<string, unknown>; criteria?: Partial<Criterion>[] };
type Editing = { key: string; version: number };

type InstalledBundle = { id: string; version: string; status: string };
const idShape = /^[a-z0-9-]+$/;
const maxNameLength = 253;
const listFailure = () => 'Không tải được dữ liệu từ máy chủ.';
const loadDefinitions = async () => (await api<{ resourceDefinitions: Definition[] }>('/resource-definitions')).resourceDefinitions ?? [];
const loadBundles = async (): Promise<InstalledBundle[]> => parseRenderBundles(await api<unknown>('/render-bundles')).filter((item) => item.status === 'AVAILABLE');
// Real application/environment IDs (keys) for the criteria editor; never derived from names.
const loadApplications = async (): Promise<ApplicationOption[]> => ((await api<{ applications: APIApplication[] }>('/applications')).applications ?? []).map((app) => ({ key: app.key, name: app.name ?? app.key, environments: (app.environments ?? []).map((env) => ({ key: env.key, name: env.name ?? env.key, environmentType: env.environmentType ?? '', snapshotKey: snapshotKey(env) })) }));

// Specific guidance comes from the stable error code; raw backend text is never shown.
function describeFailure(reason: unknown, editing: boolean): string {
  if (!(reason instanceof ApiError)) return editing ? 'Chưa nhận được phản hồi từ máy chủ nên chưa rõ thay đổi đã được lưu hay chưa. Console không tự gửi lại; hãy tải lại danh sách để xem kết quả thực tế.' : 'Chưa nhận được phản hồi từ máy chủ nên chưa rõ kết quả: mẫu có thể đã được đăng ký. Hãy tải lại danh sách trước khi thử lại.';
  const specific = specificGuidance(reason.code, reason.message, reason.field);
  if (specific) return `Máy chủ từ chối mẫu. ${specific}`;
  switch (reason.status) {
    case 400: return 'Máy chủ từ chối mẫu này. Kiểm tra ID mẫu, Tên, bundle dựng ứng dụng đã chọn (bundle có thể không còn khả dụng; hãy làm mới danh sách bundle) và Điều kiện áp dụng rồi thử lại.';
    case 401: return 'Phiên đăng nhập đã hết hạn. Hãy đăng nhập lại.';
    case 403: return 'Bạn không có quyền đăng ký hoặc sửa mẫu dựng ứng dụng. Chỉ Kỹ sư nền tảng hoặc Quản trị viên được phép.';
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
  const capabilityList = useCatalogList(loadCapabilities, () => 'Không tải được metadata Cấu hình workload.');
  const presets = capabilityList.items.find((capability) => capability.driverType === 'score-k8s')?.variables ?? [];
  const presetOf = (name: string) => presets.find((variable) => variable.name === name)?.preset ?? '';
  const templates = definitionList.items.filter((definition) => definition.resourceType === 'workload');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [idError, setIdError] = useState('');
  const [nameError, setNameError] = useState('');
  const [bundleError, setBundleError] = useState('');
  const [notice, setNotice] = useState('');
  const [key, setKey] = useState('');
  const [name, setName] = useState('');
  const [bundle, setBundle] = useState('');
  const [criteriaDraft, setCriteriaDraft] = useState<CriteriaDraft>(emptyDraft);
  // Typed placement and companions (ADR-016). Form and advanced JSON serialize to the same two objects.
  const [namespaceRef, setNamespaceRef] = useState('');
  const [clusterRef, setClusterRef] = useState('');
  const [rows, setRows] = useState<ProvisionRow[]>([]);
  const [mode, setMode] = useState<'form' | 'json'>('form');
  const [jsonText, setJsonText] = useState('{}');
  const [modeNote, setModeNote] = useState('');
  const [editing, setEditing] = useState<Editing | null>(null);
  // A stale (409) or unknown-outcome save never resubmits by itself: the draft stays and Save is off until an explicit reload.
  // A create with an unknown outcome is held the same way (`created` is the submitted ID): no automatic POST replay before the stored key is read back.
  const [review, setReview] = useState<{ kind: 'stale' | 'unknown'; created?: string } | null>(null);

  // One installed bundle is chosen for an unselected draft; several need an explicit choice.
  // The chosen ID is never replaced by a later or late list response.
  useEffect(() => {
    const [only] = bundleList.items;
    if (!bundleList.loading && !bundleList.loadError && bundle === '' && bundleList.items.length === 1 && only && editing === null) setBundle(only.id);
  }, [bundleList.loading, bundleList.loadError, bundleList.items, bundle, editing]);
  const bundleBlocked = bundleList.loading || bundleList.loadError !== '' || bundleList.items.length === 0;
  // The chosen ID stays visible even when the (stale, loading or failed) list does not hold it.
  const selectedMissing = bundle !== '' && !bundleList.items.some((item) => item.id === bundle);
  const bundleStale = selectedMissing && !bundleBlocked;

  const formState = (): TemplateForm => ({ ...emptyTemplateForm(), bundle, namespaceRef, clusterRef, rows });
  function applyForm(form: TemplateForm) { setBundle(form.bundle); setNamespaceRef(form.namespaceRef); setClusterRef(form.clusterRef); setRows(form.rows); }
  function jsonFromForm(form: TemplateForm): string {
    const body = templateToBody(form);
    // An incomplete form still round-trips: raw values are shown exactly as typed.
    const variables = body.variables ?? { render_bundle: form.bundle, namespace: form.namespaceRef, cluster: form.clusterRef };
    return JSON.stringify({ variables, ...(body.provision ? { provision: body.provision } : {}) }, null, 2);
  }
  function switchMode(next: 'form' | 'json') {
    setModeNote('');
    if (next === 'json') { setJsonText(jsonFromForm(formState())); setMode('json'); return; }
    const parsed = jsonToBody(jsonText);
    if (parsed.error) { setModeNote(parsed.error); return; }
    const converted = bodyToTemplate(parsed.variables, parsed.provision);
    if (!converted.form) { setModeNote(converted.reason ?? 'JSON nâng cao không biểu diễn được bằng biểu mẫu.'); return; }
    applyForm(converted.form); setMode('form');
  }
  // The body both modes send; a problem blocks Save without changing the draft.
  const currentBody = (): { variables?: unknown; provision?: unknown; problem?: string } => {
    if (mode === 'form') return templateToBody(formState());
    const parsed = jsonToBody(jsonText);
    if (parsed.error) return { problem: parsed.error };
    if (parsed.variables === null || typeof parsed.variables !== 'object' || Array.isArray(parsed.variables)) return { problem: 'variables phải là JSON object.' };
    return { variables: parsed.variables, ...(parsed.provision !== undefined ? { provision: parsed.provision } : {}) };
  };

  function resetForm() {
    applyForm(emptyTemplateForm()); setMode('form'); setModeNote(''); setJsonText('{}');
    setKey(''); setName(''); setBundle(''); setCriteriaDraft(emptyDraft()); setEditing(null); setReview(null);
    setError(''); setIdError(''); setNameError(''); setBundleError('');
  }
  function startEdit(template: Definition) {
    setNotice(''); setError(''); setIdError(''); setNameError(''); setBundleError(''); setReview(null);
    setKey(template.key); setName(template.name ?? ''); setBundle(bundleOf(template)); setCriteriaDraft(draftFromCriteria(template.criteria));
    const converted = bodyToTemplate(template.driverInputs?.values?.variables ?? {}, template.provision);
    if (converted.form) { applyForm(converted.form); setMode('form'); setModeNote(''); }
    else {
      applyForm({ ...emptyTemplateForm(), bundle: bundleOf(template) });
      setJsonText(JSON.stringify({ variables: template.driverInputs?.values?.variables ?? {}, ...(template.provision ? { provision: template.provision } : {}) }, null, 2));
      setMode('json'); setModeNote(`${converted.reason ?? ''} Mẫu được mở ở JSON nâng cao để không mất cấu hình.`);
    }
    setEditing({ key: template.key, version: template.version ?? 0 });
  }
  // Explicit reload after a stale or unknown-outcome save: one GET of the stored template replaces the draft.
  // After an unknown CREATE the GET decides: found means the earlier POST committed (the stored version opens, nothing is re-sent);
  // not found means it rolled back (the draft stays and the hold is released); a failed GET keeps both draft and hold.
  async function reloadLatest() {
    const target = review?.created ?? editing?.key;
    if (!target) return;
    let latest: Definition | undefined;
    try { latest = (await loadDefinitions()).find((definition) => definition.key === target); } catch { setError('Chưa tải lại được mẫu từ máy chủ. Bản nháp của bạn vẫn được giữ; hãy thử lại.'); return; }
    void definitionList.reload();
    if (review?.created) {
      if (!latest) { setReview(null); setError(''); setNotice(`Máy chủ không có mẫu ${target}: lần đăng ký trước không được lưu. Bản nháp được giữ; bạn có thể đăng ký lại.`); return; }
      startEdit(latest);
      setNotice(`Mẫu ${target} đã có trên máy chủ (lần đăng ký trước đã được lưu). Đang mở bản đã lưu để xem lại; Console không gửi lại.`);
      return;
    }
    if (!latest) { setError('Mẫu này không còn trong danh sách của máy chủ. Bản nháp của bạn vẫn được giữ.'); return; }
    startEdit(latest);
    setNotice('Đã tải lại bản mới nhất từ máy chủ; bản nháp trước đó đã được thay thế.');
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving || review) return;
    setError(''); setIdError(''); setNameError(''); setBundleError(''); setNotice('');
    // The backend owns the ID rule; the ID is checked, never changed.
    if (key === '') { setIdError('Nhập ID mẫu.'); return; }
    if (!idShape.test(key)) { setIdError('ID chỉ gồm chữ thường a-z, số 0-9 và dấu gạch ngang. ID không được tự sửa hộ.'); return; }
    if (name.trim() === '') { setNameError('Nhập Tên hiển thị của mẫu.'); return; }
    if ([...name].length > maxNameLength) { setNameError(`Tên tối đa ${maxNameLength} ký tự.`); return; }
    if (bundleBlocked) { setBundleError('Chưa có danh sách bundle khả dụng nên chưa thể lưu. Xem hướng dẫn ở mục Bundle dựng ứng dụng.'); return; }
    if (bundle === '') { setBundleError('Chọn bundle dựng ứng dụng.'); return; }
    if (bundleStale) { setBundleError('Bundle đã chọn không còn khả dụng. Hãy chọn bundle khác trong danh sách.'); return; }
    const { criteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) { setError(problem); return; }
    const submitted = key;
    const parts = currentBody();
    if (parts.problem) { setError(parts.problem); return; }
    const body = { key: submitted, name, resourceType: 'workload', driverType: 'score-k8s', driverInputs: { values: { variables: parts.variables } }, ...(parts.provision !== undefined ? { provision: parts.provision } : {}), criteria };
    setSaving(true);
    if (editing) {
      let saved: Definition;
      try {
        saved = await api<Definition>(`/resource-definitions/${encodeURIComponent(editing.key)}`, { method: 'PUT', body: JSON.stringify({ ...body, version: editing.version }) });
      } catch (reason) {
        setSaving(false);
        if (reason instanceof ApiError && isStaleConflict(reason)) { setReview({ kind: 'stale' }); return; }
        if (!(reason instanceof ApiError) || reason.code === 'COMMIT_UNCERTAIN') { setReview({ kind: 'unknown' }); void definitionList.reload(); }
        setError(describeFailure(reason, true));
        return;
      }
      setEditing({ key: saved.key, version: saved.version ?? editing.version + 1 });
      setNotice(`Đã lưu mẫu dựng ứng dụng ${saved.key} (phiên bản ${saved.version ?? editing.version + 1}). Chỉ Xem trước và Triển khai mới dùng mẫu mới; snapshot và tài nguyên đã triển khai không đổi.`);
      setSaving(false);
      await definitionList.reload();
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
    // Committed: report it and reset the submitted form before reloading.
    setNotice(`Đã đăng ký mẫu dựng ứng dụng ${submitted}.`); setKey(''); setName(''); applyForm(emptyTemplateForm()); setMode('form'); setJsonText('{}'); setCriteriaDraft(emptyDraft()); setSaving(false);
    await definitionList.reload();
  }

  const matching = ((): MatchingCandidateInput => {
    if (key === '' || !idShape.test(key)) return { definition: null, problem: 'Nhập ID mẫu hợp lệ để kiểm tra matching chính xác.' };
    if (bundle === '') return { definition: null, problem: 'Chọn bundle dựng ứng dụng để kiểm tra matching chính xác.' };
    const { criteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) return { definition: null, problem };
    return { definition: { key, name, resourceType: 'workload', driverType: 'score-k8s', driverInputs: { values: { variables: { render_bundle: bundle } } }, criteria }, problem: '' };
  })();

  const listError = definitionList.loadError;
  return <section className="page"><header className="page-header"><div><p className="eyebrow">Nền tảng</p><h1>Mẫu dựng ứng dụng</h1><p>Mọi Workload cần đúng một mẫu dựng ứng dụng (score-k8s) khớp Điều kiện áp dụng của Môi trường, theo cùng năm field và cùng cách xếp hạng với mọi cấu hình tài nguyên; không có bộ lọc profile. Không có mẫu, có nhiều mẫu đồng hạng hoặc bundle chưa cài thì Xem trước và Triển khai bị chặn trước khi tạo tài nguyên; hệ thống không dùng renderer mặc định hay renderer khác. Chỉ Kỹ sư nền tảng hoặc Quản trị viên được đăng ký và sửa.</p></div></header>
    <section className="content-panel" aria-label="Mẫu dựng ứng dụng đã đăng ký"><div className="section-header"><h2>Mẫu đã đăng ký</h2></div>{listError ? <div className="form-error" role="alert" aria-label="Lỗi tải danh sách">{listError} <Button type="button" onClick={() => { void definitionList.reload(); }}>Thử lại</Button></div> : definitionList.loading && definitionList.items.length === 0 ? <p role="status">Đang tải danh sách mẫu dựng ứng dụng…</p> : templates.length === 0 ? <p>Chưa có mẫu dựng ứng dụng nào.</p> : <div className="catalog-list">{templates.map((template) => <div key={template.key} className="catalog-entry" data-definition={template.key}><strong>{template.name || template.key}</strong><span>ID <code>{template.key}</code> · Bundle: {bundleOf(template) || 'không rõ'} · phiên bản {template.version ?? '?'} · {template.criteria?.length ?? 0} điều kiện áp dụng</span><Button type="button" aria-label={`Sửa mẫu ${template.key}`} disabled={saving} onClick={() => startEdit(template)}>Sửa</Button></div>)}</div>}</section>
    <section className="content-panel"><div className="section-header"><div><h2>{editing ? `Sửa mẫu dựng ứng dụng ${editing.key}` : 'Đăng ký mẫu dựng ứng dụng'}</h2><p>Loại tài nguyên và driver được hệ thống đặt sẵn; bạn chọn bundle đã cài và Điều kiện áp dụng. Lưu mẫu không tự Xem trước hoặc Triển khai; ID không đổi sau khi tạo.</p></div>{editing ? <Button type="button" onClick={resetForm} disabled={saving}>Hủy sửa</Button> : null}</div>
      {review ? <div className="form-warning" role="alert" aria-label="Cần tải lại mẫu"><p>{review.kind === 'stale' ? 'Mẫu đã được người khác thay đổi kể từ lần bạn tải (phiên bản không còn khớp). Chưa có gì bị ghi đè; bản nháp của bạn vẫn được giữ.' : review.created ? 'Chưa biết lần đăng ký đã được lưu hay chưa. Console không tự gửi lại.' : 'Chưa biết thay đổi đã được lưu hay chưa. Console không tự gửi lại.'} {review.created ? 'Hãy kiểm tra trên máy chủ: nếu mẫu đã có thì mở bản đã lưu, nếu chưa có thì bản nháp được giữ để bạn đăng ký lại.' : 'Hãy tải lại bản mới nhất, xem lại rồi lưu lại tường minh.'}</p>
        <div className="form-actions"><Button type="button" onClick={() => { void definitionList.reload(); }}>Tải lại danh sách</Button><Button type="button" tone="primary" onClick={() => { void reloadLatest(); }}>{review.created ? 'Kiểm tra trên máy chủ' : 'Tải lại bản mới nhất'}</Button></div></div> : null}
      <form onSubmit={submit} noValidate aria-label={editing ? 'Sửa mẫu dựng ứng dụng' : 'Đăng ký mẫu dựng ứng dụng'}><fieldset className="editor-grid form-fieldset" disabled={saving} aria-busy={saving}>
        <div className="field-grid"><label>ID mẫu<input value={key} readOnly={editing !== null} aria-readonly={editing !== null ? true : undefined} aria-required="true" aria-invalid={idError ? true : undefined} aria-describedby={idError ? 'template-id-hint template-id-error' : 'template-id-hint'} onChange={(event) => { setKey(event.target.value); setIdError(''); }} /></label>
          <label>Tên hiển thị<input value={name} maxLength={maxNameLength} aria-required="true" aria-invalid={nameError ? true : undefined} onChange={(event) => { setName(event.target.value); setNameError(''); }} /></label>
          <label>Bundle dựng ứng dụng<select value={bundle} disabled={bundleBlocked} aria-required="true" aria-invalid={bundleError || selectedMissing ? true : undefined} aria-describedby={bundleError ? 'template-bundle-hint template-bundle-error' : 'template-bundle-hint'} onChange={(event) => { setBundle(event.target.value); setBundleError(''); }}>
            {bundleList.items.length !== 1 ? <option value="">{bundleList.loading ? 'Đang tải bundle…' : 'Chọn bundle dựng ứng dụng'}</option> : null}
            {selectedMissing ? <option value={bundle}>{bundle} ({bundleBlocked ? 'chưa xác nhận khả dụng' : 'không còn khả dụng'})</option> : null}
            {bundleList.items.map((item) => <option key={item.id} value={item.id}>{item.id} (phiên bản {item.version})</option>)}
          </select></label></div>
        <p id="template-id-hint" className="feature-note">ID mẫu dùng chữ thường, số và dấu gạch ngang; ID không được tự sửa hộ và là tên kỹ thuật của mẫu (khác ID tài nguyên <code>res_id</code>). Tên hiển thị sửa được.</p>
        {idError ? <div id="template-id-error" className="form-error" role="alert">{idError}</div> : null}
        {nameError ? <div className="form-error" role="alert">{nameError}</div> : null}
        <p id="template-bundle-hint" className="feature-note">Chọn một bundle đã được cài trên máy chủ. Một bundle duy nhất được chọn sẵn khi tạo mới; nhiều bundle cần chọn tường minh. Máy chủ kiểm tra lại bundle khi lưu.</p>
        {bundleList.loadError ? <div className="form-error" role="alert" aria-label="Lỗi tải bundle">{bundleList.loadError} Dữ liệu đã nhập vẫn được giữ lại; chưa thể lưu cho đến khi tải được danh sách. <Button type="button" onClick={() => { void bundleList.reload(); }}>Thử lại tải bundle</Button></div> : bundleList.loading ? <p role="status">Đang tải danh sách bundle đã cài…</p> : bundleList.items.length === 0 ? <div className="form-error" role="alert" aria-label="Chưa có bundle">Chưa có bundle dựng ứng dụng nào được cài trên máy chủ nên chưa thể lưu mẫu. Hãy liên hệ Kỹ sư nền tảng để cài bundle, sau đó làm mới danh sách. <Button type="button" onClick={() => { void bundleList.reload(); }}>Làm mới danh sách bundle</Button></div> : <p><Button type="button" onClick={() => { void bundleList.reload(); }}>Làm mới danh sách bundle</Button></p>}
        {bundleStale ? <div className="form-error" role="alert" aria-label="Bundle không còn khả dụng">Bundle đã chọn ({bundle}) không còn khả dụng trong danh sách. Hãy chọn bundle khác; dữ liệu đã nhập được giữ lại.</div> : null}
        {bundleError ? <div id="template-bundle-error" className="form-error" role="alert">{bundleError}</div> : null}
        <section className="catalog-fields" aria-label="Đích triển khai và cấu hình workload">
          <div className="section-header"><div><h4>Đích triển khai và đầu vào</h4><p className="feature-note">Cấu hình quyết định nơi workload chạy: tham chiếu đầy đủ tới output <code>name</code> của namespace và output <code>cluster</code> của cluster. Cluster dùng tài khoản Internal của chính cấu hình cluster; Môi trường không chọn tài khoản. Biểu mẫu và JSON nâng cao là cùng một cấu hình.</p></div>
            <Button type="button" onClick={() => switchMode(mode === 'form' ? 'json' : 'form')}>{mode === 'form' ? 'JSON nâng cao' : 'Biểu mẫu'}</Button></div>
          {modeNote ? <div className="form-warning" role="status">{modeNote}</div> : null}
          {mode === 'json' ? <label>JSON nâng cao (variables, provision)<textarea aria-label="JSON nâng cao của mẫu dựng ứng dụng" rows={14} spellCheck={false} value={jsonText} onChange={(event) => setJsonText(event.target.value)} /></label> : <>
            <label>Tham chiếu namespace<input aria-label="Tham chiếu namespace" value={namespaceRef} spellCheck={false} placeholder="${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}" onChange={(event) => setNamespaceRef(event.target.value)} /></label>
            {presetOf('namespace') ? <Button type="button" tone="quiet" onClick={() => setNamespaceRef(presetOf('namespace'))}>Dùng gợi ý namespace</Button> : null}
            <label>Tham chiếu cluster<input aria-label="Tham chiếu cluster" value={clusterRef} spellCheck={false} placeholder="${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}" onChange={(event) => setClusterRef(event.target.value)} /></label>
            {presetOf('cluster') ? <Button type="button" tone="quiet" onClick={() => setClusterRef(presetOf('cluster'))}>Dùng gợi ý cluster</Button> : null}
            <ProvisionEditor rows={rows} onChange={setRows} disabled={saving} />
          </>}
        </section>
        <CriteriaEditor draft={criteriaDraft} onChange={setCriteriaDraft} applications={{ items: applicationList.items, loading: applicationList.loading, error: applicationList.loadError, onRetry: () => { void applicationList.reload(); } }} sources={{ items: templates, loading: definitionList.loading, error: definitionList.loadError, onRetry: () => { void definitionList.reload(); } }} resourceType="workload" matching={matching} scopeKey={`bundles:${bundleList.loading ? 'loading' : bundleList.loadError ? 'error' : JSON.stringify(bundleList.items.map((item) => [item.id, item.version, item.status]))}|editing:${editing ? `${editing.key}@${editing.version}` : ''}`} excludeKey={editing?.key ?? ''} />
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saving || bundleBlocked || selectedMissing || review !== null}>{saving ? (editing ? 'Đang lưu…' : 'Đang đăng ký…') : editing ? 'Lưu thay đổi' : 'Đăng ký mẫu dựng ứng dụng'}</Button></div>
      </fieldset></form>
    </section>
  </section>;
}
