import { act, render, screen, waitFor } from '@testing-library/react';
import { capabilityFixture } from '../test/capabilities';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';

type Handler = (url: string, init?: RequestInit) => Response | Promise<Response> | undefined;

let authenticated: boolean;
type StubEnvironment = { key: string; version: number; secretStoreKey?: string; targetGeneration?: number; activeOperation?: { id: string; status: string } };
type StubApplication = { key: string; name: string; subdomain: string; environments: StubEnvironment[] };
const environmentFixture = (key: string): StubEnvironment => ({ key, version: 1, secretStoreKey: '', targetGeneration: 0 });
let applications: StubApplication[];
let requests: { url: string; method: string; body?: string }[];
let override: Handler | undefined;

function stubBackend() {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    requests.push({ url, method: init?.method ?? 'GET', body: init?.body ? String(init.body) : undefined });
    const custom = await override?.(url, init);
    if (custom) return custom;
    if (url.endsWith('/auth/session')) return authenticated ? Response.json({ user: { Username: 'developer', Role: 'DEVELOPER' } }) : Response.json({ error: 'unauthorized' }, { status: 401 });
    if (url.endsWith('/auth/sign-in')) { authenticated = true; return Response.json({ user: { Username: 'developer', Role: 'DEVELOPER' } }); }
    if (url.endsWith('/auth/sign-out')) { authenticated = false; return new Response(null, { status: 204 }); }
    if (!authenticated) return Response.json({ error: 'unauthorized' }, { status: 401 });
    if (url.endsWith('/applications') && init?.method === 'POST') {
      const body = JSON.parse(String(init.body)) as { name: string; subdomain: string };
      const created: StubApplication = { key: 'generated-id', name: body.name, subdomain: body.subdomain, environments: [environmentFixture('staging'), environmentFixture('production')] };
      applications.push(created);
      return Response.json({ application: created }, { status: 201 });
    }
    if (url.endsWith('/applications')) return Response.json({ applications });
    if (url.includes('/configuration')) return Response.json({ applicationKey: 'x', environmentKey: 'staging', version: 0, keys: [] });
    if (url.includes('/workloads')) return Response.json({ draftVersion: 0, workloads: [] });
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (/\/resources$/.test(url)) return Response.json({ resources: [] });
    return Response.json({});
  }));
}

async function signIn(user: ReturnType<typeof userEvent.setup>) {
  await user.type(await screen.findByLabelText('Tên đăng nhập'), 'developer');
  await user.type(screen.getByLabelText('Mật khẩu'), 'test-password');
  await user.click(screen.getByRole('button', { name: 'Đăng nhập' }));
  await screen.findByRole('heading', { name: 'Ứng dụng của bạn' });
}

describe('developer onboarding shell (UC-00/UC-01)', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/ui/sign-in');
    authenticated = false;
    applications = [{ key: 'payment', name: 'Payment', subdomain: 'payment', environments: [environmentFixture('staging'), environmentFixture('production')] }];
    requests = [];
    override = undefined;
    stubBackend();
  });
  afterEach(() => vi.unstubAllGlobals());

  it('signs in to the applications home', async () => {
    const user = userEvent.setup();
    render(<App />);
    await signIn(user);
    expect(await screen.findByText('Payment')).toBeInTheDocument();
    expect(window.location.pathname).toBe('/ui/applications');
  });

  it('restores an existing session without showing sign-in', async () => {
    authenticated = true;
    render(<App />);
    expect(await screen.findByRole('heading', { name: 'Ứng dụng của bạn' })).toBeInTheDocument();
    expect(screen.queryByLabelText('Mật khẩu')).not.toBeInTheDocument();
    expect(window.location.pathname).toBe('/ui/applications');
  });

  it('redirects protected routes to sign-in without a session', async () => {
    window.history.replaceState({}, '', '/ui/applications/payment');
    render(<App />);
    expect(await screen.findByLabelText('Tên đăng nhập')).toBeInTheDocument();
    expect(requests.some((request) => request.url.endsWith('/applications'))).toBe(false);
  });

  it('shows a loading state, then the empty state', async () => {
    authenticated = true;
    applications = [];
    let release: () => void = () => undefined;
    override = (url) => url.endsWith('/applications') ? new Promise<Response>((resolve) => { release = () => resolve(Response.json({ applications: [] })); }) as unknown as Response : undefined;
    render(<App />);
    expect(await screen.findByRole('status', { name: 'Đang tải ứng dụng' })).toBeInTheDocument();
    release();
    expect(await screen.findByRole('heading', { name: 'Bắt đầu với ứng dụng mới' })).toBeInTheDocument();
  });

  it('shows a retryable list error without substituting data', async () => {
    authenticated = true;
    let failures = 1;
    override = (url) => url.endsWith('/applications') && failures-- > 0 ? Response.json({ error: 'database unavailable' }, { status: 500 }) : undefined;
    const user = userEvent.setup();
    render(<App />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Không thể tải danh sách ứng dụng.');
    expect(screen.getByRole('alert')).not.toHaveTextContent(/database unavailable/);
    expect(screen.queryByText(/database unavailable/)).not.toBeInTheDocument();
    expect(screen.queryByText('Payment')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Thử lại' }));
    expect(await screen.findByText('Payment')).toBeInTheDocument();
  });

  it('lists several applications in returned order with Vietnamese card labels and deep links', async () => {
    authenticated = true;
    applications = [
      { key: 'zeta', name: 'Zeta', subdomain: 'zeta', environments: [environmentFixture('staging'), environmentFixture('production')] },
      { key: 'alpha', name: 'Alpha', subdomain: 'alpha', environments: [environmentFixture('staging'), environmentFixture('production')] },
    ];
    const user = userEvent.setup();
    render(<App />);
    expect(await screen.findByText('Không gian làm việc')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Ứng dụng của bạn', level: 1 })).toBeInTheDocument();
    const cards = (await screen.findAllByText(/^(Zeta|Alpha)$/)).map((node) => node.textContent);
    expect(cards).toEqual(['Zeta', 'Alpha']);
    expect(screen.getAllByText('Sẵn sàng cấu hình')).toHaveLength(4);
    expect(screen.getAllByText('staging')).toHaveLength(2);
    expect(screen.getAllByText('production')).toHaveLength(2);
    expect(screen.queryByText(/^(Staging|Production)$/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Ready to configure|Workspace/)).not.toBeInTheDocument();
    expect(screen.queryByRole('searchbox')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Alpha/ }));
    expect(window.location.pathname).toBe('/ui/applications/alpha');
    expect(await screen.findByRole('heading', { name: 'Alpha' })).toBeInTheDocument();
  });

  it.each([
    ['header button', 0],
    ['empty-state button', 1],
  ])('opens the create form from the %s and creates nothing', async (_label, index) => {
    authenticated = true;
    applications = [];
    const user = userEvent.setup();
    render(<App />);
    expect(await screen.findByRole('heading', { name: 'Bắt đầu với ứng dụng mới' })).toBeInTheDocument();
    const buttons = screen.getAllByRole('button', { name: '+ Tạo ứng dụng' });
    expect(buttons).toHaveLength(2);
    await user.click(buttons[index]!);
    expect(await screen.findAllByRole('form', { name: 'Tạo ứng dụng' })).toHaveLength(1);
    expect(requests.some((request) => request.method === 'POST')).toBe(false);
  });

  it('restores an application deep link on initial load', async () => {
    authenticated = true;
    window.history.replaceState({}, '', '/ui/applications/payment');
    render(<App />);
    expect(await screen.findByRole('heading', { name: 'Payment' })).toBeInTheDocument();
    expect(window.location.pathname).toBe('/ui/applications/payment');
  });

  it('creates an application without a connection and Settings offers none', async () => {
    const user = userEvent.setup();
    render(<App />);
    await signIn(user);
    await user.click(screen.getAllByRole('button', { name: '+ Tạo ứng dụng' })[0]!);
    await user.type(screen.getByLabelText('Tên ứng dụng'), 'Catalog');
    expect(screen.getByLabelText('Subdomain')).toHaveValue('catalog');
    await user.clear(screen.getByLabelText('Subdomain'));
    await user.type(screen.getByLabelText('Subdomain'), 'Catalog');
    expect(screen.queryByLabelText('Connection')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Kết nối')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Tạo ứng dụng' }));

    expect(await screen.findByRole('heading', { name: 'Catalog' })).toBeInTheDocument();
    expect(screen.getByLabelText('Đích triển khai')).toHaveTextContent('staging · Đích được xác định từ Resource Definition và các tham chiếu Workload.');
    expect(screen.getByRole('status')).toHaveTextContent('Đã tạo ứng dụng.');
    expect(screen.getByRole('button', { name: /staging/ })).toHaveClass('tab-active');
    const create = requests.find((request) => request.method === 'POST' && request.url.endsWith('/applications'));
    expect(JSON.parse(create?.body ?? '{}')).toEqual({ name: 'Catalog', subdomain: 'catalog' });

    // ADR-017: Settings has no deployment Connection/profile/transition; resource Definitions select targets.
    await user.click(screen.getByRole('button', { name: 'Cài đặt môi trường' }));
    expect(await screen.findByRole('tab', { name: 'Staging' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByLabelText('Kết nối cho Staging')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Lưu kết nối' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'Production' }));
    expect(screen.queryByLabelText('Kết nối cho Production')).not.toBeInTheDocument();
    expect(requests.some((request) => request.method === 'PUT' && request.url.endsWith('/connection'))).toBe(false);
    expect(requests.some((request) => /\/(deploy|preview|deployments)$/.test(request.url) && request.method === 'POST')).toBe(false);
    await user.click(screen.getByRole('button', { name: /← Catalog/ }));
    expect(await screen.findByLabelText('Đích triển khai')).toHaveTextContent('staging · Đích được xác định từ Resource Definition và các tham chiếu Workload.');
  });

  it('keeps the Settings Environment aligned with the URL across tab clicks and back/forward', async () => {
    const user = userEvent.setup();
    window.history.replaceState({}, '', '/ui/applications/payment/settings?environment=production');
    authenticated = true;
    render(<App />);
    expect(await screen.findByRole('tab', { name: 'Production' })).toHaveAttribute('aria-selected', 'true');
    // ADR-017: Settings has no deployment target selector; Definition pins remain per resource.
    expect(screen.queryByLabelText('Kết nối triển khai')).not.toBeInTheDocument();
    await waitFor(() => expect(requests.some((request) => request.url.includes('/environments/production/configuration'))).toBe(true));
    // A tab click keeps the URL aligned, so a reload opens the same scope.
    await user.click(screen.getByRole('tab', { name: 'Staging' }));
    await waitFor(() => expect(window.location.search).toBe('?environment=staging'));
    expect(screen.getByRole('tab', { name: 'Staging' })).toHaveAttribute('aria-selected', 'true');
    // Back/forward to another scope re-scopes both the connection panel and the configuration.
    window.history.pushState({}, '', '/ui/applications/payment/settings?environment=production');
    await act(async () => { window.dispatchEvent(new PopStateEvent('popstate')); });
    await waitFor(() => expect(screen.getByRole('tab', { name: 'Production' })).toHaveAttribute('aria-selected', 'true'));
    expect(screen.queryByLabelText('Kết nối triển khai')).not.toBeInTheDocument();
    window.history.pushState({}, '', '/ui/applications/payment/settings');
    await act(async () => { window.dispatchEvent(new PopStateEvent('popstate')); });
    await waitFor(() => expect(screen.getByRole('tab', { name: 'Staging' })).toHaveAttribute('aria-selected', 'true'));
    expect(screen.queryByLabelText('Kết nối triển khai')).not.toBeInTheDocument();
  });

  it('returns to sign-in with a notice when the session expires', async () => {
    const user = userEvent.setup();
    render(<App />);
    await signIn(user);
    authenticated = false;
    await user.click(screen.getByText('Payment'));
    expect(await screen.findByText('Phiên đăng nhập đã kết thúc. Đăng nhập lại để tiếp tục.')).toBeInTheDocument();
    expect(window.location.pathname).toBe('/ui/sign-in');
    expect(screen.queryByText('Payment')).not.toBeInTheDocument();
  });

  it('signs out and clears local context even if the request fails', async () => {
    override = (url) => url.endsWith('/auth/sign-out') ? Response.json({ error: 'offline' }, { status: 503 }) : undefined;
    const user = userEvent.setup();
    render(<App />);
    await signIn(user);
    await user.click(screen.getByRole('button', { name: 'Đăng xuất' }));
    expect(await screen.findByLabelText('Tên đăng nhập')).toBeInTheDocument();
    expect(screen.queryByText('Phiên đăng nhập đã kết thúc. Đăng nhập lại để tiếp tục.')).not.toBeInTheDocument();
    expect(screen.queryByText('Payment')).not.toBeInTheDocument();
    expect(requests.some((request) => request.url.endsWith('/auth/sign-out'))).toBe(true);
  });

  it('keeps existing deployment routes reachable after sign-in', async () => {
    authenticated = true;
    window.history.replaceState({}, '', '/ui/applications/payment/environments/staging/deployments');
    render(<App />);
    await waitFor(() => expect(requests.some((request) => request.url.includes('/deployments'))).toBe(true));
    expect(screen.queryByText('Không tìm thấy ứng dụng')).not.toBeInTheDocument();
  });
  describe.each(['PLATFORM_ENGINEER', 'ADMIN'])('rendering templates page for %s (T03)', (role) => {
    it('shows the peer sidebar link and renders the page at the deep link', async () => {
      authenticated = true;
      window.history.replaceState({}, '', '/ui/platform/rendering-templates');
      override = (url) => url.endsWith('/auth/session') ? Response.json({ user: { Username: 'platform', Role: role } })
        : url.endsWith('/resource-definitions') ? Response.json({ resourceDefinitions: [] }) : undefined;
      render(<App />);
      expect(await screen.findByRole('heading', { name: 'Mẫu dựng ứng dụng' })).toBeInTheDocument();
      const link = screen.getByRole('link', { name: /Mẫu dựng ứng dụng/ });
      expect(link).toHaveAttribute('href', '/ui/platform/rendering-templates');
      expect(link).toHaveClass('nav-item-active');
      expect(screen.getByRole('link', { name: /(^|\s)Cấu hình tài nguyên$/ })).not.toHaveClass('nav-item-active');
    });
  });

  it('hides the rendering templates link and page from a developer deep link', async () => {
    authenticated = true;
    window.history.replaceState({}, '', '/ui/platform/rendering-templates');
    render(<App />);
    await screen.findByRole('navigation', { name: 'Điều hướng chính' });
    expect(screen.queryByRole('link', { name: /Mẫu dựng ứng dụng/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Mẫu dựng ứng dụng' })).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Bundle dựng ứng dụng')).not.toBeInTheDocument();
    expect(requests.some((request) => request.url.endsWith('/resource-definitions'))).toBe(false);
  });
  describe('Resource Types to Definitions navigation (T04)', () => {
    const catalog = (url: string) => url.endsWith('/auth/session') ? Response.json({ user: { Username: 'platform', Role: 'PLATFORM_ENGINEER' } })
      : url.endsWith('/resource-definition-capabilities') ? Response.json({ capabilities: capabilityFixture })
      : url.endsWith('/resource-types') ? Response.json({ resourceTypes: ['vpc', 'postgres', 'postgres-ha', 'workload'].map((key) => ({ key, inputs: [], outputs: [], classes: [] })) })
      : url.endsWith('/resource-definitions') ? Response.json({ resourceDefinitions: [] })
      : url.endsWith('/connections') ? Response.json({ connections: [] }) : undefined;

    it('opens the definitions form with the type chosen from the CTA, without any POST', async () => {
      const user = userEvent.setup();
      authenticated = true;
      window.history.replaceState({}, '', '/ui/platform/resource-types');
      override = catalog;
      render(<App />);
      await user.click(await screen.findByRole('link', { name: 'Tạo Cấu hình tài nguyên cho postgres' }));
      expect(window.location.pathname + window.location.search).toBe('/ui/platform/resource-definitions?resourceType=postgres');
      expect(await screen.findByRole('heading', { name: 'Cấu hình tài nguyên mới' })).toBeInTheDocument();
      await waitFor(() => expect(screen.getByLabelText('Loại tài nguyên')).toHaveValue('postgres'));
      expect(requests.filter((request) => request.method === 'POST')).toEqual([]);
    });

    it.each(['/ui/platform/resource-definitions', '/ui/platform/resource-definitions?resourceType=workload', '/ui/platform/resource-definitions?resourceType=nope'])('selects nothing for deep link %s', async (path) => {
      authenticated = true;
      window.history.replaceState({}, '', path);
      override = catalog;
      render(<App />);
      if (path === '/ui/platform/resource-definitions') await userEvent.setup().click(await screen.findByRole('button', { name: 'Tạo cấu hình' }));
      await screen.findByRole('option', { name: 'vpc' });
      expect(screen.getByLabelText('Loại tài nguyên')).toHaveValue('');
    });

    it('prefills a catalog type awaiting its first Git module and keeps Git registration blocked until inspection', async () => {
      authenticated = true;
      window.history.replaceState({}, '', '/ui/platform/resource-definitions?resourceType=postgres-ha');
      override = catalog;
      render(<App />);
      await waitFor(() => expect(screen.getByLabelText('Loại tài nguyên')).toHaveValue('postgres-ha'));
      expect(screen.getByRole('button', { name: 'Thêm từ Git' })).toBeEnabled();
      await userEvent.setup().click(screen.getByRole('button', { name: 'Thêm từ Git' }));
      expect(screen.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên' })).toBeDisabled();
      expect(requests.filter((request) => request.method === 'POST')).toEqual([]);
    });

    it('selects a supported type from a deep link and keeps the choice when the user navigates away and back without a query', async () => {
      const user = userEvent.setup();
      authenticated = true;
      window.history.replaceState({}, '', '/ui/platform/resource-definitions?resourceType=vpc');
      override = catalog;
      render(<App />);
      await waitFor(() => expect(screen.getByLabelText('Loại tài nguyên')).toHaveValue('vpc'));
      await user.click(screen.getByRole('link', { name: /(^|\s)Loại tài nguyên$/ }));
      expect(await screen.findByRole('heading', { name: 'Loại tài nguyên' })).toBeInTheDocument();
      await user.click(screen.getByRole('link', { name: /(^|\s)Cấu hình tài nguyên$/ }));
      await user.click(await screen.findByRole('button', { name: 'Tạo cấu hình' }));
      await screen.findByRole('option', { name: 'vpc' });
      expect(window.location.search).toBe('');
      expect(screen.getByLabelText('Loại tài nguyên')).toHaveValue('');
    });
  });
  describe('shared shell and session (T10)', () => {
    const asRole = (role: string) => (url: string) => url.endsWith('/auth/session') ? Response.json({ user: { Username: 'tester', Role: role } }) : undefined;

    it('shows the Vietnamese sign-in page with a masked password', async () => {
      render(<App />);
      expect(await screen.findByRole('heading', { name: 'Đăng nhập vào không gian làm việc' })).toBeInTheDocument();
      expect(screen.getByLabelText('Mật khẩu')).toHaveAttribute('type', 'password');
    });

    it('shows the restoring notice, then restores the session without sign-in', async () => {
      authenticated = true;
      let release: () => void = () => undefined;
      override = (url) => url.endsWith('/auth/session') ? new Promise<Response>((resolve) => { release = () => resolve(Response.json({ user: { Username: 'developer', Role: 'DEVELOPER' } })); }) as unknown as Response : undefined;
      window.history.replaceState({}, '', '/ui/applications');
      render(<App />);
      expect(screen.getByRole('status')).toHaveTextContent('Đang tải Orchestrator…');
      release();
      expect(await screen.findByRole('navigation', { name: 'Điều hướng chính' })).toBeInTheDocument();
      expect(screen.queryByLabelText('Mật khẩu')).not.toBeInTheDocument();
    });

    it('shows the invalid-credentials message and keeps the form usable', async () => {
      override = (url) => url.endsWith('/auth/sign-in') ? Response.json({ error: 'invalid', code: 'INVALID_CREDENTIALS' }, { status: 401 }) : undefined;
      const user = userEvent.setup();
      render(<App />);
      await user.type(await screen.findByLabelText('Tên đăng nhập'), 'developer');
      await user.type(screen.getByLabelText('Mật khẩu'), 'wrong-password');
      await user.click(screen.getByRole('button', { name: 'Đăng nhập' }));
      expect(await screen.findByRole('alert')).toHaveTextContent('Tên đăng nhập hoặc mật khẩu không đúng.');
      expect(screen.getByLabelText('Mật khẩu')).toHaveValue('');
    });

    it('shows the expired notice when a later request finds the session gone', async () => {
      authenticated = true;
      const user = userEvent.setup();
      render(<App />);
      await screen.findByText('Payment');
      authenticated = false;
      await user.click(screen.getByText('Payment'));
      expect(await screen.findByRole('status')).toHaveTextContent('Phiên đăng nhập đã kết thúc. Đăng nhập lại để tiếp tục.');
    });

    it('shows a Vietnamese retryable error on an application route', async () => {
      authenticated = true;
      let failures = 1;
      override = (url) => url.endsWith('/applications') && failures-- > 0 ? Response.json({ error: 'database unavailable' }, { status: 500 }) : undefined;
      window.history.replaceState({}, '', '/ui/applications/payment');
      const user = userEvent.setup();
      render(<App />);
      expect(await screen.findByRole('alert')).toHaveTextContent('Không thể tải danh sách ứng dụng.');
      expect(screen.getByRole('alert')).not.toHaveTextContent(/database unavailable/);
      expect(screen.queryByText(/database unavailable/)).not.toBeInTheDocument();
      await user.click(screen.getByRole('button', { name: 'Thử lại' }));
      await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument());
    });

    it('shows a Vietnamese loading state on an application route', async () => {
      authenticated = true;
      override = (url) => url.endsWith('/applications') ? new Promise<Response>(() => undefined) as unknown as Response : undefined;
      window.history.replaceState({}, '', '/ui/applications/payment');
      render(<App />);
      await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Đang tải ứng dụng…'));
    });

    it('shows the Vietnamese missing-application state', async () => {
      authenticated = true;
      window.history.replaceState({}, '', '/ui/applications/missing');
      render(<App />);
      expect(await screen.findByRole('heading', { name: 'Không tìm thấy ứng dụng' })).toBeInTheDocument();
    });

    it('shows only the developer navigation and role label', async () => {
      authenticated = true;
      override = asRole('DEVELOPER');
      render(<App />);
      const nav = await screen.findByRole('navigation', { name: 'Điều hướng chính' });
      expect(nav).toHaveTextContent('Ứng dụng');
      expect(nav).toHaveTextContent('Lịch sử triển khai');
      expect(nav).toHaveTextContent('Sắp có');
      for (const name of [/Loại tài nguyên/, /Cấu hình tài nguyên/, /Mẫu dựng ứng dụng/, /Tài khoản hạ tầng/, /Kết nối/, /Kho bí mật/]) expect(screen.queryByRole('link', { name })).not.toBeInTheDocument();
      expect(screen.getByText('Nhà phát triển')).toBeInTheDocument();
      expect(screen.getByText('tester')).toBeInTheDocument();
    });

    it.each([['PLATFORM_ENGINEER', 'Kỹ sư nền tảng'], ['ADMIN', 'Quản trị viên']])('shows platform links and %s role label', async (role, label) => {
      authenticated = true;
      override = asRole(role);
      render(<App />);
      await screen.findByRole('navigation', { name: 'Điều hướng chính' });
      const expected: [RegExp, string][] = [[/Ứng dụng/, '/ui/applications'], [/Loại tài nguyên/, '/ui/platform/resource-types'], [/Cấu hình tài nguyên/, '/ui/platform/resource-definitions'], [/Mẫu dựng ứng dụng/, '/ui/platform/rendering-templates'], [/Tài khoản hạ tầng/, '/ui/platform/driver-accounts'], [/Kết nối/, '/ui/platform/connections'], [/Kho bí mật/, '/ui/platform/secret-stores']];
      for (const [name, path] of expected) expect(screen.getByRole('link', { name })).toHaveAttribute('href', path);
      expect(screen.getByText(label)).toBeInTheDocument();
    });
  });
});
