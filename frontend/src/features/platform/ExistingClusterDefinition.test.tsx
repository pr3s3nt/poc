import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { capabilityFixture } from '../../test/capabilities';
import { backend, defaultAccounts, field } from '../../test/definitionKit';

// ADR-016 / UC-03 V03: an existing-cluster Definition uses its OWN READY Internal Driver Account.
afterEach(() => vi.unstubAllGlobals());

const stored = { key: 'cluster-lab', name: 'Cluster lab', version: 3, resourceType: 'k8s-cluster', driverType: 'existing-cluster', driverAccount: 'internal-cluster', criteria: [{ class: 'internal' }] };
const section = () => screen.getByRole('region', { name: 'Đăng ký cấu hình tài nguyên' });
const capabilities = [...capabilityFixture, { resourceType: 'k8s-cluster', driverType: 'existing-cluster', module: '', variables: [], executorOwned: [], bindings: [], account: { required: true, type: 'INTERNAL' }, outputs: [{ name: 'cluster', type: 'any', sensitive: false }] }];

async function open(user: ReturnType<typeof userEvent.setup>, options: Parameters<typeof backend>[0] = {}) {
  const harness = backend({ capabilities, ...options });
  render(<ResourceDefinitionsPage />);
  await waitFor(() => expect(screen.getByRole('button', { name: 'Tạo cấu hình' })).toBeEnabled());
  return { user, ...harness };
}

describe('existing-cluster in the shared Definition editor (UI3-05)', () => {
  it('registers with only a READY Internal account, no variables and no connection or credential field', async () => {
    const user = userEvent.setup();
    const { posted } = await open(user);
    expect(screen.queryByLabelText('Tài khoản hạ tầng')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Tạo cấu hình' }));
    await waitFor(() => expect(screen.getByRole('option', { name: 'k8s-cluster' })).toBeEnabled());
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-cluster');
    await user.selectOptions(field('Cách cung cấp cluster'), 'existing-cluster');
    const accounts = await screen.findByLabelText('Tài khoản hạ tầng');
    await waitFor(() => expect(within(accounts).getByRole('option', { name: /Cluster nội bộ/ })).toBeInTheDocument());
    expect(within(accounts).getAllByRole('option').map((option) => option.textContent)).toEqual(['Chọn tài khoản', 'Cluster nội bộ (internal-cluster) · Sẵn sàng · phiên bản 1']);
    expect(screen.queryByLabelText(/kubeconfig|Kết nối riêng|context/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Vùng AWS')).not.toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'Module Terraform' })).not.toBeInTheDocument();
    await user.type(within(section()).getByLabelText('ID cấu hình (Definition ID)'), 'cluster-lab');
    await user.type(within(section()).getByLabelText('Tên hiển thị'), 'Cluster lab');
    await user.click(within(section()).getByRole('button', { name: 'Đăng ký cấu hình tài nguyên' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Chọn tài khoản');
    expect(posted).toEqual([]);
    await user.selectOptions(accounts, 'internal-cluster');
    await user.click(within(section()).getByRole('button', { name: 'Đăng ký cấu hình tài nguyên' }));
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toEqual({ key: 'cluster-lab', name: 'Cluster lab', resourceType: 'k8s-cluster', driverType: 'existing-cluster', driverAccount: 'internal-cluster', driverInputs: { values: { variables: {} } }, provision: {}, criteria: [{}] });
  });

  it('explains the missing READY Internal account instead of offering another kind', async () => {
    const user = userEvent.setup();
    await open(user, { accounts: defaultAccounts.filter((account) => account.type !== 'INTERNAL') });
    await user.click(screen.getByRole('button', { name: 'Tạo cấu hình' }));
    await waitFor(() => expect(screen.getByRole('option', { name: 'k8s-cluster' })).toBeEnabled());
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-cluster');
    await user.selectOptions(field('Cách cung cấp cluster'), 'existing-cluster');
    expect(await screen.findByRole('note', { name: 'Chưa có tài khoản sẵn sàng' })).toBeInTheDocument();
    expect(within(screen.getByLabelText('Tài khoản hạ tầng')).queryByRole('option', { name: /AWS/ })).not.toBeInTheDocument();
  });

  it('edits with the expected version and keeps the ID fixed', async () => {
    const user = userEvent.setup();
    const { puts } = await open(user, { definitions: [stored] });
    await user.click(await screen.findByRole('button', { name: 'Sửa cấu hình cluster-lab' }));
    expect(field('ID cấu hình (Definition ID)')).toHaveAttribute('readonly');
    expect(field('Tài khoản hạ tầng')).toHaveValue('internal-cluster');
    await user.clear(field('Tên hiển thị'));
    await user.type(field('Tên hiển thị'), 'Cluster lab mới');
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Đã lưu cấu hình cluster-lab');
    expect(puts[0]?.body).toMatchObject({ key: 'cluster-lab', name: 'Cluster lab mới', driverAccount: 'internal-cluster', version: 3, criteria: [{ class: 'internal' }] });
  });

  it('keeps the draft and disables Save on a stale version', async () => {
    const user = userEvent.setup();
    const { puts } = await open(user, { definitions: [stored], put: () => Response.json({ code: 'STALE_VERSION' }, { status: 409 }) });
    await user.click(await screen.findByRole('button', { name: 'Sửa cấu hình cluster-lab' }));
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Lưu thay đổi' })).toBeDisabled();
    expect(field('Tên hiển thị')).toHaveValue('Cluster lab');
    expect(puts).toHaveLength(1);
  });
  it('keeps each method draft, common criteria and account when navigating tabs and catalog', async () => {
    const user = userEvent.setup();
    const { puts } = await open(user, { definitions: [stored] });
    await user.click(await screen.findByRole('button', { name: 'Sửa cấu hình cluster-lab' }));
    await user.selectOptions(field('Cách cung cấp cluster'), 'terraform');
    await user.selectOptions(field('Tài khoản hạ tầng'), 'aws-second');
    await user.type(field('Vùng AWS'), 'ap-southeast-1');
    await user.selectOptions(field('Cách cung cấp cluster'), 'existing-cluster');
    expect(field('Tài khoản hạ tầng')).toHaveValue('internal-cluster');
    await user.click(screen.getByRole('tab', { name: /^Điều kiện áp dụng/ }));
    await user.click(screen.getByRole('tab', { name: 'Cấu hình' }));
    await user.click(screen.getByRole('button', { name: 'Cấu hình tài nguyên' }));
    expect(screen.queryByRole('button', { name: 'Đăng ký cấu hình cluster có sẵn' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Tiếp tục bản nháp' }));
    await user.selectOptions(field('Cách cung cấp cluster'), 'terraform');
    expect(field('Tài khoản hạ tầng')).toHaveValue('aws-second');
    expect(field('Vùng AWS')).toHaveValue('ap-southeast-1');
    await user.selectOptions(field('Cách cung cấp cluster'), 'existing-cluster');
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(puts[0]?.body).toMatchObject({ driverType: 'existing-cluster', driverAccount: 'internal-cluster', criteria: stored.criteria, driverInputs: { values: { variables: {} } } });
  });

  it('keeps Git authoring fields mounted while switching cluster methods', async () => {
    const user = userEvent.setup();
    await open(user, { definitions: [stored] });
    await user.click(await screen.findByRole('button', { name: 'Sửa cấu hình cluster-lab' }));
    await user.selectOptions(field('Cách cung cấp cluster'), 'terraform');
    await user.click(screen.getByRole('button', { name: 'Thêm từ Git' }));
    await user.type(field('Git HTTPS URL'), 'https://example.com/cluster.git');
    await user.type(field('Tag hoặc commit'), 'release-v1');
    await user.selectOptions(field('Cách cung cấp cluster'), 'existing-cluster');
    await user.selectOptions(field('Cách cung cấp cluster'), 'terraform');
    expect(field('Git HTTPS URL')).toHaveValue('https://example.com/cluster.git');
    expect(field('Tag hoặc commit')).toHaveValue('release-v1');
  });

});
