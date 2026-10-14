import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { unconfiguredTarget, type Application } from '../../shared/types/application';
import { ScorePreviewPage } from './ScorePreviewPage';
import { parseScoreText } from './parseScore';
import { bothConfigured, configuredTarget } from '../../test/targets';

const application: Application = { id: 'shop', name: 'Shop', subdomain: 'shop', environments: bothConfigured('internal-cluster'), workloads: { staging: [], production: [] } };
const yamlScore = 'apiVersion: score.dev/v1b1\nmetadata:\n  name: api\ncontainers:\n  main:\n    image: example.invalid/api:v1\n';
const result = (overrides: Record<string, unknown> = {}) => ({
  applicationKey: 'shop', environmentKey: 'staging', baseSetId: 'set-1', baseVersion: 3, runId: 'run-7', workloadId: 'api', action: 'deploy',
  planHash: '0123456789abcdef0123', delta: { modules: { add: { api: {} } } },
  candidateSet: { modules: { api: {}, web: { spec: { containers: { main: { variables: { TOKEN: '***redacted***' } } } } } }, shared: {} },
  graph: { nodes: [{ descriptor: 'k8s-namespace.default#environments.shop.staging', kind: 'resource', resourceType: 'k8s-namespace', class: 'default', origins: [], paramKeys: [], bindings: {} }, { descriptor: 'workload.api', kind: 'workload', resourceType: 'workload', class: 'default', origins: [], paramKeys: ['size'], bindings: {} }], edges: [{ consumer: 'workload.api', provider: 'k8s-namespace.default#environments.shop.staging', reason: 'execution-profile' }] },
  matches: [{ descriptor: 'k8s-namespace.default#environments.shop.staging', definitionKey: 'namespace-kubernetes', driverType: 'kubernetes', specificity: 0 }],
  batches: [['k8s-namespace.default#environments.shop.staging']],
  classification: { existing: [], new: ['k8s-namespace.default#environments.shop.staging'], unreferenced: [] },
  ...overrides,
});

afterEach(() => vi.unstubAllGlobals());

async function fill(user: ReturnType<typeof userEvent.setup>, score = yamlScore) {
  await user.type(screen.getByLabelText('ID Workload'), 'api');
  await user.type(screen.getByLabelText('ID lần chạy'), 'run-7');
  fireEvent.change(screen.getByLabelText('Score sau (YAML hoặc JSON)'), { target: { value: score } });
}

it('renders technical environment keys as lowercase tabs', () => {
  render(<ScorePreviewPage application={application} environment="staging" />);
  const tabs = screen.getAllByRole('tab');
  expect(tabs.map((tab) => tab.textContent)).toEqual(['staging', 'production']);
  expect(screen.getByRole('tab', { name: 'staging' })).toHaveAttribute('aria-selected', 'true');
  expect(screen.getByRole('tab', { name: 'production' })).toHaveAttribute('aria-selected', 'false');
});

it('parses YAML locally and shows every planning artifact without save or deploy controls', async () => {
  const fetcher = vi.fn(async () => Response.json(result()));
  vi.stubGlobal('fetch', fetcher);
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  expect(screen.getByText(/Không dán giá trị bí mật/)).toBeInTheDocument();
  expect(screen.queryByLabelText('Score trước (YAML hoặc JSON)')).not.toBeInTheDocument();
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));

  const region = await screen.findByRole('region', { name: 'Kết quả xem trước Score' });
  const [url, init] = fetcher.mock.calls[0] as unknown as [string, RequestInit];
  expect(url).toBe('/api/v1/applications/shop/environments/staging/score-preview');
  expect(JSON.parse(String(init.body))).toEqual({ workloadId: 'api', action: 'deploy', runId: 'run-7', scoreAfter: { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/api:v1' } } } });
  expect(region).toHaveTextContent('set-1');
  expect(region).toHaveTextContent('0123456789abcdef');
  expect(region).toHaveTextContent('thêm module api');
  expect(region).toHaveTextContent('Lô 1: k8s-namespace.default#environments.shop.staging');
  expect(region).toHaveTextContent('namespace-kubernetes');
  expect(region).toHaveTextContent('workload.api → k8s-namespace.default#environments.shop.staging');
  expect(region).toHaveTextContent('bản đã lọc dữ liệu nhạy cảm');
  expect(screen.queryByRole('button', { name: /deploy|save/i })).not.toBeInTheDocument();
});

it('renders the builtin cluster as the Environment connection and keeps normal Definition rows', async () => {
  const matches = [
    { descriptor: 'k8s-cluster.internal#connections.lab', definitionKey: 'builtin-existing-cluster', driverType: 'existing-cluster', specificity: -1, binding: 'environment-connection', connectionKey: 'lab' },
    { descriptor: 'k8s-namespace.default#environments.shop.staging', definitionKey: 'namespace-kubernetes', driverType: 'kubernetes', specificity: 0 },
  ];
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(result({ matches }))));
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  const region = await screen.findByRole('region', { name: 'Kết quả xem trước Score' });
  expect(region).toHaveTextContent('Kết nối của môi trường lab');
  expect(region).not.toHaveTextContent('builtin-existing-cluster');
  expect(region).toHaveTextContent('namespace-kubernetes');
});

it('keeps input and blocks the request on local validation errors', async () => {
  const fetcher = vi.fn();
  vi.stubGlobal('fetch', fetcher);
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  await user.selectOptions(screen.getByLabelText('Thao tác'), 'update');
  fireEvent.change(screen.getByLabelText('Score sau (YAML hoặc JSON)'), { target: { value: 'containers: [' } });
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  const alert = screen.getByRole('alert');
  expect(alert).toHaveTextContent('ID Workload là bắt buộc.');
  expect(alert).toHaveTextContent('ID lần chạy là bắt buộc.');
  expect(alert).toHaveTextContent('Score trước là bắt buộc.');
  expect(alert).toHaveTextContent('Score sau không phải YAML hoặc JSON hợp lệ.');
  expect(screen.getByLabelText('Score sau (YAML hoặc JSON)')).toHaveValue('containers: [');
  expect(fetcher).not.toHaveBeenCalled();
});

it('shows server validation and failures in safe Vietnamese with retry, never raw text or a stale result', async () => {
  let call = 0;
  vi.stubGlobal('fetch', vi.fn(async () => {
    call += 1;
    if (call === 1) return Response.json({ error: 'scoreAfter metadata.name "web" does not match workloadId "api"', code: 'INVALID_REQUEST' }, { status: 400 });
    if (call === 2) return Response.json({ error: 'preview failed; retry token=SECRET-1', code: 'INTERNAL_ERROR' }, { status: 500 });
    return Response.json(result({ delta: {} }));
  }));
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  const first = await screen.findByRole('alert');
  expect(first).toHaveTextContent('Không thể xem trước: Dữ liệu gửi lên không hợp lệ.');
  expect(first).not.toHaveTextContent(/metadata\.name "web"|does not match/);
  expect(screen.queryByRole('button', { name: 'Thử lại' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  const second = await screen.findByRole('alert');
  expect(second).toHaveTextContent('Máy chủ gặp lỗi không xác định');
  expect(second).not.toHaveTextContent(/SECRET|preview failed/);
  await user.click(screen.getByRole('button', { name: 'Thử lại' }));
  expect(await screen.findByText(/Không có thay đổi Workload/)).toBeInTheDocument();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
});

it.each([
  [413, { error: 'body too large: raw', code: 'REQUEST_TOO_LARGE' }, 'Nội dung gửi lên quá lớn', false],
  [404, { error: 'application "shop" not found', code: 'NOT_FOUND' }, 'Không tìm thấy hoặc không truy cập được mục này.', true],
  [422, { error: 'other 422 raw', field: 'other', code: '__proto__' }, 'Máy chủ không chấp nhận yêu cầu này.', true],
  [422, { error: 'raw', code: 'CONNECTION_UNAVAILABLE', field: 'connectionKey' }, 'Kết nối không hợp lệ hoặc chưa sẵn sàng', true],
])('maps HTTP %i to a Vietnamese message without raw backend text', async (status, body, text, retry) => {
  const fetcher = vi.fn(async () => Response.json(body, { status }));
  vi.stubGlobal('fetch', fetcher);
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent(text);
  expect(alert).not.toHaveTextContent(/raw|not found|too large/);
  expect(screen.queryByRole('button', { name: 'Thử lại' }) !== null).toBe(retry);
  expect(screen.queryByRole('region', { name: 'Kết quả xem trước Score' })).not.toBeInTheDocument();
});

it('shows a Vietnamese error for a network failure and retry calls only score-preview', async () => {
  const fetcher = vi.fn()
    .mockRejectedValueOnce(new TypeError('Failed to fetch'))
    .mockImplementation(async () => Response.json(result()));
  vi.stubGlobal('fetch', fetcher);
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent('Không thể xem trước Score. Hãy thử lại.');
  expect(alert).not.toHaveTextContent('Failed to fetch');
  await user.click(screen.getByRole('button', { name: 'Thử lại' }));
  await screen.findByRole('region', { name: 'Kết quả xem trước Score' });
  expect(fetcher).toHaveBeenCalledTimes(2);
  for (const call of fetcher.mock.calls as unknown as [string, RequestInit][]) {
    expect(call[0]).toBe('/api/v1/applications/shop/environments/staging/score-preview');
    expect(call[1].method).toBe('POST');
  }
});

it('sends update and remove payloads with only the relevant Score editors and keeps the enum values', async () => {
  const fetcher = vi.fn(async () => Response.json(result({ action: 'update', delta: { modules: { update: { api: [{ op: 'replace', path: '/spec/image' }] }, remove: ['old'] }, shared: [{ op: 'add', path: '/x' }] } })));
  vi.stubGlobal('fetch', fetcher);
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  const body = () => JSON.parse(String((fetcher.mock.calls.at(-1) as unknown as [string, RequestInit])[1].body));
  await user.type(screen.getByLabelText('ID Workload'), 'api');
  await user.type(screen.getByLabelText('ID lần chạy'), 'run-7');
  await user.selectOptions(screen.getByLabelText('Thao tác'), 'update');
  expect(screen.getByRole('option', { name: 'Cập nhật Workload' })).toHaveValue('update');
  fireEvent.change(screen.getByLabelText('Score trước (YAML hoặc JSON)'), { target: { value: '{"metadata":{"name":"api"},"v":1}' } });
  fireEvent.change(screen.getByLabelText('Score sau (YAML hoặc JSON)'), { target: { value: 'metadata:\n  name: api\nv: 2\n' } });
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  const region = await screen.findByRole('region', { name: 'Kết quả xem trước Score' });
  expect(body()).toEqual({ workloadId: 'api', action: 'update', runId: 'run-7', scoreBefore: { metadata: { name: 'api' }, v: 1 }, scoreAfter: { metadata: { name: 'api' }, v: 2 } });
  expect(region).toHaveTextContent('cập nhật module api · 1 thay đổi: replace /spec/image');
  expect(region).toHaveTextContent('xóa module old');
  expect(region).toHaveTextContent('dùng chung · 1 thay đổi: add /x');
  await user.selectOptions(screen.getByLabelText('Thao tác'), 'remove');
  expect(screen.queryByRole('region', { name: 'Kết quả xem trước Score' })).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Score sau (YAML hoặc JSON)')).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  await screen.findByRole('region', { name: 'Kết quả xem trước Score' });
  expect(body()).toEqual({ workloadId: 'api', action: 'remove', runId: 'run-7', scoreBefore: { metadata: { name: 'api' }, v: 1 } });
});

it('explains a planner rejection by its allow-listed stage and ignores unknown params', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ error: 'raw planner SECRET', code: 'PLANNING_REJECTED', params: { stage: 'graph', reason: 'SECRET', extra: 'SECRET' } }, { status: 400 })));
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent('Không thể lập kế hoạch');
  expect(alert).toHaveTextContent('đồ thị tài nguyên');
  expect(alert).not.toHaveTextContent(/SECRET|raw planner/);
});

it('explains an UNCONFIGURED Environment instead of showing a plan', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ error: 'the Environment has no execution connection; set one in Environment Settings', field: 'connectionKey', code: 'ENVIRONMENT_UNCONFIGURED' }, { status: 422 })));
  const user = userEvent.setup();
  render(<ScorePreviewPage application={{ ...application, environments: { staging: unconfiguredTarget, production: configuredTarget('lab') } }} environment="staging" />);
  expect(screen.getByLabelText('Đích triển khai')).toHaveTextContent('staging chưa có kết nối triển khai');
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Môi trường staging chưa có kết nối triển khai');
  expect(screen.getByRole('alert')).toHaveTextContent('Cài đặt môi trường');
  expect(screen.getByRole('alert')).not.toHaveTextContent(/execution connection|Environment Settings/);
  expect(screen.queryByText(/Không có thay đổi Workload/)).not.toBeInTheDocument();
});

it('disables duplicate submits and ignores a response for input that changed', async () => {
  let resolve: (value: Response) => void = () => undefined;
  const fetcher = vi.fn(() => new Promise<Response>((done) => { resolve = done; }));
  vi.stubGlobal('fetch', fetcher);
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  expect(screen.getByRole('button', { name: 'Đang tính kết quả xem trước…' })).toBeDisabled();
  await user.type(screen.getByLabelText('ID lần chạy'), '8');
  resolve(Response.json(result()));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Xem trước' })).toBeEnabled());
  expect(screen.queryByRole('region', { name: 'Kết quả xem trước Score' })).not.toBeInTheDocument();
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it('clears the result when the Environment scope changes', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(result())));
  const user = userEvent.setup();
  const { rerender } = render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  await screen.findByRole('region', { name: 'Kết quả xem trước Score' });
  rerender(<ScorePreviewPage application={application} environment="production" />);
  expect(screen.queryByRole('region', { name: 'Kết quả xem trước Score' })).not.toBeInTheDocument();
  expect(screen.getByText('Xem trước Score').closest('header')).toHaveTextContent('production');
});

it('ignores a late response after the Environment scope changed', async () => {
  let resolve: (value: Response) => void = () => undefined;
  vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>((done) => { resolve = done; })));
  const user = userEvent.setup();
  const { rerender } = render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  rerender(<ScorePreviewPage application={application} environment="production" />);
  resolve(Response.json(result()));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Xem trước' })).toBeEnabled());
  expect(screen.queryByRole('region', { name: 'Kết quả xem trước Score' })).not.toBeInTheDocument();
});

it('shows the full plan hash and labels documents as a sanitized view', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(result())));
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  expect(await screen.findByTitle('0123456789abcdef0123')).toHaveTextContent('0123456789abcdef0123');
  expect(screen.getByRole('heading', { name: 'Tài liệu (bản đã lọc dữ liệu nhạy cảm)' })).toBeInTheDocument();
  expect(screen.getByText('Candidate Deployment Set — bản đã lọc dữ liệu nhạy cảm, không thể thực thi')).toBeInTheDocument();
});

it('rejects YAML alias bombs and self-references with actionable messages', () => {
  const bomb = ['a: &a [x, x, x, x, x, x, x, x, x]', 'b: &b [*a, *a, *a, *a, *a, *a, *a, *a, *a]', 'c: &c [*b, *b, *b, *b, *b, *b, *b, *b, *b]', 'd: [*c, *c, *c, *c, *c, *c, *c, *c, *c]'].join('\n');
  const bombResult = parseScoreText(bomb);
  expect(bombResult.ok).toBe(false);
  expect(!bombResult.ok && bombResult.error).toMatch(/alias/);
  const selfReference = parseScoreText('metadata: &m\n  name: api\n  self: *m\n');
  expect(selfReference.ok).toBe(false);
  expect(!selfReference.ok && selfReference.error).toMatch(/tự tham chiếu|alias/);
  expect(parseScoreText('base: &b {image: x}\nother: *b')).toEqual({ ok: true, score: { base: { image: 'x' }, other: { image: 'x' } } });
});

it('accepts one JSON or YAML Score object only', () => {
  expect(parseScoreText('{"metadata":{"name":"api"}}')).toEqual({ ok: true, score: { metadata: { name: 'api' } } });
  expect(parseScoreText('a: 1\n---\nb: 2')).toEqual({ ok: false, error: 'phải chứa đúng một tài liệu.' });
  expect(parseScoreText('- a')).toEqual({ ok: false, error: 'phải là một đối tượng Score.' });
  expect(parseScoreText('   ')).toEqual({ ok: false, error: 'là bắt buộc.' });
});

it('shows the Environment connection binding that planning will use', () => {
  render(<ScorePreviewPage application={{ ...application, environments: { staging: configuredTarget('internal-cluster'), production: configuredTarget('lab') } }} environment="production" />);
  expect(screen.getByLabelText('Đích triển khai')).toHaveTextContent('production · Kết nối lab (lab) · Phạm vi triển khai Cluster nội bộ (internal-k8s)');
});

it('never exposes raw YAML error text and keeps alias limits and values', () => {
  for (const text of ['containers: [', 'a: b: c', '{"a":', 'key: "unterminated']) {
    const parsed = parseScoreText(text);
    expect(parsed).toEqual({ ok: false, error: 'không phải YAML hoặc JSON hợp lệ.' });
  }
  const twentyOne = `a: &a x\nb: [${Array.from({ length: 21 }, () => '*a').join(', ')}]`;
  expect(parseScoreText(twentyOne).ok).toBe(false);
  expect(parseScoreText('a: &a x\nb: [*a, *a, *a]')).toEqual({ ok: true, score: { a: 'x', b: ['x', 'x', 'x'] } });
  expect(parseScoreText('42')).toEqual({ ok: false, error: 'phải là một đối tượng Score.' });
});

it('keeps preview read-only: no save or deploy control before or after a result', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(result())));
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  expect(screen.queryByRole('button', { name: /triển khai|lưu|deploy|save/i })).not.toBeInTheDocument();
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  await screen.findByRole('region', { name: 'Kết quả xem trước Score' });
  expect(screen.queryByRole('button', { name: /triển khai|lưu|deploy|save/i })).not.toBeInTheDocument();
});

const intent = (overrides: Record<string, unknown> = {}) => ({
  descriptor: 'redis.in-cluster#shared.cache', resourceType: 'redis', class: 'in-cluster', scope: 'SHARED:cache', ownership: 'ORCHESTRATOR', intent: 'CREATE',
  definitionKey: 'redis-module', driverType: 'terraform',
  module: { source: 'platform', module: 'redis-k8s', revision: 'v3-2026.10.13', subdirectory: 'redis-k8s', contentSha256: '446aeb69eafa37573233ff53629372172b8ce80439e8c8c2440fdbede9a860e1' },
  providers: [{ name: 'kubernetes', kind: 'kubernetes', target: 'environment', connectionKey: 'internal-cluster' }],
  ...overrides,
});

it('shows the safe resource intents, module pin, provider target and ownership of a standalone Preview (UC-05 BR-10)', async () => {
  const body = result({ resourceIntents: [intent(), intent({ descriptor: 'postgres.in-cluster#shared.db', resourceType: 'postgres', intent: 'RETAIN', scope: 'SHARED:db' })], binding: { connectionKey: 'internal-cluster', executionProfile: 'internal-k8s', region: '' } });
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(body)));
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  const table = await screen.findByLabelText('Tài nguyên hạ tầng sẽ được xử lý');
  expect(table).toHaveTextContent('redis-k8s · v3-2026.10.13');
  expect(table).toHaveTextContent('sha256:446aeb69eafa');
  expect(table).toHaveTextContent('kubernetes: cluster của Môi trường internal-cluster');
  expect(table).toHaveTextContent('Orchestrator (theo phạm vi tài nguyên)');
  expect(table).toHaveTextContent('Tạo mới');
  expect(table).toHaveTextContent('Giữ nguyên');
  expect(screen.getByLabelText('Đích đã ghim của xem trước')).toHaveTextContent('Kết nối internal-cluster · phạm vi internal-k8s');
  expect(screen.getByLabelText('Đích đã ghim của xem trước')).not.toHaveTextContent('tài khoản AWS');
});

it('shows the AWS account and region beside the pinned target, never an override', async () => {
  const body = result({ resourceIntents: [intent({ resourceType: 'vpc', class: 'default', providers: [{ name: 'aws', kind: 'aws', target: 'environment', connectionKey: 'aws-main', region: 'eu-west-1' }] })], binding: { connectionKey: 'aws-main', executionProfile: 'aws-eks', region: 'eu-west-1' }, accountId: '123456789012' });
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(body)));
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  expect(await screen.findByLabelText('Đích đã ghim của xem trước')).toHaveTextContent('Kết nối aws-main · phạm vi aws-eks · vùng eu-west-1 · tài khoản AWS 123456789012');
  expect(screen.getByLabelText('Tài nguyên hạ tầng sẽ được xử lý')).toHaveTextContent('aws: Kết nối AWS của Môi trường aws-main · vùng eu-west-1');
});

it('treats a response without resourceIntents as unavailable, never as nothing to create, and an empty list as empty', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(result())));
  const user = userEvent.setup();
  const view = render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  expect(await screen.findByText(/Chưa có thông tin tài nguyên hạ tầng trong kết quả xem trước này/)).toBeInTheDocument();
  expect(screen.queryByText('Không có tài nguyên hạ tầng nào cần tạo, cập nhật hoặc liên kết.')).not.toBeInTheDocument();
  view.unmount();
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(result({ resourceIntents: [] }))));
  render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Xem trước' }));
  expect(await screen.findByText('Không có tài nguyên hạ tầng nào cần tạo, cập nhật hoặc liên kết.')).toBeInTheDocument();
});
