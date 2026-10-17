import { useEffect, useRef, useState, type FormEvent } from 'react';
import { isStaleConflict } from '../../../shared/api/client';
import { Button } from '../../../shared/ui/Button';
import { AwsKeyFields, emptyAwsFields, type AwsFields } from './AwsKeyFields';
import { KubeconfigField, type KubeconfigDraft } from './KubeconfigField';
import {
  accountGuidance, authLabel, getAccount, maxNameLength, outcomeUnknown, parseRegions, renameAccount, replaceCredentials, statusLabel, typeLabel,
  validateAwsKeys, validateName, validateRegions, verifyAccount, type Account, type Operation,
} from './accountsApi';

const emptyKube: KubeconfigDraft = { kubeconfig: '', context: '' };
type Busy = null | 'rename' | 'verify' | 'replace';

function when(value?: string): string {
  const date = value ? new Date(value) : undefined;
  return date && !Number.isNaN(date.getTime()) ? date.toLocaleString('vi-VN') : '';
}

/**
 * One account: safe metadata, explicit rename, explicit read-only connection
 * check and a separate concealed same-target credential replacement. Rendered
 * with key={account.key}, so switching account discards every draft and secret,
 * and unmounting makes late responses no-ops. Mutations share one busy flag
 * (no duplicates), carry the loaded version and are never replayed: after a
 * stale or unknown outcome the user reloads (GET) and saves again explicitly.
 */
export function AccountDetail({ account, onChanged, onRefreshed }: { account: Account; onChanged(account: Account): void; onRefreshed(account: Account): void }) {
  const [busy, setBusy] = useState<Busy>(null);
  const [nameDraft, setNameDraft] = useState(account.name);
  const [error, setError] = useState('');
  const [stale, setStale] = useState(false);
  const [unknown, setUnknown] = useState(false);
  // After an unknown outcome the loaded result must be reviewed explicitly.
  const [review, setReview] = useState(false);
  const [reloading, setReloading] = useState(false);
  const [notice, setNotice] = useState('');
  const [verifyFailure, setVerifyFailure] = useState('');
  const [verifyRegions, setVerifyRegions] = useState((account.config?.verificationRegions ?? []).join(', '));
  const [kube, setKube] = useState<KubeconfigDraft>(emptyKube);
  const [aws, setAws] = useState<AwsFields>({ ...emptyAwsFields, regions: (account.config?.verificationRegions ?? []).join(', ') });
  const [resetSignal, setResetSignal] = useState(0);
  const [replaceError, setReplaceError] = useState('');
  const [replaceNotice, setReplaceNotice] = useState('');
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);

  const isAws = account.type === 'AWS';
  const blocked = busy !== null || unknown || reloading || review;

  async function reload() {
    if (reloading) return;
    setReloading(true);
    try {
      const fresh = await getAccount(account.key);
      if (!alive.current) return;
      // Metadata only: the user's drafts are never replaced by a loaded copy.
      onRefreshed(fresh);
      if (unknown) setReview(true);
      setStale(false); setUnknown(false); setError('');
      setNotice(`Đã tải lại tài khoản: phiên bản hiện tại ${fresh.version}${fresh.name !== nameDraft ? `, tên đang lưu là “${fresh.name}”` : ''}. Bản nháp của bạn được giữ nguyên; lưu lại tường minh nếu vẫn muốn.`);
    } catch {
      if (alive.current) setError('Chưa tải lại được tài khoản. Nhấn “Tải lại bản mới nhất” để thử lại.');
    } finally {
      if (alive.current) setReloading(false);
    }
  }

  function failure(operation: Operation, reason: unknown) {
    const text = accountGuidance(operation, reason);
    if (isStaleConflict(reason)) setStale(true);
    if (outcomeUnknown(reason)) { setUnknown(true); void reloadAfterUnknown(); }
    return text;
  }
  async function reloadAfterUnknown() {
    // GET only: finding out what happened never repeats the mutation.
    setReloading(true);
    try {
      const fresh = await getAccount(account.key);
      if (!alive.current) return;
      onRefreshed(fresh); setUnknown(false); setReview(true);
      setNotice(`Đã tải lại tài khoản để kiểm tra kết quả thực tế: phiên bản hiện tại ${fresh.version}, tên “${fresh.name}”.`);
    } catch { /* stays blocked until an explicit reload succeeds */ } finally {
      if (alive.current) setReloading(false);
    }
  }

  async function rename(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (blocked) return;
    const problem = validateName(nameDraft);
    setNotice(''); setVerifyFailure('');
    if (problem) { setError(problem); return; }
    setError(''); setBusy('rename');
    try {
      const updated = await renameAccount(account.key, { name: nameDraft.trim(), version: account.version });
      if (!alive.current) return;
      setNameDraft(updated.name); setStale(false);
      setNotice(`Đã đổi tên thành “${updated.name}”; phiên bản ${updated.version}. ID, đích kết nối, credential và kết quả xác minh được giữ nguyên.`);
      onChanged(updated);
    } catch (reason) {
      if (alive.current) setError(failure('rename', reason));
    } finally {
      if (alive.current) setBusy(null);
    }
  }

  async function verify() {
    if (blocked) return;
    let regions: string[] | undefined;
    if (isAws) {
      const problem = validateRegions(verifyRegions);
      if (problem) { setError(problem); return; }
      regions = parseRegions(verifyRegions);
    }
    setError(''); setNotice(''); setVerifyFailure(''); setBusy('verify');
    try {
      const updated = await verifyAccount(account.key, { version: account.version, ...(regions ? { verificationRegions: regions } : {}) });
      if (!alive.current) return;
      setStale(false);
      setNotice(`Kiểm tra kết nối thành công lúc ${when(updated.verification?.verifiedAt) || 'vừa xong'}; phiên bản ${updated.version}. Kết quả chỉ phản ánh lần kiểm tra này.`);
      onChanged(updated);
    } catch (reason) {
      if (!alive.current) return;
      const text = failure('verify', reason);
      // A fresh check failure is shown apart from the last successful check.
      if (isStaleConflict(reason) || outcomeUnknown(reason)) setError(text); else setVerifyFailure(text);
    } finally {
      if (alive.current) setBusy(null);
    }
  }

  function cancelReplace() {
    setAws({ ...emptyAwsFields, regions: aws.regions }); setKube(emptyKube); setResetSignal((n) => n + 1); setReplaceError('');
  }

  async function replace(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (blocked) return;
    setReplaceNotice(''); setNotice(''); setVerifyFailure('');
    const problem = isAws
      ? validateAwsKeys(aws.accessKeyId, aws.secretAccessKey) || validateRegions(aws.regions)
      : (!kube.context ? 'Tải lên hoặc dán kubeconfig rồi chọn context trước khi thay credential.' : '');
    if (problem) { setReplaceError(problem); return; }
    setReplaceError(''); setBusy('replace');
    let body: Record<string, unknown>;
    if (isAws) {
      body = { version: account.version, accessKeyId: aws.accessKeyId, secretAccessKey: aws.secretAccessKey, verificationRegions: parseRegions(aws.regions) };
      // Keys leave form state before the request resolves.
      setAws((current) => ({ ...current, accessKeyId: '', secretAccessKey: '' }));
    } else {
      body = { version: account.version, kubeconfig: kube.kubeconfig, context: kube.context };
    }
    try {
      const updated = await replaceCredentials(account.key, body);
      if (!alive.current) return;
      setStale(false); cancelReplace();
      setReplaceNotice(`Đã thay credential sau khi xác minh cùng đích; phiên bản ${updated.version}. Credential cũ vẫn được giữ trong kho vì mục tiêu đã triển khai có thể còn tham chiếu.`);
      onChanged(updated);
    } catch (reason) {
      if (alive.current) setReplaceError(failure('replace', reason));
    } finally {
      if (alive.current) setBusy(null);
    }
  }

  const config = account.config ?? {};
  const verification = account.verification ?? {};
  const checkedAt = when(verification.verifiedAt);
  return <section className="content-panel account-detail" aria-label={`Chi tiết tài khoản ${account.name}`} aria-busy={busy !== null}>
    <div className="section-header"><div><h2>{account.name}</h2><p className="muted"><code>{account.key}</code> · phiên bản {account.version}</p></div></div>
    <dl className="destination-summary" aria-label="Thông tin tài khoản">
      <div><dt>Loại hạ tầng</dt> <dd>{typeLabel(account.type)}</dd></div>
      <div><dt>Phương thức xác thực</dt> <dd>{authLabel(account.authenticationType)}</dd></div>
      <div><dt>Trạng thái</dt> <dd>{statusLabel(account.status)}</dd></div>
      {isAws ? <>
        <div><dt>Tài khoản AWS</dt> <dd>{config.accountId ?? '—'}</dd></div>
        <div><dt>Principal</dt> <dd>{config.principalArn ?? '—'}</dd></div>
        <div><dt>Vùng kiểm tra</dt> <dd>{(config.verificationRegions ?? []).join(', ') || '—'}</dd></div>
      </> : <>
        <div><dt>Cluster</dt> <dd>{config.cluster ?? '—'}</dd></div>
        <div><dt>Context</dt> <dd>{config.kubeContext ?? '—'}</dd></div>
        <div><dt>Endpoint</dt> <dd>{config.endpoint ?? '—'}</dd></div>
        <div><dt>TLS</dt> <dd>{config.tls?.mode === 'CUSTOM_CA' ? 'CA tùy chỉnh' : config.tls?.mode === 'INSECURE_SKIP_VERIFY' ? 'Bỏ qua xác minh TLS' : config.tls?.mode === 'SYSTEM_ROOTS' ? 'CA hệ thống' : '—'}{config.tls?.serverName ? ` (${config.tls.serverName})` : ''}</dd></div>
      </>}
      <div><dt>Lần kiểm tra thành công gần nhất</dt> <dd>{checkedAt || 'Chưa có'}</dd></div>
    </dl>
    <p className="muted">“Sẵn sàng” chỉ ghi nhận lần kiểm tra thành công gần nhất, không phải tình trạng kết nối hiện tại. Nhấn “Kiểm tra kết nối” để kiểm tra lại. Vùng triển khai sẽ được chọn ở Driver Inputs, không thuộc tài khoản.</p>

    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {review ? <div className="form-error" role="status">Đã tải lại để kiểm tra kết quả của thao tác chưa rõ. Xem phiên bản và dữ liệu hiện tại ở trên rồi mới tiếp tục. <Button type="button" disabled={reloading} onClick={() => setReview(false)}>Đã xem kết quả, cho phép thao tác tiếp</Button></div> : null}
    {stale || unknown ? <div className="form-error"><Button type="button" disabled={reloading} onClick={() => void reload()}>Tải lại bản mới nhất</Button></div> : null}
    {notice ? <div className="form-success" role="status">{notice}</div> : null}

    <form onSubmit={rename} autoComplete="off"><fieldset className="form-fieldset connection-form" disabled={blocked}>
      <label className="connection-field">Tên tài khoản<input name="rename" value={nameDraft} required maxLength={maxNameLength} onChange={(event) => setNameDraft(event.target.value)} /></label>
      <div className="connection-footer"><Button type="submit" disabled={blocked}>{busy === 'rename' ? 'Đang lưu…' : 'Lưu tên'}</Button></div>
    </fieldset></form>

    <fieldset className="form-fieldset connection-form" disabled={blocked}>
      <legend>Kiểm tra kết nối</legend>
      {isAws ? <label className="connection-field">Vùng kiểm tra<input name="verify-regions" value={verifyRegions} autoComplete="off" spellCheck={false} onChange={(event) => setVerifyRegions(event.target.value)} /></label> : null}
      <div className="connection-footer"><Button type="button" disabled={blocked} onClick={() => void verify()}>{busy === 'verify' ? 'Đang kiểm tra…' : 'Kiểm tra kết nối'}</Button></div>
      {verifyFailure ? <div className="form-error" role="alert" aria-label="Lần kiểm tra vừa thất bại"><strong>Lần kiểm tra vừa thất bại.</strong> {verifyFailure} Trạng thái “{statusLabel(account.status)}” ở trên vẫn là lần kiểm tra thành công gần nhất{checkedAt ? ` (${checkedAt})` : ''}, không phải kết quả lần này.</div> : null}
    </fieldset>

    <form onSubmit={replace} autoComplete="off"><fieldset className="form-fieldset connection-form" disabled={blocked} aria-label="Thay credential">
      <legend>Thay credential</legend>
      <p className="muted">{isAws
        ? 'Nhập access key mới của cùng tài khoản AWS. Key mới được xác minh trước khi lưu; khác tài khoản AWS sẽ bị từ chối — hãy đăng ký tài khoản mới.'
        : 'Cung cấp kubeconfig mới của cùng cluster, context, endpoint và cấu hình TLS. Được xác minh trước khi lưu; khác đích sẽ bị từ chối — hãy đăng ký tài khoản mới.'} Credential hiện tại không bao giờ được hiển thị hay điền sẵn.</p>
      {isAws
        ? <AwsKeyFields prefix="replace" fields={aws} disabled={blocked} onChange={(patch) => setAws((current) => ({ ...current, ...patch }))} />
        : <KubeconfigField disabled={blocked} resetSignal={resetSignal} onChange={setKube} />}
      {replaceError ? <div className="form-error" role="alert">{replaceError}</div> : null}
      {replaceNotice ? <div className="form-success" role="status">{replaceNotice}</div> : null}
      <div className="connection-footer">
        <Button type="submit" tone="primary" disabled={blocked}>{busy === 'replace' ? 'Đang xác minh và thay…' : 'Xác minh và thay credential'}</Button>
        <Button type="button" disabled={busy !== null} onClick={cancelReplace}>Hủy</Button>
      </div>
    </fieldset></form>
  </section>;
}
