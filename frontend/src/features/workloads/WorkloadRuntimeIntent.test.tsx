import { openWorkloadGroup } from '../../test/workloadGroups';
import { cleanup, render, screen, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import { bothConfigured } from '../../test/targets';
import { validRuntimeIntent } from './runtimeIntent';
import type { Execution } from './templates';

const application = { id: 'runtime', name: 'Runtime', environments: bothConfigured(), workloads: { staging: [], production: [] } };
afterEach(() => vi.unstubAllGlobals());
function setup(workload?: Record<string, unknown>, others: {id:string;ready:boolean;workload:Record<string,unknown>}[] = [], configurationFails = false) {
  const puts: { workload: Record<string, unknown> }[] = [];
  const creates: Record<string, unknown>[] = [];
  let keys = [{ name: 'TLS_KEY', kind: 'SECRET', configured: true, usedBy: [] as string[] }];
  const configuration = () => ({ version: 1, secretStoreKey: 'vault', keys });
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({workload: JSON.parse(String(init?.body)).workload, sources:{}});
    if (url.endsWith('/configuration/keys') && init?.method === 'POST') {
      const body = JSON.parse(String(init.body)); creates.push(body);
      keys = [...keys, { name: body.name, kind: 'SECRET', configured: true, usedBy: [] }];
      return Response.json(configuration(), { status: 201 });
    }
    if (url.endsWith('/configuration')) {if(configurationFails) throw new Error('offline');return Response.json(configuration());}
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [{ key: 'storage', classes: [], inputs: [], outputs: [{ name: 'claimName', type: 'string' }, { name: 'namespace', type: 'string' }] }] });
    if (init?.method === 'PUT') { puts.push(JSON.parse(String(init.body))); return Response.json({ workloads: [], draftVersion: 2 }); }
    return Response.json({ workloads: [...(workload ? [{ id: 'app', ready: false, workload }] : []),...others], draftVersion: 1 });
  }));
  render(<WorkloadEditorPage application={application} environment="staging" workloadId={workload ? 'app' : undefined} />);
  return { puts, creates, user: userEvent.setup() };
}
async function basic(user: ReturnType<typeof userEvent.setup>) {
  await user.type(await screen.findByLabelText('Tên Workload'), 'app');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example/app:1');
}
const save = () => screen.getByRole('button', { name: 'Lưu draft Workload' });

const volumeSection = (name = 'main') => screen.getByRole('group', {name:`Volume và file của ${name}`});
const sourceKinds:Record<string,string>={storage:'Lưu trữ bền vững (PVC)',emptyDir:'Volume tạm (emptyDir)',configMap:'File cấu hình (ConfigMap)',hostPath:'Đường dẫn node (hostPath)',secret:'File bí mật'};
async function openVolume(user: ReturnType<typeof userEvent.setup>, kind = 'emptyDir', container = 'main', source?:string) {
  const containerInput=screen.getAllByLabelText('Tên Container').find(el=>(el as HTMLInputElement).value===container)!;
  await openWorkloadGroup(user,'Volumes & files',containerInput.closest('section')!);
  await user.click(within(volumeSection(container)).getByRole('button',{name:'Thêm volume hoặc file'}));
  const dialog=screen.getByRole('dialog',{name:'Thêm volume hoặc file'});
  if(source) await user.click(within(dialog).getByRole('button',{name:`Dùng ${source} · ${sourceKinds[kind]}`}));
  else {await user.click(within(dialog).getByRole('button',{name:'Tạo nguồn mới'}));await user.selectOptions(within(dialog).getByLabelText('Loại'),kind);}
  return dialog;
}

async function attach(user:ReturnType<typeof userEvent.setup>) {
  await user.click(within(screen.getByRole('dialog',{name:'Thêm volume hoặc file'})).getByRole('button',{name:'Thêm vào container'}));
}

it('authors custom init/sidecar, a shared emptyDir, ConfigMap and a Secret file, then reopens losslessly', async () => {
  const { puts, user } = setup(); await basic(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm Container' }));
  await user.type(screen.getAllByLabelText('Ảnh Container')[1]!, 'example/init:1');
  await openWorkloadGroup(user,'Khởi chạy',screen.getAllByLabelText('Ảnh Container')[1]!.closest('section')!);
  await user.selectOptions(screen.getByRole('combobox', { name: 'Vai trò container container-2' }), 'init');
  await user.click(screen.getByRole('button', { name: '+ Thêm Container' }));
  await user.type(screen.getAllByLabelText('Ảnh Container')[2]!, 'example/sidecar:1');
  await openWorkloadGroup(user,'Khởi chạy',screen.getAllByLabelText('Ảnh Container')[2]!.closest('section')!);
  await user.selectOptions(screen.getByRole('combobox', { name: 'Vai trò container container-3' }), 'sidecar');
  let dialog=await openVolume(user);
  await user.clear(within(dialog).getByLabelText('Tên volume')); await user.type(within(dialog).getByLabelText('Tên volume'),'shared');
  await user.selectOptions(within(dialog).getByLabelText('Nơi lưu volume tạm'),'Memory');
  await user.type(within(dialog).getByLabelText('Giới hạn dung lượng'),'64Mi');
  await attach(user);
  dialog=await openVolume(user,'emptyDir','container-2','shared');
  await attach(user);
  dialog=await openVolume(user,'configMap');
  await user.type(within(dialog).getByLabelText('Nội dung (không chứa bí mật)'),'setting=true');
  await user.clear(within(dialog).getByLabelText('Đường dẫn mount'));await user.type(within(dialog).getByLabelText('Đường dẫn mount'),'/config/app.conf');
  await user.type(within(dialog).getByLabelText('Thư mục con / file (subPath)'),'app.conf');
  await attach(user);
  await openVolume(user,'secret','main','TLS_KEY');await attach(user);
  expect(document.body).not.toHaveTextContent('secret-file-sentinel');
  await user.click(save());
  expect(puts).toHaveLength(1);
  const saved = puts[0]!.workload;
  const execution = saved.workloadExecution as Execution;
  expect(execution.template).toBeUndefined();
  expect(execution.volumes?.[0]).toEqual({ name: 'shared', emptyDir: { medium: 'Memory', sizeLimit: '64Mi' } });
  expect(execution.volumeMounts).toHaveLength(3);
  expect(execution.volumeMounts?.[2]).toMatchObject({ volume: 'volume', path: '/config/app.conf', readOnly: true, subPath: 'app.conf' });
  expect(execution.secretFiles?.[0]?.ref).toBe('${resources.env.TLS_KEY}');
  expect(saved.resources).toEqual({ env: { type: 'environment' } });
  expect(saved.containers).toMatchObject({ 'container-2': { role: 'init', initOrder: 1 }, 'container-3': { role: 'sidecar', initOrder: 2 } });
  cleanup(); const reopened = setup(saved);
  await screen.findByLabelText('Tên Workload');
  await openWorkloadGroup(reopened.user,'Volumes & files',screen.getAllByLabelText('Ảnh Container')[0]!.closest('section')!);
  expect(within(volumeSection()).getByLabelText('Giới hạn dung lượng')).toHaveValue('64Mi');
  expect(screen.getByLabelText('Nội dung (không chứa bí mật)')).toHaveValue('setting=true');
  await reopened.user.click(save());
  expect(reopened.puts[0]?.workload).toEqual(saved);
});

it('creates PVC and mount in the custom form without a preset', async () => {
  const { puts, user } = setup(); await basic(user);
  await openVolume(user,'storage');
  await user.click(screen.getByRole('button',{name:'Tạo hoặc chọn lưu trữ'}));
  const dialog = screen.getByRole('dialog', { name: 'Thêm tài nguyên lưu trữ' });
  await user.click(within(dialog).getByRole('button', { name: 'Thêm và chọn' }));
  await user.click(save());
  expect((puts[0]?.workload.workloadExecution as Execution).mounts).toEqual([{ resource: 'data', container: 'main', path: '/data', readOnly: false }]);
});

it('reads a multiline Secret from a file into UC-12 without leaking its bytes into the workload', async () => {
  const { puts, creates, user } = setup(); await basic(user);
  await user.click(screen.getByRole('button',{name:'+ Thêm Container'}));
  await user.type(screen.getAllByLabelText('Ảnh Container')[1]!, 'agent:1');
  await openWorkloadGroup(user,'Khởi chạy',screen.getAllByLabelText('Ảnh Container')[1]!.closest('section')!);
  await user.selectOptions(screen.getByRole('combobox',{name:'Vai trò container container-2'}),'sidecar');
  const volumeDialog=await openVolume(user,'secret','container-2');
  await user.clear(within(volumeDialog).getByLabelText('Đường dẫn tệp'));await user.type(within(volumeDialog).getByLabelText('Đường dẫn tệp'),'/keys/uploaded.pem');
  await user.selectOptions(within(volumeDialog).getByLabelText('Quyền đọc'),'288');
  await user.click(screen.getByRole('button', { name: 'Tạo bí mật cho file' }));
  const dialog = screen.getByRole('dialog', { name: 'Tạo bí mật' });
  await user.type(within(dialog).getByLabelText('Tên key'), 'PRIVATE_KEY');
  const value = 'private-file-sentinel\nline two\n';
  await user.upload(within(dialog).getByLabelText('Đọc bí mật từ file'), new File([value], 'private.pem', { type: 'text/plain' }));
  const create = within(dialog).getByRole('button', { name: 'Tạo và chọn' });
  await waitFor(() => expect(create).toBeEnabled());
  await user.click(create);
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(creates[0]).toMatchObject({ name: 'PRIVATE_KEY', kind: 'SECRET', value });
  await user.click(save());
  expect(puts).toHaveLength(1);
  expect(JSON.stringify(puts)).not.toContain('private-file-sentinel');
  expect((puts[0]?.workload.workloadExecution as Execution).secretFiles?.[0]).toMatchObject({container:'container-2',path:'/keys/uploaded.pem',mode:288});
  expect((puts[0]?.workload.workloadExecution as Execution).secretFiles?.[0]?.ref).toBe('${resources.env.PRIVATE_KEY}');
});

it('blocks renamed volume references and duplicate init order without silently retargeting', () => {
  const containers = [{ name: 'app', image: 'app' }, { name: 'init', image: 'init', role: 'init', initOrder: 1 }, { name: 'sidecar', image: 'sidecar', role: 'sidecar', initOrder: 1 }];
  expect(validRuntimeIntent(containers)).toBe(false);
  expect(validRuntimeIntent(containers.slice(0, 1), { version: 1, kind: 'Deployment', volumes: [{ name: 'renamed', emptyDir: {} }], volumeMounts: [{ volume: 'old', container: 'app', path: '/data', readOnly: false }] })).toBe(false);
});

it('keeps zero replicas, annotations, container/Service protocols and extended probes through the form', async () => {
  const workload = { apiVersion: 'orchestrator.dev/workload/v1', metadata: { name: 'app', annotations: { 'example.com/owner': 'team' } }, replicas: 0,
    containers: { main: { image: 'app:1', ports: [{ name: 'dns', containerPort: 5353, protocol: 'UDP' }], readinessProbe: { type: 'tcp', port: 5353, initialDelaySeconds: 0 }, livenessProbe: { type: 'exec', command: ['check'], timeoutSeconds: 2 }, startupProbe: { path: '/start', port: 8080, scheme: 'HTTPS', failureThreshold: 5 } } },
    service: { ports: { dns: { port: 53, targetPort: 5353, protocol: 'UDP' } } } };
  const { puts, user } = setup(workload);
  await screen.findByLabelText('Ảnh Container');
  await user.click(screen.getByText('Mở rộng & cập nhật',{exact:true}));
  expect(screen.getByLabelText('Số replica')).toHaveValue(0);
  await user.click(screen.getByText('Cấu hình ít thay đổi',{exact:true}));
  expect(screen.getByLabelText('Tên annotation')).toHaveValue('example.com/owner');
  await user.click(screen.getByText('Cổng container', {exact:true}));
  await user.click(screen.getByText('Kiểm tra sức khỏe', {exact:true}));
  expect(screen.getByLabelText('Tên cổng container')).toHaveValue('dns');
  expect(screen.getAllByRole('combobox', { name: 'Giao thức' }).every(select => (select as HTMLSelectElement).value === 'UDP')).toBe(true);
  expect(screen.getByLabelText('Lệnh kiểm tra (mỗi đối số một dòng)')).toHaveValue('check');
  await user.click(save());
  expect(puts[0]?.workload).toEqual(workload);
});

it('authors hostPath with read-only defaults and keeps explicit writable mode when reopened', async () => {
  const {puts,user}=setup(); await basic(user);
  const dialog=await openVolume(user,'hostPath');
  expect(within(dialog).getByLabelText('Loại đường dẫn node')).toHaveValue('Directory');
  await user.type(within(dialog).getByLabelText('Đường dẫn trên node'),'/var/local/app');
  expect(within(dialog).getByLabelText('Chỉ đọc')).toBeChecked();
  await user.click(within(dialog).getByLabelText('Chỉ đọc'));
  await attach(user);
  await user.click(save());
  expect(puts).toHaveLength(1);
  const saved=puts[0]!.workload;
  expect((saved.workloadExecution as Execution).volumes).toEqual([{name:'volume',hostPath:{path:'/var/local/app',type:'Directory'}}]);
  expect((saved.workloadExecution as Execution).volumeMounts?.[0]?.readOnly).toBe(false);
  cleanup(); const reopened=setup(saved); await screen.findByLabelText('Ảnh Container');await openWorkloadGroup(reopened.user,'Volumes & files',screen.getAllByLabelText('Ảnh Container')[0]!.closest('section')!);
  expect(screen.getByLabelText('Đường dẫn trên node')).toHaveValue('/var/local/app');
  expect(screen.getByLabelText('Chỉ đọc')).not.toBeChecked();
  await reopened.user.click(save());expect(reopened.puts[0]?.workload).toEqual(saved);
});

it('validates hostPath source syntax and exclusive source types', () => {
 const containers=[{name:'main',image:'app'}];
 const valid:Execution={version:1,kind:'Deployment',volumes:[{name:'node',hostPath:{path:'/var/local/app',type:'Directory'}}]};
 expect(validRuntimeIntent(containers,valid)).toBe(true);
 for(const path of ['', 'relative','/a/../b','/a//b','/a/','/a\\b','/a\nb']) expect(validRuntimeIntent(containers,{...valid,volumes:[{name:'node',hostPath:{path}}]})).toBe(false);
 expect(validRuntimeIntent(containers,{...valid,volumes:[{name:'node',hostPath:{path:'/data',type:'unknown'}}]})).toBe(false);
 expect(validRuntimeIntent(containers,{...valid,volumes:[{name:'node',hostPath:{path:'/data'},emptyDir:{}}]})).toBe(false);
});

it('shares one source across containers, edits it for all users and removes only the selected mount', async () => {
  const workload={apiVersion:'orchestrator.dev/workload/v1',metadata:{name:'app'},containers:{main:{image:'app'},agent:{image:'agent',role:'sidecar',initOrder:1}},workloadExecution:{version:1,kind:'Deployment',volumes:[{name:'shared',emptyDir:{medium:'Memory',sizeLimit:'64Mi'}},{name:'unused',hostPath:{path:'/var/local/unused'}}],volumeMounts:[{volume:'shared',container:'main',path:'/shared',readOnly:false},{volume:'shared',container:'agent',path:'/shared',readOnly:false}]}};
  const {puts,user}=setup(workload);await screen.findAllByLabelText('Ảnh Container');
  for(const image of screen.getAllByLabelText('Ảnh Container'))await openWorkloadGroup(user,'Volumes & files',image.closest('section')!);
  const main=volumeSection();const agent=volumeSection('agent');
  expect(within(main).queryByLabelText('Container đích')).not.toBeInTheDocument();
  await user.click(within(agent).getByText('Cấu hình nguồn dùng chung (2 container)'));
  await user.clear(within(agent).getByLabelText('Giới hạn dung lượng'));await user.type(within(agent).getByLabelText('Giới hạn dung lượng'),'128Mi');
  expect(within(main).getByLabelText('Giới hạn dung lượng')).toHaveValue('128Mi');
  await user.click(within(main).getByRole('button',{name:'Gỡ mount'}));
  await user.click(save());
  const execution=puts[0]!.workload.workloadExecution as Execution;
  expect(execution.volumeMounts).toEqual([workload.workloadExecution.volumeMounts[1]]);
  expect(execution.volumes).toEqual([{name:'shared',emptyDir:{medium:'Memory',sizeLimit:'128Mi'}},workload.workloadExecution.volumes[1]]);
});

it('keeps canceled sources local and restores focus to the invoking container', async () => {
  const {puts,user}=setup();await basic(user);
  await openWorkloadGroup(user,'Volumes & files');
  const add=within(volumeSection()).getByRole('button',{name:'Thêm volume hoặc file'});
  const dialog=await openVolume(user,'hostPath');
  await user.type(within(dialog).getByLabelText('Đường dẫn trên node'),'/var/local/cancel');
  await user.keyboard('{Escape}');expect(add).toHaveFocus();
  await user.click(save());expect(puts[0]?.workload.workloadExecution).toEqual({version:1,kind:'Deployment',defaults:{version:'workload-defaults-v1',environment:'staging'}});
});

it('keeps renamed-container mounts visible until their target is explicitly repaired', async () => {
  const workload={apiVersion:'orchestrator.dev/workload/v1',metadata:{name:'app'},containers:{main:{image:'app'}},workloadExecution:{version:1,kind:'Deployment',volumes:[{name:'data',emptyDir:{}}],volumeMounts:[{volume:'data',container:'main',path:'/data',readOnly:false}]}};
  const {puts,user}=setup(workload);await screen.findAllByLabelText('Ảnh Container');
  await user.clear(screen.getByLabelText('Tên Container'));await user.type(screen.getByLabelText('Tên Container'),'renamed');
  expect(screen.getByRole('heading',{name:'Mount cần chọn lại container đích'})).toBeVisible();
  await user.click(save());expect(puts).toHaveLength(0);
  await user.selectOptions(screen.getByLabelText('Container đích'),'renamed');
  await user.click(save());expect((puts[0]?.workload.workloadExecution as Execution).volumeMounts?.[0]).toEqual({...workload.workloadExecution.volumeMounts[0],container:'renamed'});
});

it('opens with searchable reusable sources, no implicit selection or source-creation fields', async () => {
  const workload={apiVersion:'orchestrator.dev/workload/v1',metadata:{name:'app'},containers:{main:{image:'app'}},workloadExecution:{version:1,kind:'Deployment',volumes:[{name:'shared-cache',emptyDir:{}},{name:'node-logs',hostPath:{path:'/var/log',type:'Directory'}}],volumeMounts:[{volume:'shared-cache',container:'main',path:'/shared',readOnly:false}]}};
  const {puts,user}=setup(workload,[{id:'other',ready:true,workload:{containers:{worker:{image:'worker'}},workloadExecution:{version:1,kind:'Deployment',volumes:[{name:'other-temporary',emptyDir:{}}]}}}]);
  await screen.findByLabelText('Ảnh Container');
  await openWorkloadGroup(user,'Volumes & files');
  await user.click(within(volumeSection()).getByRole('button',{name:'Thêm volume hoặc file'}));
  const dialog=screen.getByRole('dialog',{name:'Thêm volume hoặc file'});
  expect(within(dialog).queryByLabelText('Tên volume')).not.toBeInTheDocument();
  expect(within(dialog).getByRole('button',{name:'Thêm vào container'})).toBeDisabled();
  expect(within(dialog).getByText('Đang mount trong: main')).toBeVisible();
  expect(within(dialog).queryByRole('button',{name:/other-temporary/})).not.toBeInTheDocument();
  await user.type(within(dialog).getByLabelText('Tìm nguồn'),'node');
  expect(within(dialog).getByRole('button',{name:'Dùng node-logs · Đường dẫn node (hostPath)'})).toBeVisible();
  expect(within(dialog).queryByRole('button',{name:/Dùng shared-cache/})).not.toBeInTheDocument();
  await user.selectOptions(within(dialog).getByLabelText('Loại nguồn'),'emptyDir');
  expect(within(dialog).getByText('Không có nguồn phù hợp.')).toBeVisible();
  await user.clear(within(dialog).getByLabelText('Tìm nguồn'));
  await user.click(within(dialog).getByRole('button',{name:'Dùng shared-cache · Volume tạm (emptyDir)'}));
  expect(within(dialog).getByLabelText('Đường dẫn mount')).toHaveValue('/shared-2');
  await user.type(within(dialog).getByLabelText('Tìm nguồn'),'node');
  expect(within(dialog).getByRole('button',{name:'Thêm vào container'})).toBeDisabled();
  await user.clear(within(dialog).getByLabelText('Tìm nguồn'));
  await user.click(within(dialog).getByRole('button',{name:'Dùng shared-cache · Volume tạm (emptyDir)'}));
  await attach(user);await user.click(save());
  const execution=puts[0]!.workload.workloadExecution as Execution;
  expect(execution.volumes).toEqual(workload.workloadExecution.volumes);
  expect(execution.volumeMounts?.[1]).toMatchObject({volume:'shared-cache',container:'main',path:'/shared-2'});
});

it('keeps named sources usable when the Secret catalog fails and reports the failure instead of an empty catalog', async () => {
  const workload={apiVersion:'orchestrator.dev/workload/v1',metadata:{name:'app'},containers:{main:{image:'app'}},workloadExecution:{version:1,kind:'Deployment',volumes:[{name:'local-data',emptyDir:{}}]}};
  const {user}=setup(workload,[],true);await screen.findByLabelText('Ảnh Container');
  await openWorkloadGroup(user,'Volumes & files');
  await user.click(within(volumeSection()).getByRole('button',{name:'Thêm volume hoặc file'}));
  const dialog=screen.getByRole('dialog',{name:'Thêm volume hoặc file'});
  expect(within(dialog).getByText('Không tải được bí mật. Đóng và thử tải lại key.')).toBeVisible();
  expect(within(dialog).queryByText('Chưa có nguồn để dùng lại.')).not.toBeInTheDocument();
  await user.click(within(dialog).getByRole('button',{name:'Dùng local-data · Volume tạm (emptyDir)'}));
  expect(within(dialog).getByRole('button',{name:'Thêm vào container'})).toBeEnabled();
});

it('defers template image admission to the backend while resolver metadata is refreshing',()=>{
 const container=[{name:'main',image:''}];
 expect(validRuntimeIntent(container)).toBe(false);
 expect(validRuntimeIntent(container,undefined,undefined,true)).toBe(true);
 expect(validRuntimeIntent([{name:'',image:''}],undefined,undefined,true)).toBe(false);
});
