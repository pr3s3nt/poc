import { render, screen, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { InfrastructureDriftPanel } from './InfrastructureDriftPanel';

afterEach(() => vi.unstubAllGlobals());
const report = { externalDrift: [{ address: 'kubernetes_namespace.app', type: 'kubernetes_namespace', actions: ['update'], paths: ['metadata.labels'] }], proposedActions: [], outputNames: [], complete: true, outputsUnknown: false };
const scan = { id: 'scan', status: 'SUCCEEDED', result: 'DRIFTED', createdAt: new Date().toISOString(), finishedAt: new Date().toISOString(), findings: [{ instanceId: 'namespace', descriptor: 'namespace.default#app', result: 'DRIFTED', report }] };

it('reads stored findings for developers without offering provider mutations', async () => {
  const methods: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (_input: string | URL | Request, init?: RequestInit) => { methods.push(init?.method ?? 'GET'); return Response.json({ scans: [scan] }); }));
  render(<InfrastructureDriftPanel applicationId="app" environment="staging" canManage={false} busy={false} />);
  const panel = screen.getByRole('region', { name: 'Sai lệch hạ tầng' });
  expect(await within(panel).findByText('Có sai lệch')).toBeInTheDocument();
  expect(within(panel).queryByRole('button', { name: 'Kiểm tra sai lệch' })).not.toBeInTheDocument();
  await userEvent.click(within(panel).getByText('Chi tiết kiểm tra'));
  expect(within(panel).getByText('metadata.labels')).toBeInTheDocument();
  await userEvent.click(within(panel).getByRole('button', { name: 'Tải lại' }));
  expect(methods.every(method => method === 'GET')).toBe(true);
});

it('queues only on an explicit click with the current Environment version', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    if (String(input).endsWith('/applications/app')) return Response.json({ application: { environments: [{ key: 'staging', version: 7, targetGeneration: 0 }] } });
    if (init?.method === 'POST') return Response.json({ scanId: 'new-scan' }, { status: 202 });
    return Response.json({ scans: [] });
  }));
  render(<InfrastructureDriftPanel applicationId="app" environment="staging" canManage busy={false} />);
  await screen.findByText('Chưa có lần kiểm tra.');
  expect(vi.mocked(fetch).mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false);
  await userEvent.click(screen.getByRole('button', { name: 'Kiểm tra sai lệch' }));
  await waitFor(() => expect(vi.mocked(fetch).mock.calls.some(([, init]) => init?.method === 'POST')).toBe(true));
  const post = vi.mocked(fetch).mock.calls.find(([, init]) => init?.method === 'POST');
  expect(String(post?.[0])).toBe('/api/v1/applications/app/environments/staging/drift-scans');
  expect(JSON.parse(String(post?.[1]?.body))).toEqual({ expectedEnvironmentVersion: 7, requestId: expect.any(String) });
});

it('retains computed-only drift without offering an apply and marks stale results unknown', async () => {
  vi.stubGlobal('fetch',vi.fn(async()=>Response.json({scans:[{...scan,fresh:false,effectiveResult:'UNKNOWN',findings:[{...scan.findings[0],report:{...report,proposedActions:[]}}]}]})));
  render(<InfrastructureDriftPanel applicationId="app" environment="staging" canManage busy={false}/>);
  expect(await screen.findByText(/Kết quả cũ/)).toBeInTheDocument();
  expect(screen.queryByRole('button',{name:'Xem phương án sửa'})).not.toBeInTheDocument();
  expect(screen.getByRole('status')).toHaveTextContent('Chưa xác định');
});

it('retries an unconfirmed request with the same id and original version', async () => {
  const commands: unknown[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    if (String(input).endsWith('/applications/app')) return Response.json({ application: { environments: [{ key: 'staging', version: 7 }] } });
    if (init?.method === 'POST') { commands.push(JSON.parse(String(init.body))); throw new Error('Response lost'); }
    return Response.json({ scans: [] });
  }));
  render(<InfrastructureDriftPanel applicationId="app" environment="staging" canManage busy={false} />);
  await screen.findByText('Chưa có lần kiểm tra.');
  await userEvent.click(screen.getByRole('button', { name: 'Kiểm tra sai lệch' }));
  await userEvent.click(await screen.findByRole('button', { name: 'Gửi lại yêu cầu' }));
  expect(commands).toHaveLength(2);
  expect(commands[1]).toEqual(commands[0]);
});
