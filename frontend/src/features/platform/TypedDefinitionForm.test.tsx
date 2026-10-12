import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { capabilityFixture } from '../../test/capabilities';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';

type Body = Record<string, unknown>;
const connections = [{ key: 'aws-ready', name: 'AWS chính', kind: 'AWS', status: 'READY' }];

function backend(options: { capabilities?: () => Response | Promise<Response> } = {}) {
  const posted: Body[] = [];
  const methods: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    methods.push(`${init?.method ?? 'GET'} ${url.replace('/api/v1', '')}`);
    if (init?.method === 'POST') { posted.push(JSON.parse(String(init.body)) as Body); return Response.json({}, { status: 201 }); }
    if (url.endsWith('/resource-definition-capabilities')) return options.capabilities ? options.capabilities() : Response.json({ capabilities: capabilityFixture });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: ['vpc', 'k8s-cluster', 'k8s-namespace', 'postgres', 'cache'].map((key) => ({ key })) });
    if (url.endsWith('/connections')) return Response.json({ connections });
    if (url.endsWith('/applications')) return Response.json({ applications: [] });
    return Response.json({ resourceDefinitions: [] });
  }));
  return { posted, methods };
}
afterEach(() => vi.unstubAllGlobals());
const field = (name: string) => screen.getByLabelText(name);
async function ready() {
  render(<ResourceDefinitionsPage />);
  await screen.findByText('Chưa có cấu hình tài nguyên nào.');
  await screen.findByRole('option', { name: 'vpc' });
}
const submit = (user: ReturnType<typeof userEvent.setup>) => user.click(screen.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên' }));
const driverInputs = (body?: Body) => (body?.driverInputs as { values: { variables: Body; source?: Body } }).values;

describe('T31 typed Definition form', () => {
  it('omits defaults and Resource Type params, and sends only opted-in overrides with their JSON types', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình'), 'pg-typed');
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    expect(screen.getByText(/Mặc định: 1Gi/)).toBeInTheDocument();
    expect(screen.getAllByText(/Nguồn: tham số \(param\) của Loại tài nguyên.*param của workload vẫn được ưu tiên/).length).toBe(2);
    // Nothing is sent for defaults or params until the author opts in.
    expect(screen.queryByLabelText('Giá trị storage')).not.toBeInTheDocument();
    await user.click(field('Ghi đè storage'));
    await user.clear(field('Giá trị storage'));
    await user.type(field('Giá trị storage'), '5Gi');
    await submit(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên pg-typed.');
    expect(driverInputs(posted[0])).toEqual({ variables: { storage: '5Gi' } });
    expect(driverInputs(posted[0])).not.toHaveProperty('source');
  });

  it('applies the namespace preset only on request and never shows executor-owned controls', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình'), 'ns-typed');
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-namespace');
    expect(screen.queryByLabelText('Giá trị name')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Dùng preset cho name' }));
    expect(field('Giá trị name')).toHaveValue('${context.env.namespace}');
    await submit(user);
    await screen.findByRole('status');
    expect(driverInputs(posted[0])).toEqual({ variables: { name: '${context.env.namespace}' } });

    await user.type(field('ID cấu hình'), 'aurora-typed');
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    await user.selectOptions(field('Cách tạo'), 'terraform');
    for (const name of ['region', 'tags', 'master_password']) expect(screen.queryByLabelText(`Ghi đè ${name}`)).not.toBeInTheDocument();
    expect(screen.getByText(/Hệ thống tự cung cấp, không có ô nhập/)).toBeInTheDocument();
  });

  it('keeps number, list and placeholder values typed and sends the @infra refs as whole placeholders', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình'), 'aurora-typed');
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    await user.selectOptions(field('Cách tạo'), 'terraform');
    await user.selectOptions(field('Kết nối AWS'), 'aws-ready');
    await user.click(field('Ghi đè min_capacity'));
    await user.type(field('Giá trị min_capacity'), '0.5');
    await user.click(field('Ghi đè max_capacity'));
    await user.type(field('Giá trị max_capacity'), '${{context.x}');
    await user.click(screen.getByRole('button', { name: 'Dùng preset cho subnet_ids' }));
    await user.click(screen.getByRole('button', { name: 'Dùng preset cho vpc_id' }));
    await submit(user);
    await screen.findByRole('status');
    expect(driverInputs(posted[0])).toEqual({ source: { module: 'aurora' }, variables: { min_capacity: 0.5, max_capacity: '${context.x}', subnet_ids: "${resources['vpc.default#@infra'].outputs.subnetIds}", vpc_id: "${resources['vpc.default#@infra'].outputs.id}" } });
  });

  it('rejects a malformed number locally without a POST and keeps the draft', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình'), 'vpc-bad');
    await user.selectOptions(field('Loại tài nguyên'), 'vpc');
    await user.click(field('Ghi đè az_count'));
    await user.type(field('Giá trị az_count'), 'ba');
    await submit(user);
    expect((await screen.findAllByRole('alert')).map((alert) => alert.textContent).join(' ')).toMatch(/phải là số hợp lệ/);
    expect(posted).toEqual([]);
    expect(field('Giá trị az_count')).toHaveValue('ba');
  });

  it('round-trips typed values through advanced JSON and stays advanced for unrepresentable JSON', async () => {
    const user = userEvent.setup();
    backend();
    await ready();
    await user.selectOptions(field('Loại tài nguyên'), 'vpc');
    await user.click(field('Ghi đè cidr'));
    await user.type(field('Giá trị cidr'), '10.9.0.0/16');
    await user.click(field('Ghi đè az_count'));
    await user.type(field('Giá trị az_count'), '3');
    await user.click(screen.getByRole('button', { name: 'Tham số: JSON nâng cao' }));
    expect(JSON.parse((field('Tham số cấu hình (JSON object)') as HTMLTextAreaElement).value)).toEqual({ cidr: '10.9.0.0/16', az_count: 3 });
    // Unknown/executor-owned fields cannot be shown by typed controls: stay in JSON, keep the text.
    const box = field('Tham số cấu hình (JSON object)');
    await user.clear(box);
    await user.click(box);
    await user.paste('{"cidr":"10.1.0.0/16","region":"x"}');
    await user.click(screen.getByRole('button', { name: 'Tham số: biểu mẫu' }));
    expect(screen.getByText(/không có control typed/)).toBeInTheDocument();
    expect(field('Tham số cấu hình (JSON object)')).toHaveValue('{"cidr":"10.1.0.0/16","region":"x"}');
    await user.clear(field('Tham số cấu hình (JSON object)'));
    await user.click(field('Tham số cấu hình (JSON object)'));
    await user.paste('{"az_count":4}');
    await user.click(screen.getByRole('button', { name: 'Tham số: biểu mẫu' }));
    expect(field('Giá trị az_count')).toHaveValue('4');
  });

  it('keeps a separate draft per type/driver pair and resets only the submitted pair', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình'), 'pair-test');
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    await user.click(field('Ghi đè storage'));
    await user.type(field('Giá trị storage'), '9Gi');
    await user.selectOptions(field('Cách tạo'), 'terraform');
    expect(screen.queryByLabelText('Ghi đè storage')).not.toBeInTheDocument();
    await user.click(field('Ghi đè min_capacity'));
    await user.type(field('Giá trị min_capacity'), '2');
    await user.selectOptions(field('Cách tạo'), 'kubernetes');
    expect(field('Giá trị storage')).toHaveValue('9Gi');
    await user.selectOptions(field('Cách tạo'), 'terraform');
    expect(field('Giá trị min_capacity')).toHaveValue('2');
    await user.selectOptions(field('Cách tạo'), 'kubernetes');
    await submit(user);
    await screen.findByRole('status');
    expect(driverInputs(posted[0]).variables).toEqual({ storage: '9Gi' });
    expect(field('Ghi đè storage')).not.toBeChecked();
    await user.selectOptions(field('Cách tạo'), 'terraform');
    expect(field('Giá trị min_capacity')).toHaveValue('2');
  });

  it('locks types and registration while metadata loads or fails, with GET-only retry that keeps edits', async () => {
    const user = userEvent.setup();
    let fail = true;
    const { posted, methods } = backend({ capabilities: () => (fail ? Response.json({ code: 'INTERNAL' }, { status: 500 }) : Response.json({ capabilities: capabilityFixture })) });
    render(<ResourceDefinitionsPage />);
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải metadata cách tạo' });
    await screen.findByRole('option', { name: 'vpc (chưa xác nhận hỗ trợ)' });
    expect(screen.getByRole('option', { name: 'vpc (chưa xác nhận hỗ trợ)' })).toBeDisabled();
    expect(screen.queryByText(/chưa hỗ trợ đăng ký/)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên' })).toBeDisabled();
    await user.type(field('ID cấu hình'), 'kept-id');
    fail = false;
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
    expect(screen.getByRole('option', { name: 'cache (chưa hỗ trợ đăng ký)' })).toBeDisabled();
    expect(field('ID cấu hình')).toHaveValue('kept-id');
    expect(posted).toEqual([]);
    expect(methods.filter((entry) => entry.startsWith('POST'))).toEqual([]);
  });

  it('edits provision rows with flags and explanations and keeps the rule through advanced JSON', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình'), 'pg-prov');
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên đi kèm' }));
    expect(screen.getByText(/consumer → provider/)).toBeInTheDocument();
    await user.type(field('Descriptor tài nguyên đi kèm 1'), 'vpc.default#@infra');
    await user.click(field('Tài nguyên đi kèm 1 phụ thuộc vào tài nguyên cha (is_dependent)'));
    await user.click(field('Nối consumer của tài nguyên cha tới tài nguyên đi kèm 1 (match_dependents)'));
    await user.click(screen.getByRole('button', { name: 'Quy tắc: JSON nâng cao' }));
    expect(JSON.parse((field('Quy tắc tạo tài nguyên liên quan (JSON object)') as HTMLTextAreaElement).value)).toEqual({ 'vpc.default#@infra': { is_dependent: true, match_dependents: true } });
    await user.click(screen.getByRole('button', { name: 'Quy tắc: biểu mẫu' }));
    expect(field('Descriptor tài nguyên đi kèm 1')).toHaveValue('vpc.default#@infra');
    expect(field('Tài nguyên đi kèm 1 phụ thuộc vào tài nguyên cha (is_dependent)')).toBeChecked();
    await submit(user);
    await screen.findByRole('status');
    expect(posted[0]?.provision).toEqual({ 'vpc.default#@infra': { is_dependent: true, match_dependents: true } });
  });
});
