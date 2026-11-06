import { ExecutionFields } from './ExecutionFields';
import { t } from '../../shared/i18n';
import { openAdvancedFields, openWorkloadGroup } from '../../test/workloadGroups';
import { render,screen,within,waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach,expect,it,vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import { bothConfigured } from '../../test/targets';
import { templateKey, type Execution, type WorkloadTemplate } from './templates';
const application={id:'defaults',name:'Defaults',environments:bothConfigured(),workloads:{staging:[],production:[]}};
afterEach(()=>vi.unstubAllGlobals());
// Nonsecret resolver response recorded from the real backend policy endpoint.
const backendTemplate:WorkloadTemplate={id:'backend',version:'1.1.0',sha256:'b'.repeat(64),kind:'Deployment',labels:{vi:'Backend API'},inputsSchema:{type:'object',properties:{}},defaults:{}};
const effective={apiVersion:'orchestrator.dev/workload/v1',metadata:{name:'web'},containers:{main:{image:'nginx:1',resources:{requests:{cpu:'100m',memory:'128Mi'},limits:{memory:'512Mi'}}}},workloadExecution:{version:1,kind:'Deployment',template:{id:backendTemplate.id,version:backendTemplate.version,sha256:backendTemplate.sha256},templateDefaults:{environment:'staging',profile:{}},automountServiceAccountToken:false,revisionHistoryLimit:3,updateStrategy:{type:'RollingUpdate',rollingUpdate:{maxUnavailable:0,maxSurge:1}}}};
function serve(failure=false,responseWorkload: Record<string,unknown>=effective,template:WorkloadTemplate=backendTemplate){
 const saved:{workload:Record<string,unknown>}[]=[];let fail=failure;
 vi.stubGlobal('fetch',vi.fn(async(input:string|Request|URL,init?:RequestInit)=>{
 const url=String(input);
 if(url.endsWith('/resolve-defaults'))return fail?Response.json({code:'UNAVAILABLE'},{status:503}):Response.json({workload:responseWorkload,sources:{'containers.main.resources.requests.cpu':'template','containers.main.resources.requests.memory':'template','containers.main.resources.limits.memory':'template','workloadExecution.automountServiceAccountToken':'template','workloadExecution.revisionHistoryLimit':'template','workloadExecution.updateStrategy.type':'template'}});
 if(url.endsWith('/workload-templates'))return Response.json({templates:template?[template]:[]});
 if(url.endsWith('/configuration'))return Response.json({version:1,keys:[]});
 if(url.endsWith('/resource-types'))return Response.json({resourceTypes:[]});
 if(init?.method==='PUT'){saved.push(JSON.parse(String(init.body)));return Response.json({draftVersion:2,workloads:[]})}
 return Response.json({draftVersion:1,workloads:[]});
 }));return {saved,recover(){fail=false}};
}
async function author(){const user=userEvent.setup();render(<WorkloadEditorPage application={application} environment="staging" selectedTemplate={templateKey(backendTemplate)}/>);await user.type(await screen.findByLabelText('Tên Workload'),'web');await user.type(screen.getByLabelText('Ảnh Container'),'nginx:1');await openAdvancedFields(user);return user}
it('displays backend effective values and saves inherited values as omissions',async()=>{
 const {saved}=serve();const user=await author();await waitFor(()=>expect(screen.getByText(/CPU 100m · RAM 128Mi/)).toBeInTheDocument());
 await user.click(screen.getByText('Tài nguyên tính toán',{exact:true}));const cpu=screen.getByLabelText('Yêu cầu CPU');expect(cpu).toHaveValue('');expect(cpu).toHaveAttribute('placeholder','100m');expect(screen.getByLabelText('Giới hạn bộ nhớ')).toHaveAttribute('placeholder','512Mi');
 await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect(saved).toHaveLength(1);expect(saved[0]!.workload.containers).toEqual({main:{image:'nginx:1'}});expect((saved[0]!.workload.workloadExecution as Execution).template?.id).toBe('backend');
});
it('keeps explicit false and zero and reset removes the history override',async()=>{
 const {saved}=serve();const user=await author();await user.click(screen.getByText('Bảo mật Pod',{exact:true}));await user.selectOptions(screen.getByLabelText('automountServiceAccountToken'),'false');
 await openWorkloadGroup(user,'Mở rộng & cập nhật');const history=screen.getByLabelText('revisionHistoryLimit');await user.type(history,'0');expect(history).toHaveValue(0);
 await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect((saved[0]!.workload.workloadExecution as Execution).revisionHistoryLimit).toBe(0);expect((saved[0]!.workload.workloadExecution as Execution).automountServiceAccountToken).toBe(false);
 await user.click(within(history.closest('label')!).getByRole('button',{name:'Khôi phục mặc định'}));await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect((saved[1]!.workload.workloadExecution as Execution).revisionHistoryLimit).toBeUndefined();
});
it('preserves draft and prevents save when default metadata fails, then retries safely',async()=>{
 const mock=serve(true);const user=await author();await waitFor(()=>expect(screen.getByText(/Chưa tải được cấu hình hiệu lực/)).toBeInTheDocument());await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect(mock.saved).toHaveLength(0);expect(screen.getByLabelText('Ảnh Container')).toHaveValue('nginx:1');mock.recover();await user.click(screen.getByRole('button',{name:'Thử lại'}));await waitFor(()=>expect(screen.getByText(/CPU 100m · RAM 128Mi/)).toBeInTheDocument());await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect(mock.saved).toHaveLength(1);
});

it('shows inherited template mounts, ports and readiness without saving them as overrides',async()=>{
 const {saved}=serve(false,{...effective,containers:{main:{...effective.containers.main,ports:[{name:'http',containerPort:8080}],readinessProbe:{type:'http',path:'/',port:8080}}},workloadExecution:{...effective.workloadExecution,volumeMounts:[{volume:'cache',container:'main',path:'/var/cache/nginx',readOnly:false},{volume:'tmp',container:'main',path:'/tmp',readOnly:false}]}});
 const user=await author();await openWorkloadGroup(user,'Volumes & files');await screen.findByText(/cache → \/var\/cache\/nginx, tmp → \/tmp/);
 await user.click(screen.getByText('Kiểm tra sức khỏe',{exact:true}));expect(screen.getByText(/http :8080 \/$/)).toBeInTheDocument();
 await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect(saved[0]!.workload.containers).toEqual({main:{image:'nginx:1'}});expect((saved[0]!.workload.workloadExecution as Execution).volumeMounts).toBeUndefined();
});

it('keeps a template-provided image omitted when editing runtime policy',async()=>{
 const template:WorkloadTemplate={id:'nginx',version:'1.0.0',sha256:'a'.repeat(64),kind:'Deployment',labels:{vi:'Nginx'},inputsSchema:{type:'object',properties:{}},defaults:{},runtimeDefaults:{image:'nginx:1'}};
 const {saved}=serve(false,effective,template);const user=userEvent.setup();render(<WorkloadEditorPage application={application} environment="staging" selectedTemplate={templateKey(template)}/>);await user.type(await screen.findByLabelText('Tên Workload'),'web');await openAdvancedFields(user);await waitFor(()=>expect(screen.getByLabelText('Ảnh Container')).toHaveAttribute('placeholder','nginx:1'));
 await openWorkloadGroup(user,'Mở rộng & cập nhật');await user.selectOptions(screen.getByLabelText('PodDisruptionBudget'),'off');await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect(saved).toHaveLength(1);expect(saved[0]!.workload.containers).toEqual({main:{}});expect((saved[0]!.workload.workloadExecution as Execution).podDisruptionBudget).toEqual({enabled:false});
});

it('waits for the required image of a generic template instead of reporting resolver failure',async()=>{
 const template:WorkloadTemplate={id:'backend',version:'1.0.0',sha256:'b'.repeat(64),kind:'Deployment',labels:{vi:'Backend API'},inputsSchema:{type:'object',properties:{}},defaults:{}};
 const {saved}=serve(false,effective,template);const user=userEvent.setup();render(<WorkloadEditorPage application={application} environment="staging" selectedTemplate={templateKey(template)}/>);
 await screen.findAllByText('Nhập image cho container để tính cấu hình hiệu lực.');expect(screen.queryByRole('alert')).not.toBeInTheDocument();expect(screen.queryByRole('button',{name:'Thử lại'})).not.toBeInTheDocument();
 const resolutionCalls=()=>vi.mocked(fetch).mock.calls.filter(([input])=>String(input).endsWith('/resolve-defaults'));
 expect(resolutionCalls()).toHaveLength(0);await user.type(screen.getByLabelText('Tên Workload'),'web');await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect(saved).toHaveLength(0);expect(resolutionCalls()).toHaveLength(0);
 await user.type(screen.getByLabelText('Ảnh Container'),'nginx:1');await waitFor(()=>expect(screen.getByText(/CPU 100m · RAM 128Mi/)).toBeInTheDocument());
 expect(resolutionCalls().length).toBeGreaterThan(0);for(const [,init] of resolutionCalls()){const request=JSON.parse(String(init?.body));expect(request.workload.containers.main.image).toBe('nginx:1')}
 await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect(saved).toHaveLength(1);expect(saved[0]!.workload.containers).toEqual({main:{image:'nginx:1'}});expect((saved[0]!.workload.workloadExecution as Execution).template?.id).toBe('backend');
});

it('keeps the basic form small and lets typing replace a muted default without deleting it',async()=>{
 const {saved}=serve();const user=userEvent.setup();render(<WorkloadEditorPage application={application} environment="staging" selectedTemplate={templateKey(backendTemplate)}/>);
 await user.type(await screen.findByLabelText('Tên Workload'),'web');await user.type(screen.getByLabelText('Ảnh Container'),'nginx:1');
 expect(screen.getByRole('button',{name:'+ Thêm biến'})).toBeVisible();
 for(const title of ['Tài nguyên tính toán','Volumes & files','Bảo mật Pod','Mở rộng & cập nhật','Cấu hình ít thay đổi'])expect(screen.queryByText(title,{exact:true})).not.toBeInTheDocument();
 await user.click(screen.getByRole('button',{name:'Hiện nâng cao Container main'}));await openWorkloadGroup(user,'Tài nguyên tính toán');
 const cpu=screen.getByLabelText('Yêu cầu CPU');await waitFor(()=>expect(cpu).toHaveAttribute('placeholder','100m'));expect(cpu).toHaveValue('');
 expect(screen.queryByText('Theo môi trường',{exact:true})).not.toBeInTheDocument();expect(screen.queryByText('Theo nền tảng',{exact:true})).not.toBeInTheDocument();
 await user.type(cpu,'250m');expect(cpu).toHaveValue('250m');await user.click(screen.getByRole('button',{name:'Ẩn nâng cao Container main'}));await user.click(screen.getByRole('button',{name:'Hiện nâng cao Container main'}));expect(cpu).toHaveValue('250m');
 await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect(saved).toHaveLength(1);expect(saved[0]!.workload.containers).toEqual({main:{image:'nginx:1',resources:{requests:{cpu:'250m'}}}});
 expect((saved[0]!.workload.workloadExecution as Execution).revisionHistoryLimit).toBeUndefined();
});

it('offers each select value once and reset restores omission after explicit choices',async()=>{
 const {saved}=serve(false,{...effective,workloadExecution:{...effective.workloadExecution,podSecurityContext:{seccompProfile:{type:'RuntimeDefault'}}}});
 const user=await author();
 await openWorkloadGroup(user,'Service & kết nối');
 const service=screen.getByRole('combobox',{name:'Service'});
 expect(service).toHaveValue('');expect(service).toHaveClass('inherited-default');
 expect(within(service).getAllByRole('option').map(option=>option.textContent)).toEqual(['Bật','Tắt']);
 await user.selectOptions(service,'false');
 await openWorkloadGroup(user,'Bảo mật Pod');
 const token=screen.getByRole('combobox',{name:'automountServiceAccountToken'});
 expect(within(token).getAllByRole('option').map(option=>option.textContent)).toEqual(['Có','Không']);
 await user.selectOptions(token,'false');
 const seccomp=screen.getByRole('combobox',{name:'seccompProfile'});
 expect(within(seccomp).getAllByRole('option').map(option=>option.textContent)).toEqual(['RuntimeDefault','Localhost']);
 await user.selectOptions(seccomp,'RuntimeDefault');
 await openWorkloadGroup(user,'Mở rộng & cập nhật');
 const strategy=screen.getByRole('combobox',{name:'Chiến lược cập nhật'});
 expect(within(strategy).getAllByRole('option').map(option=>option.textContent)).toEqual(['RollingUpdate','Recreate']);
 await user.selectOptions(strategy,'RollingUpdate');
 await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));
 const explicit=saved[0]!.workload.workloadExecution as Execution;
 expect(explicit.serviceEnabled).toBe(false);expect(explicit.automountServiceAccountToken).toBe(false);expect(explicit.podSecurityContext?.seccompProfile?.type).toBe('RuntimeDefault');expect(explicit.updateStrategy?.type).toBe('RollingUpdate');
 for(const control of [service,token,seccomp,strategy]){
  await user.click(within(control.closest('label')!).getByRole('button',{name:'Khôi phục mặc định'}));
  expect(control).toHaveValue('');expect(control).toHaveClass('inherited-default');
 }
 await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));
 const reset=saved[1]!.workload.workloadExecution as Execution;
 expect(reset.serviceEnabled).toBeUndefined();expect(reset.automountServiceAccountToken).toBeUndefined();expect(reset.podSecurityContext).toBeUndefined();expect(reset.updateStrategy).toBeUndefined();
});


it('shows inherited concurrency for a reopened sparse CronJob without an override',async()=>{
 const user=userEvent.setup(),onChange=vi.fn();
 const execution:Execution=JSON.parse('{"version":1,"kind":"CronJob","schedule":{"expression":"15 3 * * *","suspend":false}}');
 render(<ExecutionFields section="schedule" value={execution} onChange={onChange} disabled={false} onRawProblems={()=>{}} resolution={{workload:{workloadExecution:{schedule:{concurrencyPolicy:'Forbid'}}},sources:{}}}/>);
 const concurrency=screen.getByLabelText(t('workloads.templates.concurrency'));
 await user.click(concurrency.closest('details')!.querySelector('summary')!);
 expect(concurrency).toHaveValue('');expect(concurrency).toHaveClass('inherited-default');
 expect(within(concurrency).getAllByRole('option')).toHaveLength(3);
 expect(screen.queryByRole('button',{name:'Khôi phục mặc định'})).not.toBeInTheDocument();
 await user.selectOptions(concurrency,'Allow');
 expect(onChange.mock.lastCall?.[0].schedule.concurrencyPolicy).toBe('Allow');
 expect(execution.schedule?.concurrencyPolicy).toBeUndefined();
});


it('explains a rejected single-replica PDB and recovers after correcting replicas without losing edits',async()=>{
 const template:WorkloadTemplate={...backendTemplate,runtimeDefaults:{replicas:1}};
 const mock=serve(false,effective,template);
 const fetcher=vi.mocked(fetch).getMockImplementation()!;
 vi.mocked(fetch).mockImplementation(async(input,init)=>{
  if(String(input).endsWith('/resolve-defaults')){
   const raw=JSON.parse(String(init?.body)).workload;
   if(raw.workloadExecution?.podDisruptionBudget?.enabled && (raw.replicas??1)<2)return Response.json({code:'INVALID_WORKLOAD',error:'raw SECRET'}, {status:400});
  }
  return fetcher(input,init);
 });
 const user=await author();await openWorkloadGroup(user,'Mở rộng & cập nhật');
 await user.selectOptions(screen.getByLabelText('PodDisruptionBudget'),'on');
 await waitFor(()=>expect(screen.getByRole('alert')).toHaveTextContent('Cần ít nhất 2 replica'));
 expect(screen.getByLabelText('Ảnh Container')).toHaveValue('nginx:1');expect(screen.queryByText(/SECRET/)).not.toBeInTheDocument();
 await user.type(screen.getByLabelText('Số replica'),'2');await waitFor(()=>expect(screen.queryByRole('alert')).not.toBeInTheDocument());
 await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));
 expect(mock.saved).toHaveLength(1);expect(mock.saved[0]!.workload.replicas).toBe(2);expect((mock.saved[0]!.workload.workloadExecution as Execution).podDisruptionBudget).toEqual({enabled:true});
});
