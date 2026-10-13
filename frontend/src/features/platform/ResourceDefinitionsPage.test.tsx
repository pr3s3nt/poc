import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { capabilityFixture } from '../../test/capabilities';
import { typeFixture } from '../../test/types';
import { backend, completeDefinition, defaultApplications, field, submitDefinition, type Backend } from '../../test/definitionKit';

const pinOf = (module: string) => {
  const capability = capabilityFixture.find((candidate) => candidate.driverType === 'terraform' && candidate.module === module);
  if (!capability?.source) throw new Error(`fixture without ${module}`);
  const { sourceId, moduleId, revision, subdirectory, contentSha256 } = capability.source;
  return { source_id: sourceId, module_id: moduleId, revision, subdirectory, content_sha256: contentSha256 };
};
const values = (body?: Record<string, unknown>) => (body?.driverInputs as { values: Record<string, unknown> }).values;
const infra = "${resources['vpc.default#@infra'].outputs";

async function ready() {
  render(<ResourceDefinitionsPage />);
  await screen.findByText('Chưa có cấu hình tài nguyên nào.');
  await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
}
async function overrideValue(user: ReturnType<typeof userEvent.setup>, name: string, value: string) {
  await user.click(field(`Ghi đè ${name}`));
  await user.clear(field(`Giá trị ${name}`));
  await user.click(field(`Giá trị ${name}`));
  await user.paste(value);
}

afterEach(() => vi.unstubAllGlobals());

describe('UC-03 Definition form over verified registry pins (I04-07)', () => {
  it('offers only registry-verified pins, shows the full pin read-only and has no source, URL or digest input', async () => {
    const user = userEvent.setup();
    backend();
    await ready();
    await user.selectOptions(field('Loại tài nguyên'), 'vpc');
    const select = field('Module đã kiểm chứng');
    expect(select).toBeDisabled();
    const pin = pinOf('vpc');
    expect(within(select).getByRole('option', { name: `vpc · ${pin.revision} · sha256:${pin.content_sha256.slice(0, 12)}` })).toBeInTheDocument();
    const details = document.querySelector('dl[aria-label="Pin module đã kiểm chứng"]') as HTMLElement;
    for (const value of new Set([pin.source_id, pin.module_id, pin.revision, pin.subdirectory, pin.content_sha256])) expect(within(details).getAllByText(value).length).toBeGreaterThan(0);
    expect(within(details).getByText(pin.content_sha256)).toBeInTheDocument();
    expect(screen.queryByLabelText(/URL|digest|SHA|revision|Nguồn module|source_id/i)).not.toBeInTheDocument();
    // The legacy static Kubernetes pairs of the metadata are never a choice.
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    expect(within(field('Module đã kiểm chứng')).getAllByRole('option').map((option) => option.textContent?.split(' · ')[0])).toEqual(['Chọn module và phiên bản', 'aurora', 'postgres-k8s']);
    expect(screen.queryByRole('option', { name: /Tạo trong cluster \(Kubernetes\)/ })).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Cách tạo')).not.toBeInTheDocument();
  });

  it('registers VPC with the exact verified pin, explicit output mapping and the Environment AWS binding', async () => {
    const user = userEvent.setup();
    const { posted, gets } = backend();
    await ready();
    await completeDefinition(user, { id: 'vpc-aws', type: 'vpc' });
    expect(field('Phạm vi triển khai')).toHaveValue('aws-eks');
    expect(field('Phạm vi triển khai')).toBeDisabled();
    expect(screen.getByText(/dùng Kết nối AWS của Môi trường khi Xem trước hoặc Triển khai/)).toBeInTheDocument();
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên vpc-aws.');
    expect(posted).toEqual([{ key: 'vpc-aws', resourceType: 'vpc', executionProfile: 'aws-eks', driverType: 'terraform', provision: {}, criteria: [{}], driverInputs: { values: {
      source: pinOf('vpc'), variables: { name: '${context.infra.resourceName}' }, input_mapping: {},
      output_mapping: { id: 'id', cidr: 'cidr', subnetIds: 'subnetIds', privateSubnetIds: 'privateSubnetIds' }, provider_bindings: { aws: { target: 'environment' } } } } }]);
    expect(gets.some((url) => url.endsWith('/connections'))).toBe(false);
  });

  it('Aurora inherits the Environment account and region: no Connection selector, no override, private subnets from the shared VPC', async () => {
    const user = userEvent.setup();
    const { posted, gets } = backend();
    await ready();
    await completeDefinition(user, { id: 'aurora', type: 'postgres', module: 'aurora' });
    expect(screen.queryByLabelText('Kết nối AWS')).not.toBeInTheDocument();
    expect(screen.queryByRole('alert', { name: 'Lỗi tải kết nối' })).not.toBeInTheDocument();
    expect(screen.getByText(/cấu hình không chọn Kết nối AWS riêng, kể cả Aurora/)).toBeInTheDocument();
    // database/username arrive as Type params: they need no Definition value and outrank any fallback or module default.
    expect(screen.getAllByText(/Nguồn: tham số "(database|username)" của Loại tài nguyên \(qua ánh xạ đầu vào\)/)).toHaveLength(2);
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/Cần nhập trước khi đăng ký: engine_version, max_capacity, min_capacity/);
    expect(posted).toEqual([]);
    await overrideValue(user, 'engine_version', '16.4');
    await overrideValue(user, 'min_capacity', '0.5');
    await overrideValue(user, 'max_capacity', '2');
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên aurora.');
    expect(posted[0]).toMatchObject({ resourceType: 'postgres', executionProfile: 'aws-eks', driverType: 'terraform' });
    expect(posted[0]).not.toHaveProperty('connectionKey');
    expect(values(posted[0])).toEqual({
      source: pinOf('aurora'),
      variables: { engine_version: '16.4', min_capacity: 0.5, max_capacity: 2, name: '${context.infra.resourceName}', vpc_id: `${infra}.id}`, subnet_ids: `${infra}.subnetIds}`, private_subnet_ids: `${infra}.privateSubnetIds}` },
      input_mapping: { database: { param: 'database' }, username: { param: 'username' } },
      output_mapping: { host: 'host', port: 'port', database: 'database', username: 'username', password: 'password' },
      provider_bindings: { aws: { target: 'environment' } },
    });
    expect(gets.some((url) => url.endsWith('/connections'))).toBe(false);
  });

  it('EKS takes an explicit IPv4 /32 allowlist from the author: no default, no wide CIDR, no executor address', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'eks', type: 'k8s-cluster' });
    // Executor-owned names have no control; the allowlist is required with no default or preset.
    for (const name of ['region', 'tags']) expect(screen.queryByLabelText(`Ghi đè ${name}`)).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Dùng preset cho public_access_cidrs' })).not.toBeInTheDocument();
    expect(field('Ghi đè public_access_cidrs')).not.toBeChecked();
    await overrideValue(user, 'kubernetes_version', '1.31');
    await overrideValue(user, 'node_ami_type', 'AL2023_x86_64_STANDARD');
    await overrideValue(user, 'node_capacity_type', 'ON_DEMAND');
    await overrideValue(user, 'node_disk_size', '20');
    await overrideValue(user, 'node_instance_type', 't3.medium');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/Cần nhập trước khi đăng ký: public_access_cidrs/);
    expect(posted).toEqual([]);
    await overrideValue(user, 'public_access_cidrs', '["203.0.113.10/32"]');
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên eks.');
    expect(values(posted[0]).variables).toMatchObject({ public_access_cidrs: ['203.0.113.10/32'], subnet_ids: `${infra}.subnetIds}` });
    expect(values(posted[0]).provider_bindings).toEqual({ aws: { target: 'environment' } });
    expect(JSON.stringify(posted[0])).not.toMatch(/0\.0\.0\.0|executor/);
  });

  it('Kubernetes modules need an explicit scope and target; EKS targets come from the published cluster classes', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình'), 'pg-k8s');
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    await user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: /^postgres-k8s ·/ }));
    expect(field('Phạm vi triển khai')).toHaveValue('__unset');
    await user.click(screen.getByRole('button', { name: 'Ghép đầu vào theo tên trùng' }));
    await user.click(screen.getByRole('button', { name: 'Ghép đầu ra theo tên trùng' }));
    await user.click(screen.getByRole('button', { name: 'Dùng preset cho namespace' }));
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('Chọn phạm vi triển khai của cấu hình.');
    await user.selectOptions(field('Phạm vi triển khai'), 'aws-eks');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/Chọn nơi provider kubernetes chạy/);
    expect(posted).toEqual([]);
    const target = field('Cluster đích của provider kubernetes');
    expect(target).toHaveValue('');
    await user.selectOptions(target, 'resource');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/Chọn class cluster EKS/);
    const classes = within(field('Class cluster EKS của provider kubernetes')).getAllByRole('option').map((option) => option.getAttribute('value'));
    expect(classes).toEqual(['', 'eks']);
    await user.selectOptions(field('Class cluster EKS của provider kubernetes'), 'eks');
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên pg-k8s.');
    expect(posted[0]).toMatchObject({ executionProfile: 'aws-eks', driverType: 'terraform' });
    expect(values(posted[0]).provider_bindings).toEqual({ kubernetes: { target: 'resource', resource_ref: 'k8s-cluster.eks#environments.@app.@env' } });
    expect(values(posted[0]).source).toEqual(pinOf('postgres-k8s'));
  });

  it('registers Redis from either verified module, flags the secret output and keeps a shared scope as an empty profile', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình'), 'redis-shared');
    await user.selectOptions(field('Loại tài nguyên'), 'redis');
    await user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: /^redis-k8s ·/ }));
    await user.selectOptions(field('Phạm vi triển khai'), '');
    expect(screen.getByText(/Dùng chung được xét ở mọi phạm vi triển khai/)).toBeInTheDocument();
    expect(screen.getByText(/Output password \(string, bắt buộc, bí mật\)/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Ghép đầu ra theo tên trùng' }));
    await user.selectOptions(field('Cluster đích của provider kubernetes'), 'environment');
    await user.click(screen.getByRole('button', { name: 'Dùng preset cho namespace' }));
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('redis-shared');
    expect(posted[0]).toMatchObject({ executionProfile: '', resourceType: 'redis' });
    expect(values(posted[0]).output_mapping).toEqual({ host: 'host', port: 'port', username: 'username', password: 'password', tls: 'tls' });
    // The cloud-managed module is a second, separate choice.
    await user.type(field('ID cấu hình'), 'redis-cache');
    await user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: /^elasticache ·/ }));
    expect(field('Phạm vi triển khai')).toHaveValue('aws-eks');
    expect(screen.queryByLabelText('Cluster đích của provider kubernetes')).not.toBeInTheDocument();
  });

  it('never maps by name on its own and refuses an incomplete contract before any POST', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình'), 'aurora-unmapped');
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    await user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: /^aurora ·/ }));
    expect(field('Biến module nhận tham số database')).toHaveValue('');
    expect(field('Biến module nhận tham số username')).toHaveValue('');
    expect(field('Output module cung cấp host')).toHaveValue('');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('Chọn biến module nhận tham số "database" của Loại tài nguyên.');
    await user.selectOptions(field('Biến module nhận tham số database'), 'database');
    // A variable already taken by another parameter is not offered twice.
    expect(within(field('Biến module nhận tham số username')).queryByRole('option', { name: /^database/ })).not.toBeInTheDocument();
    await user.selectOptions(field('Biến module nhận tham số username'), 'username');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('Chọn output module cung cấp output bắt buộc "host".');
    expect(posted).toEqual([]);
    // The Type's number output only offers outputs that can carry a number.
    const portOptions = within(field('Output module cung cấp port')).getAllByRole('option').map((option) => option.getAttribute('value'));
    expect(portOptions).toContain('port');
  });

  it('keeps the mapping, bindings and values lossless between typed and advanced JSON, and stays advanced for what controls cannot hold', async () => {
    const user = userEvent.setup();
    backend();
    await ready();
    await completeDefinition(user, { id: 'pg-roundtrip', type: 'postgres', module: 'postgres-k8s', profile: 'aws-eks', binding: { cluster: 'eks' } });
    await user.click(screen.getByRole('button', { name: 'Ánh xạ: JSON nâng cao' }));
    const expected = {
      input_mapping: { database: { param: 'database' }, username: { param: 'username' } },
      output_mapping: { host: 'host', port: 'port', database: 'database', username: 'username', password: 'password' },
      provider_bindings: { kubernetes: { target: 'resource', resource_ref: 'k8s-cluster.eks#environments.@app.@env' } },
    };
    expect(JSON.parse((field('Ánh xạ module JSON') as HTMLTextAreaElement).value)).toEqual(expected);
    await user.click(screen.getByRole('button', { name: 'Ánh xạ: biểu mẫu' }));
    expect(field('Biến module nhận tham số database')).toHaveValue('database');
    expect(field('Class cluster EKS của provider kubernetes')).toHaveValue('eks');
    // A cluster reference outside the published classes cannot be shown by the controls.
    await user.click(screen.getByRole('button', { name: 'Ánh xạ: JSON nâng cao' }));
    const box = field('Ánh xạ module JSON');
    const custom = JSON.stringify({ ...expected, provider_bindings: { kubernetes: { target: 'resource', resource_ref: 'k8s-cluster.other#environments.@app.@env' } } });
    fireEvent.change(box, { target: { value: custom } });
    await user.click(screen.getByRole('button', { name: 'Ánh xạ: biểu mẫu' }));
    expect(screen.getByText(/không biểu diễn an toàn bằng control/)).toBeInTheDocument();
    expect(field('Ánh xạ module JSON')).toHaveValue(custom);
    // Unknown module variable: kept in the JSON, never dropped.
    fireEvent.change(field('Ánh xạ module JSON'), { target: { value: JSON.stringify({ ...expected, input_mapping: { missing_var: { param: 'database' } } }) } });
    await user.click(screen.getByRole('button', { name: 'Ánh xạ: biểu mẫu' }));
    expect(screen.getByText(/Biến "missing_var" không có trong module đã chọn/)).toBeInTheDocument();
    // Invalid JSON never converts and never loses the text.
    fireEvent.change(field('Ánh xạ module JSON'), { target: { value: '{bad' } });
    await user.click(screen.getByRole('button', { name: 'Ánh xạ: biểu mẫu' }));
    expect(screen.getAllByText(/phải là JSON hợp lệ/).length).toBeGreaterThan(0);
    expect(field('Ánh xạ module JSON')).toHaveValue('{bad');
  });

  it('submits advanced mapping JSON as typed and rejects unknown keys or a missing provider_bindings locally', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'ns-json', type: 'k8s-namespace' });
    await user.click(screen.getByRole('button', { name: 'Ánh xạ: JSON nâng cao' }));
    fireEvent.change(field('Ánh xạ module JSON'), { target: { value: '{"output_mapping":{"name":"name"},"provider_bindings":{"kubernetes":{"target":"environment"}},"region":"x"}' } });
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('không nhận "region"');
    fireEvent.change(field('Ánh xạ module JSON'), { target: { value: '{"output_mapping":{"name":"name"}}' } });
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('provider_bindings là một JSON object');
    expect(posted).toEqual([]);
    fireEvent.change(field('Ánh xạ module JSON'), { target: { value: '{"output_mapping":{"name":"name"},"provider_bindings":{"kubernetes":{"target":"environment"}}}' } });
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('ns-json');
    expect(values(posted[0])).toMatchObject({ input_mapping: {}, output_mapping: { name: 'name' }, provider_bindings: { kubernetes: { target: 'environment' } } });
  });

  it('sends the shared scope as an empty profile and omits it from the AWS-only choices', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'ns-shared', type: 'k8s-namespace', profile: '' });
    expect(screen.getByText(/Dùng chung được xét ở mọi phạm vi triển khai nhưng vẫn phải thỏa điều kiện áp dụng/)).toBeInTheDocument();
    await submitDefinition(user);
    await screen.findByRole('status');
    expect(posted[0]).toMatchObject({ executionProfile: '' });
  });

  it('hides workload, disables types without a verified module and never offers a source of its own', async () => {
    const user = userEvent.setup();
    backend({ types: [...typeFixture, { key: 'cache', inputs: [], outputs: [], classes: [] }] });
    await ready();
    const select = field('Loại tài nguyên');
    expect(within(select).queryByRole('option', { name: 'workload' })).not.toBeInTheDocument();
    const unsupported = within(select).getByRole('option', { name: 'cache (chưa có module đã kiểm chứng)' });
    expect(unsupported).toBeDisabled();
    await user.selectOptions(select, 'cache').catch(() => undefined);
    expect(select).toHaveValue('');
    expect(screen.queryByRole('option', { name: /score-k8s|existing-cluster/ })).not.toBeInTheDocument();
  });

  it('shows no supported choice at all when the metadata only holds legacy static pairs', async () => {
    backend({ capabilities: capabilityFixture.filter((capability) => capability.driverType === 'kubernetes') });
    render(<ResourceDefinitionsPage />);
    await screen.findByText('Chưa có cấu hình tài nguyên nào.');
    await screen.findByRole('option', { name: 'postgres (chưa có module đã kiểm chứng)' });
    expect(screen.getByRole('option', { name: 'k8s-namespace (chưa có module đã kiểm chứng)' })).toBeDisabled();
    expect(field('Module đã kiểm chứng')).toBeDisabled();
  });

  it('keeps a separate draft per verified pin and resets only the submitted pin', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình'), 'pin-drafts');
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    const pick = (module: string) => user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: new RegExp(`^${module} ·`) }));
    await pick('postgres-k8s');
    await user.selectOptions(field('Phạm vi triển khai'), 'internal-k8s');
    await user.click(screen.getByRole('button', { name: 'Ghép đầu vào theo tên trùng' }));
    await user.click(screen.getByRole('button', { name: 'Ghép đầu ra theo tên trùng' }));
    await user.selectOptions(field('Cluster đích của provider kubernetes'), 'environment');
    await overrideValue(user, 'storage', '9Gi');
    await user.click(screen.getByRole('button', { name: 'Dùng preset cho namespace' }));
    await pick('aurora');
    await overrideValue(user, 'min_capacity', '2');
    expect(screen.queryByLabelText('Ghi đè storage')).not.toBeInTheDocument();
    await pick('postgres-k8s');
    expect(field('Giá trị storage')).toHaveValue('9Gi');
    expect(field('Phạm vi triển khai')).toHaveValue('internal-k8s');
    expect(field('Biến module nhận tham số database')).toHaveValue('database');
    await pick('aurora');
    expect(field('Giá trị min_capacity')).toHaveValue('2');
    await pick('postgres-k8s');
    await submitDefinition(user);
    await screen.findByRole('status');
    expect(values(posted[0]).variables).toEqual({ storage: '9Gi', namespace: "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}" });
    expect(field('Ghi đè storage')).not.toBeChecked();
    await pick('aurora');
    expect(field('Giá trị min_capacity')).toHaveValue('2');
  });

  it('locks types and registration while metadata loads or fails, with a GET-only retry that keeps edits', async () => {
    const user = userEvent.setup();
    let fail = true;
    const { posted } = backend({ capabilityLists: [() => (fail ? Response.json({ code: 'INTERNAL_ERROR' }, { status: 500 }) : Response.json({ capabilities: capabilityFixture }))] });
    render(<ResourceDefinitionsPage />);
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải metadata module' });
    await screen.findByRole('option', { name: 'vpc (chưa xác nhận hỗ trợ)' });
    expect(screen.getByRole('option', { name: 'vpc (chưa xác nhận hỗ trợ)' })).toBeDisabled();
    expect(screen.queryByText(/chưa có module đã kiểm chứng/)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: submitName })).toBeDisabled();
    await user.type(field('ID cấu hình'), 'kept-id');
    fail = false;
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
    expect(field('ID cấu hình')).toHaveValue('kept-id');
    expect(posted).toEqual([]);
  });
});

const submitName = 'Đăng ký cấu hình tài nguyên';

describe('UC-03 Definition form states and errors', () => {
  it('lists definitions in Vietnamese, hides workload ones and marks existing-cluster as system read-only', async () => {
    backend({ definitions: [
      { key: 'pg-fast', resourceType: 'postgres', executionProfile: 'internal-k8s', driverType: 'terraform', criteria: [{}] },
      { key: 'workload-score', resourceType: 'workload', executionProfile: 'internal-k8s', driverType: 'score-k8s', criteria: [{}] },
      { key: 'cluster-existing', resourceType: 'k8s-cluster', executionProfile: 'internal-k8s', driverType: 'existing-cluster', criteria: [{}, {}] },
      { key: 'ns-shared', resourceType: 'k8s-namespace', executionProfile: '', driverType: 'terraform', criteria: [{}] },
    ] });
    render(<ResourceDefinitionsPage />);
    expect(await screen.findByText('pg-fast')).toBeInTheDocument();
    expect(screen.queryByText('workload-score')).not.toBeInTheDocument();
    expect(screen.getByText('pg-fast').nextElementSibling).toHaveTextContent('postgres · Cluster nội bộ · Terraform (module đã kiểm chứng) · 1 điều kiện áp dụng');
    expect(screen.getByText('cluster-existing').nextElementSibling).toHaveTextContent(/2 điều kiện áp dụng · Hệ thống \(chỉ đọc\)/);
    expect(screen.getByText('ns-shared').nextElementSibling).toHaveTextContent('Dùng chung');
    expect(screen.queryByRole('button', { name: /Sửa|Xóa cấu hình/ })).not.toBeInTheDocument();
  });

  it('rejects a blank or malformed ID and invalid JSON locally without any POST', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-namespace');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('Nhập ID cấu hình.');
    await user.type(field('ID cấu hình'), 'Bad_ID ');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/chỉ gồm chữ thường/);
    expect(field('ID cấu hình')).toHaveValue('Bad_ID ');
    expect(field('ID cấu hình')).toBeInvalid();
    await user.clear(field('ID cấu hình'));
    await user.type(field('ID cấu hình'), 'ns-ok');
    await user.click(screen.getByRole('button', { name: 'Tham số: JSON nâng cao' }));
    fireEvent.change(field('Tham số cấu hình (JSON object)'), { target: { value: '{bad' } });
    await user.click(screen.getByRole('button', { name: 'Ánh xạ: JSON nâng cao' }));
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('Chọn phạm vi triển khai');
    expect(posted).toHaveLength(0);
  });

  it('keeps the whole form after a duplicate 409 and shows a Vietnamese message, never the raw one', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ post: () => Response.json({ error: 'catalog: duplicate: resource definition "ns-dup"', code: 'DUPLICATE' }, { status: 409 }) });
    await ready();
    await completeDefinition(user, { id: 'ns-dup', type: 'k8s-namespace' });
    await user.click(screen.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
    await user.type(field('Điều kiện 1 Class'), 'fast');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('ID cấu hình đã tồn tại. Hãy đổi ID rồi thử lại.');
    expect(screen.queryByText(/catalog: duplicate/)).not.toBeInTheDocument();
    expect(field('ID cấu hình')).toHaveValue('ns-dup');
    expect(field('Loại tài nguyên')).toHaveValue('k8s-namespace');
    expect(field('Giá trị name')).toHaveValue('${context.env.namespace}');
    expect(field('Output module cung cấp name')).toHaveValue('name');
    expect(field('Điều kiện 1 Class')).toHaveValue('fast');
    expect(posted).toHaveLength(1);
  });

  it.each([[400, /từ chối cấu hình/], [403, /không có quyền/], [500, /Máy chủ gặp lỗi/]])('maps status %i to a safe Vietnamese message and keeps the form', async (status, message) => {
    const user = userEvent.setup();
    const { posted } = backend({ post: () => Response.json({ error: 'driverInputs.values.variables.storage must be string' }, { status }) });
    await ready();
    await completeDefinition(user, { id: 'ns-fail', type: 'k8s-namespace' });
    await submitDefinition(user);
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(message);
    expect(alert).not.toHaveTextContent('must be string');
    expect(field('ID cấu hình')).toHaveValue('ns-fail');
    expect(posted).toHaveLength(1);
  });

  it('shows the field of a typed registration error without echoing a value', async () => {
    const user = userEvent.setup();
    backend({ post: () => Response.json({ error: 'secret-ish raw text', code: 'INVALID_REQUEST', field: 'definition.driverInputs.values.source' }, { status: 400 }) });
    await ready();
    await completeDefinition(user, { id: 'ns-source', type: 'k8s-namespace' });
    await submitDefinition(user);
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Trường liên quan: Nguồn module đã kiểm chứng');
    expect(alert).not.toHaveTextContent('secret-ish raw text');
  });

  it('reports an unknown outcome after a network failure and never replays the POST', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ post: () => { throw new TypeError('network'); } });
    await ready();
    await completeDefinition(user, { id: 'ns-net', type: 'k8s-namespace' });
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/chưa rõ kết quả/);
    expect(posted).toHaveLength(1);
  });

  it('keeps metadata and a Vietnamese retry when Resource Types fail to load', async () => {
    const user = userEvent.setup();
    const { state } = backend({ typesFail: true });
    render(<ResourceDefinitionsPage />);
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải loại tài nguyên' });
    await user.type(field('ID cấu hình'), 'ns-keep');
    expect(screen.getByRole('button', { name: submitName })).toBeDisabled();
    state.typesFail = false;
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await screen.findByRole('option', { name: 'vpc' });
    expect(field('ID cấu hình')).toHaveValue('ns-keep');
  });

  it('reports a committed registration when the reload fails, resets the form and retries only the list', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ definitionLists: [() => Response.json({ resourceDefinitions: [] }), () => Response.json({ error: 'x' }, { status: 500 })] });
    await ready();
    await completeDefinition(user, { id: 'ns-new', type: 'k8s-namespace' });
    await submitDefinition(user);
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
    const { posted } = backend({ types: [...typeFixture, { key: 'constructor', inputs: [], outputs: [], classes: [] }, { key: 'toString', inputs: [], outputs: [], classes: [] }], definitions: [{ key: 'odd', resourceType: 'constructor', executionProfile: 'constructor', driverType: 'toString', criteria: [{}] }] });
    render(<ResourceDefinitionsPage />);
    expect(await screen.findByText('odd')).toBeInTheDocument();
    expect(screen.getByText('odd').nextElementSibling).toHaveTextContent('constructor · constructor · toString · 1 điều kiện áp dụng');
    await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
    expect(screen.getByRole('option', { name: 'constructor (chưa có module đã kiểm chứng)' })).toBeDisabled();
    expect(screen.getByRole('option', { name: 'toString (chưa có module đã kiểm chứng)' })).toBeDisabled();
    await user.type(field('ID cấu hình'), 'odd-new');
    fireEvent.change(field('Loại tài nguyên'), { target: { value: 'constructor' } });
    expect(field('Module đã kiểm chứng')).toBeDisabled();
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('Chọn một Loại tài nguyên và module đã kiểm chứng.');
    expect(posted).toHaveLength(0);
  });

  describe('resourceType prefill from the Resource Types CTA (T04)', () => {
    const typeSelect = () => field('Loại tài nguyên') as HTMLSelectElement;
    const extra = [{ key: 'cache', inputs: [], outputs: [], classes: [] }, { key: 'constructor', inputs: [], outputs: [], classes: [] }, { key: 'toString', inputs: [], outputs: [], classes: [] }];

    it.each(['vpc', 'k8s-cluster', 'k8s-namespace', 'postgres', 'redis'])('selects %s after the catalog loaded, without any POST', async (type) => {
      const { posted } = backend();
      render(<ResourceDefinitionsPage initialResourceType={type} />);
      await screen.findByRole('option', { name: 'vpc' });
      await waitFor(() => expect(typeSelect()).toHaveValue(type));
      expect(posted).toEqual([]);
    });

    it.each([['', 'missing'], ['workload', 'workload'], ['cache', 'unsupported custom type'], ['constructor', 'inherited property'], ['toString', 'inherited property'], ['VPC', 'wrong case'], ['unknown-type', 'unknown key'], ['vpc ', 'untrimmed key']])('leaves no selection for query %j (%s)', async (type) => {
      const { posted } = backend({ types: [...typeFixture, ...extra] });
      render(<ResourceDefinitionsPage initialResourceType={type} />);
      await screen.findByRole('option', { name: 'vpc' });
      await new Promise((done) => setTimeout(done, 30));
      expect(typeSelect()).toHaveValue('');
      expect(posted).toEqual([]);
    });

    it('does not select a key the catalog does not contain', async () => {
      backend({ types: typeFixture.filter((type) => type.key === 'k8s-namespace') });
      render(<ResourceDefinitionsPage initialResourceType="vpc" />);
      await screen.findByRole('option', { name: 'k8s-namespace' });
      await new Promise((done) => setTimeout(done, 30));
      expect(typeSelect()).toHaveValue('');
    });

    it('waits for a late catalog response and then selects, without clobbering the typed ID', async () => {
      const user = userEvent.setup();
      let release: () => void = () => undefined;
      const gate = new Promise<void>((done) => { release = done; });
      vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
        const url = String(input);
        if (url.endsWith('/resource-types')) { await gate; return Response.json({ resourceTypes: typeFixture }); }
        if (url.endsWith('/resource-definition-capabilities')) return Response.json({ capabilities: capabilityFixture });
        if (url.endsWith('/applications')) return Response.json({ applications: defaultApplications });
        return Response.json({ resourceDefinitions: [] });
      }));
      render(<ResourceDefinitionsPage initialResourceType="postgres" />);
      await screen.findByText('Chưa có cấu hình tài nguyên nào.');
      expect(typeSelect()).toHaveValue('');
      await user.type(field('ID cấu hình'), 'my-pg');
      release();
      await waitFor(() => expect(typeSelect()).toHaveValue('postgres'));
      expect(field('ID cấu hình')).toHaveValue('my-pg');
    });

    it('applies the prefill once: a later reload or a user choice is never overwritten', async () => {
      const user = userEvent.setup();
      backend();
      render(<ResourceDefinitionsPage initialResourceType="vpc" />);
      await waitFor(() => expect(typeSelect()).toHaveValue('vpc'));
      await user.selectOptions(typeSelect(), 'k8s-namespace');
      expect(typeSelect()).toHaveValue('k8s-namespace');
      await user.selectOptions(typeSelect(), '');
      expect(typeSelect()).toHaveValue('');
      await new Promise((done) => setTimeout(done, 30));
      expect(typeSelect()).toHaveValue('');
    });

    it('prefills after a failed load is retried and keeps the form data', async () => {
      const user = userEvent.setup();
      const { state } = backend({ typesFail: true });
      render(<ResourceDefinitionsPage initialResourceType="k8s-cluster" />);
      const alert = await screen.findByRole('alert', { name: 'Lỗi tải loại tài nguyên' });
      await user.type(field('ID cấu hình'), 'cluster-a');
      expect(typeSelect()).toHaveValue('');
      state.typesFail = false;
      await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
      await waitFor(() => expect(typeSelect()).toHaveValue('k8s-cluster'));
      expect(field('ID cấu hình')).toHaveValue('cluster-a');
    });

    it('does not prefill when the catalog never loads', async () => {
      backend({ typesFail: true });
      render(<ResourceDefinitionsPage initialResourceType="vpc" />);
      await screen.findByRole('alert', { name: 'Lỗi tải loại tài nguyên' });
      expect(typeSelect()).toHaveValue('');
    });
  });

  describe('criteria editor (T02B)', () => {
    const radio = (name: RegExp) => screen.getByRole('radio', { name });
    async function start(user: ReturnType<typeof userEvent.setup>, id: string, options: Backend = {}) {
      const state = backend(options);
      render(<ResourceDefinitionsPage />);
      await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
      await completeDefinition(user, { id, type: 'k8s-namespace' });
      return state;
    }

    it('posts [{}], env_type, real app/env IDs and several advanced rows without changing the rest of the payload', async () => {
      const user = userEvent.setup();
      const { posted } = await start(user, 'ns-modes');
      await submitDefinition(user);
      await screen.findByRole('status');
      expect(posted[0]?.criteria).toEqual([{}]);
      await completeDefinition(user, { id: 'ns-env', type: 'k8s-namespace' });
      await user.click(radio(/Theo loại môi trường/));
      await user.type(field('Loại môi trường'), 'staging');
      await submitDefinition(user);
      await waitFor(() => expect(posted).toHaveLength(2));
      expect(posted[1]).toMatchObject({ key: 'ns-env', criteria: [{ env_type: 'staging' }], executionProfile: 'internal-k8s' });
      await screen.findByText(/Đã đăng ký cấu hình tài nguyên ns-env/);
      expect(radio(/Mọi nơi/)).toBeChecked(); // success resets to the wildcard
      await completeDefinition(user, { id: 'ns-app', type: 'k8s-namespace' });
      await user.click(radio(/Theo ứng dụng/));
      await waitFor(() => expect(within(field('Ứng dụng')).getByRole('option', { name: 'Cửa hàng (shop)' })).toBeInTheDocument());
      await user.selectOptions(field('Ứng dụng'), 'shop');
      await user.selectOptions(field('Môi trường (tùy chọn)'), 'production');
      await submitDefinition(user);
      await waitFor(() => expect(posted).toHaveLength(3));
      expect(posted[2]?.criteria).toEqual([{ app_id: 'shop', env_id: 'production' }]);
      await screen.findByText(/Đã đăng ký cấu hình tài nguyên ns-app/);
      await completeDefinition(user, { id: 'ns-adv', type: 'k8s-namespace' });
      await user.click(radio(/Tùy chỉnh nâng cao/));
      await user.type(field('Điều kiện 1 Class'), 'fast');
      await user.click(screen.getByRole('button', { name: '+ Thêm điều kiện' }));
      await user.type(field('Điều kiện 2 ID tài nguyên'), 'db');
      await submitDefinition(user);
      await waitFor(() => expect(posted).toHaveLength(4));
      expect(posted[3]?.criteria).toEqual([{ class: 'fast' }, { res_id: 'db' }]);
    });

    it('refuses to submit an incomplete simple mode instead of registering a wildcard', async () => {
      const user = userEvent.setup();
      const { posted } = await start(user, 'ns-incomplete');
      await user.click(radio(/Theo loại môi trường/));
      await submitDefinition(user);
      expect(await screen.findByRole('alert')).toHaveTextContent('Nhập loại môi trường');
      await user.click(radio(/Theo ứng dụng/));
      await submitDefinition(user);
      expect(await screen.findByRole('alert')).toHaveTextContent('Chọn ứng dụng');
      expect(posted).toHaveLength(0);
    });

    it('keeps criteria after a failed submit and offers the copy source from the registered list', async () => {
      const user = userEvent.setup();
      const { posted } = await start(user, 'ns-copy', { definitions: [{ key: 'pg-adv', resourceType: 'postgres', executionProfile: 'internal-k8s', driverType: 'terraform', criteria: [{ env_type: 'staging', class: 'fast' }, { app_id: 'shop' }] }], post: () => Response.json({ error: 'x' }, { status: 500 }) });
      await waitFor(() => expect(within(field('Sao chép điều kiện từ cấu hình khác')).getByRole('option', { name: 'pg-adv (postgres)' })).toBeInTheDocument());
      await user.selectOptions(field('Sao chép điều kiện từ cấu hình khác'), 'pg-adv');
      await user.click(screen.getByRole('button', { name: 'Sao chép điều kiện' }));
      expect(radio(/Tùy chỉnh nâng cao/)).toBeChecked();
      await submitDefinition(user);
      expect(await screen.findByRole('alert')).toHaveTextContent('Máy chủ gặp lỗi');
      expect(radio(/Tùy chỉnh nâng cao/)).toBeChecked();
      expect(field('Điều kiện 1 Class')).toHaveValue('fast');
      expect(field('Điều kiện 2 ID ứng dụng')).toHaveValue('shop');
      expect(posted[0]?.criteria).toEqual([{ env_type: 'staging', class: 'fast' }, { app_id: 'shop' }]);
    });

    it('does not let a late or failed applications reply change edits, and retries only that list', async () => {
      const user = userEvent.setup();
      let release: (response: Response) => void = () => undefined;
      const { gets, posted } = backend({ applicationLists: [() => new Promise<Response>((resolve) => { release = resolve; })] });
      render(<ResourceDefinitionsPage />);
      await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
      await user.click(radio(/Theo ứng dụng/));
      expect(within(field('Ứng dụng')).getByRole('option', { name: 'Đang tải ứng dụng…' })).toBeInTheDocument();
      await user.click(radio(/Tùy chỉnh nâng cao/));
      await user.type(field('Điều kiện 1 Class'), 'fast');
      release(Response.json({ error: 'late' }, { status: 500 }));
      const alert = await screen.findByRole('alert', { name: 'Lỗi tải ứng dụng' });
      expect(field('Điều kiện 1 Class')).toHaveValue('fast');
      await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
      await waitFor(() => expect(screen.queryByRole('alert', { name: 'Lỗi tải ứng dụng' })).not.toBeInTheDocument());
      expect(gets.filter((url) => url.endsWith('/applications'))).toHaveLength(2);
      expect(field('Điều kiện 1 Class')).toHaveValue('fast');
      expect(posted).toHaveLength(0);
    });

    it('keeps typed criteria and the chosen mode when the applications reply arrives late and succeeds', async () => {
      const user = userEvent.setup();
      let release: (response: Response) => void = () => undefined;
      const { posted } = backend({ applicationLists: [() => new Promise<Response>((resolve) => { release = resolve; })] });
      render(<ResourceDefinitionsPage />);
      await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
      await completeDefinition(user, { id: 'ns-late', type: 'k8s-namespace' });
      await user.click(radio(/Tùy chỉnh nâng cao/));
      await user.type(field('Điều kiện 1 ID ứng dụng'), 'typed-before');
      await user.click(screen.getByRole('button', { name: '+ Thêm điều kiện' }));
      await user.type(field('Điều kiện 2 Class'), 'fast');
      release(Response.json({ applications: defaultApplications }));
      await user.click(radio(/Theo ứng dụng/)).catch(() => undefined);
      // Advanced rows are not representable as one app row: the late list neither applied nor dropped anything.
      expect(screen.getByRole('group', { name: 'Xác nhận thay đổi điều kiện' })).toBeInTheDocument();
      await user.click(screen.getByRole('button', { name: 'Giữ điều kiện nâng cao' }));
      expect(field('Điều kiện 1 ID ứng dụng')).toHaveValue('typed-before');
      expect(field('Điều kiện 2 Class')).toHaveValue('fast');
      expect(field('ID cấu hình')).toHaveValue('ns-late');
      await submitDefinition(user);
      await waitFor(() => expect(posted).toHaveLength(1));
      expect(posted[0]?.criteria).toEqual([{ app_id: 'typed-before' }, { class: 'fast' }]);
    });

    it('warns about possible overlap with a same type/profile Definition without calling it a winner or ambiguity, and never posts while previewing', async () => {
      const user = userEvent.setup();
      const { posted, gets } = await start(user, 'ns-overlap', { definitions: [{ key: 'ns-other', resourceType: 'k8s-namespace', executionProfile: 'internal-k8s', driverType: 'terraform', criteria: [{}] }, { key: 'ns-aws', resourceType: 'k8s-namespace', executionProfile: 'aws-eks', driverType: 'terraform', criteria: [{}] }] });
      const warning = await screen.findByRole('group', { name: 'Cảnh báo nguy cơ chồng lấn' });
      expect(warning).toHaveTextContent('ns-other');
      expect(warning).not.toHaveTextContent('ns-aws');
      expect(warning.textContent).not.toMatch(/ambiguous|thắng|winner|mơ hồ/i);
      const summary = screen.getByRole('group', { name: 'Tóm tắt phạm vi áp dụng' });
      expect(summary).toHaveTextContent('Phạm vi triển khai (executionProfile): Cluster nội bộ');
      await user.selectOptions(field('Ứng dụng mẫu'), 'shop');
      expect(screen.getByRole('group', { name: /Xem trước phạm vi/ })).toHaveTextContent('Cần thêm context');
      expect(posted).toHaveLength(0);
      expect(gets.every((url) => !/preview|match/.test(url))).toBe(true);
    });
  });
});
