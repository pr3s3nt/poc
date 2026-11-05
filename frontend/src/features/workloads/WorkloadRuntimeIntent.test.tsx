import { cleanup, render, screen, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import { bothConfigured } from '../../test/targets';
import { validRuntimeIntent } from './runtimeIntent';
import type { Execution } from './templates';

const application = { id: 'runtime', name: 'Runtime', environments: bothConfigured(), workloads: { staging: [], production: [] } };
afterEach(() => vi.unstubAllGlobals());
function setup(workload?: Record<string, unknown>) {
  const puts: { workload: Record<string, unknown> }[] = [];
  const creates: Record<string, unknown>[] = [];
  let keys = [{ name: 'TLS_KEY', kind: 'SECRET', configured: true, usedBy: [] as string[] }];
  const configuration = () => ({ version: 1, secretStoreKey: 'vault', keys });
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration/keys') && init?.method === 'POST') {
      const body = JSON.parse(String(init.body)); creates.push(body);
      keys = [...keys, { name: body.name, kind: 'SECRET', configured: true, usedBy: [] }];
      return Response.json(configuration(), { status: 201 });
    }
    if (url.endsWith('/configuration')) return Response.json(configuration());
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [{ key: 'storage', classes: [], inputs: [], outputs: [{ name: 'claimName', type: 'string' }, { name: 'namespace', type: 'string' }] }] });
    if (init?.method === 'PUT') { puts.push(JSON.parse(String(init.body))); return Response.json({ workloads: [], draftVersion: 2 }); }
    return Response.json({ workloads: workload ? [{ id: 'app', ready: false, workload }] : [], draftVersion: 1 });
  }));
  render(<WorkloadEditorPage application={application} environment="staging" workloadId={workload ? 'app' : undefined} />);
  return { puts, creates, user: userEvent.setup() };
}
async function basic(user: ReturnType<typeof userEvent.setup>) {
  await user.type(await screen.findByLabelText('Tên Workload'), 'app');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example/app:1');
}
const save = () => screen.getByRole('button', { name: 'Lưu draft Workload' });

it('authors custom init/sidecar, a shared emptyDir, ConfigMap and a Secret file, then reopens losslessly', async () => {
  const { puts, user } = setup(); await basic(user);
  await user.click(screen.getByRole('button', { name: '+ Thêm Container' }));
  await user.type(screen.getAllByLabelText('Ảnh Container')[1]!, 'example/init:1');
  await user.selectOptions(screen.getByRole('combobox', { name: 'Vai trò container container-2' }), 'init');
  await user.click(screen.getByRole('button', { name: '+ Thêm Container' }));
  await user.type(screen.getAllByLabelText('Ảnh Container')[2]!, 'example/sidecar:1');
  await user.selectOptions(screen.getByRole('combobox', { name: 'Vai trò container container-3' }), 'sidecar');
  await user.click(screen.getByRole('button', { name: 'Thêm volume' }));
  await user.clear(screen.getByLabelText('Tên volume')); await user.type(screen.getByLabelText('Tên volume'), 'shared');
  await user.selectOptions(screen.getByLabelText('Nơi lưu volume tạm'), 'Memory');
  await user.type(screen.getByLabelText('Giới hạn dung lượng'), '64Mi');
  await user.click(screen.getByRole('button', { name: 'Thêm mount volume' }));
  await user.click(screen.getByRole('button', { name: 'Thêm mount volume' }));
  await user.selectOptions(screen.getAllByLabelText('Container đích')[1]!, 'container-2');
  await user.click(screen.getByRole('button', { name: 'Thêm volume' }));
  await user.selectOptions(screen.getAllByLabelText('Loại volume')[1]!, 'configMap');
  await user.type(screen.getByLabelText('Nội dung (không chứa bí mật)'), 'setting=true');
  await user.click(screen.getByRole('button', { name: 'Thêm mount volume' }));
  await user.selectOptions(screen.getAllByLabelText('Volume')[2]!, 'volume');
  const configPath = screen.getAllByLabelText('Đường dẫn mount')[2]!;
  await user.clear(configPath); await user.type(configPath, '/config/app.conf');
  await user.type(screen.getAllByLabelText('Thư mục con / file (subPath)')[2]!, 'app.conf');
  await user.click(screen.getByRole('button', { name: 'Thêm tệp bí mật' }));
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
  expect(screen.getByLabelText('Giới hạn dung lượng')).toHaveValue('64Mi');
  expect(screen.getByLabelText('Nội dung (không chứa bí mật)')).toHaveValue('setting=true');
  await reopened.user.click(save());
  expect(reopened.puts[0]?.workload).toEqual(saved);
});

it('creates PVC and mount in the custom form without a preset', async () => {
  const { puts, user } = setup(); await basic(user);
  await user.click(screen.getByRole('button', { name: 'Thêm volume mount' }));
  const dialog = screen.getByRole('dialog', { name: 'Thêm tài nguyên lưu trữ' });
  await user.click(within(dialog).getByRole('button', { name: 'Thêm và chọn' }));
  await user.click(save());
  expect((puts[0]?.workload.workloadExecution as Execution).mounts).toEqual([{ resource: 'data', container: 'main', path: '/data', readOnly: false }]);
});

it('reads a multiline Secret from a file into UC-12 without leaking its bytes into the workload', async () => {
  const { puts, creates, user } = setup(); await basic(user);
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
  expect(screen.getByLabelText('Số replica')).toHaveValue(0);
  await user.click(screen.getByRole('button', { name: 'Hiện nâng cao Workload' }));
  expect(screen.getByLabelText('Tên annotation')).toHaveValue('example.com/owner');
  await user.click(screen.getByRole('button', { name: 'Hiện nâng cao Container 1' }));
  expect(screen.getByLabelText('Tên cổng container')).toHaveValue('dns');
  expect(screen.getAllByRole('combobox', { name: 'Giao thức' }).every(select => (select as HTMLSelectElement).value === 'UDP')).toBe(true);
  expect(screen.getByLabelText('Lệnh kiểm tra (mỗi đối số một dòng)')).toHaveValue('check');
  await user.click(save());
  expect(puts[0]?.workload).toEqual(workload);
});

it('authors hostPath with read-only defaults and keeps explicit writable mode when reopened', async () => {
  const {puts,user}=setup(); await basic(user);
  await user.click(screen.getByRole('button',{name:'Thêm volume'}));
  await user.selectOptions(screen.getByLabelText('Loại volume'),'hostPath');
  expect(screen.getByLabelText('Loại đường dẫn node')).toHaveValue('Directory');
  await user.type(screen.getByLabelText('Đường dẫn trên node'),'/var/local/app');
  await user.click(screen.getByRole('button',{name:'Thêm mount volume'}));
  expect(screen.getByLabelText('Chỉ đọc')).toBeChecked();
  await user.click(screen.getByLabelText('Chỉ đọc'));
  await user.click(save());
  expect(puts).toHaveLength(1);
  const saved=puts[0]!.workload;
  expect((saved.workloadExecution as Execution).volumes).toEqual([{name:'volume',hostPath:{path:'/var/local/app',type:'Directory'}}]);
  expect((saved.workloadExecution as Execution).volumeMounts?.[0]?.readOnly).toBe(false);
  cleanup(); const reopened=setup(saved); await screen.findByLabelText('Ảnh Container');
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
