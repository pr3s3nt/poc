import { useEffect, useId, useRef, useState, type ChangeEvent, type FormEvent } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { specificGuidance } from '../../shared/api/errors';
import { Button } from '../../shared/ui/Button';
import { AwsConnectionForm } from './AwsConnectionForm';
import { useCatalogList } from './useCatalogList';

type Connection = {
  key: string; name?: string; kind: string; authenticationType?: string; status: string;
  config?: { cluster?: string; kubeContext?: string; endpoint?: string; region?: string; accountId?: string; allowedRegions?: string[] };
  verification?: { identity?: string; regionCheck?: string; capabilityCheck?: string };
};
type ConnectionKind = 'kubernetes' | 'aws';
type KubeconfigContext = { name: string; cluster: string; endpoint: string };
type Source = 'upload' | 'paste';

// Same bound as the backend document limit (UC-04 API mapping).
const maxKubeconfigBytes = 1 << 20;
const maxNameLength = 100;
// A pasted document is inspected once it has stopped changing for this long.
const pasteDebounceMs = 500;

function readText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ''));
    reader.onerror = () => reject(new Error('read failed'));
    reader.readAsText(file);
  });
}

const loadConnections = async () => (await api<{ connections: Connection[] }>('/connections')).connections ?? [];

// Fixed Vietnamese guidance by operation and HTTP status. Raw backend or
// provider text never reaches the page; specific guidance comes from stable error codes.
const listFailure = () => 'Không tải được danh sách kết nối. Kiểm tra kết nối mạng rồi thử lại; dữ liệu trong form được giữ nguyên.';

function inspectGuidance(reason: unknown): string {
  if (!(reason instanceof ApiError)) return 'Chưa kiểm tra được kubeconfig vì chưa nhận được phản hồi từ máy chủ. Nhấn “Kiểm tra lại kubeconfig”.';
  switch (reason.status) {
    case 400: case 422: return `Không đọc được kubeconfig. ${specificGuidance(reason.code, reason.message) ?? 'Kiểm tra file hợp lệ, có context dùng token hoặc client certificate nhúng sẵn, rồi kiểm tra lại.'}`;
    case 401: return 'Phiên đăng nhập đã hết hạn. Đăng nhập lại rồi kiểm tra lại kubeconfig.';
    case 403: return 'Tài khoản không có quyền đọc kubeconfig. Cần vai trò Kỹ sư nền tảng hoặc Quản trị.';
    case 413: return 'Kubeconfig lớn hơn 1 MiB. Rút gọn nội dung rồi thử lại.';
    default: return reason.status >= 500 ? 'Máy chủ gặp lỗi khi đọc kubeconfig. Nhấn “Kiểm tra lại kubeconfig”.' : 'Không kiểm tra được kubeconfig. Nhấn “Kiểm tra lại kubeconfig”.';
  }
}

function registerGuidance(reason: unknown): string {
  // No HTTP status: the request may or may not have reached the backend.
  if (!(reason instanceof ApiError)) return 'Chưa nhận được phản hồi từ máy chủ nên chưa biết kết nối đã được lưu hay chưa. Hãy tải lại danh sách để kiểm tra trước khi đăng ký lại; Console không tự gửi lại.';
  switch (reason.status) {
    case 400: return `Máy chủ từ chối dữ liệu đăng ký. ${reason.code && reason.code !== 'INVALID_REQUEST' ? specificGuidance(reason.code, reason.message) ?? '' : 'Kiểm tra tên kết nối, kubeconfig và context rồi thử lại.'}`;
    case 401: return 'Phiên đăng nhập đã hết hạn. Đăng nhập lại rồi thử lại.';
    case 403: return 'Tài khoản không có quyền đăng ký kết nối. Cần vai trò Kỹ sư nền tảng hoặc Quản trị.';
    case 409: return 'Không đăng ký được vì tên hoặc khóa kết nối đã tồn tại. Đổi tên kết nối rồi thử lại.';
    case 413: return 'Dữ liệu đăng ký quá lớn. Rút gọn kubeconfig rồi thử lại.';
    case 422: return `Không xác minh được cluster nên chưa lưu kết nối. ${reason.code && reason.code !== 'VERIFICATION_FAILED' ? reason.message : 'Kiểm tra endpoint, thông tin xác thực trong kubeconfig và quyền của context, rồi thử lại.'}`;
    case 503: return 'Kho lưu thông tin xác thực của nền tảng chưa sẵn sàng. Liên hệ quản trị nền tảng rồi thử lại.';
    default: return reason.status >= 500 ? 'Máy chủ gặp lỗi khi đăng ký. Kết nối có thể chưa được lưu; tải lại danh sách để kiểm tra.' : 'Không đăng ký được kết nối. Kiểm tra dữ liệu rồi thử lại.';
  }
}

// Name suggested from the selected context; the cluster is the clearer label.
function suggestName(context: KubeconfigContext | undefined): string {
  return context ? (context.cluster || context.name).slice(0, maxNameLength) : '';
}

export function ConnectionsPage() {
  const { items, loading, loadError, reload } = useCatalogList(loadConnections, listFailure);
  // The Console shows READY Connections only (UC-04 UI states).
  const connections = items.filter((connection) => connection.status === 'READY');
  // Once the user edits the name (even to blank) the suggestion never applies
  // again until the form is reset.
  const [typedName, setTypedName] = useState<string | null>(null);
  const [kind, setKind] = useState<ConnectionKind>('kubernetes');
  const [source, setSource] = useState<Source>('upload');
  const [kubeconfig, setKubeconfig] = useState('');
  const [fileLabel, setFileLabel] = useState('');
  const [hidden, setHidden] = useState(true);
  const [contexts, setContexts] = useState<KubeconfigContext[]>([]);
  const [selected, setSelected] = useState('');
  const [reading, setReading] = useState(false);
  const [inspecting, setInspecting] = useState(false);
  const [canRetry, setCanRetry] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const fileInput = useRef<HTMLInputElement>(null);
  const formId = useId();
  // Every document change bumps the revision and every inspection takes a new
  // request id, so a late file read or inspection response (also a retry of
  // the same document) can never restore an older document's contexts.
  const revision = useRef(0);
  const request = useRef(0);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  function cancelTimer() { clearTimeout(timer.current); timer.current = undefined; }
  useEffect(() => () => { revision.current += 1; request.current += 1; cancelTimer(); }, []);

  function replaceDocument(next: string) {
    revision.current += 1; request.current += 1; cancelTimer();
    setKubeconfig(next); setContexts([]); setSelected(''); setReading(false); setInspecting(false); setCanRetry(false); setError('');
  }

  function chooseSource(next: Source) {
    if (next === source) return;
    setSource(next); setFileLabel('');
    if (fileInput.current) fileInput.current.value = '';
    replaceDocument('');
  }

  async function inspect(text: string, pending: number) {
    if (new Blob([text]).size > maxKubeconfigBytes) { setError('Kubeconfig lớn hơn 1 MiB.'); return; }
    const id = ++request.current;
    const current = () => pending === revision.current && id === request.current;
    setInspecting(true); setError(''); setCanRetry(false); setNotice('');
    try {
      const result = await api<{ contexts: KubeconfigContext[] }>('/connections/kubernetes/inspect', { method: 'POST', body: JSON.stringify({ kubeconfig: text }) });
      if (!current()) return;
      const found = result.contexts ?? [];
      setContexts(found);
      // One context is selected for the user; several never guess from current-context.
      setSelected(found.length === 1 && found[0] ? found[0].name : '');
    } catch (reason) {
      if (current()) { setError(inspectGuidance(reason)); setCanRetry(true); }
    } finally {
      if (current()) setInspecting(false);
    }
  }

  async function chooseFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    replaceDocument('');
    setFileLabel('');
    if (!file) return;
    if (file.size > maxKubeconfigBytes) { setError('File kubeconfig lớn hơn 1 MiB.'); return; }
    const pending = revision.current;
    setReading(true);
    let text: string;
    try { text = await readText(file); } catch { if (pending === revision.current) { setReading(false); setError('Không đọc được file kubeconfig.'); } return; }
    if (pending !== revision.current) return;
    setReading(false); setKubeconfig(text); setFileLabel(file.name);
    void inspect(text, pending);
  }

  function pasteDocument(next: string) {
    replaceDocument(next);
    if (!next.trim()) return;
    const pending = revision.current;
    timer.current = setTimeout(() => { timer.current = undefined; void inspect(next, pending); }, pasteDebounceMs);
  }

  const chosen = contexts.find((context) => context.name === selected);
  const name = typedName ?? suggestName(chosen);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving || inspecting || reading) return;
    if (!name.trim()) { setError('Nhập tên kết nối.'); return; }
    if (!chosen) { setError('Tải lên hoặc dán kubeconfig rồi chọn context trước khi kiểm tra và lưu.'); return; }
    cancelTimer();
    setError(''); setNotice(''); setSaving(true);
    let created: Connection;
    try {
      created = await api<Connection>('/connections/kubernetes', { method: 'POST', body: JSON.stringify({ name, kubeconfig, context: selected }) });
    } catch (reason) {
      setError(registerGuidance(reason)); setSaving(false);
      // An unknown outcome is checked by reloading the list, never by resubmitting.
      if (!(reason instanceof ApiError)) void reload();
      return;
    }
    // Committed: report it, clear every credential and the form, then reload.
    setNotice(`Đã đăng ký kết nối ${created.name ?? name.trim()} (${created.key}).`);
    setTypedName(null); setFileLabel(''); setHidden(true);
    if (fileInput.current) fileInput.current.value = '';
    replaceDocument('');
    setSaving(false);
    await reload();
  }

  const busy = saving || inspecting || reading;
  return <section className="page"><header className="page-header"><div><p className="eyebrow">Nền tảng</p><h1>Kết nối</h1><p>Đăng ký cluster Kubernetes có sẵn từ kubeconfig hoặc tài khoản AWS bằng access key. Orchestrator lưu thông tin xác thực trong kho thông tin xác thực của nền tảng, không lưu trên máy chủ backend.</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>Kết nối đã đăng ký</h2></div>{loadError ? <div className="form-error" role="alert" aria-label="Lỗi tải danh sách">{loadError} <Button type="button" onClick={() => void reload()}>Thử lại</Button></div> : loading ? <p>Đang tải danh sách kết nối…</p> : connections.length === 0 ? <p>Chưa có kết nối nào được đăng ký.</p> : <ConnectionTable connections={connections} />}</section>
    <section className="content-panel"><div className="connection-register">
      <div className="source-choices connection-kind" role="radiogroup" aria-label="Loại kết nối">
        <label className="source-choice"><input type="radio" name="connection-kind" checked={kind === 'kubernetes'} onChange={() => setKind('kubernetes')} /> Cluster Kubernetes</label>
        <label className="source-choice"><input type="radio" name="connection-kind" checked={kind === 'aws'} onChange={() => setKind('aws')} /> Tài khoản AWS</label>
      </div>
      {kind === 'aws' ? <>
        <div className="section-header"><div><h2>Đăng ký tài khoản AWS</h2><p>Nhập access key dài hạn của một người dùng IAM và các vùng được phép. “Kiểm tra và lưu” xác minh danh tính, từng vùng và quyền cấp phát VPC, EKS, ElastiCache, Aurora bằng các lời gọi chỉ đọc; đăng ký không tạo tài nguyên AWS và không tự chọn kết nối cho môi trường nào.</p></div></div>
        <AwsConnectionForm onRegistered={reload} />
      </> : <>
      <div className="section-header"><div><h2>Đăng ký cluster Kubernetes</h2><p>Tải lên hoặc dán kubeconfig có token hoặc client certificate nhúng sẵn. Console tự đọc danh sách context; “Kiểm tra và lưu” mới xác minh quyền truy cập API mà không tạo tài nguyên.</p></div></div>
      <form onSubmit={submit}><fieldset className="form-fieldset connection-form" disabled={saving} aria-busy={busy}>
        <label className="connection-field">Tên kết nối<input value={name} required maxLength={maxNameLength} onChange={(event) => setTypedName(event.target.value)} /></label>
        <div className="connection-source">
          <div className="source-choices" role="radiogroup" aria-label="Nguồn kubeconfig">
            <label className="source-choice"><input type="radio" name="kubeconfig-source" checked={source === 'upload'} onChange={() => chooseSource('upload')} /> Tải lên file kubeconfig</label>
            <label className="source-choice"><input type="radio" name="kubeconfig-source" checked={source === 'paste'} onChange={() => chooseSource('paste')} /> Dán nội dung kubeconfig</label>
          </div>
          {source === 'upload'
            ? <div className="connection-field"><span id={`${formId}-file`}>File kubeconfig</span><div className="file-picker">
                <span className="button file-picker-button">Chọn file<input ref={fileInput} type="file" aria-labelledby={`${formId}-file`} aria-describedby={`${formId}-file-name`} onChange={(event) => void chooseFile(event)} /></span>
                <small id={`${formId}-file-name`} className="file-picker-name">{fileLabel ? `Đã chọn file: ${fileLabel}. Nội dung file không được hiển thị.` : 'Chưa chọn file'}</small>
              </div></div>
            : <label className="connection-field">Nội dung kubeconfig<textarea aria-label="Nội dung kubeconfig" className={hidden ? 'masked-text' : undefined} rows={8} spellCheck={false} autoComplete="off" value={kubeconfig} onChange={(event) => pasteDocument(event.target.value)} /></label>}
          <div className="source-actions">
            {canRetry ? <Button type="button" disabled={busy || !kubeconfig.trim()} onClick={() => void inspect(kubeconfig, revision.current)}>Kiểm tra lại kubeconfig</Button> : null}
            {source === 'paste' ? <label className="inline-check"><input type="checkbox" checked={!hidden} onChange={(event) => setHidden(!event.target.checked)} /> Hiện nội dung</label> : null}</div>
          {reading || inspecting ? <p className="muted" role="status" aria-live="polite">{reading ? 'Đang đọc file kubeconfig…' : 'Đang kiểm tra kubeconfig…'}</p> : null}
        </div>
        {contexts.length > 0 ? <div className="connection-destination">
          {contexts.length > 1 ? <label className="connection-field">Context<select value={selected} required onChange={(event) => setSelected(event.target.value)}><option value="">Chọn context</option>{contexts.map((context) => <option key={context.name} value={context.name}>{context.name}</option>)}</select></label> : null}
          {chosen ? <dl className="destination-summary" aria-label="Đích đã chọn">
            <div><dt>Context</dt> <dd>{chosen.name}</dd></div>
            <div><dt>Cluster</dt> <dd>{chosen.cluster}</dd></div>
            <div><dt>Endpoint</dt> <dd>{chosen.endpoint}</dd></div>
          </dl> : null}
        </div> : null}
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}
        <div className="connection-footer"><Button type="submit" tone="primary" disabled={busy}>{saving ? 'Đang kiểm tra và lưu…' : 'Kiểm tra và lưu'}</Button></div>
      </fieldset></form>
      </>}
    </div></section>
  </section>;
}

// Destination shows only the public fields that apply to the kind, so AWS
// records never carry empty Kubernetes columns.
function destination(connection: Connection): [string, string][] {
  const config = connection.config ?? {};
  const fields: [string, string | undefined][] = connection.kind === 'AWS'
    ? [['Vùng', config.allowedRegions?.length ? config.allowedRegions.join(', ') : config.region], ['Tài khoản', config.accountId]]
    : [['Cluster', config.cluster], ['Endpoint', config.endpoint]];
  return fields.filter((field): field is [string, string] => Boolean(field[1]));
}

// Truthful AWS evidence: only a record that carries all three registration
// checks claims them; anything else says no provisioning evidence exists.
function awsEvidence(connection: Connection): string {
  const v = connection.verification;
  return v?.identity === 'VERIFIED' && v.regionCheck === 'VERIFIED' && v.capabilityCheck === 'VERIFIED'
    ? 'Đã xác minh khi đăng ký: danh tính, từng vùng và quyền cấp phát (chưa tạo tài nguyên).'
    : 'Chưa có bằng chứng xác minh quyền cấp phát.';
}

function ConnectionTable({ connections }: { connections: Connection[] }) {
  return <table className="connection-table" aria-label="Kết nối đã đăng ký">
    <thead><tr><th scope="col">Tên</th><th scope="col">Loại</th><th scope="col">Đích</th><th scope="col">Trạng thái</th></tr></thead>
    <tbody>{connections.map((connection) => {
      const label = connection.name || connection.key;
      const fields = destination(connection);
      return <tr key={connection.key}>
        <td data-label="Tên"><strong>{label}</strong>{label !== connection.key ? <small className="connection-key">{connection.key}</small> : null}</td>
        <td data-label="Loại">{connection.kind}</td>
        <td data-label="Đích">{fields.length ? fields.map(([field, value]) => <span className="destination-line" key={field}><span className="muted">{field}</span> {value}</span>) : <span className="muted">—</span>}</td>
        <td data-label="Trạng thái"><span className={`status-badge${connection.status === 'READY' ? ' status-badge-ready' : ''}`}>{connection.status === 'READY' ? 'Sẵn sàng' : connection.status}</span>{connection.kind === 'AWS' ? <small className="connection-key">{awsEvidence(connection)}</small> : null}</td>
      </tr>;
    })}</tbody>
  </table>;
}
