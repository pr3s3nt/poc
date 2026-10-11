import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResourceTypesPage } from './ResourceTypesPage';

type Body = Record<string, unknown>;
const secretNames = { name: 'password', type: 'string', required: false, secret: false };
// Flags deliberately contradict the field names: a "password" that is not secret
// and an "endpoint" that is, so inference from names would be caught.
const cache: Body = {
  key: 'cache',
  inputs: [{ name: 'size', type: 'number', required: true }, { name: 'tier', type: 'any' }],
  outputs: [secretNames, { name: 'endpoint', type: 'string', required: true, secret: true }, { name: 'flag', type: 'bool', required: false, secret: false }],
};
const seeded = ['vpc', 'k8s-cluster', 'k8s-namespace', 'postgres', 'workload', 'postgres-ha', 'constructor', 'toString'].map((key) => ({ key, inputs: [], outputs: [] }));

function backend(options: { types?: Body[]; post?: (body: Body) => Response | Promise<Response>; lists?: (() => Response)[] } = {}) {
  const posted: Body[] = [];
  const gets: string[] = [];
  const lists = options.lists;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    if (init?.method === 'POST') {
      const body = JSON.parse(String(init.body)) as Body;
      posted.push(body);
      return (options.post ?? (() => Response.json(body, { status: 201 })))(body);
    }
    gets.push(String(input));
    if (lists?.length) return (lists.shift() as () => Response)();
    return Response.json({ resourceTypes: options.types ?? [cache] });
  }));
  return { posted, gets };
}
const entry = (key: string) => screen.getByText(key, { selector: 'strong' }).closest('.catalog-entry') as HTMLElement;
const open = async (user: ReturnType<typeof userEvent.setup>, key: string) => { await user.click(await screen.findByRole('button', { name: `Xem schema ${key}` })); };

afterEach(() => { vi.unstubAllGlobals(); window.history.replaceState({}, '', '/'); });

describe('UC-02 resource type registration (Vietnamese)', () => {
  it('registers input/output contract and refreshes the organization catalog', async () => {
    const user = userEvent.setup();
    let created: Body | undefined;
    vi.stubGlobal('fetch', vi.fn(async (_input: string | URL | Request, init?: RequestInit) => {
      if (init?.method === 'POST') { created = JSON.parse(String(init.body)) as Body; return Response.json(created, { status: 201 }); }
      return Response.json({ resourceTypes: created ? [created] : [] });
    }));
    render(<ResourceTypesPage />);
    expect(await screen.findByText('Chưa có loại tài nguyên nào.')).toBeInTheDocument();
    await user.type(screen.getByLabelText('ID loại tài nguyên'), 'redis');
    await user.click(screen.getByRole('button', { name: '+ Thêm tham số đầu vào' }));
    await user.type(screen.getByLabelText('Đầu vào 1 tên'), 'size');
    await user.click(screen.getByRole('button', { name: '+ Thêm tham số đầu ra' }));
    await user.type(screen.getByLabelText('Đầu ra 1 tên'), 'password');
    await user.click(screen.getByLabelText('Đầu ra 1 bí mật'));
    await user.click(screen.getByRole('button', { name: 'Đăng ký loại tài nguyên' }));
    expect(await screen.findByText('redis', { selector: 'strong' })).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('Đã đăng ký loại tài nguyên redis.');
    expect(created).toEqual({ key: 'redis', inputs: [{ name: 'size', type: 'string', required: false }], outputs: [{ name: 'password', type: 'string', required: false, secret: true }] });
  });

  it('describes the ID rule in Vietnamese, sends the ID untrimmed and keeps the form on a 400 without raw English', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ types: [], post: () => Response.json({ error: 'catalog: invalid document: resource type ID must be non-empty and contain only lowercase letters, digits and hyphens' }, { status: 400 }) });
    render(<ResourceTypesPage />);
    const id = screen.getByLabelText('ID loại tài nguyên');
    expect(id).toHaveAccessibleDescription(/Dùng chữ thường, số và dấu gạch ngang.*environment và service là ID dành riêng/);
    await user.type(id, ' Cache_1');
    await user.click(screen.getByRole('button', { name: '+ Thêm tham số đầu vào' }));
    await user.type(screen.getByLabelText('Đầu vào 1 tên'), 'size');
    await user.click(screen.getByRole('button', { name: 'Đăng ký loại tài nguyên' }));
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Máy chủ từ chối loại tài nguyên này');
    expect(alert.textContent).not.toMatch(/catalog|invalid document|lowercase/i);
    expect(posted).toEqual([expect.objectContaining({ key: ' Cache_1' })]);
    expect(id).toHaveValue(' Cache_1');
    expect(screen.getByLabelText('Đầu vào 1 tên')).toHaveValue('size');
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('maps failures by status to Vietnamese, never raw backend text', async () => {
    const user = userEvent.setup();
    const answers = [() => Response.json({ error: 'forbidden' }, { status: 403 }), () => Response.json({ error: 'boom' }, { status: 500 }), () => { throw new TypeError('Failed to fetch'); }];
    backend({ types: [], post: () => (answers.shift() as () => Response)() });
    render(<ResourceTypesPage />);
    await user.type(screen.getByLabelText('ID loại tài nguyên'), 'cache');
    const submit = screen.getByRole('button', { name: 'Đăng ký loại tài nguyên' });
    await user.click(submit);
    expect(await screen.findByRole('alert')).toHaveTextContent('Bạn không có quyền đăng ký loại tài nguyên');
    await user.click(submit);
    expect(await screen.findByText(/Máy chủ gặp lỗi khi đăng ký loại tài nguyên/)).toBeInTheDocument();
    await user.click(submit);
    expect(await screen.findByText(/chưa rõ kết quả/)).toBeInTheDocument();
    expect(screen.getByRole('alert').textContent).not.toMatch(/forbidden|boom|Failed to fetch/);
  });

  it('shows Vietnamese loading, list error and retry, never an empty catalog on failure', async () => {
    const user = userEvent.setup();
    let release: () => void = () => undefined;
    const lists = [() => Response.json({ error: 'unavailable' }, { status: 500 })];
    vi.stubGlobal('fetch', vi.fn(async () => {
      if (lists.length) { await new Promise<void>((done) => { release = done; }); return lists.shift()?.() as Response; }
      return Response.json({ resourceTypes: [cache] });
    }));
    render(<ResourceTypesPage />);
    expect(screen.getByRole('status')).toHaveTextContent('Đang tải loại tài nguyên…');
    release();
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải danh sách' });
    expect(alert).toHaveTextContent('Không tải được danh sách loại tài nguyên từ máy chủ.');
    expect(alert.textContent).not.toMatch(/unavailable/);
    expect(screen.queryByText('Chưa có loại tài nguyên nào.')).not.toBeInTheDocument();
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    expect(await screen.findByText('cache', { selector: 'strong' })).toBeInTheDocument();
  });
});

describe('UC-02 resource type details and copy', () => {
  it('shows GET inputs/outputs with names, types, required and secret exactly as returned', async () => {
    const user = userEvent.setup();
    backend();
    render(<ResourceTypesPage />);
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
    await open(user, 'cache');
    const inputsTable = screen.getByRole('table', { name: 'Đầu vào của cache' });
    const inputs = within(inputsTable);
    expect(inputs.getAllByRole('columnheader').map((cell) => cell.textContent)).toEqual(['Tên', 'Kiểu', 'Bắt buộc']);
    const rows = (table: HTMLElement) => within(table).getAllByRole('row').slice(1).map((row) => within(row).getAllByRole('cell').map((cell) => cell.textContent));
    expect(rows(inputsTable)).toEqual([['size', 'number', 'Có'], ['tier', 'any', 'Không']]);
    const outputsTable = screen.getByRole('table', { name: 'Đầu ra của cache' });
    const outputs = within(outputsTable);
    expect(outputs.getAllByRole('columnheader').map((cell) => cell.textContent)).toEqual(['Tên', 'Kiểu', 'Bắt buộc', 'Bí mật']);
    expect(rows(outputsTable)).toEqual([['password', 'string', 'Không', 'Không'], ['endpoint', 'string', 'Có', 'Có'], ['flag', 'bool', 'Không', 'Không']]);
    await user.click(screen.getByRole('button', { name: 'Ẩn schema cache' }));
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });

  it('shows an empty schema without inventing parameters', async () => {
    const user = userEvent.setup();
    backend({ types: [{ key: 'empty' }] });
    render(<ResourceTypesPage />);
    await open(user, 'empty');
    const details = within(screen.getByRole('region', { name: 'Schema của empty' }));
    expect(details.getByText('Chưa khai báo tham số đầu vào.')).toBeInTheDocument();
    expect(details.getByText('Chưa khai báo tham số đầu ra.')).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });

  it('copies flags and types verbatim into a blank-ID draft without any request', async () => {
    const user = userEvent.setup();
    const { posted, gets } = backend();
    render(<ResourceTypesPage />);
    await open(user, 'cache');
    const before = gets.length;
    await user.click(screen.getByRole('button', { name: 'Sao chép schema cache' }));
    expect(screen.getByLabelText('ID loại tài nguyên')).toHaveValue('');
    expect(screen.getByLabelText('ID loại tài nguyên')).toHaveFocus();
    expect(screen.getByText(/Bản nháp sao chép từ/)).toHaveTextContent('cache');
    expect(screen.getByLabelText('Đầu vào 1 tên')).toHaveValue('size');
    expect(screen.getByLabelText('Đầu vào 1 kiểu')).toHaveValue('number');
    expect(screen.getByLabelText('Đầu vào 1 bắt buộc')).toBeChecked();
    expect(screen.getByLabelText('Đầu vào 2 kiểu')).toHaveValue('any');
    expect(screen.getByLabelText('Đầu vào 2 bắt buộc')).not.toBeChecked();
    // "password" stays non-secret and "endpoint" stays secret: names never decide flags.
    expect(screen.getByLabelText('Đầu ra 1 tên')).toHaveValue('password');
    expect(screen.getByLabelText('Đầu ra 1 bí mật')).not.toBeChecked();
    expect(screen.getByLabelText('Đầu ra 2 tên')).toHaveValue('endpoint');
    expect(screen.getByLabelText('Đầu ra 2 bí mật')).toBeChecked();
    expect(screen.getByLabelText('Đầu ra 2 bắt buộc')).toBeChecked();
    expect(screen.getByLabelText('Đầu ra 3 kiểu')).toHaveValue('bool');
    expect(posted).toEqual([]);
    expect(gets.length).toBe(before);
  });

  it('keeps an independent draft: edits never change the source and POST sends the edited copy', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    render(<ResourceTypesPage />);
    await open(user, 'cache');
    await user.click(screen.getByRole('button', { name: 'Sao chép schema cache' }));
    await user.type(screen.getByLabelText('ID loại tài nguyên'), 'cache-v2');
    await user.clear(screen.getByLabelText('Đầu vào 1 tên'));
    await user.type(screen.getByLabelText('Đầu vào 1 tên'), 'capacity');
    await user.click(screen.getByLabelText('Đầu ra 1 bí mật'));
    await user.click(screen.getByRole('button', { name: 'Xóa đầu vào 2' }));
    // The source schema still shows its own data.
    const table = within(screen.getByRole('table', { name: 'Đầu vào của cache' }));
    expect(table.getByText('size')).toBeInTheDocument();
    expect(table.getByText('tier')).toBeInTheDocument();
    expect(within(screen.getByRole('table', { name: 'Đầu ra của cache' })).getAllByText('Không').length).toBeGreaterThan(0);
    await user.click(screen.getByRole('button', { name: 'Đăng ký loại tài nguyên' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký loại tài nguyên cache-v2.');
    expect(posted).toEqual([{
      key: 'cache-v2',
      inputs: [{ name: 'capacity', type: 'number', required: true }],
      outputs: [{ name: 'password', type: 'string', required: false, secret: true }, { name: 'endpoint', type: 'string', required: true, secret: true }, { name: 'flag', type: 'bool', required: false, secret: false }],
    }]);
    expect(screen.getByLabelText('ID loại tài nguyên')).toHaveValue('');
    expect(screen.queryByText(/Bản nháp sao chép từ/)).not.toBeInTheDocument();
  });

  it('preserves a field type the form does not list instead of rewriting it', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ types: [{ key: 'odd', inputs: [{ name: 'blob', type: 'bytes', required: true }], outputs: [] }] });
    render(<ResourceTypesPage />);
    await open(user, 'odd');
    await user.click(screen.getByRole('button', { name: 'Sao chép schema odd' }));
    expect(screen.getByLabelText('Đầu vào 1 kiểu')).toHaveValue('bytes');
    await user.type(screen.getByLabelText('ID loại tài nguyên'), 'odd-2');
    await user.click(screen.getByRole('button', { name: 'Đăng ký loại tài nguyên' }));
    await screen.findByRole('status');
    expect(posted[0]).toMatchObject({ inputs: [{ name: 'blob', type: 'bytes', required: true }] });
  });

  it('keeps the draft and the source when the new ID is a duplicate (409), then registers a new ID', async () => {
    const user = userEvent.setup();
    const answers = [() => Response.json({ error: 'resource type "cache" already exists' }, { status: 409 }), (body: Body) => Response.json(body, { status: 201 })];
    const { posted } = backend({ post: (body) => (answers.shift() as (body: Body) => Response)(body) });
    render(<ResourceTypesPage />);
    await open(user, 'cache');
    await user.click(screen.getByRole('button', { name: 'Sao chép schema cache' }));
    await user.type(screen.getByLabelText('ID loại tài nguyên'), 'cache');
    await user.click(screen.getByLabelText('Đầu vào 2 bắt buộc'));
    await user.click(screen.getByRole('button', { name: 'Đăng ký loại tài nguyên' }));
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('ID loại tài nguyên đã tồn tại');
    expect(alert.textContent).not.toMatch(/already exists/);
    expect(screen.getByLabelText('ID loại tài nguyên')).toHaveValue('cache');
    expect(screen.getByLabelText('Đầu vào 2 bắt buộc')).toBeChecked();
    expect(screen.getByLabelText('Đầu ra 2 bí mật')).toBeChecked();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    // Source untouched: tier is still not required in the details.
    expect(within(screen.getByRole('table', { name: 'Đầu vào của cache' })).getByText('tier').closest('tr')).toHaveTextContent('Không');
    await user.type(screen.getByLabelText('ID loại tài nguyên'), '-v2');
    await user.click(screen.getByRole('button', { name: 'Đăng ký loại tài nguyên' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký loại tài nguyên cache-v2.');
    expect(posted.map((body) => body.key)).toEqual(['cache', 'cache-v2']);
  });

  it('blocks copy while a registration is saving and does not touch the draft', async () => {
    const user = userEvent.setup();
    let release: (response: Response) => void = () => undefined;
    const { posted } = backend({ post: () => new Promise<Response>((done) => { release = done; }) as unknown as Response });
    render(<ResourceTypesPage />);
    await open(user, 'cache');
    await user.type(screen.getByLabelText('ID loại tài nguyên'), 'draft-one');
    await user.click(screen.getByRole('button', { name: 'Đăng ký loại tài nguyên' }));
    expect(screen.getByRole('button', { name: 'Sao chép schema cache' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Sao chép schema cache' }));
    expect(screen.getByLabelText('ID loại tài nguyên')).toHaveValue('draft-one');
    expect(screen.queryByLabelText('Đầu vào 1 tên')).not.toBeInTheDocument();
    release(Response.json({}, { status: 201 }));
    await screen.findByRole('status');
    expect(posted).toHaveLength(1);
  });

  it('offers no edit, delete or version control on registered types', async () => {
    const user = userEvent.setup();
    backend();
    render(<ResourceTypesPage />);
    await open(user, 'cache');
    const scope = within(entry('cache'));
    expect(scope.getAllByRole('button').map((button) => button.textContent)).toEqual(['Ẩn schema', 'Sao chép schema']);
    expect(screen.queryByRole('button', { name: /sửa|xóa loại|phiên bản|version|delete|edit/i })).not.toBeInTheDocument();
  });
});

describe('UC-02 supported whitelist and definition CTA', () => {
  it('labels exactly the four runtime-supported types and every other ID as contract-only', async () => {
    backend({ types: seeded });
    render(<ResourceTypesPage />);
    await screen.findByText('vpc', { selector: 'strong' });
    const label = (key: string) => within(entry(key)).getByText(/Có cách tạo hỗ trợ|Chỉ có contract/).textContent;
    for (const key of ['vpc', 'k8s-cluster', 'k8s-namespace', 'postgres']) expect(label(key), key).toBe('Có cách tạo hỗ trợ');
    for (const key of ['workload', 'postgres-ha', 'constructor', 'toString']) expect(label(key), key).toBe('Chỉ có contract');
    expect(entry('vpc')).toHaveTextContent('Terraform/vpc');
    expect(entry('k8s-cluster')).toHaveTextContent('Terraform/eks');
    expect(entry('k8s-namespace')).toHaveTextContent('Kubernetes');
    expect(entry('k8s-namespace')).not.toHaveTextContent('Terraform');
    expect(entry('postgres')).toHaveTextContent('Kubernetes hoặc Terraform/aurora');
    expect(entry('postgres-ha')).not.toHaveTextContent('Cách tạo');
  });

  it('shows the Definition CTA only for supported types and points workload to rendering templates', async () => {
    const user = userEvent.setup();
    backend({ types: seeded });
    render(<ResourceTypesPage />);
    await screen.findByText('vpc', { selector: 'strong' });
    const ctas = screen.getAllByRole('link', { name: /^Tạo Cấu hình tài nguyên cho / }).map((link) => link.getAttribute('href'));
    expect(ctas).toEqual([
      '/ui/platform/resource-definitions?resourceType=vpc',
      '/ui/platform/resource-definitions?resourceType=k8s-cluster',
      '/ui/platform/resource-definitions?resourceType=k8s-namespace',
      '/ui/platform/resource-definitions?resourceType=postgres',
    ]);
    for (const key of ['workload', 'postgres-ha', 'constructor', 'toString']) expect(within(entry(key)).queryByRole('link', { name: /Tạo Cấu hình tài nguyên/ })).not.toBeInTheDocument();
    const guidance = within(entry('workload')).getByRole('link', { name: 'Mẫu dựng ứng dụng' });
    expect(guidance).toHaveAttribute('href', '/ui/platform/rendering-templates');
    await user.click(guidance);
    expect(window.location.pathname).toBe('/ui/platform/rendering-templates');
  });

  it('navigates the CTA to the definition form with the technical key in the query', async () => {
    const user = userEvent.setup();
    backend({ types: seeded });
    render(<ResourceTypesPage />);
    await user.click(await screen.findByRole('link', { name: 'Tạo Cấu hình tài nguyên cho postgres' }));
    expect(window.location.pathname + window.location.search).toBe('/ui/platform/resource-definitions?resourceType=postgres');
  });
});
