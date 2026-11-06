import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import { bothConfigured } from '../../test/targets';
import type { ResourceType, Workload } from './api';

afterEach(() => vi.unstubAllGlobals());
const application = { id: 'catalog', name: 'Catalog', environments: bothConfigured(), workloads: { staging: [], production: [] } };
const execution = { version: 1, kind: 'Deployment', template: { id: 'backend', version: '1.0.0', sha256: 'a'.repeat(64) } };
const storage: ResourceType = { key: 'persistent-storage', inputs: [{ name: 'storage', type: 'string' }], outputs: [{ name: 'claimName', type: 'string' }, { name: 'namespace', type: 'string' }], classes: ['default'] };
const redis: ResourceType = { key: 'redis', inputs: [], outputs: [], classes: ['default'] };
function setup({ types = [storage, redis], resources, others = [] }: { types?: ResourceType[]; resources?: Record<string, unknown>; others?: Workload[] } = {}) {
  const puts: { workload: Record<string, unknown> }[] = [];
  const workload = { apiVersion: 'orchestrator.dev/workload/v1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/backend:v1' } }, workloadExecution: execution, ...(resources ? { resources } : {}) };
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ version: 1, keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: types });
    if (init?.method === 'PUT') { puts.push(JSON.parse(String(init.body))); return Response.json({ workloads: [], draftVersion: 2 }); }
    return Response.json({ draftVersion: 1, workloads: [{ id: 'api', ready: false, state: 'PENDING_UPSERT', workload }, ...others] });
  }));
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
  return { puts, user: userEvent.setup() };
}

async function openStorage(user:ReturnType<typeof userEvent.setup>, path = '/data') {
  const add=await screen.findByRole('button',{name:'Thêm volume hoặc file'});
  await user.click(add);
  await user.click(within(screen.getByRole('dialog',{name:'Thêm volume hoặc file'})).getByRole('button',{name:'Tạo nguồn mới'}));
  await user.selectOptions(within(screen.getByRole('dialog',{name:'Thêm volume hoặc file'})).getByLabelText('Loại'),'storage');
  const picker=screen.getByRole('dialog',{name:'Thêm volume hoặc file'});
  await user.clear(within(picker).getByLabelText('Đường dẫn mount'));await user.type(within(picker).getByLabelText('Đường dẫn mount'),path);
  await user.click(screen.getByRole('button',{name:'Tạo hoặc chọn lưu trữ'}));
  return add;
}

it('adds storage and mount from Add volume mount, while Cancel keeps the local draft unchanged', async () => {
  const { puts, user } = setup();
  const add = await openStorage(user);
  let dialog = screen.getByRole('dialog', { name: 'Thêm tài nguyên lưu trữ' });
  expect(within(dialog).getByLabelText('Alias tài nguyên')).toHaveValue('data');
  expect(within(dialog).getByLabelText('Loại tài nguyên')).toHaveValue('persistent-storage');
  expect(within(dialog).queryByRole('option', { name: 'redis' })).not.toBeInTheDocument();
  await user.keyboard('{Escape}');
  expect(screen.queryByLabelText('Mount path')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Alias tài nguyên')).not.toBeInTheDocument();
  expect(add).toHaveFocus();
  await openStorage(user,'/data/custom');
  dialog = screen.getByRole('dialog', { name: 'Thêm tài nguyên lưu trữ' });
  await user.type(within(dialog).getByLabelText('Tham số storage'), '5Gi');
  await user.click(within(dialog).getByRole('button', { name: 'Thêm và chọn' }));
  expect(screen.getByLabelText('Đường dẫn mount')).toHaveValue('/data/custom');
  expect(puts).toHaveLength(0);
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(puts[0]?.workload.resources).toEqual({ data: { type: 'persistent-storage', id: expect.stringMatching(/^resource-/), params: { storage: '5Gi' } } });
  expect(puts[0]?.workload.workloadExecution).toMatchObject({ ...execution, mounts: [{ resource: 'data', container: 'main', path: '/data/custom', readOnly: false }] });
});

it('reuses an existing scoped storage resource from the mount dialog without creating a new identity', async () => {
  const { puts, user } = setup({ others: [{ id: 'worker', ready: true, workload: { resources: { volume: { type: 'persistent-storage', id: 'store-a', params: { storage: '2Gi' } } } } }] });
  await openStorage(user);
  const dialog = screen.getByRole('dialog', { name: 'Thêm tài nguyên lưu trữ' });
  await user.selectOptions(within(dialog).getByLabelText('Nguồn tài nguyên'), 'shared:store-a');
  await user.click(within(dialog).getByRole('button', { name: 'Thêm và chọn' }));
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(puts[0]?.workload.resources).toEqual({ data: { type: 'persistent-storage', id: 'store-a', params: { storage: '2Gi' } } });
});

it('chooses an unused alias and preserves a non-storage resource already called data', async () => {
  const { puts, user } = setup({ resources: { data: { type: 'redis', id: 'cache-existing' } } });
  await openStorage(user);
  const dialog = screen.getByRole('dialog', { name: 'Thêm tài nguyên lưu trữ' });
  expect(within(dialog).getByLabelText('Alias tài nguyên')).toHaveValue('data-2');
  await user.click(within(dialog).getByRole('button', { name: 'Thêm và chọn' }));
  await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
  expect(puts[0]?.workload.resources).toMatchObject({ data: { type: 'redis', id: 'cache-existing' }, 'data-2': { type: 'persistent-storage' } });
});

it('explains unavailable storage and rejects secret or non-string storage capabilities', async () => {
  const invalid = { ...storage, outputs: [{ name: 'claimName', type: 'bool' }, { name: 'namespace', type: 'string', secret: true }] };
  const {user}=setup({ types: [redis, invalid] });
  await user.click(await screen.findByRole('button', { name: 'Thêm volume hoặc file' }));
  const dialog=screen.getByRole('dialog',{name:'Thêm volume hoặc file'});
  await user.click(within(dialog).getByRole('button',{name:'Tạo nguồn mới'}));
  await user.selectOptions(within(dialog).getByLabelText('Loại'),'storage');
  expect(within(dialog).getByRole('button',{name:'Thêm vào container'})).toBeDisabled();
  expect(screen.getByText('Chưa có loại tài nguyên lưu trữ. Liên hệ quản trị nền tảng để đăng ký.')).toBeVisible();
});

it('lists app storage first and reuses its shared identity atomically with a collision-free alias', async () => {
  const {puts,user}=setup({resources:{volume:{type:'redis',id:'cache-existing'}},others:[{id:'worker',ready:true,workload:{resources:{volume:{type:'persistent-storage',id:'store-a',params:{storage:'2Gi'}}}}}]});
  await user.click(await screen.findByRole('button',{name:'Thêm volume hoặc file'}));
  const dialog=screen.getByRole('dialog',{name:'Thêm volume hoặc file'});
  expect(within(dialog).queryByLabelText('Loại')).not.toBeInTheDocument();
  await user.type(within(dialog).getByLabelText('Tìm nguồn'),'worker');
  await user.click(within(dialog).getByRole('button',{name:'Dùng volume · persistent-storage · worker · Lưu trữ bền vững (PVC)'}));
  await user.clear(within(dialog).getByLabelText('Đường dẫn mount'));await user.type(within(dialog).getByLabelText('Đường dẫn mount'),'/reused');
  await user.click(within(dialog).getByRole('button',{name:'Thêm vào container'}));
  expect(puts).toHaveLength(0);
  await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));
  expect(puts[0]?.workload.resources).toEqual({volume:{type:'redis',id:'cache-existing'},'volume-2':{type:'persistent-storage',id:'store-a',params:{storage:'2Gi'}}});
  expect(puts[0]?.workload.workloadExecution).toMatchObject({mounts:[{resource:'volume-2',container:'main',path:'/reused',readOnly:false}]});
});
