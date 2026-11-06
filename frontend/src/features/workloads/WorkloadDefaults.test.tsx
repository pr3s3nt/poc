import { render,screen,within,waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach,expect,it,vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import { bothConfigured } from '../../test/targets';
import type { Execution } from './templates';
const application={id:'defaults',name:'Defaults',environments:bothConfigured(),workloads:{staging:[],production:[]}};
afterEach(()=>vi.unstubAllGlobals());
// Nonsecret resolver response recorded from the real backend policy endpoint.
const effective={apiVersion:'orchestrator.dev/workload/v1',metadata:{name:'web'},containers:{main:{image:'nginx:1',resources:{requests:{cpu:'100m',memory:'128Mi'},limits:{memory:'512Mi'}}}},workloadExecution:{version:1,kind:'Deployment',defaults:{version:'workload-defaults-v1',environment:'staging'},automountServiceAccountToken:false,revisionHistoryLimit:3,updateStrategy:{type:'RollingUpdate',rollingUpdate:{maxUnavailable:0,maxSurge:1}}}};
function serve(failure=false,responseWorkload: Record<string,unknown>=effective){
 const saved:{workload:Record<string,unknown>}[]=[];let fail=failure;
 vi.stubGlobal('fetch',vi.fn(async(input:string|Request|URL,init?:RequestInit)=>{
 const url=String(input);
 if(url.endsWith('/resolve-defaults'))return fail?Response.json({code:'UNAVAILABLE'},{status:503}):Response.json({workload:responseWorkload,sources:{'containers.main.resources.requests.cpu':'environment','containers.main.resources.requests.memory':'environment','containers.main.resources.limits.memory':'platform','workloadExecution.automountServiceAccountToken':'platform','workloadExecution.revisionHistoryLimit':'environment','workloadExecution.updateStrategy.type':'platform'}});
 if(url.endsWith('/configuration'))return Response.json({version:1,keys:[]});
 if(url.endsWith('/resource-types'))return Response.json({resourceTypes:[]});
 if(init?.method==='PUT'){saved.push(JSON.parse(String(init.body)));return Response.json({draftVersion:2,workloads:[]})}
 return Response.json({draftVersion:1,workloads:[]});
 }));return {saved,recover(){fail=false}};
}
async function author(){const user=userEvent.setup();render(<WorkloadEditorPage application={application} environment="staging"/>);await user.type(await screen.findByLabelText('Tên Workload'),'web');await user.type(screen.getByLabelText('Ảnh Container'),'nginx:1');return user}
it('displays backend effective values and saves inherited values as omissions',async()=>{
 const {saved}=serve();const user=await author();await waitFor(()=>expect(screen.getByText(/CPU 100m \(Theo môi trường\)/)).toBeInTheDocument());
 await user.click(screen.getByText('Tài nguyên tính toán',{exact:true}));const cpu=screen.getByLabelText('Yêu cầu CPU');expect(cpu).toHaveValue('');expect(cpu).toHaveAttribute('placeholder','100m');expect(screen.getByLabelText('Giới hạn bộ nhớ')).toHaveAttribute('placeholder','512Mi');
 await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect(saved).toHaveLength(1);expect(saved[0]!.workload.containers).toEqual({main:{image:'nginx:1'}});expect((saved[0]!.workload.workloadExecution as Execution).defaults?.version).toBe('workload-defaults-v1');
});
it('keeps explicit false and zero and reset removes the history override',async()=>{
 const {saved}=serve();const user=await author();await user.click(screen.getByText('Bảo mật Pod',{exact:true}));await user.selectOptions(screen.getByLabelText('automountServiceAccountToken'),'false');
 await user.click(screen.getByText('Mở rộng & cập nhật',{exact:true}));const history=screen.getByLabelText('revisionHistoryLimit');await user.type(history,'0');expect(history).toHaveValue(0);
 await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect((saved[0]!.workload.workloadExecution as Execution).revisionHistoryLimit).toBe(0);expect((saved[0]!.workload.workloadExecution as Execution).automountServiceAccountToken).toBe(false);
 await user.click(within(history.closest('label')!).getByRole('button',{name:'Khôi phục mặc định'}));await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect((saved[1]!.workload.workloadExecution as Execution).revisionHistoryLimit).toBeUndefined();
});
it('preserves draft and prevents save when default metadata fails, then retries safely',async()=>{
 const mock=serve(true);const user=await author();await waitFor(()=>expect(screen.getByText(/Chưa tải được cấu hình hiệu lực/)).toBeInTheDocument());await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect(mock.saved).toHaveLength(0);expect(screen.getByLabelText('Ảnh Container')).toHaveValue('nginx:1');mock.recover();await user.click(screen.getByRole('button',{name:'Thử lại'}));await waitFor(()=>expect(screen.getByText(/CPU 100m \(Theo môi trường\)/)).toBeInTheDocument());await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect(mock.saved).toHaveLength(1);
});

it('shows inherited template mounts, ports and readiness without saving them as overrides',async()=>{
 const {saved}=serve(false,{...effective,containers:{main:{...effective.containers.main,ports:[{name:'http',containerPort:8080}],readinessProbe:{type:'http',path:'/',port:8080}}},workloadExecution:{...effective.workloadExecution,volumeMounts:[{volume:'cache',container:'main',path:'/var/cache/nginx',readOnly:false},{volume:'tmp',container:'main',path:'/tmp',readOnly:false}]}});
 const user=await author();await screen.findByText(/cache → \/var\/cache\/nginx, tmp → \/tmp/);
 await user.click(screen.getByText('Kiểm tra sức khỏe',{exact:true}));expect(screen.getByText(/Theo template · http :8080 \/$/)).toBeInTheDocument();
 await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect(saved[0]!.workload.containers).toEqual({main:{image:'nginx:1'}});expect((saved[0]!.workload.workloadExecution as Execution).volumeMounts).toBeUndefined();
});

it('keeps a template-provided image omitted when editing runtime policy',async()=>{
 const {saved}=serve();const user=userEvent.setup();render(<WorkloadEditorPage application={application} environment="staging"/>);await user.type(await screen.findByLabelText('Tên Workload'),'web');await waitFor(()=>expect(screen.getByLabelText('Ảnh Container')).toHaveAttribute('placeholder','nginx:1'));
 await user.click(screen.getByText('Mở rộng & cập nhật',{exact:true}));await user.selectOptions(screen.getByLabelText('PodDisruptionBudget'),'off');await user.click(screen.getByRole('button',{name:'Lưu draft Workload'}));expect(saved).toHaveLength(1);expect(saved[0]!.workload.containers).toEqual({main:{}});expect((saved[0]!.workload.workloadExecution as Execution).podDisruptionBudget).toEqual({enabled:false});
});
