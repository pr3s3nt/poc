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
