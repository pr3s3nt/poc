import { useEffect, useRef, useState } from 'react';
import { navigate, replaceRoute } from '../../app/routes';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { environmentTitle } from '../environment/labels';
import { EnvironmentConnection } from './EnvironmentConnection';
import { useEnvironmentBusy } from '../environment/useEnvironmentBusy';
import { OperationBanner } from '../environment/OperationBanner';
import { SecretStoreSelection } from '../environment/SecretStoreSelection';
import { TransitionPanel } from '../environment/TransitionPanel';
import { deleteKey, getConfiguration, putKey, renameKey, type ConfigKey, type Configuration, type KeyKind } from './api';

type Form = { kind: KeyKind; name: string; value: string; editing?: string };
type Action = { kind: 'rename' | 'delete'; key: ConfigKey; newName: string };

// 409 conflicts are shown with the server sentence; the page then reloads the
// authoritative configuration so the user resubmits against the latest version.
function describe(err: unknown): string {
  const message = (err as Error).message;
  return (err as { status?: number }).status === 409 ? `${message} Đã tải lại giá trị mới nhất; hãy xem lại rồi thử lại.` : message;
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
  const [transitionTo, setTransitionTo] = useState<string>();
  const [needStore, setNeedStore] = useState(false);
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
  const updateForm = (next: Form | undefined) => { editor.current += 1; setForm(next); };
  const updateAction = (next: Action | undefined) => { editor.current += 1; setAction(next); };
  // An active or interrupted Environment operation holds every configuration write
  // (the server answers 409 ENVIRONMENT_BUSY). The state is also polled while idle, so an
  // operation started elsewhere is noticed. Open editors keep their text and
  // simply cannot submit until the target no longer reports an operation.
  const busy = useEnvironmentBusy(application, environment, onTargetChange);

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
    setTransitionTo(undefined);
    setNeedStore(false);
    setLoading(true);
  }, [application.id, environment]);

  // A refresh (initial load, after a store change) only replaces the data: the
  // open editor and its text stay exactly as the user left them.
  useEffect(() => {
    const mine = scope.current;
    const order = ++seq.current;
    getConfiguration(application.id, environment)
      .then((result) => { if (mine === scope.current && order === seq.current) setData(result); })
      .catch(() => { if (mine === scope.current && order === seq.current) setError('Không tải được cấu hình. Hãy thử lại.'); })
      .finally(() => { if (mine === scope.current) setLoading(false); });
  }, [application.id, environment, reloadCount]);

  // Runs one mutation; its reply applies only to the scope it started in.
  async function mutate(run: () => Promise<Configuration>, done: () => void) {
    const mine = scope.current;
    const touched = editor.current;
    setSaving(true); setError('');
    try {
      const result = await run();
      if (mine !== scope.current) return;
      seq.current += 1;
      setData(result);
      if (touched === editor.current) done();
    } catch (err) {
      if (mine !== scope.current) return;
      setError(describe(err));
      await refreshOn409(err, mine);
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
  async function refreshOn409(err: unknown, mine: number) {
    if ((err as { status?: number }).status !== 409) return;
    const order = ++seq.current;
    try {
      const latest = await getConfiguration(application.id, environment);
      if (mine === scope.current && order === seq.current) setData(latest);
    } catch { /* the message already says to retry */ }
  }

  async function confirmAction() {
    if (!action || !data || busy || saving) return;
    const current = action;
    await mutate(() => current.kind === 'rename'
      ? renameKey(application.id, environment, current.key.name, current.newName.trim(), data.version)
      : deleteKey(application.id, environment, current.key.name, data.version), () => updateAction(undefined));
  }

  function section(kind: KeyKind, title: string) {
    const keys = data?.keys.filter((key) => key.kind === kind) ?? [];
    return <section className="content-panel" aria-label={title}>
      <div className="section-header"><div><h2>{title}</h2><p>{kind === 'SECRET' ? 'Giá trị được ẩn sau khi lưu và không bao giờ hiển thị lại.' : 'Dùng chung cho các workload trong môi trường này.'}</p></div><Button disabled={busy} onClick={() => { updateAction(undefined); if (kind === 'SECRET' && !application.environments[environment].secretStoreKey) { updateForm(undefined); setNeedStore(true); return; } setNeedStore(false); updateForm({ kind, name: '', value: '' }); }}>+ Thêm {kind === 'SECRET' ? 'bí mật' : 'biến'}</Button></div>
      {keys.length === 0 ? <div className="section-empty">Chưa có {title.toLowerCase()} nào trong {environment}.</div> : <div className="settings-table"><div className="settings-table-head"><span>Tên key</span><span>{kind === 'SECRET' ? 'Trạng thái' : 'Giá trị'}</span><span>Workload sử dụng</span><span>Thao tác</span></div>
        {keys.map((key) => <div className="settings-table-row" key={key.name}><strong>{key.name}</strong><span className={kind === 'SECRET' ? 'muted' : ''}>{kind === 'SECRET' ? 'Đã cấu hình' : key.value}</span><span>{key.usedBy.length ? key.usedBy.join(', ') : '—'}</span><span className="row-actions"><Button tone="quiet" disabled={busy} onClick={() => { updateAction(undefined); updateForm({ kind, name: key.name, value: kind === 'SECRET' ? '' : key.value ?? '', editing: key.name }); }}>{kind === 'SECRET' ? 'Cập nhật' : 'Sửa'}</Button><Button tone="quiet" disabled={busy} onClick={() => { updateForm(undefined); updateAction({ kind: 'rename', key, newName: key.name }); }}>Đổi tên</Button><Button tone="danger" disabled={busy} onClick={() => { updateForm(undefined); updateAction({ kind: 'delete', key, newName: '' }); }}>Xóa</Button></span></div>)}</div>}
    </section>;
  }

  const change = onTargetChange ?? (() => undefined);
  const title = environmentTitle(environment);
  const hasStore = Boolean(application.environments[environment].secretStoreKey);
  return <section className="page settings-page">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id })}>← {application.name}</button>
    <header className="page-header application-header"><div><p className="eyebrow">Cài đặt ứng dụng</p><h1>Cài đặt môi trường</h1><p>Chọn và đổi kết nối triển khai cùng kho bí mật của từng môi trường, rồi cấu hình giá trị riêng cho staging và production. Dùng Xem trước trên trang Ứng dụng để biết phần nào còn cần triển khai. Lựa chọn gợi ý chỉ là bản nháp cho đến khi bạn bấm Lưu.</p></div>{data && data.version > 0 ? <span className="pending-pill">Phiên bản cấu hình v{data.version}</span> : null}</header>
    <div className="tabs" role="tablist" aria-label="Môi trường">{(['staging', 'production'] as const).map((env) => <button key={env} role="tab" aria-selected={env === environment} className={env === environment ? 'tab tab-active' : 'tab'} onClick={() => { setEnvironment(env); replaceRoute({ name: 'settings', applicationId: application.id, environment: env }); }}>{environmentTitle(env)}</button>)}</div>
    <OperationBanner application={application} environment={environment} onTargetChange={change} />
    <EnvironmentConnection application={application} environment={environment} onTargetChange={change} onStartTransition={(destination) => setTransitionTo(destination)} />
    {transitionTo !== undefined || application.environments[environment].runtimeExists ? <TransitionPanel application={application} environment={environment} initialDestination={transitionTo ?? ''} onTargetChange={change} /> : null}
    <SecretStoreSelection application={application} environment={environment} configVersion={data?.version ?? 0} onTargetChange={change} onConfigurationChanged={() => setReloadCount((value) => value + 1)} />
    {needStore && !hasStore ? <div className="form-info" role="status">Hãy chọn kho bí mật ở trên trước khi thêm Bí mật. Biến môi trường thì thêm được mà không cần kho.</div> : null}
    {error ? <div className="form-error" role="alert">{error}{!data && !loading ? <Button type="button" onClick={() => { setError(''); setLoading(true); setReloadCount((value) => value + 1); }}>Thử lại</Button> : null}</div> : null}
    {loading ? <p>Đang tải cấu hình…</p> : null}
    {!loading && data ? <>{section('VARIABLE', 'Biến')}{section('SECRET', 'Bí mật')}</> : null}
    {form ? <div className="content-panel inline-editor"><div className="section-header"><h2>{form.editing ? (form.kind === 'SECRET' ? 'Thay bí mật' : 'Sửa biến') : (form.kind === 'SECRET' ? 'Thêm bí mật' : 'Thêm biến')}</h2></div><label>Tên key<input value={form.name} disabled={Boolean(form.editing)} onChange={(event) => updateForm({ ...form, name: event.target.value })} placeholder="API_URL" /></label><label>{form.kind === 'SECRET' ? 'Giá trị bí mật mới' : 'Giá trị'}<input type={form.kind === 'SECRET' ? 'password' : 'text'} autoComplete="off" value={form.value} onChange={(event) => updateForm({ ...form, value: event.target.value })} /></label>{form.kind === 'SECRET' ? <p className="form-info">Giá trị chỉ được nhập vào đây, được che khi nhập và không bao giờ được đọc lại.</p> : null}{busy ? <p className="form-info" role="status">Một thao tác trên môi trường đang chạy. Nội dung bạn nhập được giữ nguyên; hãy gửi lại khi thao tác kết thúc.</p> : null}<div className="form-actions"><Button onClick={() => updateForm(undefined)}>Hủy</Button><Button tone="primary" disabled={busy || saving || !form.name.trim() || !form.value} onClick={saveForm}>Lưu thay đổi chờ triển khai</Button></div></div> : null}
    {action ? <div className="content-panel inline-editor" role="dialog" aria-label={`${action.kind === 'rename' ? 'Đổi tên' : 'Xóa'} ${action.key.name}`}><h2>{action.kind === 'rename' ? 'Đổi tên' : 'Xóa'} {action.key.name}?</h2>{action.key.usedBy.length ? <p className="warning-text">Đang được tham chiếu bởi {action.key.usedBy.join(', ')}. Các tham chiếu sẽ không được cập nhật tự động; Xem trước sẽ từ chối key bị thiếu cho đến khi bạn tự sửa các workload đó.</p> : <p>Thay đổi này chỉ áp dụng cho {title}. Các workload đang chạy giữ nguyên cho đến khi Triển khai.</p>}{action.kind === 'rename' ? <label>Tên key mới<input value={action.newName} onChange={(event) => updateAction({ ...action, newName: event.target.value })} /></label> : null}{busy ? <p className="form-info" role="status">Một thao tác trên môi trường đang chạy. Hãy xác nhận lại khi thao tác kết thúc.</p> : null}<div className="form-actions"><Button onClick={() => updateAction(undefined)}>Hủy</Button><Button tone={action.kind === 'delete' ? 'danger' : 'primary'} disabled={busy || saving || (action.kind === 'rename' && !action.newName.trim())} onClick={confirmAction}>Tiếp tục</Button></div></div> : null}
  </section>;
}
