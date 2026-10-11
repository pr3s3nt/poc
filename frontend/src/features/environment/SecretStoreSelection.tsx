import { useEffect, useRef, useState } from 'react';
import { ApiError, isBusy, isStaleConflict } from '../../shared/api/client';
import { toTarget } from '../../shared/api/environment';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { environmentTitle, statusLabel } from './labels';
import { fetchEnvironment, listStoreChoices, setSecretStore, type StoreChoice } from './api';

type State = { status: 'loading' | 'error' } | { status: 'ready'; stores: readonly StoreChoice[] };

// Editable Secret Store Connection of ONE Environment (UC-01/UC-12, ADR-012).
// Changing it copies the desired Secrets first and commits only when every copy
// verified; running workloads move after the next Preview and Deploy.
// With no binding and exactly one READY store, that store is only SUGGESTED in
// the draft (not an Organization default or fallback); nothing persists until
// Save. The draft is derived from `picked`, so a reload or a late response never
// overwrites the user's choice, and a bound store is never retargeted.
export function SecretStoreSelection({ application, environment, configVersion, onTargetChange, onConfigurationChanged }: {
  application: Application; environment: EnvironmentKey; configVersion: number;
  onTargetChange(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void;
  onConfigurationChanged(): void;
}) {
  const target = application.environments[environment];
  const busy = Boolean(target.activeOperation);
  const [choices, setChoices] = useState<State>({ status: 'loading' });
  const [picked, setPicked] = useState<string>();
  const [saving, setSaving] = useState(false);
  // A stale conflict whose reload failed: saving stays off until a reload succeeds.
  const [needsReload, setNeedsReload] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [attempt, setAttempt] = useState(0);
  const generation = useRef(0);
  // `scope` changes only with the Application/Environment or unmount (not with a
  // choices reload), so a saved store refreshes the configuration of the page the
  // user is still on and never of one they have left.
  const scope = useRef(0);
  useEffect(() => () => { generation.current += 1; scope.current += 1; }, []);
  useEffect(() => { scope.current += 1; generation.current += 1; setPicked(undefined); setError(''); setNotice(''); setSaving(false); setNeedsReload(false); }, [application.id, environment]);
  useEffect(() => {
    const current = ++generation.current;
    setChoices({ status: 'loading' });
    listStoreChoices().then((stores) => { if (current === generation.current) setChoices({ status: 'ready', stores }); }).catch(() => { if (current === generation.current) setChoices({ status: 'error' }); });
  }, [application.id, environment, attempt]);

  const title = environmentTitle(environment);
  const eligible = choices.status === 'ready' ? choices.stores.filter((item) => item.status === 'READY') : [];
  // The bound store stays listed even if it is no longer READY.
  const bound = choices.status === 'ready' && target.secretStoreKey && !eligible.some((item) => item.key === target.secretStoreKey)
    ? [choices.stores.find((item) => item.key === target.secretStoreKey) ?? { key: target.secretStoreKey, name: target.secretStoreName || target.secretStoreKey, provider: '', status: '' }] : [];
  const stores = [...bound, ...eligible];
  const suggestion = !target.secretStoreKey && eligible.length === 1 ? eligible[0] : undefined;
  const candidate = picked ?? (target.secretStoreKey || suggestion?.key || '');
  const selected = stores.some((item) => item.key === candidate) ? candidate : '';
  const suggested = picked === undefined && Boolean(suggestion) && selected === suggestion?.key;
  const lostPick = picked !== undefined && picked !== '' && selected === '' && choices.status === 'ready';

  // True only when the authoritative GET succeeded and returned this Environment.
  async function reloadLatest(current: number): Promise<boolean> {
    try {
      const latest = await fetchEnvironment(application.id, environment);
      if (!latest) return false;
      if (current === generation.current) { onTargetChange(application.id, environment, latest); onConfigurationChanged(); }
      return true;
    } catch { return false; }
  }

  async function retryReload() {
    const current = ++generation.current;
    if (await reloadLatest(current) && current === generation.current) { setNeedsReload(false); setError(''); setNotice('Đã tải lại lựa chọn mới nhất; xem lại rồi lưu nếu vẫn muốn đổi.'); }
    else if (current === generation.current) setError('Vẫn chưa tải lại được trạng thái môi trường. Lựa chọn của bạn được giữ nguyên; hãy thử lại.');
  }

  async function save() {
    if (!selected || saving || busy || needsReload) return;
    const current = ++generation.current;
    const mine = scope.current;
    setSaving(true); setError(''); setNotice('');
    try {
      const result = await setSecretStore(application.id, environment, selected, target.version, configVersion);
      // The confirmed result belongs to the Environment it was saved for; only the refresh is scope-bound.
      onTargetChange(application.id, environment, toTarget(result.environment));
      if (mine === scope.current) onConfigurationChanged();
      if (current === generation.current) { setPicked(undefined); setNotice(!result.changed ? 'Đã chọn kho này; không sao chép gì thêm.' : `Đã lưu kho bí mật. ${result.copiedSecrets} bí mật đã được sao chép và xác minh. Hãy Xem trước và Triển khai để chuyển các workload đang chạy.`); }
    } catch (err) {
      if (current !== generation.current) return;
      if (isBusy(err)) {
        setError((err as ApiError).message);
      } else if (isStaleConflict(err)) {
        const base = (err as ApiError).message;
        const reloaded = await reloadLatest(current);
        if (current !== generation.current) return;
        setNeedsReload(!reloaded);
        setError(reloaded ? `${base} Đã tải lại lựa chọn mới nhất; xem lại rồi bấm Lưu kho bí mật nếu vẫn muốn đổi.` : `${base} Chưa tải lại được lựa chọn mới nhất; lựa chọn của bạn được giữ nguyên. Bấm “Tải lại trạng thái” rồi xem lại trước khi lưu.`);
      } else if (err instanceof ApiError && err.code === 'SECRET_COPY_FAILED') {
        setError('Không sao chép được các bí mật sang kho mới. Kho cũ vẫn đang được chọn và không có gì thay đổi.');
      } else if (err instanceof ApiError && err.status === 422) {
        setError(err.code === 'SECRET_STORE_UNAVAILABLE' ? 'Kho bí mật đã chọn không còn khả dụng. Hãy chọn kho khác.' : err.message);
        setAttempt((value) => value + 1);
      } else {
        // A network failure does not prove the selection was not saved.
        setError(err instanceof ApiError ? `Không lưu được kho bí mật. ${err.message}` : 'Không lưu được kho bí mật. Chưa rõ kết quả; hãy tải lại môi trường trước khi thử lại.');
      }
    } finally {
      if (current === generation.current) setSaving(false);
    }
  }

  return <section className="content-panel secret-store-selection" aria-label="Kho bí mật">
    <div className="section-header"><div><h2>Kho bí mật</h2><p>{target.secretStoreKey ? `${title} lưu Bí mật trong kho này. Biến vẫn lưu trong Orchestrator.` : `${title} chưa có kho bí mật. Biến dùng được khi chưa có kho; cần chọn kho mới thêm được Bí mật.`}</p></div><span className="pending-pill">{target.secretStoreKey ? (target.secretStoreName || target.secretStoreKey) : 'Chưa chọn'}</span></div>
    {error ? <div className="form-error" role="alert">{error}{needsReload ? <Button type="button" onClick={() => void retryReload()}>Tải lại trạng thái</Button> : null}</div> : null}
    {notice ? <div className="form-success" role="status">{notice}</div> : null}
    {busy ? <div className="form-info" role="status">Một thao tác đang chạy trên môi trường này. Cài đặt chỉ đọc cho đến khi thao tác kết thúc.</div> : null}
    {choices.status === 'loading' ? <p role="status" aria-busy="true">Đang tải danh sách kho bí mật…</p> : null}
    {choices.status === 'error' ? <div className="form-error" role="alert">Không tải được danh sách kho bí mật.<Button type="button" onClick={() => setAttempt((value) => value + 1)}>Thử lại</Button></div> : null}
    {choices.status === 'ready' && stores.length === 0 ? <div className="form-info" role="status">Chưa có kho bí mật Sẵn sàng (READY) để chọn. Nhờ Kỹ sư nền tảng đăng ký kho rồi bấm Thử lại.<Button type="button" onClick={() => setAttempt((value) => value + 1)}>Thử lại</Button></div> : null}
    {stores.length > 0 ? <>
      <label htmlFor="environment-secret-store">Kho bí mật cho {title}</label>
      <select id="environment-secret-store" value={selected} disabled={saving || busy} onChange={(event) => { setPicked(event.target.value); setNotice(''); }}>
        {!target.secretStoreKey || !selected ? <option value="" disabled>Chọn một kho bí mật</option> : null}
        {stores.map((item) => <option key={item.key} value={item.key}>{item.name} ({item.key}){item.status && item.status !== 'READY' ? ` · ${statusLabel(item.status)}` : ''}</option>)}
      </select>
      {suggested ? <div className="form-info" role="status">Gợi ý chưa lưu: <strong>{suggestion?.name}</strong> là kho bí mật Sẵn sàng duy nhất. Đây chỉ là lựa chọn nháp, không phải mặc định của tổ chức; môi trường chỉ dùng kho này sau khi bạn bấm Lưu kho bí mật.</div> : null}
      {lostPick ? <div className="form-info" role="status">Kho bạn chọn trước đó không còn Sẵn sàng. Hãy chọn lại.</div> : null}
      {!target.secretStoreKey && stores.length > 1 && !selected ? <small>Có nhiều kho Sẵn sàng; hãy tự chọn một kho, hệ thống không chọn thay bạn.</small> : null}
      <small>Đổi kho sẽ sao chép các Bí mật hiện tại sang kho mới trước và xác minh từng bản sao. Nếu một bản sao lỗi, kho cũ vẫn được giữ. Workload đang chạy giữ giá trị cũ cho đến lần Triển khai tiếp theo.</small>
      <div className="form-actions"><Button tone="primary" disabled={!selected || saving || busy || needsReload || selected === target.secretStoreKey} onClick={() => void save()}>{saving ? 'Đang sao chép bí mật…' : 'Lưu kho bí mật'}</Button></div>
    </> : null}
  </section>;
}
