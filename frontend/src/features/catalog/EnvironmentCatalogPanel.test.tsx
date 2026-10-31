import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { EnvironmentCatalogPanel } from './EnvironmentCatalogPanel';
afterEach(() => vi.unstubAllGlobals());
it('saves only Environment intent with CAS; retains applied release and strips read metadata', async () => {
  const calls: Record<string, unknown>[] = [];
  let saved = false;
  const initial = { catalog: { revision: 2, status: 'APPLIED', desired: { sourceId: 'company', tag: 'v1.0.0' }, applied: { sourceId: 'company', tag: 'v1.0.0' } }, environmentVersion: 7, releases: [{ sourceId: 'company', tag: 'v1.0.0', fingerprint: 'a'.repeat(64) }, { sourceId: 'company', tag: 'v2.0.0', fingerprint: 'b'.repeat(64) }] };
  vi.stubGlobal('fetch', vi.fn(async (_input: string, init?: RequestInit) => {
    if (init?.method === 'PUT') { calls.push(JSON.parse(String(init.body)) as Record<string, unknown>); saved = true; return Response.json({ catalog: {} }); }
    return Response.json(saved ? { ...initial, catalog: { ...initial.catalog, revision: 3, desired: { sourceId: 'company', tag: 'v2.0.0' }, status: 'PENDING' } } : initial);
  }));
  const onSaved = vi.fn(); const user = userEvent.setup();
  render(<EnvironmentCatalogPanel applicationId="shop" environment="staging" busy={false} onSaved={onSaved} />);
  await user.selectOptions(await screen.findByRole('combobox'), JSON.stringify(['company', 'v2.0.0']));
  await user.click(screen.getByRole('button', { name: 'Lưu lựa chọn' }));
  await screen.findByText('Đã lưu mục tiêu. Preview và Deploy để áp dụng.');
  expect(calls).toEqual([{ sourceId: 'company', tag: 'v2.0.0', expectedEnvironmentVersion: 7, expectedRevision: 2 }]);
  expect(screen.getByText('Đang chạy:').textContent).toContain('v1.0.0');
  expect(onSaved).toHaveBeenCalledOnce();
});
it('does not retry an uncertain save and requires an explicit reload', async () => {
  const user = userEvent.setup(); let puts = 0;
  vi.stubGlobal('fetch', vi.fn(async (_input: string, init?: RequestInit) => {
    if (init?.method === 'PUT') { puts++; throw new Error('network'); }
    return Response.json({ catalog: { revision: 0, status: 'LEGACY' }, environmentVersion: 1, releases: [{ sourceId: 'company', tag: 'v1.0.0' }] });
  }));
  render(<EnvironmentCatalogPanel applicationId="shop" environment="staging" busy={false} onSaved={() => undefined} />);
  await user.selectOptions(await screen.findByRole('combobox'), JSON.stringify(['company', 'v1.0.0']));
  await user.click(screen.getByRole('button', { name: 'Lưu lựa chọn' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Lưu lựa chọn' })).toBeDisabled());
  expect(puts).toBe(1); expect(screen.getByRole('button', { name: 'Tải lại' })).toBeVisible();
});
