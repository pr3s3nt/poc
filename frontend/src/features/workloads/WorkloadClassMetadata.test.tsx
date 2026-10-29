import { cleanup, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import type { ResourceType, Workload } from './api';
import type { Application } from '../../shared/types/application';
import { bothConfigured } from '../../test/targets';
import { typeFixture } from '../../test/types';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: bothConfigured(), workloads: { staging: [], production: [] } };
type Saved = { score: Record<string, unknown>; version: number };

// Types are the recorded GET /resource-types of the product backend (classes included).
function serve(options: { types?: () => Response | Promise<Response>; workloads?: Workload[]; calls?: string[] } = {}) {
  const puts: Saved[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    options.calls?.push(`${init?.method ?? 'GET'} ${url.replace('/api/v1', '')}`);
    if (url.endsWith('/configuration')) return Response.json({ version: 1, keys: [] });
    if (url.endsWith('/resource-types')) return options.types ? options.types() : Response.json({ resourceTypes: typeFixture });
    if (init?.method === 'PUT') { puts.push(JSON.parse(String(init.body)) as Saved); return Response.json({ draftVersion: 2, workloads: [] }); }
    return Response.json({ draftVersion: 1, workloads: options.workloads ?? [] });
  }));
  return { puts };
}
afterEach(() => vi.unstubAllGlobals());
const save = () => screen.getByRole('button', { name: 'Lưu draft Workload' });
const resourcesOf = (saved: Saved | undefined) => saved?.score.resources as Record<string, Record<string, unknown>>;

async function newWorkload(user: ReturnType<typeof userEvent.setup>) {
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'api');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:test');
}
async function addDependency(user: ReturnType<typeof userEvent.setup>, alias: string, type: string) {
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  const aliases = screen.getAllByLabelText('Alias tài nguyên');
  const last = aliases[aliases.length - 1]!;
  await user.type(last, alias);
  const types = screen.getAllByLabelText('Loại tài nguyên');
  await user.selectOptions(types[types.length - 1]!, type);
}
const fillPostgres = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.type(screen.getByLabelText('Tham số database'), 'appdb');
  await user.type(screen.getByLabelText('Tham số username'), 'appuser');
};

describe('UC-16 dependency class from authoritative metadata (I04-07)', () => {
  it('offers the published classes with Vietnamese names, keeps default empty and never maps it', async () => {
    const user = userEvent.setup();
    const { puts } = serve();
    await newWorkload(user);
    await addDependency(user, 'db', 'postgres');
    const select = screen.getByLabelText('Cách cung cấp (class) của db');
    expect(select).toHaveValue('');
    expect(within(select).getAllByRole('option').map((option) => option.textContent)).toEqual(['Mặc định (default)', 'Dịch vụ managed AWS (cloud-managed)', 'Trong cluster (in-cluster)']);
    for (const label of [/module/i, /provider/i, /credential|xác thực/i]) expect(screen.queryByLabelText(label)).not.toBeInTheDocument();
    await fillPostgres(user);
    await user.click(save());
    expect(puts).toHaveLength(1);
    // default stays default: no class is written, none is chosen for the user.
    expect(resourcesOf(puts[0])?.db).toEqual({ type: 'postgres', id: expect.stringMatching(/^resource-/), params: { database: 'appdb', username: 'appuser' } });
  });

  it('serializes the chosen class and an automatically generated shared ID, and reads both back when the draft is edited', async () => {
    const user = userEvent.setup();
    const { puts } = serve();
    await newWorkload(user);
    await addDependency(user, 'db', 'postgres');
    await user.selectOptions(screen.getByLabelText('Cách cung cấp (class) của db'), 'cloud-managed');
    await fillPostgres(user);
    await user.click(save());
    const sharedId = resourcesOf(puts[0])?.db?.id;
    expect(sharedId).toEqual(expect.stringMatching(/^resource-/));
    expect(resourcesOf(puts[0])?.db).toEqual({ type: 'postgres', class: 'cloud-managed', id: sharedId, params: { database: 'appdb', username: 'appuser' } });
    // Opening the saved draft restores class, shared ID and params exactly.
    const score = puts[0]?.score as Record<string, unknown>;
    cleanup();
    vi.unstubAllGlobals();
    const second = serve({ workloads: [{ id: 'api', ready: true, score }] });
    render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
    expect(await screen.findByLabelText('Cách cung cấp (class) của db')).toHaveValue('cloud-managed');
    expect(screen.queryByLabelText(/ID dùng chung/)).not.toBeInTheDocument();
    expect(screen.getByLabelText('Tham số database')).toHaveValue('appdb');
    await user.clear(screen.getByLabelText('Ảnh Container'));
    await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:v2');
    await user.click(save());
    expect(resourcesOf(second.puts[0])?.db).toEqual({ type: 'postgres', class: 'cloud-managed', id: sharedId, params: { database: 'appdb', username: 'appuser' } });
  });

  it('keeps a custom or imported class and a shared ID untouched, offering it as typed and never switching it', async () => {
    const user = userEvent.setup();
    const score = { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/api:v1' } },
      resources: { cache: { type: 'redis', class: 'team-a', id: 'shared-cache' }, db: { type: 'postgres', class: 'default', id: 'orders-db', params: { database: 'appdb', username: 'appuser' } } } };
    const { puts } = serve({ workloads: [{ id: 'api', ready: true, score }] });
    render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
    const cache = await screen.findByLabelText('Cách cung cấp (class) của cache');
    expect(cache).toHaveValue('team-a');
    expect(within(cache).getByRole('option', { name: 'team-a (đã nhập, giữ nguyên)' })).toBeInTheDocument();
    expect(screen.getByText(/Class "team-a" không nằm trong metadata của loại redis/)).toBeInTheDocument();
    // "default" typed explicitly in the Score is a custom class name too, never remapped to a provider class.
    expect(screen.getByLabelText('Cách cung cấp (class) của db')).toHaveValue('default');
    await user.clear(screen.getByLabelText('Ảnh Container'));
    await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:v2');
    await user.click(save());
    expect(resourcesOf(puts[0])).toEqual({
      cache: { type: 'redis', class: 'team-a', id: 'shared-cache' },
      db: { type: 'postgres', class: 'default', id: 'orders-db', params: { database: 'appdb', username: 'appuser' } },
    });
  });

  it('does not rewrite a class when the Type changes and shows the mismatch instead', async () => {
    const user = userEvent.setup();
    serve();
    await newWorkload(user);
    await addDependency(user, 'db', 'postgres');
    await user.selectOptions(screen.getByLabelText('Cách cung cấp (class) của db'), 'cloud-managed');
    await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'vpc');
    expect(screen.getByLabelText('Cách cung cấp (class) của db')).toHaveValue('cloud-managed');
    expect(screen.getByText(/Class "cloud-managed" không nằm trong metadata của loại vpc/)).toBeInTheDocument();
    expect(screen.getByText(/Danh mục chưa công bố class nào cho loại vpc/)).toBeInTheDocument();
  });

  it('locks class controls and Save while metadata reloads or fails, keeps every edit and retries by GET only', async () => {
    const user = userEvent.setup();
    const calls: string[] = [];
    let fail = false;
    const { puts } = serve({ calls, types: () => (fail ? Response.json({ code: 'INTERNAL_ERROR' }, { status: 500 }) : Response.json({ resourceTypes: typeFixture })) });
    await newWorkload(user);
    await addDependency(user, 'db', 'postgres');
    await user.selectOptions(screen.getByLabelText('Cách cung cấp (class) của db'), 'in-cluster');
    await fillPostgres(user);
    fail = true;
    await user.click(screen.getByRole('button', { name: 'Làm mới metadata class' }));
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải metadata class' });
    expect(alert).toHaveTextContent('lựa chọn class đang bị khóa');
    expect(screen.getByLabelText('Cách cung cấp (class) của db')).toBeDisabled();
    expect(screen.getByLabelText('Cách cung cấp (class) của db')).toHaveValue('in-cluster');
    expect(screen.getByLabelText('Tham số database')).toHaveValue('appdb');
    await user.click(save());
    expect(await screen.findByText(/Chưa xác nhận được metadata class của tài nguyên/)).toBeInTheDocument();
    expect(puts).toHaveLength(0);
    fail = false;
    await user.click(within(alert).getByRole('button', { name: 'Thử lại tải metadata class' }));
    await vi.waitFor(() => expect(screen.getByLabelText('Cách cung cấp (class) của db')).toBeEnabled());
    expect(screen.getByLabelText('Cách cung cấp (class) của db')).toHaveValue('in-cluster');
    await user.click(save());
    expect(puts).toHaveLength(1);
    expect(resourcesOf(puts[0])?.db).toMatchObject({ class: 'in-cluster' });
    expect(calls.filter((entry) => entry.startsWith('PUT'))).toHaveLength(1);
  });

  it('blocks the whole editor with a retry when the initial metadata load fails, never an empty class list', async () => {
    const user = userEvent.setup();
    let fail = true;
    serve({ types: () => (fail ? Response.json({ code: 'INTERNAL_ERROR' }, { status: 500 }) : Response.json({ resourceTypes: typeFixture })) });
    render(<WorkloadEditorPage application={application} environment="staging" />);
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(/Không tải được|Thử lại|máy chủ/i);
    expect(screen.queryByLabelText('Cách cung cấp (class) của db')).not.toBeInTheDocument();
    fail = false;
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  });

  it('treats a Type without published classes as having none and still saves default', async () => {
    const user = userEvent.setup();
    const types: ResourceType[] = [{ key: 'postgres', inputs: [], outputs: [{ name: 'host' }] }];
    const { puts } = serve({ types: () => Response.json({ resourceTypes: types.map(({ key, inputs, outputs }) => ({ key, inputs, outputs, classes: [] })) }) });
    await newWorkload(user);
    await addDependency(user, 'db', 'postgres');
    expect(within(screen.getByLabelText('Cách cung cấp (class) của db')).getAllByRole('option')).toHaveLength(1);
    expect(screen.getByText(/Danh mục chưa công bố class nào cho loại postgres/)).toBeInTheDocument();
    await user.click(save());
    expect(resourcesOf(puts[0])?.db).toEqual({ type: 'postgres', id: expect.stringMatching(/^resource-/) });
  });

  // Absent or malformed metadata is unavailable, never a fake-empty class list.
  it.each([
    ['a body without resourceTypes', {}],
    ['a Type without classes', { resourceTypes: [{ key: 'postgres', inputs: [], outputs: [] }] }],
    ['classes that are not an array', { resourceTypes: [{ key: 'postgres', inputs: [], outputs: [], classes: 'in-cluster' }] }],
    ['a non-string class', { resourceTypes: [{ key: 'postgres', inputs: [], outputs: [], classes: ['in-cluster', 7] }] }],
  ])('treats %s on refresh as unavailable metadata that locks class controls and keeps edits', async (_name, body) => {
    const user = userEvent.setup();
    let malformed = false;
    const { puts } = serve({ types: () => Response.json(malformed ? body : { resourceTypes: typeFixture }) });
    await newWorkload(user);
    await addDependency(user, 'db', 'postgres');
    await user.selectOptions(screen.getByLabelText('Cách cung cấp (class) của db'), 'in-cluster');
    malformed = true;
    await user.click(screen.getByRole('button', { name: 'Làm mới metadata class' }));
    await screen.findByRole('alert', { name: 'Lỗi tải metadata class' });
    expect(screen.getByLabelText('Cách cung cấp (class) của db')).toBeDisabled();
    expect(screen.getByLabelText('Cách cung cấp (class) của db')).toHaveValue('in-cluster');
    await user.click(save());
    expect(puts).toHaveLength(0);
  });

  it('fails the first load of malformed metadata with a retry instead of an editor with no classes', async () => {
    serve({ types: () => Response.json({ resourceTypes: [{ key: 'postgres', inputs: [], outputs: [] }] }) });
    render(<WorkloadEditorPage application={application} environment="staging" />);
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    expect(save()).toBeDisabled();
  });

  it('ignores a late older refresh reply: the newest request decides the metadata state', async () => {
    const user = userEvent.setup();
    const gates: ((response: Response) => void)[] = [];
    let call = 0;
    serve({ types: () => { call += 1; return call === 1 ? Response.json({ resourceTypes: typeFixture }) : new Promise<Response>((resolve) => { gates.push(resolve); }); } });
    await newWorkload(user);
    await addDependency(user, 'db', 'postgres');
    await user.click(screen.getByRole('button', { name: 'Làm mới metadata class' }));
    await user.click(screen.getByRole('button', { name: 'Làm mới metadata class' }));
    await vi.waitFor(() => expect(gates).toHaveLength(2));
    gates[1]?.(Response.json({ resourceTypes: typeFixture }));
    await vi.waitFor(() => expect(screen.getByLabelText('Cách cung cấp (class) của db')).toBeEnabled());
    gates[0]?.(Response.json({ code: 'INTERNAL_ERROR' }, { status: 500 }));
    await new Promise((done) => setTimeout(done, 30));
    expect(screen.queryByRole('alert', { name: 'Lỗi tải metadata class' })).not.toBeInTheDocument();
    expect(screen.getByLabelText('Cách cung cấp (class) của db')).toBeEnabled();
  });
});

describe('ADR-016 workload class (Score metadata.class)', () => {
  it('writes metadata.class only when chosen and leaves a missing class missing', async () => {
    const user = userEvent.setup();
    const { puts } = serve();
    await newWorkload(user);
    await user.click(save());
    expect((puts[0]?.score as { metadata: Record<string, unknown> }).metadata).toEqual({ name: 'api' });
    await user.type(screen.getByLabelText('Class Workload'), 'gold');
    await user.click(save());
    expect((puts[1]?.score as { metadata: Record<string, unknown> }).metadata).toEqual({ name: 'api', class: 'gold' });
  });

  it('opens an imported class in the form, keeps it through an edit and never converts default', async () => {
    const user = userEvent.setup();
    for (const className of ['gold', 'default']) {
      cleanup();
      vi.unstubAllGlobals();
      const score = { apiVersion: 'score.dev/v1b1', metadata: { name: 'api', class: className }, containers: { main: { image: 'example.invalid/api:v1' } } };
      const { puts } = serve({ workloads: [{ id: 'api', ready: true, score }] });
      render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
      expect(await screen.findByLabelText('Class Workload')).toHaveValue(className);
      await user.clear(screen.getByLabelText('Ảnh Container'));
      await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:v2');
      await user.click(save());
      expect((puts[0]?.score as { metadata: Record<string, unknown> }).metadata).toEqual({ name: 'api', class: className });
    }
  });

  it('refuses an invalid class before saving and keeps the typed value', async () => {
    const user = userEvent.setup();
    const { puts } = serve();
    await newWorkload(user);
    await user.type(screen.getByLabelText('Class Workload'), 'Gold Class');
    await user.click(save());
    expect(await screen.findByRole('alert')).toHaveTextContent('Class Workload không hợp lệ');
    expect(puts).toEqual([]);
    expect(screen.getByLabelText('Class Workload')).toHaveValue('Gold Class');
  });

  it('keeps a Score whose metadata.class is not a valid identifier in the advanced Score editor instead of rewriting it', async () => {
    const score = { apiVersion: 'score.dev/v1b1', metadata: { name: 'api', class: 'Not Valid' }, containers: { main: { image: 'example.invalid/api:v1' } } };
    serve({ workloads: [{ id: 'api', ready: true, score }] });
    render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
    expect((await screen.findAllByText(/Nhập một file Score|Đã phân tích Workload/)).length).toBeGreaterThan(0);
    expect(screen.queryByLabelText('Class Workload')).not.toBeInTheDocument();
  });
});

it('reuses an existing shared database with a local alias and locks its configuration', async () => {
  const user = userEvent.setup();
  const spec = { type: 'postgres', class: 'in-cluster', id: 'existing-db', params: { database: 'appdb', username: 'appuser' } };
  const { puts } = serve({ workloads: [{ id: 'backend', ready: true, score: { resources: { db: spec } } }] });
  await newWorkload(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  await user.type(screen.getByLabelText('Alias tài nguyên'), 'store');
  await user.selectOptions(screen.getByLabelText('Nguồn tài nguyên'), 'shared:existing-db');
  expect(screen.getByLabelText('Alias tài nguyên')).toHaveValue('store');
  expect(screen.getByLabelText('Loại tài nguyên')).toBeDisabled();
  expect(screen.getByLabelText('Tham số database')).toHaveValue('appdb');
  expect(screen.getByLabelText('Tham số database')).toBeDisabled();
  await user.click(save());
  expect(resourcesOf(puts[0])?.store).toEqual(spec);
});

it('supports explicit private scope in advanced controls and keeps it private on edit', async () => {
  const user = userEvent.setup();
  const { puts } = serve();
  await newWorkload(user);
  await addDependency(user, 'db', 'postgres');
  await fillPostgres(user);
  await user.click(screen.getByRole('button', { name: 'Hiện nâng cao tài nguyên db' }));
  await user.click(screen.getByLabelText('Chỉ dùng cho workload này'));
  await user.click(save());
  expect(resourcesOf(puts[0])?.db).not.toHaveProperty('id');
  const score = puts[0]!.score;
  cleanup();
  const second = serve({ workloads: [{ id: 'api', ready: true, score }] });
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  await screen.findByLabelText('Ảnh Container');
  await user.type(screen.getByLabelText('Ảnh Container'), '-updated');
  await user.click(save());
  expect(resourcesOf(second.puts[0])?.db).not.toHaveProperty('id');
});

it('keeps separate generated identities for same-type resources and after validation failures', async () => {
  const user = userEvent.setup();
  const { puts } = serve();
  await newWorkload(user);
  await addDependency(user, 'first', 'redis');
  await addDependency(user, 'second', 'redis');
  await user.clear(screen.getByLabelText('Tên Workload'));
  await user.click(save());
  expect(puts).toHaveLength(0);
  await user.type(screen.getByLabelText('Tên Workload'), 'api');
  await user.click(save());
  const resources = resourcesOf(puts[0]);
  expect(resources.first?.id).toEqual(expect.stringMatching(/^resource-/));
  expect(resources.second?.id).not.toEqual(resources.first?.id);
});

it('retries a failed Save with the same generated resource identity', async () => {
  const user = userEvent.setup();
  const { puts } = serve();
  const fallback = vi.mocked(fetch).getMockImplementation()!;
  let failed: Record<string, unknown> | undefined;
  vi.mocked(fetch).mockImplementation(async (input, init) => {
    if (init?.method === 'PUT' && !failed) {
      failed = JSON.parse(String(init.body)) as Record<string, unknown>;
      return Response.json({ code: 'INTERNAL_ERROR' }, { status: 500 });
    }
    return fallback(input, init);
  });
  await newWorkload(user);
  await addDependency(user, 'cache', 'redis');
  await user.click(save());
  expect(await screen.findByRole('alert')).toBeInTheDocument();
  await user.click(save());
  expect(puts).toHaveLength(1);
  expect(puts[0]?.score.resources).toEqual((failed?.score as Record<string, unknown>).resources);
});

it('preserves scheduled execution and container overrides while editing an existing shared resource workload', async () => {
  const user = userEvent.setup();
  const execution = { version: 1, kind: 'CronJob', template: { id: 'scheduled', version: '1.0.0', sha256: 'a'.repeat(64) },
    schedule: { expression: '0 1 * * *', timeZone: 'UTC', concurrencyPolicy: 'Forbid', suspend: true } };
  const container = { image: 'example.invalid/job:v1', command: ['/bin/sh'], args: ['-c', 'run-job'], workingDir: '/app', startupProbe: { path: '/health', port: 8080 } };
  const resource = { type: 'redis', class: 'in-cluster', id: 'shared-cache' };
  const score = { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, workloadExecution: execution, containers: { main: container }, resources: { cache: resource } };
  const { puts } = serve({ workloads: [{ id: 'api', ready: true, score }] });
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  await screen.findByLabelText('Ảnh Container');
  expect(screen.getByLabelText('Nguồn tài nguyên')).toHaveValue('current');
  await user.clear(screen.getByLabelText('Ảnh Container'));
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/job:v2');
  await user.click(save());
  expect(puts[0]?.score.workloadExecution).toEqual(execution);
  expect(puts[0]?.score.containers).toEqual({ main: { ...container, image: 'example.invalid/job:v2' } });
  expect(resourcesOf(puts[0])?.cache).toEqual(resource);
});
