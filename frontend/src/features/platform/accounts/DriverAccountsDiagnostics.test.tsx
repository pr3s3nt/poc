import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { stubApi, type Call } from '../../../test/api';
import { DriverAccountsPage } from './DriverAccountsPage';
import type { Account } from './accountsApi';

const internal: Account = {
  id: 'id-2', key: 'lab-cluster', name: 'Lab cluster', type: 'INTERNAL', authenticationType: 'KUBECONFIG', status: 'READY', version: 1,
  config: { cluster: 'lab', kubeContext: 'lab-ctx', endpoint: 'https://lab.example:6443', tls: { mode: 'CUSTOM_CA' } },
  verification: { verified: true, verifiedAt: '2026-10-17T07:00:00Z' },
};
afterEach(() => vi.unstubAllGlobals());

const serve = (accounts: Account[]) => (call: Call) => {
  if (call.method === 'GET' && call.url === '/api/v1/driver-accounts') return Response.json({ accounts });
  const m = /^\/api\/v1\/driver-accounts\/([^/]+)$/.exec(call.url);
  const found = m && call.method === 'GET' ? accounts.find((a) => a.key === m[1]) : undefined;
  return found ? Response.json(found) : undefined;
};

describe('Infrastructure Accounts with diagnostics', () => {
  it('shows connection status apart from the latest diagnostic, scoped and timestamped, and never rewrites the connection', async () => {
    const latest = { reportId: 'r-1234567890', scope: { purpose: 'USE_CASE', mode: 'CONDITIONS', useCases: ['database'], accountKey: 'lab-cluster', accountName: 'Lab cluster', accountType: 'INTERNAL', accountVersion: 1, context: { applicationKey: 'shop', applicationName: 'Shop', environmentKey: 'staging' } },
      status: 'COMPLETED', counts: { pass: 4, fail: 1, blocked: 0, unknown: 0, na: 0, pending: 0 }, ready: false, cleanupStatus: 'NOT_REQUIRED', stale: false, createdAt: '2026-10-25T09:00:00Z' };
    const calls = stubApi(serve([{ ...internal, latestDiagnostic: latest as Account['latestDiagnostic'] }]),
      (call) => call.url.includes('/diagnostics?') ? Response.json({ reports: [], nextCursor: '' }) : undefined);
    render(<DriverAccountsPage />);
    const table = await screen.findByRole('table', { name: 'Tài khoản hạ tầng' });
    expect(within(table).getByText('Sẵn sàng')).toBeInTheDocument();
    const cell = within(table).getByRole('button', { name: /Mở chẩn đoán gần nhất/ });
    expect(cell).toHaveTextContent('1 lỗi');
    expect(cell).toHaveTextContent('Theo use case');
    expect(cell).toHaveTextContent('Shop / staging');
    await userEvent.click(cell);
    expect(await screen.findByRole('navigation', { name: 'Lab cluster' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Chẩn đoán' })).toHaveAttribute('aria-current', 'page');
    // Information keeps every management form; the verify button is the connection check.
    await userEvent.click(screen.getByRole('button', { name: 'Thông tin' }));
    expect((await screen.findAllByRole('button', { name: /Kiểm tra kết nối|Xác minh/ })).length).toBeGreaterThan(0);
    expect(calls.some((c) => c.method !== 'GET')).toBe(false);
  });

  it('does not call an unreadable history "never checked"', async () => {
    stubApi(serve([{ ...internal, diagnosticsUnavailable: true }]));
    render(<DriverAccountsPage />);
    const table = await screen.findByRole('table', { name: 'Tài khoản hạ tầng' });
    expect(within(table).getByText('Không đọc được lịch sử chẩn đoán')).toBeInTheDocument();
    expect(within(table).queryByText('Chưa chẩn đoán')).not.toBeInTheDocument();
  });
});
