import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { JobHistoryDialog } from './JobHistoryDialog';
import { api } from '../../shared/api/client';
vi.mock('../../shared/api/client', async importOriginal => ({ ...await importOriginal<typeof import('../../shared/api/client')>(), api: vi.fn() }));
const history = { generatedAt: '2026-10-28T00:00:00Z', code: 'OK', truncated: false, jobs: [{ name: 'backup-123', uid: 'job-one', createdAt: '2026-10-28T00:00:00Z', state: 'Succeeded' }] };
it('shows observed Job outcome separately from schedule and restores focus on close', async () => {
 vi.mocked(api).mockResolvedValue(history);
 const close = vi.fn(); render(<JobHistoryDialog applicationId="app" environment="staging" workload="backup" onClose={close} />);
 expect(await screen.findByText('backup-123')).toBeVisible(); expect(screen.getByText('Thành công')).toBeVisible();
 expect(api).toHaveBeenCalledWith('/applications/app/environments/staging/workloads/backup/jobs', expect.objectContaining({ signal: expect.any(AbortSignal) }));
 await userEvent.click(screen.getByRole('button', {name:'Đóng'})); expect(close).toHaveBeenCalled();
});
it('shows unavailable as error and retries rather than a successful empty history', async () => {
 vi.mocked(api).mockResolvedValueOnce({...history,code:'FORBIDDEN',jobs:[]}).mockResolvedValueOnce(history);
 render(<JobHistoryDialog applicationId="app" environment="staging" workload="backup" onClose={vi.fn()} />);
 await screen.findByRole('alert'); expect(screen.queryByText('Không có Job được lưu.')).not.toBeInTheDocument();
 await userEvent.click(screen.getByRole('button',{name:'Tải lại'})); expect(await screen.findByText('backup-123')).toBeVisible();
});
it('drops a late response after environment changes', async () => {
 let resolve!: (value: unknown) => void;
 vi.mocked(api).mockImplementationOnce(() => new Promise(done => {resolve=done;})).mockResolvedValueOnce({...history,jobs:[]});
 const {rerender}=render(<JobHistoryDialog applicationId="app" environment="staging" workload="backup" onClose={vi.fn()} />);
 rerender(<JobHistoryDialog applicationId="app" environment="production" workload="backup" onClose={vi.fn()} />);
 await screen.findByText('Không có Job được lưu.'); resolve(history);
 expect(screen.queryByText('backup-123')).not.toBeInTheDocument();
});
