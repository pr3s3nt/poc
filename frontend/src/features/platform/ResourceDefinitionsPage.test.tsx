import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';

type Body = Record<string, unknown>;
type Backend = {
  types?: string[]; definitions?: Body[]; connections?: Body[]; post?: () => Response;
  typesFail?: boolean; connectionsFail?: boolean; definitionLists?: (() => Response)[];
};
const defaultConnections = [
  { key: 'aws-ready', name: 'AWS chính', kind: 'AWS', status: 'READY' },
  { key: 'aws-pending', kind: 'AWS', status: 'VERIFYING' },
  { key: 'kube-ready', name: 'Cluster dev', kind: 'KUBERNETES', status: 'READY' },
  { key: 'kube-pending', kind: 'KUBERNETES', status: 'FAILED' },
];
const allTypes = ['vpc', 'k8s-cluster', 'k8s-namespace', 'postgres', 'workload', 'cache'];

// Serves the three GET lists and records every request.
function backend(options: Backend = {}) {
  const posted: Body[] = [];
  const gets: string[] = [];
  const state = { ...options };
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'POST') {
      posted.push(JSON.parse(String(init.body)) as Body);
      return (state.post ?? (() => Response.json({}, { status: 201 })))();
    }
    gets.push(url);
    if (url.endsWith('/resource-types')) return state.typesFail ? Response.json({ error: 'boom' }, { status: 500 }) : Response.json({ resourceTypes: (state.types ?? allTypes).map((key) => ({ key })) });
    if (url.endsWith('/connections')) return state.connectionsFail ? Response.json({ error: 'boom' }, { status: 500 }) : Response.json({ connections: state.connections ?? defaultConnections });
    const next = state.definitionLists?.shift();
    return next ? next() : Response.json({ resourceDefinitions: state.definitions ?? [] });
  }));
  return { posted, gets, state };
}

const submitName = 'Đăng ký cấu hình tài nguyên';
const submit = (user: ReturnType<typeof userEvent.setup>) => user.click(screen.getByRole('button', { name: submitName }));
async function ready() {
  render(<ResourceDefinitionsPage />);
  await screen.findByText('Chưa có cấu hình tài nguyên nào.');
  await screen.findByRole('option', { name: 'vpc' });
}
const field = (name: string) => screen.getByLabelText(name);

afterEach(() => vi.unstubAllGlobals());

describe('UC-03 definition registration form', () => {
  it('fills the driver, module and locked profile from the Resource Type and posts the Terraform contract', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    expect(field('Cách tạo')).toBeDisabled();
    await user.type(field('ID cấu hình'), 'vpc-aws');
    await user.selectOptions(field('Loại tài nguyên'), 'vpc');
    expect(field('Cách tạo')).toHaveValue('terraform');
    expect(field('Cách tạo')).toBeDisabled();
    expect(field('Phạm vi triển khai')).toHaveValue('aws-eks');
    expect(field('Phạm vi triển khai')).toBeDisabled();
    expect(screen.getByText('vpc', { selector: 'code' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Hiện nâng cao' }));
    fireEvent.change(field('Tham số cấu hình (JSON object)'), { target: { value: '{"cidr":"10.0.0.0/16"}' } });
    await submit(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('Chọn một Kết nối AWS sẵn sàng');
    expect(posted).toHaveLength(0);
    await user.selectOptions(field('Kết nối AWS'), 'aws-ready');
    await submit(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên vpc-aws.');
    expect(posted).toEqual([{ key: 'vpc-aws', resourceType: 'vpc', executionProfile: 'aws-eks', driverType: 'terraform', connectionKey: 'aws-ready', driverInputs: { values: { variables: { cidr: '10.0.0.0/16' }, source: { module: 'vpc' } } }, provision: {}, criteria: [{}] }]);
  });

  it('maps k8s-cluster to the eks module and namespace to Kubernetes', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình'), 'cluster-eks');
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-cluster');
    expect(field('Cách tạo')).toHaveValue('terraform');
    expect(screen.getByText('eks', { selector: 'code' })).toBeInTheDocument();
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-namespace');
    expect(field('Cách tạo')).toHaveValue('kubernetes');
    expect(field('Cách tạo')).toBeDisabled();
    expect(screen.queryByLabelText('Kết nối AWS')).not.toBeInTheDocument();
    await submit(user);
    await screen.findByRole('status');
    expect(posted[0]).toMatchObject({ resourceType: 'k8s-namespace', driverType: 'kubernetes', executionProfile: 'internal-k8s', connectionKey: '', driverInputs: { values: { variables: {} } } });
    expect(posted[0]?.driverInputs).not.toHaveProperty('values.source');
  });

  it('lets the user choose the Postgres driver and keeps JSON, criteria and per-driver choices across switches', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình'), 'pg-main');
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    expect(field('Cách tạo')).toBeEnabled();
    expect(field('Cách tạo')).toHaveValue('kubernetes');
    await user.click(screen.getByRole('button', { name: 'Hiện nâng cao' }));
    fireEvent.change(field('Tham số cấu hình (JSON object)'), { target: { value: '{"storage":"2Gi"}' } });
    fireEvent.change(field('Quy tắc tạo tài nguyên liên quan (JSON object)'), { target: { value: '{"keep":true}' } });
    await user.type(field('Điều kiện 1 Class'), 'fast');
    await user.selectOptions(field('Kết nối riêng cho cluster (tùy chọn)'), 'kube-ready');
    await user.selectOptions(field('Phạm vi triển khai'), '');
    await user.selectOptions(field('Cách tạo'), 'terraform');
    expect(field('Phạm vi triển khai')).toHaveValue('aws-eks');
    expect(screen.getByText('aurora', { selector: 'code' })).toBeInTheDocument();
    expect(screen.queryByLabelText('Kết nối riêng cho cluster (tùy chọn)')).not.toBeInTheDocument();
    await user.selectOptions(field('Kết nối AWS'), 'aws-ready');
    await user.selectOptions(field('Cách tạo'), 'kubernetes');
    expect(field('Tham số cấu hình (JSON object)')).toHaveValue('{"storage":"2Gi"}');
    expect(field('Quy tắc tạo tài nguyên liên quan (JSON object)')).toHaveValue('{"keep":true}');
    expect(field('Điều kiện 1 Class')).toHaveValue('fast');
    expect(field('Kết nối riêng cho cluster (tùy chọn)')).toHaveValue('kube-ready');
    expect(field('Phạm vi triển khai')).toHaveValue('');
    // Collapsing the advanced section never clears data either.
    await user.click(screen.getByRole('button', { name: 'Ẩn nâng cao' }));
    expect(field('Tham số cấu hình (JSON object)')).toHaveValue('{"storage":"2Gi"}');
    await submit(user);
    await screen.findByRole('status');
    // Only the effective driver's connection/source is sent, never the stale AWS choice.
    expect(posted[0]).toEqual({ key: 'pg-main', resourceType: 'postgres', executionProfile: '', driverType: 'kubernetes', connectionKey: 'kube-ready', driverInputs: { values: { variables: { storage: '2Gi' } } }, provision: { keep: true }, criteria: [{ class: 'fast' }] });
  });

  it('sends an empty profile for Dùng chung and warns about overlap', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình'), 'ns-shared');
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-namespace');
    expect(screen.queryByRole('note')).not.toBeInTheDocument();
    await user.selectOptions(field('Phạm vi triển khai'), '');
    expect(screen.getByRole('note')).toHaveTextContent(/mọi phạm vi triển khai.*trùng mức ưu tiên/);
    await submit(user);
    await screen.findByRole('status');
    expect(posted[0]).toMatchObject({ executionProfile: '', connectionKey: '' });
  });

  it('lists only READY Connections of the matching kind and keeps the Kubernetes override advanced and empty by default', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.selectOptions(field('Loại tài nguyên'), 'vpc');
    expect(within(field('Kết nối AWS')).getAllByRole('option').map((option) => option.getAttribute('value'))).toEqual(['', 'aws-ready']);
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-namespace');
    const override = field('Kết nối riêng cho cluster (tùy chọn)');
    expect(override).not.toBeVisible();
    expect(within(override).getAllByRole('option', { hidden: true }).map((option) => option.getAttribute('value'))).toEqual(['', 'kube-ready']);
    expect(override).toHaveValue('');
    await user.type(field('ID cấu hình'), 'ns-default');
    await submit(user);
    await screen.findByRole('status');
    expect(posted[0]?.connectionKey).toBe('');
    await user.click(screen.getByRole('button', { name: 'Hiện nâng cao' }));
    await user.selectOptions(override, 'kube-ready');
    await user.type(field('ID cấu hình'), 'ns-explicit');
    await submit(user);
    await waitFor(() => expect(posted).toHaveLength(2));
    expect(posted[1]?.connectionKey).toBe('kube-ready');
  });

  it('explains when no READY AWS Connection exists and refuses to submit Terraform', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ connections: [{ key: 'aws-pending', kind: 'AWS', status: 'VERIFYING' }] });
    await ready();
    await user.type(field('ID cấu hình'), 'vpc-none');
    await user.selectOptions(field('Loại tài nguyên'), 'vpc');
    await waitFor(() => expect(screen.getByText(/Hiện chưa có Kết nối AWS sẵn sàng/)).toBeInTheDocument());
    await submit(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('Chọn một Kết nối AWS sẵn sàng');
    expect(posted).toHaveLength(0);
  });

  it('hides workload and shows unsupported types as disabled, never registrable', async () => {
    const user = userEvent.setup();
    backend();
    await ready();
    const select = field('Loại tài nguyên');
    expect(within(select).queryByRole('option', { name: 'workload' })).not.toBeInTheDocument();
    const unsupported = within(select).getByRole('option', { name: 'cache (chưa hỗ trợ đăng ký)' });
    expect(unsupported).toBeDisabled();
    await user.selectOptions(select, 'cache').catch(() => undefined);
    expect(select).toHaveValue('');
    const drivers = within(field('Cách tạo')).getAllByRole('option').map((option) => option.textContent);
    expect(drivers.join()).not.toMatch(/existing-cluster|score-k8s/);
    // The renderer registration form moved out of this page (T03).
    expect(screen.queryByLabelText(/Rendering bundle|Bộ dựng|render_bundle/)).not.toBeInTheDocument();
    expect(screen.queryByRole('option', { name: /score-k8s/ })).not.toBeInTheDocument();
  });

  it('lists definitions in Vietnamese, hides workload ones and marks existing-cluster as system read-only', async () => {
    backend({ definitions: [
      { key: 'pg-fast', resourceType: 'postgres', executionProfile: 'internal-k8s', driverType: 'kubernetes', criteria: [{}] },
      { key: 'workload-score', resourceType: 'workload', executionProfile: 'internal-k8s', driverType: 'score-k8s', criteria: [{}] },
      { key: 'cluster-existing', resourceType: 'k8s-cluster', executionProfile: 'internal-k8s', driverType: 'existing-cluster', criteria: [{}, {}] },
      { key: 'ns-shared', resourceType: 'k8s-namespace', executionProfile: '', driverType: 'kubernetes', criteria: [{}] },
    ] });
    render(<ResourceDefinitionsPage />);
    expect(await screen.findByText('pg-fast')).toBeInTheDocument();
    expect(screen.queryByText('workload-score')).not.toBeInTheDocument();
    expect(screen.getByText('pg-fast').nextElementSibling).toHaveTextContent('postgres · Cluster nội bộ · Tạo trong cluster (Kubernetes) · 1 điều kiện áp dụng');
    expect(screen.getByText('cluster-existing').nextElementSibling).toHaveTextContent(/2 điều kiện áp dụng · Hệ thống \(chỉ đọc\)/);
    expect(screen.getByText('ns-shared').nextElementSibling).toHaveTextContent('Dùng chung');
    expect(screen.queryByRole('button', { name: /Sửa|Xóa cấu hình/ })).not.toBeInTheDocument();
  });

  it('rejects a blank or malformed ID and invalid JSON locally without any POST', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-namespace');
    await submit(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('Nhập ID cấu hình.');
    await user.type(field('ID cấu hình'), 'Bad_ID ');
    await submit(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/chỉ gồm chữ thường/);
    expect(field('ID cấu hình')).toHaveValue('Bad_ID ');
    expect(field('ID cấu hình')).toBeInvalid();
    await user.clear(field('ID cấu hình'));
    await user.type(field('ID cấu hình'), 'ns-ok');
    fireEvent.change(field('Tham số cấu hình (JSON object)'), { target: { value: '{bad' } });
    await submit(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('Tham số cấu hình phải là JSON hợp lệ.');
    expect(field('Tham số cấu hình (JSON object)')).toBeVisible();
    fireEvent.change(field('Tham số cấu hình (JSON object)'), { target: { value: '[]' } });
    await submit(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('phải là một JSON object');
    expect(posted).toHaveLength(0);
  });

  it('keeps the whole form after a duplicate 409 and shows a Vietnamese message, never the raw one', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ post: () => Response.json({ error: 'catalog: duplicate: resource definition "ns-dup"' }, { status: 409 }) });
    await ready();
    await user.type(field('ID cấu hình'), 'ns-dup');
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-namespace');
    await user.click(screen.getByRole('button', { name: 'Hiện nâng cao' }));
    fireEvent.change(field('Tham số cấu hình (JSON object)'), { target: { value: '{"name":"orch"}' } });
    await user.type(field('Điều kiện 1 Class'), 'fast');
    await submit(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('ID cấu hình đã tồn tại. Hãy đổi ID rồi thử lại.');
    expect(screen.queryByText(/catalog: duplicate/)).not.toBeInTheDocument();
    expect(field('ID cấu hình')).toHaveValue('ns-dup');
    expect(field('Loại tài nguyên')).toHaveValue('k8s-namespace');
    expect(field('Tham số cấu hình (JSON object)')).toHaveValue('{"name":"orch"}');
    expect(field('Điều kiện 1 Class')).toHaveValue('fast');
    expect(posted).toHaveLength(1);
  });

  it.each([[400, /từ chối cấu hình/], [403, /không có quyền/], [500, /Máy chủ gặp lỗi/]])('maps status %i to a safe Vietnamese message and keeps the form', async (status, message) => {
    const user = userEvent.setup();
    const { posted } = backend({ post: () => Response.json({ error: 'driverInputs.values.variables.storage must be string' }, { status }) });
    await ready();
    await user.type(field('ID cấu hình'), 'ns-fail');
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-namespace');
    await submit(user);
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(message);
    expect(alert).not.toHaveTextContent('must be string');
    expect(field('ID cấu hình')).toHaveValue('ns-fail');
    expect(posted).toHaveLength(1);
  });

  it('reports an unknown outcome after a network failure and never replays the POST', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ post: () => { throw new TypeError('network'); } });
    await ready();
    await user.type(field('ID cấu hình'), 'ns-net');
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-namespace');
    await submit(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/chưa rõ kết quả/);
    expect(posted).toHaveLength(1);
  });

  it('keeps edits while lists reload and while a load fails, with separate Thử lại buttons', async () => {
    const user = userEvent.setup();
    const { gets, state } = backend({ connectionsFail: true });
    await ready();
    await user.type(field('ID cấu hình'), 'ns-edit');
    await user.selectOptions(field('Loại tài nguyên'), 'vpc');
    await user.click(screen.getByRole('button', { name: 'Hiện nâng cao' }));
    fireEvent.change(field('Tham số cấu hình (JSON object)'), { target: { value: '{"cidr":"10.1.0.0/16"}' } });
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải kết nối' });
    expect(screen.queryByText('Chưa có cấu hình tài nguyên nào.')).toBeInTheDocument();
    state.connectionsFail = false;
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await waitFor(() => expect(screen.queryByRole('alert', { name: 'Lỗi tải kết nối' })).not.toBeInTheDocument());
    expect(gets.filter((url) => url.endsWith('/connections'))).toHaveLength(2);
    expect(field('ID cấu hình')).toHaveValue('ns-edit');
    expect(field('Loại tài nguyên')).toHaveValue('vpc');
    expect(field('Tham số cấu hình (JSON object)')).toHaveValue('{"cidr":"10.1.0.0/16"}');
    expect(within(field('Kết nối AWS')).getByRole('option', { name: 'AWS chính (aws-ready)' })).toBeInTheDocument();
  });

  it('keeps metadata and a Vietnamese retry when Resource Types fail to load', async () => {
    const user = userEvent.setup();
    const { state } = backend({ typesFail: true });
    render(<ResourceDefinitionsPage />);
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải loại tài nguyên' });
    await user.type(field('ID cấu hình'), 'ns-keep');
    state.typesFail = false;
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await screen.findByRole('option', { name: 'vpc' });
    expect(field('ID cấu hình')).toHaveValue('ns-keep');
  });

  it('reports a committed registration when the reload fails, resets the form and retries only the list', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ definitionLists: [() => Response.json({ resourceDefinitions: [] }), () => Response.json({ error: 'x' }, { status: 500 })] });
    await ready();
    await user.type(field('ID cấu hình'), 'ns-new');
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-namespace');
    await submit(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên ns-new.');
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải danh sách' });
    expect(field('ID cấu hình')).toHaveValue('');
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await screen.findByText('Chưa có cấu hình tài nguyên nào.');
    expect(posted).toHaveLength(1);
    expect(screen.getByRole('status')).toHaveTextContent('ns-new');
  });

  it('treats inherited Object property names as unsupported custom types without crashing', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ types: ['constructor', 'toString', 'vpc'], definitions: [{ key: 'odd', resourceType: 'constructor', executionProfile: 'constructor', driverType: 'toString', criteria: [{}] }] });
    render(<ResourceDefinitionsPage />);
    expect(await screen.findByText('odd')).toBeInTheDocument();
    expect(screen.getByText('odd').nextElementSibling).toHaveTextContent('constructor · constructor · toString · 1 điều kiện áp dụng');
    await screen.findByRole('option', { name: 'vpc' });
    expect(screen.getByRole('option', { name: 'constructor (chưa hỗ trợ đăng ký)' })).toBeDisabled();
    expect(screen.getByRole('option', { name: 'toString (chưa hỗ trợ đăng ký)' })).toBeDisabled();
    await user.type(field('ID cấu hình'), 'odd-new');
    fireEvent.change(field('Loại tài nguyên'), { target: { value: 'constructor' } });
    expect(field('Cách tạo')).toBeDisabled();
    await submit(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('Chọn một Loại tài nguyên được hỗ trợ.');
    expect(posted).toHaveLength(0);
  });
});
