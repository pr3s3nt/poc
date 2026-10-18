import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { backend, defaultAccounts, field } from '../../test/definitionKit';

// ADR-016 / UC-03 V03: an existing-cluster Definition uses its OWN READY Internal Driver Account.
afterEach(() => vi.unstubAllGlobals());

const stored = { key: 'cluster-lab', name: 'Cluster lab', version: 3, resourceType: 'k8s-cluster', driverType: 'existing-cluster', driverAccount: 'internal-cluster', criteria: [{ class: 'internal' }] };
const section = () => screen.getByRole('region', { name: 'Cluster có sẵn (existing-cluster)' });

async function open(user: ReturnType<typeof userEvent.setup>, options: Parameters<typeof backend>[0] = {}) {
  const harness = backend(options);
  render(<ResourceDefinitionsPage />);
  await screen.findByText(/Cluster có sẵn không bao giờ bị tạo hay xóa/);
  return { user, ...harness };
}

describe('existing-cluster Definition section (V03)', () => {
  it('registers with only a READY Internal account, no variables and no connection or credential field', async () => {
    const user = userEvent.setup();
    const { posted } = await open(user);
    expect(screen.queryByLabelText('Tài khoản Internal của cluster')).not.toBeInTheDocument();
    await user.click(within(section()).getByRole('button', { name: 'Đăng ký cấu hình cluster có sẵn' }));
    const accounts = await screen.findByLabelText('Tài khoản Internal của cluster');
    await waitFor(() => expect(within(accounts).getByRole('option', { name: /Cluster nội bộ/ })).toBeInTheDocument());
    expect(within(accounts).getAllByRole('option').map((option) => option.textContent)).toEqual(['Chọn tài khoản Internal READY', 'Cluster nội bộ (internal-cluster)']);
    expect(screen.queryByLabelText(/kubeconfig|Kết nối|context/i)).not.toBeInTheDocument();
    await user.type(within(section()).getByLabelText('ID cấu hình cluster'), 'cluster-lab');
    await user.type(within(section()).getByLabelText('Tên hiển thị cluster'), 'Cluster lab');
    await user.click(within(section()).getByRole('button', { name: 'Đăng ký cấu hình cluster' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Chọn tài khoản Internal READY');
    expect(posted).toEqual([]);
    await user.selectOptions(accounts, 'internal-cluster');
    await user.click(within(section()).getByRole('button', { name: 'Đăng ký cấu hình cluster' }));
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toEqual({ key: 'cluster-lab', name: 'Cluster lab', resourceType: 'k8s-cluster', driverType: 'existing-cluster', driverAccount: 'internal-cluster', driverInputs: { values: { variables: {} } }, criteria: [{}] });
  });

  it('explains the missing READY Internal account instead of offering another kind', async () => {
    const user = userEvent.setup();
    await open(user, { accounts: defaultAccounts.filter((account) => account.type !== 'INTERNAL') });
    await user.click(within(section()).getByRole('button', { name: 'Đăng ký cấu hình cluster có sẵn' }));
    expect(await screen.findByText('Chưa có tài khoản Internal READY. Hãy đăng ký và xác minh tài khoản trước.')).toBeInTheDocument();
    expect(within(screen.getByLabelText('Tài khoản Internal của cluster')).queryByRole('option', { name: /AWS/ })).not.toBeInTheDocument();
  });

  it('edits with the expected version, keeps the ID fixed and holds Save after a stale 409 without resending', async () => {
    const user = userEvent.setup();
    const { puts } = await open(user, { definitions: [stored] });
    await user.click(await screen.findByRole('button', { name: 'Sửa cluster cluster-lab' }));
    expect(field('ID cấu hình cluster')).toHaveAttribute('readonly');
    expect(field('Tài khoản Internal của cluster')).toHaveValue('internal-cluster');
    await user.clear(field('Tên hiển thị cluster'));
    await user.type(field('Tên hiển thị cluster'), 'Cluster lab mới');
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi cluster' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Đã lưu cấu hình cluster cluster-lab (phiên bản 4).');
    expect(puts[0]?.body).toMatchObject({ key: 'cluster-lab', name: 'Cluster lab mới', driverAccount: 'internal-cluster', version: 3, criteria: [{ class: 'internal' }] });
  });

  it('keeps the draft and disables Save on a stale version', async () => {
    const user = userEvent.setup();
    const { puts } = await open(user, { definitions: [stored], put: () => Response.json({ code: 'STALE_VERSION' }, { status: 409 }) });
    await user.click(await screen.findByRole('button', { name: 'Sửa cluster cluster-lab' }));
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi cluster' }));
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Lưu thay đổi cluster' })).toBeDisabled();
    expect(field('Tên hiển thị cluster')).toHaveValue('Cluster lab');
    expect(puts).toHaveLength(1);
  });
});
