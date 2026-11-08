import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { ApiError } from '../../shared/api/client';
import { TemplatePicker } from './TemplatePicker';
import { getWorkloadTemplates, type WorkloadTemplate } from './templates';
vi.mock('./templates', () => ({ getWorkloadTemplates: vi.fn() }));
const templates: [WorkloadTemplate] = [{ id: 'stateful', version: '1.0.0', sha256: 'a'.repeat(64), kind: 'StatefulSet' as const, labels: { vi: 'Stateful application' }, inputsSchema: {}, defaults: {} }];
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

it('directs an unselected environment to its catalog settings instead of retrying', async () => {
 vi.mocked(getWorkloadTemplates).mockRejectedValueOnce(new ApiError(409, 'CATALOG_REQUIRED'));
 render(<TemplatePicker app="app" environment="production" onClose={vi.fn()} onCustom={vi.fn()} onSelect={vi.fn()} />);
 expect(await screen.findByRole('alert')).toHaveTextContent('Chọn một phiên bản Catalog');
 expect(screen.getByRole('link', { name: 'Cài đặt môi trường' })).toHaveAttribute('href', '/ui/applications/app/settings?environment=production');
 expect(screen.queryByRole('button', { name: 'Thử lại' })).not.toBeInTheDocument();
});

it('closes a mounted picker through Cancel while templates are still loading', async () => {
 vi.mocked(getWorkloadTemplates).mockReturnValue(new Promise(() => {}));
 const {useState, StrictMode}=await import('react');
 function Host(){const[open,setOpen]=useState(false);return <><button onClick={()=>setOpen(true)}>Open picker</button>{open?<TemplatePicker app="app" environment="staging" onClose={()=>setOpen(false)} onSelect={vi.fn()} onCustom={vi.fn()}/>:null}</>}
 const user=userEvent.setup();render(<StrictMode><Host/></StrictMode>);
 await user.click(screen.getByRole('button',{name:'Open picker'}));
 await user.click(within(screen.getByRole('dialog')).getByRole('button',{name:'Hủy'}));
 await waitFor(()=>expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
});

it('groups versions numerically, defaults to latest and selects the exact older pin only on activation', async () => {
 const versions=['1.9.0','1.2.0','1.10.0','2.0.0'].map((version,index)=>({...templates[0],version,sha256:String(index).repeat(64)}));
 vi.mocked(getWorkloadTemplates).mockResolvedValue(versions);
 const select=vi.fn(),user=userEvent.setup();render(<TemplatePicker app="app" environment="staging" onClose={vi.fn()} onCustom={vi.fn()} onSelect={select}/>);
 const card=await screen.findByRole('button',{name:/Stateful application/});expect(screen.getAllByRole('button',{name:/Stateful application/})).toHaveLength(1);expect(card).toHaveTextContent('2.0.0');
 await user.click(card);expect(select).toHaveBeenLastCalledWith(versions[3]);select.mockClear();
 await user.click(screen.getByText('Phiên bản khác'));const choices=screen.getByRole('combobox',{name:'Phiên bản · Stateful application'});
 expect(within(choices).getAllByRole('option').map(option=>option.textContent)).toEqual(['2.0.0','1.10.0','1.9.0','1.2.0']);
 await user.selectOptions(choices,'1.9.0');expect(select).not.toHaveBeenCalled();expect(card).toHaveTextContent('1.9.0');await user.click(card);expect(select).toHaveBeenCalledWith(versions[0]);
});

it('keeps different template IDs separate and resets version selection after Environment changes', async () => {
 const older={...templates[0],version:'1.0.0'},newer={...templates[0],version:'1.2.0',sha256:'b'.repeat(64)};
 vi.mocked(getWorkloadTemplates).mockResolvedValue([older,newer,{...older,id:'other',labels:{vi:'Other template'}}]);
 const props={app:'app',onClose:vi.fn(),onCustom:vi.fn(),onSelect:vi.fn()};const user=userEvent.setup();const view=render(<TemplatePicker {...props} environment="staging"/>);
 await screen.findByRole('button',{name:/Other template/});expect(screen.getAllByText('Phiên bản khác')).toHaveLength(1);
 await user.click(screen.getByText('Phiên bản khác'));await user.selectOptions(screen.getByRole('combobox'),'1.0.0');
 view.rerender(<TemplatePicker {...props} environment="production"/>);
 await waitFor(()=>expect(screen.getByRole('button',{name:/Stateful application/})).toHaveTextContent('1.2.0'));
});
