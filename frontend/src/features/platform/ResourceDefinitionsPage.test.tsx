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

  it('registers VPC with the exact verified pin, its own Driver Account, a literal region and the account binding', async () => {
    const user = userEvent.setup();
    const { posted, gets } = backend();
    await ready();
    await completeDefinition(user, { id: 'vpc-aws', name: 'VPC AWS chính', type: 'vpc' });
    expect(field('Driver Type')).toHaveValue('Terraform (module đã kiểm chứng)');
    expect(screen.queryByLabelText('Phạm vi triển khai')).not.toBeInTheDocument();
    expect(screen.getByText(/Cấu hình không lấy tài khoản hay vùng từ Môi trường/)).toBeInTheDocument();
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên vpc-aws.');
    expect(posted).toEqual([{ key: 'vpc-aws', name: 'VPC AWS chính', resourceType: 'vpc', driverType: 'terraform', driverAccount: 'aws-account', provision: {}, criteria: [{}], driverInputs: { values: {
      source: pinOf('vpc'), variables: { name: '${context.infra.resourceName}', region: 'us-east-1' }, input_mapping: {},
      output_mapping: { id: 'id', cidr: 'cidr', subnetIds: 'subnetIds', privateSubnetIds: 'privateSubnetIds' }, provider_bindings: { aws: { target: 'account' } } } } }]);
    expect(posted[0]).not.toHaveProperty('executionProfile');
    expect(posted[0]).not.toHaveProperty('connectionKey');
    expect(gets.some((url) => url.endsWith('/connections'))).toBe(false);
  });

  it('offers only compatible READY AWS accounts, with distinct loading, failed and empty states and a retry that keeps the draft', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ accountLists: [() => Response.json({ code: 'INTERNAL_ERROR' }, { status: 500 }), () => Response.json({ accounts: [] })] });
    await ready();
    await user.type(field('ID cấu hình (Definition ID)'), 'vpc-accounts');
    await user.selectOptions(field('Loại tài nguyên'), 'vpc');
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải tài khoản' });
    expect(field('Tài khoản hạ tầng')).toBeDisabled();
    expect(within(field('Tài khoản hạ tầng')).getByRole('option', { name: 'Chưa tải được tài khoản' })).toBeInTheDocument();
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    expect(await screen.findByRole('note', { name: 'Chưa có tài khoản sẵn sàng' })).toHaveTextContent(/Tạo và xác minh tài khoản/);
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('vpc-accounts');
    expect(posted).toEqual([]);
  });

  it('lists only READY AWS accounts of the right type, never an internal or unverified one', async () => {
    const user = userEvent.setup();
    backend();
    await ready();
    await user.selectOptions(field('Loại tài nguyên'), 'vpc');
    const select = field('Tài khoản hạ tầng');
    await waitFor(() => expect(within(select).getByRole('option', { name: /AWS chính/ })).toBeInTheDocument());
    const labels = within(select).getAllByRole('option').map((option) => option.textContent);
    expect(labels).toEqual(['Chọn tài khoản', 'AWS chính (aws-account) · Sẵn sàng · phiên bản 3', 'AWS thứ hai (aws-second) · Sẵn sàng · phiên bản 1']);
  });

  it('guides a missing account or a malformed region before any POST', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'vpc-guard', type: 'vpc', account: '', region: '' }).catch(() => undefined);
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/Chọn tài khoản AWS sẵn sàng/);
    await user.selectOptions(field('Tài khoản hạ tầng'), 'aws-account');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/Nhập vùng AWS là một chuỗi literal/);
    await user.click(field('Vùng AWS'));
    await user.paste('${context.env.region}');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/Nhập vùng AWS là một chuỗi literal/);
    expect(posted).toEqual([]);
  });

  it('Aurora uses its own account and region: no Environment inheritance, private subnets from the shared VPC', async () => {
    const user = userEvent.setup();
    const { posted, gets } = backend();
    await ready();
    await completeDefinition(user, { id: 'aurora', type: 'postgres', module: 'aurora', account: 'aws-second', region: 'eu-west-1' });
    expect(screen.queryByLabelText('Kết nối AWS')).not.toBeInTheDocument();
    expect(screen.getByText(/mỗi cấu hình có thể dùng tài khoản và vùng khác nhau/)).toBeInTheDocument();
    // The region has its own control and is never a generic typed variable.
    expect(screen.queryByLabelText('Ghi đè region')).not.toBeInTheDocument();
    // database/username arrive as Type params: they need no Definition value and outrank any fallback or module default.
    expect(screen.getAllByText(/Nguồn: tham số "(database|username)" của Loại tài nguyên \(qua ánh xạ đầu vào\)/)).toHaveLength(2);
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/Cần nhập trước khi lưu: engine_version, max_capacity, min_capacity/);
    expect(posted).toEqual([]);
    await overrideValue(user, 'engine_version', '16.4');
    await overrideValue(user, 'min_capacity', '0.5');
    await overrideValue(user, 'max_capacity', '2');
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên aurora.');
    expect(posted[0]).toMatchObject({ resourceType: 'postgres', driverType: 'terraform', driverAccount: 'aws-second' });
    expect(values(posted[0])).toEqual({
      source: pinOf('aurora'),
      variables: { engine_version: '16.4', min_capacity: 0.5, max_capacity: 2, name: '${context.infra.resourceName}', vpc_id: `${infra}.id}`, subnet_ids: `${infra}.subnetIds}`, private_subnet_ids: `${infra}.privateSubnetIds}`, region: 'eu-west-1' },
      input_mapping: { database: { param: 'database' }, username: { param: 'username' } },
      output_mapping: { host: 'host', port: 'port', database: 'database', username: 'username', password: 'password' },
      provider_bindings: { aws: { target: 'account' } },
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
    expect(await screen.findByRole('alert')).toHaveTextContent(/Cần nhập trước khi lưu: public_access_cidrs/);
    expect(posted).toEqual([]);
    await overrideValue(user, 'public_access_cidrs', '["203.0.113.10/32"]');
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên eks.');
    expect(values(posted[0]).variables).toMatchObject({ public_access_cidrs: ['203.0.113.10/32'], subnet_ids: `${infra}.subnetIds}`, region: 'us-east-1' });
    expect(values(posted[0]).provider_bindings).toEqual({ aws: { target: 'account' } });
    expect(JSON.stringify(posted[0])).not.toMatch(/0\.0\.0\.0|executor/);
  });

  it('Kubernetes modules take no account: the cluster comes from a published EKS class or the labelled legacy Environment target', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình (Definition ID)'), 'pg-k8s');
    await user.type(field('Tên hiển thị'), 'PostgreSQL trong cluster');
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    await user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: /^postgres-k8s ·/ }));
    expect(screen.queryByLabelText('Tài khoản hạ tầng')).not.toBeInTheDocument();
    expect(screen.getByText(/Module này không chọn tài khoản. Truy cập cluster được kế thừa/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Ghép đầu vào theo tên trùng' }));
    await user.click(screen.getByRole('button', { name: 'Ghép đầu ra theo tên trùng' }));
    await user.click(screen.getByRole('button', { name: 'Dùng preset cho namespace' }));
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/Chọn nơi provider kubernetes lấy cluster/);
    expect(posted).toEqual([]);
    const target = field('Cluster đích của provider kubernetes');
    expect(target).toHaveValue('');
    expect(within(target).getByRole('option', { name: /Cluster của Môi trường \(cách cũ, giữ tới V03\)/ })).toBeInTheDocument();
    await user.selectOptions(target, 'resource');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/Chọn class cluster EKS/);
    const classes = within(field('Class cluster EKS của provider kubernetes')).getAllByRole('option').map((option) => option.getAttribute('value'));
    expect(classes).toEqual(['', 'eks']);
    await user.selectOptions(field('Class cluster EKS của provider kubernetes'), 'eks');
    expect(screen.getByText(/Nguồn cluster của provider/)).toHaveTextContent('cluster do node EKS cung cấp (class eks)');
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên pg-k8s.');
    expect(posted[0]).toMatchObject({ driverType: 'terraform' });
    expect(posted[0]).not.toHaveProperty('driverAccount');
    expect(values(posted[0]).provider_bindings).toEqual({ kubernetes: { target: 'resource', resource_ref: 'k8s-cluster.eks#environments.@app.@env' } });
    expect(values(posted[0]).source).toEqual(pinOf('postgres-k8s'));
  });

  it('registers Redis from either verified module and flags the secret output; the cloud-managed module asks for an account', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình (Definition ID)'), 'redis-shared');
    await user.type(field('Tên hiển thị'), 'Redis dùng chung');
    await user.selectOptions(field('Loại tài nguyên'), 'redis');
    await user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: /^redis-k8s ·/ }));
    expect(screen.getByText(/Output password \(string, bắt buộc, bí mật\)/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Ghép đầu ra theo tên trùng' }));
    await user.selectOptions(field('Cluster đích của provider kubernetes'), 'environment');
    await user.click(screen.getByRole('button', { name: 'Dùng preset cho namespace' }));
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('redis-shared');
    expect(posted[0]).toMatchObject({ resourceType: 'redis' });
    expect(values(posted[0]).output_mapping).toEqual({ host: 'host', port: 'port', username: 'username', password: 'password', tls: 'tls' });
    // The cloud-managed module is a second, separate choice that needs an account and a region.
    await user.type(field('ID cấu hình (Definition ID)'), 'redis-cache');
    await user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: /^elasticache ·/ }));
    expect(field('Tài khoản hạ tầng')).toBeRequired();
    expect(field('Vùng AWS')).toBeRequired();
    expect(screen.queryByLabelText('Cluster đích của provider kubernetes')).not.toBeInTheDocument();
  });

  it('never maps by name on its own and refuses an incomplete contract before any POST', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.type(field('ID cấu hình (Definition ID)'), 'aurora-unmapped');
    await user.type(field('Tên hiển thị'), 'Aurora chưa ánh xạ');
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    await user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: /^aurora ·/ }));
    await waitFor(() => expect(within(field('Tài khoản hạ tầng')).getByRole('option', { name: /AWS chính/ })).toBeInTheDocument());
    await user.selectOptions(field('Tài khoản hạ tầng'), 'aws-account');
    await user.type(field('Vùng AWS'), 'us-east-1');
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
    await completeDefinition(user, { id: 'pg-roundtrip', type: 'postgres', module: 'postgres-k8s', binding: { cluster: 'eks' } });
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
    await user.type(field('ID cấu hình (Definition ID)'), 'pin-drafts');
    await user.type(field('Tên hiển thị'), 'Bản nháp theo pin');
    await user.selectOptions(field('Loại tài nguyên'), 'postgres');
    const pick = (module: string) => user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: new RegExp(`^${module} ·`) }));
    await pick('postgres-k8s');
    await user.click(screen.getByRole('button', { name: 'Ghép đầu vào theo tên trùng' }));
    await user.click(screen.getByRole('button', { name: 'Ghép đầu ra theo tên trùng' }));
    await user.selectOptions(field('Cluster đích của provider kubernetes'), 'environment');
    await overrideValue(user, 'storage', '9Gi');
    await user.click(screen.getByRole('button', { name: 'Dùng preset cho namespace' }));
    await pick('aurora');
    await waitFor(() => expect(within(field('Tài khoản hạ tầng')).getByRole('option', { name: /AWS thứ hai/ })).toBeInTheDocument());
    await user.selectOptions(field('Tài khoản hạ tầng'), 'aws-second');
    await user.type(field('Vùng AWS'), 'eu-west-1');
    await overrideValue(user, 'min_capacity', '2');
    expect(screen.queryByLabelText('Ghi đè storage')).not.toBeInTheDocument();
    await pick('postgres-k8s');
    expect(field('Giá trị storage')).toHaveValue('9Gi');
    expect(screen.queryByLabelText('Tài khoản hạ tầng')).not.toBeInTheDocument();
    expect(field('Biến module nhận tham số database')).toHaveValue('database');
    await pick('aurora');
    expect(field('Giá trị min_capacity')).toHaveValue('2');
    expect(field('Tài khoản hạ tầng')).toHaveValue('aws-second');
    expect(field('Vùng AWS')).toHaveValue('eu-west-1');
    await pick('postgres-k8s');
    await submitDefinition(user);
    await screen.findByRole('status');
    expect(values(posted[0]).variables).toEqual({ storage: '9Gi', namespace: "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}" });
    expect(field('Ghi đè storage')).not.toBeChecked();
    await pick('aurora');
    expect(field('Giá trị min_capacity')).toHaveValue('2');
    expect(field('Vùng AWS')).toHaveValue('eu-west-1');
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
    await user.type(field('ID cấu hình (Definition ID)'), 'kept-id');
    fail = false;
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('kept-id');
    expect(posted).toEqual([]);
  });
});

const submitName = 'Đăng ký cấu hình tài nguyên';

describe('UC-03 Definition form states and errors', () => {
  it('lists definitions with Name, ID, version and account, hides workload ones, marks existing-cluster as system read-only and offers Sửa only for editable ones', async () => {
    backend({ definitions: [
      { key: 'pg-fast', name: 'PostgreSQL nhanh', version: 4, resourceType: 'postgres', driverType: 'terraform', driverAccount: 'aws-account', criteria: [{}] },
      { key: 'workload-score', name: 'Mẫu', version: 1, resourceType: 'workload', driverType: 'score-k8s', criteria: [{}] },
      { key: 'cluster-existing', name: 'Cluster hệ thống', version: 1, resourceType: 'k8s-cluster', driverType: 'existing-cluster', criteria: [{}, {}] },
      { key: 'ns-shared', name: 'Namespace chung', version: 2, resourceType: 'k8s-namespace', driverType: 'terraform', criteria: [{}] },
    ] });
    render(<ResourceDefinitionsPage />);
    expect(await screen.findByText('PostgreSQL nhanh')).toBeInTheDocument();
    expect(screen.queryByText('Mẫu')).not.toBeInTheDocument();
    expect(screen.getByText('PostgreSQL nhanh').nextElementSibling).toHaveTextContent('ID pg-fast · postgres · Terraform (module đã kiểm chứng) · tài khoản AWS chính (aws-account) · phiên bản 4 · 1 điều kiện áp dụng');
    expect(screen.getByText('Cluster hệ thống').nextElementSibling).toHaveTextContent(/2 điều kiện áp dụng · Hệ thống \(chỉ đọc\)/);
    expect(screen.getByText('Namespace chung').nextElementSibling).toHaveTextContent('phiên bản 2');
    expect(screen.getByRole('button', { name: 'Sửa cấu hình pg-fast' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Sửa cấu hình cluster-existing' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Xóa cấu hình/ })).not.toBeInTheDocument();
  });

  it('rejects a blank or malformed ID, a blank Name and invalid JSON locally without any POST', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-namespace');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('Nhập ID cấu hình.');
    await user.type(field('ID cấu hình (Definition ID)'), 'Bad_ID ');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent(/chỉ gồm chữ thường/);
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('Bad_ID ');
    expect(field('ID cấu hình (Definition ID)')).toBeInvalid();
    await user.clear(field('ID cấu hình (Definition ID)'));
    await user.type(field('ID cấu hình (Definition ID)'), 'ns-ok');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('Nhập Tên hiển thị của cấu hình.');
    expect(field('Tên hiển thị')).toBeInvalid();
    await user.type(field('Tên hiển thị'), '   ');
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('Nhập Tên hiển thị của cấu hình.');
    await user.clear(field('Tên hiển thị'));
    await user.type(field('Tên hiển thị'), 'Namespace OK');
    await user.click(screen.getByRole('button', { name: 'Tham số: JSON nâng cao' }));
    fireEvent.change(field('Tham số cấu hình (JSON object)'), { target: { value: '{bad' } });
    await user.click(screen.getByRole('button', { name: 'Ánh xạ: JSON nâng cao' }));
    await submitDefinition(user);
    expect(await screen.findByRole('alert')).toHaveTextContent('phải là JSON hợp lệ');
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
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('ns-dup');
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
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('ns-fail');
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
    expect(await screen.findByRole('alert', { name: 'Cần tải lại cấu hình' })).toHaveTextContent(/Chưa biết lần đăng ký/);
    expect(posted).toHaveLength(1);
  });

  it('holds the create after an unknown outcome, never replays the POST and opens the stored Definition when the key exists', async () => {
    const user = userEvent.setup();
    const stored = { key: 'ns-net', name: 'Tên ns-net', version: 1, resourceType: 'k8s-namespace', driverType: 'terraform', driverInputs: { values: { source: pinOf('k8s-namespace'), variables: {}, input_mapping: {}, output_mapping: {}, provider_bindings: {} } }, provision: {}, criteria: [{}] };
    const { posted, state } = backend({ post: () => { throw new TypeError('network'); } });
    await ready();
    await completeDefinition(user, { id: 'ns-net', type: 'k8s-namespace' });
    await submitDefinition(user);
    const review = await screen.findByRole('alert', { name: 'Cần tải lại cấu hình' });
    expect(review).toHaveTextContent(/Chưa biết lần đăng ký đã được lưu hay chưa/);
    expect(screen.getByRole('button', { name: submitName })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: submitName }));
    expect(posted).toHaveLength(1);
    state.definitions = [stored];
    await user.click(within(review).getByRole('button', { name: 'Kiểm tra trên máy chủ' }));
    expect(await screen.findByRole('heading', { name: 'Sửa cấu hình ns-net' })).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent(/đã có trên máy chủ/);
    expect(screen.queryByRole('alert', { name: 'Cần tải lại cấu hình' })).not.toBeInTheDocument();
    expect(posted).toHaveLength(1);
  });

  it('releases the hold with the draft kept when the unknown create rolled back, and keeps the hold when the check itself fails', async () => {
    const user = userEvent.setup();
    const { posted, state } = backend({ post: () => Response.json({ code: 'COMMIT_UNCERTAIN' }, { status: 503 }) });
    await ready();
    await completeDefinition(user, { id: 'ns-net', type: 'k8s-namespace' });
    await submitDefinition(user);
    const review = await screen.findByRole('alert', { name: 'Cần tải lại cấu hình' });
    expect(screen.getByRole('button', { name: submitName })).toBeDisabled();
    // The check fails: draft and hold stay.
    state.definitionLists = [() => Response.json({ error: 'x' }, { status: 500 })];
    await user.click(within(review).getByRole('button', { name: 'Kiểm tra trên máy chủ' }));
    await waitFor(() => expect(screen.getByRole('alert', { name: 'Cần tải lại cấu hình' })).toBeInTheDocument());
    expect(screen.getByRole('button', { name: submitName })).toBeDisabled();
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('ns-net');
    // The key is absent: the earlier create rolled back, the hold is released and the draft is intact.
    await user.click(within(screen.getByRole('alert', { name: 'Cần tải lại cấu hình' })).getByRole('button', { name: 'Kiểm tra trên máy chủ' }));
    await waitFor(() => expect(screen.queryByRole('alert', { name: 'Cần tải lại cấu hình' })).not.toBeInTheDocument());
    expect(screen.getByRole('status')).toHaveTextContent(/lần đăng ký trước không được lưu/);
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('ns-net');
    expect(screen.getByRole('button', { name: submitName })).toBeEnabled();
    expect(posted).toHaveLength(1);
  });

  it('keeps metadata and a Vietnamese retry when Resource Types fail to load', async () => {
    const user = userEvent.setup();
    const { state } = backend({ typesFail: true });
    render(<ResourceDefinitionsPage />);
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải loại tài nguyên' });
    await user.type(field('ID cấu hình (Definition ID)'), 'ns-keep');
    expect(screen.getByRole('button', { name: submitName })).toBeDisabled();
    state.typesFail = false;
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await screen.findByRole('option', { name: 'vpc' });
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('ns-keep');
  });

  it('reports a committed registration when the reload fails, resets the form and retries only the list', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ definitionLists: [() => Response.json({ resourceDefinitions: [] }), () => Response.json({ error: 'x' }, { status: 500 })] });
    await ready();
    await completeDefinition(user, { id: 'ns-new', type: 'k8s-namespace' });
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên ns-new.');
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải danh sách' });
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('');
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await screen.findByText('Chưa có cấu hình tài nguyên nào.');
    expect(posted).toHaveLength(1);
    expect(screen.getByRole('status')).toHaveTextContent('ns-new');
  });

  it('treats inherited Object property names as unsupported custom types without crashing', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ types: [...typeFixture, { key: 'constructor', inputs: [], outputs: [], classes: [] }, { key: 'toString', inputs: [], outputs: [], classes: [] }], definitions: [{ key: 'odd', name: 'Lạ', version: 1, resourceType: 'constructor', driverType: 'toString', criteria: [{}] }] });
    render(<ResourceDefinitionsPage />);
    expect(await screen.findByText('Lạ')).toBeInTheDocument();
    expect(screen.getByText('Lạ').nextElementSibling).toHaveTextContent('ID odd · constructor · toString · phiên bản 1 · 1 điều kiện áp dụng');
    await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
    expect(screen.getByRole('option', { name: 'constructor (chưa có module đã kiểm chứng)' })).toBeDisabled();
    expect(screen.getByRole('option', { name: 'toString (chưa có module đã kiểm chứng)' })).toBeDisabled();
    await user.type(field('ID cấu hình (Definition ID)'), 'odd-new');
    await user.type(field('Tên hiển thị'), 'Cấu hình lạ');
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
      await user.type(field('ID cấu hình (Definition ID)'), 'my-pg');
      release();
      await waitFor(() => expect(typeSelect()).toHaveValue('postgres'));
      expect(field('ID cấu hình (Definition ID)')).toHaveValue('my-pg');
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
      await user.type(field('ID cấu hình (Definition ID)'), 'cluster-a');
      expect(typeSelect()).toHaveValue('');
      state.typesFail = false;
      await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
      await waitFor(() => expect(typeSelect()).toHaveValue('k8s-cluster'));
      expect(field('ID cấu hình (Definition ID)')).toHaveValue('cluster-a');
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
      expect(posted[1]).toMatchObject({ key: 'ns-env', criteria: [{ env_type: 'staging' }] });
      expect(posted[1]).not.toHaveProperty('executionProfile');
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
      const { posted } = await start(user, 'ns-copy', { definitions: [{ key: 'pg-adv', name: 'PG nâng cao', version: 1, resourceType: 'postgres', driverType: 'terraform', criteria: [{ env_type: 'staging', class: 'fast' }, { app_id: 'shop' }] }], post: () => Response.json({ error: 'x' }, { status: 500 }) });
      await waitFor(() => expect(within(field('Sao chép điều kiện từ cấu hình khác')).getByRole('option', { name: 'PG nâng cao (pg-adv) (postgres)' })).toBeInTheDocument());
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
      expect(field('ID cấu hình (Definition ID)')).toHaveValue('ns-late');
      await submitDefinition(user);
      await waitFor(() => expect(posted).toHaveLength(1));
      expect(posted[0]?.criteria).toEqual([{ app_id: 'typed-before' }, { class: 'fast' }]);
    });

    it('warns about possible overlap with a same-type Definition without calling it a winner, shows the matching rules, has no profile or browser-only scope preview, and never posts while previewing', async () => {
      const user = userEvent.setup();
      const { posted, gets } = await start(user, 'ns-overlap', { definitions: [{ key: 'ns-other', name: 'Namespace khác', version: 1, resourceType: 'k8s-namespace', driverType: 'terraform', criteria: [{}] }, { key: 'pg-other', name: 'PG', version: 1, resourceType: 'postgres', driverType: 'terraform', criteria: [{}] }] });
      const warning = await screen.findByRole('group', { name: 'Cảnh báo nguy cơ chồng lấn' });
      expect(warning).toHaveTextContent('ns-other');
      expect(warning).not.toHaveTextContent('pg-other');
      expect(warning.textContent).not.toMatch(/ambiguous|mơ hồ|executionProfile|phạm vi triển khai/i);
      const rules = screen.getByRole('note', { name: 'Quy tắc so khớp' });
      expect(rules).toHaveTextContent('AND trong một dòng, OR giữa các dòng');
      expect(rules).toHaveTextContent('env_type 1 · app_id 2 · env_id 4 · res_id 8 · class 16');
      expect(rules).toHaveTextContent('Dòng trống {} là wildcard tường minh');
      expect(rules).toHaveTextContent('khác ID tài nguyên res_id');
      const summary = screen.getByRole('group', { name: 'Tóm tắt phạm vi áp dụng' });
      expect(summary).not.toHaveTextContent(/executionProfile|Phạm vi triển khai/);
      expect(screen.queryByRole('group', { name: /Xem trước phạm vi/ })).not.toBeInTheDocument();
      expect(screen.queryByLabelText(/Phạm vi triển khai/)).not.toBeInTheDocument();
      expect(screen.getByRole('group', { name: 'Ngữ cảnh kiểm tra matching' })).toHaveTextContent('Cần thêm ngữ cảnh');
      expect(posted).toHaveLength(0);
      expect(gets.every((url) => !/preview|match/.test(url))).toBe(true);
    });
  });
});

// UC-03 V02-06: PE/Admin edit the whole configuration except the ID and Resource Type; Save is one
// version compare-and-set and never mutates resources.
describe('UC-03 V02 Definition edit', () => {
  const stored = (over: Record<string, unknown> = {}) => ({
    key: 'vpc-main', name: 'VPC chính', version: 3, resourceType: 'vpc', driverType: 'terraform', driverAccount: 'aws-account',
    driverInputs: { values: { source: pinOf('vpc'), variables: { name: '${context.infra.resourceName}', region: 'ap-southeast-1' }, input_mapping: {},
      output_mapping: { id: 'id', cidr: 'cidr', subnetIds: 'subnetIds', privateSubnetIds: 'privateSubnetIds' }, provider_bindings: { aws: { target: 'account' } } } },
    provision: {}, criteria: [{ app_id: 'shop', env_id: 'production' }], ...over,
  });
  async function openEditor(user: ReturnType<typeof userEvent.setup>, options: Backend = {}) {
    const harness = backend({ definitions: [stored()], ...options });
    render(<ResourceDefinitionsPage />);
    await screen.findByText('VPC chính');
    await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'Sửa cấu hình vpc-main' }));
    return harness;
  }

  it('loads the stored configuration, keeps the ID and Resource Type read-only and saves one PUT with the expected version', async () => {
    const user = userEvent.setup();
    const { puts, posted } = await openEditor(user);
    expect(screen.getByRole('heading', { name: 'Sửa cấu hình vpc-main' })).toBeInTheDocument();
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('vpc-main');
    expect(field('ID cấu hình (Definition ID)')).toHaveAttribute('readonly');
    expect(field('Loại tài nguyên')).toBeDisabled();
    expect(field('Tên hiển thị')).toHaveValue('VPC chính');
    expect(field('Tài khoản hạ tầng')).toHaveValue('aws-account');
    expect(field('Vùng AWS')).toHaveValue('ap-southeast-1');
    expect(field('Module đã kiểm chứng')).toBeDisabled();
    expect(screen.getByRole('radio', { name: /Theo ứng dụng/ })).toBeChecked();
    expect(screen.getByRole('button', { name: 'Lưu thay đổi' })).toBeEnabled();
    expect(screen.getByText(/Chỉ lần Xem trước và Triển khai mới dùng cấu hình mới/)).toBeInTheDocument();
    // Edit Name, account and region.
    await user.clear(field('Tên hiển thị'));
    await user.type(field('Tên hiển thị'), 'VPC đã đổi tên');
    await user.selectOptions(field('Tài khoản hạ tầng'), 'aws-second');
    await user.clear(field('Vùng AWS'));
    await user.type(field('Vùng AWS'), 'eu-west-1');
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Đã lưu cấu hình vpc-main (phiên bản 4).');
    expect(posted).toEqual([]);
    expect(puts).toHaveLength(1);
    expect(puts[0]?.url).toMatch(/\/resource-definitions\/vpc-main$/);
    expect(puts[0]?.body).toMatchObject({ key: 'vpc-main', name: 'VPC đã đổi tên', resourceType: 'vpc', driverType: 'terraform', driverAccount: 'aws-second', version: 3, criteria: [{ app_id: 'shop', env_id: 'production' }] });
    expect(values(puts[0]?.body)).toMatchObject({ source: pinOf('vpc'), variables: { name: '${context.infra.resourceName}', region: 'eu-west-1' }, provider_bindings: { aws: { target: 'account' } } });
    // The next save is based on the version the server returned.
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    await waitFor(() => expect(puts).toHaveLength(2));
    expect(puts[1]?.body.version).toBe(4);
  });

  it('shows a 409 as a stale-version review, keeps the draft, disables Save and never resubmits; an explicit reload replaces the draft', async () => {
    const user = userEvent.setup();
    const { puts } = await openEditor(user, { put: () => Response.json({ code: 'STALE_VERSION', error: 'x' }, { status: 409 }),
      definitionLists: [() => Response.json({ resourceDefinitions: [stored()] }), () => Response.json({ resourceDefinitions: [stored({ name: 'Bản của người khác', version: 4 })] }), () => Response.json({ resourceDefinitions: [stored({ name: 'Bản của người khác', version: 4 })] })] });
    await user.clear(field('Tên hiển thị'));
    await user.type(field('Tên hiển thị'), 'Bản nháp của tôi');
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    const review = await screen.findByRole('alert', { name: 'Cần tải lại cấu hình' });
    expect(review).toHaveTextContent(/Chưa có gì bị ghi đè/);
    expect(field('Tên hiển thị')).toHaveValue('Bản nháp của tôi');
    expect(screen.getByRole('button', { name: 'Lưu thay đổi' })).toBeDisabled();
    expect(puts).toHaveLength(1);
    await user.click(within(review).getByRole('button', { name: 'Tải lại bản mới nhất' }));
    await waitFor(() => expect(field('Tên hiển thị')).toHaveValue('Bản của người khác'));
    expect(screen.queryByRole('alert', { name: 'Cần tải lại cấu hình' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Lưu thay đổi' })).toBeEnabled();
    expect(puts).toHaveLength(1);
  });

  it('reports an unknown outcome without replaying the PUT and requires an explicit reload', async () => {
    const user = userEvent.setup();
    const { puts } = await openEditor(user, { put: () => { throw new TypeError('network'); } });
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    expect(await screen.findByRole('alert', { name: 'Cần tải lại cấu hình' })).toHaveTextContent(/Chưa biết thay đổi đã được lưu hay chưa/);
    expect(screen.getByRole('button', { name: 'Lưu thay đổi' })).toBeDisabled();
    expect(puts).toHaveLength(1);
  });

  it('keeps the draft on a rejected edit (400) and maps 403 and 404 to safe messages', async () => {
    for (const [status, code, text] of [[400, 'INVALID_REQUEST', /từ chối cấu hình/], [403, 'FORBIDDEN', /không có quyền/], [404, 'NOT_FOUND', /Không tìm thấy cấu hình/]] as const) {
      const user = userEvent.setup();
      const { puts } = await openEditor(user, { put: () => Response.json({ code, error: 'raw tài khoản secret', field: 'definition.driverAccount' }, { status }) });
      await user.clear(field('Tên hiển thị'));
      await user.type(field('Tên hiển thị'), 'Giữ lại');
      await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
      const alert = await screen.findByRole('alert');
      expect(alert).toHaveTextContent(text);
      expect(alert).not.toHaveTextContent('raw tài khoản secret');
      expect(field('Tên hiển thị')).toHaveValue('Giữ lại');
      expect(screen.getByRole('button', { name: 'Lưu thay đổi' })).toBeEnabled();
      expect(puts).toHaveLength(1);
      document.body.innerHTML = '';
      vi.unstubAllGlobals();
    }
  });

  it('keeps the saved result when the list reload fails and retries only the list', async () => {
    const user = userEvent.setup();
    const { puts, gets } = await openEditor(user, { definitionLists: [() => Response.json({ resourceDefinitions: [stored()] }), () => Response.json({ error: 'x' }, { status: 500 })] });
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Đã lưu cấu hình vpc-main (phiên bản 4).');
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải danh sách' });
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await waitFor(() => expect(screen.queryByRole('alert', { name: 'Lỗi tải danh sách' })).not.toBeInTheDocument());
    expect(puts).toHaveLength(1);
    expect(screen.getByRole('status')).toHaveTextContent('phiên bản 4');
    expect(gets.filter((url) => url.endsWith('/resource-definitions')).length).toBeGreaterThanOrEqual(3);
  });

  it('flags an account that is no longer compatible and keeps the edit', async () => {
    const user = userEvent.setup();
    await openEditor(user, { definitions: [stored({ driverAccount: 'aws-pending' })] });
    expect(await screen.findByText(/Tài khoản đã chọn \(aws-pending\) không còn tương thích hoặc chưa Sẵn sàng/)).toBeInTheDocument();
    expect(within(field('Tài khoản hạ tầng')).getByRole('option', { name: /AWS chưa xác minh \(aws-pending\) \(không còn tương thích/ })).toBeInTheDocument();
  });

  it('opens a Definition whose module pin is no longer verified without carrying its old module configuration; Save waits for an explicit module choice', async () => {
    const user = userEvent.setup();
    const drifted = stored({ resourceType: 'postgres', driverInputs: { values: { source: { ...pinOf('aurora'), revision: 'gone' }, variables: { region: 'eu-west-1' }, input_mapping: {}, output_mapping: {}, provider_bindings: { aws: { target: 'account' } } } } });
    const { puts } = backend({ definitions: [drifted] });
    render(<ResourceDefinitionsPage />);
    await screen.findByText('VPC chính');
    await waitFor(() => expect(screen.getByRole('option', { name: 'postgres' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'Sửa cấu hình vpc-main' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/không còn nằm trong metadata đã kiểm chứng/);
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('vpc-main');
    expect(field('Module đã kiểm chứng')).toBeEnabled();
    expect(field('Module đã kiểm chứng')).toHaveValue('');
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    expect(puts).toEqual([]);
    expect(screen.queryByLabelText('Tài khoản hạ tầng')).not.toBeInTheDocument();
  });

  it('switches the verified module of the same Resource Type during an edit, keeps each pair draft and saves one PUT with the chosen module', async () => {
    const user = userEvent.setup();
    const aurora = stored({ key: 'pg-main', name: 'PG chính', resourceType: 'postgres', driverAccount: 'aws-second', criteria: [{}],
      driverInputs: { values: { source: pinOf('aurora'), variables: { engine_version: '16.4', min_capacity: 0.5, max_capacity: 2, name: '${context.infra.resourceName}', vpc_id: `${infra}.id}`, subnet_ids: `${infra}.subnetIds}`, private_subnet_ids: `${infra}.privateSubnetIds}`, region: 'eu-west-1' },
        input_mapping: { database: { param: 'database' }, username: { param: 'username' } }, output_mapping: { host: 'host', port: 'port', database: 'database', username: 'username', password: 'password' }, provider_bindings: { aws: { target: 'account' } } } } });
    const { puts, posted } = backend({ definitions: [aurora] });
    render(<ResourceDefinitionsPage />);
    await screen.findByText('PG chính');
    await waitFor(() => expect(screen.getByRole('option', { name: 'postgres' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'Sửa cấu hình pg-main' }));
    expect(field('Module đã kiểm chứng')).toBeEnabled();
    expect(field('Loại tài nguyên')).toBeDisabled();
    expect(field('ID cấu hình (Definition ID)')).toHaveAttribute('readonly');
    expect(field('Tài khoản hạ tầng')).toHaveValue('aws-second');
    expect(field('Vùng AWS')).toHaveValue('eu-west-1');
    // Switch to the in-cluster module: no account field, nothing is sent for the cloud pair.
    await user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: /^postgres-k8s ·/ }));
    expect(screen.queryByLabelText('Tài khoản hạ tầng')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Ghép đầu vào theo tên trùng' }));
    await user.click(screen.getByRole('button', { name: 'Ghép đầu ra theo tên trùng' }));
    await user.click(screen.getByRole('button', { name: 'Dùng preset cho namespace' }));
    await user.selectOptions(field('Cluster đích của provider kubernetes'), 'resource');
    await user.selectOptions(field('Class cluster EKS của provider kubernetes'), 'eks');
    // Switching back restores the cloud pair draft exactly; the in-cluster draft survives another round trip.
    await user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: /^aurora ·/ }));
    expect(field('Tài khoản hạ tầng')).toHaveValue('aws-second');
    expect(field('Vùng AWS')).toHaveValue('eu-west-1');
    await user.selectOptions(field('Module đã kiểm chứng'), screen.getByRole('option', { name: /^postgres-k8s ·/ }));
    expect(field('Class cluster EKS của provider kubernetes')).toHaveValue('eks');
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Đã lưu cấu hình pg-main (phiên bản 4).');
    expect(posted).toEqual([]);
    expect(puts).toHaveLength(1);
    expect(puts[0]?.url).toMatch(/\/resource-definitions\/pg-main$/);
    expect(puts[0]?.body).toMatchObject({ key: 'pg-main', resourceType: 'postgres', version: 3 });
    expect(puts[0]?.body).not.toHaveProperty('driverAccount');
    expect(values(puts[0]?.body).source).toEqual(pinOf('postgres-k8s'));
    expect(values(puts[0]?.body).provider_bindings).toEqual({ kubernetes: { target: 'resource', resource_ref: 'k8s-cluster.eks#environments.@app.@env' } });
  });

  it('edits stored provision rules with nested params and both flags, keeps explicit false/empty values through form and advanced JSON, and leaves unrelated configuration untouched', async () => {
    const user = userEvent.setup();
    const provision = {
      'postgres.audit#shared.audit': { is_dependent: true, match_dependents: false, params: { database: 'audit', options: { retention: 7, tags: ['a', { deep: true }], enabled: false } } },
      'redis.cache#shared.cache': { is_dependent: false, match_dependents: true, params: {} },
    };
    const original = stored({ provision });
    const { puts } = await openEditor(user, { definitions: [original] });
    expect(field('Descriptor tài nguyên đi kèm 1')).toHaveValue('postgres.audit#shared.audit');
    expect(field('Tài nguyên đi kèm 1 phụ thuộc vào tài nguyên cha (is_dependent)')).toBeChecked();
    expect(field('Nối consumer của tài nguyên cha tới tài nguyên đi kèm 1 (match_dependents)')).not.toBeChecked();
    expect(field('Tài nguyên đi kèm 2 phụ thuộc vào tài nguyên cha (is_dependent)')).not.toBeChecked();
    expect(field('Nối consumer của tài nguyên cha tới tài nguyên đi kèm 2 (match_dependents)')).toBeChecked();
    expect(JSON.parse((field('Params tài nguyên đi kèm 1 (JSON object)') as HTMLTextAreaElement).value)).toEqual(provision['postgres.audit#shared.audit'].params);
    expect(screen.getAllByText(/Đảo vai trò: tài nguyên đi kèm \(consumer\) phụ thuộc vào tài nguyên cha \(provider\)/)).toHaveLength(2);
    // Form -> advanced JSON -> form keeps everything, including the explicit false flag and empty params.
    await user.click(screen.getByRole('button', { name: 'Quy tắc: JSON nâng cao' }));
    expect(JSON.parse((field('Quy tắc tạo tài nguyên liên quan (JSON object)') as HTMLTextAreaElement).value)).toEqual(provision);
    await user.click(screen.getByRole('button', { name: 'Quy tắc: biểu mẫu' }));
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(puts[0]?.body.provision).toEqual(provision);
    // Only the provision changes: every other field of the stored Definition is sent back as stored.
    expect(puts[0]?.body).toMatchObject({ key: original.key, name: original.name, resourceType: original.resourceType, driverType: original.driverType, driverAccount: original.driverAccount, criteria: original.criteria, version: 3 });
    expect(puts[0]?.body.driverInputs).toEqual(original.driverInputs);
    // Toggle both flags of the first rule and remove the second.
    await user.click(field('Tài nguyên đi kèm 1 phụ thuộc vào tài nguyên cha (is_dependent)'));
    await user.click(field('Nối consumer của tài nguyên cha tới tài nguyên đi kèm 1 (match_dependents)'));
    await user.click(screen.getByRole('button', { name: 'Xóa tài nguyên đi kèm 2' }));
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    await waitFor(() => expect(puts).toHaveLength(2));
    expect(puts[1]?.body.version).toBe(4);
    expect(puts[1]?.body.provision).toEqual({ 'postgres.audit#shared.audit': { is_dependent: false, match_dependents: true, params: provision['postgres.audit#shared.audit'].params } });
  });

  it('keeps a stored provision rule the form cannot represent in advanced JSON and saves it unchanged', async () => {
    const user = userEvent.setup();
    const provision = { 'redis.cache#shared.cache': { is_dependent: true, extra: { kept: 1 } } };
    const { puts } = await openEditor(user, { definitions: [stored({ provision })] });
    expect(screen.getByRole('button', { name: 'Quy tắc: JSON nâng cao' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByText(/biểu mẫu không biểu diễn/)).toBeInTheDocument();
    expect(JSON.parse((field('Quy tắc tạo tài nguyên liên quan (JSON object)') as HTMLTextAreaElement).value)).toEqual(provision);
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(puts[0]?.body.provision).toEqual(provision);
  });

  it('refuses a duplicate descriptor and invalid params locally without a request', async () => {
    const user = userEvent.setup();
    const { puts } = await openEditor(user);
    for (const index of [1, 2]) {
      await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên đi kèm' }));
      await user.type(field(`Descriptor tài nguyên đi kèm ${index}`), 'redis.cache#shared.cache');
    }
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Descriptor "redis.cache#shared.cache" bị trùng');
    await user.clear(field('Descriptor tài nguyên đi kèm 2'));
    await user.type(field('Descriptor tài nguyên đi kèm 2'), 'redis.other#shared.other');
    fireEvent.change(field('Params tài nguyên đi kèm 2 (JSON object)'), { target: { value: '[1]' } });
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/Params của "redis.other#shared.other"/);
    expect(puts).toEqual([]);
  });

  it('cancels an edit back to a blank registration form', async () => {
    const user = userEvent.setup();
    await openEditor(user);
    await user.click(screen.getByRole('button', { name: 'Hủy sửa' }));
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('');
    expect(field('ID cấu hình (Definition ID)')).not.toHaveAttribute('readonly');
    expect(screen.getByRole('button', { name: submitName })).toBeInTheDocument();
  });
});
