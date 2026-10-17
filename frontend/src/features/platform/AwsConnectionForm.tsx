import { useId, useState, type FormEvent } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';

// Public, secret-free result of a registration (UC-04 VAR-02). Never holds a key.
export type AwsRegistered = { key: string; name?: string; config?: { accountId?: string; principalArn?: string; region?: string; allowedRegions?: string[] } };

// Same bounds as the backend (application/connection/aws.go); the server stays the final guard.
const maxNameLength = 100;
const maxRegions = 10;
const accessKeyShape = /^AKIA[A-Z0-9]{16}$/;
const secretKeyShape = /^[A-Za-z0-9/+]{40}$/;
const regionShape = /^[a-z]{2}(-[a-z]+)+-[0-9]$/;

/** Splits the region field on commas and white space, keeping the typed order (the first is the default). */
export function parseRegions(text: string): string[] {
  return text.split(/[\s,;]+/).filter(Boolean);
}

type Fields = { name: string; accessKeyId: string; secretAccessKey: string; regions: string };
const emptyFields: Fields = { name: '', accessKeyId: '', secretAccessKey: '', regions: '' };

// Messages never include a submitted value.
function validate(fields: Fields): string {
  const name = fields.name.trim();
  if (!name) return 'Nhập tên kết nối.';
  if ([...name].length > maxNameLength) return `Tên kết nối tối đa ${maxNameLength} ký tự.`;
  if (!accessKeyShape.test(fields.accessKeyId)) return 'Access key ID không đúng dạng: 20 ký tự bắt đầu bằng AKIA (khóa dài hạn của người dùng IAM).';
  if (!secretKeyShape.test(fields.secretAccessKey)) return 'Secret access key không đúng dạng: đúng 40 ký tự gồm chữ, số, dấu / và +.';
  const regions = parseRegions(fields.regions);
  if (regions.length === 0) return 'Nhập ít nhất một vùng được phép, ví dụ us-east-1.';
  if (regions.length > maxRegions) return `Chỉ cho phép tối đa ${maxRegions} vùng.`;
  if (regions.some((region) => !regionShape.test(region))) return 'Tên vùng không hợp lệ. Dùng dạng như us-east-1 hoặc eu-west-1, cách nhau bằng dấu phẩy.';
  if (new Set(regions).size !== regions.length) return 'Mỗi vùng chỉ được nhập một lần.';
  return '';
}

// Fixed Vietnamese guidance by status and stable code. Raw backend or AWS text never reaches the page.
function registerGuidance(reason: unknown): string {
  if (!(reason instanceof ApiError) || reason.code === 'COMMIT_UNCERTAIN') return 'Chưa nhận được phản hồi từ máy chủ nên chưa biết kết nối đã được lưu hay chưa. Hãy tải lại danh sách để kiểm tra trước khi đăng ký lại; Console không tự gửi lại. Hãy nhập lại access key khi thử lại.';
  switch (reason.status) {
    case 400: return 'Máy chủ từ chối dữ liệu đăng ký. Kiểm tra tên, access key và danh sách vùng rồi nhập lại khóa.';
    case 401: return 'Phiên đăng nhập đã hết hạn. Đăng nhập lại rồi thử lại.';
    case 403: return 'Tài khoản không có quyền đăng ký kết nối. Cần vai trò Kỹ sư nền tảng hoặc Quản trị.';
    case 413: return 'Dữ liệu đăng ký quá lớn. Kiểm tra lại các trường rồi thử lại.';
    case 422:
      switch (reason.code) {
        case 'VERIFICATION_CREDENTIAL_REJECTED': return 'AWS từ chối access key này nên chưa lưu kết nối. Kiểm tra access key ID và secret access key còn hiệu lực rồi nhập lại.';
        case 'VERIFICATION_PERMISSION_DENIED': return 'Access key thiếu quyền cần để tạo VPC, EKS, ElastiCache hoặc Aurora (hoặc quyền đọc ở một vùng) nên chưa lưu kết nối. Bổ sung quyền cho người dùng IAM rồi nhập lại khóa.';
        case 'VERIFICATION_UNREACHABLE': return 'Không kết nối được AWS để xác minh nên chưa lưu kết nối. Kiểm tra mạng của máy chủ rồi thử lại.';
        case 'VERIFICATION_REGION_UNAVAILABLE':
        case 'VERIFICATION_CAPABILITY_UNVERIFIABLE':
        case 'VERIFICATION_PRINCIPAL_UNSUPPORTED': return `${reason.message} Chưa lưu kết nối; nhập lại khóa sau khi xử lý.`;
        default: return 'Không xác minh được tài khoản AWS nên chưa lưu kết nối. Kiểm tra access key, vùng và quyền rồi nhập lại khóa.';
      }
    case 503: return 'Kho lưu thông tin xác thực của nền tảng chưa sẵn sàng. Liên hệ quản trị nền tảng rồi thử lại.';
    default: return reason.status >= 500 ? 'Máy chủ gặp lỗi khi đăng ký. Kết nối có thể chưa được lưu; tải lại danh sách để kiểm tra.' : 'Không đăng ký được kết nối. Kiểm tra dữ liệu rồi thử lại.';
  }
}

/**
 * AWS access-key onboarding. The key pair lives only in two concealed controlled
 * inputs until the request is sent; both are cleared after every attempt that
 * reached the server and after success. Nothing is stored or logged client-side.
 */
export function AwsConnectionForm({ onRegistered }: { onRegistered: () => Promise<void> | void }) {
  const [fields, setFields] = useState<Fields>(emptyFields);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [result, setResult] = useState<AwsRegistered | null>(null);
  const id = useId();
  const set = (patch: Partial<Fields>) => setFields((current) => ({ ...current, ...patch }));

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    const problem = validate(fields);
    setNotice(''); setResult(null);
    if (problem) { setError(problem); return; }
    setError(''); setSaving(true);
    const { name, accessKeyId, secretAccessKey, regions } = fields;
    // The keys leave the form state before the request resolves; only the payload holds them.
    set({ accessKeyId: '', secretAccessKey: '' });
    let created: AwsRegistered;
    try {
      created = await api<AwsRegistered>('/connections/aws', { method: 'POST', body: JSON.stringify({ name: name.trim(), accessKeyId, secretAccessKey, allowedRegions: parseRegions(regions) }) });
    } catch (reason) {
      setError(registerGuidance(reason)); setSaving(false);
      // An unknown outcome is checked by reloading the list, never by resubmitting.
      if (!(reason instanceof ApiError) || reason.code === 'COMMIT_UNCERTAIN') void onRegistered();
      return;
    }
    setFields(emptyFields); setSaving(false);
    setResult(created);
    setNotice(`Đã đăng ký tài khoản AWS ${created.name ?? name.trim()} (${created.key}). Kết nối chưa được chọn cho môi trường nào và chưa có tài nguyên nào được tạo.`);
    await onRegistered();
  }

  const regionCount = parseRegions(fields.regions).length;
  return <form onSubmit={submit} autoComplete="off"><fieldset className="form-fieldset connection-form" disabled={saving} aria-busy={saving}>
    <label className="connection-field">Tên kết nối<input name="aws-connection-name" value={fields.name} required maxLength={maxNameLength} autoComplete="off" onChange={(event) => set({ name: event.target.value })} /></label>
    <label className="connection-field">Access key ID<input type="password" name="aws-access-key-id" value={fields.accessKeyId} autoComplete="new-password" spellCheck={false} maxLength={20} aria-describedby={`${id}-keys`} onChange={(event) => set({ accessKeyId: event.target.value })} /></label>
    <label className="connection-field">Secret access key<input type="password" name="aws-secret-access-key" value={fields.secretAccessKey} autoComplete="new-password" spellCheck={false} maxLength={40} aria-describedby={`${id}-keys`} onChange={(event) => set({ secretAccessKey: event.target.value })} /></label>
    <small id={`${id}-keys`} className="muted">Hai ô này luôn được che và không bao giờ được đọc lại. Dùng access key dài hạn (AKIA…) của một người dùng IAM riêng; không dùng tài khoản gốc.</small>
    <label className="connection-field">Vùng được phép<input name="aws-allowed-regions" value={fields.regions} autoComplete="off" spellCheck={false} placeholder="us-east-1, eu-west-1" aria-describedby={`${id}-regions`} onChange={(event) => set({ regions: event.target.value })} /></label>
    <small id={`${id}-regions`} className="muted">Cách nhau bằng dấu phẩy, tối đa {maxRegions} vùng. Vùng đầu tiên là vùng mặc định{regionCount > 0 ? ` (đang nhập ${regionCount} vùng)` : ''}.</small>
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {notice ? <div className="form-success" role="status">{notice}</div> : null}
    {result ? <dl className="destination-summary" aria-label="Kết quả xác minh AWS">
      <div><dt>Tài khoản</dt> <dd>{result.config?.accountId ?? '—'}</dd></div>
      <div><dt>Principal</dt> <dd>{result.config?.principalArn ?? '—'}</dd></div>
      <div><dt>Vùng</dt> <dd>{(result.config?.allowedRegions ?? []).join(', ') || result.config?.region || '—'}</dd></div>
      <div><dt>Danh tính</dt> <dd>Đã xác minh (STS)</dd></div>
      <div><dt>Vùng được phép</dt> <dd>Đã xác minh từng vùng</dd></div>
      <div><dt>Quyền cấp phát</dt> <dd>Đã xác minh VPC, EKS, ElastiCache, Aurora (chỉ đọc, chưa tạo tài nguyên)</dd></div>
    </dl> : null}
    <div className="connection-footer"><Button type="submit" tone="primary" disabled={saving}>{saving ? 'Đang kiểm tra và lưu…' : 'Kiểm tra và lưu'}</Button></div>
  </fieldset></form>;
}
