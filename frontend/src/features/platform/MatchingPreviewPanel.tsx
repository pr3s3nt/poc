import { useEffect, useRef, useState } from 'react';
import { fieldNote } from '../../shared/api/errors';
import { api, ApiError, sessionExpiredEvent } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';

// Exact single-context matching preview (UC-03 V02-05, OC-04c). The server
// computes it from a read-only snapshot with the same five-field matcher the
// planner uses; nothing is saved. The result belongs to exactly one
// candidate+context (its signature): any input, scope or session change drops
// the result and ignores a reply still in flight. Only this server result may
// name a winner; the browser never ranks Definitions.

export type MatchingCandidateInput = { definition: Record<string, unknown> | null; problem: string };
export type MatchingSample = { applicationId: string; environmentId: string; resourceId: string; class: string };

type Criterion = { env_type?: string; app_id?: string; env_id?: string; res_id?: string; class?: string };
type Winner = {
  key: string; name?: string; version?: number; driverType?: string; score: number;
  account?: { key: string; name: string; type: string; version: number };
  region?: string;
  clusterSource?: { kind: 'RESOURCE' | 'LEGACY_ENVIRONMENT'; reference?: string };
};
type Result = {
  status: 'INCOMPLETE' | 'MATCHED' | 'NO_MATCH' | 'AMBIGUOUS' | 'CONNECTION_CONFLICT';
  missingFields?: string[];
  candidates?: { key: string; name?: string; matched: boolean; bestCriterion?: Criterion; score?: number }[];
  winner?: Winner;
  ties?: string[];
  connectionConflict?: { definitionKey: string; driverAccount?: string; reason: string };
  snapshot?: { marker: string; environmentVersion: number };
};
type State = { phase: 'idle' } | { phase: 'loading' } | { phase: 'result'; result: Result } | { phase: 'error'; message: string };

const statusText: Record<Result['status'], string> = {
  INCOMPLETE: 'Chưa đủ context để kết luận',
  MATCHED: 'Có đúng một cấu hình thắng',
  NO_MATCH: 'Không có cấu hình nào khớp (khác với hòa điểm)',
  AMBIGUOUS: 'Hòa điểm: nhiều cấu hình cùng điểm cao nhất, chưa có cấu hình thắng',
  CONNECTION_CONFLICT: 'Cấu hình thắng có tài khoản hoặc vùng không dùng được; hệ thống không chọn cấu hình khác',
};
const conflictText: Record<string, string> = {
  ACCOUNT_MISSING: 'cấu hình chưa chọn tài khoản',
  ACCOUNT_NOT_FOUND: 'không tìm thấy tài khoản trong Tổ chức',
  ACCOUNT_KIND_MISMATCH: 'tài khoản không đúng loại (cần tài khoản AWS)',
  ACCOUNT_NOT_READY: 'tài khoản chưa Sẵn sàng (READY)',
  REGION_INVALID: 'vùng AWS không hợp lệ hoặc không phải chuỗi literal',
};
const fieldText: Record<string, string> = { resourceId: 'ID tài nguyên (res_id)', class: 'Class' };

function describeCriterion(criterion?: Criterion): string {
  const parts = Object.entries(criterion ?? {}).filter(([, value]) => typeof value === 'string' && value !== '').map(([key, value]) => `${key}=${value}`);
  return parts.length === 0 ? 'mọi nơi' : parts.join(', ');
}

// Transport/validation failures are never shown as a matching result.
function describeTransport(reason: unknown): string {
  if (!(reason instanceof ApiError)) return 'Chưa nhận được phản hồi từ máy chủ nên chưa có kết quả matching. Đây không phải kết quả matching; dữ liệu đã nhập được giữ nguyên.';
  switch (reason.status) {
    case 400: return 'Máy chủ từ chối cấu hình đang nhập để kiểm tra matching (ID, Loại tài nguyên, Tên hoặc điều kiện không hợp lệ, hoặc ID dành riêng cho hệ thống).' + fieldNote(reason.field);
    case 401: return 'Phiên đăng nhập đã hết hạn. Hãy đăng nhập lại.';
    case 403: return 'Bạn không có quyền kiểm tra matching. Chỉ Kỹ sư nền tảng hoặc Quản trị viên được dùng.';
    case 404: return 'Không tìm thấy ứng dụng hoặc môi trường mẫu trong Tổ chức của bạn.';
    case 413: return 'Nội dung cấu hình quá lớn để kiểm tra. Hãy rút gọn rồi thử lại.';
    default: return 'Máy chủ gặp lỗi khi kiểm tra matching. Đây không phải kết quả matching; hãy thử lại.';
  }
}

export function MatchingPreviewPanel({ candidate, sample, scopeKey = '' }: { candidate: MatchingCandidateInput; sample: MatchingSample; scopeKey?: string }) {
  const [state, setState] = useState<State>({ phase: 'idle' });
  const seq = useRef(0);
  const body = candidate.definition === null ? '' : JSON.stringify({ definition: candidate.definition, context: { applicationId: sample.applicationId, environmentId: sample.environmentId, resourceId: sample.resourceId.trim(), class: sample.class.trim() } });
  const signature = `${scopeKey}\u0000${body}`;

  // Any change of candidate, context or scope invalidates the result and the pending reply.
  useEffect(() => { seq.current += 1; setState({ phase: 'idle' }); }, [signature]);
  useEffect(() => {
    const expire = () => { seq.current += 1; setState({ phase: 'idle' }); };
    window.addEventListener(sessionExpiredEvent, expire);
    return () => { seq.current += 1; window.removeEventListener(sessionExpiredEvent, expire); }; // unmount: ignore any late reply
  }, []);

  const noScope = sample.applicationId === '' || sample.environmentId === '';
  const blocked = candidate.definition === null ? candidate.problem : noScope ? 'Chọn ứng dụng và môi trường mẫu để kiểm tra matching chính xác.' : '';

  async function run() {
    if (blocked || state.phase === 'loading') return;
    const request = ++seq.current;
    setState({ phase: 'loading' });
    try {
      const result = await api<Result>('/resource-definitions/matching-preview', { method: 'POST', body });
      if (request === seq.current) setState({ phase: 'result', result });
    } catch (reason) {
      if (request === seq.current) setState({ phase: 'error', message: describeTransport(reason) });
    }
  }

  const result = state.phase === 'result' ? state.result : null;
  return <div className="criteria-preview matching-preview" role="group" aria-label="Kiểm tra matching chính xác">
    <h4>Kiểm tra matching chính xác</h4>
    <p className="feature-note">Máy chủ tính trên snapshot hiện tại với cấu hình đang nhập (chưa lưu) và ngữ cảnh ở trên, bằng cùng bộ so khớp năm field của bộ lập kế hoạch: không lọc theo profile, tài khoản hay vùng. Không cần Môi trường đã cấu hình kết nối. Không lưu, không tạo tài nguyên, không thay Xem trước toàn bộ.</p>
    {blocked ? <p>{blocked}</p> : null}
    <div className="form-actions"><Button type="button" onClick={() => { void run(); }} disabled={blocked !== '' || state.phase === 'loading'}>Kiểm tra matching</Button></div>
    {state.phase === 'loading' ? <p role="status">Đang kiểm tra matching…</p> : null}
    {state.phase === 'error' ? <div className="form-error" role="alert" aria-label="Lỗi kiểm tra matching">{state.message} <Button type="button" onClick={() => { void run(); }}>Thử lại kiểm tra matching</Button></div> : null}
    {result ? <div role="region" aria-label="Kết quả matching chính xác">
      <p><strong>{statusText[result.status] ?? 'Trạng thái không xác định'}</strong></p>
      {result.status === 'INCOMPLETE' ? <p>Cần bổ sung context: {(result.missingFields ?? []).map((field) => fieldText[field] ?? field).join(', ')}. Chưa kết luận tie hay cấu hình thắng.</p> : null}
      {result.winner ? <div role="group" aria-label="Cấu hình thắng"><p>Cấu hình thắng: <strong>{result.winner.name || result.winner.key}</strong> (ID <code>{result.winner.key}</code>{result.winner.version ? `, phiên bản ${result.winner.version}` : ''}, điểm {result.winner.score}).</p>
        {result.winner.account ? <p>Tài khoản: <strong>{result.winner.account.name}</strong> (<code>{result.winner.account.key}</code>, {result.winner.account.type}, phiên bản {result.winner.account.version}){result.winner.region ? <>; vùng hiệu lực: <strong>{result.winner.region}</strong></> : null}.</p> : null}
        {result.winner.clusterSource ? <p>Cluster kế thừa từ: {result.winner.clusterSource.kind === 'RESOURCE' ? <>tài nguyên <code>{result.winner.clusterSource.reference}</code></> : <><strong>Môi trường (cách cũ, giữ tới V03)</strong></>}; không chọn lại tài khoản.</p> : null}</div> : null}
      {result.ties && result.ties.length > 0 ? <p>Các cấu hình đang hòa ở điểm cao nhất: <strong>{result.ties.join(', ')}</strong>. Theo snapshot này, matching mơ hồ cho tới khi các điều kiện thay đổi; kết quả thật vẫn do hệ thống kiểm tra lại.</p> : null}
      {result.connectionConflict ? <p role="alert">Xung đột tài khoản của cấu hình thắng <strong>{result.connectionConflict.definitionKey}</strong>{result.connectionConflict.driverAccount ? ` (tài khoản ${result.connectionConflict.driverAccount})` : ''}: {conflictText[result.connectionConflict.reason] ?? 'tài khoản không dùng được'}. Hệ thống không chọn cấu hình thay thế.</p> : null}
      {(result.candidates ?? []).length > 0 ? <ul aria-label="Chi tiết từng cấu hình">{(result.candidates ?? []).map((item) => <li key={item.key}><strong>{item.name || item.key}</strong> (<code>{item.key}</code>): {result.status === 'INCOMPLETE' ? 'chưa đánh giá do thiếu context' : item.matched ? `khớp, điểm ${item.score ?? 0}, điều kiện tốt nhất: ${describeCriterion(item.bestCriterion)}` : 'không khớp context'}</li>)}</ul> : null}
      {result.snapshot ? <p className="feature-note">Snapshot {result.snapshot.marker.slice(0, 12)} · phiên bản môi trường {result.snapshot.environmentVersion}. Kết quả theo snapshot; hệ thống vẫn kiểm tra lại khi đăng ký và triển khai.</p> : null}
    </div> : null}
  </div>;
}
