import { render, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { ExecutionProgress, type ExecutionRecord } from './ExecutionProgress';
const failed: ExecutionRecord={version:1,steps:[{id:'wait',workload:'backend',kind:'READINESS',status:'FAILED',startedAt:'2026-11-11T01:00:00Z',updatedAt:'2026-11-11T01:02:00Z',finishedAt:'2026-11-11T01:02:00Z',pods:[{name:'backend-1',image:'ghcr.io/acme/backend:missing',reason:'ImagePullBackOff',restarts:0}],events:[{object:'backend-1',reason:'Failed',at:'2026-11-11T01:01:30Z'}]}]};
it('shows persisted failure evidence and guidance after a fresh render without starting a timer',()=>{
 const timer=vi.spyOn(window,'setInterval');render(<ExecutionProgress record={failed} operationStatus="FAILED" />);
 expect(screen.getByRole('alert')).toHaveTextContent('ImagePullBackOff');expect(screen.getByRole('alert')).toHaveTextContent('Environment đã được mở khóa');
 expect(screen.getByRole('table')).toHaveTextContent('ghcr.io/acme/backend:missing');expect(screen.getByText(/Kiểm tra tên\/tag image/)).toBeInTheDocument();
 expect(screen.getByText('2:00')).toBeInTheDocument();expect(timer).not.toHaveBeenCalled();timer.mockRestore();
});
it('keeps an interrupted Environment held and does not invent guidance for an unknown failure',()=>{
 render(<ExecutionProgress record={{version:1,steps:[{...failed.steps[0]!,pods:[],reason:'UNKNOWN'}]}} operationStatus="INTERRUPTED" />);
 expect(screen.getByRole('alert')).toHaveTextContent('Environment vẫn được giữ');expect(screen.getByRole('alert')).not.toHaveTextContent('Kiểm tra tên/tag image');
});
it('keeps shared publication after workload preparation and names collapsed Terraform resources',()=>{
 const base={startedAt:'2026-11-11T01:00:00Z',updatedAt:'2026-11-11T01:00:10Z',finishedAt:'2026-11-11T01:00:10Z',status:'SUCCEEDED'};
 render(<ExecutionProgress record={{version:1,steps:[{...base,id:'claim',kind:'CLAIM'},{...base,id:'infra',workload:'backend',kind:'INFRASTRUCTURE'},{...base,id:'postgres',parent:'infra',kind:'TERRAFORM',resourceName:'postgres.in-cluster#db'},{...base,id:'publish',kind:'PUBLISH'}]}} />);
 expect(screen.getAllByRole('heading',{level:3}).map(h=>h.textContent)).toEqual(['Environment','backend','Environment']);
 expect(screen.getByText('postgres (Terraform)')).toBeInTheDocument();
});
it('explains a Fleet permission failure without pod evidence or raw controller output',()=>{
 render(<ExecutionProgress record={{version:1,steps:[{...failed.steps[0]!,kind:'APPLY',pods:[],reason:'FLEET_FORBIDDEN'}]}} operationStatus="FAILED" />);
 expect(screen.getByRole('alert')).toHaveTextContent('Fleet chưa có quyền truy cập tài nguyên trong namespace');
 expect(screen.getByRole('alert')).toHaveTextContent('Cần kỹ sư nền tảng kiểm tra quyền');
});
it('shows actual preparation results, explicit zero and no empty disclosure for legacy steps',()=>{
 const base={startedAt:'2026-11-11T01:00:00Z',updatedAt:'2026-11-11T01:00:01Z',status:'SUCCEEDED'};
 const {container}=render(<ExecutionProgress record={{version:1,steps:[
 {...base,id:'old',kind:'PLAN'},
 {...base,id:'none',kind:'SECRETS',secretBindings:0},
 {...base,id:'secret',kind:'SECRETS',secretBindings:3},
 {...base,id:'render',kind:'RENDER',rendered:{objects:3,kinds:[{kind:'Deployment',count:1},{kind:'Service',count:2}]}},
 ]}} />);
 expect(screen.getByText('Không có binding secret cần chuẩn bị.')).toBeInTheDocument();
 expect(screen.getByText('Đã chuẩn bị 3 binding secret.')).toBeInTheDocument();
 expect(screen.getByText('Đã render 3 object.')).toBeInTheDocument();
 expect(screen.getByText('Service: 2')).toBeInTheDocument();
 expect(container.querySelectorAll('.execution-step > details')).toHaveLength(3);
 expect(screen.getByText('Kiểm tra & lập kế hoạch').closest('details')).toBeNull();
});
it('does not present an interrupted secret preparation as prepared',()=>{
 render(<ExecutionProgress record={{version:1,steps:[{...failed.steps[0]!,id:'secret',kind:'SECRETS',status:'INTERRUPTED',pods:[],events:[],secretBindings:2}]}} />);
 expect(screen.getByText('Cần chuẩn bị 2 binding secret.')).toBeInTheDocument();
 expect(screen.queryByText(/Đã chuẩn bị/)).not.toBeInTheDocument();
});
