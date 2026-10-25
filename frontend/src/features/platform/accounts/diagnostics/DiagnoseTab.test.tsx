import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { apiError, stubApi, type Call } from '../../../../test/api';
import type { Account } from '../accountsApi';
import { DiagnoseTab } from './DiagnoseTab';
import { LatestDiagnostic } from './LatestDiagnostic';
import type { Counts, Finding, Purpose, Report, RunStatus, Scope, Summary } from './diagnosticsApi';

const account: Account = {
  id: 'id-2', key: 'lab-cluster', name: 'Lab cluster', type: 'INTERNAL', authenticationType: 'KUBECONFIG', status: 'READY', version: 3,
  config: { cluster: 'lab', endpoint: 'https://lab.example:6443' },
};
const aws: Account = { ...account, id: 'id-3', key: 'lab-aws', name: 'Lab AWS', type: 'AWS', authenticationType: 'AWS_ACCESS_KEY', config: { accountId: '123456789012', verificationRegions: ['us-east-1'] } };

const scope = (over: Partial<Scope> = {}): Scope => ({ purpose: 'GENERAL', mode: 'CONDITIONS', useCases: [], accountKey: 'lab-cluster', accountName: 'Lab cluster', accountType: 'INTERNAL', accountVersion: 3, ...over });
const finding = (id: string, verdict: Finding['verdict'], reason: string, over: Partial<Finding> = {}): Finding => ({ id, group: 'access', verdict, reason, facts: {}, affectedUseCases: [], dependsOn: [], observedAt: '2026-10-25T09:00:00Z', durationMs: 200, ...over });
const counts = (findings: Finding[], total = findings.length): Counts => ({
  pass: findings.filter((f) => f.verdict === 'PASS').length, fail: findings.filter((f) => f.verdict === 'FAIL').length, blocked: findings.filter((f) => f.verdict === 'BLOCKED').length,
  unknown: findings.filter((f) => f.verdict === 'UNKNOWN').length, na: findings.filter((f) => f.verdict === 'NA').length, pending: total - findings.length,
});
function report(id: string, findings: Finding[], over: Partial<Report> = {}): Report {
  const total = over.progress?.total ?? findings.length;
  return {
    id, scope: scope(), status: 'COMPLETED', progress: { completed: findings.length, total }, counts: counts(findings, total), ready: false, findings,
    cleanup: { status: 'NOT_REQUIRED', resources: [] }, limits: ['CONDITIONS_ONLY'], stale: false, staleReasons: [], createdAt: '2026-10-25T09:00:00Z', updatedAt: '2026-10-25T09:00:05Z', ...over,
  };
}
const summaryOf = (r: Report): Summary => ({ reportId: r.id, scope: r.scope, status: r.status, counts: r.counts, ready: r.ready, cleanupStatus: r.cleanup.status, stale: r.stale, createdAt: r.createdAt });

const base = '/api/v1/driver-accounts/lab-cluster/diagnostics';
const isList = (call: Call) => call.method === 'GET' && call.url.startsWith(`${base}?`);

function harness(reports: Report[]) {
  const store = new Map(reports.map((r) => [r.id, r]));
  const handler = (call: Call) => {
    if (isList(call)) {
      const purpose = new URL(call.url, 'http://x').searchParams.get('purpose');
      return Response.json({ reports: [...store.values()].filter((r) => r.scope.purpose === purpose).map(summaryOf), nextCursor: '' });
    }
    const m = /\/diagnostics\/([^/]+)$/.exec(call.url);
    if (call.method === 'GET' && m && store.has(m[1]!)) return Response.json(store.get(m[1]!));
    return undefined;
  };
  return { store, handler };
}

const mount = (props: { account?: Account; purpose?: Purpose; reportId?: string } = {}) => {
  const onRoute = vi.fn(); const onStale = vi.fn(); const onChanged = vi.fn();
  const view = render(<DiagnoseTab account={props.account ?? account} purpose={props.purpose ?? 'GENERAL'} reportId={props.reportId} onRoute={onRoute} onStale={onStale} onChanged={onChanged} />);
  const rerender = (reportId?: string, purpose: Purpose = props.purpose ?? 'GENERAL') => view.rerender(<DiagnoseTab account={props.account ?? account} purpose={purpose} reportId={reportId} onRoute={onRoute} onStale={onStale} onChanged={onChanged} />);
  return { onRoute, onStale, onChanged, rerender };
};

beforeEach(() => { vi.useFakeTimers({ shouldAdvanceTime: true, toFake: ['setTimeout', 'clearTimeout'] }); });
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });
const tick = (ms = 1100) => act(async () => { await vi.advanceTimersByTimeAsync(ms); });
const user = () => userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });

describe('account diagnostics tab', () => {
  it('shows the first-use state, starts a general check without an application and follows the persisted progress to its end', async () => {
    const running = report('r1', [finding('k8s.credential', 'PASS', 'CREDENTIAL_RESOLVED')], { status: 'RUNNING', progress: { completed: 1, total: 3 }, scope: scope() });
    const done = report('r1', [finding('k8s.credential', 'PASS', 'CREDENTIAL_RESOLVED'), finding('k8s.tls', 'FAIL', 'TLS_UNTRUSTED_CA', { remediation: 'provide-ca', group: 'access' }), finding('k8s.api', 'BLOCKED', 'PREREQUISITE_FAILED', { dependsOn: ['k8s.tls'] })]);
    let phase = 0;
    const h = harness([]);
    const calls = stubApi((call) => {
      if (call.method === 'POST' && call.url === base) return Response.json(running, { status: 202 });
      if (call.method === 'GET' && call.url === `${base}/r1`) { phase += 1; return Response.json(phase < 2 ? running : done); }
      return h.handler(call);
    });
    const { onRoute } = mount();
    expect(await screen.findByText('Chưa có kiểm tra chung nào')).toBeInTheDocument();
    await user().click(screen.getAllByRole('button', { name: 'Chạy kiểm tra' })[0]!);
    expect(calls.filter((c) => c.method === 'POST')).toHaveLength(1);
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ version: 3, purpose: 'GENERAL', mode: 'CONDITIONS' });
    expect((await screen.findAllByText('Đã hoàn tất 1/3 kiểm tra')).length).toBeGreaterThan(0);
    expect(onRoute).toHaveBeenCalledWith('GENERAL', 'r1');
    await tick(); await tick();
    // The failed and blocked findings are shown, nothing is turned into a pass.
    expect(await screen.findByText('Tài khoản có điều kiện chưa đạt')).toBeInTheDocument();
    expect(screen.getAllByText('TLS của API server').length).toBeGreaterThan(0);
    expect(screen.getByRole('list', { name: 'Giới hạn của báo cáo' })).toHaveTextContent('Chế độ điều kiện không chứng minh');
    expect(calls.filter((c) => c.method === 'POST')).toHaveLength(1);
  });

  it('explains a finding from safe codes: observed, expected, steps, dependency and the code', async () => {
    const r = report('r2', [finding('k8s.tls', 'FAIL', 'TLS_UNTRUSTED_CA', { remediation: 'provide-ca', facts: { host: '127.0.0.1:6443', tlsMode: 'CUSTOM_CA' } }), finding('k8s.api', 'BLOCKED', 'PREREQUISITE_FAILED', { dependsOn: ['k8s.tls'] }), finding('k8s.nodes', 'UNKNOWN', 'READ_FORBIDDEN')]);
    const h = harness([r]);
    stubApi(h.handler);
    mount();
    await screen.findByText('Tài khoản có điều kiện chưa đạt');
    const inspector = screen.getByRole('complementary', { name: 'Chi tiết' });
    expect(within(inspector).getByText('Chứng chỉ do CA không được tin cậy cấp.')).toBeInTheDocument();
    expect(within(inspector).getByText(/Chứng chỉ API server được CA đã cấu hình tin cậy/)).toBeInTheDocument();
    expect(within(inspector).getByText(/Lấy CA của cụm/)).toBeInTheDocument();
    await user().click(screen.getByRole('button', { name: 'Xem API Kubernetes' }));
    expect(within(screen.getByRole('complementary', { name: 'Chi tiết' })).getByRole('button', { name: /TLS của API server/ })).toBeInTheDocument();
    // UNKNOWN is counted apart from pass.
    expect(screen.getByLabelText('Tài khoản có điều kiện chưa đạt')).toHaveTextContent('Chưa xác định');
  });

  it('keeps the setup draft on a validation error and sends the chosen application, environment and mode', async () => {
    const h = harness([]);
    const created = report('r3', [], { status: 'QUEUED', progress: { completed: 0, total: 5 }, scope: scope({ purpose: 'USE_CASE', mode: 'EXECUTION', useCases: ['database'], context: { applicationKey: 'shop', applicationName: 'Shop', environmentKey: 'staging' } }) });
    const calls = stubApi(
      (call) => call.method === 'GET' && call.url === '/api/v1/applications' ? Response.json({ applications: [{ key: 'shop', name: 'Shop', environments: [{ key: 'staging', name: 'staging' }, { key: 'production', name: 'production' }] }] }) : undefined,
      (call) => call.method === 'POST' && call.url === base ? Response.json(created, { status: 202 }) : undefined,
      (call) => call.method === 'GET' && call.url === `${base}/r3` ? Response.json(created) : undefined,
      h.handler,
    );
    mount({ purpose: 'USE_CASE' });
    const u = user();
    await screen.findByText('Chưa có kiểm tra theo use case');
    await u.click(screen.getAllByRole('button', { name: 'Chạy kiểm tra' })[0]!);
    const dialog = await screen.findByRole('dialog', { hidden: true });
    await u.click(within(dialog).getByRole('button', { name: 'Bắt đầu kiểm tra' }));
    expect(await within(dialog).findByText('Chọn ít nhất một use case.')).toBeInTheDocument();
    expect(calls.some((c) => c.method === 'POST')).toBe(false);
    await u.click(within(dialog).getByLabelText('Cơ sở dữ liệu'));
    await u.selectOptions(await within(dialog).findByLabelText('Ứng dụng (tùy chọn)'), 'shop');
    await u.click(within(dialog).getByLabelText(/Thực thi/));
    await u.click(within(dialog).getByRole('button', { name: 'Bắt đầu kiểm tra' }));
    expect(await within(dialog).findByText('Chọn môi trường của ứng dụng.')).toBeInTheDocument();
    await u.selectOptions(within(dialog).getByLabelText('Môi trường'), 'staging');
    await u.click(within(dialog).getByRole('button', { name: 'Bắt đầu kiểm tra' }));
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ version: 3, purpose: 'USE_CASE', mode: 'EXECUTION', useCases: ['database'], application: 'shop', environment: 'staging' }));
  });

  it('requires a valid AWS region and sends it', async () => {
    const calls = stubApi((call) => call.url.startsWith('/api/v1/driver-accounts/lab-aws/diagnostics') && call.method === 'GET' ? Response.json({ reports: [], nextCursor: '' }) : undefined,
      (call) => call.method === 'POST' ? Response.json(report('r4', [], { status: 'QUEUED', scope: scope({ accountType: 'AWS', region: 'us-east-1', accountKey: 'lab-aws' }) }), { status: 202 }) : undefined);
    mount({ account: aws });
    const u = user();
    await screen.findByText('Chưa có kiểm tra chung nào');
    await u.click(screen.getAllByRole('button', { name: 'Chạy kiểm tra' })[0]!);
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ version: 3, purpose: 'GENERAL', mode: 'CONDITIONS', region: 'us-east-1' });
  });

  it('never resubmits after a lost start response and reconciles by reading', async () => {
    const h = harness([]);
    let posts = 0;
    const calls = stubApi((call) => { if (call.method === 'POST') { posts += 1; throw new TypeError('network'); } return undefined; }, h.handler);
    mount();
    await screen.findByText('Chưa có kiểm tra chung nào');
    await user().click(screen.getAllByRole('button', { name: 'Chạy kiểm tra' })[0]!);
    expect(await screen.findByText(/Chưa biết yêu cầu có được nhận hay không/)).toBeInTheDocument();
    expect(posts).toBe(1);
    expect(calls.filter((c) => c.method === 'GET' && c.url.startsWith(`${base}?`)).length).toBeGreaterThanOrEqual(2);
  });

  it('maps BUSY and stale-version rejections to safe guidance', async () => {
    const h = harness([]);
    let code = 'BUSY';
    stubApi((call) => call.method === 'POST' ? apiError(409, code) : undefined, h.handler);
    const { onStale } = mount();
    const u = user();
    await screen.findByText('Chưa có kiểm tra chung nào');
    await u.click(screen.getAllByRole('button', { name: 'Chạy kiểm tra' })[0]!);
    expect(await screen.findByText(/đang có một lượt chẩn đoán/)).toBeInTheDocument();
    code = 'STALE_VERSION';
    await u.click(screen.getAllByRole('button', { name: 'Chạy kiểm tra' })[0]!);
    await waitFor(() => expect(onStale).toHaveBeenCalled());
    expect(screen.getByText(/Tài khoản đã thay đổi/)).toBeInTheDocument();
  });

  it('shows a load failure, not an empty or clean report, when the history cannot be read', async () => {
    stubApi((call) => isList(call) ? apiError(500, 'INTERNAL_ERROR') : undefined);
    mount();
    expect(await screen.findByRole('alert')).toHaveTextContent('Không tải được báo cáo');
    expect(screen.queryByText('Chưa có kiểm tra chung nào')).not.toBeInTheDocument();
  });

  it('ignores a late response for another purpose after switching', async () => {
    const general = report('g1', [finding('k8s.api', 'PASS', 'API_ANSWERED')], { ready: true });
    const useCase = report('u1', [finding('uc.context', 'UNKNOWN', 'APPLICATION_CONTEXT_REQUIRED')], { scope: scope({ purpose: 'USE_CASE', useCases: ['database'] }) });
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    const h = harness([general, useCase]);
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.includes('purpose=GENERAL')) await gate; // the general history answers late
      const response = h.handler({ method: 'GET', url });
      return response ?? Response.json({ code: 'NOT_FOUND' }, { status: 404 });
    }));
    const view = render(<DiagnoseTab account={account} purpose="GENERAL" onRoute={vi.fn()} onStale={vi.fn()} />);
    view.rerender(<DiagnoseTab account={account} purpose="USE_CASE" onRoute={vi.fn()} onStale={vi.fn()} />);
    await screen.findByText('Cần xử lý', { exact: false });
    release();
    await tick();
    expect(screen.queryByText(/API Kubernetes/)).not.toBeInTheDocument();
    expect(screen.getAllByText('Ngữ cảnh ứng dụng').length).toBeGreaterThan(0);
  });

  it('exports only the safe report with its scope, limits and findings', async () => {
    const r = report('r5', [finding('k8s.api', 'PASS', 'API_ANSWERED')]);
    stubApi(harness([r]).handler);
    let blob: Blob | undefined;
    vi.stubGlobal('URL', Object.assign(URL, { createObjectURL: (b: Blob) => { blob = b; return 'blob:x'; }, revokeObjectURL: () => undefined }));
    mount();
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined);
    await user().click(await screen.findByRole('button', { name: 'Xuất JSON' }));
    const text = await new Promise<string>((resolve) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result)); reader.readAsText(blob!); });
    const body = JSON.parse(text) as { account: Record<string, string>; report: Report };
    expect(body.account).toEqual({ key: 'lab-cluster', name: 'Lab cluster', type: 'INTERNAL' });
    expect(body.report.scope.accountVersion).toBe(3);
    expect(body.report.limits).toEqual(['CONDITIONS_ONLY']);
    expect(JSON.stringify(body)).not.toMatch(/secret|token|kubeconfig/i);
  });

  it('marks a report stale and keeps unknown cleanup visible', async () => {
    const r = report('r6', [finding('k8s.exec.cleanup', 'UNKNOWN', 'CLEANUP_UNCONFIRMED')], { stale: true, staleReasons: ['DRAFTS_CHANGED'], cleanup: { status: 'UNKNOWN', resources: [{ kind: 'namespace', name: 'orch-diag-abc', state: 'UNKNOWN' }] } });
    stubApi(harness([r]).handler);
    mount();
    expect(await screen.findByText('Báo cáo cũ so với hiện tại.')).toBeInTheDocument();
    expect(screen.getByText('Bản nháp workload đã thay đổi.')).toBeInTheDocument();
    expect(screen.getAllByText(/Chưa xác nhận được việc dọn/).length).toBeGreaterThan(0);
  });
});

describe('latest diagnostic on the account list', () => {
  const sum = (over: Partial<Summary> = {}): Summary => ({ reportId: 'r1', scope: scope(), status: 'COMPLETED', counts: { pass: 3, fail: 0, blocked: 0, unknown: 1, na: 0, pending: 0 }, ready: false, cleanupStatus: 'NOT_REQUIRED', stale: false, createdAt: '2026-10-25T09:00:00Z', ...over });
  it('keeps "not diagnosed", "history unreadable" and a result with unknowns apart', async () => {
    const open = vi.fn();
    const { rerender } = render(<LatestDiagnostic account={account} onOpen={open} />);
    expect(screen.getByText('Chưa chẩn đoán')).toBeInTheDocument();
    rerender(<LatestDiagnostic account={{ ...account, diagnosticsUnavailable: true }} onOpen={open} />);
    expect(screen.getByRole('status')).toHaveTextContent('Không đọc được lịch sử chẩn đoán');
    expect(screen.queryByText('Chưa chẩn đoán')).not.toBeInTheDocument();
    rerender(<LatestDiagnostic account={{ ...account, latestDiagnostic: sum() }} onOpen={open} />);
    const button = screen.getByRole('button', { name: /Mở chẩn đoán gần nhất/ });
    expect(button).toHaveTextContent('1 chưa xác định');
    expect(button).not.toHaveTextContent('Đã đạt');
    await userEvent.click(button);
    expect(open).toHaveBeenCalledWith('GENERAL', 'r1');
    const status: RunStatus = 'CANCELLED';
    rerender(<LatestDiagnostic account={{ ...account, latestDiagnostic: sum({ status, counts: { pass: 2, fail: 0, blocked: 0, unknown: 0, na: 0, pending: 1 } }) }} onOpen={open} />);
    expect(screen.getByRole('button')).not.toHaveTextContent('Đã đạt');
  });
});

describe('report selection', () => {
  const pass = (id: string) => report(id, [finding(`k8s.${id}`, 'PASS', 'API_ANSWERED')], { ready: true });
  const gate = () => { let release: () => void = () => undefined; const promise = new Promise<void>((resolve) => { release = resolve; }); return { promise, release }; };

  it('keeps the last chosen report when two replies of the same purpose arrive out of order', async () => {
    const [a, b, latest] = [pass('a'), pass('b'), pass('latest')];
    const h = harness([latest, a, b]);
    const slowA = gate();
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url === `${base}/a`) await slowA.promise; // the older request answers last
      return h.handler({ method: 'GET', url }) ?? Response.json({ code: 'NOT_FOUND' }, { status: 404 });
    }));
    const { rerender } = mount();
    await screen.findByText('Mọi kiểm tra đã đạt');
    rerender('a');
    rerender('b');
    await waitFor(() => expect(shownReportCode()).toContain('v3'));
    expect(screen.getAllByText(/API Kubernetes|k8s\.b/).length).toBeGreaterThan(0);
    slowA.release();
    await tick();
    // The late reply for A must not replace B.
    expect(screen.queryByText('k8s.a')).not.toBeInTheDocument();
    expect(screen.getAllByText('k8s.b').length).toBeGreaterThan(0);
  });

  it('reads an explicit report directly, even beyond the first history page, and refuses another scope', async () => {
    const old = pass('old');
    const mismatch = report('other', [finding('uc.context', 'UNKNOWN', 'APPLICATION_CONTEXT_REQUIRED')], { scope: scope({ purpose: 'USE_CASE', useCases: ['deploy'] }) });
    const calls = stubApi((call) => isList(call) ? Response.json({ reports: [summaryOf(pass('newest'))], nextCursor: 'more' }) : undefined,
      (call) => call.url === `${base}/old` ? Response.json(old) : call.url === `${base}/other` ? Response.json(mismatch) : call.url === `${base}/gone` ? apiError(404, 'NOT_FOUND') : undefined);
    const { rerender } = mount({ reportId: 'old' });
    await waitFor(() => expect(calls.some((c) => c.url === `${base}/old`)).toBe(true));
    expect(await screen.findByText('Mọi kiểm tra đã đạt')).toBeInTheDocument();
    // The tab is already open: a different requested report is read, not ignored.
    rerender('other');
    expect(await screen.findByText(/thuộc phạm vi khác/)).toBeInTheDocument();
    expect(screen.queryByText('Mọi kiểm tra đã đạt')).not.toBeInTheDocument();
    rerender('gone');
    expect(await screen.findByRole('alert')).toHaveTextContent('Không tìm thấy');
    expect(screen.getByRole('button', { name: 'Mở báo cáo mới nhất' })).toBeInTheDocument();
  });

  it('configures from the scope of the report on screen and tells the parent when a run starts and ends', async () => {
    const shownReport = report('exec', [finding('k8s.api', 'PASS', 'API_ANSWERED')], { scope: scope({ purpose: 'USE_CASE', mode: 'EXECUTION', useCases: ['database', 'cache'], context: { applicationKey: 'shop', applicationName: 'Shop', environmentKey: 'staging' } }) });
    const created = { ...shownReport, id: 'next', status: 'QUEUED' as RunStatus, progress: { completed: 0, total: 3 }, findings: [], counts: counts([], 3) };
    const finished = { ...created, status: 'COMPLETED' as RunStatus, findings: shownReport.findings, progress: { completed: 1, total: 1 }, counts: counts(shownReport.findings) };
    let polls = 0;
    const calls = stubApi(
      (call) => call.method === 'GET' && call.url === '/api/v1/applications' ? Response.json({ applications: [{ key: 'shop', name: 'Shop', environments: [{ key: 'staging', name: 'staging' }] }] }) : undefined,
      (call) => call.method === 'POST' && call.url === base ? Response.json(created, { status: 202 }) : undefined,
      (call) => { if (call.method === 'GET' && call.url === `${base}/next`) { polls += 1; return Response.json(finished); } return undefined; },
      harness([shownReport]).handler,
    );
    const { onChanged } = mount({ purpose: 'USE_CASE' });
    await screen.findByText('Kết quả chưa đầy đủ');
    const u = user();
    await u.click(screen.getByRole('button', { name: 'Thiết lập' }));
    const dialog = await screen.findByRole('dialog', { hidden: true });
    expect(within(dialog).getByLabelText('Cơ sở dữ liệu')).toBeChecked();
    expect(within(dialog).getByLabelText('Cache')).toBeChecked();
    expect(within(dialog).getByLabelText('Triển khai')).not.toBeChecked();
    expect(within(dialog).getByLabelText(/Thực thi/)).toBeChecked();
    await waitFor(() => expect(within(dialog).getByLabelText('Ứng dụng (tùy chọn)')).toHaveValue('shop'));
    await u.click(within(dialog).getByRole('button', { name: 'Bắt đầu kiểm tra' }));
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toMatchObject({ purpose: 'USE_CASE', mode: 'EXECUTION', useCases: ['database', 'cache'], application: 'shop', environment: 'staging' }));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    const afterStart = onChanged.mock.calls.length;
    await tick(); await tick();
    expect(polls).toBeGreaterThan(0);
    await waitFor(() => expect(onChanged.mock.calls.length).toBeGreaterThan(afterStart));
  });
});

function shownReportCode(): string {
  return document.querySelector('.ac-report-time')?.textContent ?? '';
}
