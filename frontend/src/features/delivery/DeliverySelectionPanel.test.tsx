import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { DeliverySelectionPanel } from './DeliverySelectionPanel';

const direct = { mode: 'DIRECT', selfHeal: false };
const initial = { desired: direct, applied: direct, environmentVersion: 4, configurationRevision: 2, pending: false };
const binding = { id: 'fleet-binding', version: 1, provider: 'fleet', name: 'Internal platform', status: 'READY', capabilities: { selfHeal: true } };
afterEach(() => vi.unstubAllGlobals());

function setup(canEdit = true, failure = false, choices = [binding]) {
  const writes: unknown[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    if (String(input).endsWith('/delivery-bindings')) return Response.json({ bindings: choices });
    if (init?.method === 'PUT') {
      const body = JSON.parse(String(init.body)) as { mode: string; bindingId?: string; bindingVersion?: number; selfHeal: boolean };
      writes.push(body);
      if (failure) return Response.json({ code: 'STALE_VERSION' }, { status: 409 });
      return Response.json({ ...initial, desired: body, pending: true, environmentVersion: 5, configurationRevision: 3 });
    }
    return Response.json(initial);
  }));
  const saved = vi.fn(), dirty = vi.fn();
  render(<DeliverySelectionPanel applicationId="shop" environment="staging" canEdit={canEdit} busy={false} onDirtyChange={dirty} onSaved={saved} />);
  return { writes, saved, dirty };
}

it('requires a CD binding and saves intent without applying it', async () => {
  const { writes, saved } = setup(); const user = userEvent.setup();
  await screen.findByLabelText('CD provider');
  expect(screen.queryByRole('checkbox', { name: 'Dùng CD provider' })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu thay đổi' })).toBeDisabled();
  await user.selectOptions(screen.getByLabelText('CD provider'), 'fleet-binding:1');
  expect(screen.getByRole('checkbox', { name: 'Tự sửa sai lệch' })).not.toBeChecked();
  expect(writes).toHaveLength(0);
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
  expect(await screen.findByText('Đã lưu. Xem trước để áp dụng.')).toBeInTheDocument();
  expect(writes).toEqual([{ mode: 'CD', bindingId: 'fleet-binding', bindingVersion: 1, selfHeal: false, expectedEnvironmentVersion: 4, expectedConfigurationRevision: 2 }]);
  expect(screen.getByText('Đang dùng: Chưa áp dụng CD')).toBeInTheDocument();
  expect(screen.getByText('Chờ áp dụng')).toBeInTheDocument(); expect(saved).toHaveBeenCalledOnce();
});

it('requires explicit reload after a stale mutation and never replays it', async () => {
  const { writes } = setup(true, true); const user = userEvent.setup();
  await screen.findByLabelText('CD provider');
  await user.selectOptions(screen.getByLabelText('CD provider'), 'fleet-binding:1');
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
  expect(await screen.findByRole('alert')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu thay đổi' })).toBeDisabled();
  expect(screen.getByLabelText('CD provider')).toHaveValue('fleet-binding:1');
  await user.click(screen.getByRole('button', { name: 'Tải lại' }));
  await waitFor(() => expect(screen.getByLabelText('CD provider')).toHaveValue(''));
  expect(writes).toHaveLength(1);
});

it('shows developer read-only selection', async () => {
  const { writes } = setup(false);
  expect(await screen.findByLabelText('CD provider')).toBeDisabled();
  expect(screen.queryByRole('button', { name: 'Lưu thay đổi' })).not.toBeInTheDocument();
  expect(writes).toHaveLength(0);
});

it('keeps deployment configuration incomplete when no verified provider exists', async () => {
  const { writes } = setup(true, false, []);
  expect(await screen.findByLabelText('CD provider')).toHaveValue('');
  expect(screen.getByRole('button', { name: 'Lưu thay đổi' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Cấu hình CD' })).toBeInTheDocument();
  expect(screen.queryByRole('checkbox', { name: 'Dùng CD provider' })).not.toBeInTheDocument();
  expect(writes).toHaveLength(0);
});
