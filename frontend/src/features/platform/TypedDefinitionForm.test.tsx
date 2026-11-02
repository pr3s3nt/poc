import { fireEvent, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { renderDefinition, backend, card, completeDefinition, field, provideLiteral, provideSource, submitDefinition } from '../../test/definitionKit';

afterEach(() => vi.unstubAllGlobals());
const variables = (body?: Record<string, unknown>) => ((body?.driverInputs as { values: { variables: Record<string, unknown> } }).values).variables;
async function ready() {
  renderDefinition(<ResourceDefinitionsPage />);
  await screen.findByText('Chưa có cấu hình tài nguyên nào.');
  await screen.findByRole('option', { name: 'vpc' });
}

describe('T31 typed Definition variables over verified pins', () => {
  it('omits defaults and Resource Type params, and sends only explicit source choices with their JSON types', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'pg-typed', type: 'postgres', module: 'postgres-k8s' });
    // Nothing is sent for a default variable or a mapped param until the author picks a source.
    expect(card('storage')).toHaveTextContent('Mặc định của module');
    expect(card('storage')).toHaveTextContent('1Gi');
    expect(card('database')).toHaveTextContent('Tham số ứng dụng cung cấp');
    expect(card('database')).toHaveTextContent('database → database');
    expect(within(card('database')).getByText(/Tham số ứng dụng được ưu tiên/)).toBeInTheDocument();
    expect(screen.queryByLabelText('Giá trị storage')).not.toBeInTheDocument();
    await provideLiteral(user, 'storage', '5Gi');
    expect(card('storage')).toHaveTextContent('Nhập trực tiếp');
    await submitDefinition(user);
    expect(await screen.findByText(/^Đã đăng ký cấu hình tài nguyên/)).toHaveTextContent('Đã đăng ký cấu hình tài nguyên pg-typed.');
    expect(variables(posted[0])).toEqual({ storage: '5Gi', namespace: "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}" });
  });

  it('applies a source only when the author chooses it and never shows executor-owned controls', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình (Definition ID)'), 'ns-typed');
    await user.type(field('Tên hiển thị'), 'Namespace typed');
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-namespace');
    await user.click(screen.getByRole('button', { name: 'Ghép đầu ra theo tên trùng' }));
    await user.selectOptions(field('Nhóm cluster của provider kubernetes'), 'eks');
    expect(card('name')).toHaveTextContent('Chưa cung cấp');
    expect(screen.queryByLabelText('Giá trị name')).not.toBeInTheDocument();
    await provideSource(user, 'name', '${context.env.namespace}');
    expect(card('name')).toHaveTextContent('Từ ngữ cảnh triển khai');
    expect(card('name')).toHaveTextContent('Namespace của môi trường');
    expect(within(card('name')).getByText('${context.env.namespace}')).toBeInTheDocument();
    await submitDefinition(user);
    await screen.findByText(/^Đã đăng ký cấu hình tài nguyên/);
    expect(variables(posted[0])).toEqual({ name: '${context.env.namespace}' });

    await user.click(screen.getByRole('button', { name: 'Module đã đăng ký' }));
    await user.type(field('ID cấu hình (Definition ID)'), 'aurora-typed');
    await user.type(field('Tên hiển thị'), 'Aurora typed');
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    await user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: /^aurora ·/ }));
    for (const name of ['region', 'tags', 'master_password', 'password']) expect(card(name)).toBeNull();
    expect(screen.getByText(/Hệ thống tự cung cấp/)).toBeInTheDocument();
    expect(screen.getByText('master_password', { selector: 'code' })).toBeInTheDocument();
  });

  it('keeps number, whole-placeholder and interpolated values typed and sends the @infra refs as whole placeholders', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'aurora-typed', type: 'postgres', module: 'aurora', skipVariables: ['name'] });
    await provideLiteral(user, 'engine_version', '16.4');
    await provideLiteral(user, 'min_capacity', '0.5');
    await provideLiteral(user, 'max_capacity', '${context.x}');
    expect(card('max_capacity')).toHaveTextContent('Biểu thức nâng cao');
    await provideLiteral(user, 'name', 'db-${context.env.profile}');
    await submitDefinition(user);
    await screen.findByText(/^Đã đăng ký cấu hình tài nguyên/);
    expect(variables(posted[0])).toEqual({
      engine_version: '16.4', min_capacity: 0.5, max_capacity: '${context.x}', name: 'db-${context.env.profile}',
      vpc_id: "${resources['vpc.default#@infra'].outputs.id}", subnet_ids: "${resources['vpc.default#@infra'].outputs.subnetIds}", private_subnet_ids: "${resources['vpc.default#@infra'].outputs.privateSubnetIds}",
      region: 'us-east-1', // the literal region comes from its own control, never from a source override
    });
  });

  it('refuses a malformed number inside the chooser, keeps the draft and sends nothing', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'vpc-bad', type: 'vpc' });
    await user.click(screen.getByRole('button', { name: 'Thay đổi az_count' }));
    const dialog = within(screen.getByRole('dialog'));
    await user.click(dialog.getByRole('radio', { name: 'Nhập giá trị' }));
    await user.type(dialog.getByLabelText('Giá trị az_count'), 'ba');
    expect(dialog.getByText(/phải là số hợp lệ/)).toBeInTheDocument();
    expect(dialog.getByRole('button', { name: 'Áp dụng' })).toBeDisabled();
    await user.click(dialog.getByRole('button', { name: 'Hủy' }));
    expect(card('az_count')).toHaveTextContent('Mặc định của module');
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('vpc-bad');
    expect(posted).toEqual([]);
  });

  it('round-trips typed values through advanced JSON and stays advanced for unrepresentable JSON', async () => {
    const user = userEvent.setup();
    backend();
    await ready();
    await user.selectOptions(field('Loại tài nguyên'), 'vpc');
    await provideLiteral(user, 'cidr', '10.9.0.0/16');
    await provideLiteral(user, 'az_count', '3');
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
    expect(card('az_count')).toHaveTextContent('Nhập trực tiếp');
    expect(card('az_count')).toHaveTextContent('4');
  });

  it('builds a companion from Type, class and scope, names who waits for whom and keeps the rule through advanced JSON', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'pg-prov', type: 'postgres', module: 'postgres-k8s' });
    await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên đi kèm' }));
    await user.selectOptions(field('Loại tài nguyên đi kèm 1'), 'vpc');
    await user.selectOptions(field('Phạm vi tài nguyên đi kèm 1'), '@infra');
    expect(field('Descriptor tài nguyên đi kèm 1')).toHaveValue('vpc.default#@infra');
    expect(screen.getByText(/Cờ tắt không thêm cạnh phụ thuộc/)).toBeInTheDocument();
    await user.click(field('Tài nguyên đi kèm 1: vpc chờ postgres tạo xong (is_dependent)'));
    await user.click(field('Tài nguyên đi kèm 1: các tài nguyên đang dùng postgres cũng phải chờ vpc (match_dependents)'));
    await user.click(screen.getByRole('button', { name: 'Nâng cao: JSON nâng cao' }));
    expect(JSON.parse((field('Quy tắc tạo tài nguyên liên quan (JSON object)') as HTMLTextAreaElement).value)).toEqual({ 'vpc.default#@infra': { is_dependent: true, match_dependents: true } });
    await user.click(screen.getByRole('button', { name: 'Nâng cao: biểu mẫu' }));
    expect(field('Descriptor tài nguyên đi kèm 1')).toHaveValue('vpc.default#@infra');
    expect(field('Tài nguyên đi kèm 1: vpc chờ postgres tạo xong (is_dependent)')).toBeChecked();
    await submitDefinition(user);
    await screen.findByText(/^Đã đăng ký cấu hình tài nguyên/);
    expect(posted[0]?.provision).toEqual({ 'vpc.default#@infra': { is_dependent: true, match_dependents: true } });
  });
});
