import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { RenderingTemplatesPage } from './RenderingTemplatesPage';

type Body = Record<string, unknown>;
type Backend = { definitions?: Body[]; post?: () => Response; definitionLists?: (() => Response)[] };
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
    const next = options.definitionLists?.shift();
    return next ? next() : Response.json({ resourceDefinitions: options.definitions ?? [] });
  }));
  return { posted, gets };
}

afterEach(() => vi.unstubAllGlobals());

const field = (label: string) => screen.getByLabelText(label);
const submitName = 'Đăng ký mẫu dựng ứng dụng';
const submitButton = () => screen.getByRole('button', { name: submitName });
async function fill(user: ReturnType<typeof userEvent.setup>, id = 'web-tpl', bundle = 'bundle-web') {
  await user.type(field('ID mẫu'), id);
  if (bundle) await user.type(field('ID bundle dựng ứng dụng'), bundle);
}

describe('UC-03 rendering templates page (T03)', () => {
  it('lists only workload Definitions with ID, bundle and criteria', async () => {
    backend({ definitions: [other, template] });
    render(<RenderingTemplatesPage />);
    expect(await screen.findByText('web-tpl')).toBeInTheDocument();
    expect(screen.getByText(/Bundle: bundle-web · 1 điều kiện áp dụng/)).toBeInTheDocument();
    expect(screen.queryByText('pg-main', { selector: 'strong' })).not.toBeInTheDocument();
  });

  it('shows the empty state and the optional/no-fallback guidance in Vietnamese', async () => {
    backend();
    render(<RenderingTemplatesPage />);
    expect(await screen.findByText('Chưa có mẫu dựng ứng dụng nào.')).toBeInTheDocument();
    expect(screen.getByText(/Mẫu là tùy chọn và chỉ dùng cho cluster nội bộ/)).toBeInTheDocument();
    expect(screen.getByText(/không chuyển âm thầm sang renderer mặc định/)).toBeInTheDocument();
    expect(screen.getByText(/chưa phải bộ chọn từ danh sách bundle đã cài/)).toBeInTheDocument();
  });

  it('posts only workload/score-k8s/internal-k8s with render_bundle and criteria', async () => {
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
    expect(field('ID bundle dựng ứng dụng')).toHaveValue('');
  });

  it('shows no infrastructure controls and reuses the shared criteria editor', async () => {
    backend();
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    for (const label of [/Kết nối/, /Terraform/, /module/i, /JSON/, /Quy tắc tạo tài nguyên/, /Loại tài nguyên/, /Cách tạo/]) expect(screen.queryByLabelText(label)).not.toBeInTheDocument();
    expect(screen.getByRole('radiogroup', { name: 'Chế độ điều kiện áp dụng' })).toBeInTheDocument();
    expect(screen.getByText(/Phạm vi triển khai \(executionProfile\):/)).toHaveTextContent('Cluster nội bộ');
  });

  it('validates ID shape and required bundle before any POST', async () => {
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
    await user.click(submitButton());
    expect(await screen.findByRole('alert')).toHaveTextContent('Nhập ID bundle dựng ứng dụng.');
    expect(posted).toHaveLength(0);
  });

  it.each([
    ['unavailable bundle (400)', 400, 'bundle not installed: secret-detail', /bundle có thể chưa được cài/],
    ['duplicate (409)', 409, 'duplicate id', /ID mẫu đã tồn tại/],
    ['server error (500)', 500, 'stack trace internals', /Máy chủ gặp lỗi/],
  ])('keeps ID, bundle and criteria on %s with a safe Vietnamese message', async (_name, status, raw, message) => {
    const user = userEvent.setup();
    const { posted } = backend({ post: () => Response.json({ error: raw }, { status }) });
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await fill(user, 'web-tpl', 'missing-bundle');
    await user.click(screen.getByLabelText(/Theo loại môi trường/));
    await user.type(screen.getByLabelText('Loại môi trường'), 'staging');
    await user.click(submitButton());
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(message);
    expect(alert).not.toHaveTextContent(raw);
    expect(field('ID mẫu')).toHaveValue('web-tpl');
    expect(field('ID bundle dựng ứng dụng')).toHaveValue('missing-bundle');
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
      return String(input).endsWith('/applications') ? Response.json({ applications }) : Response.json({ resourceDefinitions: [] });
    }));
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await fill(user);
    await user.click(submitButton());
    expect(field('ID mẫu')).toBeDisabled();
    expect(field('ID bundle dựng ứng dụng')).toBeDisabled();
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
});
