import { useState } from 'react';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CompanionResources } from './CompanionResources';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { rowsToProvision, type ProvisionRow } from './typedProvision';
import { capabilityFixture } from '../../test/capabilities';
import { typeFixture } from '../../test/types';
import { renderDefinition, backend, card, completeDefinition, field, submitDefinition } from '../../test/definitionKit';

afterEach(() => vi.unstubAllGlobals());
const values = (body?: Record<string, unknown>) => (body?.driverInputs as { values: Record<string, unknown> }).values;
const pinOf = (module: string) => {
  const source = capabilityFixture.find((candidate) => candidate.driverType === 'terraform' && candidate.module === module)?.source;
  if (!source) throw new Error(`fixture without ${module}`);
  return { source_id: source.sourceId, module_id: source.moduleId, revision: source.revision, subdirectory: source.subdirectory, content_sha256: source.contentSha256 };
};
async function ready() {
  renderDefinition(<ResourceDefinitionsPage />);
  await screen.findByText('Chưa có cấu hình tài nguyên nào.');
  await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
}
const optionValues = (select: HTMLElement) => within(select).getAllByRole('option').map((option) => option.getAttribute('value')).filter((value) => value !== '');

describe('UI11-02 reference picker', () => {
  it('chooses Type → class → scope → output with only public compatible outputs and writes one generated placeholder on Apply', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'pg-ref', type: 'postgres', module: 'postgres-k8s', skipVariables: ['namespace'] });
    expect(card('namespace')).toHaveTextContent('Chưa cung cấp');
    await user.click(screen.getByRole('button', { name: 'Cung cấp giá trị namespace' }));
    let dialog = within(screen.getByRole('dialog', { name: 'Chọn nguồn giá trị' }));
    await user.click(dialog.getByRole('radio', { name: 'Lấy từ tài nguyên khác' }));
    expect(dialog.getByRole('button', { name: 'Áp dụng' })).toBeDisabled();
    await user.selectOptions(dialog.getByLabelText('Loại tài nguyên nguồn'), 'postgres');
    expect(optionValues(dialog.getByLabelText('Nhóm cấu hình nguồn'))).toEqual(['default', 'cloud-managed', 'in-cluster']);
    // The secret password and the numeric port cannot fill a string.
    expect(optionValues(dialog.getByLabelText('Output cần lấy'))).toEqual(['host', 'database', 'username']);
    await user.selectOptions(dialog.getByLabelText('Loại tài nguyên nguồn'), 'k8s-namespace');
    await user.selectOptions(dialog.getByLabelText('Output cần lấy'), 'name');
    await user.click(dialog.getByText('Biểu thức sinh ra (kỹ thuật)'));
    expect(dialog.getByText("${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}")).toBeInTheDocument();
    // Cancel changes nothing.
    await user.click(dialog.getByRole('button', { name: 'Hủy' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(card('namespace')).toHaveTextContent('Chưa cung cấp');

    await user.click(screen.getByRole('button', { name: 'Cung cấp giá trị namespace' }));
    dialog = within(screen.getByRole('dialog'));
    await user.click(dialog.getByRole('radio', { name: 'Lấy từ tài nguyên khác' }));
    await user.selectOptions(dialog.getByLabelText('Loại tài nguyên nguồn'), 'k8s-namespace');
    await user.selectOptions(dialog.getByLabelText('Phạm vi nguồn'), 'resource');
    await user.type(dialog.getByLabelText('ID tài nguyên nguồn'), 'shared.ns');
    await user.selectOptions(dialog.getByLabelText('Output cần lấy'), 'name');
    await user.click(dialog.getByRole('button', { name: 'Áp dụng' }));
    expect(card('namespace')).toHaveTextContent('Từ tài nguyên khác');
    expect(card('namespace')).toHaveTextContent('k8s-namespace → name · default · Tài nguyên shared.ns');
    // Reopening starts from the stored choice, not from the raw expression.
    await user.click(screen.getByRole('button', { name: 'Thay đổi namespace' }));
    dialog = within(screen.getByRole('dialog'));
    expect(dialog.getByRole('radio', { name: 'Lấy từ tài nguyên khác' })).toBeChecked();
    expect(dialog.getByLabelText('Loại tài nguyên nguồn')).toHaveValue('k8s-namespace');
    expect(dialog.getByLabelText('ID tài nguyên nguồn')).toHaveValue('shared.ns');
    expect(dialog.getByLabelText('Output cần lấy')).toHaveValue('name');
    await user.click(dialog.getByRole('button', { name: 'Hủy' }));
    await submitDefinition(user);
    await screen.findByText(/^Đã đăng ký cấu hình tài nguyên/);
    expect(values(posted[0]).variables).toEqual({ namespace: "${resources['k8s-namespace.default#shared.ns'].outputs.name}" });
  });

  it('offers no context choice for a number and lets a provided source go back to the module default', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'vpc-default', type: 'vpc' });
    await user.click(screen.getByRole('button', { name: 'Thay đổi az_count' }));
    let dialog = within(screen.getByRole('dialog'));
    expect(dialog.queryByRole('radio', { name: 'Lấy từ ngữ cảnh triển khai' })).not.toBeInTheDocument();
    expect(dialog.getByRole('radio', { name: 'Dùng mặc định của module' })).toBeChecked();
    await user.click(dialog.getByRole('radio', { name: 'Nhập giá trị' }));
    await user.clear(dialog.getByLabelText('Giá trị az_count'));
    await user.type(dialog.getByLabelText('Giá trị az_count'), '3');
    await user.click(dialog.getByRole('button', { name: 'Áp dụng' }));
    expect(card('az_count')).toHaveTextContent(/Nhập trực tiếp.*3/);
    await user.click(screen.getByRole('button', { name: 'Thay đổi az_count' }));
    dialog = within(screen.getByRole('dialog'));
    await user.click(dialog.getByRole('radio', { name: 'Dùng mặc định của module' }));
    await user.click(dialog.getByRole('button', { name: 'Áp dụng' }));
    expect(card('az_count')).toHaveTextContent('Mặc định của module');
    await submitDefinition(user);
    await screen.findByText(/^Đã đăng ký cấu hình tài nguyên/);
    expect(values(posted[0]).variables).not.toHaveProperty('az_count');
  });

  it('keeps unsupported and no-longer-known sources verbatim through edit and save', async () => {
    const user = userEvent.setup();
    const variables = { name: '${context.env.id}', cidr: "${resources['cache.default#@infra'].outputs.url}", region: 'ap-southeast-1' };
    const original = { key: 'vpc-kept', name: 'VPC giữ nguyên', version: 2, resourceType: 'vpc', driverType: 'terraform', driverAccount: 'aws-account',
      driverInputs: { values: { source: pinOf('vpc'), variables, input_mapping: {}, output_mapping: { id: 'id', cidr: 'cidr', subnetIds: 'subnetIds', privateSubnetIds: 'privateSubnetIds' }, provider_bindings: { aws: { target: 'account' } } } }, provision: {}, criteria: [{}] };
    const { puts } = backend({ definitions: [original] });
    render(<ResourceDefinitionsPage />);
    await screen.findByText('VPC giữ nguyên');
    await waitFor(() => expect(screen.getByRole('option', { name: 'vpc', hidden: true })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'Sửa cấu hình vpc-kept' }));
    expect(card('name')).toHaveTextContent('Biểu thức nâng cao');
    expect(card('cidr')).toHaveTextContent('chưa có trong metadata');
    await user.click(screen.getByRole('button', { name: 'Thay đổi name' }));
    const dialog = within(screen.getByRole('dialog'));
    expect(dialog.getByRole('radio', { name: 'Giữ biểu thức nâng cao' })).toBeChecked();
    await user.click(dialog.getByRole('button', { name: 'Áp dụng' }));
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(values(puts[0]?.body).variables).toEqual(variables);
  });
});

describe('UI11-01 infrastructure connection', () => {
  it('serializes the cluster source and the AWS account once through the mapping draft and keeps it read-only while the mapping is JSON', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'pg-conn', type: 'postgres', module: 'postgres-k8s' });
    expect(screen.queryByLabelText('Vùng AWS')).not.toBeInTheDocument();
    const section = screen.getByRole('region', { name: 'Kết nối Kubernetes kubernetes' });
    expect(within(section).getByLabelText('Nhóm cluster của provider kubernetes')).toHaveValue('eks');
    await user.click(screen.getByText('Thiết lập nâng cao · ánh xạ & tài nguyên liên quan'));
    await user.click(screen.getByRole('button', { name: 'Nâng cao: JSON nâng cao' }));
    expect(within(section).getByLabelText('Nhóm cluster của provider kubernetes')).toBeDisabled();
    expect(section).toHaveTextContent('Ánh xạ module đang chỉnh bằng JSON nâng cao');
    // The binding lives only in the mapping JSON now.
    expect(JSON.parse((field('Ánh xạ module JSON') as HTMLTextAreaElement).value).provider_bindings).toEqual({ kubernetes: { target: 'resource', resource_ref: 'k8s-cluster.eks#environments.@app.@env' } });
    await user.click(screen.getByRole('button', { name: 'Nâng cao: biểu mẫu' }));
    expect(within(section).getByLabelText('Nhóm cluster của provider kubernetes')).toBeEnabled();
    await submitDefinition(user);
    await screen.findByText(/^Đã đăng ký cấu hình tài nguyên/);
    expect(values(posted[0]).provider_bindings).toEqual({ kubernetes: { target: 'resource', resource_ref: 'k8s-cluster.eks#environments.@app.@env' } });
  });

  it('renders one AWS card with a single account and region for the VPC module', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    await ready();
    await completeDefinition(user, { id: 'vpc-conn', type: 'vpc', region: 'eu-west-1' });
    expect(screen.getAllByLabelText('Vùng AWS')).toHaveLength(1);
    expect(screen.getAllByLabelText('Tài khoản hạ tầng')).toHaveLength(1);
    expect(card('region')).toBeNull();
    await submitDefinition(user);
    await screen.findByText(/^Đã đăng ký cấu hình tài nguyên/);
    expect(posted[0]).toMatchObject({ driverAccount: 'aws-account' });
    expect((values(posted[0]).variables as Record<string, unknown>).region).toBe('eu-west-1');
    expect(values(posted[0]).provider_bindings).toEqual({ aws: { target: 'account' } });
  });
});

function Harness({ initial, onRows }: { initial: ProvisionRow[]; onRows: (rows: ProvisionRow[]) => void }) {
  const [rows, setRows] = useState(initial);
  return <CompanionResources rows={rows} onChange={(next) => { setRows(next); onRows(next); }} parentType="postgres" types={typeFixture} />;
}
const row = (over: Partial<ProvisionRow> = {}): ProvisionRow => ({ id: 1, descriptor: '', params: '{}', isDependent: false, matchDependents: false, ...over });

describe('UI11-04 companion resources', () => {
  it('names who waits for whom and serializes all four flag combinations', async () => {
    const user = userEvent.setup();
    let latest: ProvisionRow[] = [];
    render(<Harness initial={[row({ descriptor: 'vpc.default#@infra' })]} onRows={(rows) => { latest = rows; }} />);
    expect(screen.getAllByText('postgres', { selector: 'strong' }).length).toBeGreaterThan(0);
    const dependent = screen.getByRole('checkbox', { name: 'Tài nguyên đi kèm 1: vpc chờ postgres tạo xong (is_dependent)' });
    const match = screen.getByRole('checkbox', { name: 'Tài nguyên đi kèm 1: các tài nguyên đang dùng postgres cũng phải chờ vpc (match_dependents)' });
    expect(rowsToProvision([row({ descriptor: 'vpc.default#@infra' })]).value).toEqual({ 'vpc.default#@infra': {} });
    await user.click(dependent);
    expect(rowsToProvision(latest).value).toEqual({ 'vpc.default#@infra': { is_dependent: true } });
    await user.click(match);
    expect(rowsToProvision(latest).value).toEqual({ 'vpc.default#@infra': { is_dependent: true, match_dependents: true } });
    await user.click(dependent);
    expect(rowsToProvision(latest).value).toEqual({ 'vpc.default#@infra': { match_dependents: true } });
    expect(match).toBeChecked();
    expect(dependent).not.toBeChecked();
  });

  it('builds params from the Resource Type schema, not from Terraform variables, and generates the descriptor', async () => {
    const user = userEvent.setup();
    let latest: ProvisionRow[] = [];
    render(<Harness initial={[]} onRows={(rows) => { latest = rows; }} />);
    await user.click(screen.getByRole('button', { name: '+ Thêm tài nguyên đi kèm' }));
    await user.selectOptions(screen.getByLabelText('Loại tài nguyên đi kèm 1'), 'postgres');
    await user.selectOptions(screen.getByLabelText('Nhóm cấu hình tài nguyên đi kèm 1'), 'in-cluster');
    expect(latest[0]?.descriptor).toBe('postgres.in-cluster#environments.@app.@env');
    const params = within(screen.getByLabelText('Params của postgres'));
    expect(['database', 'username'].every((name) => params.getByRole('button', { name: `Cung cấp giá trị ${name}` }))).toBe(true);
    expect(params.queryByRole('button', { name: /storage|namespace/ })).not.toBeInTheDocument();
    await user.click(params.getByRole('button', { name: 'Cung cấp giá trị database' }));
    const dialog = within(screen.getByRole('dialog'));
    await user.click(dialog.getByRole('radio', { name: 'Nhập giá trị' }));
    await user.click(dialog.getByLabelText('Giá trị database'));
    await user.paste('audit');
    await user.click(dialog.getByRole('button', { name: 'Áp dụng' }));
    expect(JSON.parse(latest[0]?.params ?? '')).toEqual({ database: 'audit' });
    expect(rowsToProvision(latest).value).toEqual({ 'postgres.in-cluster#environments.@app.@env': { params: { database: 'audit' } } });
  });

  it('keeps a descriptor or params the cards cannot represent untouched in the technical fields', async () => {
    const onRows = vi.fn();
    render(<Harness initial={[row({ id: 1, descriptor: 'redis.audit', params: '{"x":1}' }), row({ id: 2, descriptor: 'postgres.audit#shared.audit', params: '{"database":"audit","options":{"a":1}}' })]} onRows={onRows} />);
    expect(screen.getByText(/Descriptor này không biểu diễn bằng control/)).toBeInTheDocument();
    expect(screen.getByText(/Param "options" không thuộc schema của Loại postgres/)).toBeInTheDocument();
    expect(screen.getByLabelText('Descriptor tài nguyên đi kèm 1')).toHaveValue('redis.audit');
    expect(screen.getByLabelText('Params tài nguyên đi kèm 2 (JSON object)')).toHaveValue('{"database":"audit","options":{"a":1}}');
    expect(onRows).not.toHaveBeenCalled();
  });
});
