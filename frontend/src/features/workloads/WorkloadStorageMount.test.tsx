import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import { bothConfigured } from '../../test/targets';
import type { ResourceType, Workload } from './api';

afterEach(() => vi.unstubAllGlobals());
const application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: bothConfigured(), workloads: { staging: [], production: [] } };
const execution = { version: 1, kind: 'Deployment', template: { id: 'backend', version: '1.0.0', sha256: 'a'.repeat(64) } };
const storage: ResourceType = { key: 'persistent-storage', inputs: [{ name: 'storage', type: 'string' }], outputs: [{ name: 'claimName', type: 'string' }, { name: 'namespace', type: 'string' }], classes: ['default'] };
const redis: ResourceType = { key: 'redis', inputs: [], outputs: [], classes: ['default'] };
function setup({ types = [storage, redis], resources, others = [] }: { types?: ResourceType[]; resources?: Record<string, unknown>; others?: Workload[] } = {}) {
  const puts: { score: Record<string, unknown> }[] = [];
  const score = { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/backend:v1' } }, workloadExecution: execution, ...(resources ? { resources } : {}) };
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ version: 1, keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: types });
    if (init?.method === 'PUT') { puts.push(JSON.parse(String(init.body))); return Response.json({ workloads: [], draftVersion: 2 }); }
    return Response.json({ draftVersion: 1, workloads: [{ id: 'api', ready: false, state: 'PENDING_UPSERT', score }, ...others] });
  }));
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  return { puts, user: userEvent.setup() };
}

it('adds storage and mount from Add volume mount, while Cancel keeps the local draft unchanged', async () => {
  const { puts, user } = setup();
  const add = await screen.findByRole('button', { name: 'Thêm volume mount' });
  expect(add).toBeEnabled();
  await user.click(add);
  let dialog = screen.getByRole('dialog', { name: 'Thêm tài nguyên lưu trữ' });
  expect(within(dialog).getByLabelText('Alias tài nguyên')).toHaveValue('data');
  expect(within(dialog).getByLabelText('Loại tài nguyên')).toHaveValue('persistent-storage');
  expect(within(dialog).queryByRole('option', { name: 'redis' })).not.toBeInTheDocument();
  await user.keyboard('{Escape}');
  expect(screen.queryByLabelText('Mount path')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Alias tài nguyên')).not.toBeInTheDocument();
  expect(add).toHaveFocus();
  await user.click(add);
  dialog = screen.getByRole('dialog', { name: 'Thêm tài nguyên lưu trữ' });
  await user.type(within(dialog).getByLabelText('Tham số storage'), '5Gi');
  await user.click(within(dialog).getByRole('button', { name: 'Thêm và chọn' }));
  expect(screen.getByLabelText('Mount path')).toHaveValue('/data');
  expect(puts).toHaveLength(0);
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(puts[0]?.score.resources).toEqual({ data: { type: 'persistent-storage', id: expect.stringMatching(/^resource-/), params: { storage: '5Gi' } } });
  expect(puts[0]?.score.workloadExecution).toMatchObject({ ...execution, mounts: [{ resource: 'data', container: 'main', path: '/data', readOnly: false }] });
});

it('reuses an existing scoped storage resource from the mount dialog without creating a new identity', async () => {
  const { puts, user } = setup({ others: [{ id: 'worker', ready: true, score: { resources: { volume: { type: 'persistent-storage', id: 'store-a', params: { storage: '2Gi' } } } } }] });
  await user.click(await screen.findByRole('button', { name: 'Thêm volume mount' }));
  const dialog = screen.getByRole('dialog', { name: 'Thêm tài nguyên lưu trữ' });
  await user.selectOptions(within(dialog).getByLabelText('Nguồn tài nguyên'), 'shared:store-a');
  await user.click(within(dialog).getByRole('button', { name: 'Thêm và chọn' }));
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(puts[0]?.score.resources).toEqual({ data: { type: 'persistent-storage', id: 'store-a', params: { storage: '2Gi' } } });
});

it('chooses an unused alias and preserves a non-storage resource already called data', async () => {
  const { puts, user } = setup({ resources: { data: { type: 'redis', id: 'cache-existing' } } });
  await user.click(await screen.findByRole('button', { name: 'Thêm volume mount' }));
  const dialog = screen.getByRole('dialog', { name: 'Thêm tài nguyên lưu trữ' });
  expect(within(dialog).getByLabelText('Alias tài nguyên')).toHaveValue('data-2');
  await user.click(within(dialog).getByRole('button', { name: 'Thêm và chọn' }));
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(puts[0]?.score.resources).toMatchObject({ data: { type: 'redis', id: 'cache-existing' }, 'data-2': { type: 'persistent-storage' } });
});

it('explains unavailable storage and rejects secret or non-string storage capabilities', async () => {
  const invalid = { ...storage, outputs: [{ name: 'claimName', type: 'bool' }, { name: 'namespace', type: 'string', secret: true }] };
  setup({ types: [redis, invalid] });
  expect(await screen.findByRole('button', { name: 'Thêm volume mount' })).toBeDisabled();
  expect(screen.getByText('Chưa có loại tài nguyên lưu trữ. Liên hệ quản trị nền tảng để đăng ký.')).toBeVisible();
});
