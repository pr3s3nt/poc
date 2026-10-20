import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import type { Application } from '../../shared/types/application';
import { bothConfigured } from '../../test/targets';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: bothConfigured(), workloads: { staging: [], production: [] } };

afterEach(() => vi.unstubAllGlobals());

it('saves an Application key as a Score reference, not a copied value', async () => {
  let saved: Record<string, unknown> | undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [{ name: 'API_URL', kind: 'VARIABLE', value: 'https://internal.example', configured: true, usedBy: [] }] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (init?.method === 'PUT') {
      saved = JSON.parse(String(init.body)) as Record<string, unknown>;
      return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', draftVersion: 1, workloads: [] });
    }
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', draftVersion: 0, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'frontend');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/frontend:test');
  await user.click(within(screen.getByRole('group', { name: 'Biến của ứng dụng cho main' })).getByRole('checkbox', { name: 'API_URL' }));
  await user.click(screen.getByRole('checkbox', { name: 'Dùng tên Container khác cho API_URL' }));
  await user.clear(screen.getByLabelText('Tên trong Container cho API_URL'));
  await user.type(screen.getByLabelText('Tên trong Container cho API_URL'), 'BACKEND_URL');
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(saved).toBeDefined();
  const score = saved?.score as { containers: { main: { variables: Record<string, string> } }; resources: { env: { type: string } } };
  expect(score.containers.main.variables).toEqual({ BACKEND_URL: '${resources.env.API_URL}' });
  expect(score.resources.env.type).toBe('environment');
  expect(JSON.stringify(saved)).not.toContain('https://internal.example');
});

it('keeps edits after a save conflict, blocks Save until an explicit reload, and never resends on its own', async () => {
  const puts: number[] = [];
  let lists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (init?.method === 'PUT') {
      puts.push((JSON.parse(String(init.body)) as { version: number }).version);
      return puts.length === 1 ? Response.json({ error: 'workloads changed since they were loaded; reload and try again' }, { status: 409 }) : Response.json({ draftVersion: 6, workloads: [] });
    }
    lists += 1;
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', draftVersion: lists === 1 ? 4 : 5, workloads: lists === 1 ? [] : [{ id: 'web', state: 'PENDING_UPSERT', ready: false, score: { metadata: { name: 'web' }, containers: { main: { image: 'other:v9' } } } }] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'web');
  await user.type(screen.getByLabelText('Ảnh Container'), 'mine:v1');
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(await screen.findByText(/đã thay đổi sau khi trang này được tải/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeDisabled();
  expect(screen.getByLabelText('Ảnh Container')).toHaveValue('mine:v1');
  expect(puts).toEqual([4]);
  await user.click(screen.getByRole('button', { name: 'Tải lại trạng thái hiện tại' }));
  expect(await screen.findByText(/Đã tải lại draft phiên bản 5/)).toBeInTheDocument();
  expect(screen.getByText(/có thay đổi đang chờ/)).toBeInTheDocument();
  expect(screen.getByLabelText('Ảnh Container')).toHaveValue('mine:v1');
  expect(puts).toEqual([4]);
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(puts).toEqual([4, 5]);
});

it('does not treat ENVIRONMENT_BUSY (409) as a stale draft: edits stay, no stale panel, Save stays available', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (init?.method === 'PUT') return Response.json({ error: 'busy', code: 'ENVIRONMENT_BUSY' }, { status: 409 });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', draftVersion: 4, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'web');
  await user.type(screen.getByLabelText('Ảnh Container'), 'mine:v1');
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Môi trường đang bận');
  expect(screen.queryByText(/đã thay đổi sau khi trang này được tải/)).not.toBeInTheDocument();
  expect(screen.getByLabelText('Ảnh Container')).toHaveValue('mine:v1');
  expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeEnabled();
});

it('keeps the stale panel and retry when reloading the current state fails', async () => {
  let lists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (init?.method === 'PUT') return Response.json({ error: 'conflict' }, { status: 409 });
    lists += 1;
    return lists === 2 ? Response.json({ error: 'unavailable' }, { status: 500 }) : Response.json({ draftVersion: lists, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'web');
  await user.type(screen.getByLabelText('Ảnh Container'), 'mine:v1');
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  await user.click(await screen.findByRole('button', { name: 'Tải lại trạng thái hiện tại' }));
  const panel = screen.getByLabelText('Draft Workload đã cũ');
  expect(await within(panel).findByText(/Không tải lại được trạng thái hiện tại/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeDisabled();
  await user.click(within(panel).getByRole('button', { name: 'Tải lại trạng thái hiện tại' }));
  expect(await screen.findByText(/Đã tải lại draft phiên bản 3/)).toBeInTheDocument();
  expect(screen.queryByLabelText('Draft Workload đã cũ')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Ảnh Container')).toHaveValue('mine:v1');
});

it('offers retry when the editor cannot load', async () => {
  let calls = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    calls += 1;
    return calls === 1 ? Response.json({ error: 'unavailable' }, { status: 500 }) : Response.json({ draftVersion: 1, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await user.click(await screen.findByRole('button', { name: 'Thử lại' }));
  expect(await screen.findByRole('heading', { name: 'Thông tin cơ bản' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeEnabled();
});

it('requires PostgreSQL inputs and saves them as Score resource params', async () => {
  let saved: Record<string, unknown> | undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [{ key: 'postgres', classes: [], inputs: [{ name: 'database', type: 'string', required: true }, { name: 'username', type: 'string', required: true }], outputs: [{ name: 'host' }] }] });
    if (init?.method === 'PUT') { saved = JSON.parse(String(init.body)) as Record<string, unknown>; return Response.json({ draftVersion: 1, workloads: [] }); }
    return Response.json({ draftVersion: 0, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'backend');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/backend:test');
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  await user.type(screen.getByLabelText('Alias tài nguyên'), 'db');
  await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'postgres');
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('database là bắt buộc');
  expect(saved).toBeUndefined();
  await user.type(screen.getByLabelText('Tham số database'), 'catalog');
  await user.type(screen.getByLabelText('Tham số username'), 'app');
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(saved).toBeDefined();
  const score = saved?.score as { resources: { db: { type: string; params: Record<string, string> } } };
  expect(score.resources.db).toEqual({ type: 'postgres', params: { database: 'catalog', username: 'app' } });
});

it('preserves PostgreSQL params when editing a workload on the form', async () => {
  let saved: Record<string, unknown> | undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [{ key: 'postgres', classes: [], inputs: [{ name: 'database', type: 'string', required: true }, { name: 'username', type: 'string', required: true }], outputs: [] }] });
    if (init?.method === 'PUT') { saved = JSON.parse(String(init.body)) as Record<string, unknown>; return Response.json({ draftVersion: 2, workloads: [] }); }
    return Response.json({ draftVersion: 1, workloads: [{ id: 'backend', score: { apiVersion: 'score.dev/v1b1', metadata: { name: 'backend' }, containers: { main: { image: 'example.invalid/backend:v1' } }, resources: { db: { type: 'postgres', params: { database: 'catalog', username: 'app' } } } } }] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="backend" />);
  expect(await screen.findByLabelText('Tham số database')).toHaveValue('catalog');
  expect(screen.getByLabelText('Tham số username')).toHaveValue('app');
  await user.clear(screen.getByLabelText('Tham số database'));
  await user.type(screen.getByLabelText('Tham số database'), 'catalog_v2');
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  const score = saved?.score as { resources: { db: { params: Record<string, string> } } };
  expect(score.resources.db.params).toEqual({ database: 'catalog_v2', username: 'app' });
});

it('shows a backend resource param rejection, keeps the form and does not report a saved draft', async () => {
  const puts: unknown[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [{ key: 'postgres', classes: [], inputs: [{ name: 'database', type: 'string', required: true }], outputs: [{ name: 'host' }] }] });
    if (init?.method === 'PUT') { puts.push(JSON.parse(String(init.body))); return Response.json({ error: 'workloadconfig: invalid input: resources.db.params.database is required by resource type postgres', code: 'INVALID_SCORE', params: { resourceType: 'postgres' } }, { status: 400 }); }
    return Response.json({ draftVersion: 3, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'backend');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/backend:test');
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  await user.type(screen.getByLabelText('Alias tài nguyên'), 'db');
  await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'postgres');
  await user.type(screen.getByLabelText('Tham số database'), 'catalog');
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Tham số của tài nguyên thuộc loại postgres chưa đúng');
  expect(screen.getByRole('alert')).not.toHaveTextContent(/resources\.db|workloadconfig/);
  expect(puts).toHaveLength(1);
  expect((puts[0] as { version: number }).version).toBe(3);
  expect(screen.getByRole('heading', { name: 'Thêm Workload' })).toBeInTheDocument();
  expect(screen.getByLabelText('Tên Workload')).toHaveValue('backend');
  expect(screen.getByLabelText('Tham số database')).toHaveValue('catalog');
  expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeEnabled();
});

it('shows a Score import rejection without presenting the file as parsed', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (url.endsWith('/workloads/parse')) return Response.json({ error: 'workloadconfig: invalid input: resources.cache.type is not a registered resource type', code: 'INVALID_VARIABLE' }, { status: 400 });
    return Response.json({ draftVersion: 0, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.click(screen.getByRole('button', { name: 'Nhập Score' }));
  const content = 'apiVersion: score.dev/v1b1\nmetadata:\n  name: api\n';
  // jsdom File has no text(); provide it so the page reads the upload.
  const file = Object.assign(new File([content], 'score.yaml', { type: 'text/yaml' }), { text: async () => content });
  await user.upload(screen.getByLabelText('File Score'), file);
  expect(await screen.findByRole('alert')).toHaveTextContent('Tên, giá trị hoặc tham chiếu biến không hợp lệ');
  expect(screen.queryByText(/Đã phân tích Workload/)).not.toBeInTheDocument();
});

it('disables Save while an environment operation runs, keeps the typed text and re-enables on release', async () => {
  const puts: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'PUT') { puts.push(url); return Response.json({ draftVersion: 1, workloads: [] }); }
    if (url.endsWith('/configuration')) return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', draftVersion: 0, workloads: [] });
  }));
  const busyApp: Application = { ...application, environments: { ...application.environments, staging: { ...application.environments.staging, activeOperation: { id: 'op1', kind: 'DEPLOY', status: 'ACTIVE', stage: 'DEPLOY web', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: false } } } };
  const user = userEvent.setup();
  const view = render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'web');
  await user.type(screen.getByLabelText('Ảnh Container'), 'mine:v1');
  view.rerender(<WorkloadEditorPage application={busyApp} environment="staging" />);
  expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeDisabled();
  expect(screen.getByRole('status')).toHaveTextContent('đang chạy');
  expect(screen.getByLabelText('Ảnh Container')).toHaveValue('mine:v1');
  expect(puts).toEqual([]);
  view.rerender(<WorkloadEditorPage application={application} environment="staging" />);
  expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeEnabled();
  expect(screen.getByLabelText('Ảnh Container')).toHaveValue('mine:v1');
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(puts).toHaveLength(1);
});

// ---- T08: basic/advanced form, effective defaults, typed params, public route targets ----
// Saved Score as the page sends it; tests read nested fields directly.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Saved = { score: Record<string, any>; version: number };
type Serve = { workloads?: unknown[]; types?: unknown[]; keys?: unknown[] };
function serve(options: Serve = {}) {
  const puts: Saved[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ version: 1, keys: options.keys ?? [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: (options.types ?? []).map((type) => ({ classes: [], ...(type as object) })) });
    if (init?.method === 'PUT') { puts.push(JSON.parse(String(init.body)) as Saved); return Response.json({ draftVersion: 2, workloads: [] }); }
    return Response.json({ draftVersion: 1, workloads: options.workloads ?? [] });
  }));
  return { puts };
}
const saveButton = () => screen.getByRole('button', { name: 'Lưu draft Workload' });
const note = (name: string) => screen.getByRole('note', { name });
async function fillBasic(user: ReturnType<typeof userEvent.setup>, name = 'web') {
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), name);
  await user.type(screen.getByLabelText('Ảnh Container'), `example.invalid/${name}:v1`);
}
async function addPort(user: ReturnType<typeof userEvent.setup>, name: string, port: string, index = 0) {
  await user.click(screen.getByRole('button', { name: '+ Thêm cổng' }));
  if (name) await user.type(screen.getAllByLabelText('Tên cổng Service')[index]!, name);
  await user.type(screen.getAllByLabelText('Cổng Service', { exact: true })[index]!, port);
}
const routeSelect = () => screen.getByLabelText('Cổng Service công khai') as HTMLSelectElement;

it('keeps resources, class and target port in closed advanced sections with effective defaults and writes nothing for them', async () => {
  const { puts } = serve({ types: [{ key: 'redis', inputs: [], outputs: [] }] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  await user.type(screen.getByLabelText('Alias tài nguyên'), 'cache');
  await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'redis');
  await addPort(user, 'http', '8080');
  expect(screen.queryByLabelText('Yêu cầu CPU')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Lớp tài nguyên')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Cổng đích của Container')).not.toBeInTheDocument();
  expect(note('Tóm tắt nâng cao Container 1')).toHaveTextContent('Yêu cầu hiệu lực: CPU 10m (mặc định) · Bộ nhớ 32Mi (mặc định). Giới hạn: CPU chưa đặt · Bộ nhớ chưa đặt.');
  expect(note('Tóm tắt nâng cao tài nguyên cache')).toHaveTextContent('Lớp hiệu lực: default (mặc định)');
  expect(note('Tóm tắt nâng cao cổng http')).toHaveTextContent('Cổng đích hiệu lực: 8080 (theo cổng Service)');
  await user.click(saveButton());
  const score = puts[0]!.score;
  expect(score.containers.main).toEqual({ image: 'example.invalid/web:v1' });
  expect(score.resources.cache).toEqual({ type: 'redis' });
  expect(score.service).toEqual({ ports: { http: { port: 8080, targetPort: 8080 } } });
});

it('shows the effective request as explicit, else same-dimension limit, else default, and never adds a limit', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  const summary = () => note('Tóm tắt nâng cao Container 1');
  await user.click(screen.getByRole('button', { name: 'Hiện nâng cao Container 1' }));
  await user.type(screen.getByLabelText('Giới hạn CPU'), '500m');
  expect(summary()).toHaveTextContent('CPU 500m (theo limit) · Bộ nhớ 32Mi (mặc định). Giới hạn: CPU 500m · Bộ nhớ chưa đặt.');
  await user.type(screen.getByLabelText('Yêu cầu bộ nhớ'), '64Mi');
  expect(summary()).toHaveTextContent('Bộ nhớ 64Mi (tường minh)');
  await user.type(screen.getByLabelText('Yêu cầu CPU'), '100m');
  expect(summary()).toHaveTextContent('CPU 100m (tường minh)');
  await user.click(screen.getByRole('button', { name: 'Ẩn nâng cao Container 1' }));
  expect(screen.queryByLabelText('Giới hạn CPU')).not.toBeInTheDocument();
  expect(summary()).toHaveTextContent('CPU 100m (tường minh) · Bộ nhớ 64Mi (tường minh). Giới hạn: CPU 500m · Bộ nhớ chưa đặt.');
  await user.click(saveButton());
  expect(puts[0]!.score.containers.main.resources).toEqual({ requests: { cpu: '100m', memory: '64Mi' }, limits: { cpu: '500m' } });
});

it('keeps imported advanced overrides through collapse and basic edits and writes them back unchanged', async () => {
  const score = { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/api:v1', resources: { requests: { cpu: '250m' }, limits: { cpu: '1', memory: '256Mi' } } } }, resources: { db: { type: 'postgres', class: 'large', params: { database: 'shop' } } }, service: { ports: { http: { port: 80, targetPort: 9090 } } } };
  const { puts } = serve({ workloads: [{ id: 'api', state: 'PENDING_UPSERT', score }], types: [{ key: 'postgres', classes: [], inputs: [{ name: 'database', type: 'string', required: true }], outputs: [] }] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  await screen.findByRole('heading', { name: 'Sửa api' });
  expect(note('Tóm tắt nâng cao Container 1')).toHaveTextContent('CPU 250m (tường minh) · Bộ nhớ 256Mi (theo limit). Giới hạn: CPU 1 · Bộ nhớ 256Mi.');
  expect(note('Tóm tắt nâng cao tài nguyên db')).toHaveTextContent('Lớp hiệu lực: large (tường minh)');
  expect(note('Tóm tắt nâng cao cổng http')).toHaveTextContent('Cổng đích hiệu lực: 9090 (ghi đè)');
  await user.click(screen.getByRole('button', { name: 'Hiện nâng cao tài nguyên db' }));
  expect(screen.getByLabelText('Lớp tài nguyên')).toHaveValue('large');
  await user.click(screen.getByRole('button', { name: 'Ẩn nâng cao tài nguyên db' }));
  await user.click(screen.getByRole('button', { name: 'Hiện nâng cao cổng http' }));
  expect(screen.getByLabelText('Cổng đích của Container')).toHaveValue(9090);
  await user.clear(screen.getByLabelText('Ảnh Container'));
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:v2');
  await user.click(saveButton());
  expect(puts[0]!.score).toEqual({ ...score, containers: { main: { ...score.containers.main, image: 'example.invalid/api:v2' } } });
});

it('omits an empty class and writes a class only while it is set', async () => {
  const { puts } = serve({ types: [{ key: 'redis', inputs: [], outputs: [] }] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  await user.type(screen.getByLabelText('Alias tài nguyên'), 'cache');
  await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'redis');
  await user.click(screen.getByRole('button', { name: 'Hiện nâng cao tài nguyên cache' }));
  await user.type(screen.getByLabelText('Lớp tài nguyên'), 'big');
  expect(note('Tóm tắt nâng cao tài nguyên cache')).toHaveTextContent('Lớp hiệu lực: big (tường minh)');
  await user.clear(screen.getByLabelText('Lớp tài nguyên'));
  expect(note('Tóm tắt nâng cao tài nguyên cache')).toHaveTextContent('Lớp hiệu lực: default (mặc định)');
  await user.click(saveButton());
  expect(puts[0]!.score.resources.cache).toEqual({ type: 'redis' });
});

it('uses a per-Service target port override as a number and falls back to the Service port', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  await addPort(user, 'http', '80');
  await addPort(user, 'metrics', '9100', 1);
  await user.click(screen.getByRole('button', { name: 'Hiện nâng cao cổng http' }));
  await user.type(screen.getByLabelText('Cổng đích của Container'), '8080');
  await user.click(screen.getByRole('button', { name: 'Ẩn nâng cao cổng http' }));
  expect(note('Tóm tắt nâng cao cổng http')).toHaveTextContent('Cổng đích hiệu lực: 8080 (ghi đè)');
  await user.click(saveButton());
  expect(puts[0]!.score.service.ports).toEqual({ http: { port: 80, targetPort: 8080 }, metrics: { port: 9100, targetPort: 9100 } });
});

it('keeps number and bool resource params typed, also when edited from an imported Score', async () => {
  const types = [{ key: 'queue', inputs: [{ name: 'replicas', type: 'number' }, { name: 'durable', type: 'bool' }, { name: 'topic', type: 'string' }], outputs: [] }];
  const { puts } = serve({ types, workloads: [{ id: 'q', state: 'PENDING_UPSERT', score: { apiVersion: 'score.dev/v1b1', metadata: { name: 'q' }, containers: { main: { image: 'example.invalid/q:v1' } }, resources: { jobs: { type: 'queue', params: { replicas: 3, durable: false, topic: '3' } } } } }] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="q" />);
  expect(await screen.findByLabelText('Tham số replicas')).toHaveValue(3);
  expect(screen.getByLabelText('Tham số durable')).toHaveValue('false');
  await user.clear(screen.getByLabelText('Tham số replicas'));
  await user.type(screen.getByLabelText('Tham số replicas'), '5');
  await user.selectOptions(screen.getByLabelText('Tham số durable'), 'true');
  await user.click(saveButton());
  expect(puts[0]!.score.resources.jobs.params).toEqual({ replicas: 5, durable: true, topic: '3' });
});

it('never adds a public route on its own and selects a port only when exactly one is valid', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  await addPort(user, 'http', '8080');
  expect(screen.queryByLabelText('Đường dẫn công khai')).not.toBeInTheDocument();
  await user.click(saveButton());
  expect(puts[0]!.score.service.publicRoutes).toBeUndefined();
});

it('selects the only valid Service port after Add public route, not before', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  await addPort(user, 'http', '8080');
  await user.click(screen.getByRole('button', { name: '+ Thêm đường dẫn công khai' }));
  expect(routeSelect()).toHaveValue('http');
  await user.type(screen.getByLabelText('Đường dẫn công khai'), '/');
  await addPort(user, 'metrics', '9100', 1);
  expect(routeSelect()).toHaveValue('http');
  await user.click(saveButton());
  expect(puts[0]!.score.service.publicRoutes).toEqual([{ path: '/', port: 'http' }]);
});

it('requires an explicit choice when no or several Service ports are valid', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm đường dẫn công khai' }));
  expect(routeSelect()).toHaveValue('');
  await user.type(screen.getByLabelText('Đường dẫn công khai'), '/');
  await user.click(saveButton());
  expect(screen.getByRole('alert')).toHaveTextContent('Hãy chọn một cổng Service');
  await addPort(user, 'http', '8080');
  await addPort(user, 'admin', '9000', 1);
  await user.click(screen.getByRole('button', { name: '+ Thêm đường dẫn công khai' }));
  expect(screen.getAllByLabelText('Cổng Service công khai')[1]).toHaveValue('');
  expect(puts).toEqual([]);
});

it('does not count blank, duplicate or out-of-range Service ports as valid targets', async () => {
  serve();
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  await addPort(user, 'ok', '8080');
  await addPort(user, '', '81', 1);
  await addPort(user, 'big', '70000', 2);
  await addPort(user, 'dup', '82', 3);
  await addPort(user, 'dup', '83', 4);
  await user.click(screen.getByRole('button', { name: '+ Thêm đường dẫn công khai' }));
  expect(routeSelect()).toHaveValue('ok');
  expect(within(routeSelect()).getAllByRole('option').map((option) => option.textContent)).toEqual(['Chọn cổng', 'ok · 8080']);
});

it('keeps the old route target when its port is renamed, removed or invalid, shows it and blocks Save', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  await addPort(user, 'http', '8080');
  await addPort(user, 'other', '9000', 1);
  await user.click(screen.getByRole('button', { name: '+ Thêm đường dẫn công khai' }));
  await user.selectOptions(routeSelect(), 'http');
  await user.type(screen.getByLabelText('Đường dẫn công khai'), '/');
  // invalid number
  await user.clear(screen.getAllByLabelText('Cổng Service', { exact: true })[0]!);
  await user.type(screen.getAllByLabelText('Cổng Service', { exact: true })[0]!, '70000');
  expect(routeSelect()).toHaveValue('http');
  expect(screen.getByText(/Cổng Service "http" không còn hợp lệ/, { selector: 'p' })).toBeInTheDocument();
  await user.click(saveButton());
  expect(screen.getByRole('alert')).toHaveTextContent('Cổng Service "http" không còn hợp lệ');
  // restore, then rename
  await user.clear(screen.getAllByLabelText('Cổng Service', { exact: true })[0]!);
  await user.type(screen.getAllByLabelText('Cổng Service', { exact: true })[0]!, '8080');
  expect(screen.queryByText(/không còn hợp lệ/, { selector: 'p' })).not.toBeInTheDocument();
  await user.type(screen.getAllByLabelText('Tên cổng Service')[0]!, '2');
  expect(routeSelect()).toHaveValue('http');
  expect(within(routeSelect()).getByRole('option', { name: 'http · không hợp lệ' })).toBeInTheDocument();
  await user.click(saveButton());
  expect(puts).toEqual([]);
  // remove: still the old target, never the remaining valid port
  await user.click(within(screen.getAllByLabelText('Tên cổng Service')[0]!.closest('.binding-row') as HTMLElement).getByRole('button', { name: 'Xóa' }));
  expect(routeSelect()).toHaveValue('http');
  expect(screen.getByText(/Cổng Service "http" không còn hợp lệ/, { selector: 'p' })).toBeInTheDocument();
  await user.click(saveButton());
  expect(puts).toEqual([]);
  // an explicit choice repairs it
  await user.selectOptions(routeSelect(), 'other');
  await user.click(saveButton());
  expect(puts[0]!.score.service.publicRoutes).toEqual([{ path: '/', port: 'other' }]);
});

it('does not select keys or secrets on its own and sends no variables for an untouched picker', async () => {
  const { puts } = serve({ keys: [{ name: 'API_URL', kind: 'VARIABLE', configured: true, value: 'x', usedBy: [] }, { name: 'TOKEN', kind: 'SECRET', configured: true, value: 'y', usedBy: [] }] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  expect(await screen.findByRole('checkbox', { name: 'API_URL' })).not.toBeChecked();
  expect(screen.getByRole('checkbox', { name: 'TOKEN' })).not.toBeChecked();
  await user.click(saveButton());
  expect(puts[0]!.score.containers.main.variables).toBeUndefined();
  expect(puts[0]!.score.resources).toBeUndefined();
});

it('blocks Save with Vietnamese validation for an invalid Workload name and keeps the form', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/x:v1');
  await user.click(saveButton());
  expect(screen.getByRole('alert')).toHaveTextContent('Cần nhập tên Workload.');
  expect(screen.getByLabelText('Ảnh Container')).toHaveValue('example.invalid/x:v1');
  expect(puts).toEqual([]);
});

it('round-trips several publicRoutes unchanged and never serializes publicPort', async () => {
  const score = { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/api:v1' } }, service: { ports: { http: { port: 8080 }, admin: { port: 9000 } }, publicRoutes: [{ path: '/', port: 'http' }, { path: '/admin', port: 'admin' }] } };
  const { puts } = serve({ workloads: [{ id: 'api', state: 'PENDING_UPSERT', score }] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  await screen.findByRole('heading', { name: 'Sửa api' });
  expect(screen.getAllByLabelText('Đường dẫn công khai')).toHaveLength(2);
  await user.click(saveButton());
  expect(puts[0]!.score.service).toEqual(score.service);
  expect(JSON.stringify(puts[0])).not.toContain('publicPort');
});

it('rejects an import with publicPort in Vietnamese, keeps the form and converts nothing', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (url.endsWith('/workloads/parse')) return Response.json({ error: 'raw SECRET', code: 'INVALID_SCORE', field: 'service.publicPort' }, { status: 400 });
    return Response.json({ draftVersion: 0, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.click(screen.getByRole('button', { name: 'Nhập Score' }));
  const content = '{"apiVersion":"score.dev/v1b1","metadata":{"name":"api"},"service":{"publicPort":"http"}}';
  const file = Object.assign(new File([content], 'score.json', { type: 'application/json' }), { text: async () => content });
  await user.upload(screen.getByLabelText('File Score'), file);
  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent('publicRoutes');
  expect(alert).not.toHaveTextContent('SECRET');
  expect(screen.queryByText(/Đã phân tích Workload/)).not.toBeInTheDocument();
});

it('does not open a draft that still declares publicPort and does not drop its route', async () => {
  const score = { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/api:v1' } }, service: { ports: { http: { port: 8080 } }, publicPort: 'http' } };
  const { puts } = serve({ workloads: [{ id: 'api', state: 'PENDING_UPSERT', score }] });
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  expect(await screen.findByRole('alert')).toHaveTextContent('publicRoutes');
  expect(screen.queryByLabelText('Đường dẫn công khai')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeDisabled();
  expect(puts).toHaveLength(0);
});
