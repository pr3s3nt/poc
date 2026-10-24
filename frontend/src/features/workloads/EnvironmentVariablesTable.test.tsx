import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import type { ConfigKey } from '../configuration/api';
import type { ResourceType, Workload } from './api';
import type { Application } from '../../shared/types/application';
import { bothConfigured } from '../../test/targets';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: bothConfigured(), workloads: { staging: [], production: [] } };
const SECRET_VALUE = 'p4ss-never-shown';
const VARIABLE_VALUE = 'https://internal.example';
const catalogKeys: ConfigKey[] = [
  { name: 'API_URL', kind: 'VARIABLE', configured: true, value: VARIABLE_VALUE, usedBy: [] },
  { name: 'LOG_LEVEL', kind: 'VARIABLE', configured: true, value: 'debug', usedBy: [] },
  { name: 'DATABASE_PASSWORD', kind: 'SECRET', configured: true, value: SECRET_VALUE, usedBy: [] },
];
const postgres: ResourceType = { key: 'postgres', classes: [], inputs: [{ name: 'name', type: 'string' }], outputs: [{ name: 'host' }, { name: 'password', secret: true }] };

type Call = { method: string; url: string; body?: Record<string, unknown> };
type Options = {
  keys?: ConfigKey[];
  workloads?: Workload[];
  types?: ResourceType[];
  secretStoreKey?: string;
  stores?: { key: string; name: string; provider: string; status: string }[];
  create?: (body: Record<string, unknown>, call: number) => Response | Promise<Response>;
  put?: (call: number) => Response | Promise<Response>;
  parse?: Record<string, unknown>;
};

// Stub backend with a mutable configuration, so a create is visible to the next GET like the real one.
function serve(options: Options = {}) {
  const calls: Call[] = [];
  let keys = [...(options.keys ?? catalogKeys)];
  let version = 1;
  let store = options.secretStoreKey ?? 'vault-a';
  let creates = 0;
  const configuration = () => ({ applicationKey: 'catalog', environmentKey: 'staging', version, secretStoreKey: store || undefined, keys });
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    const method = init?.method ?? 'GET';
    const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
    calls.push({ method, url, body });
    if (url.endsWith('/configuration/keys') && method === 'POST') {
      creates += 1;
      if (options.create) return options.create(body!, creates);
      keys = [...keys, { name: String(body!.name), kind: body!.kind as ConfigKey['kind'], configured: true, usedBy: [], ...(body!.kind === 'VARIABLE' ? { value: String(body!.value) } : {}) }];
      version += 1;
      return Response.json(configuration(), { status: 201 });
    }
    if (url.endsWith('/configuration')) return Response.json(configuration());
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: options.stores ?? [] });
    if (url.endsWith('/secret-store') && method === 'PUT') { store = String(body!.secretStoreKey); return Response.json({ environment: { key: 'staging', version: 3, secretStoreKey: store, targetGeneration: 0, draftVersion: 0 }, changed: true, copiedSecrets: 0 }); }
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: (options.types ?? []).map((type) => ({ classes: [], ...type })) });
    if (url.endsWith('/workloads/parse')) return Response.json({ score: options.parse });
    if (method === 'PUT') return options.put ? options.put(calls.filter((call) => call.method === 'PUT').length) : Response.json({ draftVersion: 2, workloads: [] });
    return Response.json({ draftVersion: 1, workloads: options.workloads ?? [] });
  }));
  return {
    calls,
    puts: () => calls.filter((call) => call.method === 'PUT').map((call) => call.body as { score: Score; version: number }),
    creates: () => calls.filter((call) => call.method === 'POST' && call.url.endsWith('/configuration/keys')),
    configurationChanged: () => calls.filter((call) => call.url.includes('/configuration/keys/')),
  };
}

type Score = { containers: Partial<Record<string, { variables?: Record<string, string> }>>; resources?: Record<string, unknown> };
const save = () => screen.getByRole('button', { name: 'Lưu draft Workload' });
const table = (container = 'main') => within(screen.getByRole('region', { name: `Biến môi trường của container ${container}` }));
const addButton = (container = 'main') => table(container).getByRole('button', { name: '+ Thêm biến' });

async function newWorkload(user: ReturnType<typeof userEvent.setup>) {
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'api');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:test');
}

// Adds one row and fills name + source in the given container.
async function addRow(user: ReturnType<typeof userEvent.setup>, row: number, name: string, source: string, container = 'main') {
  await user.click(addButton(container));
  const scope = table(container);
  if (name) await user.type(scope.getByLabelText(`Tên biến, dòng ${row}`), name);
  if (source !== 'Giá trị nhập') await user.selectOptions(scope.getByLabelText(`Lấy giá trị từ, dòng ${row}`), source);
}

afterEach(() => vi.unstubAllGlobals());

// ── E01: one table, five sources ───────────────────────────────────────────

const dbWorkload: Workload = { id: 'db-api', ready: true, servicePorts: ['http'], score: { apiVersion: 'score.dev/v1b1', metadata: { name: 'db-api' }, containers: { main: { image: 'i' } }, service: { ports: { http: { port: 80 } } } } };

it('keeps one table per container with five sources and a new row that is a blank entered value', async () => {
  const api = serve({ workloads: [dbWorkload], types: [postgres] });
  const user = userEvent.setup();
  await newWorkload(user);
  expect(table().getByText('Chưa có biến môi trường')).toBeInTheDocument();
  await user.click(addButton());
  expect(table().getByLabelText('Tên biến, dòng 1')).toHaveFocus();
  expect(table().getByLabelText('Lấy giá trị từ, dòng 1')).toHaveDisplayValue('Giá trị nhập');
  expect(within(table().getByLabelText('Lấy giá trị từ, dòng 1')).getAllByRole('option').map((option) => option.textContent)).toEqual(['Giá trị nhập', 'Biến của ứng dụng', 'Bí mật', 'Tài nguyên', 'Workload khác']);
  expect(table().getByRole('columnheader', { name: 'Giá trị / Nguồn' })).toBeInTheDocument();
  expect(api.configurationChanged()).toHaveLength(0);
});

it('saves each source as its Score projection without copying a key value or touching keys', async () => {
  const api = serve({ workloads: [dbWorkload], types: [postgres] });
  const user = userEvent.setup();
  await newWorkload(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  await user.type(screen.getByLabelText('Alias tài nguyên'), 'db');
  await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'postgres');
  await addRow(user, 1, 'LOG', 'Giá trị nhập');
  await user.type(table().getByLabelText('Giá trị nhập, dòng 1'), 'debug');
  await addRow(user, 2, 'REGION', 'Biến của ứng dụng');
  await user.selectOptions(table().getByLabelText('Biến của ứng dụng, dòng 2'), 'API_URL');
  await addRow(user, 3, 'TOKEN', 'Bí mật');
  await user.selectOptions(table().getByLabelText('Bí mật, dòng 3'), 'DATABASE_PASSWORD');
  expect(table().getByText('Đã cấu hình')).toBeInTheDocument();
  await addRow(user, 4, 'DB_HOST', 'Tài nguyên');
  await user.selectOptions(table().getByLabelText('Tài nguyên, dòng 4'), 'db');
  await user.selectOptions(table().getByLabelText('Output, dòng 4'), 'host');
  await addRow(user, 5, 'DB_PASSWORD', 'Tài nguyên');
  await user.selectOptions(table().getByLabelText('Tài nguyên, dòng 5'), 'db');
  expect(within(table().getByLabelText('Output, dòng 5')).getByRole('option', { name: 'password · Bí mật' })).toBeInTheDocument();
  await user.selectOptions(table().getByLabelText('Output, dòng 5'), 'password');
  await addRow(user, 6, 'BACKEND_URL', 'Workload khác');
  await user.selectOptions(table().getByLabelText('Workload, dòng 6'), 'db-api');
  await user.selectOptions(table().getByLabelText('Cổng Service, dòng 6'), 'http');
  expect(document.body.textContent).not.toContain(SECRET_VALUE);
  expect(document.body.textContent).not.toContain(VARIABLE_VALUE);
  await user.click(save());
  const score = api.puts()[0]!.score;
  expect(score.containers.main?.variables).toEqual({
    LOG: 'debug', REGION: '${resources.env.API_URL}', TOKEN: '${resources.env.DATABASE_PASSWORD}',
    DB_HOST: '${resources.db.host}', DB_PASSWORD: '${resources.db.password}', BACKEND_URL: '${resources.svc_db-api_http.url}',
  });
  expect(score.resources).toMatchObject({ env: { type: 'environment' }, 'svc_db-api_http': { type: 'service', params: { workload: 'db-api', port: 'http' } } });
  expect(JSON.stringify(api.puts()[0])).not.toContain(SECRET_VALUE);
  expect(api.configurationChanged()).toHaveLength(0);
  expect(api.creates()).toHaveLength(0);
});

it('keeps the name and drops the old source payload when the source changes, and never turns a secret into a literal', async () => {
  const api = serve();
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'PGPASSWORD', 'Bí mật');
  await user.selectOptions(table().getByLabelText('Bí mật, dòng 1'), 'DATABASE_PASSWORD');
  await user.selectOptions(table().getByLabelText('Lấy giá trị từ, dòng 1'), 'Giá trị nhập');
  expect(table().getByLabelText('Tên biến, dòng 1')).toHaveValue('PGPASSWORD');
  expect(table().getByLabelText('Giá trị nhập, dòng 1')).toHaveValue('');
  await user.selectOptions(table().getByLabelText('Lấy giá trị từ, dòng 1'), 'Biến của ứng dụng');
  expect(table().getByLabelText('Biến của ứng dụng, dòng 1')).toHaveValue('');
  await user.selectOptions(table().getByLabelText('Lấy giá trị từ, dòng 1'), 'Giá trị nhập');
  await user.type(table().getByLabelText('Giá trị nhập, dòng 1'), 'plain');
  await user.click(save());
  const score = api.puts()[0]!.score;
  expect(score.containers.main?.variables).toEqual({ PGPASSWORD: 'plain' });
  expect(score.resources).toBeUndefined();
  expect(JSON.stringify(api.puts()[0])).not.toContain('DATABASE_PASSWORD');
});

it('removes only the mapping of a row and leaves keys, dependencies and other rows', async () => {
  const api = serve({ types: [postgres] });
  const user = userEvent.setup();
  await newWorkload(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  await user.type(screen.getByLabelText('Alias tài nguyên'), 'db');
  await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'postgres');
  await addRow(user, 1, 'A', 'Biến của ứng dụng');
  await user.selectOptions(table().getByLabelText('Biến của ứng dụng, dòng 1'), 'API_URL');
  await addRow(user, 2, 'B', 'Giá trị nhập');
  await user.click(table().getByRole('button', { name: 'Xóa biến A, dòng 1' }));
  expect(table().queryByLabelText('Biến của ứng dụng, dòng 1')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Alias tài nguyên')).toHaveValue('db');
  await user.click(save());
  const score = api.puts()[0]!.score;
  expect(score.containers.main?.variables).toEqual({ B: '' });
  expect(score.resources).toEqual({ db: { type: 'postgres' } });
  expect(api.configurationChanged()).toHaveLength(0);
});

// ── E02: ordinary values ──────────────────────────────────────────────────

it('saves empty, whitespace and placeholder-looking values exactly and escapes ${...} typed in the form', async () => {
  const api = serve();
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'EMPTY', 'Giá trị nhập');
  await addRow(user, 2, 'PADDED', 'Giá trị nhập');
  await user.type(table().getByLabelText('Giá trị nhập, dòng 2'), '  two  ');
  await addRow(user, 3, 'TEMPLATE', 'Giá trị nhập');
  await user.click(table().getByLabelText('Giá trị nhập, dòng 3'));
  await user.paste('x ${resources.env.API_URL} y $${already}');
  expect(table().getByText(/Giá trị thường được lưu cùng cấu hình/)).toBeInTheDocument();
  await user.click(save());
  expect(api.puts()[0]!.score.containers.main?.variables).toEqual({ EMPTY: '', PADDED: '  two  ', TEMPLATE: 'x $${resources.env.API_URL} y $$${already}' });
  expect(api.puts()[0]!.score.resources).toBeUndefined();
});

it('shows a stored value back as typed and writes it unchanged when edited on the form', async () => {
  const stored: Workload = { id: 'api', ready: true, score: { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/api:v1', variables: { EMPTY: '', PADDED: '  two  ', TEMPLATE: 'x $${resources.env.API_URL} y' } } } } };
  const api = serve({ workloads: [stored] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  await screen.findByRole('heading', { name: 'Sửa api' });
  expect(table().getByLabelText('Giá trị nhập, dòng 1')).toHaveValue('');
  expect(table().getByLabelText('Giá trị nhập, dòng 2')).toHaveValue('  two  ');
  expect(table().getByLabelText('Giá trị nhập, dòng 3')).toHaveValue('x ${resources.env.API_URL} y');
  await user.type(screen.getByLabelText('Ảnh Container'), '1');
  await user.click(save());
  expect(api.puts()[0]!.score.containers.main?.variables).toEqual(stored.score!.containers && (stored.score as unknown as Score).containers.main?.variables);
});

it('blocks Save for a NUL character in a value without echoing it', async () => {
  const api = serve();
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'BAD', 'Giá trị nhập');
  fireInput(table().getByLabelText('Giá trị nhập, dòng 1') as HTMLInputElement, 'a\u0000b');
  await user.click(save());
  expect(await screen.findAllByText(/không được chứa ký tự NUL/)).not.toHaveLength(0);
  expect(api.puts()).toHaveLength(0);
});

function fireInput(element: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
  act(() => { setter.call(element, value); element.dispatchEvent(new Event('input', { bubbles: true })); });
}

it('keeps a Score with text mixed into a reference on the advanced Score path instead of rewriting it', async () => {
  const mixed = { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'i', variables: { URL: 'https://${resources.env.API_URL}/x' } } }, resources: { env: { type: 'environment' } } };
  const api = serve({ workloads: [{ id: 'api', ready: true, score: mixed }] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  expect(await screen.findByText(/các trường mà form không thể giữ nguyên/)).toBeInTheDocument();
  await user.click(save());
  expect(api.puts()[0]!.score).toEqual(mixed);
});

// ── E03: keys, aliases and duplicates ─────────────────────────────────────

it('never selects a key on its own, suggests the name only while it is empty and lets one key feed several rows', async () => {
  const api = serve();
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, '', 'Biến của ứng dụng');
  expect(table().getByLabelText('Biến của ứng dụng, dòng 1')).toHaveValue('');
  await user.selectOptions(table().getByLabelText('Biến của ứng dụng, dòng 1'), 'API_URL');
  expect(table().getByLabelText('Tên biến, dòng 1')).toHaveValue('API_URL');
  await addRow(user, 2, 'BACKEND_URL', 'Biến của ứng dụng');
  await user.selectOptions(table().getByLabelText('Biến của ứng dụng, dòng 2'), 'API_URL');
  expect(table().getByLabelText('Tên biến, dòng 2')).toHaveValue('BACKEND_URL');
  await user.click(save());
  expect(api.puts()[0]!.score.containers.main?.variables).toEqual({ API_URL: '${resources.env.API_URL}', BACKEND_URL: '${resources.env.API_URL}' });
});

it('allows one name in two containers but blocks Save for a duplicate inside a container', async () => {
  const api = serve();
  const user = userEvent.setup();
  await newWorkload(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm Container' }));
  await user.type(screen.getAllByLabelText('Ảnh Container')[1]!, 'example.invalid/worker:test');
  await addRow(user, 1, 'LEVEL', 'Giá trị nhập', 'main');
  await addRow(user, 1, 'LEVEL', 'Giá trị nhập', 'container-2');
  await addRow(user, 2, 'LEVEL', 'Giá trị nhập', 'container-2');
  expect(await within(screen.getByRole('region', { name: 'Biến môi trường của container container-2' })).findAllByRole('alert')).toHaveLength(2);
  await user.click(save());
  expect(api.puts()).toHaveLength(0);
  await user.clear(table('container-2').getByLabelText('Tên biến, dòng 2'));
  await user.type(table('container-2').getByLabelText('Tên biến, dòng 2'), 'OTHER');
  await user.click(save());
  expect(api.puts()[0]!.score.containers['container-2']?.variables).toEqual({ LEVEL: '', OTHER: '' });
});

const twoContainers: Workload = { id: 'api', ready: true, score: {
  apiVersion: 'score.dev/v1b1', metadata: { name: 'api' },
  containers: {
    main: { image: 'example.invalid/api:v1', variables: { API_URL: '${resources.env.API_URL}', PGPASSWORD: '${resources.env.DATABASE_PASSWORD}', DB_PASSWORD: '${resources.env.DATABASE_PASSWORD}', DB_HOST: '${resources.db.host}', LEVEL: 'debug' } },
    worker: { image: 'example.invalid/worker:v1', variables: { API_URL: '${resources.env.API_URL}', DB_HOST: '${resources.db.host}' } },
  },
  resources: { env: { type: 'environment' }, db: { type: 'postgres' } },
} };

it('restores sources, aliases, repeated keys and containers on Edit and writes them back unchanged', async () => {
  const api = serve({ workloads: [twoContainers], types: [postgres] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  await screen.findByRole('heading', { name: 'Sửa api' });
  expect(table().getByLabelText('Lấy giá trị từ, dòng 1')).toHaveDisplayValue('Biến của ứng dụng');
  expect(table().getByLabelText('Lấy giá trị từ, dòng 2')).toHaveDisplayValue('Bí mật');
  expect(table().getByLabelText('Bí mật, dòng 2')).toHaveValue('DATABASE_PASSWORD');
  expect(table().getByLabelText('Bí mật, dòng 3')).toHaveValue('DATABASE_PASSWORD');
  expect(table().getByLabelText('Lấy giá trị từ, dòng 4')).toHaveDisplayValue('Tài nguyên');
  expect(table().getByLabelText('Output, dòng 4')).toHaveValue('host');
  expect(table().getByLabelText('Giá trị nhập, dòng 5')).toHaveValue('debug');
  expect(table('worker').getByLabelText('Tên biến, dòng 1')).toHaveValue('API_URL');
  await user.type(screen.getAllByLabelText('Ảnh Container')[1]!, '1');
  await user.click(save());
  const score = api.puts()[0]!.score;
  expect(score.containers.main?.variables).toEqual((twoContainers.score as unknown as Score).containers.main?.variables);
  expect(score.containers.worker?.variables).toEqual((twoContainers.score as unknown as Score).containers.worker?.variables);
});

// ── E04: create a key without leaving the editor ──────────────────────────

it('creates a variable in place with the create-only API, selects it on the row and keeps the draft unsaved', async () => {
  const api = serve();
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'REGION', 'Biến của ứng dụng');
  await user.click(table().getByRole('button', { name: 'Tạo biến' }));
  const dialog = screen.getByRole('dialog', { name: 'Tạo biến' });
  expect(within(dialog).getByLabelText('Tên key')).toHaveValue('REGION');
  expect(within(dialog).getByText(/Key được lưu cho staging; workload áp dụng sau Lưu → Xem trước → Triển khai\./)).toBeInTheDocument();
  await user.type(within(dialog).getByLabelText('Giá trị'), 'eu-west');
  await user.click(within(dialog).getByRole('button', { name: 'Tạo và chọn' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(api.creates()).toHaveLength(1);
  expect(api.creates()[0]!.body).toEqual({ name: 'REGION', kind: 'VARIABLE', value: 'eu-west', version: 1 });
  expect(table().getByLabelText('Biến của ứng dụng, dòng 1')).toHaveValue('REGION');
  expect(table().getByLabelText('Tên biến, dòng 1')).toHaveValue('REGION');
  expect(table().getByLabelText('Biến của ứng dụng, dòng 1')).toHaveFocus();
  expect(api.puts()).toHaveLength(0);
  expect(api.calls.some((call) => call.method === 'PUT')).toBe(false);
});

it('creates a masked secret, never shows or stores its value and keeps the key when the workload is cancelled', async () => {
  const api = serve();
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'API_TOKEN', 'Bí mật');
  await user.click(table().getByRole('button', { name: 'Tạo bí mật' }));
  const dialog = screen.getByRole('dialog', { name: 'Tạo bí mật' });
  const value = within(dialog).getByLabelText('Giá trị bí mật');
  expect(value).toHaveAttribute('type', 'password');
  expect(value).toHaveValue('');
  await user.type(value, SECRET_VALUE);
  await user.click(within(dialog).getByRole('button', { name: 'Tạo và chọn' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(api.creates()[0]!.body).toEqual({ name: 'API_TOKEN', kind: 'SECRET', value: SECRET_VALUE, version: 1 });
  expect(table().getByLabelText('Bí mật, dòng 1')).toHaveValue('API_TOKEN');
  expect(document.body.textContent).not.toContain(SECRET_VALUE);
  await user.click(screen.getByRole('button', { name: 'Hủy' }));
  await user.click(save()).catch(() => undefined);
  expect(api.calls.some((call) => call.method === 'DELETE')).toBe(false);
  expect(JSON.stringify(api.calls.filter((call) => call.method === 'PUT'))).not.toContain(SECRET_VALUE);
});

it('reports a duplicate key without overwriting, reviews the catalog and selects the existing key only on request', async () => {
  const api = serve({ create: () => Response.json({ code: 'DUPLICATE' }, { status: 409 }) });
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'LOG_LEVEL', 'Biến của ứng dụng');
  await user.click(table().getByRole('button', { name: 'Tạo biến' }));
  const dialog = screen.getByRole('dialog', { name: 'Tạo biến' });
  await user.type(within(dialog).getByLabelText('Giá trị'), 'trace');
  await user.click(within(dialog).getByRole('button', { name: 'Tạo và chọn' }));
  expect(await within(dialog).findByRole('alert')).toBeInTheDocument();
  await within(dialog).findByRole('button', { name: 'Dùng key hiện có' });
  expect(api.creates()).toHaveLength(1);
  expect(table().getByLabelText('Biến của ứng dụng, dòng 1')).toHaveValue('');
  // The create button was disabled while the request ran, so focus may be on the page: Escape still closes the dialog.
  await user.keyboard('{Escape}');
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(table().getByLabelText('Biến của ứng dụng, dòng 1')).toHaveValue('');
  await user.click(table().getByRole('button', { name: 'Tạo biến' }));
  await user.type(within(screen.getByRole('dialog')).getByLabelText('Giá trị'), 'trace');
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Tạo và chọn' }));
  await user.click(await within(screen.getByRole('dialog')).findByRole('button', { name: 'Dùng key hiện có' }));
  expect(table().getByLabelText('Biến của ứng dụng, dòng 1')).toHaveValue('LOG_LEVEL');
  expect(api.calls.some((call) => call.method === 'PUT')).toBe(false);
});

it('does not replay a create whose outcome is unknown: it asks for a review first', async () => {
  const api = serve({ create: () => { throw new TypeError('network'); } });
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'NEW_KEY', 'Biến của ứng dụng');
  await user.click(table().getByRole('button', { name: 'Tạo biến' }));
  const dialog = screen.getByRole('dialog', { name: 'Tạo biến' });
  await user.type(within(dialog).getByLabelText('Giá trị'), 'v');
  await user.click(within(dialog).getByRole('button', { name: 'Tạo và chọn' }));
  expect(await within(dialog).findByText(/Chưa rõ key đã được tạo hay chưa/)).toBeInTheDocument();
  expect(within(dialog).getByRole('button', { name: 'Tạo và chọn' })).toBeDisabled();
  expect(api.creates()).toHaveLength(1);
  await user.click(within(dialog).getByRole('button', { name: 'Tải lại cấu hình' }));
  await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Tạo và chọn' })).toBeEnabled());
  expect(api.creates()).toHaveLength(1);
});

it('keeps the form and does not retry on a stale or busy create', async () => {
  const replies = [Response.json({ code: 'STALE_VERSION' }, { status: 409 }), Response.json({ code: 'ENVIRONMENT_BUSY' }, { status: 409 })];
  const api = serve({ create: (_, call) => replies[call - 1]! });
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'NEW_KEY', 'Biến của ứng dụng');
  await user.click(table().getByRole('button', { name: 'Tạo biến' }));
  const dialog = screen.getByRole('dialog', { name: 'Tạo biến' });
  await user.type(within(dialog).getByLabelText('Giá trị'), 'v');
  await user.click(within(dialog).getByRole('button', { name: 'Tạo và chọn' }));
  expect(await within(dialog).findByRole('alert')).toBeInTheDocument();
  expect(within(dialog).getByLabelText('Giá trị')).toHaveValue('v');
  expect(api.creates()).toHaveLength(1);
  await user.click(within(dialog).getByRole('button', { name: 'Tạo và chọn' }));
  await waitFor(() => expect(api.creates()).toHaveLength(2));
  expect(within(dialog).getByLabelText('Giá trị')).toHaveValue('v');
});

it('traps focus, closes on Escape and returns focus to the control that opened the dialog', async () => {
  serve();
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'X', 'Biến của ứng dụng');
  const opener = table().getByRole('button', { name: 'Tạo biến' });
  await user.click(opener);
  const dialog = screen.getByRole('dialog', { name: 'Tạo biến' });
  expect(within(dialog).getByLabelText('Tên key')).toHaveFocus();
  for (let i = 0; i < 6; i += 1) await user.tab();
  expect(dialog.contains(document.activeElement)).toBe(true);
  await user.keyboard('{Escape}');
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(opener).toHaveFocus();
});

// ── E05: initial Secret Store ─────────────────────────────────────────────

it('lets a missing store be chosen and saved in the dialog before the secret is created', async () => {
  const api = serve({ secretStoreKey: '', stores: [{ key: 'vault-a', name: 'Vault A', provider: 'vault', status: 'READY' }, { key: 'vault-b', name: 'Vault B', provider: 'vault', status: 'PENDING' }] });
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'TOKEN', 'Bí mật');
  await user.click(table().getByRole('button', { name: 'Tạo bí mật' }));
  const dialog = screen.getByRole('dialog', { name: 'Tạo bí mật' });
  await user.type(within(dialog).getByLabelText('Giá trị bí mật'), 'v');
  expect(within(dialog).getByRole('button', { name: 'Tạo và chọn' })).toBeDisabled();
  expect(await within(dialog).findByRole('option', { name: 'Vault A (vault-a)' })).toBeInTheDocument();
  expect(within(dialog).queryByRole('option', { name: /Vault B/ })).not.toBeInTheDocument();
  await user.selectOptions(within(dialog).getByLabelText('Kho bí mật'), 'vault-a');
  await user.click(within(dialog).getByRole('button', { name: 'Lưu kho bí mật' }));
  await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Tạo và chọn' })).toBeEnabled());
  expect(api.calls.some((call) => call.method === 'PUT' && call.url.endsWith('/secret-store'))).toBe(true);
  await user.click(within(dialog).getByRole('button', { name: 'Tạo và chọn' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(api.creates()[0]!.body).toMatchObject({ name: 'TOKEN', kind: 'SECRET' });
});

it('explains that no ready store exists and still allows the other rows to be edited', async () => {
  serve({ secretStoreKey: '', stores: [] });
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'TOKEN', 'Bí mật');
  await user.click(table().getByRole('button', { name: 'Tạo bí mật' }));
  expect(await within(screen.getByRole('dialog')).findByText('Chưa có kho bí mật sẵn sàng.')).toBeInTheDocument();
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Hủy' }));
  await addRow(user, 2, 'OTHER', 'Giá trị nhập');
  expect(table().getByLabelText('Giá trị nhập, dòng 2')).toBeEnabled();
});

// ── E06: add a dependency in place ────────────────────────────────────────

it('adds a dependency to the draft only, selects its alias, leaves the output to the user and makes it usable in another container', async () => {
  const api = serve({ types: [postgres] });
  const user = userEvent.setup();
  await newWorkload(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm Container' }));
  await addRow(user, 1, 'DB_HOST', 'Tài nguyên');
  await user.click(table().getByRole('button', { name: 'Thêm tài nguyên' }));
  const dialog = screen.getByRole('dialog', { name: 'Thêm tài nguyên' });
  await user.type(within(dialog).getByLabelText('Alias tài nguyên'), 'db');
  await user.selectOptions(within(dialog).getByLabelText('Loại tài nguyên'), 'postgres');
  await user.type(within(dialog).getByLabelText('Tham số name'), 'orders');
  await user.click(within(dialog).getByRole('button', { name: 'Thêm và chọn' }));
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(table().getByLabelText('Tài nguyên, dòng 1')).toHaveValue('db');
  expect(table().getByLabelText('Output, dòng 1')).toHaveValue('');
  expect(table().getByLabelText('Output, dòng 1')).toHaveFocus();
  expect(screen.getByLabelText('Alias tài nguyên')).toHaveValue('db');
  expect(api.calls.filter((call) => call.method !== 'GET')).toHaveLength(0);
  expect(JSON.stringify(api.calls)).not.toContain('provision');
  await addRow(user, 1, 'ORDERS_DB', 'Tài nguyên', 'container-2');
  await user.selectOptions(table('container-2').getByLabelText('Tài nguyên, dòng 1'), 'db');
  await user.selectOptions(table('container-2').getByLabelText('Output, dòng 1'), 'host');
  await user.selectOptions(table().getByLabelText('Output, dòng 1'), 'host');
  await user.type(screen.getAllByLabelText('Ảnh Container')[1]!, 'example.invalid/w:1');
  await user.click(save());
  const score = api.puts()[0]!.score;
  expect(score.resources).toEqual({ db: { type: 'postgres', params: { name: 'orders' } } });
  expect(score.containers['container-2']?.variables).toEqual({ ORDERS_DB: '${resources.db.host}' });
});

it('adds nothing on Cancel and rejects a taken or reserved alias', async () => {
  const api = serve({ types: [postgres] });
  const user = userEvent.setup();
  await newWorkload(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  await user.type(screen.getByLabelText('Alias tài nguyên'), 'db');
  await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'postgres');
  await addRow(user, 1, 'X', 'Tài nguyên');
  await user.click(table().getByRole('button', { name: 'Thêm tài nguyên' }));
  let dialog = screen.getByRole('dialog', { name: 'Thêm tài nguyên' });
  await user.type(within(dialog).getByLabelText('Alias tài nguyên'), 'db');
  await user.selectOptions(within(dialog).getByLabelText('Loại tài nguyên'), 'postgres');
  await user.click(within(dialog).getByRole('button', { name: 'Thêm và chọn' }));
  expect(await within(dialog).findByText('Workload đã có tài nguyên db.')).toBeInTheDocument();
  await user.clear(within(dialog).getByLabelText('Alias tài nguyên'));
  await user.type(within(dialog).getByLabelText('Alias tài nguyên'), 'env');
  await user.click(within(dialog).getByRole('button', { name: 'Thêm và chọn' }));
  expect(await within(dialog).findByText(/dành riêng/)).toBeInTheDocument();
  await user.click(within(dialog).getByRole('button', { name: 'Hủy' }));
  dialog = undefined as never;
  expect(screen.getAllByLabelText('Alias tài nguyên')).toHaveLength(1);
  expect(table().getByLabelText('Tài nguyên, dòng 1')).toHaveValue('');
  await user.click(save());
  expect(api.puts()).toHaveLength(0);
});

// ── E07/E08: missing targets stay visible ────────────────────────────────

it('keeps a missing key, alias, output and Service port on its row, blocks Save and never retargets', async () => {
  const broken: Workload = { id: 'api', ready: true, score: {
    apiVersion: 'score.dev/v1b1', metadata: { name: 'api' },
    containers: { main: { image: 'i', variables: { GONE: '${resources.env.REMOVED_KEY}', ALIAS: '${resources.cache.host}', OUT: '${resources.db.removed}', PEER: '${resources.svc_peer_http.url}' } } },
    resources: { env: { type: 'environment' }, db: { type: 'postgres' }, cache: { type: 'postgres' }, svc_peer_http: { type: 'service', params: { workload: 'peer', port: 'http' } } },
  } };
  const peer: Workload = { id: 'peer', ready: true, servicePorts: ['grpc'], score: { apiVersion: 'score.dev/v1b1', metadata: { name: 'peer' }, containers: { main: { image: 'i' } } } };
  const doc = broken.score as Record<string, unknown>;
  delete (doc.resources as Record<string, unknown>).cache;
  const api = serve({ workloads: [broken, peer], types: [postgres] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  await screen.findByRole('heading', { name: 'Sửa api' });
  expect(table().getByLabelText('Biến của ứng dụng, dòng 1')).toHaveDisplayValue('REMOVED_KEY (không còn)');
  expect(table().getByLabelText('Tài nguyên, dòng 2')).toHaveDisplayValue('cache (không còn)');
  expect(table().getByLabelText('Output, dòng 3')).toHaveDisplayValue('removed (không còn)');
  expect(table().getByLabelText('Cổng Service, dòng 4')).toHaveDisplayValue('http (không còn)');
  expect(table().getAllByRole('alert')).toHaveLength(4);
  await user.type(screen.getByLabelText('Ảnh Container'), '1');
  await user.click(save());
  expect(api.puts()).toHaveLength(0);
  expect(table().getByLabelText('Tên biến, dòng 1')).toHaveValue('GONE');
});

it('offers only other workloads of the Environment that are not pending deletion and have a Service port', async () => {
  const peers: Workload[] = [
    { id: 'api', ready: true, servicePorts: ['http'], score: { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'i' } } } },
    { id: 'good', ready: true, servicePorts: ['http', 'admin'] },
    { id: 'doomed', ready: true, state: 'PENDING_DELETE', servicePorts: ['http'] },
    { id: 'portless', ready: true },
  ];
  serve({ workloads: peers });
  const user = userEvent.setup();
  await newWorkload(user);
  await user.clear(screen.getByLabelText('Tên Workload'));
  await user.type(screen.getByLabelText('Tên Workload'), 'api');
  await addRow(user, 1, 'PEER', 'Workload khác');
  expect(within(table().getByLabelText('Workload, dòng 1')).getAllByRole('option').map((option) => option.textContent)).toEqual(['Chọn workload', 'good']);
  await user.selectOptions(table().getByLabelText('Workload, dòng 1'), 'good');
  expect(within(table().getByLabelText('Cổng Service, dòng 1')).getAllByRole('option').map((option) => option.textContent)).toEqual(['Chọn cổng', 'http', 'admin']);
});

// ── E09: catalog states and late replies ─────────────────────────────────

it('shows a loading or failed key catalog as such, not as an empty list, and keeps literal rows saveable', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ code: 'INTERNAL' }, { status: 500 });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (init?.method === 'PUT') return Response.json({ draftVersion: 2, workloads: [] });
    return Response.json({ draftVersion: 1, workloads: [] });
  }));
  const user = userEvent.setup();
  await newWorkload(user);
  expect(await screen.findByRole('button', { name: 'Thử tải lại key' })).toBeInTheDocument();
  await addRow(user, 1, 'X', 'Biến của ứng dụng');
  expect(table().getByLabelText('Biến của ứng dụng, dòng 1')).toBeDisabled();
  expect(table().getByLabelText('Biến của ứng dụng, dòng 1')).toHaveDisplayValue('Không tải được');
  expect(table().getByRole('button', { name: 'Tạo biến' })).toBeDisabled();
  await user.selectOptions(table().getByLabelText('Lấy giá trị từ, dòng 1'), 'Giá trị nhập');
  await user.click(save());
  expect(screen.queryByText(/Chưa chọn key|chưa có biến của ứng dụng/)).not.toBeInTheDocument();
});

type Gate = { resolve: (response: Response) => void };
function gate(gates: Gate[]): Promise<Response> { return new Promise<Response>((resolve) => { gates.push({ resolve }); }); }

it('ignores a late key catalog from the previous Environment', async () => {
  const gates: Gate[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/staging/configuration')) return gate(gates);
    if (url.endsWith('/configuration')) return Response.json({ version: 1, keys: [{ name: 'PROD_ONLY', kind: 'VARIABLE', configured: true, usedBy: [] }] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    return Response.json({ draftVersion: 1, workloads: [] });
  }));
  const user = userEvent.setup();
  const view = render(<WorkloadEditorPage application={application} environment="staging" />);
  await waitFor(() => expect(gates).toHaveLength(1));
  view.rerender(<WorkloadEditorPage application={application} environment="production" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await addRow(user, 1, 'K', 'Biến của ứng dụng');
  expect(await within(table().getByLabelText('Biến của ứng dụng, dòng 1')).findByRole('option', { name: 'PROD_ONLY' })).toBeInTheDocument();
  await act(async () => { gates[0]!.resolve(Response.json({ version: 1, keys: catalogKeys })); await new Promise((resolve) => setTimeout(resolve, 0)); });
  expect(within(table().getByLabelText('Biến của ứng dụng, dòng 1')).queryByRole('option', { name: 'API_URL' })).not.toBeInTheDocument();
});

it('applies the late reply of a running create to the row that opened the dialog and keeps the dialog open until then', async () => {
  const gates: Gate[] = [];
  const api = serve({ create: () => gate(gates) });
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'FIRST', 'Biến của ứng dụng');
  await user.click(table().getByRole('button', { name: 'Tạo biến' }));
  const dialog = screen.getByRole('dialog', { name: 'Tạo biến' });
  await user.type(within(dialog).getByLabelText('Giá trị'), 'v');
  await user.click(within(dialog).getByRole('button', { name: 'Tạo và chọn' }));
  await waitFor(() => expect(gates).toHaveLength(1));
  // The dialog cannot be left while the create is running; the reply lands on the original row only.
  expect(within(dialog).getByRole('button', { name: /Đang tạo/ })).toBeDisabled();
  await act(async () => {
    gates[0]!.resolve(Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 2, secretStoreKey: 'vault-a', keys: [...catalogKeys, { name: 'FIRST', kind: 'VARIABLE', configured: true, value: 'v', usedBy: [] }] }, { status: 201 }));
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  expect(table().getByLabelText('Biến của ứng dụng, dòng 1')).toHaveValue('FIRST');
  expect(api.puts()).toHaveLength(0);
});

it('drops the reply of a create started in the previous Environment: no key, row or draft of the new scope changes', async () => {
  const gates: Gate[] = [];
  const api = serve({ create: () => gate(gates) });
  const user = userEvent.setup();
  const view = render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await addRow(user, 1, 'FIRST', 'Biến của ứng dụng');
  await user.click(table().getByRole('button', { name: 'Tạo biến' }));
  const dialog = screen.getByRole('dialog', { name: 'Tạo biến' });
  await user.type(within(dialog).getByLabelText('Giá trị'), 'v');
  await user.click(within(dialog).getByRole('button', { name: 'Tạo và chọn' }));
  await waitFor(() => expect(gates).toHaveLength(1));
  view.rerender(<WorkloadEditorPage application={application} environment="production" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await addRow(user, 1, 'OTHER', 'Biến của ứng dụng');
  await act(async () => {
    gates[0]!.resolve(Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 2, keys: [...catalogKeys, { name: 'FIRST', kind: 'VARIABLE', configured: true, value: 'v', usedBy: [] }] }, { status: 201 }));
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(table().getByLabelText('Biến của ứng dụng, dòng 1')).toHaveValue('');
  expect(within(table().getByLabelText('Biến của ứng dụng, dòng 1')).queryByRole('option', { name: 'FIRST' })).not.toBeInTheDocument();
  expect(table().getByLabelText('Tên biến, dòng 1')).toHaveValue('OTHER');
  expect(api.puts()).toHaveLength(0);
});

it('starts a different Environment with a fresh form and closes the dialog of the previous one', async () => {
  serve();
  const user = userEvent.setup();
  const view = render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'api');
  await addRow(user, 1, 'X', 'Bí mật');
  await user.click(table().getByRole('button', { name: 'Tạo bí mật' }));
  expect(screen.getByRole('dialog')).toBeInTheDocument();
  view.rerender(<WorkloadEditorPage application={application} environment="production" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Tên Workload')).toHaveValue('');
  expect(table().getByText('Chưa có biến môi trường')).toBeInTheDocument();
});

const parsedScore = { apiVersion: 'score.dev/v1b1', metadata: { name: 'importer' }, containers: { main: { image: 'example.invalid/i:v1', variables: { LEVEL: 'debug', PGPASSWORD: '${resources.env.DATABASE_PASSWORD}' } } }, resources: { env: { type: 'environment' } } };

it('shows an imported Score in the same table: literals as entered values, references by source', async () => {
  serve({ parse: parsedScore });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.click(screen.getByRole('button', { name: 'Nhập Score' }));
  const content = 'apiVersion: score.dev/v1b1\n';
  await user.upload(screen.getByLabelText('File Score'), Object.assign(new File([content], 'score.yaml'), { text: async () => content }));
  await screen.findByText(/Đã phân tích Workload/);
  await user.click(screen.getByRole('button', { name: 'Nhập bằng form' }));
  expect(table().getByLabelText('Giá trị nhập, dòng 1')).toHaveValue('debug');
  expect(table().getByLabelText('Bí mật, dòng 2')).toHaveValue('DATABASE_PASSWORD');
});

it('ignores a late Score import parse response from the previous Environment', async () => {
  const gates: Gate[] = [];
  serve();
  const fallback = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation(async (input, init) => String(input).endsWith('/workloads/parse') ? gate(gates) : fallback(input, init));
  const user = userEvent.setup();
  const view = render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.click(screen.getByRole('button', { name: 'Nhập Score' }));
  const content = 'apiVersion: score.dev/v1b1\n';
  await user.upload(screen.getByLabelText('File Score'), Object.assign(new File([content], 'score.yaml'), { text: async () => content }));
  await waitFor(() => expect(gates).toHaveLength(1));
  view.rerender(<WorkloadEditorPage application={application} environment="production" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await act(async () => { gates[0]!.resolve(Response.json({ score: parsedScore })); await new Promise((resolve) => setTimeout(resolve, 0)); });
  await user.click(screen.getByRole('button', { name: 'Nhập Score' }));
  expect(screen.queryByText(/Đã phân tích Workload/)).not.toBeInTheDocument();
});

it.each([
  ['an error', () => Response.json({ code: 'INTERNAL' }, { status: 503 })],
  ['a conflict', () => Response.json({ code: 'STALE_VERSION' }, { status: 409 })],
  ['a success', () => Response.json({ draftVersion: 2, workloads: [] })],
])('ignores %s from a Save started in the previous Environment', async (_, reply) => {
  const gates: Gate[] = [];
  const api = serve({ put: () => gate(gates) });
  const user = userEvent.setup();
  window.history.pushState({}, '', '/editor-under-test');
  const view = render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'api');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:test');
  await addRow(user, 1, 'KEEP', 'Giá trị nhập');
  await user.click(save());
  await waitFor(() => expect(gates).toHaveLength(1));
  view.rerender(<WorkloadEditorPage application={application} environment="production" />);
  await screen.findByRole('heading', { name: 'Thêm Workload' });
  await act(async () => { gates[0]!.resolve(reply()); await new Promise((resolve) => setTimeout(resolve, 0)); });
  expect(window.location.pathname).toBe('/editor-under-test');
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(save()).toBeEnabled();
  expect(table().getByText('Chưa có biến môi trường')).toBeInTheDocument();
  expect(api.puts()).toHaveLength(1);
});

// ── Names that match Object members stay own properties ──────────────────

it('serializes variable names that match Object prototype members as own container variables', async () => {
  const keys: ConfigKey[] = ['__proto__', 'constructor', 'toString'].map((name) => ({ name, kind: 'VARIABLE', configured: true, value: 'v', usedBy: [] }));
  const api = serve({ keys });
  const user = userEvent.setup();
  await newWorkload(user);
  for (const [index, key] of keys.entries()) {
    await addRow(user, index + 1, '', 'Biến của ứng dụng');
    await user.selectOptions(table().getByLabelText(`Biến của ứng dụng, dòng ${index + 1}`), key.name);
  }
  await user.click(save());
  const variables = api.puts()[0]!.score.containers.main?.variables ?? {};
  expect(Object.keys(variables)).toEqual(['__proto__', 'constructor', 'toString']);
  expect(Object.getOwnPropertyDescriptor(variables, '__proto__')?.value).toBe('${resources.env.__proto__}');
});

// ── E12: English ─────────────────────────────────────────────────────────

it('renders the table, source choices and dialog names from the English catalog', async () => {
  const { setCatalogs } = await import('../../shared/i18n');
  const { readFileSync } = await import('node:fs');
  const { resolve } = await import('node:path');
  const read = (name: string) => JSON.parse(readFileSync(resolve(__dirname, '../../../public/locales', name), 'utf8')) as Record<string, string>;
  const vi_ = read('vi.json');
  setCatalogs('en', read('en.json'), vi_);
  try {
    serve();
    const user = userEvent.setup();
    render(<WorkloadEditorPage application={application} environment="staging" />);
    await screen.findByRole('heading', { name: 'Basic information' });
    const region = within(screen.getByRole('region', { name: 'Environment variables of container main' }));
    await user.click(region.getByRole('button', { name: '+ Add variable' }));
    expect(region.getByRole('columnheader', { name: 'Get value from' })).toBeInTheDocument();
    expect(within(region.getByLabelText('Get value from, row 1')).getAllByRole('option').map((option) => option.textContent)).toEqual(['Entered value', 'Application variable', 'Secret', 'Resource', 'Other workload']);
    await user.selectOptions(region.getByLabelText('Get value from, row 1'), 'Secret');
    await user.click(region.getByRole('button', { name: 'Create secret' }));
    const dialog = screen.getByRole('dialog', { name: 'Create secret' });
    expect(within(dialog).getByRole('button', { name: 'Create and select' })).toBeInTheDocument();
    expect(within(dialog).getByText(/The key is saved for staging/)).toBeInTheDocument();
  } finally {
    setCatalogs('vi', read('vi.json'));
  }
});
