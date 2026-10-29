import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { DeleteApplicationDialog } from './DeleteApplicationDialog';
import { DeletionPanel } from './DeletionPanel';
import { apiError, stubApi, type Call } from '../../../test/api';
import type { DeletionPreview, DeletionProgress } from './api';

afterEach(() => vi.unstubAllGlobals());

const isPreview = (c: Call) => c.method === 'POST' && c.url.endsWith('/applications/shop/deletion-preview');
const isDelete = (c: Call) => c.method === 'POST' && c.url.endsWith('/applications/shop/deletions');

const preview = (over: Partial<DeletionPreview> = {}): DeletionPreview => ({
  token: 'ad1.token', expiresAt: new Date(Date.now() + 600_000).toISOString(), applicationId: 'shop', status: 'ACTIVE', confirmationName: 'Shop',
  environments: [
    { key: 'production', workloads: [], pendingDrafts: [], resources: [], secretValues: 0, deployments: 0, blockers: [] },
    { key: 'staging', workloads: ['api'], pendingDrafts: [], resources: [{ id: 'r1', descriptor: 'postgres.default#modules.api.externals.db', type: 'postgres', class: 'default', status: 'READY', generation: 0 }], secretValues: 2, deployments: 1, blockers: [] },
  ],
  kept: { secretStores: ['platform-vault'], platformResources: 1, definitions: [] }, warnings: [], blocked: false, ...over,
});

function open() {
  const handlers = { onClose: vi.fn(), onAccepted: vi.fn(), onLost: vi.fn() };
  render(<DeleteApplicationDialog applicationId="shop" {...handlers} />);
  return handlers;
}

describe('application deletion dialog (UC-18 MS-01..04)', () => {
  it('shows the scope per environment and sends nothing until the exact name is typed', async () => {
    const calls = stubApi((c) => isPreview(c) ? Response.json({ preview: preview() }) : undefined,
      (c) => isDelete(c) ? Response.json({ deletion: { operationId: 'op-1', status: 'RUNNING', replayed: false } }, { status: 202 }) : undefined);
    const { onAccepted } = open();
    expect(await screen.findByRole('region', { name: 'staging' })).toHaveTextContent('postgres.default#modules.api.externals.db');
    expect(screen.getByText(/Xóa vĩnh viễn dữ liệu, không thể hoàn tác/)).toBeInTheDocument();
    const confirm = screen.getByRole('button', { name: 'Xóa vĩnh viễn ứng dụng' });
    expect(confirm).toBeDisabled();
    await userEvent.type(screen.getByRole('textbox'), 'shop');
    expect(confirm).toBeDisabled();
    await userEvent.clear(screen.getByRole('textbox'));
    await userEvent.type(screen.getByRole('textbox'), 'Shop');
    expect(confirm).toBeEnabled();
    await userEvent.click(confirm);
    await waitFor(() => expect(onAccepted).toHaveBeenCalledWith({ operationId: 'op-1', status: 'RUNNING', replayed: false }));
    expect(calls.filter(isDelete)).toHaveLength(1);
    expect(calls.filter(isDelete)[0]?.body).toEqual({ token: 'ad1.token', confirmationName: 'Shop' });
  });

  it('shows blockers without a confirmation field', async () => {
    stubApi((c) => isPreview(c) ? Response.json({ preview: preview({ token: undefined, blocked: true, environments: [{ key: 'staging', workloads: [], pendingDrafts: [], resources: [], secretValues: 0, deployments: 0, blockers: [{ code: 'ENVIRONMENT_BUSY', operationId: 'op' }] }] }) }) : undefined);
    open();
    expect(await screen.findByText('Môi trường đang có thao tác khác')).toBeInTheDocument();
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
  });

  it('asks for a new review after a stale token and never resends', async () => {
    const calls = stubApi((c) => isPreview(c) ? Response.json({ preview: preview() }) : undefined,
      (c) => isDelete(c) ? apiError(409, 'STALE_PREVIEW', undefined, 'token') : undefined);
    open();
    await userEvent.type(await screen.findByRole('textbox'), 'Shop');
    await userEvent.click(screen.getByRole('button', { name: 'Xóa vĩnh viễn ứng dụng' }));
    expect(await screen.findByRole('button', { name: 'Xem lại phạm vi' })).toBeInTheDocument();
    expect(calls.filter(isDelete)).toHaveLength(1);
  });
});

describe('application deletion progress', () => {
  const progress = (over: Partial<DeletionProgress> = {}): DeletionProgress => ({
    operationId: 'op-1', applicationId: 'shop', applicationStatus: 'DELETING', status: 'FAILED', failureCode: 'CLEANUP_INCOMPLETE', actor: 'developer', attempt: 1, running: false,
    requestedAt: '2026-10-27T00:00:00Z', updatedAt: '2026-10-27T00:00:00Z',
    environments: [{ key: 'staging', stage: 'DESTROYING_RESOURCES', status: 'FAILED', failureCode: 'CLEANUP_INCOMPLETE', removedWorkloads: ['api'], cleanupOperationIds: ['c1'], resources: [{ id: 'r1', descriptor: 'postgres.default#x', outcome: 'REMAINING', remaining: ['kubernetes_stateful_set.db'] }], secretsPurged: 0 }], ...over,
  });

  it('shows the failed stage and remaining objects with a Retry, and reports a finished deletion', async () => {
    stubApi((c) => c.method === 'GET' && c.url.endsWith('/deletions/op-1') ? Response.json({ deletion: progress() }) : undefined);
    const onDeleted = vi.fn();
    const { unmount } = render(<DeletionPanel applicationId="shop" operationId="op-1" onDeleted={onDeleted} onOperation={vi.fn()} pollMs={10} />);
    expect(await screen.findByText(/kubernetes_stateful_set\.db/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Thử lại' })).toBeInTheDocument();
    expect(onDeleted).not.toHaveBeenCalled();
    unmount();
    vi.unstubAllGlobals();
    stubApi((c) => c.method === 'GET' ? Response.json({ deletion: progress({ status: 'SUCCEEDED', failureCode: undefined, applicationStatus: 'DELETED' }) }) : undefined);
    render(<DeletionPanel applicationId="shop" operationId="op-1" onDeleted={onDeleted} onOperation={vi.fn()} pollMs={10} />);
    await waitFor(() => expect(onDeleted).toHaveBeenCalledTimes(1));
  });
});
