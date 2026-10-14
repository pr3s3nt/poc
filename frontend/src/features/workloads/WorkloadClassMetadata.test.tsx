import { cleanup, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import type { ResourceType, Workload } from './api';
import type { Application } from '../../shared/types/application';
import { bothConfigured } from '../../test/targets';
import { typeFixture } from '../../test/types';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: bothConfigured('internal-cluster'), workloads: { staging: [], production: [] } };
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
    expect(screen.getByText(/Developer không chọn module Terraform, provider hay thông tin xác thực/)).toBeInTheDocument();
    for (const label of [/module/i, /provider/i, /credential|xác thực/i]) expect(screen.queryByLabelText(label)).not.toBeInTheDocument();
    await fillPostgres(user);
    await user.click(save());
    expect(puts).toHaveLength(1);
    // default stays default: no class is written, none is chosen for the user.
    expect(resourcesOf(puts[0])?.db).toEqual({ type: 'postgres', params: { database: 'appdb', username: 'appuser' } });
  });

  it('serializes the chosen class and an optional shared ID, and reads both back when the draft is edited', async () => {
    const user = userEvent.setup();
    const { puts } = serve();
    await newWorkload(user);
    await addDependency(user, 'db', 'postgres');
    await user.selectOptions(screen.getByLabelText('Cách cung cấp (class) của db'), 'cloud-managed');
    await user.type(screen.getByLabelText('ID dùng chung của db'), 'orders-db');
    await fillPostgres(user);
    await user.click(save());
    expect(resourcesOf(puts[0])?.db).toEqual({ type: 'postgres', class: 'cloud-managed', id: 'orders-db', params: { database: 'appdb', username: 'appuser' } });
    // Opening the saved draft restores class, shared ID and params exactly.
    const score = puts[0]?.score as Record<string, unknown>;
    cleanup();
    vi.unstubAllGlobals();
    const second = serve({ workloads: [{ id: 'api', ready: true, score }] });
    render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
    expect(await screen.findByLabelText('Cách cung cấp (class) của db')).toHaveValue('cloud-managed');
    expect(screen.getByLabelText('ID dùng chung của db')).toHaveValue('orders-db');
    expect(screen.getByLabelText('Tham số database')).toHaveValue('appdb');
    await user.clear(screen.getByLabelText('Ảnh Container'));
    await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:v2');
    await user.click(save());
    expect(resourcesOf(second.puts[0])?.db).toEqual({ type: 'postgres', class: 'cloud-managed', id: 'orders-db', params: { database: 'appdb', username: 'appuser' } });
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
    expect(resourcesOf(puts[0])?.db).toEqual({ type: 'postgres' });
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
