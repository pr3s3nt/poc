import { useEffect, useRef, useState } from 'react';
import { api, ApiError, isBusy, isStaleConflict } from '../../shared/api/client';
import { toTarget, type APIEnvironment } from '../../shared/api/environment';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { environmentTitle, profileLabel, statusLabel } from '../environment/labels';

type Choice = { key: string; name: string; kind: string; status: string };
type ChoicesState = { status: 'loading' | 'error' } | { status: 'ready'; connections: readonly Choice[] };

export { toTarget };
const kindLabel = (kind?: string) => kind === 'AWS' ? 'AWS' : 'Kubernetes';
const eligibleKinds = new Set(['KUBERNETES', 'AWS']);
// Only a READY Connection of a deployable kind is a safe choice.
const isEligible = (item: Choice) => item.status === 'READY' && eligibleKinds.has(item.kind);

// Editable execution Connection of ONE Environment (UC-01 ES-01..06, ADR-012).
// Before runtime exists Save is a versioned metadata change; once resources
// exist a different destination is an explicit transition, never a silent Save.
// With no binding and exactly one eligible Connection that one is only SUGGESTED
// in the draft (not an Organization default or fallback); nothing persists
// until the user presses Save. The draft is derived, never copied from a reply,
// so a reload or a late response cannot overwrite what the user chose.
export function EnvironmentConnection({ application, environment, onTargetChange, onStartTransition }: {
  application: Application; environment: EnvironmentKey;
  onTargetChange(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void;
  onStartTransition?(destinationKey: string): void;
}) {
  const target = application.environments[environment];
  const busy = Boolean(target.activeOperation);
  const [choices, setChoices] = useState<ChoicesState>({ status: 'loading' });
  // `picked` is the user's own selection; undefined means "no pending edit".
  const [picked, setPicked] = useState<string>();
  const [saving, setSaving] = useState(false);
  // A stale conflict whose reload failed: saving stays off until a reload succeeds.
  const [needsReload, setNeedsReload] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [attempt, setAttempt] = useState(0);
  // Every Environment switch, every choices (re)load and unmount bumps a
  // generation: replies started for an earlier one are ignored instead of
  // changing the current selection, list or message. A server-confirmed save
  // updates the Environment it was made for even after this view is gone.
  const generation = useRef(0);
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; generation.current += 1; }; }, []);
  useEffect(() => { generation.current += 1; setPicked(undefined); setError(''); setNotice(''); setSaving(false); setNeedsReload(false); }, [application.id, environment]);
  useEffect(() => {
    const current = ++generation.current;
    setChoices({ status: 'loading' });
    api<{ connections: Choice[] }>('/application-connections')
      .then((response) => { if (current === generation.current) setChoices({ status: 'ready', connections: response.connections ?? [] }); })
      .catch(() => { if (current === generation.current) setChoices({ status: 'error' }); });
  }, [application.id, environment, attempt]);

  const title = environmentTitle(environment);
  const eligible = choices.status === 'ready' ? choices.connections.filter(isEligible) : [];
  // The bound Connection stays selectable even if it is no longer READY, so the
  // select always shows what is really configured.
  const bound = choices.status === 'ready' && target.configured && !eligible.some((item) => item.key === target.connectionKey)
    ? [choices.connections.find((item) => item.key === target.connectionKey) ?? { key: target.connectionKey, name: target.connectionName || target.connectionKey, kind: target.connectionKind ?? '', status: target.runtimeStatus }] : [];
  const options = [...bound, ...eligible];
  const suggestion = !target.configured && eligible.length === 1 ? eligible[0] : undefined;
  const candidate = picked ?? (target.configured ? target.connectionKey : suggestion?.key ?? '');
  const selected = options.some((item) => item.key === candidate) ? candidate : '';
  const suggested = picked === undefined && Boolean(suggestion) && selected === suggestion?.key;
  const lostPick = picked !== undefined && picked !== '' && selected === '' && choices.status === 'ready';

  // True only when the authoritative GET succeeded and returned this Environment.
  async function reloadEnvironment(current: number): Promise<boolean> {
    try {
      const response = await api<{ application: { environments?: APIEnvironment[] } }>(`/applications/${encodeURIComponent(application.id)}`);
      const item = response.application.environments?.find((env) => env.key === environment);
      if (!item) return false;
      if (current === generation.current || !mounted.current) onTargetChange(application.id, environment, toTarget(item));
      return true;
    } catch { return false; }
  }

  async function retryReload() {
    const current = ++generation.current;
    if (await reloadEnvironment(current) && current === generation.current) { setNeedsReload(false); setError(''); setNotice('Đã tải lại trạng thái mới nhất; xem lại lựa chọn rồi lưu nếu vẫn muốn đổi.'); }
    else if (current === generation.current) setError('Vẫn chưa tải lại được trạng thái môi trường. Lựa chọn của bạn được giữ nguyên; hãy thử lại.');
  }

  async function save() {
    if (!selected || saving || busy || needsReload) return;
    const current = ++generation.current;
    setSaving(true); setError(''); setNotice('');
    try {
      const response = await api<{ environment: APIEnvironment }>(`/applications/${encodeURIComponent(application.id)}/environments/${environment}/connection`, { method: 'PUT', body: JSON.stringify({ connectionKey: selected, expectedVersion: target.version }) });
      onTargetChange(application.id, environment, toTarget(response.environment));
      if (current === generation.current) { setPicked(undefined); setNotice(selected === target.connectionKey ? 'Đã chọn kết nối này; không có gì thay đổi.' : 'Đã lưu kết nối. Xem trước và Triển khai sẽ dùng kết nối này.'); }
    } catch (err) {
      if (current !== generation.current) return;
      if (isBusy(err)) {
        // Busy is not a version conflict: nothing is reloaded and the selection is kept.
        setError((err as ApiError).message);
      } else if (isStaleConflict(err) || (err instanceof ApiError && err.code === 'TRANSITION_REQUIRED')) {
        const base = (err as ApiError).message;
        const reloaded = await reloadEnvironment(current);
        if (current !== generation.current) return;
        setNeedsReload(!reloaded);
        setError(reloaded ? `${base} Đã tải lại trạng thái mới nhất; xem lại lựa chọn rồi bấm Lưu kết nối nếu vẫn muốn đổi.` : `${base} Chưa tải lại được trạng thái mới nhất; lựa chọn của bạn được giữ nguyên. Bấm “Tải lại trạng thái” rồi xem lại trước khi lưu.`);
      } else if (err instanceof ApiError && err.status === 422) {
        setError(err.code === 'CONNECTION_UNAVAILABLE' ? 'Kết nối đã chọn không còn khả dụng. Hãy chọn kết nối khác.' : err.message);
        setAttempt((value) => value + 1);
      } else {
        // A network failure does not prove the connection was not saved.
        setError(err instanceof ApiError ? `Không lưu được kết nối. ${err.message}` : 'Không lưu được kết nối. Chưa rõ kết quả; hãy tải lại trạng thái môi trường trước khi thử lại.');
      }
    } finally {
      if (current === generation.current) setSaving(false);
    }
  }

  const ready = choices.status === 'ready' && options.length > 0;
  const changed = selected !== target.connectionKey;
  const needsTransition = target.configured && target.runtimeExists && changed;
  return <section className="content-panel environment-connection" aria-label="Kết nối triển khai">
    <div className="section-header"><div><h2>Kết nối triển khai</h2><p>{target.configured ? `${title} triển khai tới kết nối này. Bạn có thể đổi kết nối; mỗi thay đổi có phiên bản.` : `${title} chưa có kết nối nên chưa thể xem trước hoặc triển khai. Vẫn có thể chuẩn bị biến và bản nháp workload.`}</p></div><span className="pending-pill">{target.configured ? `Thế hệ ${target.targetGeneration}` : 'Chưa cấu hình'}</span></div>
    {target.configured ? <dl className="connection-facts">
      <dt>Kết nối</dt><dd><strong>{target.connectionName || target.connectionKey}</strong> ({target.connectionKey})</dd>
      <dt>Loại</dt><dd>{kindLabel(target.connectionKind)}</dd>
      <dt>Phạm vi triển khai</dt><dd>{profileLabel(target.profile)}</dd>
      {target.region ? <><dt>Vùng</dt><dd>{target.region}</dd></> : null}
      <dt>Trạng thái runtime</dt><dd>{statusLabel(target.runtimeStatus)}</dd>
    </dl> : null}
    {error ? <div className="form-error" role="alert">{error}{needsReload ? <Button type="button" onClick={() => void retryReload()}>Tải lại trạng thái</Button> : null}</div> : null}
    {notice ? <div className="form-success" role="status">{notice}</div> : null}
    {busy ? <div className="form-info" role="status">Một thao tác đang chạy trên môi trường này. Cài đặt chỉ đọc cho đến khi thao tác kết thúc.</div> : null}
    {choices.status === 'loading' ? <p role="status" aria-busy="true">Đang tải danh sách kết nối…</p> : null}
    {choices.status === 'error' ? <div className="form-error" role="alert">Không tải được danh sách kết nối.<Button type="button" onClick={() => setAttempt((value) => value + 1)}>Thử lại</Button></div> : null}
    {choices.status === 'ready' && options.length === 0 ? <div className="form-info" role="status">Chưa có kết nối Sẵn sàng (READY) để chọn. Nhờ Kỹ sư nền tảng đăng ký kết nối rồi bấm Thử lại.<Button type="button" onClick={() => setAttempt((value) => value + 1)}>Thử lại</Button></div> : null}
    {ready ? <>
      <label htmlFor="environment-connection">Kết nối cho {title}</label>
      <select id="environment-connection" value={selected} disabled={saving || busy} onChange={(event) => { setPicked(event.target.value); setNotice(''); }}>
        {!target.configured || !selected ? <option value="" disabled>Chọn một kết nối</option> : null}
        {options.map((item) => <option key={item.key} value={item.key}>{item.name} ({item.key}) · {kindLabel(item.kind)}{item.status !== 'READY' ? ` · ${statusLabel(item.status)}` : ''}</option>)}
      </select>
      {suggested ? <div className="form-info" role="status">Gợi ý chưa lưu: <strong>{suggestion?.name}</strong> là kết nối Sẵn sàng duy nhất. Đây chỉ là lựa chọn nháp, không phải mặc định của tổ chức; môi trường chỉ dùng kết nối này sau khi bạn bấm Lưu kết nối.</div> : null}
      {lostPick ? <div className="form-info" role="status">Kết nối bạn chọn trước đó không còn Sẵn sàng. Hãy chọn lại.</div> : null}
      {!target.configured && options.length > 1 && !selected ? <small>Có nhiều kết nối Sẵn sàng; hãy tự chọn một kết nối, hệ thống không chọn thay bạn.</small> : null}
      {needsTransition
        ? <><small>Môi trường này đã có tài nguyên runtime. Đổi đích triển khai sẽ tạo một thế hệ mới, có thể kèm chuyển dữ liệu PostgreSQL, và giữ thế hệ cũ để bạn xem lại.</small>
          <div className="form-actions"><Button tone="primary" disabled={busy || !selected} onClick={() => onStartTransition?.(selected)}>Xem lại chuyển đích…</Button></div></>
        : <><small>{target.configured ? 'Chỉ lưu được kết nối khác khi chưa có tài nguyên runtime.' : 'Chọn một kết nối rồi bấm Lưu. Bạn có thể đổi lại sau.'}</small>
          <div className="form-actions"><Button tone="primary" disabled={!selected || saving || busy || needsReload || (!changed && target.configured)} onClick={() => void save()}>{saving ? 'Đang lưu kết nối…' : 'Lưu kết nối'}</Button></div></>}
    </> : null}
  </section>;
}
