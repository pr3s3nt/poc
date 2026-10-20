import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { backend, completeDefinition, field, submitDefinition } from '../../test/definitionKit';

afterEach(() => vi.unstubAllGlobals());
const variables = (body?: Record<string, unknown>) => ((body?.driverInputs as { values: { variables: Record<string, unknown> } }).values).variables;
async function ready() {
  render(<ResourceDefinitionsPage />);
  await screen.findByText('Chưa có cấu hình tài nguyên nào.');
  await screen.findByRole('option', { name: 'vpc' });
}

describe('T31 typed Definition variables over verified pins', () => {
  it('omits defaults and Resource Type params, and sends only opted-in overrides with their JSON types', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'pg-typed', type: 'postgres', module: 'postgres-k8s' });
    // Nothing is sent for an optional variable or a param until the author opts in.
    expect(screen.queryByLabelText('Giá trị storage')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Giá trị database')).not.toBeInTheDocument();
    expect(screen.getAllByText(/Param thật của workload luôn được ưu tiên hơn giá trị dự phòng/).length).toBe(2);
    await user.click(field('Ghi đè storage'));
    await user.clear(field('Giá trị storage'));
    await user.type(field('Giá trị storage'), '5Gi');
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên pg-typed.');
    expect(variables(posted[0])).toEqual({ storage: '5Gi', namespace: "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}" });
  });

  it('applies the namespace preset only on request and never shows executor-owned controls', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình (Definition ID)'), 'ns-typed');
    await user.type(field('Tên hiển thị'), 'Namespace typed');
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-namespace');
    await user.click(screen.getByRole('button', { name: 'Ghép đầu ra theo tên trùng' }));
    await user.selectOptions(field('Cluster đích của provider kubernetes'), 'resource');
    await user.selectOptions(field('Class cluster EKS của provider kubernetes'), 'eks');
    expect(screen.queryByLabelText('Giá trị name')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Dùng preset cho name' }));
    expect(field('Giá trị name')).toHaveValue('${context.env.namespace}');
    await submitDefinition(user);
    await screen.findByRole('status');
    expect(variables(posted[0])).toEqual({ name: '${context.env.namespace}' });

    await user.type(field('ID cấu hình (Definition ID)'), 'aurora-typed');
    await user.type(field('Tên hiển thị'), 'Aurora typed');
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    await user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: /^aurora ·/ }));
    for (const name of ['region', 'tags', 'master_password', 'password']) expect(screen.queryByLabelText(`Ghi đè ${name}`)).not.toBeInTheDocument();
    expect(screen.getByText(/Hệ thống tự cung cấp, không có ô nhập/)).toBeInTheDocument();
    expect(screen.getByText('master_password', { selector: 'code' })).toBeInTheDocument();
  });

  it('keeps number, list and placeholder values typed and sends the @infra refs as whole placeholders', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'aurora-typed', type: 'postgres', module: 'aurora', skipVariables: ['name'] });
    await user.click(field('Ghi đè engine_version'));
    await user.type(field('Giá trị engine_version'), '16.4');
    await user.click(field('Ghi đè min_capacity'));
    await user.type(field('Giá trị min_capacity'), '0.5');
    await user.click(field('Ghi đè max_capacity'));
    await user.type(field('Giá trị max_capacity'), '${{context.x}');
    await user.click(field('Ghi đè name'));
    await user.type(field('Giá trị name'), 'db-${{context.env.profile}');
    await submitDefinition(user);
    await screen.findByRole('status');
    expect(variables(posted[0])).toEqual({
      engine_version: '16.4', min_capacity: 0.5, max_capacity: '${context.x}', name: 'db-${context.env.profile}',
      vpc_id: "${resources['vpc.default#@infra'].outputs.id}", subnet_ids: "${resources['vpc.default#@infra'].outputs.subnetIds}", private_subnet_ids: "${resources['vpc.default#@infra'].outputs.privateSubnetIds}",
      region: 'us-east-1', // the literal region comes from its own control, never from a typed override
    });
  });

  it('rejects a malformed number locally without a POST and keeps the draft', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'vpc-bad', type: 'vpc' });
    await user.click(field('Ghi đè az_count'));
    await user.type(field('Giá trị az_count'), 'ba');
    await submitDefinition(user);
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
    fireEvent.change(box, { target: { value: '{"cidr":"10.1.0.0/16","region":"x"}' } });
    await user.click(screen.getByRole('button', { name: 'Tham số: biểu mẫu' }));
    expect(screen.getByText(/không có control typed/)).toBeInTheDocument();
    expect(field('Tham số cấu hình (JSON object)')).toHaveValue('{"cidr":"10.1.0.0/16","region":"x"}');
    fireEvent.change(field('Tham số cấu hình (JSON object)'), { target: { value: '{"az_count":4}' } });
    await user.click(screen.getByRole('button', { name: 'Tham số: biểu mẫu' }));
    expect(field('Giá trị az_count')).toHaveValue('4');
  });

  it('edits provision rows with flags and explanations and keeps the rule through advanced JSON', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'pg-prov', type: 'postgres', module: 'postgres-k8s' });
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
    await submitDefinition(user);
    await screen.findByRole('status');
    expect(posted[0]?.provision).toEqual({ 'vpc.default#@infra': { is_dependent: true, match_dependents: true } });
  });
});
