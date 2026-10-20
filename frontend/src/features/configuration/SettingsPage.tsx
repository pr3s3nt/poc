import { useEffect, useRef, useState } from 'react';
import { navigate, replaceRoute } from '../../app/routes';
import { ApiError, isStaleConflict } from '../../shared/api/client';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { environmentTitle } from '../environment/labels';
import { useEnvironmentBusy } from '../environment/useEnvironmentBusy';
import { OperationBanner } from '../environment/OperationBanner';
import { SecretStoreSelection } from '../environment/SecretStoreSelection';
import { deleteKey, getConfiguration, putKey, renameKey, type ConfigKey, type Configuration, type KeyKind } from './api';

type Form = { kind: KeyKind; name: string; value: string; editing?: string };
type Action = { kind: 'rename' | 'delete'; key: ConfigKey; newName: string; updateReferences: boolean };
type Notice = { text: string; workloads: string[] };

const unknownOutcome = 'Chưa rõ thay đổi đã được lưu hay chưa. Hãy tải lại cấu hình và kiểm tra trước khi thử lại; Console không tự gửi lại.';

// Text comes from the stable error code. A network failure never claims the
// change was not saved; reload outcome wording is added only after the GET result is known.
// The guards are only taken from one authoritative GET; a missing version is never replaced by zero.
function referenceGuards(data: Configuration) {
  return Number.isInteger(data.environmentVersion) && Number.isInteger(data.draftVersion)
    ? { environmentVersion: data.environmentVersion as number, draftVersion: data.draftVersion as number } : undefined;
}

function describe(err: unknown): string {
  if (!(err instanceof ApiError)) return 'Chưa nhận được phản hồi từ máy chủ nên chưa rõ thay đổi đã được lưu hay chưa. Hãy tải lại cấu hình trước khi thử lại; Console không tự gửi lại.';
  return err.message;
}

export function SettingsPage({ application, initialEnvironment = 'staging', onTargetChange }: { application: Application; initialEnvironment?: EnvironmentKey; onTargetChange?(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void }) {
  const [environment, setEnvironment] = useState<EnvironmentKey>(initialEnvironment);
  const [data, setData] = useState<Configuration>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [form, setForm] = useState<Form>();
  const [action, setAction] = useState<Action>();
  const [saving, setSaving] = useState(false);
  const [reloadCount, setReloadCount] = useState(0);
  const [needStore, setNeedStore] = useState(false);
  // Set when a stale conflict could not reload the authoritative configuration: writes stay off until a reload succeeds.
  const [reloadNeeded, setReloadNeeded] = useState(false);
  const [notice, setNotice] = useState<Notice>();
  // scope changes with the Application/Environment; a mutation or reload reply
  // that belongs to an older scope is dropped instead of showing another
  // Environment's data. seq orders replies inside one scope: only the newest
  // data wins, so a late reload cannot undo a newer save.
  const scope = useRef(0);
  const seq = useRef(0);
  // editor changes whenever the user opens, edits or closes a form or confirmation.
  // A mutation that started before such a change must not close what the user is
  // doing now: its reply still updates the data, but only an untouched editor is closed.
  const editor = useRef(0);
  const updateForm = (next: Form | undefined) => { editor.current += 1; setNotice(undefined); setForm(next); };
  const updateAction = (next: Action | undefined) => { editor.current += 1; setNotice(undefined); setAction(next); };
  // An active or interrupted Environment operation holds every configuration write
  // (the server answers 409 ENVIRONMENT_BUSY). The state is also polled while idle, so an
  // operation started elsewhere is noticed. Open editors keep their text and
  // simply cannot submit until the target no longer reports an operation.
  const operationBusy = useEnvironmentBusy(application, environment, onTargetChange);
  const busy = operationBusy || reloadNeeded;

  // Leaving the page invalidates every pending reply.
  useEffect(() => () => { scope.current += 1; seq.current += 1; }, []);

  // Switching Application or Environment resets the editors and loads afresh.
  useEffect(() => {
    scope.current += 1;
    setData(undefined);
    updateForm(undefined);
    updateAction(undefined);
    setSaving(false);
    setError('');
    setNeedStore(false);
    setReloadNeeded(false);
    setNotice(undefined);
    setLoading(true);
  }, [application.id, environment]);

  // A refresh (initial load, after a store change) only replaces the data: the
  // open editor and its text stay exactly as the user left them.
  useEffect(() => {
    const mine = scope.current;
    const order = ++seq.current;
    getConfiguration(application.id, environment)
      .then((result) => { if (mine === scope.current && order === seq.current) { setData(result); setReloadNeeded(false); } })
      .catch(() => { if (mine === scope.current && order === seq.current) setError('Không tải được cấu hình. Hãy thử lại.'); })
      .finally(() => { if (mine === scope.current) setLoading(false); });
  }, [application.id, environment, reloadCount]);

  // Runs one mutation; its reply applies only to the scope it started in.
  async function mutate(run: () => Promise<Configuration>, done: (result: Configuration) => void, guardOutcome = false) {
    const mine = scope.current;
    const touched = editor.current;
    setSaving(true); setError('');
    try {
      const result = await run();
      if (mine !== scope.current) return;
      seq.current += 1;
      setData(result);
      if (touched === editor.current) done(result);
    } catch (err) {
      if (mine !== scope.current) return;
      const base = describe(err);
      setError(base);
      // A reference rename that may have committed must be reloaded before another attempt; never replayed.
      if (guardOutcome && (!(err instanceof ApiError) || err.status >= 500)) { setReloadNeeded(true); setError(`${base} ${unknownOutcome}`); return; }
      const refreshed = await refreshOn409(err, mine);
      if (refreshed === 'ok' && mine === scope.current) setError(`${base} Đã tải lại giá trị mới nhất; hãy xem lại rồi thử lại.`);
      if (refreshed === 'failed' && mine === scope.current) { setReloadNeeded(true); setError(`${base} Chưa tải lại được cấu hình mới nhất; bản chỉnh sửa của bạn được giữ nguyên. Bấm “Tải lại cấu hình” rồi xem lại trước khi lưu.`); }
    } finally {
      if (mine === scope.current) setSaving(false);
    }
  }

  async function saveForm() {
    if (!form || !data || busy || saving) return;
    const current = form;
    await mutate(() => putKey(application.id, environment, current.name.trim(), current.kind, current.value, data.version), () => updateForm(undefined));
  }

  // After a conflict the authoritative configuration replaces the stale copy
  // without clearing the message or the editor; the user reviews it and
  // resubmits explicitly.
  // Reports whether the authoritative GET succeeded: 'skipped' when the error is not a stale conflict.
  async function refreshOn409(err: unknown, mine: number): Promise<'ok' | 'failed' | 'skipped'> {
    if (!isStaleConflict(err)) return 'skipped';
    const order = ++seq.current;
    try {
      const latest = await getConfiguration(application.id, environment);
      if (mine === scope.current && order === seq.current) { setData(latest); setReloadNeeded(false); }
      return 'ok';
    } catch { return 'failed'; }
  }

  async function retryReload() {
    const mine = scope.current;
    const order = ++seq.current;
    try {
      const latest = await getConfiguration(application.id, environment);
      if (mine === scope.current && order === seq.current) { setData(latest); setReloadNeeded(false); setError(''); }
    } catch { if (mine === scope.current) setError('Vẫn chưa tải lại được cấu hình. Bản chỉnh sửa được giữ nguyên; hãy thử lại.'); }
  }

  async function confirmAction() {
    if (!action || !data || busy || saving) return;
    const current = action;
    const references = current.kind === 'rename' && current.updateReferences ? referenceGuards(data) : undefined;
    if (current.kind === 'rename' && current.updateReferences && !references) return;
    setNotice(undefined);
    await mutate(() => current.kind === 'rename'
      ? renameKey(application.id, environment, current.key.name, current.newName.trim(), data.version, references)
      : deleteKey(application.id, environment, current.key.name, data.version), (result) => {
      updateAction(undefined);
      // The affected workloads come from the authoritative reply for the new key, not from metadata captured when the dialog opened.
      const renamed = current.newName.trim();
      if (references) setNotice({ text: `Đã đổi tên ${current.key.name} thành ${renamed} và cập nhật tham chiếu trong thay đổi chờ triển khai.`, workloads: result.keys.find((key) => key.name === renamed)?.usedBy ?? [] });
    }, Boolean(references));
  }

  function section(kind: KeyKind, title: string) {
    const keys = data?.keys.filter((key) => key.kind === kind) ?? [];
    return <section className="content-panel" aria-label={title}>
      <div className="section-header"><div><h2>{title}</h2><p>{kind === 'SECRET' ? 'Giá trị được ẩn sau khi lưu và không bao giờ hiển thị lại.' : 'Dùng chung cho các workload trong môi trường này.'}</p></div><Button disabled={busy} onClick={() => { updateAction(undefined); if (kind === 'SECRET' && !application.environments[environment].secretStoreKey) { updateForm(undefined); setNeedStore(true); return; } setNeedStore(false); updateForm({ kind, name: '', value: '' }); }}>+ Thêm {kind === 'SECRET' ? 'bí mật' : 'biến'}</Button></div>
      {keys.length === 0 ? <div className="section-empty">Chưa có {title.toLowerCase()} nào trong {environment}.</div> : <div className="settings-table"><div className="settings-table-head"><span>Tên key</span><span>{kind === 'SECRET' ? 'Trạng thái' : 'Giá trị'}</span><span>Workload sử dụng</span><span>Thao tác</span></div>
        {keys.map((key) => <div className="settings-table-row" key={key.name}><strong>{key.name}</strong><span className={kind === 'SECRET' ? 'muted' : ''}>{kind === 'SECRET' ? 'Đã cấu hình' : key.value}</span><span>{key.usedBy.length ? key.usedBy.join(', ') : '—'}</span><span className="row-actions"><Button tone="quiet" disabled={busy} onClick={() => { updateAction(undefined); updateForm({ kind, name: key.name, value: kind === 'SECRET' ? '' : key.value ?? '', editing: key.name }); }}>{kind === 'SECRET' ? 'Cập nhật' : 'Sửa'}</Button><Button tone="quiet" disabled={busy} onClick={() => { updateForm(undefined); updateAction({ kind: 'rename', key, newName: key.name, updateReferences: false }); }}>Đổi tên</Button><Button tone="danger" disabled={busy} onClick={() => { updateForm(undefined); updateAction({ kind: 'delete', key, newName: '', updateReferences: false }); }}>Xóa</Button></span></div>)}</div>}
    </section>;
  }

  // The dialog names the consumers of the key in the latest authoritative data (it changes after a stale reload).
  const consumers = action ? (data?.keys.find((key) => key.name === action.key.name && key.kind === action.key.kind)?.usedBy ?? action.key.usedBy) : [];
  const change = onTargetChange ?? (() => undefined);
  const title = environmentTitle(environment);
  const hasStore = Boolean(application.environments[environment].secretStoreKey);
  return <section className="page settings-page">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id })}>← {application.name}</button>
    <header className="page-header application-header"><div><p className="eyebrow">Cài đặt ứng dụng</p><h1>Cài đặt môi trường</h1><p>Chọn kho bí mật của từng môi trường, rồi cấu hình giá trị riêng cho staging và production. Dùng Xem trước trên trang Ứng dụng để biết phần nào còn cần triển khai. Lựa chọn gợi ý chỉ là bản nháp cho đến khi bạn bấm Lưu.</p></div>{data && data.version > 0 ? <span className="pending-pill">Phiên bản cấu hình v{data.version}</span> : null}</header>
    <div className="tabs" role="tablist" aria-label="Môi trường">{(['staging', 'production'] as const).map((env) => <button key={env} role="tab" aria-selected={env === environment} className={env === environment ? 'tab tab-active' : 'tab'} onClick={() => { setEnvironment(env); replaceRoute({ name: 'settings', applicationId: application.id, environment: env }); }}>{environmentTitle(env)}</button>)}</div>
    <OperationBanner application={application} environment={environment} onTargetChange={change} />
    <SecretStoreSelection application={application} environment={environment} configVersion={data?.version ?? 0} onTargetChange={change} onConfigurationChanged={() => setReloadCount((value) => value + 1)} />
    {needStore && !hasStore ? <div className="form-info" role="status">Hãy chọn kho bí mật ở trên trước khi thêm Bí mật. Biến môi trường thì thêm được mà không cần kho.</div> : null}
    {error ? <div className="form-error" role="alert">{error}{reloadNeeded ? <Button type="button" onClick={() => void retryReload()}>Tải lại cấu hình</Button> : null}{!data && !loading ? <Button type="button" onClick={() => { setError(''); setLoading(true); setReloadCount((value) => value + 1); }}>Thử lại</Button> : null}</div> : null}
    {notice ? <div className="form-info" role="status">{notice.text}{notice.workloads.length ? ` Workload bị ảnh hưởng: ${notice.workloads.join(', ')}.` : ''} Chưa có gì được triển khai: dùng Xem trước thay đổi trên trang ứng dụng rồi Triển khai tường minh. <Button type="button" onClick={() => navigate({ name: 'application', applicationId: application.id, environment })}>Đến Xem trước thay đổi</Button></div> : null}
    {loading ? <p>Đang tải cấu hình…</p> : null}
    {!loading && data ? <>{section('VARIABLE', 'Biến')}{section('SECRET', 'Bí mật')}</> : null}
    {form ? <div className="content-panel inline-editor"><div className="section-header"><h2>{form.editing ? (form.kind === 'SECRET' ? 'Thay bí mật' : 'Sửa biến') : (form.kind === 'SECRET' ? 'Thêm bí mật' : 'Thêm biến')}</h2></div><label>Tên key<input value={form.name} disabled={Boolean(form.editing)} onChange={(event) => updateForm({ ...form, name: event.target.value })} placeholder="API_URL" /></label><label>{form.kind === 'SECRET' ? 'Giá trị bí mật mới' : 'Giá trị'}<input type={form.kind === 'SECRET' ? 'password' : 'text'} autoComplete="off" value={form.value} onChange={(event) => updateForm({ ...form, value: event.target.value })} /></label>{form.kind === 'SECRET' ? <p className="form-info">Giá trị chỉ được nhập vào đây, được che khi nhập và không bao giờ được đọc lại.</p> : null}{operationBusy ? <p className="form-info" role="status">Một thao tác trên môi trường đang chạy. Nội dung bạn nhập được giữ nguyên; hãy gửi lại khi thao tác kết thúc.</p> : null}<div className="form-actions"><Button onClick={() => updateForm(undefined)}>Hủy</Button><Button tone="primary" disabled={busy || saving || !form.name.trim() || !form.value} onClick={saveForm}>Lưu thay đổi chờ triển khai</Button></div></div> : null}
    {action ? <div className="content-panel inline-editor" role="dialog" aria-label={`${action.kind === 'rename' ? 'Đổi tên' : 'Xóa'} ${action.key.name}`}><h2>{action.kind === 'rename' ? 'Đổi tên' : 'Xóa'} {action.key.name}?</h2>{consumers.length ? <p className="warning-text">Đang được tham chiếu bởi {consumers.join(', ')}. {action.updateReferences ? 'Tham chiếu của các workload này trong thay đổi chờ triển khai sẽ được cập nhật cùng lúc; môi trường đang chạy giữ nguyên cho tới khi Xem trước rồi Triển khai.' : 'Các tham chiếu sẽ không được cập nhật tự động; Xem trước sẽ từ chối key bị thiếu cho đến khi bạn tự sửa các workload đó.'}</p> : <p>Thay đổi này chỉ áp dụng cho {title}. Các workload đang chạy giữ nguyên cho đến khi Triển khai.</p>}{action.kind === 'rename' ? <label>Tên key mới<input value={action.newName} onChange={(event) => updateAction({ ...action, newName: event.target.value })} /></label> : null}{action.kind === 'rename' ? <label className="checkbox-row"><input type="checkbox" checked={action.updateReferences} onChange={(event) => updateAction({ ...action, updateReferences: event.target.checked })} /> Đổi tên và cập nhật tham chiếu</label> : null}{action.kind === 'rename' && action.updateReferences && !referenceGuards(data!) ? <p className="form-info" role="status">Chưa có đủ phiên bản môi trường và bản nháp từ máy chủ. Hãy tải lại cấu hình trước khi cập nhật tham chiếu.</p> : null}{operationBusy ? <p className="form-info" role="status">Một thao tác trên môi trường đang chạy. Hãy xác nhận lại khi thao tác kết thúc.</p> : null}<div className="form-actions"><Button onClick={() => updateAction(undefined)}>Hủy</Button><Button tone={action.kind === 'delete' ? 'danger' : 'primary'} disabled={busy || saving || (action.kind === 'rename' && (!action.newName.trim() || (action.updateReferences && !referenceGuards(data!))))} onClick={confirmAction}>Tiếp tục</Button></div></div> : null}
  </section>;
}
