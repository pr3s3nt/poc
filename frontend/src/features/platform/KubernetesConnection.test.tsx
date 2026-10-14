import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { backend, completeDefinition, field, submitDefinition } from '../../test/definitionKit';

const connections = [
  { key: 'lab', name: 'Lab cluster', kind: 'KUBERNETES', status: 'READY' },
  { key: 'edge', kind: 'KUBERNETES', status: 'READY' },
  { key: 'verifying', kind: 'KUBERNETES', status: 'VERIFYING' },
  { key: 'aws-main', kind: 'AWS', status: 'READY' },
];
const advanced = 'Kết nối riêng cho cluster (tùy chọn)';

async function ready() {
  render(<ResourceDefinitionsPage />);
  await screen.findByText('Chưa có cấu hình tài nguyên nào.');
  await waitFor(() => expect(screen.getByRole('option', { name: 'postgres' })).toBeEnabled());
}
// Fills a Kubernetes module Definition, leaving the optional Connection alone.
async function fillPostgres(user: ReturnType<typeof userEvent.setup>, id = 'pg-k8s') {
  await completeDefinition(user, { id, type: 'postgres', module: 'postgres-k8s' });
}

afterEach(() => vi.unstubAllGlobals());

describe('UC-03 optional same-Environment Kubernetes Connection (I04-07)', () => {
  it('is an advanced option of a Kubernetes module, empty by default: nothing is sent', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ connections });
    await ready();
    await fillPostgres(user);
    const control = await screen.findByLabelText(advanced);
    expect(control).toHaveValue('');
    expect(within(control).getByRole('option', { name: 'Dùng Kết nối của Môi trường (mặc định)' })).toBeInTheDocument();
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên pg-k8s.');
    expect(posted[0]).not.toHaveProperty('connectionKey');
  });

  it('lists READY Kubernetes Connections only and sends the chosen key; the form never picks one for the author', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ connections });
    await ready();
    await fillPostgres(user);
    const control = await screen.findByLabelText(advanced);
    await waitFor(() => expect(within(control).getAllByRole('option')).toHaveLength(3));
    expect(within(control).getAllByRole('option').map((option) => option.textContent)).toEqual(['Dùng Kết nối của Môi trường (mặc định)', 'Lab cluster (lab)', 'edge']);
    expect(screen.getByText(/Kết nối riêng không đổi cluster đích: khi lập kế hoạch, Kết nối này phải trùng với Kết nối của Môi trường/)).toBeInTheDocument();
    await user.selectOptions(control, 'lab');
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên pg-k8s.');
    expect(posted[0]).toMatchObject({ connectionKey: 'lab', driverType: 'terraform', executionProfile: 'internal-k8s' });
    // A committed registration resets the choice with the rest of the draft.
    expect(await screen.findByLabelText(advanced)).toHaveValue('');
  });

  it('AWS modules never offer a Connection, account or region, and never read the Connection list', async () => {
    const user = userEvent.setup();
    const { gets } = backend({ connections });
    await ready();
    await completeDefinition(user, { id: 'vpc-aws', type: 'vpc' });
    expect(screen.queryByLabelText(advanced)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/Kết nối AWS|Tài khoản|Vùng|Region/i)).not.toBeInTheDocument();
    expect(gets.some((url) => url.endsWith('/connections'))).toBe(false);
  });

  it('a Kubernetes module scoped to AWS has no Kubernetes Connection and drops a previous choice', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ connections });
    await ready();
    await fillPostgres(user);
    await user.selectOptions(await screen.findByLabelText(advanced), 'lab');
    await user.selectOptions(field('Phạm vi triển khai'), 'aws-eks');
    expect(screen.queryByLabelText(advanced)).not.toBeInTheDocument();
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký cấu hình tài nguyên pg-k8s.');
    expect(posted[0]).not.toHaveProperty('connectionKey');
    expect(posted[0]).toMatchObject({ executionProfile: 'aws-eks' });
  });

  it('keeps the choice per module pin while switching pins and scopes', async () => {
    const user = userEvent.setup();
    backend({ connections });
    await ready();
    await fillPostgres(user, 'pg-keep');
    await user.selectOptions(await screen.findByLabelText(advanced), 'edge');
    await user.selectOptions(field('Phạm vi triển khai'), 'aws-eks');
    await user.selectOptions(field('Phạm vi triển khai'), 'internal-k8s');
    expect(await screen.findByLabelText(advanced)).toHaveValue('edge');
    await user.selectOptions(field('Loại tài nguyên'), 'k8s-namespace');
    expect(await screen.findByLabelText(advanced)).toHaveValue('');
  });

  it('a list failure is a distinct, retryable state that keeps every edit; an unchosen option still registers', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ connectionLists: [() => Response.json({ code: 'INTERNAL_ERROR' }, { status: 500 }), () => Response.json({ connections })] });
    await ready();
    await fillPostgres(user, 'pg-retry');
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải kết nối' });
    expect(alert).toHaveTextContent('Dữ liệu đã nhập được giữ nguyên');
    expect(field('ID cấu hình')).toHaveValue('pg-retry');
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await waitFor(() => expect(screen.queryByRole('alert', { name: 'Lỗi tải kết nối' })).not.toBeInTheDocument());
    await user.selectOptions(screen.getByLabelText(advanced), 'lab');
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toBeInTheDocument();
    expect(posted[0]).toMatchObject({ connectionKey: 'lab' });
  });
});
