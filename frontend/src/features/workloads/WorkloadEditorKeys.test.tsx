import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import type { ConfigKey } from '../configuration/api';
import type { ResourceType, Workload } from './api';
import type { Application } from '../../shared/types/application';
import { bothConfigured } from '../../test/targets';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: bothConfigured('internal-cluster'), workloads: { staging: [], production: [] } };
const SECRET_VALUE = 'p4ss-never-shown';
const VARIABLE_VALUE = 'https://internal.example';
const catalogKeys: ConfigKey[] = [
  { name: 'API_URL', kind: 'VARIABLE', configured: true, value: VARIABLE_VALUE, usedBy: [] },
  { name: 'LOG_LEVEL', kind: 'VARIABLE', configured: true, value: 'debug', usedBy: [] },
  { name: 'DATABASE_PASSWORD', kind: 'SECRET', configured: true, value: SECRET_VALUE, usedBy: [] },
];

type Saved = { score: Record<string, unknown>; version: number };
type Options = {
  keys?: ConfigKey[];
  workloads?: Workload[];
  types?: ResourceType[];
  configuration?: (call: number) => Response | Promise<Response>;
  put?: (call: number) => Response | Promise<Response>;
  parse?: Record<string, unknown>;
};

function serve(options: Options = {}) {
  const puts: Saved[] = [];
  let configCalls = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) {
      configCalls += 1;
      return options.configuration ? options.configuration(configCalls) : Response.json({ version: 1, keys: options.keys ?? catalogKeys });
    }
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: (options.types ?? []).map((type) => ({ classes: [], ...type })) });
    if (url.endsWith('/workloads/parse')) return Response.json({ score: options.parse });
    if (init?.method === 'PUT') {
      puts.push(JSON.parse(String(init.body)) as Saved);
      return options.put ? options.put(puts.length) : Response.json({ draftVersion: 2, workloads: [] });
    }
    return Response.json({ draftVersion: 1, workloads: options.workloads ?? [] });
  }));
  return { puts };
}

const group = (name: string) => screen.getByRole('group', { name });
const keyBox = (groupName: string, key: string) => within(group(groupName)).getByRole('checkbox', { name: key });
type ScoreShape = { containers: Partial<Record<string, { variables?: Record<string, string> }>>; resources?: Record<string, unknown> };
const scoreOf = (saved: Saved | undefined) => saved?.score as ScoreShape;
const save = () => screen.getByRole('button', { name: 'Lưu draft Workload' });

async function newWorkload(user: ReturnType<typeof userEvent.setup>) {
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'api');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:test');
}

afterEach(() => vi.unstubAllGlobals());

it('ticks a variable and a secret into same-name Score references without showing or copying any value', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  await newWorkload(user);
  expect(await screen.findByRole('checkbox', { name: 'API_URL' })).not.toBeChecked();
  expect(keyBox('Bí mật của ứng dụng cho main', 'DATABASE_PASSWORD')).not.toBeChecked();
  await user.click(keyBox('Biến của ứng dụng cho main', 'API_URL'));
  await user.click(keyBox('Bí mật của ứng dụng cho main', 'DATABASE_PASSWORD'));
  expect(screen.queryByLabelText('Tên trong Container cho DATABASE_PASSWORD')).not.toBeInTheDocument();
  expect(document.body.textContent).not.toContain(SECRET_VALUE);
  expect(document.body.textContent).not.toContain(VARIABLE_VALUE);
  expect(document.querySelector('input[type=password]')).toBeNull();
  await user.click(save());
  expect(puts).toHaveLength(1);
  const score = scoreOf(puts[0]);
  expect(score.containers.main?.variables).toEqual({ API_URL: '${resources.env.API_URL}', DATABASE_PASSWORD: '${resources.env.DATABASE_PASSWORD}' });
  expect(score.resources).toEqual({ env: { type: 'environment' } });
  expect(JSON.stringify(puts[0])).not.toContain(SECRET_VALUE);
  expect(JSON.stringify(puts[0])).not.toContain(VARIABLE_VALUE);
  expect(JSON.stringify(puts[0])).not.toContain('LOG_LEVEL');
});

it('uses a different container name only when asked and resets it to the key name when turned off', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  await newWorkload(user);
  await user.click(await screen.findByRole('checkbox', { name: 'DATABASE_PASSWORD' }));
  const override = screen.getByRole('checkbox', { name: 'Dùng tên Container khác cho DATABASE_PASSWORD' });
  await user.click(override);
  await user.clear(screen.getByLabelText('Tên trong Container cho DATABASE_PASSWORD'));
  await user.type(screen.getByLabelText('Tên trong Container cho DATABASE_PASSWORD'), 'PGPASSWORD');
  await user.click(override);
  expect(screen.queryByLabelText('Tên trong Container cho DATABASE_PASSWORD')).not.toBeInTheDocument();
  await user.click(save());
  expect(scoreOf(puts[0]).containers.main?.variables).toEqual({ DATABASE_PASSWORD: '${resources.env.DATABASE_PASSWORD}' });
  await user.click(override);
  expect(screen.getByLabelText('Tên trong Container cho DATABASE_PASSWORD')).toHaveValue('DATABASE_PASSWORD');
  await user.clear(screen.getByLabelText('Tên trong Container cho DATABASE_PASSWORD'));
  await user.type(screen.getByLabelText('Tên trong Container cho DATABASE_PASSWORD'), 'PGPASSWORD');
  await user.click(save());
  expect(scoreOf(puts[1]).containers.main?.variables).toEqual({ PGPASSWORD: '${resources.env.DATABASE_PASSWORD}' });
});

it('selects keys independently per container and allows the same name in two containers', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  await newWorkload(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm Container' }));
  await user.type(screen.getAllByLabelText('Ảnh Container')[1]!, 'example.invalid/worker:test');
  expect(keyBox('Biến của ứng dụng cho container-2', 'API_URL')).not.toBeChecked();
  await user.click(keyBox('Biến của ứng dụng cho main', 'API_URL'));
  expect(keyBox('Biến của ứng dụng cho container-2', 'API_URL')).not.toBeChecked();
  await user.click(keyBox('Biến của ứng dụng cho container-2', 'API_URL'));
  await user.click(keyBox('Biến của ứng dụng cho container-2', 'LOG_LEVEL'));
  await user.click(save());
  const score = scoreOf(puts[0]);
  expect(score.containers.main?.variables).toEqual({ API_URL: '${resources.env.API_URL}' });
  expect(score.containers['container-2']?.variables).toEqual({ API_URL: '${resources.env.API_URL}', LOG_LEVEL: '${resources.env.LOG_LEVEL}' });
});

const postgres: ResourceType = { key: 'postgres', classes: [], inputs: [], outputs: [{ name: 'host' }, { name: 'password', secret: true }] };
const twoContainers: Workload = { id: 'api', ready: true, score: {
  apiVersion: 'score.dev/v1b1', metadata: { name: 'api' },
  containers: {
    main: { image: 'example.invalid/api:v1', variables: { API_URL: '${resources.env.API_URL}', PGPASSWORD: '${resources.env.DATABASE_PASSWORD}', DB_PASSWORD: '${resources.env.DATABASE_PASSWORD}', DB_HOST: '${resources.db.host}' } },
    worker: { image: 'example.invalid/worker:v1', variables: { API_URL: '${resources.env.API_URL}', DB_HOST: '${resources.db.host}' } },
  },
  resources: { env: { type: 'environment' }, db: { type: 'postgres' } },
} };

it('restores custom and multiple container names for one key on edit and saves them unchanged', async () => {
  const { puts } = serve({ workloads: [twoContainers], types: [postgres] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  await screen.findByRole('heading', { name: 'Sửa api' });
  expect(keyBox('Bí mật của ứng dụng cho main', 'DATABASE_PASSWORD')).toBeChecked();
  expect(screen.getByLabelText('Tên trong Container cho DATABASE_PASSWORD (1)')).toHaveValue('PGPASSWORD');
  expect(screen.getByLabelText('Tên trong Container cho DATABASE_PASSWORD (2)')).toHaveValue('DB_PASSWORD');
  expect(keyBox('Bí mật của ứng dụng cho worker', 'DATABASE_PASSWORD')).not.toBeChecked();
  await user.clear(screen.getAllByLabelText('Ảnh Container')[0]!);
  await user.type(screen.getAllByLabelText('Ảnh Container')[0]!, 'example.invalid/api:v2');
  await user.click(save());
  const score = scoreOf(puts[0]);
  expect(score.containers.main?.variables).toEqual({ API_URL: '${resources.env.API_URL}', PGPASSWORD: '${resources.env.DATABASE_PASSWORD}', DB_PASSWORD: '${resources.env.DATABASE_PASSWORD}', DB_HOST: '${resources.db.host}' });
  expect(score.containers.worker?.variables).toEqual({ API_URL: '${resources.env.API_URL}', DB_HOST: '${resources.db.host}' });
});

it('unchecking a key removes only its names in that container and keeps resource sources and other containers', async () => {
  const { puts } = serve({ workloads: [twoContainers], types: [postgres] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  await screen.findByRole('heading', { name: 'Sửa api' });
  await user.click(keyBox('Bí mật của ứng dụng cho main', 'DATABASE_PASSWORD'));
  await user.click(keyBox('Biến của ứng dụng cho main', 'API_URL'));
  expect(screen.queryByLabelText(/Tên trong Container cho DATABASE_PASSWORD/)).not.toBeInTheDocument();
  expect(keyBox('Biến của ứng dụng cho worker', 'API_URL')).toBeChecked();
  await user.click(save());
  const score = scoreOf(puts[0]);
  expect(score.containers.main?.variables).toEqual({ DB_HOST: '${resources.db.host}' });
  expect(score.containers.worker?.variables).toEqual({ API_URL: '${resources.env.API_URL}', DB_HOST: '${resources.db.host}' });
  expect(score.resources).toEqual({ db: { type: 'postgres' }, env: { type: 'environment' } });
});

it('restores a custom container name from an imported Score', async () => {
  const parsed = { apiVersion: 'score.dev/v1b1', metadata: { name: 'importer' }, containers: { main: { image: 'example.invalid/i:v1', variables: { PGPASSWORD: '${resources.env.DATABASE_PASSWORD}' } } }, resources: { env: { type: 'environment' } } };
  const { puts } = serve({ parse: parsed });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.click(screen.getByRole('button', { name: 'Nhập Score' }));
  const content = 'apiVersion: score.dev/v1b1\n';
  const file = Object.assign(new File([content], 'score.yaml', { type: 'text/yaml' }), { text: async () => content });
  await user.upload(screen.getByLabelText('File Score'), file);
  await screen.findByText(/Đã phân tích Workload/);
  await user.click(screen.getByRole('button', { name: 'Nhập bằng form' }));
  expect(keyBox('Bí mật của ứng dụng cho main', 'DATABASE_PASSWORD')).toBeChecked();
  expect(screen.getByLabelText('Tên trong Container cho DATABASE_PASSWORD')).toHaveValue('PGPASSWORD');
  await user.click(save());
  expect(scoreOf(puts[0]).containers.main?.variables).toEqual({ PGPASSWORD: '${resources.env.DATABASE_PASSWORD}' });
});

it('keeps an unavailable selected key visible and blocks Save until it is removed', async () => {
  const workload: Workload = { id: 'api', ready: true, score: { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/api:v1', variables: { LEGACY_TOKEN: '${resources.env.OLD_TOKEN}', API_URL: '${resources.env.API_URL}' } } }, resources: { env: { type: 'environment' } } } };
  const { puts } = serve({ workloads: [workload] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  const missing = await screen.findByRole('group', { name: 'Key ứng dụng không còn tồn tại cho main' });
  expect(missing).toHaveTextContent('OLD_TOKEN');
  expect(missing).toHaveTextContent('LEGACY_TOKEN');
  await user.click(save());
  expect(screen.getByRole('alert')).toHaveTextContent('OLD_TOKEN không còn tồn tại');
  expect(puts).toHaveLength(0);
  await user.click(within(missing).getByRole('button', { name: 'Xóa key không còn tồn tại OLD_TOKEN với tên LEGACY_TOKEN' }));
  await user.click(save());
  expect(scoreOf(puts[0]).containers.main?.variables).toEqual({ API_URL: '${resources.env.API_URL}' });
});

it('blocks Save for a name used by both an Application key and a resource output, and for an empty custom name', async () => {
  const { puts } = serve({ types: [postgres] });
  const user = userEvent.setup();
  await newWorkload(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  await user.type(screen.getByLabelText('Alias tài nguyên'), 'db');
  await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'postgres');
  await user.click(screen.getByRole('button', { name: '+ Thêm nguồn khác' }));
  await user.type(screen.getByLabelText('Tên biến Container'), 'API_URL');
  await user.selectOptions(screen.getByLabelText('Tài nguyên được tham chiếu'), 'db');
  await user.selectOptions(screen.getByLabelText('Output của tài nguyên'), 'host');
  await user.click(keyBox('Biến của ứng dụng cho main', 'API_URL'));
  await user.click(save());
  expect(screen.getByRole('alert')).toHaveTextContent('Container main: tên API_URL bị dùng nhiều lần');
  await user.click(screen.getByRole('checkbox', { name: 'Dùng tên Container khác cho API_URL' }));
  await user.clear(screen.getByLabelText('Tên trong Container cho API_URL'));
  await user.click(save());
  expect(screen.getByRole('alert')).toHaveTextContent('nhập tên trong Container cho API_URL');
  expect(puts).toHaveLength(0);
  await user.type(screen.getByLabelText('Tên trong Container cho API_URL'), 'BACKEND_URL');
  await user.click(save());
  expect(scoreOf(puts[0]).containers.main?.variables).toEqual({ API_URL: '${resources.db.host}', BACKEND_URL: '${resources.env.API_URL}' });
});

it('blocks Save for an empty other-source name or a Service collision instead of dropping or overwriting it', async () => {
  const { puts } = serve({ workloads: [{ id: 'orders', ready: true, servicePorts: ['http'] }] });
  const user = userEvent.setup();
  await newWorkload(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm nguồn khác' }));
  await user.selectOptions(screen.getByLabelText('Nguồn tham chiếu'), 'SERVICE');
  await user.selectOptions(screen.getByLabelText('Workload cung cấp Service'), 'orders');
  await user.selectOptions(screen.getByLabelText('Cổng của Service'), 'http');
  await user.click(save());
  expect(screen.getByRole('alert')).toHaveTextContent('nhập tên trong Container cho mọi nguồn khác');
  await user.type(screen.getByLabelText('Tên biến Container'), 'LOG_LEVEL');
  await user.click(keyBox('Biến của ứng dụng cho main', 'LOG_LEVEL'));
  await user.click(save());
  expect(screen.getByRole('alert')).toHaveTextContent('tên LOG_LEVEL bị dùng nhiều lần');
  expect(puts).toHaveLength(0);
});

it('saves resource output and workload Service bindings from Other sources, which offer no Application key source', async () => {
  const { puts } = serve({ types: [postgres], workloads: [{ id: 'orders', ready: true, servicePorts: ['http'] }] });
  const user = userEvent.setup();
  await newWorkload(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  await user.type(screen.getByLabelText('Alias tài nguyên'), 'db');
  await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'postgres');
  await user.click(screen.getByRole('button', { name: '+ Thêm nguồn khác' }));
  await user.click(screen.getByRole('button', { name: '+ Thêm nguồn khác' }));
  const [first, second] = screen.getAllByLabelText('Nguồn tham chiếu');
  expect(within(first!).getAllByRole('option').map((option) => option.textContent)).toEqual(['Output tài nguyên', 'Service của Workload']);
  const [firstName, secondName] = screen.getAllByLabelText('Tên biến Container');
  await user.selectOptions(second!, 'SERVICE');
  await user.type(firstName!, 'DB_PASSWORD');
  await user.selectOptions(screen.getByLabelText('Tài nguyên được tham chiếu'), 'db');
  await user.selectOptions(screen.getByLabelText('Output của tài nguyên'), 'password');
  await user.type(secondName!, 'ORDERS_URL');
  await user.selectOptions(screen.getByLabelText('Workload cung cấp Service'), 'orders');
  await user.selectOptions(screen.getByLabelText('Cổng của Service'), 'http');
  await user.click(save());
  const score = scoreOf(puts[0]);
  expect(score.containers.main?.variables).toEqual({ DB_PASSWORD: '${resources.db.password}', ORDERS_URL: '${resources.svc_orders_http.url}' });
  expect(score.resources).toEqual({ db: { type: 'postgres' }, svc_orders_http: { type: 'service', params: { workload: 'orders', port: 'http' } } });
});

it('shows key catalog loading and a retryable error distinct from an empty list, and blocks Save until loaded', async () => {
  let release: (response: Response) => void = () => undefined;
  const { puts } = serve({ configuration: (call) => call === 1 ? Response.json({ error: 'settings unavailable', code: 'SERVICE_UNAVAILABLE' }, { status: 503 }) : new Promise<Response>((resolve) => { release = resolve; }) });
  const user = userEvent.setup();
  await newWorkload(user);
  expect(await screen.findByText(/Không tải được biến và bí mật của ứng dụng\. Dịch vụ cần thiết chưa sẵn sàng/)).toBeInTheDocument();
  expect(screen.queryByText(/chưa có/)).not.toBeInTheDocument();
  expect(screen.queryByRole('checkbox', { name: 'API_URL' })).not.toBeInTheDocument();
  await user.click(save());
  expect(screen.getByText(/Hãy thử tải lại trước khi lưu/)).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Thử tải lại key' }));
  expect(await screen.findByText('Đang tải biến và bí mật của ứng dụng…')).toBeInTheDocument();
  await user.click(save());
  expect(screen.getByRole('alert')).toHaveTextContent('đang tải');
  expect(puts).toHaveLength(0);
  expect(screen.getByLabelText('Ảnh Container')).toHaveValue('example.invalid/api:test');
  release(Response.json({ version: 1, keys: catalogKeys }));
  await user.click(await screen.findByRole('checkbox', { name: 'API_URL' }));
  await user.click(save());
  expect(scoreOf(puts[0]).containers.main?.variables).toEqual({ API_URL: '${resources.env.API_URL}' });
});

it('explains where keys are created when the Environment has none', async () => {
  serve({ keys: [] });
  const user = userEvent.setup();
  await newWorkload(user);
  expect(within(group('Biến của ứng dụng cho main')).getByText(/chưa có biến của ứng dụng/)).toBeInTheDocument();
  expect(within(group('Bí mật của ứng dụng cho main')).getByText(/chưa có bí mật của ứng dụng/)).toBeInTheDocument();
});

it('freezes key selection and container name controls while Save is pending', async () => {
  let release: (response: Response) => void = () => undefined;
  const { puts } = serve({ put: () => new Promise<Response>((resolve) => { release = resolve; }) });
  const user = userEvent.setup();
  await newWorkload(user);
  await user.click(await screen.findByRole('checkbox', { name: 'API_URL' }));
  await user.click(screen.getByRole('checkbox', { name: 'Dùng tên Container khác cho API_URL' }));
  await user.click(save());
  await waitFor(() => expect(puts).toHaveLength(1));
  expect(screen.getByRole('checkbox', { name: 'API_URL' })).toBeDisabled();
  expect(screen.getByRole('checkbox', { name: 'LOG_LEVEL' })).toBeDisabled();
  expect(screen.getByRole('checkbox', { name: 'Dùng tên Container khác cho API_URL' })).toBeDisabled();
  expect(screen.getByLabelText('Tên trong Container cho API_URL')).toBeDisabled();
  expect(screen.getByRole('button', { name: '+ Thêm nguồn khác' })).toBeDisabled();
  release(Response.json({ error: 'temporarily unavailable', code: 'SERVICE_UNAVAILABLE' }, { status: 503 }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Dịch vụ cần thiết chưa sẵn sàng');
  expect(screen.getByRole('checkbox', { name: 'API_URL' })).toBeEnabled();
  expect(screen.getByRole('checkbox', { name: 'API_URL' })).toBeChecked();
});

it('keeps key selections and custom names after a stale Save and reload', async () => {
  const { puts } = serve({ put: (call) => call === 1 ? Response.json({ error: 'workloads changed since they were loaded; reload and try again' }, { status: 409 }) : Response.json({ draftVersion: 3, workloads: [] }) });
  const user = userEvent.setup();
  await newWorkload(user);
  await user.click(await screen.findByRole('checkbox', { name: 'DATABASE_PASSWORD' }));
  await user.click(screen.getByRole('checkbox', { name: 'Dùng tên Container khác cho DATABASE_PASSWORD' }));
  await user.clear(screen.getByLabelText('Tên trong Container cho DATABASE_PASSWORD'));
  await user.type(screen.getByLabelText('Tên trong Container cho DATABASE_PASSWORD'), 'PGPASSWORD');
  await user.click(save());
  await user.click(await screen.findByRole('button', { name: 'Tải lại trạng thái hiện tại' }));
  expect(await screen.findByText(/Đã tải lại draft phiên bản/)).toBeInTheDocument();
  expect(screen.getByRole('checkbox', { name: 'DATABASE_PASSWORD' })).toBeChecked();
  expect(screen.getByLabelText('Tên trong Container cho DATABASE_PASSWORD')).toHaveValue('PGPASSWORD');
  await user.click(save());
  expect(puts).toHaveLength(2);
  expect(scoreOf(puts[1]).containers.main?.variables).toEqual({ PGPASSWORD: '${resources.env.DATABASE_PASSWORD}' });
});

it('saves an advanced Score losslessly without passing through the form', async () => {
  const advanced = { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/api:v1', command: ['serve'], variables: { PGPASSWORD: '${resources.env.DATABASE_PASSWORD}', PGPASSWORD_COPY: '${resources.env.DATABASE_PASSWORD}' } } }, resources: { env: { type: 'environment' } } };
  const { puts } = serve({ workloads: [{ id: 'api', ready: true, score: advanced }] });
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  expect(await screen.findByText(/các trường mà form không thể giữ nguyên/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Nhập bằng form' })).toBeDisabled();
  await user.click(save());
  expect(puts[0]?.score).toEqual(advanced);
});

it('serializes selected key names that match Object prototype members as own container variables', async () => {
  const keys: ConfigKey[] = ['__proto__', 'constructor', 'toString'].map((name) => ({ name, kind: 'VARIABLE', configured: true, value: 'v', usedBy: [] }));
  const { puts } = serve({ keys });
  const user = userEvent.setup();
  await newWorkload(user);
  for (const key of keys) await user.click(await screen.findByRole('checkbox', { name: key.name }));
  await user.click(save());
  expect(puts).toHaveLength(1);
  const variables = scoreOf(puts[0]).containers.main?.variables ?? {};
  expect(Object.keys(variables)).toEqual(['__proto__', 'constructor', 'toString']);
  expect(Object.getOwnPropertyDescriptor(variables, '__proto__')?.value).toBe('${resources.env.__proto__}');
  expect(variables.constructor).toBe('${resources.env.constructor}');
  expect(variables.toString).toBe('${resources.env.toString}');
});

const otherApplication: Application = { id: 'billing', name: 'Billing', subdomain: 'billing', environments: bothConfigured('internal-cluster'), workloads: { staging: [], production: [] } };

async function stagePasswordAlias(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('checkbox', { name: 'DATABASE_PASSWORD' }));
  await user.click(screen.getByRole('checkbox', { name: 'Dùng tên Container khác cho DATABASE_PASSWORD' }));
  await user.clear(screen.getByLabelText('Tên trong Container cho DATABASE_PASSWORD'));
  await user.type(screen.getByLabelText('Tên trong Container cho DATABASE_PASSWORD'), 'PGPASSWORD');
}

async function expectFreshNewForm() {
  await screen.findByRole('heading', { name: 'Thêm Workload' });
  expect(await screen.findByRole('checkbox', { name: 'DATABASE_PASSWORD' })).not.toBeChecked();
  expect(screen.getByRole('checkbox', { name: 'API_URL' })).not.toBeChecked();
  expect(screen.queryByLabelText(/Tên trong Container cho DATABASE_PASSWORD/)).not.toBeInTheDocument();
  expect(screen.getByLabelText('Tên Workload')).toHaveValue('');
  expect(screen.getAllByLabelText('Ảnh Container')).toHaveLength(1);
  expect(screen.getByLabelText('Ảnh Container')).toHaveValue('');
}

it('starts a different Environment with a fresh form even when it has the same key names', async () => {
  const { puts } = serve();
  const user = userEvent.setup();
  const view = render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'api');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:test');
  await stagePasswordAlias(user);
  view.rerender(<WorkloadEditorPage application={application} environment="production" />);
  await expectFreshNewForm();
  await user.type(screen.getByLabelText('Tên Workload'), 'api');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:prod');
  await user.click(save());
  expect(puts).toHaveLength(1);
  expect(scoreOf(puts[0]).containers.main?.variables).toBeUndefined();
});

it('starts a different Application with a fresh form', async () => {
  serve();
  const user = userEvent.setup();
  const view = render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'api');
  await stagePasswordAlias(user);
  view.rerender(<WorkloadEditorPage application={otherApplication} environment="staging" />);
  await expectFreshNewForm();
});

it('clears an edited workload, including its selected keys, when switching to Add workload', async () => {
  serve({ workloads: [twoContainers], types: [postgres] });
  const view = render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  await screen.findByRole('heading', { name: 'Sửa api' });
  expect(keyBox('Bí mật của ứng dụng cho main', 'DATABASE_PASSWORD')).toBeChecked();
  view.rerender(<WorkloadEditorPage application={application} environment="staging" />);
  await expectFreshNewForm();
  expect(screen.queryByLabelText('Tên biến Container')).not.toBeInTheDocument();
});

it('clears advanced imported Score state when switching from an advanced workload to Add workload', async () => {
  const advanced = { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/api:v1', command: ['serve'] } } };
  serve({ workloads: [{ id: 'api', ready: true, score: advanced }] });
  const view = render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  expect(await screen.findByText(/các trường mà form không thể giữ nguyên/)).toBeInTheDocument();
  view.rerender(<WorkloadEditorPage application={application} environment="staging" />);
  await expectFreshNewForm();
  expect(screen.queryByText(/các trường mà form không thể giữ nguyên/)).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Nhập bằng form' })).toBeEnabled();
  await userEvent.setup().click(screen.getByRole('button', { name: 'Nhập Score' }));
  expect(screen.queryByText(/Đã phân tích Workload/)).not.toBeInTheDocument();
});

it('clears a blocked missing-workload state when switching to Add workload', async () => {
  serve();
  const view = render(<WorkloadEditorPage application={application} environment="staging" workloadId="ghost" />);
  expect(await screen.findByRole('alert')).toHaveTextContent('Không tìm thấy Workload');
  view.rerender(<WorkloadEditorPage application={application} environment="staging" />);
  await expectFreshNewForm();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(save()).toBeEnabled();
});

type Gate = { resolve: (response: Response) => void };
function gate(gates: Gate[]): Promise<Response> { return new Promise<Response>((resolve) => { gates.push({ resolve }); }); }

// Runs after every already-queued continuation. Used only once the page has
// read the late body, so it orders assertions after handling, not a time guess.
const drainQueued = () => new Promise<void>((resolve) => { setTimeout(resolve, 0); });
function bodyRead(response: Response): Promise<void> {
  const json = response.json.bind(response);
  return new Promise<void>((done) => { response.json = () => { const read = json(); read.then(() => done(), () => done()); return read; }; });
}
// Releases late replies inside act and returns after the page handled them.
async function deliver(...replies: [Gate, Response][]) {
  const reads = replies.map(([, response]) => bodyRead(response));
  await act(async () => {
    for (const [late, response] of replies) late.resolve(response);
    await Promise.all(reads);
    await drainQueued();
  });
}

it('ignores a late key catalog from the previous Environment', async () => {
  const gates: Gate[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/staging/configuration')) return gate(gates);
    if (url.endsWith('/configuration')) return Response.json({ version: 1, keys: [{ name: 'PROD_ONLY', kind: 'VARIABLE', configured: true, usedBy: [] }] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    return Response.json({ draftVersion: 1, workloads: [] });
  }));
  const view = render(<WorkloadEditorPage application={application} environment="staging" />);
  await waitFor(() => expect(gates).toHaveLength(1));
  view.rerender(<WorkloadEditorPage application={application} environment="production" />);
  expect(await screen.findByRole('checkbox', { name: 'PROD_ONLY' })).toBeInTheDocument();
  await deliver([gates[0]!, Response.json({ version: 1, keys: catalogKeys })]);
  expect(screen.getByRole('checkbox', { name: 'PROD_ONLY' })).toBeInTheDocument();
  expect(screen.queryByRole('checkbox', { name: 'API_URL' })).not.toBeInTheDocument();
});

const parsedScore = { apiVersion: 'score.dev/v1b1', metadata: { name: 'importer' }, containers: { main: { image: 'example.invalid/i:v1', variables: { PGPASSWORD: '${resources.env.DATABASE_PASSWORD}' } } }, resources: { env: { type: 'environment' } } };

async function expectNoImportCarryover(user: ReturnType<typeof userEvent.setup>) {
  await expectFreshNewForm();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Nhập Score' }));
  expect(screen.queryByText(/Đã phân tích Workload/)).not.toBeInTheDocument();
}

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
  await deliver([gates[0]!, Response.json({ score: parsedScore })]);
  await expectNoImportCarryover(user);
});

it('ignores a Score file read that finishes after the Environment changed', async () => {
  serve({ parse: parsedScore });
  const user = userEvent.setup();
  const view = render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.click(screen.getByRole('button', { name: 'Nhập Score' }));
  let finishRead: (content: string) => void = () => undefined;
  const file = Object.assign(new File(['x'], 'score.yaml'), { text: () => new Promise<string>((resolve) => { finishRead = resolve; }) });
  await user.upload(screen.getByLabelText('File Score'), file);
  view.rerender(<WorkloadEditorPage application={application} environment="production" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await act(async () => { finishRead('apiVersion: score.dev/v1b1\n'); await drainQueued(); });
  await expectNoImportCarryover(user);
  expect(vi.mocked(fetch).mock.calls.some(([input]) => String(input).endsWith('/workloads/parse'))).toBe(false);
});

it.each([
  ['an error', () => Response.json({ error: 'staging save failed' }, { status: 503 })],
  ['a conflict', () => Response.json({ error: 'conflict' }, { status: 409 })],
  ['a success', () => Response.json({ draftVersion: 2, workloads: [] })],
])('ignores %s from a Save started in the previous Environment', async (_, reply) => {
  const gates: Gate[] = [];
  const { puts } = serve({ put: () => gate(gates) });
  const user = userEvent.setup();
  window.history.pushState({}, '', '/editor-under-test');
  const view = render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'api');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:test');
  await stagePasswordAlias(user);
  await user.click(save());
  await waitFor(() => expect(gates).toHaveLength(1));
  view.rerender(<WorkloadEditorPage application={application} environment="production" />);
  await expectFreshNewForm();
  await deliver([gates[0]!, reply()]);
  expect(window.location.pathname).toBe('/editor-under-test');
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(save()).toBeEnabled();
  expect(screen.getByRole('checkbox', { name: 'DATABASE_PASSWORD' })).toBeEnabled();
  expect(puts).toHaveLength(1);
});

it.each([
  ['data', () => [Response.json({ version: 1, keys: [{ name: 'STAGING_LATE', kind: 'VARIABLE', configured: true, usedBy: [] }] }), Response.json({ draftVersion: 99, workloads: [] })]],
  ['error', () => [Response.json({ error: 'staging reload failed' }, { status: 503 }), Response.json({ error: 'staging reload failed' }, { status: 503 })]],
])('keeps Reload usable in the next Environment while a previous reload is pending, then ignores its late %s', async (_, replies) => {
  const gates: Gate[] = [];
  const { puts } = serve({ put: () => Response.json({ error: 'workloads changed since they were loaded; reload and try again' }, { status: 409 }) });
  const fallback = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.includes('/staging/') && !init?.method && puts.length > 0) return gate(gates);
    if (url.endsWith('/production/configuration')) return Response.json({ version: 1, keys: [{ name: 'PROD_ONLY', kind: 'VARIABLE', configured: true, usedBy: [] }] });
    if (url.endsWith('/production/workloads')) return Response.json({ draftVersion: 7, workloads: [] });
    return fallback(input, init);
  });
  const user = userEvent.setup();
  const view = render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'api');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:test');
  await user.click(save());
  await user.click(await screen.findByRole('button', { name: 'Tải lại trạng thái hiện tại' }));
  await waitFor(() => expect(gates).toHaveLength(2));
  expect(screen.getByRole('button', { name: 'Đang tải lại…' })).toBeDisabled();
  view.rerender(<WorkloadEditorPage application={application} environment="production" />);
  await screen.findByRole('heading', { name: 'Thêm Workload' });
  expect(await screen.findByRole('checkbox', { name: 'PROD_ONLY' })).toBeInTheDocument();
  await user.type(screen.getByLabelText('Tên Workload'), 'api');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:prod');
  await user.click(save());
  expect(puts).toHaveLength(2);
  const reload = await screen.findByRole('alert', { name: 'Draft Workload đã cũ' });
  expect(within(reload).getByRole('button', { name: 'Tải lại trạng thái hiện tại' })).toBeEnabled();
  await user.click(within(reload).getByRole('button', { name: 'Tải lại trạng thái hiện tại' }));
  expect(await screen.findByText(/Đã tải lại draft phiên bản 7\./)).toBeInTheDocument();
  const [configuration, workloads] = replies();
  await deliver([gates[0]!, configuration!], [gates[1]!, workloads!]);
  expect(screen.getByText(/Đã tải lại draft phiên bản 7\./)).toBeInTheDocument();
  expect(screen.getByRole('checkbox', { name: 'PROD_ONLY' })).toBeInTheDocument();
  expect(screen.queryByRole('checkbox', { name: 'STAGING_LATE' })).not.toBeInTheDocument();
  expect(screen.queryByText(/Could not reload|staging reload failed/)).not.toBeInTheDocument();
  expect(screen.queryByRole('alert', { name: 'Draft Workload đã cũ' })).not.toBeInTheDocument();
  expect(screen.getByLabelText('Ảnh Container')).toHaveValue('example.invalid/api:prod');
});

// JSON.parse keeps __proto__ as an own key, as a server or Score file can.
const protoScoreJson = '{"apiVersion":"score.dev/v1b1","metadata":{"name":"api"},"containers":{"__proto__":{"image":"example.invalid/api:v1","variables":{"__proto__":"${resources.env.__proto__}","DB_HOST":"${resources.__proto__.host}"}}},"service":{"ports":{"__proto__":{"port":80,"targetPort":8080}}},"resources":{"__proto__":{"type":"postgres","params":{"__proto__":"primary","name":"db"}},"env":{"type":"environment"}}}';
const protoType: ResourceType = { key: 'postgres', classes: [], inputs: [{ name: '__proto__', type: 'string' }, { name: 'name', type: 'string' }], outputs: [{ name: 'host' }] };
const protoKeys: ConfigKey[] = [{ name: '__proto__', kind: 'VARIABLE', configured: true, usedBy: [] }];

it.each([
  ['an edited workload', async () => {
    render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
    await screen.findByRole('heading', { name: 'Sửa api' });
  }],
  ['an imported Score', async (user: ReturnType<typeof userEvent.setup>) => {
    render(<WorkloadEditorPage application={application} environment="staging" />);
    await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
    await user.click(screen.getByRole('button', { name: 'Nhập Score' }));
    const content = 'apiVersion: score.dev/v1b1\n';
    await user.upload(screen.getByLabelText('File Score'), Object.assign(new File([content], 'score.yaml'), { text: async () => content }));
    await screen.findByText(/Đã phân tích Workload/);
    await user.click(screen.getByRole('button', { name: 'Nhập bằng form' }));
  }],
])('keeps container, variable, port, resource alias and input names called __proto__ when %s is changed on the form', async (_, open) => {
  const score = JSON.parse(protoScoreJson) as Record<string, unknown>;
  const { puts } = serve({ keys: protoKeys, types: [protoType], workloads: [{ id: 'api', ready: true, score }], parse: score });
  const user = userEvent.setup();
  await open(user);
  await screen.findByRole('group', { name: 'Biến của ứng dụng cho __proto__' });
  expect(keyBox('Biến của ứng dụng cho __proto__', '__proto__')).toBeChecked();
  expect(screen.getByLabelText('Tham số __proto__')).toHaveValue('primary');
  await user.clear(screen.getByLabelText('Ảnh Container'));
  await user.type(screen.getByLabelText('Ảnh Container'), 'example.invalid/api:v2');
  await user.click(save());
  expect(puts).toHaveLength(1);
  expect(JSON.stringify(puts[0]?.score)).toBe(protoScoreJson.replace('api:v1', 'api:v2'));
});

// An unset input named __proto__ must read as empty, not as Object.prototype.
it.each([
  ['string', false], ['bool', false], ['string', true], ['bool', true],
] as const)('treats an unset %s resource input called __proto__ as empty (required: %s)', async (type, required) => {
  const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
  const protoInputType: ResourceType = { key: 'postgres', classes: [], inputs: [{ name: '__proto__', type, required }], outputs: [{ name: 'host' }] };
  const { puts } = serve({ types: [protoInputType] });
  const user = userEvent.setup();
  await newWorkload(user);
  await user.click(await screen.findByRole('checkbox', { name: 'API_URL' }));
  await user.click(keyBox('Bí mật của ứng dụng cho main', 'DATABASE_PASSWORD'));
  await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên' }));
  await user.type(screen.getByLabelText('Alias tài nguyên'), 'db');
  await user.selectOptions(screen.getByLabelText('Loại tài nguyên'), 'postgres');
  expect(screen.getByLabelText('Tham số __proto__')).toHaveValue('');
  await user.click(save());
  if (required) {
    expect(await screen.findByText('db: __proto__ là bắt buộc.')).toBeInTheDocument();
    expect(puts).toHaveLength(0);
  } else {
    await waitFor(() => expect(puts).toHaveLength(1));
    const score = scoreOf(puts[0]);
    expect(score.resources).toEqual({ db: { type: 'postgres' }, env: { type: 'environment' } });
    expect(score.containers.main?.variables).toEqual({ API_URL: '${resources.env.API_URL}', DATABASE_PASSWORD: '${resources.env.DATABASE_PASSWORD}' });
  }
  expect(keyBox('Biến của ứng dụng cho main', 'API_URL')).toBeChecked();
  expect(keyBox('Bí mật của ứng dụng cho main', 'DATABASE_PASSWORD')).toBeChecked();
  expect(consoleError).not.toHaveBeenCalled();
  consoleError.mockRestore();
});
