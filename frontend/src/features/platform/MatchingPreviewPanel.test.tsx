import { within, act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MatchingPreviewPanel, type MatchingSample } from './MatchingPreviewPanel';
import { CriteriaEditor, emptyDraft, type ApplicationOption, type CriteriaSource } from './CriteriaEditor';
import { RenderingTemplatesPage } from './RenderingTemplatesPage';

const sample: MatchingSample = { applicationId: 'shop', environmentId: 'staging', resourceId: 'db', class: 'default' };
const definition = { key: 'pg-new', name: 'PG mới', resourceType: 'postgres', criteria: [{}] };
const tie = { status: 'AMBIGUOUS', missingFields: [], candidates: [{ key: 'pg-new', name: 'PG mới', matched: true, score: 0, bestCriterion: {} }, { key: 'pg-shared', name: 'PG chung', matched: true, score: 0, bestCriterion: {} }, { key: 'pg-aws', name: 'PG AWS', matched: false }], ties: ['pg-new', 'pg-shared'], snapshot: { marker: 'abcdef0123456789', environmentVersion: 3 } };

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
    expect(await screen.findByText(/Hòa điểm: nhiều cấu hình cùng điểm cao nhất, chưa có cấu hình thắng/)).toBeInTheDocument();
    expect(calls).toEqual([{ definition, context: { applicationId: 'shop', environmentId: 'staging', resourceId: 'db', class: 'default' } }]);
    expect(screen.getByText('pg-new, pg-shared')).toBeInTheDocument();
    expect(screen.getByRole('list', { name: 'Chi tiết từng cấu hình' }).textContent).toMatch(/PG AWS \(pg-aws\): không khớp context/);
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

  it('does not call candidates unmatched when the context is incomplete', async () => {
    const candidates = [{ key: 'pg-new', name: 'PG mới', matched: false }, { key: 'pg-aws', name: 'PG AWS', matched: false }];
    stub(() => Response.json({ status: 'INCOMPLETE', missingFields: ['class'], candidates, snapshot: { marker: 'm', environmentVersion: 1 } }));
    const first = render(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={sample} />);
    await userEvent.setup().click(run());
    const list = await screen.findByRole('list', { name: 'Chi tiết từng cấu hình' });
    expect(list.textContent).toMatch(/PG mới \(pg-new\): chưa đánh giá do thiếu context/);
    expect(list.textContent).not.toMatch(/không khớp context/);
    first.unmount();
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
    const calls = stub(() => (fail ? Response.json({ error: 'internal failure with secret' }, { status: 500 }) : Response.json({ status: 'MATCHED', winner: { key: 'pg-new', name: 'PG mới', driverType: 'terraform', score: 0 }, candidates: [], snapshot: { marker: 'm', environmentVersion: 1 } })));
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
    expect(await screen.findByText('Có đúng một cấu hình thắng')).toBeInTheDocument();
    expect(calls).toHaveLength(2);
  });

  it('maps validation, scope and target statuses to Vietnamese transport messages', async () => {
    for (const [status, text] of [[400, /từ chối cấu hình/], [404, /Không tìm thấy ứng dụng/], [403, /không có quyền/]] as const) {
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
      { candidate: { definition: { ...definition, driverAccount: 'c' }, problem: '' }, sample, scopeKey: 'a' },
      { candidate: { definition: { ...definition, name: 'Tên khác' }, problem: '' }, sample, scopeKey: 'a' },
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
      if (init?.method === 'POST') { posts.push({ url, body: JSON.parse(String(init.body)) as Record<string, unknown> }); return Response.json({ status: 'NO_MATCH', candidates: [], snapshot: { marker: 'm1', environmentVersion: 2 } }); }
      if (url.endsWith('/applications')) return Response.json({ applications: [{ key: 'shop', name: 'shop', environments: [{ key: 'staging', name: 'staging', environmentType: 'staging' }] }] });
      if (url.endsWith('/render-bundles')) return Response.json({ renderBundles: [{ id: 'bundle-web', version: '1', status: 'AVAILABLE' }] });
      return Response.json({ resourceDefinitions: [] });
    }));
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    expect(screen.getByText('Nhập ID mẫu hợp lệ để kiểm tra matching chính xác.')).toBeInTheDocument();
    await user.type(screen.getByLabelText('ID mẫu'), 'web-tpl');
    await user.type(screen.getByLabelText('Tên hiển thị'), 'Mẫu web');
    await user.selectOptions(screen.getByLabelText('Ứng dụng kiểm tra'), 'shop');
    await user.selectOptions(screen.getByLabelText('Môi trường kiểm tra'), 'staging');
    await user.type(screen.getByLabelText('ID tài nguyên kiểm tra (res_id)'), 'modules.web');
    await user.type(screen.getByLabelText('Class kiểm tra'), 'default');
    await waitFor(() => expect(run()).toBeEnabled());
    expect(screen.queryByRole('group', { name: /Xem trước phạm vi/ })).not.toBeInTheDocument();
    await user.click(run());
    expect(await screen.findByText('Không có cấu hình nào khớp (khác với hòa điểm)')).toBeInTheDocument();
    expect(within(screen.getByRole('region', { name: 'Kết quả matching chính xác' })).queryByText(/renderer mặc định/)).not.toBeInTheDocument();
    expect(posts).toHaveLength(1);
    expect(posts[0]?.url).toBe('/api/v1/resource-definitions/matching-preview');
    expect(posts[0]?.body).toMatchObject({ definition: { key: 'web-tpl', name: 'Mẫu web', resourceType: 'workload', driverType: 'score-k8s' }, context: { applicationId: 'shop', environmentId: 'staging', resourceId: 'modules.web', class: 'default' } });
    expect(posts[0]?.body.definition).not.toHaveProperty('executionProfile');
    expect(screen.queryByText(/Đã đăng ký mẫu/)).not.toBeInTheDocument();
    await user.type(screen.getByLabelText('ID mẫu'), 'x');
    expect(screen.queryByRole('region', { name: 'Kết quả matching chính xác' })).not.toBeInTheDocument();
  });

  it('drops the result when only the availability metadata of the installed bundles changed', async () => {
    const user = userEvent.setup();
    let bundleVersion = '1';
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (init?.method === 'POST') return Response.json({ status: 'NO_MATCH', candidates: [], snapshot: { marker: 'm1', environmentVersion: 2 } });
      if (url.endsWith('/applications')) return Response.json({ applications: [{ key: 'shop', name: 'shop', environments: [{ key: 'staging', name: 'staging', environmentType: 'staging' }] }] });
      if (url.endsWith('/render-bundles')) return Response.json({ renderBundles: [{ id: 'bundle-web', version: bundleVersion, status: 'AVAILABLE' }] });
      return Response.json({ resourceDefinitions: [] });
    }));
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await user.type(screen.getByLabelText('ID mẫu'), 'web-tpl');
    await user.type(screen.getByLabelText('Tên hiển thị'), 'Mẫu web');
    await user.selectOptions(screen.getByLabelText('Ứng dụng kiểm tra'), 'shop');
    await user.selectOptions(screen.getByLabelText('Môi trường kiểm tra'), 'staging');
    await user.type(screen.getByLabelText('ID tài nguyên kiểm tra (res_id)'), 'modules.web');
    await user.type(screen.getByLabelText('Class kiểm tra'), 'default');
    await waitFor(() => expect(run()).toBeEnabled());
    await user.click(run());
    await screen.findByRole('region', { name: 'Kết quả matching chính xác' });
    bundleVersion = '2';
    await user.click(screen.getByRole('button', { name: 'Làm mới danh sách bundle' }));
    await waitFor(() => expect(screen.queryByRole('region', { name: 'Kết quả matching chính xác' })).not.toBeInTheDocument());
  });
});

describe('exact matching preview shows the safe winner, account, region and conflicts (UC-03 V02-05)', () => {
  it('shows the winner Name, ID, version, account and effective region and never a secret field', async () => {
    stub(() => Response.json({ status: 'MATCHED', missingFields: [], candidates: [{ key: 'pg-aws', name: 'PG AWS', matched: true, score: 16, bestCriterion: { class: 'fast' } }],
      winner: { key: 'pg-aws', name: 'PG AWS', version: 5, driverType: 'terraform', score: 16, account: { key: 'aws-account', name: 'AWS chính', type: 'AWS', version: 3 }, region: 'ap-southeast-1' }, snapshot: { marker: 'mm', environmentVersion: 2 } }));
    render(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={sample} />);
    await userEvent.setup().click(run());
    const winner = await screen.findByRole('group', { name: 'Cấu hình thắng' });
    expect(winner).toHaveTextContent('PG AWS (ID pg-aws, phiên bản 5, điểm 16)');
    expect(winner).toHaveTextContent('Tài khoản: AWS chính (aws-account, AWS, phiên bản 3); vùng hiệu lực: ap-southeast-1');
    expect(screen.queryByText(/secret|credential/i)).not.toBeInTheDocument();
  });

  it('shows resource cluster provenance and safely labels retired Environment targets', async () => {
    for (const [clusterSource, expected] of [[{ kind: 'RESOURCE', reference: 'k8s-cluster.eks#environments.@app.@env' }, /tài nguyên k8s-cluster.eks#environments.@app.@env/], [{ kind: 'LEGACY_ENVIRONMENT' }, /Nguồn cluster không được hỗ trợ/]] as const) {
      stub(() => Response.json({ status: 'MATCHED', missingFields: [], candidates: [], winner: { key: 'pg-k8s', name: 'PG k8s', driverType: 'terraform', score: 0, clusterSource }, snapshot: { marker: 'mm', environmentVersion: 2 } }));
      const { unmount } = render(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={sample} />);
      await userEvent.setup().click(run());
      expect(await screen.findByRole('group', { name: 'Cấu hình thắng' })).toHaveTextContent(expected);
      expect(screen.queryByText(/Tài khoản:/)).not.toBeInTheDocument();
      unmount();
    }
  });

  it('distinguishes no match, a tie and an account conflict, and offers no fallback winner', async () => {
    const cases: [Record<string, unknown>, RegExp][] = [
      [{ status: 'NO_MATCH', candidates: [] }, /Không có cấu hình nào khớp \(khác với hòa điểm\)/],
      [{ status: 'AMBIGUOUS', candidates: [], ties: ['a', 'b'] }, /Hòa điểm/],
      [{ status: 'CONNECTION_CONFLICT', candidates: [], winner: { key: 'pg-aws', name: 'PG AWS', driverType: 'terraform', score: 16 }, connectionConflict: { definitionKey: 'pg-aws', driverAccount: 'gone', reason: 'ACCOUNT_NOT_FOUND' } }, /Xung đột tài khoản của cấu hình thắng pg-aws \(tài khoản gone\): không tìm thấy tài khoản trong Tổ chức\. Hệ thống không chọn cấu hình thay thế/],
    ];
    for (const [body, expected] of cases) {
      stub(() => Response.json({ missingFields: [], snapshot: { marker: 'mm', environmentVersion: 2 }, ...body }));
      const { unmount } = render(<MatchingPreviewPanel candidate={{ definition, problem: '' }} sample={sample} />);
      await userEvent.setup().click(run());
      expect(await screen.findByRole('region', { name: 'Kết quả matching chính xác' })).toHaveTextContent(expected);
      unmount();
    }
  });
});

describe('exact matching preview invalidation by context metadata (T19)', () => {
  const app = (snapshotKey: string): ApplicationOption[] => [{ key: 'shop', name: 'shop', environments: [{ key: 'staging', name: 'staging', environmentType: 'staging', snapshotKey }] }];
  const editor = (apps: ApplicationOption[], loading = false, defs: CriteriaSource[] = [{ key: 'pg', resourceType: 'postgres', criteria: [{}] }]) => <CriteriaEditor draft={emptyDraft()} onChange={() => undefined}
    applications={{ items: apps, loading, error: '', onRetry: () => undefined }} sources={{ items: defs, loading: false, error: '', onRetry: () => undefined }}
    matching={{ definition, problem: '' }} />;

  async function ranResult() {
    stub(() => Response.json(tie));
    const view = render(editor(app('v1')));
    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText('Ứng dụng kiểm tra'), 'shop');
    await user.selectOptions(screen.getByLabelText('Môi trường kiểm tra'), 'staging');
    await user.click(run());
    await screen.findByRole('region', { name: 'Kết quả matching chính xác' });
    return view;
  }
  const gone = () => expect(screen.queryByRole('region', { name: 'Kết quả matching chính xác' })).not.toBeInTheDocument();

  it('drops the result when the Environment configuration version changed while matching inputs stayed the same', async () => {
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
    expect(screen.getByLabelText('ID tài nguyên kiểm tra (res_id)')).toBeInTheDocument();
  });
  it('drops the result when only matching-relevant metadata of a source changed (version, account, region, name), not just its criteria', async () => {
    const same = { key: 'pg', name: 'PG', version: 1, driverAccount: 'aws-account', driverInputs: { values: { variables: { region: 'us-east-1' } } }, resourceType: 'postgres', criteria: [{}] };
    for (const change of [{ version: 2 }, { driverAccount: 'aws-second' }, { driverInputs: { values: { variables: { region: 'eu-west-1' } } } }, { name: 'PG đổi tên' }]) {
      const view = await ranResult2(same);
      view.rerender(editor(app('v1'), false, [{ ...same, ...change }]));
      gone();
      view.unmount();
    }
  });
  async function ranResult2(source: Record<string, unknown>) {
    stub(() => Response.json(tie));
    const view = render(editor(app('v1'), false, [source as CriteriaSource]));
    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText('Ứng dụng kiểm tra'), 'shop');
    await user.selectOptions(screen.getByLabelText('Môi trường kiểm tra'), 'staging');
    await user.click(run());
    await screen.findByRole('region', { name: 'Kết quả matching chính xác' });
    return view;
  }
  it('ignores a late reply that arrives after a metadata-only source change', async () => {
    const source = { key: 'pg', name: 'PG', version: 1, resourceType: 'postgres', criteria: [{}] };
    let release: (response: Response) => void = () => undefined;
    stub(() => new Promise<Response>((resolve) => { release = resolve; }));
    const view = render(editor(app('v1'), false, [source]));
    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText('Ứng dụng kiểm tra'), 'shop');
    await user.selectOptions(screen.getByLabelText('Môi trường kiểm tra'), 'staging');
    await user.click(run());
    await screen.findByText('Đang kiểm tra matching…');
    view.rerender(editor(app('v1'), false, [{ ...source, version: 2 }]));
    await act(async () => { release(Response.json(tie)); await new Promise((resolve) => setTimeout(resolve, 20)); });
    gone();
  });
  it('drops the result when the catalog sources change', async () => {
    const view = await ranResult();
    view.rerender(editor(app('v1'), false, [{ key: 'pg', resourceType: 'postgres', criteria: [{ class: 'fast' }] }]));
    gone();
  });
});
