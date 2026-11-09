// Legacy paths stay covered with switches enabled; production defaults have separate coverage.
vi.mock('../../shared/authoringFeatures', () => ({ authoringFeatures: { workloadImport: true, nonGitDefinitionSources: true } }));
import { openAdvancedFields, openWorkloadGroup } from '../../test/workloadGroups';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import type { Application } from '../../shared/types/application';
import { bothConfigured } from '../../test/targets';

const application: Application = { id: 'catalog', name: 'Catalog', environments: bothConfigured(), workloads: { staging: [], production: [] } };

afterEach(() => vi.unstubAllGlobals());

it('saves an Application key as a Workload reference, not a copied value', async () => {
  let saved: Record<string, unknown> | undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({workload: JSON.parse(String(init?.body)).workload, sources:{}});
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
  await user.click(screen.getByRole('button', { name: '+ Thêm biến' }));
  await user.type(screen.getByLabelText('Tên biến, dòng 1'), 'BACKEND_URL');
  await user.click(screen.getByRole('button', { name: 'Chọn nguồn, dòng 1' }));
  await user.click(screen.getByRole('button', { name: 'Ứng dụng / API_URL' }));
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(saved).toBeDefined();
  const workload = saved?.workload as { containers: { main: { variables: Record<string, string> } }; resources: { env: { type: string } } };
  expect(workload.containers.main.variables).toEqual({ BACKEND_URL: '${resources.env.API_URL}' });
  expect(workload.resources.env.type).toBe('environment');
  expect(JSON.stringify(saved)).not.toContain('https://internal.example');
});

it('keeps edits after a save conflict, blocks Save until an explicit reload, and never resends on its own', async () => {
  const puts: number[] = [];
  let lists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({workload: JSON.parse(String(init?.body)).workload, sources:{}});
    if (url.endsWith('/configuration')) return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (init?.method === 'PUT') {
      puts.push((JSON.parse(String(init.body)) as { version: number }).version);
      return puts.length === 1 ? Response.json({ error: 'workloads changed since they were loaded; reload and try again' }, { status: 409 }) : Response.json({ draftVersion: 6, workloads: [] });
    }
    lists += 1;
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', draftVersion: lists === 1 ? 4 : 5, workloads: lists === 1 ? [] : [{ id: 'web', state: 'PENDING_UPSERT', ready: false, workload: { metadata: { name: 'web' }, containers: { main: { image: 'other:v9' } } } }] });
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
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({workload: JSON.parse(String(init?.body)).workload, sources:{}});
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
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({workload: JSON.parse(String(init?.body)).workload, sources:{}});
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
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({workload: JSON.parse(String(init?.body)).workload, sources:{}});
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

it('requires PostgreSQL inputs and saves them as Workload resource params', async () => {
  let saved: Record<string, unknown> | undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({workload: JSON.parse(String(init?.body)).workload, sources:{}});
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
  await openWorkloadGroup(user,'Tài nguyên phụ thuộc');
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
  const workload = saved?.workload as { resources: { db: { type: string; params: Record<string, string> } } };
  expect(workload.resources.db).toEqual({ type: 'postgres', id: expect.stringMatching(/^resource-/), params: { database: 'catalog', username: 'app' } });
});

it('preserves PostgreSQL params when editing a workload on the form', async () => {
  let saved: Record<string, unknown> | undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({workload: JSON.parse(String(init?.body)).workload, sources:{}});
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [{ key: 'postgres', classes: [], inputs: [{ name: 'database', type: 'string', required: true }, { name: 'username', type: 'string', required: true }], outputs: [] }] });
    if (init?.method === 'PUT') { saved = JSON.parse(String(init.body)) as Record<string, unknown>; return Response.json({ draftVersion: 2, workloads: [] }); }
    return Response.json({ draftVersion: 1, workloads: [{ id: 'backend', workload: { apiVersion: 'orchestrator.dev/workload/v1', metadata: { name: 'backend' }, containers: { main: { image: 'example.invalid/backend:v1' } }, resources: { db: { type: 'postgres', params: { database: 'catalog', username: 'app' } } } } }] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="backend" />);
    await openWorkloadGroup(user,'Tài nguyên phụ thuộc');
  expect(await screen.findByLabelText('Tham số database')).toHaveValue('catalog');
  expect(screen.getByLabelText('Tham số username')).toHaveValue('app');
  await user.clear(screen.getByLabelText('Tham số database'));
  await user.type(screen.getByLabelText('Tham số database'), 'catalog_v2');
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  const workload = saved?.workload as { resources: { db: { params: Record<string, string> } } };
  expect(workload.resources.db.params).toEqual({ database: 'catalog_v2', username: 'app' });
});

it('shows a backend resource param rejection, keeps the form and does not report a saved draft', async () => {
  const puts: unknown[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({workload: JSON.parse(String(init?.body)).workload, sources:{}});
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [{ key: 'postgres', classes: [], inputs: [{ name: 'database', type: 'string', required: true }], outputs: [{ name: 'host' }] }] });
    if (init?.method === 'PUT') { puts.push(JSON.parse(String(init.body))); return Response.json({ error: 'workloadconfig: invalid input: resources.db.params.database is required by resource type postgres', code: 'INVALID_WORKLOAD', params: { resourceType: 'postgres' } }, { status: 400 }); }
    return Response.json({ draftVersion: 3, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'backend');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/backend:test');
  await openWorkloadGroup(user,'Tài nguyên phụ thuộc');
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

it('shows a Workload import rejection without presenting the file as parsed', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({workload: JSON.parse(String(init?.body)).workload, sources:{}});
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (url.endsWith('/workloads/parse')) return Response.json({ error: 'workloadconfig: invalid input: resources.cache.type is not a registered resource type', code: 'INVALID_VARIABLE' }, { status: 400 });
    return Response.json({ draftVersion: 0, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.click(screen.getByRole('button', { name: 'Nhập Workload' }));
  const content = 'apiVersion: orchestrator.dev/workload/v1\nmetadata:\n  name: api\n';
  // jsdom File has no text(); provide it so the page reads the upload.
  const file = Object.assign(new File([content], 'workload.yaml', { type: 'text/yaml' }), { text: async () => content });
  await user.upload(screen.getByLabelText('File Workload'), file);
  expect(await screen.findByRole('alert')).toHaveTextContent('Tên, giá trị hoặc tham chiếu biến không hợp lệ');
  expect(screen.queryByText(/Đã phân tích Workload/)).not.toBeInTheDocument();
});

it('disables Save while an environment operation runs, keeps the typed text and re-enables on release', async () => {
  const puts: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({workload: JSON.parse(String(init?.body)).workload, sources:{}});
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
  expect(screen.getAllByRole('status').some(element=>element.textContent?.includes('đang chạy'))).toBe(true);
  expect(screen.getByLabelText('Ảnh Container')).toHaveValue('mine:v1');
  expect(puts).toEqual([]);
  view.rerender(<WorkloadEditorPage application={application} environment="staging" />);
  expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeEnabled();
  expect(screen.getByLabelText('Ảnh Container')).toHaveValue('mine:v1');
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(puts).toHaveLength(1);
});

// ---- T08: basic/advanced form, effective defaults, typed params, public route targets ----
// Saved Workload as the page sends it; tests read nested fields directly.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Saved = { workload: Record<string, any>; version: number };
type Serve = { workloads?: unknown[]; types?: unknown[]; keys?: unknown[] };
function serve(options: Serve = {}) {
  const puts: Saved[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({workload: JSON.parse(String(init?.body)).workload, sources:{}});
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
  await user.type(screen.getByLabelText('Ảnh Container'), `example.invalid/${name}:v1`);await openAdvancedFields(user);
}
async function addPort(user: ReturnType<typeof userEvent.setup>, name: string, port: string, index = 0) {
  await openWorkloadGroup(user,'Service & kết nối');
  await user.click(screen.getByRole('button', { name: '+ Thêm cổng' }));
  if (name) await user.type(screen.getAllByLabelText('Tên cổng Service')[index]!, name);
  await user.type(screen.getAllByLabelText('Cổng Service', { exact: true })[index]!, port);
}

it('keeps resources, class and target port in closed advanced sections with effective defaults and writes nothing for them', async () => {
  const { puts } = serve({ types: [{ key: 'redis', inputs: [], outputs: [] }] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  await openWorkloadGroup(user,'Tài nguyên phụ thuộc');
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  await user.type(screen.getByLabelText('Alias tài nguyên'), 'cache');
  await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'redis');
  await addPort(user, 'http', '8080');
  expect(screen.queryByLabelText('Yêu cầu CPU')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Lớp tài nguyên')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Cổng đích của Container')).not.toBeInTheDocument();
  expect(screen.getByText('Tài nguyên tính toán', {exact:true}).closest('summary')!).toHaveTextContent('CPU Chưa cấu hình · RAM Chưa cấu hình');
  expect(note('Tóm tắt nâng cao tài nguyên cache')).toHaveTextContent('Lớp hiệu lực: default (mặc định)');
  expect(note('Tóm tắt nâng cao cổng http')).toHaveTextContent('Cổng đích hiệu lực: 8080 (theo cổng Service)');
  await user.click(saveButton());
  const workload = puts[0]!.workload;
  expect(workload.containers.main).toEqual({ image: 'example.invalid/web:v1' });
  expect(workload.resources.cache).toEqual({ type: 'redis', id: expect.stringMatching(/^resource-/) });
  expect(workload.service).toEqual({ ports: { http: { port: 8080, targetPort: 8080 } } });
});

it('shows the effective request as explicit, else same-dimension limit, else default, and never adds a limit', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  const summary = () => screen.getByText('Tài nguyên tính toán', {exact:true}).closest('summary')!;
  await user.click(screen.getByText('Tài nguyên tính toán', {exact:true}));
  await user.type(screen.getByLabelText('Giới hạn CPU'), '500m');
  expect(summary()).toHaveTextContent('CPU 500m · RAM Chưa cấu hình');
  await user.type(screen.getByLabelText('Yêu cầu bộ nhớ'), '64Mi');
  expect(summary()).toHaveTextContent('RAM 64Mi');
  await user.type(screen.getByLabelText('Yêu cầu CPU'), '100m');
  expect(summary()).toHaveTextContent('CPU 100m');
  await user.click(screen.getByText('Tài nguyên tính toán', {exact:true}));
  expect(screen.queryByLabelText('Giới hạn CPU')).not.toBeInTheDocument();
  expect(summary()).toHaveTextContent('CPU 100m · RAM 64Mi');
  await user.click(saveButton());
  expect(puts[0]!.workload.containers.main.resources).toEqual({ requests: { cpu: '100m', memory: '64Mi' }, limits: { cpu: '500m' } });
});

it('keeps imported advanced overrides through collapse and basic edits and writes them back unchanged', async () => {
  const workload = { apiVersion: 'orchestrator.dev/workload/v1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/api:v1', resources: { requests: { cpu: '250m' }, limits: { cpu: '1', memory: '256Mi' } } } }, resources: { db: { type: 'postgres', class: 'large', params: { database: 'shop' } } }, service: { ports: { http: { port: 80, targetPort: 9090 } } } };
  const { puts } = serve({ workloads: [{ id: 'api', state: 'PENDING_UPSERT', workload }], types: [{ key: 'postgres', classes: [], inputs: [{ name: 'database', type: 'string', required: true }], outputs: [] }] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
    await openWorkloadGroup(user,'Tài nguyên phụ thuộc');
  await screen.findByRole('heading', { name: 'Sửa api' });
  expect(screen.getByText('Tài nguyên tính toán', {exact:true}).closest('summary')!).toHaveTextContent('CPU 250m · RAM 256Mi');
  expect(note('Tóm tắt nâng cao tài nguyên db')).toHaveTextContent('Lớp hiệu lực: large (tường minh)');
  await openWorkloadGroup(user,'Service & kết nối');
  expect(note('Tóm tắt nâng cao cổng http')).toHaveTextContent('Cổng đích hiệu lực: 9090 (ghi đè)');
  await user.click(screen.getByRole('button', { name: 'Hiện nâng cao tài nguyên db' }));
  expect(screen.getByLabelText('Lớp tài nguyên')).toHaveValue('large');
  await user.click(screen.getByRole('button', { name: 'Ẩn nâng cao tài nguyên db' }));
  await user.click(screen.getByRole('button', { name: 'Hiện nâng cao cổng http' }));
  expect(screen.getByLabelText('Cổng đích của Container')).toHaveValue(9090);
  await user.clear(screen.getByLabelText('Ảnh Container'));
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:v2');
  await user.click(saveButton());
  expect(puts[0]!.workload).toEqual({ ...workload, containers: { main: { ...workload.containers.main, image: 'example.invalid/api:v2' } } });
});

it('omits an empty class and writes a class only while it is set', async () => {
  const { puts } = serve({ types: [{ key: 'redis', inputs: [], outputs: [] }] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  await openWorkloadGroup(user,'Tài nguyên phụ thuộc');
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  await user.type(screen.getByLabelText('Alias tài nguyên'), 'cache');
  await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'redis');
  await user.click(screen.getByRole('button', { name: 'Hiện nâng cao tài nguyên cache' }));
  await user.type(screen.getByLabelText('Lớp tài nguyên'), 'big');
  expect(note('Tóm tắt nâng cao tài nguyên cache')).toHaveTextContent('Lớp hiệu lực: big (tường minh)');
  await user.clear(screen.getByLabelText('Lớp tài nguyên'));
  expect(note('Tóm tắt nâng cao tài nguyên cache')).toHaveTextContent('Lớp hiệu lực: default (mặc định)');
  await user.click(saveButton());
  expect(puts[0]!.workload.resources.cache).toEqual({ type: 'redis', id: expect.stringMatching(/^resource-/) });
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
  expect(puts[0]!.workload.service.ports).toEqual({ http: { port: 80, targetPort: 8080 }, metrics: { port: 9100, targetPort: 9100 } });
});

it('keeps number and bool resource params typed, also when edited from an imported Workload', async () => {
  const types = [{ key: 'queue', inputs: [{ name: 'replicas', type: 'number' }, { name: 'durable', type: 'bool' }, { name: 'topic', type: 'string' }], outputs: [] }];
  const { puts } = serve({ types, workloads: [{ id: 'q', state: 'PENDING_UPSERT', workload: { apiVersion: 'orchestrator.dev/workload/v1', metadata: { name: 'q' }, containers: { main: { image: 'example.invalid/q:v1' } }, resources: { jobs: { type: 'queue', params: { replicas: 3, durable: false, topic: '3' } } } } }] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="q" />);
    await openWorkloadGroup(user,'Tài nguyên phụ thuộc');
  expect(await screen.findByLabelText('Tham số replicas')).toHaveValue(3);
  expect(screen.getByLabelText('Tham số durable')).toHaveValue('false');
  await user.clear(screen.getByLabelText('Tham số replicas'));
  await user.type(screen.getByLabelText('Tham số replicas'), '5');
  await user.selectOptions(screen.getByLabelText('Tham số durable'), 'true');
  await user.click(saveButton());
  expect(puts[0]!.workload.resources.jobs.params).toEqual({ replicas: 5, durable: true, topic: '3' });
});

it('never adds a public route on its own and selects a port only when exactly one is valid', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  await addPort(user, 'http', '8080');
  expect(screen.queryByLabelText('Đường dẫn công khai')).not.toBeInTheDocument();
  await user.click(saveButton());
  expect(puts[0]!.workload.service.publicRoutes).toBeUndefined();
});





it('does not add variables, keys or secrets on its own and sends no variables for an untouched table', async () => {
  const { puts } = serve({ keys: [{ name: 'API_URL', kind: 'VARIABLE', configured: true, value: 'x', usedBy: [] }, { name: 'TOKEN', kind: 'SECRET', configured: true, value: 'y', usedBy: [] }] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await fillBasic(user);
  expect(await screen.findByText('Chưa có biến môi trường')).toBeInTheDocument();
  expect(screen.queryByLabelText(/^Tên biến, dòng/)).not.toBeInTheDocument();
  await user.click(saveButton());
  expect(puts[0]!.workload.containers.main.variables).toBeUndefined();
  expect(puts[0]!.workload.resources).toBeUndefined();
});

it('blocks Save with Vietnamese validation for an invalid Workload name and keeps the form', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/x:v1');
  expect(screen.getByLabelText('Tên Workload')).toHaveAttribute('aria-invalid', 'false');
  await user.click(saveButton());
  expect(screen.getByLabelText('Tên Workload')).toHaveAttribute('aria-invalid', 'true');
  expect(screen.getByLabelText('Tên Workload')).toHaveAccessibleDescription('Cần nhập tên Workload.');
  expect(screen.getByLabelText('Tên Workload')).toHaveFocus();
  expect(screen.getByLabelText('Ảnh Container')).toHaveValue('example.invalid/x:v1');
  expect(puts).toEqual([]);
});


it('rejects an import with publicPort in Vietnamese, keeps the form and converts nothing', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({workload: JSON.parse(String(init?.body)).workload, sources:{}});
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (url.endsWith('/workloads/parse')) return Response.json({ error: 'raw SECRET', code: 'INVALID_WORKLOAD', field: 'service.publicPort' }, { status: 400 });
    return Response.json({ draftVersion: 0, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.click(screen.getByRole('button', { name: 'Nhập Workload' }));
  const content = '{"apiVersion":"orchestrator.dev/workload/v1","metadata":{"name":"api"},"service":{"publicPort":"http"}}';
  const file = Object.assign(new File([content], 'workload.json', { type: 'application/json' }), { text: async () => content });
  await user.upload(screen.getByLabelText('File Workload'), file);
  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent('publicRoutes');
  expect(alert).not.toHaveTextContent('SECRET');
  expect(screen.queryByText(/Đã phân tích Workload/)).not.toBeInTheDocument();
});

it('does not open a draft that still declares publicPort and does not drop its route', async () => {
  const workload = { apiVersion: 'orchestrator.dev/workload/v1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/api:v1' } }, service: { ports: { http: { port: 8080 } }, publicPort: 'http' } };
  const { puts } = serve({ workloads: [{ id: 'api', state: 'PENDING_UPSERT', workload }] });
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  expect(await screen.findByRole('alert')).toHaveTextContent('publicRoutes');
  expect(screen.queryByLabelText('Đường dẫn công khai')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeDisabled();
  expect(puts).toHaveLength(0);
});

it('saves a selected scheduled template with Service omitted and explicit scheduling policies', async () => {
 const digest = 'a'.repeat(64);let saved: Record<string, unknown> | undefined;
 vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
  const url = String(input);
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({workload: JSON.parse(String(init?.body)).workload, sources:{}});
  if (url.endsWith('/configuration')) return Response.json({ version: 1, keys: [] });
  if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
  if (url.endsWith('/workload-templates')) return Response.json({ templates: [{ id: 'scheduled', version: '1.0.0', sha256: digest, kind: 'CronJob', labels: { vi: 'Theo lịch' }, inputsSchema: { type: 'object', properties: {}, additionalProperties: false }, defaults: {} }] });
  if (init?.method === 'PUT') { saved = JSON.parse(String(init.body)); return Response.json({ workloads: [], draftVersion: 1 }); }
  return Response.json({ workloads: [], draftVersion: 0 });
 }));
 const user = userEvent.setup();render(<WorkloadEditorPage application={application} environment="staging" selectedTemplate={`scheduled@1.0.0:${digest}`} />);
 await openWorkloadGroup(user,'Cấu hình ít thay đổi');await screen.findByRole('heading',{name:'Kiểu thực thi'});
 await user.type(screen.getByLabelText('Tên Workload'),'backup');await user.type(screen.getByLabelText('Ảnh Container'),'registry.example/backup:v1');
 await user.clear(screen.getByLabelText('Lịch cron'));await user.type(screen.getByLabelText('Lịch cron'),'0 1 * * *');
 await user.click(screen.getByLabelText('Tạm dừng lịch'));await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));
 const workload = saved?.workload as { workloadExecution: { kind: string; template: { sha256: string }; schedule: { expression: string; suspend: boolean } }; service?: unknown };
 expect(workload.workloadExecution.kind).toBe('CronJob');expect(workload.workloadExecution.template.sha256).toBe(digest);expect(workload.workloadExecution.schedule).toMatchObject({expression:'0 1 * * *',suspend:true});expect(workload.service).toBeUndefined();
});

it('shows networking and dependencies immediately and saves an explicitly requested endpoint from Service', async () => {
 let saved: {workload: {resources: Record<string, unknown>; service: {ports: Record<string, unknown>}}} | undefined;
 vi.stubGlobal('fetch',vi.fn(async (input:string|URL|Request,init?:RequestInit)=>{
  const url=String(input);
  if(url.endsWith('/resource-types'))return Response.json({resourceTypes:[{key:'endpoint',classes:[],inputs:[{name:'name',type:'string',required:true},{name:'port',type:'string',required:true},{name:'path',type:'string'}],outputs:[{name:'url',type:'string'}]}]});
  if(url.endsWith('/configuration'))return Response.json({version:1,keys:[]});
  if(init?.method==='PUT'){saved=JSON.parse(String(init.body));return Response.json({draftVersion:1,workloads:[]});}
  return Response.json({draftVersion:0,workloads:[],templates:[]});
 }));
 const user=userEvent.setup();render(<WorkloadEditorPage application={application} environment="staging"/>);
 await screen.findByLabelText('Tên Workload');
 expect(screen.getByRole('button',{name:'+ Thêm tài nguyên'})).toBeVisible();expect(screen.getByRole('button',{name:'+ Thêm cổng'})).toBeVisible();
 await user.type(screen.getByLabelText('Tên Workload'),'web');await user.type(screen.getByLabelText('Ảnh Container'),'nginx:1');
 await user.click(screen.getByRole('button',{name:'+ Thêm cổng'}));await user.type(screen.getByLabelText('Tên cổng Service'),'http');await user.type(screen.getByLabelText('Cổng Service'),'8080');
 expect(screen.queryByLabelText('Tên endpoint')).not.toBeInTheDocument();await user.click(screen.getByRole('button',{name:'Tạo endpoint từ cổng http'}));
 expect(screen.getByLabelText('Tên endpoint')).toHaveValue('web');await user.clear(screen.getByLabelText('Đường dẫn endpoint'));await user.type(screen.getByLabelText('Đường dẫn endpoint'),'/api');
 await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));
 expect(saved?.workload.resources.public).toEqual({type:'endpoint',params:{name:'web',port:'http',path:'/api'}});
 expect(saved?.workload.service.ports.http).toEqual({port:8080,targetPort:8080});
});

it('rejects a workload name that is not a DNS label at the field and sends nothing (UC-16 BR-01)', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  const name = screen.getByLabelText('Tên Workload');
  expect(name).toHaveAccessibleDescription('Không đổi được sau khi lưu.');
  await user.type(name, 'My App');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/web:v1');
  expect(name).toHaveAttribute('aria-invalid', 'true');
  expect(name).toHaveAccessibleDescription(/Chỉ dùng chữ thường, số và "-".*tối đa 63 ký tự/);
  await user.click(saveButton());
  expect(name).toHaveFocus();
  expect(puts).toHaveLength(0);
  await user.clear(name);
  await user.type(name, 'my-app');
  expect(name).toHaveAttribute('aria-invalid', 'false');
  await user.click(saveButton());
  await vi.waitFor(() => expect(puts).toHaveLength(1));
  expect((puts[0]!.workload as { metadata: { name: string } }).metadata.name).toBe('my-app');
});

it('shows an existing workload name in the title without a locked name input', async () => {
  serve({ workloads: [{ id: 'api', workload: { apiVersion: 'orchestrator.dev/workload/v1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/api:v1' } } } }] });
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  await screen.findByRole('heading', { name: 'Sửa api' });
  expect(screen.queryByLabelText('Tên Workload')).not.toBeInTheDocument();
  expect(screen.queryByRole('heading', { name: 'Thông tin cơ bản' })).not.toBeInTheDocument();
});
