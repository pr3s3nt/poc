import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { RenderingTemplatesPage } from './RenderingTemplatesPage';

type Body = Record<string, unknown>;
type Backend = { workloadInputs?: { name: string; type: string; required?: boolean }[]; definitions?: Body[]; post?: () => Response | Promise<Response>; put?: () => Response | Promise<Response>; definitionLists?: (() => Response)[]; bundles?: Body[]; bundleLists?: (() => Response | Promise<Response>)[] };
const nsRef = "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}";
const clusterRef = "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}";
const bundleWeb = { id: 'bundle-web', version: '0.15.0', status: 'AVAILABLE' };
const applications = [{ key: 'shop', name: 'Cửa hàng', environments: [{ key: 'staging', name: 'Staging', environmentType: 'staging' }] }];
const template = { key: 'web-tpl', name: 'Mẫu web', version: 2, resourceType: 'workload', driverType: 'score-k8s', driverInputs: { values: { variables: { render_bundle: 'bundle-web', namespace: nsRef, cluster: clusterRef } } }, criteria: [{ env_type: 'staging' }] };
const other = { key: 'pg-main', name: 'PG chính', version: 1, resourceType: 'postgres', driverType: 'kubernetes', criteria: [{}] };

function backend(options: Backend = {}) {
  const posted: Body[] = [];
  const puts: { url: string; body: Body }[] = [];
  const gets: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'PUT') { const body = JSON.parse(String(init.body)) as Body; puts.push({ url, body }); return (options.put ?? (() => Response.json({ ...body, version: Number(body.version) + 1 })))(); }
    if (init?.method === 'POST') { posted.push(JSON.parse(String(init.body)) as Body); return (options.post ?? (() => Response.json({}, { status: 201 })))(); }
    gets.push(url);
    if (url.endsWith('/applications')) return Response.json({ applications });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [{ key: 'workload', inputs: options.workloadInputs ?? [], outputs: [], classes: [] }] });
    if (url.endsWith('/resource-definition-capabilities')) return Response.json({ capabilities: [{ resourceType: 'workload', driverType: 'score-k8s', module: '', variables: [
      { name: 'render_bundle', type: 'string', required: true, hasDefault: false, resourceParam: false },
      { name: 'namespace', type: 'string', required: true, hasDefault: false, resourceParam: false, preset: nsRef },
      { name: 'cluster', type: 'string', required: true, hasDefault: false, resourceParam: false, preset: clusterRef }], executorOwned: [], account: { required: false }, bindings: [] }] });
    if (url.endsWith('/render-bundles')) { const nextBundles = options.bundleLists?.shift(); return nextBundles ? nextBundles() : Response.json({ renderBundles: options.bundles ?? [bundleWeb] }); }
    const next = options.definitionLists?.shift();
    return next ? next() : Response.json({ resourceDefinitions: options.definitions ?? [] });
  }));
  return { posted, puts, gets };
}

afterEach(() => vi.unstubAllGlobals());

const field = (label: string) => screen.getByLabelText(label);
const submitName = 'Đăng ký mẫu dựng ứng dụng';
const submitButton = () => screen.getByRole('button', { name: submitName });
const bundleSelect = () => screen.getByLabelText('Bundle dựng ứng dụng');
async function fill(user: ReturnType<typeof userEvent.setup>, id = 'web-tpl', bundle = '', name = 'Mẫu web') {
  await user.type(field('ID mẫu'), id);
  if (name) await user.type(field('Tên hiển thị'), name);
  if (bundle) await user.selectOptions(bundleSelect(), bundle);
  await user.click(await screen.findByRole('button', { name: 'Dùng gợi ý namespace' }));
  await user.click(screen.getByRole('button', { name: 'Dùng gợi ý cluster' }));
}

describe('UC-03 rendering templates page (T03)', () => {
  it('lists only workload Definitions with ID, bundle and criteria', async () => {
    backend({ definitions: [other, template] });
    render(<RenderingTemplatesPage />);
    expect(await screen.findByText('Mẫu web')).toBeInTheDocument();
    expect(screen.getByText('Mẫu web', { selector: 'strong' }).nextElementSibling).toHaveTextContent('ID web-tpl · Bundle: bundle-web · phiên bản 2 · 1 điều kiện áp dụng');
    expect(screen.queryByText('PG chính', { selector: 'strong' })).not.toBeInTheDocument();
    expect(screen.queryByText(/internal-k8s|aws-eks|Phạm vi triển khai/)).not.toBeInTheDocument();
  });

  it('shows the empty state and the exactly-one-template/no-fallback guidance in Vietnamese', async () => {
    backend();
    render(<RenderingTemplatesPage />);
    expect(await screen.findByText('Chưa có mẫu dựng ứng dụng nào.')).toBeInTheDocument();
    expect(screen.getByText(/cần đúng một mẫu dựng ứng dụng \(score-k8s\) khớp Điều kiện áp dụng của Môi trường, theo cùng năm field/)).toBeInTheDocument();
    expect(screen.getByText(/không có bộ lọc profile/)).toBeInTheDocument();
    expect(screen.getByText(/hệ thống không dùng renderer mặc định hay renderer khác/)).toBeInTheDocument();
    expect(screen.queryByText(/dùng renderer mặc định\./)).not.toBeInTheDocument();
    expect(screen.getByText(/Một bundle duy nhất được chọn sẵn khi tạo mới; nhiều bundle cần chọn tường minh/)).toBeInTheDocument();
  });

  it('posts only workload/score-k8s with the Name, render_bundle and criteria', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ definitionLists: [() => Response.json({ resourceDefinitions: [] }), () => Response.json({ resourceDefinitions: [template] })] });
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await fill(user);
    await user.click(submitButton());
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký mẫu dựng ứng dụng web-tpl.');
    expect(posted).toEqual([{ key: 'web-tpl', name: 'Mẫu web', resourceType: 'workload', driverType: 'score-k8s', driverInputs: { values: { variables: { render_bundle: 'bundle-web', namespace: nsRef, cluster: clusterRef } } }, criteria: [{}] }]);
    for (const forbidden of ['connectionKey', 'driverAccount', 'executionProfile', 'provision']) expect(posted[0]).not.toHaveProperty(forbidden);
    expect(posted[0]).not.toHaveProperty('driverInputs.values.source');
    expect(await screen.findByText(/Bundle: bundle-web/)).toBeInTheDocument();
    expect(field('ID mẫu')).toHaveValue('');
    expect(field('Tên hiển thị')).toHaveValue('');
    expect(bundleSelect()).toHaveValue('bundle-web');
  });

  it('requires a Name and registers with explicit criteria instead of a deployment scope', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await fill(user, 'web-cloud', '', '');
    await user.click(submitButton());
    expect(await screen.findByRole('alert')).toHaveTextContent('Nhập Tên hiển thị của mẫu.');
    expect(posted).toEqual([]);
    expect(screen.queryByLabelText('Phạm vi triển khai')).not.toBeInTheDocument();
    await user.type(field('Tên hiển thị'), 'Mẫu cloud');
    await user.click(screen.getByLabelText(/Theo ứng dụng/));
    await user.selectOptions(screen.getByLabelText('Ứng dụng'), 'shop');
    await user.click(submitButton());
    expect(await screen.findByRole('status')).toHaveTextContent('web-cloud');
    expect(posted[0]).toMatchObject({ key: 'web-cloud', name: 'Mẫu cloud', driverType: 'score-k8s', resourceType: 'workload', criteria: [{ app_id: 'shop' }] });
    expect(posted[0]).not.toHaveProperty('executionProfile');
  });

  it('shows no infrastructure controls and reuses the shared criteria editor', async () => {
    backend();
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    for (const label of [/Kết nối/, /Terraform/, /module/i, /Loại tài nguyên/, /Cách tạo/]) expect(screen.queryByLabelText(label)).not.toBeInTheDocument();
    expect(screen.getByRole('radiogroup', { name: 'Chế độ điều kiện áp dụng' })).toBeInTheDocument();
    expect(screen.queryByLabelText('Phạm vi triển khai')).not.toBeInTheDocument();
    expect(screen.getByRole('note', { name: 'Quy tắc so khớp' })).toHaveTextContent('AND trong một dòng, OR giữa các dòng');
  });

  it('validates ID shape before any POST', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await user.click(submitButton());
    expect(await screen.findByRole('alert')).toHaveTextContent('Nhập ID mẫu.');
    await user.type(field('ID mẫu'), 'Bad_ID');
    await user.click(submitButton());
    expect(await screen.findByRole('alert')).toHaveTextContent('ID chỉ gồm chữ thường');
    expect(field('ID mẫu')).toHaveValue('Bad_ID');
    await user.clear(field('ID mẫu'));
    await user.clear(field('ID mẫu'));
    await user.type(field('ID mẫu'), 'ok-id');
    await user.type(field('Tên hiển thị'), 'Mẫu ok');
    await user.click(await screen.findByRole('button', { name: 'Dùng gợi ý namespace' }));
    await user.click(screen.getByRole('button', { name: 'Dùng gợi ý cluster' }));
    await user.click(submitButton());
    expect(posted).toHaveLength(1);
  });

  it.each([
    ['unavailable bundle (400)', 400, 'bundle not installed: secret-detail', /bundle có thể không còn khả dụng/],
    ['duplicate (409)', 409, 'duplicate id', /ID mẫu đã tồn tại/],
    ['server error (500)', 500, 'stack trace internals', /Máy chủ gặp lỗi/],
  ])('keeps ID, bundle and criteria on %s with a safe Vietnamese message', async (_name, status, raw, message) => {
    const user = userEvent.setup();
    const { posted } = backend({ post: () => Response.json({ error: raw }, { status }) });
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await fill(user);
    await user.click(screen.getByLabelText(/Theo loại môi trường/));
    await user.type(screen.getByLabelText('Loại môi trường'), 'staging');
    await user.click(submitButton());
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(message);
    expect(alert).not.toHaveTextContent(raw);
    expect(field('ID mẫu')).toHaveValue('web-tpl');
    expect(bundleSelect()).toHaveValue('bundle-web');
    expect(screen.getByLabelText('Loại môi trường')).toHaveValue('staging');
    expect(submitButton()).toBeEnabled();
    expect(screen.queryByText(/Đã đăng ký mẫu/)).not.toBeInTheDocument();
    expect(posted).toHaveLength(1);
  });

  it('locks the form while saving', async () => {
    const user = userEvent.setup();
    let release: (response: Response) => void = () => undefined;
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      if (init?.method === 'POST') return new Promise<Response>((done) => { release = done; });
      const url = String(input);
      if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [{ key: 'workload', inputs: [], outputs: [], classes: [] }] });
    if (url.endsWith('/resource-definition-capabilities')) return Response.json({ capabilities: [{ resourceType: 'workload', driverType: 'score-k8s', module: '', variables: [
      { name: 'render_bundle', type: 'string', required: true, hasDefault: false, resourceParam: false },
      { name: 'namespace', type: 'string', required: true, hasDefault: false, resourceParam: false, preset: nsRef },
      { name: 'cluster', type: 'string', required: true, hasDefault: false, resourceParam: false, preset: clusterRef }], executorOwned: [], account: { required: false }, bindings: [] }] });
    if (url.endsWith('/render-bundles')) return Response.json({ renderBundles: [bundleWeb] });
      return url.endsWith('/applications') ? Response.json({ applications }) : Response.json({ resourceDefinitions: [] });
    }));
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await fill(user);
    await user.click(submitButton());
    expect(field('ID mẫu')).toBeDisabled();
    expect(bundleSelect()).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Đang đăng ký…' })).toBeDisabled();
    release(Response.json({}, { status: 201 }));
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký');
  });

  it('shows a list load failure with Retry, never an empty list', async () => {
    const user = userEvent.setup();
    backend({ definitionLists: [() => Response.json({ error: 'boom' }, { status: 500 }), () => Response.json({ resourceDefinitions: [template] })] });
    render(<RenderingTemplatesPage />);
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải danh sách' });
    expect(screen.queryByText('Chưa có mẫu dựng ứng dụng nào.')).not.toBeInTheDocument();
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    expect(await screen.findByText('Mẫu web')).toBeInTheDocument();
  });

  it('separates a created registration from a reload failure; Retry only reloads the list', async () => {
    const user = userEvent.setup();
    const { posted, gets } = backend({ definitionLists: [() => Response.json({ resourceDefinitions: [] }), () => Response.json({ error: 'boom' }, { status: 500 }), () => Response.json({ resourceDefinitions: [template] })] });
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await fill(user);
    await user.click(submitButton());
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải danh sách' });
    expect(screen.getByRole('status')).toHaveTextContent('Đã đăng ký mẫu dựng ứng dụng web-tpl.');
    expect(field('ID mẫu')).toHaveValue('');
    expect(screen.queryByText('Chưa có mẫu dựng ứng dụng nào.')).not.toBeInTheDocument();
    const before = gets.length;
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    expect(await screen.findByText('Mẫu web')).toBeInTheDocument();
    expect(gets.length).toBeGreaterThan(before);
    expect(posted).toHaveLength(1);
  });

  describe('installed bundle selector (T18)', () => {
    const second = { id: 'bundle-b', version: '1.0.0', status: 'AVAILABLE' };
    it('reads the bundle list from the API and auto-selects a single bundle', async () => {
      const { gets } = backend();
      render(<RenderingTemplatesPage />);
      await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
      expect(gets.some((url) => url.endsWith('/render-bundles'))).toBe(true);
      expect(bundleSelect()).toHaveValue('bundle-web');
      expect(screen.getByRole('option', { name: 'bundle-web (phiên bản 0.15.0)' })).toBeInTheDocument();
      expect(screen.queryByRole('textbox', { name: /bundle/i })).not.toBeInTheDocument();
    });

    it('requires an explicit choice with several bundles and posts the chosen ID', async () => {
      const user = userEvent.setup();
      const { posted } = backend({ bundles: [bundleWeb, second] });
      render(<RenderingTemplatesPage />);
      await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
      expect(bundleSelect()).toHaveValue('');
      await user.type(field('ID mẫu'), 'web-tpl');
      await user.type(field('Tên hiển thị'), 'Mẫu web');
      await user.click(submitButton());
      expect(await screen.findByRole('alert')).toHaveTextContent('Chọn bundle dựng ứng dụng.');
      expect(posted).toHaveLength(0);
      await user.selectOptions(bundleSelect(), 'bundle-b');
      await user.click(await screen.findByRole('button', { name: 'Dùng gợi ý namespace' }));
      await user.click(screen.getByRole('button', { name: 'Dùng gợi ý cluster' }));
      await user.click(submitButton());
      expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký mẫu dựng ứng dụng web-tpl.');
      expect(posted[0]).toMatchObject({ driverInputs: { values: { variables: { render_bundle: 'bundle-b' } } } });
      expect(bundleSelect()).toHaveValue('');
    });

    it('blocks submit and guides in Vietnamese when no bundle is installed', async () => {
      const user = userEvent.setup();
      const { posted } = backend({ bundles: [] });
      render(<RenderingTemplatesPage />);
      const alert = await screen.findByRole('alert', { name: 'Chưa có bundle' });
      expect(alert).toHaveTextContent('Hãy liên hệ Kỹ sư nền tảng');
      expect(bundleSelect()).toBeDisabled();
      expect(submitButton()).toBeDisabled();
      await user.type(field('ID mẫu'), 'web-tpl');
      expect(submitButton()).toBeDisabled();
      expect(posted).toHaveLength(0);
    });

    it('blocks submit while loading, keeps the draft on list error and retries without POST', async () => {
      const user = userEvent.setup();
      let release: (response: Response) => void = () => undefined;
      const { posted } = backend({ bundleLists: [() => new Promise<Response>((done) => { release = done; })] });
      render(<RenderingTemplatesPage />);
      await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
      expect(screen.getByText('Đang tải danh sách bundle đã cài…')).toBeInTheDocument();
      expect(submitButton()).toBeDisabled();
      await user.type(field('ID mẫu'), 'web-tpl');
      release(Response.json({ error: 'boom' }, { status: 500 }));
      const alert = await screen.findByRole('alert', { name: 'Lỗi tải bundle' });
      expect(alert).toHaveTextContent('Không tải được danh sách bundle đã cài.');
      expect(alert).not.toHaveTextContent('boom');
      expect(field('ID mẫu')).toHaveValue('web-tpl');
      expect(submitButton()).toBeDisabled();
      await user.click(within(alert).getByRole('button', { name: 'Thử lại tải bundle' }));
      await waitFor(() => expect(bundleSelect()).toHaveValue('bundle-web'));
      expect(field('ID mẫu')).toHaveValue('web-tpl');
      expect(submitButton()).toBeEnabled();
      expect(posted).toHaveLength(0);
    });

    it('keeps the user choice when a refresh or late reply arrives', async () => {
      const user = userEvent.setup();
      let release: (response: Response) => void = () => undefined;
      backend({ bundles: [bundleWeb, second], bundleLists: [() => Response.json({ renderBundles: [bundleWeb, second] }), () => new Promise<Response>((done) => { release = done; })] });
      render(<RenderingTemplatesPage />);
      await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
      await user.selectOptions(bundleSelect(), 'bundle-b');
      await user.click(screen.getByRole('button', { name: 'Làm mới danh sách bundle' }));
      release(Response.json({ renderBundles: [bundleWeb, second] }));
      await waitFor(() => expect(bundleSelect()).toBeEnabled());
      expect(bundleSelect()).toHaveValue('bundle-b');
    });

    it('does not auto-select over an explicit choice when a refresh returns a single other bundle', async () => {
      const user = userEvent.setup();
      const { posted } = backend({ bundles: [bundleWeb, second], bundleLists: [() => Response.json({ renderBundles: [bundleWeb, second] }), () => Response.json({ renderBundles: [bundleWeb] })] });
      render(<RenderingTemplatesPage />);
      await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
      await user.selectOptions(bundleSelect(), 'bundle-b');
      await user.type(field('ID mẫu'), 'web-tpl');
      await user.type(field('Tên hiển thị'), 'Mẫu web');
      await user.click(screen.getByRole('button', { name: 'Làm mới danh sách bundle' }));
      const alert = await screen.findByRole('alert', { name: 'Bundle không còn khả dụng' });
      expect(alert).toHaveTextContent('bundle-b');
      expect(bundleSelect()).toHaveValue('bundle-b');
      expect(field('ID mẫu')).toHaveValue('web-tpl');
      expect(submitButton()).toBeDisabled();
      expect(posted).toHaveLength(0);
      await user.selectOptions(bundleSelect(), 'bundle-web');
      await user.click(await screen.findByRole('button', { name: 'Dùng gợi ý namespace' }));
      await user.click(screen.getByRole('button', { name: 'Dùng gợi ý cluster' }));
      await user.click(submitButton());
      await waitFor(() => expect(posted).toHaveLength(1));
      expect(posted[0]).toMatchObject({ driverInputs: { values: { variables: { render_bundle: 'bundle-web' } } } });
    });

    it('keeps draft and selection when POST reports the bundle unavailable and does not replay on list retry', async () => {
      const user = userEvent.setup();
      const { posted } = backend({ bundles: [bundleWeb, second], post: () => Response.json({ error: 'render bundle is unavailable' }, { status: 400 }) });
      render(<RenderingTemplatesPage />);
      await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
      await fill(user, 'web-tpl', 'bundle-b');
      await user.click(submitButton());
      const alert = await screen.findByRole('alert');
      expect(alert).toHaveTextContent(/bundle có thể không còn khả dụng/);
      expect(alert).not.toHaveTextContent('render bundle is unavailable');
      expect(field('ID mẫu')).toHaveValue('web-tpl');
      expect(bundleSelect()).toHaveValue('bundle-b');
      await user.click(screen.getByRole('button', { name: 'Làm mới danh sách bundle' }));
      await waitFor(() => expect(bundleSelect()).toBeEnabled());
      expect(posted).toHaveLength(1);
    });

    it('keeps a missing selected option visible and unsubmittable through refresh, error and empty', async () => {
      const user = userEvent.setup();
      let release: (response: Response) => void = () => undefined;
      const { posted } = backend({ bundles: [bundleWeb, second], bundleLists: [
        () => Response.json({ renderBundles: [bundleWeb, second] }),
        () => Response.json({ renderBundles: [bundleWeb] }),
        () => new Promise<Response>((done) => { release = done; }),
        () => Response.json({ error: 'boom' }, { status: 500 }),
        () => Response.json({ renderBundles: [] }),
      ] });
      render(<RenderingTemplatesPage />);
      await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
      await user.selectOptions(bundleSelect(), 'bundle-b');
      await user.type(field('ID mẫu'), 'web-tpl');
      const kept = () => { expect(bundleSelect()).toHaveValue('bundle-b'); expect(screen.getByRole('option', { name: /^bundle-b \(/ })).toBeInTheDocument(); expect(submitButton()).toBeDisabled(); };
      await user.click(screen.getByRole('button', { name: 'Làm mới danh sách bundle' }));
      await screen.findByRole('alert', { name: 'Bundle không còn khả dụng' });
      expect(screen.getByRole('option', { name: 'bundle-b (không còn khả dụng)' })).toBeInTheDocument();
      kept();
      await user.click(screen.getByRole('button', { name: 'Làm mới danh sách bundle' }));
      await screen.findByText('Đang tải danh sách bundle đã cài…');
      expect(screen.getByRole('option', { name: 'bundle-b (chưa xác nhận khả dụng)' })).toBeInTheDocument();
      kept();
      release(Response.json({ renderBundles: [bundleWeb] }));
      await screen.findByRole('alert', { name: 'Bundle không còn khả dụng' });
      await user.click(screen.getByRole('button', { name: 'Làm mới danh sách bundle' }));
      await screen.findByRole('alert', { name: 'Lỗi tải bundle' });
      kept();
      await user.click(screen.getByRole('button', { name: 'Thử lại tải bundle' }));
      await screen.findByRole('alert', { name: 'Chưa có bundle' });
      kept();
      expect(field('ID mẫu')).toHaveValue('web-tpl');
      expect(posted).toHaveLength(0);
    });
  });
});

// UC-03 V02-06 for the rendering template page: the same Name/version contract and one CAS edit.
describe('UC-03 V02 rendering template edit', () => {
  async function open(user: ReturnType<typeof userEvent.setup>, options: Backend = {}) {
    const harness = backend({ definitions: [template], ...options });
    render(<RenderingTemplatesPage />);
    await screen.findByText('Mẫu web');
    await user.click(screen.getByRole('button', { name: 'Sửa mẫu web-tpl' }));
    return harness;
  }
  it('loads the template, keeps the ID fixed and saves one PUT with the expected version', async () => {
    const user = userEvent.setup();
    const { puts, posted } = await open(user);
    expect(field('ID mẫu')).toHaveAttribute('readonly');
    expect(field('ID mẫu')).toHaveValue('web-tpl');
    expect(field('Tên hiển thị')).toHaveValue('Mẫu web');
    expect(bundleSelect()).toHaveValue('bundle-web');
    expect(screen.getByLabelText(/Theo loại môi trường/)).toBeChecked();
    await user.clear(field('Tên hiển thị'));
    await user.type(field('Tên hiển thị'), 'Mẫu web mới');
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Đã lưu mẫu dựng ứng dụng web-tpl (phiên bản 3).');
    expect(posted).toEqual([]);
    expect(puts[0]?.url).toMatch(/\/resource-definitions\/web-tpl$/);
    expect(puts[0]?.body).toEqual({ key: 'web-tpl', name: 'Mẫu web mới', resourceType: 'workload', driverType: 'score-k8s', driverInputs: { values: { variables: { render_bundle: 'bundle-web', namespace: nsRef, cluster: clusterRef } } }, criteria: [{ env_type: 'staging' }], version: 2 });
  });
  it('keeps the draft on a stale 409 and needs an explicit reload before saving again', async () => {
    const user = userEvent.setup();
    const { puts } = await open(user, { put: () => Response.json({ code: 'STALE_VERSION' }, { status: 409 }) });
    await user.clear(field('Tên hiển thị'));
    await user.type(field('Tên hiển thị'), 'Bản nháp');
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    expect(await screen.findByRole('alert', { name: 'Cần tải lại mẫu' })).toHaveTextContent(/Chưa có gì bị ghi đè/);
    expect(field('Tên hiển thị')).toHaveValue('Bản nháp');
    expect(screen.getByRole('button', { name: 'Lưu thay đổi' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Tải lại bản mới nhất' }));
    expect(field('Tên hiển thị')).toHaveValue('Mẫu web');
    expect(screen.getByRole('button', { name: 'Lưu thay đổi' })).toBeEnabled();
    expect(puts).toHaveLength(1);
  });
});

// UC-03 V02-06: an unknown CREATE outcome holds the registration like an unknown edit holds Save.
describe('UC-03 V02 rendering template create with an unknown outcome', () => {
  async function submitCreate(user: ReturnType<typeof userEvent.setup>, options: Backend) {
    const harness = backend(options);
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await waitFor(() => expect(bundleSelect()).toBeEnabled());
    await fill(user, 'web-tpl', 'bundle-web');
    await user.click(submitButton());
    return harness;
  }
  it('holds the registration after a network failure, never replays the POST and opens the stored template when the key exists', async () => {
    const user = userEvent.setup();
    const definitionLists = [() => Response.json({ resourceDefinitions: [] }), () => Response.json({ resourceDefinitions: [template] }), () => Response.json({ resourceDefinitions: [template] })];
    const { posted } = await submitCreate(user, { post: () => { throw new TypeError('network'); }, definitionLists });
    const review = await screen.findByRole('alert', { name: 'Cần tải lại mẫu' });
    expect(review).toHaveTextContent(/Chưa biết lần đăng ký đã được lưu hay chưa/);
    expect(submitButton()).toBeDisabled();
    expect(posted).toHaveLength(1);
    await user.click(within(review).getByRole('button', { name: 'Kiểm tra trên máy chủ' }));
    expect(await screen.findByRole('heading', { name: 'Sửa mẫu dựng ứng dụng web-tpl' })).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent(/đã có trên máy chủ/);
    expect(posted).toHaveLength(1);
  });
  it('keeps draft and hold when the check fails, then releases the hold with the draft when the create rolled back', async () => {
    const user = userEvent.setup();
    const definitionLists = [() => Response.json({ resourceDefinitions: [] }), () => Response.json({ resourceDefinitions: [] }), () => Response.json({ error: 'x' }, { status: 500 }), () => Response.json({ resourceDefinitions: [] }), () => Response.json({ resourceDefinitions: [] })];
    const { posted } = await submitCreate(user, { post: () => Response.json({ code: 'COMMIT_UNCERTAIN' }, { status: 503 }), definitionLists });
    const review = await screen.findByRole('alert', { name: 'Cần tải lại mẫu' });
    await user.click(within(review).getByRole('button', { name: 'Kiểm tra trên máy chủ' }));
    await waitFor(() => expect(screen.getByRole('alert', { name: 'Cần tải lại mẫu' })).toBeInTheDocument());
    expect(submitButton()).toBeDisabled();
    expect(field('ID mẫu')).toHaveValue('web-tpl');
    await user.click(within(screen.getByRole('alert', { name: 'Cần tải lại mẫu' })).getByRole('button', { name: 'Kiểm tra trên máy chủ' }));
    await waitFor(() => expect(screen.queryByRole('alert', { name: 'Cần tải lại mẫu' })).not.toBeInTheDocument());
    expect(screen.getByRole('status')).toHaveTextContent(/lần đăng ký trước không được lưu/);
    expect(field('ID mẫu')).toHaveValue('web-tpl');
    expect(bundleSelect()).toHaveValue('bundle-web');
    expect(submitButton()).toBeEnabled();
    expect(posted).toHaveLength(1);
  });
});

describe('ADR-016 workload placement and companions', () => {
  const jsonField = () => screen.getByLabelText('JSON nâng cao của mẫu dựng ứng dụng') as HTMLTextAreaElement;

  it('sends the typed namespace/cluster references and a companion rule exactly, with no other variable', async () => {
    const user = userEvent.setup();
    // A workload Type that declares inputs adds no control: the driver would not consume them.
    const { posted } = backend({ workloadInputs: [{ name: 'backend_url', type: 'string' }] });
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await fill(user);
    expect(screen.queryByLabelText('Đầu vào backend_url')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên đi kèm' }));
    await user.type(screen.getByLabelText('Descriptor tài nguyên đi kèm 1'), 'k8s-namespace.default#environments.@app.@env-extra');
    await user.click(submitButton());
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký mẫu dựng ứng dụng web-tpl.');
    expect(posted[0]).toEqual({
      key: 'web-tpl', name: 'Mẫu web', resourceType: 'workload', driverType: 'score-k8s', criteria: [{}],
      driverInputs: { values: { variables: { render_bundle: 'bundle-web', namespace: nsRef, cluster: clusterRef } } },
      provision: { 'k8s-namespace.default#environments.@app.@env-extra': {} },
    });
    expect(posted[0]).not.toHaveProperty('driverAccount');
  });

  it('refuses a half pair of references and sends the render-only AWS legacy form only when both are empty', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await fill(user);
    await user.clear(screen.getByLabelText('Tham chiếu cluster'));
    await user.click(submitButton());
    expect(await screen.findByRole('alert')).toHaveTextContent('Nhập đủ tham chiếu namespace và cluster');
    expect(posted).toEqual([]);
    await user.clear(screen.getByLabelText('Tham chiếu namespace'));
    await user.click(submitButton());
    await screen.findByRole('status');
    expect(posted[0]).toMatchObject({ driverInputs: { values: { variables: { render_bundle: 'bundle-web' } } } });
    expect((posted[0]?.driverInputs as { values: { variables: Record<string, unknown> } }).values.variables).not.toHaveProperty('namespace');
  });

  it('keeps form and advanced JSON as one lossless document', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await fill(user);
    await user.click(screen.getByRole('button', { name: 'JSON nâng cao' }));
    expect(JSON.parse(jsonField().value)).toEqual({ variables: { render_bundle: 'bundle-web', namespace: nsRef, cluster: clusterRef } });
    await user.click(screen.getByRole('button', { name: 'Biểu mẫu' }));
    expect(screen.getByLabelText('Tham chiếu cluster')).toHaveValue(clusterRef);
    // A key the form cannot show stays authoritative in JSON; nothing is dropped.
    await user.click(screen.getByRole('button', { name: 'JSON nâng cao' }));
    await user.clear(jsonField());
    await user.click(jsonField());
    await user.paste(JSON.stringify({ variables: { render_bundle: 'bundle-web', namespace: nsRef, cluster: clusterRef, unknown_key: 'x' } }));
    await user.click(screen.getByRole('button', { name: 'Biểu mẫu' }));
    expect(screen.getByText(/không được trình điều khiển workload hỗ trợ/)).toBeInTheDocument();
    expect(jsonField()).toBeInTheDocument();
    await user.click(submitButton());
    expect(posted[0]).toMatchObject({ driverInputs: { values: { variables: { unknown_key: 'x' } } } });
  });

  it('opens a stored template with a companion rule in the form and saves it unchanged', async () => {
    const user = userEvent.setup();
    const stored = { ...template, provision: { 'k8s-namespace.default#environments.@app.@env-extra': { match_dependents: true } } };
    const { puts } = backend({ definitions: [stored] });
    render(<RenderingTemplatesPage />);
    await screen.findByText('Mẫu web');
    await user.click(screen.getByRole('button', { name: 'Sửa mẫu web-tpl' }));
    expect(screen.getByLabelText('Descriptor tài nguyên đi kèm 1')).toHaveValue('k8s-namespace.default#environments.@app.@env-extra');
    expect(screen.getByLabelText(/match_dependents/)).toBeChecked();
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    await screen.findByRole('status');
    expect(puts[0]?.body).toMatchObject({ provision: { 'k8s-namespace.default#environments.@app.@env-extra': { match_dependents: true } }, driverInputs: { values: { variables: { namespace: nsRef, cluster: clusterRef } } } });
  });

  it('keeps all four flag combinations of the workload companion rules lossless through the form (BR-08)', async () => {
    const user = userEvent.setup();
    const rules = {
      'k8s-namespace.default#environments.@app.@env-a': {},
      'k8s-namespace.default#environments.@app.@env-b': { is_dependent: true },
      'k8s-namespace.default#environments.@app.@env-c': { match_dependents: true },
      'k8s-namespace.default#environments.@app.@env-d': { is_dependent: true, match_dependents: true },
    };
    const { puts } = backend({ definitions: [{ ...template, provision: rules }] });
    render(<RenderingTemplatesPage />);
    await screen.findByText('Mẫu web');
    await user.click(screen.getByRole('button', { name: 'Sửa mẫu web-tpl' }));
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    await screen.findByRole('status');
    // No flag is added, dropped or flipped: false/absent stays absent, true stays true, a rule with both keeps both.
    expect((puts[0]?.body as { provision: unknown }).provision).toEqual(rules);
  });

  it('opens a stored template the form cannot represent in advanced JSON and preserves it', async () => {
    const user = userEvent.setup();
    const stored = { ...template, driverInputs: { values: { variables: { render_bundle: 'bundle-web', namespace: nsRef, cluster: clusterRef, legacy_extra: { nested: true } } } } };
    const { puts } = backend({ definitions: [stored] });
    render(<RenderingTemplatesPage />);
    await screen.findByText('Mẫu web');
    await user.click(screen.getByRole('button', { name: 'Sửa mẫu web-tpl' }));
    expect(JSON.parse(jsonField().value).variables.legacy_extra).toEqual({ nested: true });
    expect(screen.getByRole('status')).toHaveTextContent('Mẫu được mở ở JSON nâng cao');
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    await screen.findAllByRole('status');
    expect((puts[0]?.body.driverInputs as { values: { variables: Record<string, unknown> } }).values.variables.legacy_extra).toEqual({ nested: true });
  });
});
