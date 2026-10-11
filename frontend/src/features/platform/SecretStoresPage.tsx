import { useCallback, useEffect, useRef, useState } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';

type Store = { key: string; name: string; provider: string; backendAddress: string; workloadAddress: string; mount: string; authMount: string; tlsCustomCa: boolean; status: string; legacy?: boolean; verification?: Record<string, unknown> | null };
type Form = { name: string; backendAddress: string; workloadAddress: string; sameAddress: boolean; mount: string; authMount: string; tlsCaPem: string; token: string };
type Field = 'name' | 'backendAddress' | 'workloadAddress' | 'mount' | 'authMount' | 'tlsCaPem' | 'token' | 'form';
type Problems = Partial<Record<Field, string>>;

// Defaults the backend applies to an empty mount (secretstore.DefaultKVMount / DefaultAuthMount).
export const DEFAULT_MOUNT = 'kv';
export const DEFAULT_AUTH_MOUNT = 'kubernetes';
// Limits mirror the backend registration guard (secretstores/service.go, stores.go).
export const MAX_NAME_CHARS = 100;
export const MAX_TOKEN_BYTES = 4096;
export const MAX_CA_BYTES = 64 << 10;
export const MAX_ENVELOPE_BYTES = 256 << 10;

const empty: Form = { name: '', backendAddress: '', workloadAddress: '', sameAddress: false, mount: '', authMount: '', tlsCaPem: '', token: '' };
const mountPattern = /^[A-Za-z0-9_-]{1,64}$/;
// eslint-disable-next-line no-control-regex -- control characters are exactly what is rejected.
const controlPattern = /[\u0000-\u001f\u007f-\u009f]/;
const base64Pattern = /^[A-Za-z0-9+/]+={0,2}$/;
const bytes = (text: string) => new TextEncoder().encode(text).length;

function addressProblem(raw: string, label: string): string {
  const text = raw.trim();
  if (!text) return `${label} là bắt buộc.`;
  const fix = `${label} phải là URL http(s):// có host, không chứa thông tin đăng nhập, query, fragment, khoảng trắng, dấu gạch chéo ngược hoặc ký tự điều khiển.`;
  // Reject what WHATWG URL would silently normalize (backslashes, control characters) but Go url.Parse refuses.
  if (/\s/.test(text) || /[?#\\]/.test(text) || controlPattern.test(text)) return fix;
  const authority = /^https?:\/\/([^/]*)/i.exec(text)?.[1];
  if (authority === undefined || authority === '' || authority.includes('@')) return fix;
  try {
    const url = new URL(text);
    if ((url.protocol !== 'http:' && url.protocol !== 'https:') || !url.hostname || url.username || url.password || url.search || url.hash) return fix;
  } catch { return fix; }
  return '';
}

function mountProblem(raw: string, label: string): string {
  const text = raw.trim();
  return text === '' || mountPattern.test(text) ? '' : `${label} chỉ gồm 1–64 ký tự chữ, số, gạch dưới hoặc gạch ngang (ASCII).`;
}

// Shape check only: CERTIFICATE PEM blocks with base64 bodies and nothing else.
// It proves nothing about X.509 validity, trust or reachability; the backend
// parses and verifies for real.
export function caShapeProblem(raw: string): string {
  const text = raw.trim();
  if (!text) return '';
  if (bytes(text) > MAX_CA_BYTES) return 'CA PEM vượt quá 64 KiB.';
  const block = /-----BEGIN CERTIFICATE-----\r?\n([A-Za-z0-9+/=\r\n]+?)\r?\n-----END CERTIFICATE-----/y;
  let index = 0;
  let count = 0;
  while (index < text.length) {
    block.lastIndex = index;
    const match = block.exec(text);
    if (!match) return 'CA PEM chỉ nhận các khối CERTIFICATE (base64), không có nội dung thừa. Đây mới là kiểm tra hình dạng, chưa xác nhận chứng chỉ hợp lệ hay đáng tin.';
    const body = (match[1] ?? '').replace(/\s+/g, '');
    if (body.length % 4 !== 0 || !base64Pattern.test(body)) return 'Nội dung base64 của CA PEM không hợp lệ.';
    count += 1;
    index = block.lastIndex;
    while (index < text.length && /\s/.test(text[index] ?? '')) index += 1;
  }
  return count > 0 ? '' : 'CA PEM cần ít nhất một khối CERTIFICATE.';
}

export function buildPayload(form: Form) {
  const backendAddress = form.backendAddress.trim();
  const workloadAddress = form.sameAddress ? backendAddress : form.workloadAddress.trim();
  const tlsCaPem = form.tlsCaPem.trim();
  return { name: form.name.trim(), backendAddress, workloadAddress, mount: form.mount.trim() || DEFAULT_MOUNT, authMount: form.authMount.trim() || DEFAULT_AUTH_MOUNT, ...(tlsCaPem ? { tlsCaPem } : {}), token: form.token };
}

export function validate(form: Form): Problems {
  const problems: Problems = {};
  const name = form.name.trim();
  if (!name) problems.name = 'Tên kho là bắt buộc.';
  else if ([...name].length > MAX_NAME_CHARS || controlPattern.test(name)) problems.name = `Tên kho tối đa ${MAX_NAME_CHARS} ký tự và không chứa ký tự điều khiển.`;
  const backend = addressProblem(form.backendAddress, 'Địa chỉ backend');
  if (backend) problems.backendAddress = backend;
  if (!form.sameAddress) {
    const workload = addressProblem(form.workloadAddress, 'Địa chỉ workload');
    if (workload) problems.workloadAddress = workload;
  }
  const mount = mountProblem(form.mount, 'Mount KV v2');
  if (mount) problems.mount = mount;
  const authMount = mountProblem(form.authMount, 'Mount xác thực Kubernetes');
  if (authMount) problems.authMount = authMount;
  const ca = caShapeProblem(form.tlsCaPem);
  if (ca) problems.tlsCaPem = ca;
  const token = form.token.trim();
  if (!token) problems.token = 'Token là bắt buộc.';
  else if (bytes(token) > MAX_TOKEN_BYTES || controlPattern.test(token)) problems.token = `Token tối đa ${MAX_TOKEN_BYTES} byte UTF-8 và không chứa ký tự điều khiển.`;
  if (Object.keys(problems).length === 0 && bytes(JSON.stringify(buildPayload(form))) > MAX_ENVELOPE_BYTES) problems.form = 'Dữ liệu gửi đi vượt quá 256 KiB; hãy rút gọn địa chỉ hoặc CA PEM.';
  return problems;
}

// Fixed Vietnamese guidance by HTTP status. Raw backend text is never shown;
// stable error codes for the whole Console are T20.
export function failureGuidance(error: unknown): string {
  if (!(error instanceof ApiError)) return 'Chưa nhận được phản hồi từ máy chủ nên chưa biết kho đã được lưu hay chưa. Token đã bị xóa. Hãy tải lại danh sách để kiểm tra trước khi đăng ký lại; Console không tự gửi lại.';
  switch (error.status) {
    case 400: return 'Máy chủ từ chối dữ liệu đăng ký. Kiểm tra tên, địa chỉ, mount, CA PEM và token rồi nhập lại token.';
    case 401: return 'Phiên đăng nhập đã hết hạn. Đăng nhập lại rồi nhập lại token.';
    case 403: return 'Tài khoản không có quyền đăng ký kho bí mật. Cần vai trò Kỹ sư nền tảng hoặc Quản trị.';
    case 413: return 'Dữ liệu đăng ký quá lớn. Rút gọn CA PEM rồi nhập lại token.';
    case 422: return 'Không xác minh được kho bí mật nên chưa lưu kho. Kiểm tra địa chỉ backend, mount, CA PEM và quyền của token (đọc/ghi KV v2, quản lý policy và role xác thực Kubernetes), rồi nhập lại token.';
    case 503: return 'Kho lưu thông tin xác thực của nền tảng chưa sẵn sàng. Liên hệ quản trị nền tảng rồi thử lại.';
    default: return error.status >= 500 ? 'Máy chủ gặp lỗi khi đăng ký. Kho có thể chưa được lưu; tải lại danh sách để kiểm tra, token đã bị xóa nên cần nhập lại.' : 'Không đăng ký được kho bí mật. Kiểm tra dữ liệu rồi nhập lại token.';
  }
}

export function backendVerification(store: Store): string {
  const verified = store.verification?.verified;
  if (verified === true) return 'Backend đã xác minh khi đăng ký';
  if (verified === false) return 'Backend xác minh không đạt';
  return 'Chưa có kết quả xác minh backend';
}

export function kubernetesAuth(store: Store): string {
  switch (String(store.verification?.kubernetesAuth ?? '').toUpperCase()) {
    case 'CONFIGURED': return 'Đã cấu hình';
    case 'ABSENT': return 'Chưa cấu hình';
    default: return 'Chưa rõ';
  }
}

export function statusLabel(status: string): string {
  switch (status) {
    case 'READY': return 'Sẵn sàng';
    case 'REJECTED': return 'Bị từ chối';
    case '': return 'Chưa rõ';
    default: return `Trạng thái chưa nhận diện (${status})`;
  }
}

// Platform Engineer registration of Organization-scoped Vault KV v2 stores
// (UC-04 SS-01..06, T05). The token is write-only: it is cleared after every
// server failure and after success, and never shown again; the list shows only
// safe metadata.
export function SecretStoresPage() {
  const [stores, setStores] = useState<readonly Store[]>([]);
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading');
  const [form, setForm] = useState<Form>(empty);
  const [advanced, setAdvanced] = useState(false);
  const [saving, setSaving] = useState(false);
  const [problems, setProblems] = useState<Problems>({});
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const submitting = useRef(false);
  const load = useCallback(async () => {
    setState('loading');
    try { const response = await api<{ secretStores: Store[] }>('/secret-stores'); setStores(response.secretStores ?? []); setState('ready'); } catch { setState('error'); }
  }, []);
  useEffect(() => { void load(); }, [load]);

  async function register() {
    if (submitting.current) return;
    const found = validate(form);
    setProblems(found); setSuccess('');
    if (Object.keys(found).length > 0) {
      setError('');
      // Advanced fields must not hide an actionable error.
      if (found.mount || found.authMount || found.tlsCaPem) setAdvanced(true);
      return;
    }
    submitting.current = true;
    setSaving(true); setError('');
    let created: Store;
    try {
      created = await api<Store>('/secret-stores', { method: 'POST', body: JSON.stringify(buildPayload(form)) });
    } catch (err) {
      // The token is cleared on every server failure; metadata and choices stay.
      setForm((value) => ({ ...value, token: '' }));
      setError(failureGuidance(err));
      submitting.current = false; setSaving(false);
      return;
    }
    setForm(empty); setAdvanced(false); setProblems({});
    setSuccess(`Kho bí mật "${created.name}" đã sẵn sàng. Nhà phát triển có thể chọn kho này trong Cài đặt môi trường.`);
    submitting.current = false; setSaving(false);
    await load();
  }
  const set = (key: 'name' | 'backendAddress' | 'workloadAddress' | 'mount' | 'authMount' | 'tlsCaPem' | 'token') => (event: { target: { value: string } }) => setForm((value) => ({ ...value, [key]: event.target.value }));
  // Help and error text sit outside the <label> so the accessible name stays the bare field name.
  const describe = (field: Field) => ({ 'aria-describedby': `ss-${field}-help${problems[field] ? ` ss-${field}-error` : ''}`, ...(problems[field] ? { 'aria-invalid': true } : {}) });
  const note = (field: Field, help: string) => <><small id={`ss-${field}-help`} className="muted">{help}</small>{problems[field] ? <small id={`ss-${field}-error`} className="field-error" role="alert">{problems[field]}</small> : null}</>;
  const effectiveMount = form.mount.trim() || DEFAULT_MOUNT;
  const effectiveAuthMount = form.authMount.trim() || DEFAULT_AUTH_MOUNT;
  const workloadValue = form.sameAddress ? form.backendAddress : form.workloadAddress;
  return <section className="page">
    <header className="page-header"><div><p className="eyebrow">Nền tảng</p><h1>Kho bí mật</h1><p>Đăng ký kho Vault KV v2 để Nhà phát triển chọn cho từng môi trường. Token chỉ nằm trong kho thông tin xác thực của nền tảng và không bao giờ hiển thị lại.</p></div></header>
    {success ? <div className="form-success" role="status">{success}</div> : null}
    <section className="content-panel" aria-label="Kho bí mật đã đăng ký">
      <div className="section-header"><div><h2>Kho đã đăng ký</h2><p>“Sẵn sàng” nghĩa là backend đã được xác minh lúc đăng ký; không đảm bảo ứng dụng đã sẵn sàng triển khai.</p></div></div>
      {state === 'loading' ? <p role="status" aria-busy="true">Đang tải danh sách kho bí mật…</p> : null}
      {state === 'error' ? <div className="form-error" role="alert">Không tải được danh sách kho bí mật. Danh sách dưới đây (nếu có) có thể đã cũ; dữ liệu trong form được giữ nguyên.<Button type="button" onClick={() => void load()}>Thử lại</Button></div> : null}
      {state === 'ready' && stores.length === 0 ? <div className="section-empty">Chưa có kho bí mật nào được đăng ký.</div> : null}
      {stores.length > 0 ? <div className="settings-table"><div className="settings-table-head"><span>Kho</span><span>Địa chỉ</span><span>Mount và CA</span><span>Trạng thái và xác minh</span></div>
        {stores.map((store) => <div className="settings-table-row" key={store.key}>
          <strong>{store.name}{store.legacy ? ' (cấu hình cũ)' : ''}<small className="muted"> {store.key} · {store.provider}</small></strong>
          <span>Backend: {store.backendAddress}<br />Workload: {store.workloadAddress}</span>
          <span>KV v2: {store.mount}<br />Xác thực Kubernetes: {store.authMount}<br />CA: {store.tlsCustomCa ? 'tùy chỉnh' : 'mặc định'}</span>
          <span>Trạng thái: {statusLabel(store.status)}<br />Xác minh backend: {backendVerification(store)}<br />Xác thực Kubernetes: {kubernetesAuth(store)}</span></div>)}</div> : null}
    </section>
    <form className="create-form secret-store-form" aria-label="Đăng ký kho bí mật" noValidate onSubmit={(event) => { event.preventDefault(); void register(); }}>
      <h2>Đăng ký kho Vault KV v2</h2>
      {error ? <div className="form-error" role="alert">{error}</div> : null}
      {problems.form ? <div className="form-error" role="alert">{problems.form}</div> : null}
      <fieldset className="form-fieldset" disabled={saving} aria-busy={saving}>
        <div className="editor-grid">
          <div className="field"><label>Tên kho<input value={form.name} onChange={set('name')} placeholder="Vault của nhóm" {...describe('name')} /></label>{note('name', 'Tối đa 100 ký tự.')}</div>
          <div className="field"><label>Địa chỉ backend<input value={form.backendAddress} onChange={set('backendAddress')} placeholder="https://vault.example.com:8200" {...describe('backendAddress')} /></label>{note('backendAddress', 'Orchestrator dùng địa chỉ này để đọc và ghi giá trị.')}</div>
          <label className="checkbox"><input type="checkbox" checked={form.sameAddress} onChange={(event) => setForm((value) => ({ ...value, sameAddress: event.target.checked }))} />Dùng cùng địa chỉ backend</label>
          <div className="field"><label>Địa chỉ workload<input value={workloadValue} onChange={set('workloadAddress')} readOnly={form.sameAddress} aria-readonly={form.sameAddress} placeholder="http://vault.vault.svc:8200" {...describe('workloadAddress')} /></label>{note('workloadAddress', form.sameAddress ? 'Đang dùng địa chỉ backend. Bỏ chọn để khôi phục địa chỉ workload riêng đã nhập.' : 'Địa chỉ mà Pod workload và Vault Secrets Operator dùng bên trong cluster.')}</div>
          <div className="field"><label>Token Vault<input type="password" autoComplete="off" value={form.token} onChange={set('token')} {...describe('token')} /></label>{note('token', 'Cần quyền giá trị KV v2 cùng quyền quản lý policy workload và role xác thực Kubernetes. Được xác minh trước khi kho chuyển sang “Sẵn sàng”.')}</div>
        </div>
        <section className="form-info" aria-label="Tóm tắt cấu hình hiệu lực">
          <strong>Cấu hình hiệu lực</strong>
          <ul>
            <li>Mount KV v2: <code>{effectiveMount}</code>{form.mount.trim() ? '' : ' (mặc định)'}</li>
            <li>Mount xác thực Kubernetes: <code>{effectiveAuthMount}</code>{form.authMount.trim() ? '' : ' (mặc định)'}</li>
            <li>CA: {form.tlsCaPem.trim() ? 'tùy chỉnh' : 'mặc định của hệ thống'}. TLS luôn được xác minh.</li>
          </ul>
        </section>
        <div className="section-header"><div><h3>Nâng cao</h3><p>Mount và CA PEM tùy chọn. Thu gọn không xóa dữ liệu đã nhập.</p></div><Button type="button" aria-expanded={advanced} aria-controls="secret-store-advanced" onClick={() => setAdvanced(!advanced)}>{advanced ? 'Ẩn nâng cao' : 'Hiện nâng cao'}</Button></div>
        <div id="secret-store-advanced" hidden={!advanced} className="editor-grid">
          <div className="field"><label>Mount KV v2<input value={form.mount} onChange={set('mount')} placeholder={DEFAULT_MOUNT} {...describe('mount')} /></label>{note('mount', `Để trống để dùng mặc định ${DEFAULT_MOUNT}.`)}</div>
          <div className="field"><label>Mount xác thực Kubernetes<input value={form.authMount} onChange={set('authMount')} placeholder={DEFAULT_AUTH_MOUNT} {...describe('authMount')} /></label>{note('authMount', `Để trống để dùng mặc định ${DEFAULT_AUTH_MOUNT}.`)}</div>
          <div className="field"><label>Chứng chỉ CA tùy chỉnh (PEM, tùy chọn)<textarea value={form.tlsCaPem} onChange={set('tlsCaPem')} rows={4} spellCheck={false} {...describe('tlsCaPem')} /></label>{note('tlsCaPem', 'Console chỉ kiểm hình dạng PEM CERTIFICATE; không chứng minh chứng chỉ hợp lệ, đáng tin hay kết nối được. Backend kiểm tra và xác minh thật. TLS luôn được xác minh.')}</div>
        </div>
        <div className="form-actions"><Button tone="primary" type="submit" disabled={saving}>{saving ? 'Đang xác minh…' : 'Xác minh và đăng ký'}</Button></div>
      </fieldset>
    </form>
  </section>;
}
