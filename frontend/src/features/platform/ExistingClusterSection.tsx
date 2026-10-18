import { useState, type FormEvent } from 'react';
import { api, ApiError, isStaleConflict } from '../../shared/api/client';
import { specificGuidance } from '../../shared/api/errors';
import { Button } from '../../shared/ui/Button';
import { snapshotKey, type ApiEnvironment } from './environmentSnapshot';
import { CriteriaEditor, draftFromCriteria, draftToCriteria, emptyDraft, type ApplicationOption, type Criterion, type CriteriaDraft } from './CriteriaEditor';
import type { MatchingCandidateInput } from './MatchingPreviewPanel';
import { useCatalogList } from './useCatalogList';
import { listAccounts, type Account } from './accounts/accountsApi';

// Existing-cluster Definition (ADR-016, UC-03 V03-01): a k8s-cluster resource backed by the Definition's OWN READY Internal Driver Account.
// The cluster identity and credentials come from that account; the form has no variables, kubeconfig or Environment connection.
type APIApplication = { key: string; name?: string; environments?: ApiEnvironment[] };
export type ClusterDefinition = { key: string; name?: string; version?: number; resourceType: string; driverType: string; driverAccount?: string; criteria?: Partial<Criterion>[] };
type Editing = { key: string; version: number };

const idShape = /^[a-z0-9-]+$/;
const maxNameLength = 253;
const clusterType = 'k8s-cluster';
const loadApplications = async (): Promise<ApplicationOption[]> => ((await api<{ applications: APIApplication[] }>('/applications')).applications ?? []).map((app) => ({ key: app.key, name: app.name ?? app.key, environments: (app.environments ?? []).map((env) => ({ key: env.key, name: env.name ?? env.key, environmentType: env.environmentType ?? '', snapshotKey: snapshotKey(env) })) }));
const accountLabel = (account: Account) => (account.name && account.name !== account.key ? `${account.name} (${account.key})` : account.key);

function describeFailure(reason: unknown): string {
  if (!(reason instanceof ApiError)) return 'Chưa nhận được phản hồi từ máy chủ nên chưa rõ cấu hình cluster đã được lưu hay chưa. Console không tự gửi lại; hãy tải lại danh sách.';
  const specific = specificGuidance(reason.code, reason.message, reason.field);
  if (specific) return `Máy chủ từ chối cấu hình cluster. ${specific}`;
  if (reason.status === 403) return 'Chỉ Kỹ sư nền tảng hoặc Quản trị viên được đăng ký và sửa cấu hình cluster.';
  if (reason.status === 409) return 'ID cấu hình đã tồn tại hoặc phiên bản đã đổi. Hãy tải lại rồi thử lại.';
  return 'Máy chủ từ chối cấu hình cluster này. Kiểm tra ID, Tên, tài khoản Internal READY và Điều kiện áp dụng; dữ liệu đã nhập được giữ lại.';
}

// Lists existing-cluster Definitions; the registration form (and its account/application reads) mounts only when opened.
export function ExistingClusterSection({ definitions: all, reload }: { definitions: ClusterDefinition[]; reload(): Promise<boolean> }) {
  const definitionList = { items: all.filter((definition) => definition.driverType === 'existing-cluster' && definition.key !== 'builtin-existing-cluster'), loadError: '', reload };
  const [form, setForm] = useState<{ edit?: ClusterDefinition } | null>(null);
  return <section className="content-panel" aria-label="Cluster có sẵn (existing-cluster)">
    <div className="section-header"><div><h2>Cluster có sẵn (existing-cluster)</h2><p className="feature-note">Một cấu hình cluster khớp theo Điều kiện áp dụng như mọi cấu hình khác và dùng tài khoản Internal READY của chính nó. Namespace và workload tham chiếu cluster này nên kế thừa tài khoản; Môi trường không chọn tài khoản. Cluster có sẵn không bao giờ bị tạo hay xóa.</p></div>{form ? null : <Button type="button" onClick={() => setForm({})}>Đăng ký cấu hình cluster có sẵn</Button>}</div>
    {definitionList.loadError ? <div className="form-error" role="alert">{definitionList.loadError} <Button type="button" onClick={() => { void definitionList.reload(); }}>Thử lại</Button></div> : definitionList.items.length === 0 ? <p>Chưa có cấu hình cluster nào.</p> : <div className="catalog-list">{definitionList.items.map((definition) => <div key={definition.key} className="catalog-entry" data-definition={definition.key}><strong>{definition.name || definition.key}</strong><span>ID <code>{definition.key}</code> · {definition.resourceType} · tài khoản {definition.driverAccount ?? '?'} · phiên bản {definition.version ?? '?'}</span><Button type="button" aria-label={`Sửa cluster ${definition.key}`} onClick={() => setForm({ edit: definition })}>Sửa</Button></div>)}</div>}
    {form ? <ExistingClusterForm key={form.edit?.key ?? 'new'} initial={form.edit} definitions={definitionList.items} reload={definitionList.reload} onClose={() => setForm(null)} /> : null}
  </section>;
}

function ExistingClusterForm({ initial, definitions, reload, onClose }: { initial?: ClusterDefinition; definitions: ClusterDefinition[]; reload(): Promise<boolean>; onClose(): void }) {
  const applicationList = useCatalogList(loadApplications, () => 'Không tải được danh sách ứng dụng.');
  const accountList = useCatalogList(listAccounts, () => 'Không tải được danh sách tài khoản hạ tầng.');
  const accounts = accountList.items.filter((account) => account.type === 'INTERNAL' && account.status === 'READY');
  const [key, setKey] = useState(initial?.key ?? '');
  const [name, setName] = useState(initial?.name ?? '');
  const [account, setAccount] = useState(initial?.driverAccount ?? '');
  const [criteriaDraft, setCriteriaDraft] = useState<CriteriaDraft>(() => (initial ? draftFromCriteria(initial.criteria) : emptyDraft()));
  const [editing, setEditing] = useState<Editing | null>(initial ? { key: initial.key, version: initial.version ?? 0 } : null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [stale, setStale] = useState(false);

  function reset() { setKey(''); setName(''); setAccount(''); setCriteriaDraft(emptyDraft()); setEditing(null); setStale(false); setError(''); }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving || stale) return;
    setError(''); setNotice('');
    if (!idShape.test(key)) { setError('ID cấu hình chỉ gồm chữ thường a-z, số 0-9 và dấu gạch ngang.'); return; }
    if (name.trim() === '' || [...name].length > maxNameLength) { setError(`Nhập Tên hiển thị (tối đa ${maxNameLength} ký tự).`); return; }
    if (account === '') { setError('Chọn tài khoản Internal READY cho cluster này.'); return; }
    const { criteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) { setError(problem); return; }
    const body = { key, name, resourceType: clusterType, driverType: 'existing-cluster', driverAccount: account, driverInputs: { values: { variables: {} } }, criteria };
    setSaving(true);
    try {
      if (editing) {
        const saved = await api<ClusterDefinition>(`/resource-definitions/${encodeURIComponent(editing.key)}`, { method: 'PUT', body: JSON.stringify({ ...body, version: editing.version }) });
        setEditing({ key: saved.key, version: saved.version ?? editing.version + 1 });
        setNotice(`Đã lưu cấu hình cluster ${saved.key} (phiên bản ${saved.version ?? editing.version + 1}). Chỉ Xem trước và Triển khai mới dùng cấu hình mới; workload đã triển khai giữ đích đã ghim.`);
      } else {
        await api('/resource-definitions', { method: 'POST', body: JSON.stringify(body) });
        setNotice(`Đã đăng ký cấu hình cluster ${key}.`); reset();
      }
    } catch (reason) {
      if (reason instanceof ApiError && isStaleConflict(reason)) setStale(true);
      setError(describeFailure(reason));
    } finally {
      setSaving(false);
    }
    await reload();
  }
  const matching: MatchingCandidateInput = (() => {
    if (!idShape.test(key)) return { definition: null, problem: 'Nhập ID cấu hình hợp lệ để kiểm tra matching chính xác.' };
    if (account === '') return { definition: null, problem: 'Chọn tài khoản Internal để kiểm tra matching chính xác.' };
    const { criteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) return { definition: null, problem };
    return { definition: { key, name, resourceType: clusterType, driverType: 'existing-cluster', driverAccount: account, driverInputs: { values: { variables: {} } }, criteria }, problem: '' };
  })();

  return <>
    <form onSubmit={submit} noValidate aria-label={editing ? 'Sửa cấu hình cluster' : 'Đăng ký cấu hình cluster'}><fieldset className="editor-grid form-fieldset" disabled={saving}>
      <h3>{editing ? `Sửa cấu hình cluster ${editing.key}` : 'Đăng ký cấu hình cluster'}</h3>
      <div className="field-grid"><label>ID cấu hình cluster<input value={key} readOnly={editing !== null} aria-readonly={editing !== null ? true : undefined} onChange={(event) => setKey(event.target.value)} /></label>
        <label>Tên hiển thị cluster<input value={name} maxLength={maxNameLength} onChange={(event) => setName(event.target.value)} /></label>
        <label>Tài khoản Internal của cluster<select aria-label="Tài khoản Internal của cluster" value={account} disabled={accountList.loading} onChange={(event) => setAccount(event.target.value)}><option value="">{accountList.loading ? 'Đang tải tài khoản…' : 'Chọn tài khoản Internal READY'}</option>{account !== '' && !accounts.some((item) => item.key === account) ? <option value={account}>{account} (không còn READY)</option> : null}{accounts.map((item) => <option key={item.key} value={item.key}>{accountLabel(item)}</option>)}</select></label></div>
      {accountList.loadError ? <div className="form-error" role="alert">{accountList.loadError} <Button type="button" onClick={() => { void accountList.reload(); }}>Thử lại</Button></div> : accounts.length === 0 && !accountList.loading ? <p className="feature-note">Chưa có tài khoản Internal READY. Hãy đăng ký và xác minh tài khoản trước.</p> : null}
      <CriteriaEditor draft={criteriaDraft} onChange={setCriteriaDraft} applications={{ items: applicationList.items, loading: applicationList.loading, error: applicationList.loadError, onRetry: () => { void applicationList.reload(); } }} sources={{ items: definitions, loading: false, error: '', onRetry: () => { void reload(); } }} resourceType={clusterType} matching={matching} scopeKey={`cluster|${editing ? `${editing.key}@${editing.version}` : ''}|${account}`} excludeKey={editing?.key ?? ''} />
      {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}
      <div className="form-actions"><Button type="button" onClick={onClose}>Đóng biểu mẫu</Button><Button type="submit" tone="primary" disabled={saving || stale}>{saving ? 'Đang lưu…' : editing ? 'Lưu thay đổi cluster' : 'Đăng ký cấu hình cluster'}</Button></div>
    </fieldset></form>
  </>;
}
