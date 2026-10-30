import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { DeliverySelectionPanel } from './DeliverySelectionPanel';

const direct = { mode: 'DIRECT', selfHeal: false };
const initial = { desired: direct, applied: direct, environmentVersion: 4, configurationRevision: 2, pending: false };
const binding = { id: 'fleet-binding', version: 1, provider: 'fleet', name: 'Internal platform', status: 'READY', capabilities: { selfHeal: true } };
afterEach(() => vi.unstubAllGlobals());

function setup(canEdit = true, failure = false) {
  const writes: unknown[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    if (String(input).endsWith('/delivery-bindings')) return Response.json({ bindings: [binding] });
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

it('saves staging intent with both guards and keeps applied direct until deployment', async () => {
  const { writes, saved } = setup(); const user = userEvent.setup();
  const checkbox = await screen.findByRole('checkbox', { name: 'Dùng CD provider' });
  expect(checkbox).not.toBeChecked();
  await user.click(checkbox);
  await user.selectOptions(screen.getByLabelText('CD provider'), 'fleet-binding:1');
  expect(screen.getByRole('checkbox', { name: 'Tự sửa sai lệch' })).not.toBeChecked();
  expect(writes).toHaveLength(0);
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
  expect(await screen.findByText('Đã lưu. Xem trước để áp dụng.')).toBeInTheDocument();
  expect(writes).toEqual([{ mode: 'CD', bindingId: 'fleet-binding', bindingVersion: 1, selfHeal: false, expectedEnvironmentVersion: 4, expectedConfigurationRevision: 2 }]);
  expect(screen.getByText('Đang dùng: Trực tiếp')).toBeInTheDocument();
  expect(screen.getByText('Chờ áp dụng')).toBeInTheDocument(); expect(saved).toHaveBeenCalledOnce();
});

it('requires explicit reload after a stale mutation and never replays it', async () => {
  const { writes } = setup(true, true); const user = userEvent.setup();
  await user.click(await screen.findByRole('checkbox', { name: 'Dùng CD provider' }));
  await user.selectOptions(screen.getByLabelText('CD provider'), 'fleet-binding:1');
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
  expect(await screen.findByRole('alert')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu thay đổi' })).toBeDisabled();
  expect(screen.getByRole('checkbox', { name: 'Dùng CD provider' })).toBeChecked();
  await user.click(screen.getByRole('button', { name: 'Tải lại' }));
  await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Dùng CD provider' })).not.toBeChecked());
  expect(writes).toHaveLength(1);
});

it('shows developer read-only selection', async () => {
  const { writes } = setup(false);
  expect(await screen.findByRole('checkbox', { name: 'Dùng CD provider' })).toBeDisabled();
  expect(screen.queryByRole('button', { name: 'Lưu thay đổi' })).not.toBeInTheDocument();
  expect(writes).toHaveLength(0);
});
