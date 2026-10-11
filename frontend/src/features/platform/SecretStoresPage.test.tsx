import { fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { SecretStoresPage, caShapeProblem, validate } from './SecretStoresPage';

afterEach(() => vi.unstubAllGlobals());

const store = { key: 'team-vault', name: 'Team Vault', provider: 'VAULT_KV_V2', backendAddress: 'http://vault:8200', workloadAddress: 'http://vault.vault.svc:8200', mount: 'kv', authMount: 'kubernetes', tlsCustomCa: false, status: 'READY', verification: { verified: true, kubernetesAuth: 'CONFIGURED' } };
const EMPTY = 'Chưa có kho bí mật nào được đăng ký.';
const SUBMIT = { name: 'Xác minh và đăng ký' };
const body = (call: unknown[] | undefined) => JSON.parse(String((call?.[1] as RequestInit).body));
const posts = () => vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === 'POST');
// A structurally valid certificate block; it is not a real certificate.
const PEM = '-----BEGIN CERTIFICATE-----\nTUlJQmtUQ0IrNkFEQWdFQ0FoUUE=\n-----END CERTIFICATE-----';

function stub(onPost: () => Response | Promise<Response>, list: () => Response | Promise<Response> = () => Response.json({ secretStores: [] })) {
  vi.stubGlobal('fetch', vi.fn(async (_input: string | URL | Request, init?: RequestInit) => (init?.method === 'POST' ? onPost() : list())));
}

async function fill(user: ReturnType<typeof userEvent.setup>, over: { name?: string; backend?: string; workload?: string; token?: string } = {}) {
  await user.type(screen.getByLabelText('Tên kho'), over.name ?? 'Team Vault');
  await user.type(screen.getByLabelText('Địa chỉ backend'), over.backend ?? 'http://vault:8200');
  await user.type(screen.getByLabelText('Địa chỉ workload'), over.workload ?? 'http://vault.vault.svc:8200');
  await user.type(screen.getByLabelText(/^Token/), over.token ?? 'hvs.secret-token');
}

async function open() {
  const user = userEvent.setup();
  render(<SecretStoresPage />);
  await screen.findByText(EMPTY);
  return user;
}

it('registers with a concealed token, sends defaults, shows safe Vietnamese metadata and clears the form', async () => {
  let created = false;
  stub(() => { created = true; return Response.json(store, { status: 201 }); }, () => Response.json({ secretStores: created ? [store] : [] }));
  const user = await open();
  expect(screen.getByRole('heading', { level: 1, name: 'Kho bí mật' })).toBeInTheDocument();
  const token = screen.getByLabelText(/^Token/);
  expect(token).toHaveAttribute('type', 'password');
  await fill(user);
  await user.click(screen.getByRole('button', SUBMIT));
  expect(await screen.findByRole('status')).toHaveTextContent('Kho bí mật "Team Vault" đã sẵn sàng');
  expect(posts()).toHaveLength(1);
  expect(body(posts()[0])).toEqual({ name: 'Team Vault', backendAddress: 'http://vault:8200', workloadAddress: 'http://vault.vault.svc:8200', mount: 'kv', authMount: 'kubernetes', token: 'hvs.secret-token' });
  expect(await screen.findByText(/Team Vault/, { selector: 'strong' })).toBeInTheDocument();
  expect(screen.getByLabelText(/^Token/)).toHaveValue('');
  expect(screen.getByLabelText('Tên kho')).toHaveValue('');
  expect(document.body.textContent).not.toContain('hvs.secret-token');
});

it('always shows effective defaults and keeps advanced values when the section is closed', async () => {
  stub(() => Response.json(store, { status: 201 }));
  const user = await open();
  const summary = screen.getByLabelText('Tóm tắt cấu hình hiệu lực');
  expect(summary).toHaveTextContent('Mount KV v2: kv (mặc định)');
  expect(summary).toHaveTextContent('Mount xác thực Kubernetes: kubernetes (mặc định)');
  expect(summary).toHaveTextContent('CA: mặc định của hệ thống');
  expect(screen.getByLabelText('Mount KV v2')).not.toBeVisible();
  const toggle = screen.getByRole('button', { name: 'Hiện nâng cao' });
  expect(toggle).toHaveAttribute('aria-expanded', 'false');
  await user.click(toggle);
  await user.type(screen.getByLabelText('Mount KV v2'), 'team-kv');
  await user.type(screen.getByLabelText('Mount xác thực Kubernetes'), 'k8s_prod');
  fireEvent.change(screen.getByLabelText(/^Chứng chỉ CA tùy chỉnh/), { target: { value: PEM } });
  await user.click(screen.getByRole('button', { name: 'Ẩn nâng cao' }));
  expect(summary).toHaveTextContent('Mount KV v2: team-kv');
  expect(summary).not.toHaveTextContent('Mount KV v2: team-kv (mặc định)');
  expect(summary).toHaveTextContent('Mount xác thực Kubernetes: k8s_prod');
  expect(summary).toHaveTextContent('CA: tùy chỉnh');
  await user.click(screen.getByRole('button', { name: 'Hiện nâng cao' }));
  expect(screen.getByLabelText('Mount KV v2')).toHaveValue('team-kv');
  await fill(user);
  await user.click(screen.getByRole('button', SUBMIT));
  await screen.findByRole('status');
  expect(body(posts()[0])).toMatchObject({ mount: 'team-kv', authMount: 'k8s_prod', tlsCaPem: PEM });
});

it('same-address checkbox is off first, follows backend edits when on and restores the independent workload draft when off', async () => {
  stub(() => Response.json(store, { status: 201 }));
  const user = await open();
  const same = screen.getByLabelText('Dùng cùng địa chỉ backend');
  expect(same).not.toBeChecked();
  await user.type(screen.getByLabelText('Địa chỉ backend'), 'http://a:8200');
  await user.type(screen.getByLabelText('Địa chỉ workload'), 'http://workload:8200');
  await user.click(same);
  expect(screen.getByLabelText('Địa chỉ workload')).toHaveValue('http://a:8200');
  await user.type(screen.getByLabelText('Địa chỉ backend'), '/x');
  expect(screen.getByLabelText('Địa chỉ workload')).toHaveValue('http://a:8200/x');
  await user.type(screen.getByLabelText(/^Tên kho/), 'S');
  await user.type(screen.getByLabelText(/^Token/), 't');
  await user.click(screen.getByRole('button', SUBMIT));
  await screen.findByRole('status');
  expect(body(posts()[0])).toMatchObject({ backendAddress: 'http://a:8200/x', workloadAddress: 'http://a:8200/x' });
});

it('toggling the checkbox off restores the separate workload draft untouched', async () => {
  stub(() => Response.json(store, { status: 201 }));
  const user = await open();
  const same = screen.getByLabelText('Dùng cùng địa chỉ backend');
  await user.type(screen.getByLabelText('Địa chỉ backend'), 'http://a:8200');
  await user.type(screen.getByLabelText('Địa chỉ workload'), 'http://workload:8200');
  await user.click(same);
  await user.type(screen.getByLabelText('Địa chỉ backend'), '/1');
  await user.click(same);
  expect(screen.getByLabelText('Địa chỉ workload')).toHaveValue('http://workload:8200');
  await user.type(screen.getByLabelText('Tên kho'), 'S');
  await user.type(screen.getByLabelText(/^Token/), 't');
  await user.click(screen.getByRole('button', SUBMIT));
  await screen.findByRole('status');
  expect(body(posts()[0])).toMatchObject({ backendAddress: 'http://a:8200/1', workloadAddress: 'http://workload:8200' });
});

it('does not POST invalid input and names each field in Vietnamese', async () => {
  stub(() => Response.json(store, { status: 201 }));
  const user = await open();
  await user.click(screen.getByRole('button', SUBMIT));
  expect(posts()).toHaveLength(0);
  expect(screen.getByText('Tên kho là bắt buộc.')).toBeInTheDocument();
  expect(screen.getByText('Địa chỉ backend là bắt buộc.')).toBeInTheDocument();
  expect(screen.getByText('Địa chỉ workload là bắt buộc.')).toBeInTheDocument();
  expect(screen.getByText('Token là bắt buộc.')).toBeInTheDocument();
  expect(screen.getByLabelText('Tên kho')).toHaveAttribute('aria-invalid', 'true');
  await fill(user, { backend: 'ftp://vault', workload: 'http://u:p@vault:8200', token: 'x' });
  await user.click(screen.getByRole('button', { name: 'Hiện nâng cao' }));
  await user.type(screen.getByLabelText('Mount KV v2'), 'bad/mount');
  await user.click(screen.getByRole('button', SUBMIT));
  expect(posts()).toHaveLength(0);
  expect(screen.getByText(/^Địa chỉ backend phải là URL http\(s\)/)).toBeInTheDocument();
  expect(screen.getByText(/^Địa chỉ workload phải là URL http\(s\)/)).toBeInTheDocument();
  expect(screen.getByText(/^Mount KV v2 chỉ gồm 1–64/)).toBeInTheDocument();
});

it('opens the advanced section on an invalid mount, then keeps the visible actionable error and sends no POST', async () => {
  stub(() => Response.json(store, { status: 201 }));
  const user = await open();
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Hiện nâng cao' }));
  await user.type(screen.getByLabelText('Mount KV v2'), 'bad/mount');
  await user.click(screen.getByRole('button', { name: 'Ẩn nâng cao' }));
  expect(screen.getByLabelText('Mount KV v2')).not.toBeVisible();
  await user.click(screen.getByRole('button', SUBMIT));
  expect(posts()).toHaveLength(0);
  expect(screen.getByRole('button', { name: 'Ẩn nâng cao' })).toHaveAttribute('aria-expanded', 'true');
  expect(screen.getByLabelText('Mount KV v2')).toBeVisible();
  expect(screen.getByText(/^Mount KV v2 chỉ gồm 1–64/)).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Ẩn nâng cao' }));
  fireEvent.change(screen.getByLabelText('Mount KV v2'), { target: { value: '' } });
  fireEvent.change(screen.getByLabelText(/^Chứng chỉ CA tùy chỉnh/), { target: { value: 'not pem' } });
  await user.click(screen.getByRole('button', SUBMIT));
  expect(posts()).toHaveLength(0);
  expect(screen.getByText(/^CA PEM chỉ nhận/)).toBeVisible();
});

const base = { name: 'n', backendAddress: 'http://v:8200', workloadAddress: 'http://w:8200', sameAddress: false, mount: '', authMount: '', tlsCaPem: '', token: 't' };

it('validates URL, mount, name, token and CA boundaries', () => {
  expect(validate(base)).toEqual({});
  for (const bad of ['https://', 'http:///x', 'http:\\\\vault', 'http://vault\\path', 'http://va\u0000ult', 'http://va\u0007ult', 'http:/vault', 'vault:8200', 'http://h/?q=1', 'http://h/#f', 'http://h/ x', 'http://u@h', 'http://:p@h', 'file:///x', ' ']) {
    expect(validate({ ...base, backendAddress: bad }).backendAddress, bad).toBeTruthy();
  }
  expect(validate({ ...base, backendAddress: ' https://v.example:8200/base ' })).toEqual({});
  expect(validate({ ...base, mount: 'a'.repeat(64), authMount: 'A_b-9' })).toEqual({});
  for (const bad of ['a'.repeat(65), 'a/b', 'a b', 'kv.v2', 'kvé']) expect(validate({ ...base, mount: bad }).mount, bad).toBeTruthy();
  expect(validate({ ...base, name: 'é'.repeat(100) })).toEqual({});
  expect(validate({ ...base, name: 'é'.repeat(101) }).name).toBeTruthy();
  expect(validate({ ...base, name: 'a\u0007b' }).name).toBeTruthy();
  expect(validate({ ...base, name: '   ' }).name).toBeTruthy();
  expect(validate({ ...base, token: 'a'.repeat(4096) })).toEqual({});
  expect(validate({ ...base, token: 'a'.repeat(4097) }).token).toBeTruthy();
  expect(validate({ ...base, token: 'é'.repeat(2048) })).toEqual({});
  expect(validate({ ...base, token: 'é'.repeat(2049) }).token).toBeTruthy();
  expect(validate({ ...base, token: 'a\nb' }).token).toBeTruthy();
  expect(validate({ ...base, token: '  ' }).token).toBeTruthy();
  expect(validate({ ...base, sameAddress: true, workloadAddress: '' })).toEqual({});
  expect(validate({ ...base, sameAddress: true, backendAddress: 'bad' }).workloadAddress).toBeUndefined();
});

it('checks only the CA PEM shape and size, and the request envelope', () => {
  expect(caShapeProblem('')).toBe('');
  expect(caShapeProblem(PEM)).toBe('');
  expect(caShapeProblem(`${PEM}\n${PEM}\n`)).toBe('');
  expect(caShapeProblem(`${PEM}\r\n`)).toBe('');
  expect(caShapeProblem('not pem')).toBeTruthy();
  expect(caShapeProblem(`note\n${PEM}`)).toBeTruthy();
  expect(caShapeProblem(`${PEM}\ntrailing`)).toBeTruthy();
  expect(caShapeProblem(PEM.replace('CERTIFICATE', 'PRIVATE KEY'))).toBeTruthy();
  expect(caShapeProblem(PEM.replace('TUlJ', 'T!JJ'))).toBeTruthy();
  expect(caShapeProblem(PEM.replace('UUE=', 'UUE'))).toBeTruthy();
  const wrap = (letters: number, breaks: number) => `-----BEGIN CERTIFICATE-----\n${'A'.repeat(letters)}${'\n'.repeat(breaks)}-----END CERTIFICATE-----`;
  expect(new TextEncoder().encode(wrap(65480, 3)).length).toBe(64 * 1024);
  expect(caShapeProblem(wrap(65480, 3))).toBe('');
  expect(new TextEncoder().encode(wrap(65480, 4)).length).toBe(64 * 1024 + 1);
  expect(caShapeProblem(wrap(65480, 4))).toMatch(/64 KiB/);
  // Many valid blocks stay below the CA limit but the envelope guard still applies at 256 KiB.
  expect(validate({ ...base, tlsCaPem: PEM })).toEqual({});
  expect(validate({ ...base, backendAddress: `http://h/${'a'.repeat(100_000)}` }).form).toBeUndefined();
  expect(validate({ ...base, backendAddress: `http://h/${'a'.repeat(256 * 1024)}` }).form).toMatch(/256 KiB/);
});

it('keeps metadata, advanced choices and the same-address option, and clears the token on every server failure without raw errors', async () => {
  for (const status of [400, 401, 403, 413, 422, 500, 503]) {
    const raw = `RAW-PROVIDER-ERROR-${status} vault at http://10.0.0.1`;
    stub(() => Response.json({ error: raw }, { status }));
    const { unmount } = render(<SecretStoresPage />);
    const user = userEvent.setup();
    await screen.findByText(EMPTY);
    await user.type(screen.getByLabelText('Tên kho'), 'Bad');
    await user.type(screen.getByLabelText('Địa chỉ backend'), 'http://v:8200');
    await user.click(screen.getByLabelText('Dùng cùng địa chỉ backend'));
    await user.click(screen.getByRole('button', { name: 'Hiện nâng cao' }));
    await user.type(screen.getByLabelText('Mount KV v2'), 'team-kv');
    await user.type(screen.getByLabelText(/^Token/), 'wrong');
    await user.click(screen.getByRole('button', SUBMIT));
    const alert = await screen.findByRole('alert');
    expect(alert.textContent, String(status)).toMatch(/\S/);
    expect(document.body.textContent).not.toContain('RAW-PROVIDER-ERROR');
    expect(document.body.textContent).not.toContain('10.0.0.1');
    expect(screen.getByLabelText('Tên kho')).toHaveValue('Bad');
    expect(screen.getByLabelText('Địa chỉ backend')).toHaveValue('http://v:8200');
    expect(screen.getByLabelText('Dùng cùng địa chỉ backend')).toBeChecked();
    expect(screen.getByLabelText('Mount KV v2')).toHaveValue('team-kv');
    expect(screen.getByLabelText('Mount KV v2')).toBeVisible();
    expect(screen.getByLabelText(/^Token/)).toHaveValue('');
    expect(posts()).toHaveLength(1);
    unmount();
  }
});

it('treats a lost response as an unknown outcome: token cleared, no claim that nothing was saved, no automatic resubmit', async () => {
  stub(() => { throw new TypeError('network down'); });
  const user = await open();
  await fill(user);
  await user.click(screen.getByRole('button', SUBMIT));
  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent('chưa biết kho đã được lưu hay chưa');
  expect(alert).not.toHaveTextContent('network down');
  expect(screen.getByLabelText(/^Token/)).toHaveValue('');
  expect(screen.getByLabelText('Tên kho')).toHaveValue('Team Vault');
  expect(posts()).toHaveLength(1);
});

it('ignores a second submit while the first is in flight', async () => {
  let release: (response: Response) => void = () => undefined;
  stub(() => new Promise<Response>((resolve) => { release = resolve; }));
  const user = await open();
  await fill(user);
  const form = screen.getByRole('form', { name: 'Đăng ký kho bí mật' });
  fireEvent.submit(form);
  fireEvent.submit(form);
  expect(await screen.findByRole('button', { name: 'Đang xác minh…' })).toBeDisabled();
  expect(posts()).toHaveLength(1);
  expect(screen.getByLabelText('Tên kho')).toBeDisabled();
  release(Response.json(store, { status: 201 }));
  await screen.findByRole('status');
  expect(posts()).toHaveLength(1);
});

it('keeps registration success separate from a list reload failure and retries only GET', async () => {
  let created = false;
  let failList = false;
  stub(() => { created = true; failList = true; return Response.json(store, { status: 201 }); }, () => (failList ? Response.json({ error: 'boom' }, { status: 500 }) : Response.json({ secretStores: created ? [store] : [] })));
  const user = await open();
  await fill(user);
  await user.click(screen.getByRole('button', SUBMIT));
  expect(await screen.findByText(/^Không tải được danh sách kho bí mật/)).toBeInTheDocument();
  expect(screen.getByRole('status')).toHaveTextContent('đã sẵn sàng');
  expect(screen.queryByText(EMPTY)).not.toBeInTheDocument();
  failList = false;
  await user.click(screen.getByRole('button', { name: 'Thử lại' }));
  expect(await screen.findByText(/Team Vault/, { selector: 'strong' })).toBeInTheDocument();
  expect(posts()).toHaveLength(1);
});

it('keeps typed metadata when the first list load fails and never shows the empty state as success', async () => {
  stub(() => Response.json(store, { status: 201 }), () => Response.json({ error: 'x' }, { status: 500 }));
  const user = userEvent.setup();
  render(<SecretStoresPage />);
  expect(screen.getByText('Đang tải danh sách kho bí mật…')).toBeInTheDocument();
  expect(await screen.findByText(/^Không tải được danh sách kho bí mật/)).toBeInTheDocument();
  expect(screen.queryByText(EMPTY)).not.toBeInTheDocument();
  await user.type(screen.getByLabelText('Tên kho'), 'Giữ nguyên');
  await user.click(screen.getByRole('button', { name: 'Thử lại' }));
  expect(await screen.findByText(/^Không tải được danh sách kho bí mật/)).toBeInTheDocument();
  expect(screen.getByLabelText('Tên kho')).toHaveValue('Giữ nguyên');
});

it('lists both addresses, backend verification and Kubernetes auth separately, without token or credential reference', async () => {
  const rows = [
    store,
    { ...store, key: 'absent', name: 'Absent', status: 'READY', tlsCustomCa: true, verification: { verified: true, kubernetesAuth: 'ABSENT' } },
    { ...store, key: 'unknown', name: 'Unknown', status: 'READY', verification: {} },
    { ...store, key: 'nullv', name: 'NullV', status: 'READY', verification: null, credentialRef: 'secret-ref-123', token: 'hvs.leak' },
    { ...store, key: 'rej', name: 'Rej', status: 'REJECTED' },
    { ...store, key: 'old', name: 'Old', legacy: true, status: 'PENDING', verification: { verified: false, kubernetesAuth: 'weird' } },
  ];
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ secretStores: rows })));
  render(<SecretStoresPage />);
  const list = await screen.findByLabelText('Kho bí mật đã đăng ký');
  const row = (name: string) => within(list).getAllByText((_, node) => node?.tagName === 'STRONG' && node.firstChild?.textContent?.replace(' (cấu hình cũ)', '') === name)[0]?.closest('.settings-table-row') as HTMLElement;
  expect(row('Team Vault')).toHaveTextContent('Backend: http://vault:8200');
  expect(row('Team Vault')).toHaveTextContent('Workload: http://vault.vault.svc:8200');
  expect(row('Team Vault')).toHaveTextContent('Trạng thái: Sẵn sàng');
  expect(row('Team Vault')).toHaveTextContent('Xác minh backend: Backend đã xác minh khi đăng ký');
  expect(row('Team Vault')).toHaveTextContent('Xác thực Kubernetes: Đã cấu hình');
  expect(row('Team Vault')).toHaveTextContent('CA: mặc định');
  expect(row('Absent')).toHaveTextContent('Xác thực Kubernetes: Chưa cấu hình');
  expect(row('Absent')).toHaveTextContent('CA: tùy chỉnh');
  expect(row('Unknown')).toHaveTextContent('Xác minh backend: Chưa có kết quả xác minh backend');
  expect(row('Unknown')).toHaveTextContent('Xác thực Kubernetes: Chưa rõ');
  expect(row('NullV')).toHaveTextContent('Xác thực Kubernetes: Chưa rõ');
  expect(row('Rej')).toHaveTextContent('Trạng thái: Bị từ chối');
  expect(row('Old')).toHaveTextContent('(cấu hình cũ)');
  expect(row('Old')).toHaveTextContent('Trạng thái: Trạng thái chưa nhận diện (PENDING)');
  expect(row('Old')).not.toHaveTextContent('legacy');
  expect(row('Old')).toHaveTextContent('Xác minh backend: Backend xác minh không đạt');
  expect(row('Old')).toHaveTextContent('Xác thực Kubernetes: Chưa rõ');
  expect(list).toHaveTextContent('không đảm bảo ứng dụng đã sẵn sàng triển khai');
  expect(document.body.textContent).not.toMatch(/secret-ref-123|hvs\.leak/);
});
