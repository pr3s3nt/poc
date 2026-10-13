import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { RenderingTemplatesPage } from './RenderingTemplatesPage';

type Body = Record<string, unknown>;
type Backend = { definitions?: Body[]; post?: () => Response; definitionLists?: (() => Response)[]; bundles?: Body[]; bundleLists?: (() => Response | Promise<Response>)[] };
const bundleWeb = { id: 'bundle-web', version: '0.15.0', status: 'AVAILABLE' };
const applications = [{ key: 'shop', name: 'Cửa hàng', environments: [{ key: 'staging', name: 'Staging', environmentType: 'staging' }] }];
const template = { key: 'web-tpl', resourceType: 'workload', driverType: 'score-k8s', executionProfile: 'internal-k8s', driverInputs: { values: { variables: { render_bundle: 'bundle-web' } } }, criteria: [{ env_type: 'staging' }] };
const other = { key: 'pg-main', resourceType: 'postgres', driverType: 'kubernetes', executionProfile: 'internal-k8s', criteria: [{}] };

function backend(options: Backend = {}) {
  const posted: Body[] = [];
  const gets: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'POST') { posted.push(JSON.parse(String(init.body)) as Body); return (options.post ?? (() => Response.json({}, { status: 201 })))(); }
    gets.push(url);
    if (url.endsWith('/applications')) return Response.json({ applications });
    if (url.endsWith('/render-bundles')) { const nextBundles = options.bundleLists?.shift(); return nextBundles ? nextBundles() : Response.json({ renderBundles: options.bundles ?? [bundleWeb] }); }
    const next = options.definitionLists?.shift();
    return next ? next() : Response.json({ resourceDefinitions: options.definitions ?? [] });
  }));
  return { posted, gets };
}

afterEach(() => vi.unstubAllGlobals());

const field = (label: string) => screen.getByLabelText(label);
const submitName = 'Đăng ký mẫu dựng ứng dụng';
const submitButton = () => screen.getByRole('button', { name: submitName });
const bundleSelect = () => screen.getByLabelText('Bundle dựng ứng dụng');
async function fill(user: ReturnType<typeof userEvent.setup>, id = 'web-tpl', bundle = '', profile = 'internal-k8s') {
  await user.type(field('ID mẫu'), id);
  if (profile) await user.selectOptions(field('Phạm vi triển khai'), profile);
  if (bundle) await user.selectOptions(bundleSelect(), bundle);
}

describe('UC-03 rendering templates page (T03)', () => {
  it('lists only workload Definitions with ID, bundle and criteria', async () => {
    backend({ definitions: [other, template] });
    render(<RenderingTemplatesPage />);
    expect(await screen.findByText('web-tpl')).toBeInTheDocument();
    expect(screen.getByText(/Cluster nội bộ \(internal-k8s\) · Bundle: bundle-web · 1 điều kiện áp dụng/)).toBeInTheDocument();
    expect(screen.queryByText('pg-main', { selector: 'strong' })).not.toBeInTheDocument();
  });

  it('shows the empty state and the required-on-both-profiles/no-fallback guidance in Vietnamese', async () => {
    backend();
    render(<RenderingTemplatesPage />);
    expect(await screen.findByText('Chưa có mẫu dựng ứng dụng nào.')).toBeInTheDocument();
    expect(screen.getByText(/ở cả cluster nội bộ và AWS, cần đúng một mẫu dựng ứng dụng/)).toBeInTheDocument();
    expect(screen.getByText(/hệ thống không dùng renderer mặc định hay renderer khác/)).toBeInTheDocument();
    expect(screen.queryByText(/dùng renderer mặc định\./)).not.toBeInTheDocument();
    expect(screen.getByText(/Một bundle duy nhất được chọn sẵn; nhiều bundle cần chọn tường minh/)).toBeInTheDocument();
  });

  it('posts only workload/score-k8s with the chosen profile, render_bundle and criteria', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ definitionLists: [() => Response.json({ resourceDefinitions: [] }), () => Response.json({ resourceDefinitions: [template] })] });
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await fill(user);
    await user.click(submitButton());
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký mẫu dựng ứng dụng web-tpl.');
    expect(posted).toEqual([{ key: 'web-tpl', resourceType: 'workload', executionProfile: 'internal-k8s', driverType: 'score-k8s', driverInputs: { values: { variables: { render_bundle: 'bundle-web' } } }, criteria: [{}] }]);
    for (const forbidden of ['connectionKey', 'provision']) expect(posted[0]).not.toHaveProperty(forbidden);
    expect(posted[0]).not.toHaveProperty('driverInputs.values.source');
    expect(await screen.findByText(/Bundle: bundle-web/)).toBeInTheDocument();
    expect(field('ID mẫu')).toHaveValue('');
    expect(field('Phạm vi triển khai')).toHaveValue('');
    expect(bundleSelect()).toHaveValue('bundle-web');
  });

  it('registers the same template for the AWS profile and never defaults the scope', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await fill(user, 'web-cloud', '', '');
    await user.click(submitButton());
    expect(await screen.findByRole('alert')).toHaveTextContent('Chọn phạm vi triển khai của mẫu');
    expect(posted).toEqual([]);
    expect(field('Phạm vi triển khai')).toHaveValue('');
    await user.selectOptions(field('Phạm vi triển khai'), 'aws-eks');
    expect(screen.getByText(/Phạm vi triển khai \(executionProfile\):/)).toHaveTextContent('AWS (aws-eks)');
    await user.click(submitButton());
    expect(await screen.findByRole('status')).toHaveTextContent('web-cloud');
    expect(posted[0]).toMatchObject({ key: 'web-cloud', executionProfile: 'aws-eks', driverType: 'score-k8s', resourceType: 'workload' });
  });

  it('shows no infrastructure controls and reuses the shared criteria editor', async () => {
    backend();
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    for (const label of [/Kết nối/, /Terraform/, /module/i, /JSON/, /Quy tắc tạo tài nguyên/, /Loại tài nguyên/, /Cách tạo/]) expect(screen.queryByLabelText(label)).not.toBeInTheDocument();
    expect(screen.getByRole('radiogroup', { name: 'Chế độ điều kiện áp dụng' })).toBeInTheDocument();
    await userEvent.setup().selectOptions(field('Phạm vi triển khai'), 'internal-k8s');
    expect(screen.getByText(/Phạm vi triển khai \(executionProfile\):/)).toHaveTextContent('Cluster nội bộ');
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
    await user.type(field('ID mẫu'), 'ok-id');
    await user.selectOptions(field('Phạm vi triển khai'), 'internal-k8s');
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
    expect(await screen.findByText('web-tpl')).toBeInTheDocument();
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
    expect(await screen.findByText('web-tpl')).toBeInTheDocument();
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
      await user.selectOptions(field('Phạm vi triển khai'), 'internal-k8s');
      await user.click(submitButton());
      expect(await screen.findByRole('alert')).toHaveTextContent('Chọn bundle dựng ứng dụng.');
      expect(posted).toHaveLength(0);
      await user.selectOptions(bundleSelect(), 'bundle-b');
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
      await user.selectOptions(field('Phạm vi triển khai'), 'aws-eks');
      await user.click(screen.getByRole('button', { name: 'Làm mới danh sách bundle' }));
      const alert = await screen.findByRole('alert', { name: 'Bundle không còn khả dụng' });
      expect(alert).toHaveTextContent('bundle-b');
      expect(bundleSelect()).toHaveValue('bundle-b');
      expect(field('ID mẫu')).toHaveValue('web-tpl');
      expect(submitButton()).toBeDisabled();
      expect(posted).toHaveLength(0);
      await user.selectOptions(bundleSelect(), 'bundle-web');
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
