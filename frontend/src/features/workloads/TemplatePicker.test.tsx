import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { TemplatePicker } from './TemplatePicker';
import { getWorkloadTemplates } from './templates';
vi.mock('./templates', () => ({ getWorkloadTemplates: vi.fn() }));
const templates = [{ id: 'stateful', version: '1.0.0', sha256: 'a'.repeat(64), kind: 'StatefulSet' as const, labels: { vi: 'Stateful application' }, inputsSchema: {}, defaults: {} }];
it('opens a scoped catalog, selects its immutable template and supports filtering', async () => {
 vi.mocked(getWorkloadTemplates).mockResolvedValue(templates);
 const select = vi.fn();const user = userEvent.setup();render(<TemplatePicker app="app" environment="staging" onClose={vi.fn()} onCustom={vi.fn()} onSelect={select} />);
 await user.click(await screen.findByRole('button', { name: /Stateful application/ }));expect(select).toHaveBeenCalledWith(templates[0]);expect(getWorkloadTemplates).toHaveBeenCalledWith('app','staging');
 await user.type(screen.getByRole('textbox'),'not-found');expect(screen.queryByRole('button',{name:/Stateful application/})).not.toBeInTheDocument();
});
it('reports catalog errors, retries explicitly and ignores late replies after scope changes', async () => {
 vi.mocked(getWorkloadTemplates).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(templates);
 const user = userEvent.setup();render(<TemplatePicker app="app" environment="staging" onClose={vi.fn()} onCustom={vi.fn()} onSelect={vi.fn()} />);
 await screen.findByRole('alert');await user.click(screen.getByRole('button',{name:'Thử lại'}));await waitFor(()=>expect(screen.getByRole('button',{name:/Stateful application/})).toBeInTheDocument());
});
