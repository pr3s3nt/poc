import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { DatabasePurgeDialog } from './DatabasePurgeDialog';
import { stubApi, type Call } from '../../../test/api';

afterEach(() => vi.unstubAllGlobals());

it('requires exact app name and confirmation that a held writer has stopped', async () => {
  const isPurge = (call: Call) => call.url.endsWith('/applications/shop/database-purge');
  const calls = stubApi(
    call => isPurge(call) && call.method === 'GET'
      ? Response.json({ preview: {
        applicationId: 'immutable-id', displayName: 'Shop', busy: false, priorStopRequired: true,
        environments: [{ key: 'staging', namespace: 'shop-staging' }], fleet: [], resources: [], gitRepositories: [],
      } })
      : undefined,
    call => isPurge(call) && call.method === 'POST' ? new Response(null, { status: 204 }) : undefined,
  );
  const onDeleted = vi.fn();
  const user = userEvent.setup();
  render(<DatabasePurgeDialog applicationId="shop" onClose={vi.fn()} onDeleted={onDeleted} />);
  const name = await screen.findByRole('textbox');
  const confirm = screen.getByRole('button', { name: 'Xóa app khỏi DB' });
  await user.type(name, 'shop');
  await user.click(screen.getByRole('checkbox'));
  expect(confirm).toBeDisabled();
  await user.clear(name);
  await user.type(name, 'Shop');
  await user.click(screen.getByRole('checkbox'));
  expect(confirm).toBeDisabled();
  expect(calls.filter(call => call.method === 'POST')).toHaveLength(0);
  await user.click(screen.getByRole('checkbox'));
  await user.click(confirm);
  await waitFor(() => expect(onDeleted).toHaveBeenCalledOnce());
  expect(calls.filter(call => call.method === 'POST').map(call => call.body)).toEqual([
    { applicationId: 'immutable-id', confirmationName: 'Shop', priorExecutionStopped: true },
  ]);
});
