import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MatchingPreviewPanel, type MatchingSample } from './MatchingPreviewPanel';
import { CriteriaEditor, emptyDraft, type ApplicationOption } from './CriteriaEditor';
import { RenderingTemplatesPage } from './RenderingTemplatesPage';

const sample: MatchingSample = { applicationId: 'shop', environmentId: 'staging', resourceId: 'db', class: 'default' };
const definition = { key: 'pg-new', resourceType: 'postgres', criteria: [{}] };
const tie = { status: 'AMBIGUOUS', missingFields: [], candidates: [{ key: 'pg-new', eligible: true, matched: true, score: 0, bestCriterion: {} }, { key: 'pg-shared', eligible: true, matched: true, score: 0, bestCriterion: {} }, { key: 'pg-aws', eligible: false, matched: false }], ties: ['pg-new', 'pg-shared'], snapshot: { marker: 'abcdef0123456789', environmentVersion: 3 } };

afterEach(() => vi.unstubAllGlobals());

function stub(handler: (body: Record<string, unknown>) => Response | Promise<Response>) {
  const calls: Record<string, unknown>[] = [];
  vi.stubGlobal('fetch', vi.fn(async (_url: string | URL | Request, init?: RequestInit) => { const body = JSON.parse(String(init?.body)) as Record<string, unknown>; calls.push(body); return handler(body); }));
  return calls;
}
const run = () => screen.getByRole('button', { name: 'Kiểm tra matching' });

describe('exact matching preview panel (T19)', () => {
  it('sends the full unsaved candidate with the explicit sample context and shows ties and the snapshot warning', async () => {
    const calls = stub(() => Response.json(tie));
    render(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={sample} />);
    expect(calls).toHaveLength(0); // nothing is sent until the explicit action
    await userEvent.setup().click(run());
    expect(await screen.findByText(/Mơ hồ: nhiều cấu hình cùng điểm cao nhất/)).toBeInTheDocument();
    expect(calls).toEqual([{ definition, context: { applicationId: 'shop', environmentId: 'staging', resourceId: 'db', class: 'default' } }]);
    expect(screen.getByText('pg-new, pg-shared')).toBeInTheDocument();
    expect(screen.getByRole('list', { name: 'Chi tiết từng cấu hình' }).textContent).toMatch(/pg-aws: không đủ điều kiện/);
    expect(screen.getByText(/Snapshot abcdef012345 · phiên bản môi trường 3/)).toBeInTheDocument();
    expect(screen.getByText(/hệ thống vẫn kiểm tra lại khi đăng ký và triển khai/)).toBeInTheDocument();
  });

  it('shows INCOMPLETE as its own state without a winner or tie', async () => {
    stub(() => Response.json({ status: 'INCOMPLETE', missingFields: ['resourceId', 'class'], candidates: [], snapshot: { marker: 'm', environmentVersion: 1 } }));
    render(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={{ ...sample, resourceId: '', class: '' }} />);
    await userEvent.setup().click(run());
    expect(await screen.findByText(/Cần bổ sung context: ID tài nguyên \(res_id\), Class/)).toBeInTheDocument();
    expect(screen.queryByText(/Cấu hình thắng:/)).not.toBeInTheDocument();
    expect(screen.queryByText(/hòa/)).not.toBeInTheDocument();
  });

  it('does not call eligible candidates unmatched when the context is incomplete or the node is a system binding', async () => {
    const candidates = [{ key: 'pg-new', eligible: true, matched: false }, { key: 'pg-aws', eligible: false, matched: false }];
    stub(() => Response.json({ status: 'INCOMPLETE', missingFields: ['class'], candidates, snapshot: { marker: 'm', environmentVersion: 1 } }));
    const first = render(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={sample} />);
    await userEvent.setup().click(run());
    const list = await screen.findByRole('list', { name: 'Chi tiết từng cấu hình' });
    expect(list.textContent).toMatch(/pg-new: chưa đánh giá do thiếu context/);
    expect(list.textContent).not.toMatch(/không khớp context/);
    expect(list.textContent).toMatch(/pg-aws: không đủ điều kiện/);
    first.unmount();
    stub(() => Response.json({ status: 'SYSTEM_BINDING', missingFields: [], candidates, snapshot: { marker: 'm', environmentVersion: 1 } }));
    render(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={sample} />);
    await userEvent.setup().click(run());
    const system = await screen.findByRole('list', { name: 'Chi tiết từng cấu hình' });
    expect(system.textContent).toMatch(/pg-new: không xét điều kiện: node dùng binding hệ thống/);
    expect(system.textContent).not.toMatch(/không khớp context/);
  });

  it('blocks the action with a reason when the candidate or the scope is incomplete', async () => {
    const calls = stub(() => Response.json(tie));
    const { rerender } = render(<MatchingPreviewPanel candidate={{ definition: null, problem: 'Nhập ID cấu hình hợp lệ.' }} sample={sample} />);
    expect(screen.getByText('Nhập ID cấu hình hợp lệ.')).toBeInTheDocument();
    expect(run()).toBeDisabled();
    rerender(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={{ ...sample, applicationId: '', environmentId: '' }} />);
    expect(screen.getByText(/Chọn ứng dụng và môi trường mẫu/)).toBeInTheDocument();
    expect(run()).toBeDisabled();
    expect(calls).toHaveLength(0);
  });

  it('separates transport errors from matching results and retries only on explicit action', async () => {
    let fail = true;
    const calls = stub(() => (fail ? Response.json({ error: 'internal failure with secret' }, { status: 500 }) : Response.json({ status: 'MATCHED', winner: { key: 'pg-new', score: 0 }, candidates: [], snapshot: { marker: 'm', environmentVersion: 1 } })));
    render(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={sample} />);
    const user = userEvent.setup();
    await user.click(run());
    const alert = await screen.findByRole('alert', { name: 'Lỗi kiểm tra matching' });
    expect(alert).toHaveTextContent('Đây không phải kết quả matching');
    expect(alert).not.toHaveTextContent('secret');
    expect(screen.queryByRole('region', { name: 'Kết quả matching chính xác' })).not.toBeInTheDocument();
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 50)); });
    expect(calls).toHaveLength(1); // no automatic retry
    fail = false;
    await user.click(screen.getByRole('button', { name: 'Thử lại kiểm tra matching' }));
    expect(await screen.findByText('Có một cấu hình thắng')).toBeInTheDocument();
    expect(calls).toHaveLength(2);
  });

  it('maps validation, scope and target statuses to Vietnamese transport messages', async () => {
    for (const [status, text] of [[400, /từ chối cấu hình/], [404, /Không tìm thấy ứng dụng/], [422, /chưa có đích thực thi/], [403, /không có quyền/]] as const) {
      stub(() => Response.json({ error: 'English backend text' }, { status }));
      const { unmount } = render(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={sample} />);
      await userEvent.setup().click(run());
      const alert = await screen.findByRole('alert', { name: 'Lỗi kiểm tra matching' });
      expect(alert).toHaveTextContent(text);
      expect(alert).not.toHaveTextContent('English backend text');
      unmount();
    }
  });

  it('drops the result when the candidate, context or scope changes', async () => {
    stub(() => Response.json(tie));
    const { rerender } = render(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={sample} scopeKey="a" />);
    const user = userEvent.setup();
    await user.click(run());
    await screen.findByRole('region', { name: 'Kết quả matching chính xác' });
    for (const next of [
      { candidate: { definition: { ...definition, key: 'pg-other' }, problem: '' }, sample, scopeKey: 'a' },
      { candidate: { definition: { ...definition, criteria: [{ class: 'fast' }] }, problem: '' }, sample, scopeKey: 'a' },
      { candidate: { definition: { ...definition, driverInputs: { values: { x: 1 } } }, problem: '' }, sample, scopeKey: 'a' },
      { candidate: { definition: { ...definition, connectionKey: 'c' }, problem: '' }, sample, scopeKey: 'a' },
      { candidate: { definition, problem: '' }, sample: { ...sample, resourceId: 'cache' }, scopeKey: 'a' },
      { candidate: { definition, problem: '' }, sample, scopeKey: 'b' },
    ]) {
      rerender(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={sample} scopeKey="a" />);
      await user.click(run());
      await screen.findByRole('region', { name: 'Kết quả matching chính xác' });
      rerender(<MatchingPreviewPanel {...next} />);
      expect(screen.queryByRole('region', { name: 'Kết quả matching chính xác' })).not.toBeInTheDocument();
    }
  });

  it('ignores a late reply of an older request after the input changed', async () => {
    let release: (response: Response) => void = () => undefined;
    stub(() => new Promise<Response>((resolve) => { release = resolve; }));
    const { rerender } = render(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={sample} />);
    await userEvent.setup().click(run());
    expect(await screen.findByText('Đang kiểm tra matching…')).toBeInTheDocument();
    rerender(<MatchingPreviewPanel candidate={{ definition: { ...definition, key: 'pg-changed' }, problem: '' }} sample={sample} />);
    expect(screen.queryByText('Đang kiểm tra matching…')).not.toBeInTheDocument();
    await act(async () => { release(Response.json(tie)); await new Promise((resolve) => setTimeout(resolve, 20)); });
    expect(screen.queryByRole('region', { name: 'Kết quả matching chính xác' })).not.toBeInTheDocument();
    expect(run()).toBeEnabled();
  });

  it('ignores a late failure after the session expired', async () => {
    let fail: (reason: unknown) => void = () => undefined;
    stub(() => new Promise<Response>((_, reject) => { fail = reject; }));
    render(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={sample} />);
    await userEvent.setup().click(run());
    await screen.findByText('Đang kiểm tra matching…');
    act(() => { window.dispatchEvent(new Event('orchestrator:session-expired')); });
    await act(async () => { fail(new TypeError('network')); await new Promise((resolve) => setTimeout(resolve, 20)); });
    expect(screen.queryByRole('alert', { name: 'Lỗi kiểm tra matching' })).not.toBeInTheDocument();
  });
});

describe('exact matching preview in the template form (T19)', () => {
  it('previews the unsaved template without registering it and keeps the frontend hint separate', async () => {
    const user = userEvent.setup();
    const posts: { url: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (init?.method === 'POST') { posts.push({ url, body: JSON.parse(String(init.body)) as Record<string, unknown> }); return Response.json({ status: 'NATIVE_RENDERER', candidates: [], snapshot: { marker: 'm1', environmentVersion: 2 } }); }
      if (url.endsWith('/applications')) return Response.json({ applications: [{ key: 'shop', name: 'shop', environments: [{ key: 'staging', name: 'staging', environmentType: 'staging', executionProfile: 'internal-k8s' }] }] });
      if (url.endsWith('/render-bundles')) return Response.json({ renderBundles: [{ id: 'bundle-web', version: '1', status: 'AVAILABLE' }] });
      return Response.json({ resourceDefinitions: [] });
    }));
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    expect(screen.getByText('Nhập ID mẫu hợp lệ để kiểm tra matching chính xác.')).toBeInTheDocument();
    await user.type(screen.getByLabelText('ID mẫu'), 'web-tpl');
    await user.selectOptions(screen.getByLabelText('Ứng dụng mẫu'), 'shop');
    await user.selectOptions(screen.getByLabelText('Môi trường mẫu'), 'staging');
    await user.type(screen.getByLabelText('ID tài nguyên mẫu (res_id)'), 'modules.web');
    await user.type(screen.getByLabelText('Class mẫu'), 'default');
    await waitFor(() => expect(run()).toBeEnabled());
    expect(screen.getByRole('group', { name: 'Xem trước phạm vi (trợ giúp ở giao diện)' })).toBeInTheDocument();
    await user.click(run());
    expect(await screen.findByText('Không có mẫu khớp: dùng renderer mặc định')).toBeInTheDocument();
    expect(posts).toHaveLength(1);
    expect(posts[0]?.url).toBe('/api/v1/resource-definitions/matching-preview');
    expect(posts[0]?.body).toMatchObject({ definition: { key: 'web-tpl', resourceType: 'workload', driverType: 'score-k8s', executionProfile: 'internal-k8s' }, context: { applicationId: 'shop', environmentId: 'staging', resourceId: 'modules.web', class: 'default' } });
    expect(screen.queryByText(/Đã đăng ký mẫu/)).not.toBeInTheDocument();
    await user.type(screen.getByLabelText('ID mẫu'), 'x');
    expect(screen.queryByRole('region', { name: 'Kết quả matching chính xác' })).not.toBeInTheDocument();
  });
});

describe('exact matching preview invalidation by context metadata (T19)', () => {
  const app = (snapshotKey: string): ApplicationOption[] => [{ key: 'shop', name: 'shop', environments: [{ key: 'staging', name: 'staging', environmentType: 'staging', executionProfile: 'internal-k8s', snapshotKey }] }];
  const editor = (apps: ApplicationOption[], loading = false, defs = [{ key: 'pg', resourceType: 'postgres', criteria: [{}] }]) => <CriteriaEditor draft={emptyDraft()} onChange={() => undefined}
    applications={{ items: apps, loading, error: '', onRetry: () => undefined }} sources={{ items: defs, loading: false, error: '', onRetry: () => undefined }}
    matching={{ definition, problem: '' }} />;

  async function ranResult() {
    stub(() => Response.json(tie));
    const view = render(editor(app('v1')));
    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText('Ứng dụng mẫu'), 'shop');
    await user.selectOptions(screen.getByLabelText('Môi trường mẫu'), 'staging');
    await user.click(run());
    await screen.findByRole('region', { name: 'Kết quả matching chính xác' });
    return view;
  }
  const gone = () => expect(screen.queryByRole('region', { name: 'Kết quả matching chính xác' })).not.toBeInTheDocument();

  it('drops the result when the Environment target/version changed but keys, type and profile did not', async () => {
    const view = await ranResult();
    view.rerender(editor(app('v2')));
    gone();
  });
  it('drops the result when the application list reloads with identical data and keeps the draft', async () => {
    const view = await ranResult();
    view.rerender(editor(app('v1'), true));
    gone();
    view.rerender(editor(app('v1'), false));
    gone();
    expect(screen.getByLabelText('ID tài nguyên mẫu (res_id)')).toBeInTheDocument();
  });
  it('drops the result when the catalog sources change', async () => {
    const view = await ranResult();
    view.rerender(editor(app('v1'), false, [{ key: 'pg', resourceType: 'postgres', criteria: [{ class: 'fast' }] }]));
    gone();
  });
});
