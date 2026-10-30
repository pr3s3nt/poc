import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { InfrastructureRepairDialog } from './InfrastructureRepairDialog';
import type { DriftScan } from './InfrastructureDriftPanel';

afterEach(() => vi.unstubAllGlobals());
const report = { externalDrift: [], proposedActions: [{ address: 'kubernetes_namespace.this', type: 'kubernetes_namespace', actions: ['update'], paths: ['/metadata/labels'] }], outputNames: [], complete: true, outputsUnknown: false };
const scan: DriftScan = { id: 'scan', status: 'SUCCEEDED', result: 'DRIFTED', createdAt: new Date().toISOString(), findings: [{ instanceId: 'namespace', descriptor: 'namespace.default#app', result: 'DRIFTED', report }] };
const ready = () => ({ review: { id: 'review', status: 'READY', expiresAt: new Date(Date.now() + 600_000).toISOString(), blockers: [] as string[], instances: [{ instanceId: 'namespace', report, blockers: [] as string[] }] }, reviewToken: 'private-confirmation-token' });

it('requires review and explicit confirmation; displays only paths and actions', async () => {
  const done = vi.fn();
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/applications/app')) return Response.json({ application: { environments: [{ key: 'staging', version: 9 }] } });
    if (url.endsWith('/drift-repairs/preview')) return Response.json(ready(), { status: 202 });
    if (url.endsWith('/drift-repairs') && init?.method === 'POST') return Response.json({ operationId: 'repair-operation' }, { status: 202 });
    if (url.endsWith('/operations')) return Response.json({ operations: [{ id: 'repair-operation', status: 'SUCCEEDED' }] });
    return Response.json({});
  }));
  render(<InfrastructureRepairDialog applicationId="app" environment="staging" scan={scan} onClose={vi.fn()} onComplete={done} />);
  expect(screen.queryByRole('button', { name: 'Xác nhận sửa' })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Chuẩn bị phương án' }));
  const confirm = await screen.findByRole('button', { name: 'Xác nhận sửa' });
  expect(confirm).toBeDisabled();
  expect(screen.getByText('/metadata/labels')).toBeInTheDocument();
  expect(screen.queryByText('private-confirmation-token')).not.toBeInTheDocument();
  expect(vi.mocked(fetch).mock.calls.filter(([url, init]) => String(url).endsWith('/drift-repairs') && init?.method === 'POST')).toHaveLength(0);
  await userEvent.click(screen.getByRole('checkbox', { name: 'Tôi đã xem phương án sửa' }));
  await userEvent.click(confirm);
  await screen.findByText('Đã sửa hạ tầng và xác nhận trạng thái.');
  const posts = vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === 'POST');
  expect(posts).toHaveLength(2);
  expect(JSON.parse(String(posts[0]?.[1]?.body))).toEqual({ scanId: 'scan', instanceIds: ['namespace'], expectedEnvironmentVersion: 9, requestId: expect.any(String) });
  expect(JSON.parse(String(posts[1]?.[1]?.body))).toEqual({ reviewId: 'review', reviewToken: 'private-confirmation-token', requestId: expect.any(String) });
  await userEvent.click(screen.getByRole('button', { name: 'Hoàn tất' }));
  expect(done).toHaveBeenCalledOnce();
});

it('shows replacement/output blockers without a confirmation action', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    if (String(input).endsWith('/applications/app')) return Response.json({ application: { environments: [{ key: 'staging', version: 9 }] } });
    const view = ready(); view.review.status = 'BLOCKED'; view.review.instances[0]!.blockers = ['IN_PLACE_STABLE_OUTPUTS_REQUIRED'];
    return Response.json(view, { status: 202 });
  }));
  render(<InfrastructureRepairDialog applicationId="app" environment="staging" scan={scan} onClose={vi.fn()} onComplete={vi.fn()} />);
  await userEvent.click(screen.getByRole('button', { name: 'Chuẩn bị phương án' }));
  expect(await screen.findByText(/Chỉ sửa tại chỗ khi đầu ra không đổi/)).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Xác nhận sửa' })).not.toBeInTheDocument();
});

it('retries a lost confirmation with exactly the original review and request id', async () => {
  const commands: unknown[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/applications/app')) return Response.json({ application: { environments: [{ key: 'staging', version: 9 }] } });
    if (url.endsWith('/drift-repairs/preview')) return Response.json(ready(), { status: 202 });
    if (url.endsWith('/drift-repairs') && init?.method === 'POST') { commands.push(JSON.parse(String(init.body))); throw new Error('Response lost'); }
    return Response.json({});
  }));
  render(<InfrastructureRepairDialog applicationId="app" environment="staging" scan={scan} onClose={vi.fn()} onComplete={vi.fn()} />);
  await userEvent.click(screen.getByRole('button', { name: 'Chuẩn bị phương án' }));
  await userEvent.click(await screen.findByRole('checkbox', { name: 'Tôi đã xem phương án sửa' }));
  await userEvent.click(screen.getByRole('button', { name: 'Xác nhận sửa' }));
  await userEvent.click(await screen.findByRole('button', { name: 'Gửi lại xác nhận' }));
  await waitFor(() => expect(commands).toHaveLength(2));
  expect(commands[1]).toEqual(commands[0]);
});
