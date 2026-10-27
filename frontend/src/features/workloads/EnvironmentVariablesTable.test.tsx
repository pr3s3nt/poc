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

async function addRow(user: ReturnType<typeof userEvent.setup>, row: number, name: string, _source = 'Giá trị nhập', container = 'main') {
  await user.click(addButton(container));
  if (name) await user.type(table(container).getByLabelText(`Tên biến, dòng ${row}`), name);
}
async function picker(user: ReturnType<typeof userEvent.setup>, row = 1, container = 'main') {
  const scope = table(container);
  await user.click(scope.queryByRole('button', { name: `Chọn nguồn, dòng ${row}` }) ?? scope.getByRole('button', { name: `Đổi nguồn, dòng ${row}` }));
  return within(screen.getByRole('dialog', { name: 'Chọn giá trị có sẵn' }));
}
async function reference(user: ReturnType<typeof userEvent.setup>, row: number, label: string, container = 'main') {
  const dialog = await picker(user, row, container);
  await user.click(dialog.getByRole('button', { name: label }));
}
async function create(user: ReturnType<typeof userEvent.setup>, kind: string, row = 1) {
  const dialog = await picker(user, row);
  await user.click(dialog.getByRole('button', { name: kind }));
  return within(screen.getByRole('dialog', { name: kind }));
}
afterEach(() => vi.unstubAllGlobals());
type Gate = { resolve(response: Response): void };
const gate = (gates: Gate[]) => new Promise<Response>((resolve) => gates.push({ resolve }));
const dbWorkload: Workload = { id: 'db-api', ready: true, servicePorts: ['http'] };

it('adds a directly editable name/value row without a source column and focuses its name', async () => {
  const api = serve(); const user = userEvent.setup(); await newWorkload(user);
  await user.click(addButton());
  expect(table().getByLabelText('Tên biến, dòng 1')).toHaveFocus();
  expect(table().getByLabelText('Giá trị nhập, dòng 1')).toHaveValue('');
  expect(table().queryByRole('combobox')).not.toBeInTheDocument();
  expect(table().getByRole('columnheader', { name: 'Giá trị' })).toBeInTheDocument();
  expect(api.creates()).toHaveLength(0);
});

it('projects literal, shared Variable/Secret, secret output and Service without copying values', async () => {
  const api = serve({ workloads: [dbWorkload], types: [postgres] }); const user = userEvent.setup(); await newWorkload(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  await user.type(screen.getByLabelText('Alias tài nguyên'), 'db'); await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'postgres');
  await addRow(user, 1, 'LOG'); await user.type(table().getByLabelText('Giá trị nhập, dòng 1'), 'debug');
  await addRow(user, 2, 'REGION'); await reference(user, 2, 'Ứng dụng / API_URL');
  await addRow(user, 3, 'TOKEN'); await reference(user, 3, 'Ứng dụng / DATABASE_PASSWORD Bí mật');
  await addRow(user, 4, 'DB_HOST'); await reference(user, 4, 'db.host');
  await addRow(user, 5, 'DB_PASSWORD'); await reference(user, 5, 'db.password Bí mật');
  await addRow(user, 6, 'BACKEND_URL'); await reference(user, 6, 'db-api / http');
  expect(table().getByLabelText('Bí mật, dòng 3')).toBeChecked(); expect(table().getByLabelText('Bí mật, dòng 3')).toBeDisabled();
  expect(table().getByLabelText('Bí mật, dòng 5')).toBeChecked();
  expect(document.body.textContent).not.toContain(SECRET_VALUE); expect(document.body.textContent).not.toContain(VARIABLE_VALUE);
  await user.click(save());
  expect(api.puts()[0]!.score.containers.main?.variables).toEqual({ LOG:'debug', REGION:'${resources.env.API_URL}', TOKEN:'${resources.env.DATABASE_PASSWORD}', DB_HOST:'${resources.db.host}', DB_PASSWORD:'${resources.db.password}', BACKEND_URL:'${resources.svc_db-api_http.url}' });
  expect(api.creates()).toHaveLength(0); expect(api.configurationChanged()).toHaveLength(0);
});

it('searches only scoped shared keys, declared outputs and eligible Services, never local rows', async () => {
  serve({workloads:[dbWorkload,{id:'api',ready:true,servicePorts:['self']},{id:'deleted',ready:true,state:'PENDING_DELETE',servicePorts:['http']},{id:'no-service',ready:true}]});
  const user=userEvent.setup();await newWorkload(user);await addRow(user,1,'LOCAL_ONLY');
  const dialog=await picker(user);
  expect(dialog.queryByRole('button',{name:/LOCAL_ONLY/})).not.toBeInTheDocument();
  expect(dialog.queryByRole('button',{name:/api \/ self|deleted \/ http|no-service/})).not.toBeInTheDocument();
  expect(dialog.getByRole('button',{name:'db-api / http'})).toBeInTheDocument();
  await user.type(dialog.getByRole('searchbox'),'API_URL');
  expect(dialog.getByRole('button',{name:'Ứng dụng / API_URL'})).toBeInTheDocument();
  expect(dialog.queryByRole('button',{name:'Ứng dụng / LOG_LEVEL'})).not.toBeInTheDocument();
});

it('preserves an entered name, suggests only for a blank Application key and permits repeated references',async()=>{
  const api=serve();const user=userEvent.setup();await newWorkload(user);
  await addRow(user,1,'');await reference(user,1,'Ứng dụng / API_URL');expect(table().getByLabelText('Tên biến, dòng 1')).toHaveValue('API_URL');
  await addRow(user,2,'ALIAS');await reference(user,2,'Ứng dụng / API_URL');await user.click(save());
  expect(api.puts()[0]!.score.containers.main?.variables).toEqual({API_URL:'${resources.env.API_URL}',ALIAS:'${resources.env.API_URL}'});
});

it('clears old source payload when returning to direct entry and leaves shared values untouched',async()=>{
  const api=serve();const user=userEvent.setup();await newWorkload(user);await addRow(user,1,'TOKEN');await reference(user,1,'Ứng dụng / DATABASE_PASSWORD Bí mật');
  const dialog=await picker(user);await user.click(dialog.getByRole('button',{name:'Nhập giá trị trực tiếp'}));
  expect(table().getByLabelText('Giá trị nhập, dòng 1')).toHaveValue('');await user.type(table().getByLabelText('Giá trị nhập, dòng 1'),'plain');await user.click(save());
  expect(api.puts()[0]!.score.containers.main?.variables).toEqual({TOKEN:'plain'});expect(api.puts()[0]!.score.resources).toBeUndefined();expect(api.configurationChanged()).toHaveLength(0);
});

it('removes one mapping without deleting the key or other aliases',async()=>{
  const api=serve();const user=userEvent.setup();await newWorkload(user);await addRow(user,1,'A');await reference(user,1,'Ứng dụng / API_URL');await addRow(user,2,'B');await reference(user,2,'Ứng dụng / API_URL');
  await user.click(table().getByRole('button',{name:'Xóa biến A, dòng 1'}));await user.click(save());
  expect(api.puts()[0]!.score.containers.main?.variables).toEqual({B:'${resources.env.API_URL}'});expect(api.calls.some(c=>c.method==='DELETE')).toBe(false);
});

it('creates a shared variable with create-only semantics and keeps the parent draft unsaved',async()=>{
  const api=serve();const user=userEvent.setup();await newWorkload(user);await addRow(user,1,'REGION');const dialog=await create(user,'Tạo biến');
  expect(dialog.getByLabelText('Tên key')).toHaveValue('REGION');await user.type(dialog.getByLabelText('Giá trị'),'eu-west');await user.click(dialog.getByRole('button',{name:'Tạo và chọn'}));
  await waitFor(()=>expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(api.creates()[0]!.body).toEqual({name:'REGION',kind:'VARIABLE',value:'eu-west',version:1});expect(table().getByRole('button',{name:'Đổi nguồn, dòng 1'})).toHaveTextContent('Ứng dụng / REGION');expect(api.puts()).toHaveLength(0);
});

it('opens masked Secret creation from the checkbox, retains app/env scope and never serializes its value',async()=>{
  const api=serve();const user=userEvent.setup();await newWorkload(user);await addRow(user,1,'API_TOKEN');await user.click(table().getByLabelText('Bí mật, dòng 1'));
  const dialog=within(screen.getByRole('dialog',{name:'Tạo bí mật'}));const value=dialog.getByLabelText('Giá trị bí mật');expect(value).toHaveAttribute('type','password');expect(value).toHaveValue('');
  await user.type(value,SECRET_VALUE);await user.click(dialog.getByRole('button',{name:'Tạo và chọn'}));await waitFor(()=>expect(screen.queryByRole('dialog')).not.toBeInTheDocument());await user.click(save());
  expect(api.creates()[0]!.body).toEqual({name:'API_TOKEN',kind:'SECRET',value:SECRET_VALUE,version:1});expect(api.puts()[0]!.score.containers.main?.variables).toEqual({API_TOKEN:'${resources.env.API_TOKEN}'});expect(JSON.stringify(api.puts())).not.toContain(SECRET_VALUE);expect(document.body.textContent).not.toContain(SECRET_VALUE);expect(api.calls.some(c=>c.method==='DELETE')).toBe(false);
});

it('Cancel Secret creation preserves the previous literal without publishing it as a Secret',async()=>{
  const api=serve();const user=userEvent.setup();await newWorkload(user);await addRow(user,1,'X');await user.type(table().getByLabelText('Giá trị nhập, dòng 1'),'ordinary');await user.click(table().getByLabelText('Bí mật, dòng 1'));
  expect(within(screen.getByRole('dialog')).getByLabelText('Giá trị bí mật')).toHaveValue('');await user.keyboard('{Escape}');expect(table().getByRole('button', { name: 'Chọn nguồn, dòng 1' })).toHaveFocus();expect(table().getByLabelText('Giá trị nhập, dòng 1')).toHaveValue('ordinary');expect(api.creates()).toHaveLength(0);
});

it('duplicate create never overwrites and selecting existing remains explicit',async()=>{
  const api=serve({create:()=>Response.json({code:'DUPLICATE'},{status:409})});const user=userEvent.setup();await newWorkload(user);await addRow(user,1,'LOG_LEVEL');const dialog=await create(user,'Tạo biến');
  await user.type(dialog.getByLabelText('Giá trị'),'trace');await user.click(dialog.getByRole('button',{name:'Tạo và chọn'}));await dialog.findByRole('alert');expect(api.creates()).toHaveLength(1);expect(api.puts()).toHaveLength(0);
  expect(table().getByLabelText('Giá trị nhập, dòng 1')).toHaveValue('');await user.click(dialog.getByRole('button',{name:'Dùng key hiện có'}));expect(table().getByRole('button',{name:'Đổi nguồn, dòng 1'})).toHaveTextContent('Ứng dụng / LOG_LEVEL');expect(api.configurationChanged()).toHaveLength(0);
});

it('Escape closes source search and returns focus without losing a typed value',async()=>{
  serve();const user=userEvent.setup();await newWorkload(user);await addRow(user,1,'KEEP');await user.type(table().getByLabelText('Giá trị nhập, dòng 1'),'text');const opener=table().getByRole('button',{name:'Chọn nguồn, dòng 1'});await user.click(opener);const dialog=screen.getByRole('dialog');
  for(let i=0;i<20;i++)await user.tab();expect(dialog.contains(document.activeElement)).toBe(true);await user.keyboard('{Escape}');expect(opener).toHaveFocus();expect(table().getByLabelText('Giá trị nhập, dòng 1')).toHaveValue('text');
});

it('keeps unavailable imported references visible and blocks Save without retargeting',async()=>{
  const api=serve({workloads:[{id:'api',ready:true,score:{apiVersion:'score.dev/v1b1',metadata:{name:'api'},containers:{main:{image:'i',variables:{MISSING:'${resources.env.REMOVED}',OUTPUT:'${resources.db.missing}'}}},resources:{env:{type:'environment'},db:{type:'postgres'}}}}],types:[postgres]});const user=userEvent.setup();render(<WorkloadEditorPage application={application} environment="staging" workloadId="api"/>);await screen.findByRole('heading',{name:'Thông tin cơ bản'});
  expect(table().getByRole('button',{name:'Đổi nguồn, dòng 1'})).toHaveTextContent('Ứng dụng / REMOVED');expect(table().getByRole('button',{name:'Đổi nguồn, dòng 2'})).toHaveTextContent('db.missing');await user.click(save());expect(api.puts()).toHaveLength(0);expect(screen.getAllByRole('alert').length).toBeGreaterThan(0);
});

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


it('keeps a Score with text mixed into a reference on the advanced Score path instead of rewriting it', async () => {
  const mixed = { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'i', variables: { URL: 'https://${resources.env.API_URL}/x' } } }, resources: { env: { type: 'environment' } } };
  const api = serve({ workloads: [{ id: 'api', ready: true, score: mixed }] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  expect(await screen.findByText(/các trường mà form không thể giữ nguyên/)).toBeInTheDocument();
  await user.click(save());
  expect(api.puts()[0]!.score).toEqual(mixed);
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


it('blocks duplicate resource aliases entered in the main resource editor', async () => {
  const api = serve({ types: [postgres] });
  const user = userEvent.setup();
  await newWorkload(user);
  for (let index = 0; index < 2; index++) {
    await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
    await user.type(screen.getAllByLabelText('Alias tài nguyên')[index]!, 'db');
    await user.selectOptions(screen.getAllByLabelText('Loại tài nguyên')[index]!, 'postgres');
  }
  await user.click(save());
  expect(api.puts()).toHaveLength(0);
  expect(screen.getByRole('alert')).toHaveTextContent('Workload đã có tài nguyên db.');
});


it('does not replay a create whose outcome is unknown: it asks for a review first', async () => {
  const api = serve({ create: () => { throw new TypeError('network'); } });
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'NEW_KEY', 'Biến của ứng dụng');
  await create(user, 'Tạo biến');
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
  await create(user, 'Tạo biến');
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


it('lets a missing store be chosen and saved in the dialog before the secret is created', async () => {
  const api = serve({ secretStoreKey: '', stores: [{ key: 'vault-a', name: 'Vault A', provider: 'vault', status: 'READY' }, { key: 'vault-b', name: 'Vault B', provider: 'vault', status: 'PENDING' }] });
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'TOKEN', 'Bí mật');
  await create(user, 'Tạo bí mật');
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
  await create(user, 'Tạo bí mật');
  expect(await within(screen.getByRole('dialog')).findByText('Chưa có kho bí mật sẵn sàng.')).toBeInTheDocument();
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Hủy' }));
  await addRow(user, 2, 'OTHER', 'Giá trị nhập');
  expect(table().getByLabelText('Giá trị nhập, dòng 2')).toBeEnabled();
});


it('applies the late reply of a running create to the row that opened the dialog and keeps the dialog open until then', async () => {
  const gates: Gate[] = [];
  const api = serve({ create: () => gate(gates) });
  const user = userEvent.setup();
  await newWorkload(user);
  await addRow(user, 1, 'FIRST', 'Biến của ứng dụng');
  await create(user, 'Tạo biến');
  const dialog = screen.getByRole('dialog', { name: 'Tạo biến' });
  await user.type(within(dialog).getByLabelText('Giá trị'), 'v');
  await user.click(within(dialog).getByRole('button', { name: 'Tạo và chọn' }));
  await waitFor(() => expect(gates).toHaveLength(1));
  // The dialog cannot be left while the create is running; the reply lands on the original row only.
  expect(within(dialog).getByRole('button', { name: /Đang tạo/ })).toBeDisabled();
  await user.tab();
  expect(dialog === document.activeElement || dialog.contains(document.activeElement)).toBe(true);
  await user.keyboard('{Escape}');
  expect(dialog).toBeInTheDocument();
  await act(async () => {
    gates[0]!.resolve(Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 2, secretStoreKey: 'vault-a', keys: [...catalogKeys, { name: 'FIRST', kind: 'VARIABLE', configured: true, value: 'v', usedBy: [] }] }, { status: 201 }));
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  expect(table().getByRole('button', { name: 'Đổi nguồn, dòng 1' })).toHaveTextContent('Ứng dụng / FIRST');
  expect(api.puts()).toHaveLength(0);
});


it('drops the reply of a create started in the previous Environment: no key, row or draft of the new scope changes', async () => {
  const gates: Gate[] = [];
  const api = serve({ create: () => gate(gates) });
  const user = userEvent.setup();
  const view = render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await addRow(user, 1, 'FIRST', 'Biến của ứng dụng');
  await create(user, 'Tạo biến');
  const dialog = screen.getByRole('dialog', { name: 'Tạo biến' });
  await user.type(within(dialog).getByLabelText('Giá trị'), 'v');
  await user.click(within(dialog).getByRole('button', { name: 'Tạo và chọn' }));
  await waitFor(() => expect(gates).toHaveLength(1));
  view.rerender(<WorkloadEditorPage application={application} environment="production" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await addRow(user, 1, 'OTHER', 'Biến của ứng dụng');
  await create(user, 'Tạo biến');
  await act(async () => {
    gates[0]!.resolve(Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 2, keys: [...catalogKeys, { name: 'FIRST', kind: 'VARIABLE', configured: true, value: 'v', usedBy: [] }] }, { status: 201 }));
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  expect(screen.getByRole('dialog', { name: 'Tạo biến' })).toBeInTheDocument();
  expect(within(screen.getByRole('dialog')).getByLabelText('Tên key')).toHaveValue('OTHER');
  expect(table().getByLabelText('Giá trị nhập, dòng 1')).toHaveValue('');
  expect(table().queryByRole('button', { name: 'Đổi nguồn, dòng 1' })).not.toBeInTheDocument();
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
  await create(user, 'Tạo bí mật');
  expect(screen.getByRole('dialog')).toBeInTheDocument();
  view.rerender(<WorkloadEditorPage application={application} environment="production" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Tên Workload')).toHaveValue('');
  expect(table().getByText('Chưa có biến môi trường')).toBeInTheDocument();
});


function fireInput(element: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
  act(() => { setter.call(element, value); element.dispatchEvent(new Event('input', { bubbles: true })); });
}

const parsedScore = { apiVersion: 'score.dev/v1b1', metadata: { name: 'importer' }, containers: { main: { image: 'example.invalid/i:v1', variables: { LEVEL: 'debug', PGPASSWORD: '${resources.env.DATABASE_PASSWORD}' } } }, resources: { env: { type: 'environment' } } };

it('renders direct entry and the shared source picker in English', async () => {
  const { setCatalogs } = await import('../../shared/i18n');
  const { readFileSync } = await import('node:fs'); const { resolve } = await import('node:path');
  const read = (file: string) => JSON.parse(readFileSync(resolve(__dirname, '../../../public/locales', file), 'utf8')) as Record<string, string>;
  setCatalogs('en', read('en.json'), read('vi.json'));
  try {
    serve(); const user = userEvent.setup(); render(<WorkloadEditorPage application={application} environment="staging" />);
    await screen.findByRole('heading', { name: 'Basic information' });
    const region = within(screen.getByRole('region', { name: 'Environment variables of container main' }));
    await user.click(region.getByRole('button', { name: '+ Add variable' }));
    expect(region.getByRole('columnheader', { name: 'Value' })).toBeInTheDocument();
    expect(region.queryByRole('combobox')).not.toBeInTheDocument();
    await user.click(region.getByRole('button', { name: 'Choose source, row 1' }));
    const dialog = within(screen.getByRole('dialog', { name: 'Choose an existing value' }));
    expect(dialog.getByRole('button', { name: 'Application / DATABASE_PASSWORD Secret' })).toBeInTheDocument();
    await user.click(dialog.getByRole('button', { name: 'Create secret' }));
    expect(within(screen.getByRole('dialog', { name: 'Create secret' })).getByRole('button', { name: 'Create and select' })).toBeInTheDocument();
  } finally { setCatalogs('vi', read('vi.json')); }
});

it('preserves imported literal and repeated shared references in the form', async () => {
  const score = { ...parsedScore, containers: { main: { image: 'i', variables: { LEVEL: 'debug', A: '${resources.env.API_URL}', B: '${resources.env.API_URL}', TOKEN: '${resources.env.DATABASE_PASSWORD}' } } } };
  const api = serve({ workloads: [{ id: 'importer', ready: true, score }] }); const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="importer" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  expect(table().getByLabelText('Giá trị nhập, dòng 1')).toHaveValue('debug');
  expect(table().getByRole('button', { name: 'Đổi nguồn, dòng 2' })).toHaveTextContent('Ứng dụng / API_URL');
  expect(table().getByRole('button', { name: 'Đổi nguồn, dòng 3' })).toHaveTextContent('Ứng dụng / API_URL');
  expect(table().getByLabelText('Bí mật, dòng 4')).toBeChecked();
  await user.click(save()); expect(api.puts()[0]!.score.containers.main?.variables).toEqual(score.containers.main.variables);
});

it('adds a dependency inline, explicitly selects its output and uses it from another container', async () => {
  const api = serve({ types: [postgres] }); const user = userEvent.setup(); await newWorkload(user); await addRow(user, 1, 'DB_HOST');
  const pickerDialog = await picker(user); await user.click(pickerDialog.getByRole('button', { name: 'Thêm tài nguyên' }));
  const dialog = within(screen.getByRole('dialog', { name: 'Thêm tài nguyên' }));
  await user.type(dialog.getByLabelText('Alias tài nguyên'), 'db'); await user.selectOptions(dialog.getByLabelText('Loại tài nguyên'), 'postgres');
  await user.click(dialog.getByRole('button', { name: 'Thêm và chọn' }));
  const outputs = within(screen.getByRole('dialog', { name: 'Chọn giá trị có sẵn' }));
  expect(outputs.getByRole('button', { name: 'db.host' })).toBeInTheDocument();
  expect(api.puts()).toHaveLength(0); await user.click(outputs.getByRole('button', { name: 'db.host' }));
  await user.click(screen.getByRole('button', { name: '+ Thêm Container' }));
  await user.type(screen.getAllByLabelText('Ảnh Container')[1]!, 'i');
  await addRow(user, 1, 'HOST', 'Giá trị nhập', 'container-2'); await reference(user, 1, 'db.host', 'container-2');
  await user.click(save()); expect(api.puts()[0]!.score.containers['container-2']?.variables).toEqual({ HOST: '${resources.db.host}' });
});

it('requires explicitly reselecting an output after changing dependency type', async () => {
  const api = serve({ types: [postgres, { ...postgres, key: 'mysql' }] }); const user = userEvent.setup(); await newWorkload(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' })); await user.type(screen.getByLabelText('Alias tài nguyên'), 'db'); await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'postgres');
  await addRow(user, 1, 'DB_HOST'); await reference(user, 1, 'db.host');
  await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'mysql');
  expect(table().getByRole('button', { name: 'Đổi nguồn, dòng 1' })).toHaveTextContent('db.host'); await user.click(save());
  expect(api.puts()).toHaveLength(0); expect(screen.getAllByRole('alert').some((alert) => alert.textContent?.includes('đã đổi loại'))).toBe(true);
  await reference(user, 1, 'db.host'); await user.click(save()); expect(api.puts()).toHaveLength(1);
});

it('retains local edits on catalog failure and distinguishes it from an empty list', async () => {
  serve(); const fallback = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation(async (input, init) => String(input).endsWith('/configuration') ? Response.json({ code: 'INTERNAL' }, { status: 503 }) : fallback(input, init));
  const user = userEvent.setup(); await newWorkload(user); await addRow(user, 1, 'LOCAL'); await user.type(table().getByLabelText('Giá trị nhập, dòng 1'), 'keep');
  const dialog = await picker(user); expect(dialog.getByRole('status')).toHaveTextContent('Không tải được');
  expect(dialog.getByRole('button', { name: 'Tạo bí mật' })).toBeDisabled(); await user.keyboard('{Escape}');
  expect(table().getByLabelText('Giá trị nhập, dòng 1')).toHaveValue('keep');
});

it('serializes Object prototype names as own variables rather than dropping mappings', async () => {
  const keys: ConfigKey[] = ['__proto__', 'constructor', 'toString'].map((name) => ({ name, kind: 'VARIABLE', configured: true, value: 'v', usedBy: [] }));
  const api = serve({ keys }); const user = userEvent.setup(); await newWorkload(user);
  for (const [i, key] of keys.entries()) { await addRow(user, i + 1, ''); await reference(user, i + 1, `Ứng dụng / ${key.name}`); }
  await user.click(save());
  const variables = api.puts()[0]!.score.containers.main?.variables ?? {};
  expect(Object.keys(variables)).toEqual(['__proto__', 'constructor', 'toString']);
  expect(Object.getOwnPropertyDescriptor(variables, '__proto__')?.value).toBe('${resources.env.__proto__}');
});

it('does not leak a late catalog response from staging into production source search', async () => {
  const gates: Gate[] = []; serve(); const fallback = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation(async (input, init) => {
    if (String(input).endsWith('/staging/configuration')) return gate(gates);
    if (String(input).endsWith('/production/configuration')) return Response.json({ version: 1, keys: [{ name: 'PROD_ONLY', kind: 'VARIABLE', configured: true, usedBy: [] }] });
    return fallback(input, init);
  });
  const user = userEvent.setup(); const view = render(<WorkloadEditorPage application={application} environment="staging" />);
  await waitFor(() => expect(gates).toHaveLength(1));
  view.rerender(<WorkloadEditorPage application={application} environment="production" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' }); await addRow(user, 1, 'K'); const dialog = await picker(user);
  expect(dialog.getByRole('button', { name: 'Ứng dụng / PROD_ONLY' })).toBeInTheDocument();
  await act(async () => { gates[0]!.resolve(Response.json({ version: 1, keys: catalogKeys })); await new Promise((resolve) => setTimeout(resolve, 0)); });
  expect(dialog.queryByRole('button', { name: 'Ứng dụng / API_URL' })).not.toBeInTheDocument();
});
