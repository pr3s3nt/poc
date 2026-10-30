import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { PlatformSettingsPage } from './PlatformSettingsPage';

const saved = { url: 'https://git.example.com/platform', username: 'reader', authenticated: true, caPem: '', tokenConfigured: true, version: 1 };
function response(status: number, value: unknown) { return new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } }); }
afterEach(() => vi.unstubAllGlobals());

it('keeps the stored token blank and sends versioned updates', async () => {
  let submitted: Record<string, unknown> = {};
  vi.stubGlobal('fetch', vi.fn(async (_input: unknown, init?: RequestInit) => {
    if (init?.method === 'PUT') {
      submitted = JSON.parse(String(init.body)) as Record<string, unknown>;
      return response(200, { settings: { ...saved, version: 2 } });
    }
    return response(200, { settings: saved });
  }));
  const user = userEvent.setup(); render(<PlatformSettingsPage />);
  const token = await screen.findByLabelText('Token');
  expect(token).toHaveValue('');
  await user.type(token, 'replacement-token');
  await user.click(screen.getByRole('button', { name: 'Lưu', exact: true }));
  await screen.findByText('Đã lưu cấu hình Git.');
  expect(submitted.token).toBe('replacement-token');
  expect(submitted.expectedVersion).toBe(1);
  expect(token).toHaveValue('');
});

it('blocks stale writes until settings are reloaded and clears entered token', async () => {
  vi.stubGlobal('fetch', vi.fn(async (_input: unknown, init?: RequestInit) => init?.method === 'PUT' ? response(409, { code: 'STALE_VERSION' }) : response(200, { settings: saved })));
  const user = userEvent.setup(); render(<PlatformSettingsPage />);
  const token = await screen.findByLabelText('Token');
  await user.type(token, 'new-token');
  await user.click(screen.getByRole('button', { name: 'Lưu', exact: true }));
  await screen.findByText('Cấu hình đã thay đổi. Tải lại và xem lại trước khi lưu.');
  expect(token).toHaveValue('');
  expect(screen.getByRole('button', { name: 'Lưu', exact: true })).toBeDisabled();
  await user.click(screen.getByRole('button', { name: 'Tải lại cấu hình' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Lưu', exact: true })).toBeEnabled());
});

it('tests a draft without saving and invalidates its result on edit', async () => {
  const methods: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (_input: unknown, init?: RequestInit) => {
    methods.push(init?.method ?? 'GET');
    return response(200, init?.method === 'POST' ? { reachable: true } : { settings: saved });
  }));
  const user = userEvent.setup(); render(<PlatformSettingsPage />);
  await screen.findByLabelText('Token');
  await user.click(screen.getByRole('button', { name: 'Kiểm tra kết nối' }));
  await screen.findByText('Đọc được repo và ref này. Cấu hình chưa được lưu bởi thao tác kiểm tra.');
  expect(methods).toEqual(['GET', 'POST']);
  await user.type(screen.getByLabelText('URL Git / phạm vi'), '/more');
  expect(screen.queryByText('Đọc được repo và ref này. Cấu hình chưa được lưu bởi thao tác kiểm tra.')).not.toBeInTheDocument();
});
