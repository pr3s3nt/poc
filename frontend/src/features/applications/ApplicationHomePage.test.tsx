import { act, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { ApplicationHomePage } from './ApplicationHomePage';
import type { Application } from '../../shared/types/application';
import { bothConfigured, configuredTarget } from '../../test/targets';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: bothConfigured(), workloads: { staging: [], production: [] } };

afterEach(() => vi.unstubAllGlobals());

it('previews pending changes and deploys only after confirmation', async () => {
  let deployToken = '';
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads')) return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', draftVersion: 1, workloads: [{ id: 'frontend', state: 'PENDING_UPSERT', ready: false, score: {} }] });
    if (url.endsWith('/preview')) return Response.json({ token: 'pinned-preview', draftVersion: 1, configRevisionId: 'revision-1', changes: [{ workloadId: 'frontend', action: 'DEPLOY', resources: { existing: [], new: [], unreferenced: [] }, planHash: 'plan-1' }] });
    if (url.endsWith('/deploy')) {
      deployToken = (JSON.parse(String(init?.body)) as { token: string }).token;
      return Response.json({ status: 'SUCCEEDED', results: [{ workloadId: 'frontend', action: 'DEPLOY', status: 'SUCCEEDED' }] });
    }
    return Response.json({}, { status: 404 });
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('frontend');
  expect(screen.queryByRole('button', { name: 'Triển khai các thay đổi này' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  expect(await screen.findByRole('heading', { name: 'Xem trước cho staging' })).toBeInTheDocument();
  expect(deployToken).toBe('');
  expect(within(screen.getByLabelText('Xem trước triển khai')).getByLabelText('Đích triển khai')).toHaveTextContent('staging · Đích được xác định từ Resource Definition và các tham chiếu Workload.');
  await user.click(screen.getByRole('button', { name: 'Triển khai các thay đổi này' }));
  expect(await screen.findByRole('heading', { name: 'Triển khai thành công' })).toBeInTheDocument();
  expect(within(screen.getByLabelText('Kết quả triển khai')).getByLabelText('Đích triển khai')).toHaveTextContent('staging · Đích được xác định từ Resource Definition và các tham chiếu Workload.');
  expect(deployToken).toBe('pinned-preview');
});

it('offers route-only retry when workloads have already been applied', async () => {
  let deployCalls = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/workloads')) return Response.json({ draftVersion: 2, workloads: [{ id: 'frontend', state: '', ready: true, score: {} }] });
    if (url.endsWith('/preview')) return Response.json({ token: 'route-retry', draftVersion: 2, routePending: true, changes: [] });
    if (url.endsWith('/deploy')) { deployCalls += 1; return Response.json({ status: 'SUCCEEDED', results: [] }); }
    return Response.json({}, { status: 404 });
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('frontend');
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  expect(await screen.findByText('Đường dẫn công khai cần được đồng bộ; Workload sẽ không khởi động lại.')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Thử lại đường dẫn công khai' }));
  expect(await screen.findByRole('heading', { name: 'Triển khai thành công' })).toBeInTheDocument();
  expect(deployCalls).toBe(1);
});

const list = (env: string, workloads: unknown[], draftVersion = 1) => Response.json({ applicationKey: 'catalog', environmentKey: env, draftVersion, workloads });

it('confirms deletion by name and Environment, disables duplicate actions while busy, and undoes', async () => {
  let release: () => void = () => undefined;
  let deletes = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads/api') && init?.method === 'DELETE') {
      deletes += 1;
      await new Promise<void>((done) => { release = done; });
      return list('staging', [{ id: 'api', state: 'PENDING_DELETE', ready: true }], 2);
    }
    if (url.endsWith('/workloads/api/undo')) return list('staging', [{ id: 'api', state: '', ready: true, score: {} }], 3);
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.includes('/resources')) return Response.json({ resources: [] });
    return list('staging', [{ id: 'api', state: '', ready: true, score: {} }]);
  }));
  const confirm = vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValueOnce(true);
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xóa' }));
  expect(confirm).toHaveBeenLastCalledWith('Đánh dấu xóa api trong staging? Workload đang chạy chỉ thay đổi sau khi Triển khai.');
  expect(deletes).toBe(0);
  await user.click(screen.getByRole('button', { name: 'Xóa' }));
  expect(await screen.findByRole('button', { name: 'Đang đánh dấu…' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Xem trước thay đổi' })).toBeDisabled();
  release();
  await user.click(await screen.findByRole('button', { name: 'Hoàn tác' }));
  expect(await screen.findByText('Sẵn sàng')).toBeInTheDocument();
  expect(deletes).toBe(1);
  confirm.mockRestore();
});

it('reloads the list on a version conflict and never retries the mutation', async () => {
  let deletes = 0;
  let lists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'DELETE') { deletes += 1; return Response.json({ error: 'workloads changed since they were loaded; reload and try again' }, { status: 409 }); }
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.includes('/resources')) return Response.json({ resources: [] });
    lists += 1;
    return list('staging', [{ id: 'api', state: '', ready: true, score: {} }, ...(lists > 1 ? [{ id: 'web', state: 'PENDING_UPSERT', ready: false, score: {} }] : [])], lists);
  }));
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xóa' }));
  expect(await screen.findByText(/đã thay đổi kể từ khi trang được tải/)).toBeInTheDocument();
  expect(await screen.findByText('web')).toBeInTheDocument();
  expect(deletes).toBe(1);
  vi.restoreAllMocks();
});

it('keeps the deploy outcome when the list reload fails and blocks mutations until a retry succeeds', async () => {
  let lists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.includes('/resources')) return Response.json({ resources: [] });
    if (url.endsWith('/preview')) return Response.json({ token: 't', draftVersion: 1, changes: [{ workloadId: 'api', action: 'UPDATE', resources: { existing: [], new: [], unreferenced: [] }, planHash: 'p' }] });
    if (url.endsWith('/deploy')) return Response.json({ status: 'SUCCEEDED', results: [{ workloadId: 'api', action: 'UPDATE', status: 'SUCCEEDED', deploymentId: 'dep-1' }] });
    lists += 1;
    if (lists === 2) return Response.json({ error: 'unavailable' }, { status: 500 });
    return list('staging', [{ id: 'api', state: lists === 1 ? 'PENDING_UPSERT' : '', ready: true, score: {} }], lists);
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  await user.click(await screen.findByRole('button', { name: 'Triển khai các thay đổi này' }));
  expect(await screen.findByRole('heading', { name: 'Triển khai thành công' })).toBeInTheDocument();
  expect(await screen.findByText(/Không thể tải lại danh sách Workload/)).toBeInTheDocument();
  expect(screen.getByRole('heading', { name: 'Triển khai thành công' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Xem trước thay đổi' })).toBeDisabled();
  expect(screen.queryByRole('button', { name: 'Xóa' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Thử lại' }));
  expect(await screen.findByText('Sẵn sàng')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Xem trước thay đổi' })).toBeEnabled();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
});

it('keeps controls busy until the post-deploy reload finishes', async () => {
  let lists = 0;
  let releaseReload: (value: Response) => void = () => undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.includes('/resources')) return Response.json({ resources: [] });
    if (url.endsWith('/preview')) return Response.json({ token: 't', draftVersion: 1, changes: [{ workloadId: 'api', action: 'UPDATE', resources: { existing: [], new: [], unreferenced: [] }, planHash: 'p' }] });
    if (url.endsWith('/deploy')) return Response.json({ status: 'SUCCEEDED', results: [{ workloadId: 'api', action: 'UPDATE', status: 'SUCCEEDED' }] });
    lists += 1;
    if (lists === 2) return new Promise<Response>((done) => { releaseReload = done; });
    return list('staging', [{ id: 'api', state: 'PENDING_UPSERT', ready: true, score: {} }], 1);
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  await user.click(await screen.findByRole('button', { name: 'Triển khai các thay đổi này' }));
  expect(await screen.findByRole('heading', { name: 'Triển khai thành công' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Xóa' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Xem trước thay đổi' })).toBeDisabled();
  releaseReload(list('staging', [{ id: 'api', state: '', ready: true, score: {} }], 2));
  expect(await screen.findByText('Sẵn sàng')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Xóa' })).toBeEnabled();
});

it('does not show a staging conflict notice after switching to production', async () => {
  let releaseReload: (value: Response) => void = () => undefined;
  let stagingLists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'DELETE') return Response.json({ error: 'conflict' }, { status: 409 });
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.includes('/resources')) return Response.json({ resources: [] });
    if (url.includes('/production/')) return list('production', [{ id: 'prod-only', state: '', ready: true, score: {} }]);
    stagingLists += 1;
    if (stagingLists === 2) return new Promise<Response>((done) => { releaseReload = done; });
    return list('staging', [{ id: 'api', state: '', ready: true, score: {} }]);
  }));
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xóa' }));
  await user.click(screen.getByRole('button', { name: /^production/ }));
  await screen.findByText('prod-only');
  releaseReload(list('staging', [{ id: 'api', state: '', ready: true, score: {} }], 2));
  await new Promise((done) => setTimeout(done, 20));
  expect(screen.queryByText(/đã thay đổi kể từ khi trang được tải/)).not.toBeInTheDocument();
  expect(screen.getByText('prod-only')).toBeInTheDocument();
  vi.restoreAllMocks();
});

it('reports truthfully when the reload after a conflict fails', async () => {
  let lists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'DELETE') return Response.json({ error: 'conflict' }, { status: 409 });
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.includes('/resources')) return Response.json({ resources: [] });
    lists += 1;
    if (lists === 2) return Response.json({ error: 'unavailable' }, { status: 500 });
    return list('staging', [{ id: 'api', state: '', ready: true, score: {} }], lists);
  }));
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xóa' }));
  expect(await screen.findByText(/Không thể tải lại danh sách Workload/)).toBeInTheDocument();
  expect(screen.queryByText(/Danh sách đã được tải lại/)).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Xem trước thay đổi' })).toBeDisabled();
  vi.restoreAllMocks();
});

it('ignores a late Preview after switching Environment and blocks switching during Deploy', async () => {
  let resolvePreview: (value: Response) => void = () => undefined;
  let resolveDeploy: (value: Response) => void = () => undefined;
  let previews = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.includes('/resources')) return Response.json({ resources: [] });
    if (url.endsWith('/preview')) {
      previews += 1;
      if (previews === 1) return new Promise<Response>((done) => { resolvePreview = done; });
      return Response.json({ token: 't', draftVersion: 1, changes: [{ workloadId: 'api', action: 'REMOVE', resources: { existing: [], new: [], unreferenced: ['postgres.default#shared.db'] }, planHash: 'p' }] });
    }
    if (url.endsWith('/deploy')) return new Promise<Response>((done) => { resolveDeploy = done; });
    return list(url.includes('/production/') ? 'production' : 'staging', [{ id: url.includes('/production/') ? 'prod-only' : 'api', state: 'PENDING_DELETE', ready: true }]);
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  await user.click(screen.getByRole('button', { name: /^production/ }));
  await screen.findByText('prod-only');
  resolvePreview(Response.json({ token: 'staging-token', draftVersion: 1, changes: [{ workloadId: 'api', action: 'REMOVE', resources: { existing: [], new: [], unreferenced: [] }, planHash: 'x' }] }));
  await new Promise((done) => setTimeout(done, 20));
  expect(screen.queryByLabelText('Xem trước triển khai')).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  expect(await screen.findByText(/1 tài nguyên sẽ không còn được tham chiếu \(không bị hủy\)/)).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Triển khai các thay đổi này' }));
  expect(screen.getByRole('button', { name: /^staging/ })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Đang triển khai…' })).toBeDisabled();
  resolveDeploy(Response.json({ status: 'SUCCEEDED', results: [{ workloadId: 'api', action: 'REMOVE', status: 'SUCCEEDED', deploymentId: 'dep-9' }] }));
  expect(await screen.findByRole('link', { name: 'xem triển khai' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /^staging/ })).toBeEnabled();
});

it('shows the target of the selected Environment only and switches with the tab', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ draftVersion: 0, workloads: [], deployments: [], resources: [] })));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={{ ...application, environments: { staging: configuredTarget(), production: configuredTarget() } }} />);
  expect(screen.getAllByLabelText('Đích triển khai')[0]).toHaveTextContent('staging · Đích được xác định từ Resource Definition và các tham chiếu Workload.');
  await screen.findByText('Môi trường này chưa có Workload nào.');
  await user.click(screen.getByRole('button', { name: /^production/ }));
  expect(screen.getAllByLabelText('Đích triển khai')[0]).toHaveTextContent('production · Đích được xác định từ Resource Definition và các tham chiếu Workload.');
  expect(screen.getAllByLabelText('Đích triển khai')[0]).not.toHaveTextContent('Lab');
});


it('disables draft Delete and Undo while an environment operation runs and enables them after release', async () => {
  const calls: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method && init.method !== 'GET') calls.push(`${init.method} ${url}`);
    if (url.endsWith('/workloads')) return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', draftVersion: 1, workloads: [{ id: 'frontend', ready: true, score: {} }, { id: 'old', state: 'PENDING_DELETE', ready: true }] });
    return Response.json({}, { status: 404 });
  }));
  const busyApp: Application = { ...application, environments: { ...application.environments, staging: { ...application.environments.staging, activeOperation: { id: 'op1', kind: 'DEPLOY', status: 'ACTIVE', stage: 'DEPLOY web', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: false } } } };
  const view = render(<ApplicationHomePage application={application} />);
  await screen.findByText('frontend');
  expect(screen.getByRole('button', { name: 'Xóa' })).toBeEnabled();
  view.rerender(<ApplicationHomePage application={busyApp} />);
  expect(screen.getByRole('button', { name: 'Xóa' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Hoàn tác' })).toBeDisabled();
  expect(screen.getByText(/Thay đổi bản nháp tạm dừng/)).toBeInTheDocument();
  view.rerender(<ApplicationHomePage application={application} />);
  expect(screen.getByRole('button', { name: 'Xóa' })).toBeEnabled();
  expect(screen.getByRole('button', { name: 'Hoàn tác' })).toBeEnabled();
  expect(calls).toEqual([]);
});

const pendingList = () => list('staging', [{ id: 'api', state: 'PENDING_UPSERT', ready: true, score: {} }]);

it('shows only a safe Vietnamese message when Preview fails with a raw provider error', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.includes('/resources')) return Response.json({ resources: [] });
    if (url.endsWith('/preview')) return Response.json({ error: 'provider denied key AKIA-SECRET-123 value hunter2' }, { status: 500 });
    return pendingList();
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent('Không thể xem trước thay đổi.');
  expect(document.body).not.toHaveTextContent(/AKIA-SECRET|hunter2/);
  expect(screen.queryByLabelText('Xem trước triển khai')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Mở Cài đặt môi trường' })).not.toBeInTheDocument();
});

it('shows a safe Vietnamese message when the first list load fails', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => String(input).includes('/deployments') ? Response.json({ deployments: [] }) : String(input).includes('/resources') ? Response.json({ resources: [] }) : Response.json({ error: 'password=hunter2' }, { status: 500 })));
  render(<ApplicationHomePage application={application} />);
  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent('Không thể tải danh sách Workload của staging.');
  expect(document.body).not.toHaveTextContent('hunter2');
  expect(screen.getByRole('button', { name: 'Thử lại' })).toBeInTheDocument();
});

it('reports a partial Deploy per Workload without raw errors and retries through a fresh Preview', async () => {
  const calls: string[] = [];
  let previews = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'POST') calls.push(url.split('/').pop() ?? '');
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.includes('/resources')) return Response.json({ resources: [] });
    if (url.endsWith('/preview')) { previews += 1; return Response.json({ token: `tok-${previews}`, draftVersion: 1, changes: [{ workloadId: 'api', action: 'UPDATE', resources: { existing: [], new: [], unreferenced: [] }, planHash: 'p' }] }); }
    if (url.endsWith('/deploy')) return Response.json({ status: 'PARTIAL', results: [{ workloadId: 'web', action: 'DEPLOY', status: 'SUCCEEDED', deploymentId: 'dep-web' }, { workloadId: 'api', action: 'UPDATE', status: 'FAILED', error: 'helm: token=abc123 refused' }] });
    return pendingList();
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  await user.click(await screen.findByRole('button', { name: 'Triển khai các thay đổi này' }));
  const result = await screen.findByLabelText('Kết quả triển khai');
  expect(within(result).getByRole('heading', { name: 'Triển khai một phần' })).toBeInTheDocument();
  expect(result).toHaveTextContent('web · triển khai: thành công');
  expect(result).toHaveTextContent('api · cập nhật: thất bại');
  expect(result).toHaveTextContent('Thay đổi chưa hoàn tất vẫn ở trạng thái chờ');
  expect(within(result).getAllByRole('link', { name: 'xem triển khai' })).toHaveLength(1);
  expect(document.body).not.toHaveTextContent(/abc123|helm/);
  expect(screen.queryByRole('button', { name: 'Triển khai các thay đổi này' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  await user.click(await screen.findByRole('button', { name: 'Triển khai các thay đổi này' }));
  expect(calls.filter((call) => call === 'preview')).toHaveLength(2);
  expect(calls.filter((call) => call === 'deploy')).toHaveLength(2);
});

it('drops a stale Preview on a 409 Deploy, reloads the list and does not redeploy', async () => {
  let deploys = 0;
  let lists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.includes('/resources')) return Response.json({ resources: [] });
    if (url.endsWith('/preview')) return Response.json({ token: 'old', draftVersion: 1, changes: [{ workloadId: 'api', action: 'UPDATE', resources: { existing: [], new: [], unreferenced: [] }, planHash: 'p' }] });
    if (url.endsWith('/deploy')) { deploys += 1; return Response.json({ error: 'preview token is stale: raw detail xyz' }, { status: 409 }); }
    lists += 1;
    return list('staging', [{ id: 'api', state: 'PENDING_UPSERT', ready: true, score: {} }], lists);
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  await user.click(await screen.findByRole('button', { name: 'Triển khai các thay đổi này' }));
  expect(await screen.findByText(/hãy kiểm tra và xem trước lại/)).toBeInTheDocument();
  expect(screen.queryByLabelText('Xem trước triển khai')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Triển khai các thay đổi này' })).not.toBeInTheDocument();
  expect(document.body).not.toHaveTextContent('xyz');
  expect(deploys).toBe(1);
  expect(lists).toBe(2);
});

it('keeps Deploy unavailable for a no-op Preview', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.includes('/resources')) return Response.json({ resources: [] });
    if (url.endsWith('/preview')) return Response.json({ token: 'noop', draftVersion: 1, changes: [] });
    return list('staging', [{ id: 'api', state: '', ready: true, score: {} }]);
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  expect(await screen.findByText('Không có thay đổi Workload nào để triển khai.')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Triển khai các thay đổi này' })).toBeDisabled();
});

it('ignores a late Deploy result after the same page is rerendered for another Application', async () => {
  let resolveDeploy: (value: Response) => void = () => undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.includes('/resources')) return Response.json({ resources: [] });
    if (url.endsWith('/preview')) return Response.json({ token: 't', draftVersion: 1, changes: [{ workloadId: 'api', action: 'UPDATE', resources: { existing: [], new: [], unreferenced: [] }, planHash: 'p' }] });
    if (url.endsWith('/deploy')) return new Promise<Response>((done) => { resolveDeploy = done; });
    return url.includes('/applications/other/') ? list('staging', [{ id: 'other-api', state: '', ready: true, score: {} }]) : pendingList();
  }));
  const user = userEvent.setup();
  const view = render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  await user.click(await screen.findByRole('button', { name: 'Triển khai các thay đổi này' }));
  expect(screen.getByRole('button', { name: 'Đang triển khai…' })).toBeDisabled();
  view.rerender(<ApplicationHomePage application={{ ...application, id: 'other', name: 'Other', subdomain: 'other' }} />);
  expect(await screen.findByText('other-api')).toBeInTheDocument();
  await act(async () => { resolveDeploy(Response.json({ status: 'SUCCEEDED', results: [{ workloadId: 'api', action: 'UPDATE', status: 'SUCCEEDED', deploymentId: 'dep-old' }] })); });
  expect(screen.queryByLabelText('Kết quả triển khai')).not.toBeInTheDocument();
  expect(screen.queryByRole('link', { name: 'xem triển khai' })).not.toBeInTheDocument();
  expect(screen.getByRole('heading', { name: 'Other' })).toBeInTheDocument();
  expect(screen.getByText('other-api')).toBeInTheDocument();
  expect(screen.queryByText('api')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Xem trước thay đổi' })).toBeEnabled();
});

it('shows unknown action and status keys verbatim without reading inherited properties', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes('/deployments')) return Response.json({ deployments: [{ id: 'd1', workloadId: 'api', action: 'constructor', status: 'toString', startedAt: '2026-01-01T00:00:00Z' }] });
    if (url.endsWith('/preview')) return Response.json({ token: 't', draftVersion: 1, changes: [{ workloadId: 'api', action: 'toString', resources: { existing: [], new: [], unreferenced: [] }, planHash: 'p' }] });
    return list('staging', [{ id: 'api', state: '', ready: true, score: {} }]);
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('Xem trước thay đổi');
  const entry = await screen.findByRole('button', { name: /constructor/ });
  expect(entry).toHaveTextContent('toString');
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  const item = (await within(await screen.findByLabelText('Xem trước triển khai')).findByRole('listitem'));
  expect(item).toHaveTextContent('api · toString · trình dựng: Chưa xác định trình dựng score-k8s (không có renderer mặc định)');
  expect(item).not.toHaveTextContent('Kubernetes tích hợp sẵn');
});

it('uses a fixed Vietnamese message when delete fails and keeps the list', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    if (init?.method === 'DELETE') return Response.json({ error: 'db password=hunter2' }, { status: 500 });
    if (String(input).includes('/deployments')) return Response.json({ deployments: [] });
    if (String(input).includes('/resources')) return Response.json({ resources: [] });
    return list('staging', [{ id: 'api', state: '', ready: true, score: {} }]);
  }));
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xóa' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Không thể đánh dấu xóa Workload.');
  expect(document.body).not.toHaveTextContent('hunter2');
  expect(screen.getByText('api')).toBeInTheDocument();
  vi.restoreAllMocks();
});

it('opens on the Environment given by the route and previews that Environment', async () => {
  const urls: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    urls.push(url);
    if (url.endsWith('/workloads')) return Response.json({ applicationKey: 'catalog', environmentKey: 'production', draftVersion: 3, workloads: [{ id: 'api', state: 'PENDING_UPSERT', ready: false, score: {} }] });
    if (url.endsWith('/preview')) return Response.json({ token: 't', draftVersion: 3, configRevisionId: 'r', changes: [{ workloadId: 'api', action: 'UPDATE', resources: { existing: [], new: [], unreferenced: [] }, planHash: 'p' }] });
    return Response.json({}, { status: 404 });
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} initialEnvironment="production" />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  expect(await screen.findByRole('heading', { name: 'Xem trước cho production' })).toBeInTheDocument();
  expect(urls.filter((url) => url.includes('/workloads') || url.endsWith('/preview')).every((url) => url.includes('/environments/production/'))).toBe(true);
  expect(urls.some((url) => url.includes('/environments/staging/'))).toBe(false);
});

it('shows the safe Preview projection: renderer, module pin, per-resource provider targets, ownership and intent only', async () => {
  const digest = 'a'.repeat(64);
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/workloads')) return Response.json({ draftVersion: 1, workloads: [{ id: 'api', state: 'PENDING_UPSERT', ready: false, score: {} }] });
    if (url.endsWith('/preview')) return Response.json({
      token: 't', draftVersion: 1,
      changes: [{ workloadId: 'api', action: 'DEPLOY', planHash: 'p', resources: { existing: [], new: ['postgres.default#shared.orders'], unreferenced: [] },
        rendering: { definitionKey: 'render-cloud', driverType: 'score-k8s', bundle: { id: 'score-k8s-internal-v1', version: '0.15.0' } },
        resourceIntents: [
          { descriptor: 'postgres.cloud-managed#shared.orders', resourceType: 'postgres', class: 'cloud-managed', scope: 'SHARED:orders', ownership: 'ORCHESTRATOR', intent: 'CREATE', definitionKey: 'aurora-prod', driverType: 'terraform',
            module: { module: 'aurora', sourceId: 'platform', revision: 'v3-2026.10.13', subdirectory: 'aurora', contentSha256: digest }, providers: [{ name: 'aws', kind: 'aws', target: 'account', connectionKey: 'aws-prod', region: 'us-east-1' }] },
          { descriptor: 'vpc.default#@infra', resourceType: 'vpc', class: 'default', scope: 'ENVIRONMENT:shop.prod', ownership: 'ORCHESTRATOR', intent: 'RETAIN', definitionKey: 'vpc-aws', driverType: 'terraform' },
          { descriptor: 'k8s-cluster.eks#environments.shop.prod', resourceType: 'k8s-cluster', class: 'eks', scope: 'ENVIRONMENT:shop.prod', ownership: 'PLATFORM', intent: 'BIND', definitionKey: 'cluster-aws-eks', driverType: 'existing-cluster' },
        ] }],
    });
    return Response.json({}, { status: 404 });
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  const panel = await screen.findByLabelText('Xem trước triển khai');
  expect(within(panel).queryByLabelText('Đích đã ghim của xem trước')).not.toBeInTheDocument();
  expect(panel.querySelector('.preview-changes > li')).toHaveTextContent('trình dựng: score-k8s 0.15.0 (mẫu render-cloud)');
  const table = within(panel).getByRole('table');
  const rows = within(table).getAllByRole('row').slice(1);
  expect(rows[0]).toHaveTextContent('postgres / cloud-managed');
  expect(rows[0]).toHaveTextContent('aurora-prod');
  expect(rows[0]).toHaveTextContent('aurora · v3-2026.10.13');
  expect(rows[0]).toHaveTextContent('sha256:aaaaaaaaaaaa');
  expect(rows[0]).toHaveTextContent('aws: tài khoản AWS của cấu hình aws-prod · vùng us-east-1');
  expect(rows[0]).toHaveTextContent('Tạo mới');
  expect(rows[1]).toHaveTextContent('Giữ nguyên (đã xác minh không đổi');
  expect(rows[2]).toHaveTextContent('Nền tảng (liên kết hệ thống)');
  expect(rows[2]).toHaveTextContent('Liên kết hệ thống, không tạo');
  expect(panel.textContent).toMatch(/không phải kế hoạch Terraform hay manifest cuối cùng/);
  for (const leak of [/password/i, /kubeconfig/i, /credential/i, /state/i, /secret/i]) expect(panel.textContent).not.toMatch(leak);
});

it('never shows an existing identity as unchanged without proof, and treats a missing projection as unavailable', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/workloads')) return Response.json({ draftVersion: 1, workloads: [{ id: 'api', state: 'PENDING_UPSERT', ready: false, score: {} }] });
    if (url.endsWith('/preview')) return Response.json({ token: 't', draftVersion: 1, changes: [
      { workloadId: 'api', action: 'UPDATE', planHash: 'p', resources: { existing: [], new: [], unreferenced: [] }, resourceIntents: [
        { descriptor: 'postgres.default#modules.api.externals.db', resourceType: 'postgres', class: 'default', scope: 'WORKLOAD:api', ownership: 'ORCHESTRATOR', intent: 'UPDATE', definitionKey: 'pg', driverType: 'terraform' },
        { descriptor: 'redis.default#modules.api.externals.cache', resourceType: 'redis', class: 'default', scope: 'WORKLOAD:api', ownership: 'ORCHESTRATOR', intent: 'UNVERIFIED', definitionKey: 'redis', driverType: 'terraform' },
      ] },
      { workloadId: 'web', action: 'UPDATE', planHash: 'q', resources: { existing: [], new: [], unreferenced: [] } },
      { workloadId: 'empty', action: 'UPDATE', planHash: 'r', resources: { existing: [], new: [], unreferenced: [] }, resourceIntents: [] },
    ] });
    return Response.json({}, { status: 404 });
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Xem trước thay đổi' }));
  const panel = await screen.findByLabelText('Xem trước triển khai');
  const rows = within(panel).getAllByRole('row');
  expect(rows[1]).toHaveTextContent('Cập nhật tài nguyên đã có');
  expect(rows[2]).toHaveTextContent('chưa xác minh được thay đổi; có thể được cập nhật');
  expect(rows[2]).not.toHaveTextContent('Giữ nguyên');
  expect(panel).toHaveTextContent('Chưa có thông tin tài nguyên hạ tầng trong kết quả xem trước này nên chưa thể kết luận');
  expect(panel).toHaveTextContent('Không có tài nguyên hạ tầng nào cần tạo, cập nhật hoặc liên kết.');
});
