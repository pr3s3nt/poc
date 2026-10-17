import { api, ApiError, isStaleConflict } from '../../../shared/api/client';

// V01 infrastructure account (UC-04 AC-01..12). Public DTO only: the server
// never returns a secret reference, key or kubeconfig, and neither does this file.
export type AccountType = 'AWS' | 'INTERNAL';
export type Account = {
  id?: string; key: string; name: string; type: AccountType; authenticationType: string;
  status: string; version: number;
  config?: { cluster?: string; kubeContext?: string; endpoint?: string; tls?: { mode?: string; serverName?: string }; accountId?: string; principalArn?: string; verificationRegions?: string[] };
  verification?: { verified?: boolean; endpoint?: string; serverVersion?: string; accountId?: string; principalArn?: string; identity?: string; regionCheck?: string; capabilityCheck?: string; verifiedAt?: string; regions?: string[]; capabilities?: string[] };
};
export type KubeconfigContext = { name: string; cluster: string; endpoint: string };

export const maxNameLength = 100;
export const maxRegions = 10;
export const maxKubeconfigBytes = 1 << 20;
const accessKeyShape = /^AKIA[A-Z0-9]{16}$/;
const secretKeyShape = /^[A-Za-z0-9/+]{40}$/;
const regionShape = /^[a-z]{2}(-[a-z]+)+-[0-9]$/;

export const listAccounts = async () => (await api<{ accounts: Account[] }>('/driver-accounts')).accounts ?? [];
export const getAccount = (key: string) => api<Account>(`/driver-accounts/${encodeURIComponent(key)}`);
export const inspectKubeconfig = (kubeconfig: string) => api<{ contexts: KubeconfigContext[] }>('/driver-accounts/internal/inspect', { method: 'POST', body: JSON.stringify({ kubeconfig }) });
export const createInternal = (body: { name: string; kubeconfig: string; context: string }) => api<Account>('/driver-accounts/internal', { method: 'POST', body: JSON.stringify(body) });
export const createAws = (body: { name: string; accessKeyId: string; secretAccessKey: string; verificationRegions: string[] }) => api<Account>('/driver-accounts/aws', { method: 'POST', body: JSON.stringify(body) });
export const renameAccount = (key: string, body: { name: string; version: number }) => api<Account>(`/driver-accounts/${encodeURIComponent(key)}`, { method: 'PUT', body: JSON.stringify(body) });
export const verifyAccount = (key: string, body: { version: number; verificationRegions?: string[] }) => api<Account>(`/driver-accounts/${encodeURIComponent(key)}/verify`, { method: 'POST', body: JSON.stringify(body) });
export const replaceCredentials = (key: string, body: Record<string, unknown>) => api<Account>(`/driver-accounts/${encodeURIComponent(key)}/credentials`, { method: 'PUT', body: JSON.stringify(body) });

/** Splits on commas and white space, keeping typed order. */
export function parseRegions(text: string): string[] { return text.split(/[\s,;]+/).filter(Boolean); }

export function validateName(raw: string): string {
  const name = raw.trim();
  if (!name) return 'Nhập tên tài khoản.';
  if ([...name].length > maxNameLength) return `Tên tài khoản tối đa ${maxNameLength} ký tự.`;
  return '';
}

export function validateRegions(text: string): string {
  const regions = parseRegions(text);
  if (regions.length === 0) return 'Nhập ít nhất một vùng kiểm tra, ví dụ us-east-1.';
  if (regions.length > maxRegions) return `Chỉ kiểm tra tối đa ${maxRegions} vùng.`;
  if (regions.some((region) => !regionShape.test(region))) return 'Tên vùng không hợp lệ. Dùng dạng như us-east-1, cách nhau bằng dấu phẩy.';
  if (new Set(regions).size !== regions.length) return 'Mỗi vùng chỉ được nhập một lần.';
  return '';
}

export function validateAwsKeys(accessKeyId: string, secretAccessKey: string): string {
  if (!accessKeyShape.test(accessKeyId)) return 'Access key ID không đúng dạng: 20 ký tự bắt đầu bằng AKIA (khóa dài hạn của người dùng IAM).';
  if (!secretKeyShape.test(secretAccessKey)) return 'Secret access key không đúng dạng: đúng 40 ký tự gồm chữ, số, dấu / và +.';
  return '';
}

export type Operation = 'create' | 'rename' | 'verify' | 'replace' | 'inspect';

/** True when the mutation may or may not have been committed: no HTTP status, or the server itself reports an uncertain commit. */
export const outcomeUnknown = (reason: unknown) => !(reason instanceof ApiError) || reason.code === 'COMMIT_UNCERTAIN';

/**
 * Fixed Vietnamese guidance by operation, status and stable code. Raw server
 * or provider text never reaches the page; ApiError.message is already the
 * safe text for a known code.
 */
export function accountGuidance(operation: Operation, reason: unknown): string {
  if (outcomeUnknown(reason) && operation !== 'inspect') {
    return 'Kết nối bị gián đoạn hoặc máy chủ chưa xác nhận nên chưa biết thao tác đã được lưu hay chưa. Credential mới, nếu đã ghi, được giữ lại. Console đang tải lại dữ liệu hiện hành; hãy xem kết quả rồi mới quyết định, Console không tự gửi lại.';
  }
  if (!(reason instanceof ApiError)) {
    return operation === 'inspect'
      ? 'Chưa kiểm tra được kubeconfig vì chưa nhận được phản hồi từ máy chủ. Nhấn “Kiểm tra lại kubeconfig”.'
      : 'Chưa nhận được phản hồi từ máy chủ nên chưa biết thao tác đã được lưu hay chưa. Console đang tải lại dữ liệu hiện hành; kiểm tra rồi mới thử lại, Console không tự gửi lại.';
  }
  if (isStaleConflict(reason)) return 'Tài khoản đã thay đổi kể từ lần tải trước (phiên bản không còn khớp). Chưa có gì bị ghi đè. Nhấn “Tải lại bản mới nhất”, xem lại rồi lưu lại tường minh.';
  switch (reason.status) {
    case 401: return 'Phiên đăng nhập đã hết hạn. Đăng nhập lại rồi thử lại.';
    case 403: return 'Tài khoản đăng nhập không có quyền quản lý tài khoản hạ tầng. Cần vai trò Kỹ sư nền tảng hoặc Quản trị.';
    case 404: return 'Không tìm thấy tài khoản này trong tổ chức của bạn. Tải lại danh sách.';
    case 413: return 'Dữ liệu gửi lên quá lớn. Rút gọn rồi thử lại.';
    case 503: return 'Kho lưu thông tin xác thực của nền tảng chưa sẵn sàng. Liên hệ quản trị nền tảng rồi thử lại.';
  }
  if (reason.status === 422) {
    const effect = operation === 'verify'
      ? 'Lần kiểm tra vừa rồi thất bại; bản ghi và credential đang lưu được giữ nguyên.'
      : operation === 'replace' ? 'Credential cũ vẫn được giữ và đang dùng.' : 'Chưa lưu tài khoản.';
    return `${reason.message} ${effect}`;
  }
  if (reason.status === 400) return `${reason.message} Dữ liệu đang nhập được giữ nguyên, trừ các ô khóa đã được xóa.`;
  return reason.status >= 500
    ? 'Máy chủ gặp lỗi khi xử lý. Tải lại tài khoản để xem kết quả thực tế trước khi thử lại.'
    : 'Không thực hiện được thao tác. Kiểm tra dữ liệu rồi thử lại.';
}

export function typeLabel(type: AccountType | string): string { return type === 'AWS' ? 'AWS' : type === 'INTERNAL' ? 'Internal (cluster có sẵn)' : type; }
export function authLabel(auth: string): string { return auth === 'AWS_ACCESS_KEY' ? 'Access key' : auth === 'KUBECONFIG' ? 'Kubeconfig' : auth; }
export function statusLabel(status: string): string {
  if (status === 'READY') return 'Sẵn sàng';
  if (status === 'VERIFYING') return 'Chưa xác minh';
  if (status === 'REJECTED') return 'Bị từ chối';
  return status;
}
