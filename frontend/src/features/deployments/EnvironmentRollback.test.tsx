import { render,screen,waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach,expect,it,vi } from 'vitest';
import { EnvironmentRollback } from './EnvironmentRollback';

afterEach(()=>vi.unstubAllGlobals());
it('reviews the whole Environment and requires typed confirmation before rollback',async()=>{
 const fetcher=vi.fn(async(input:string|URL|Request)=>String(input).endsWith('/rollback-preview')?Response.json({token:'pin',targetOperationId:'successful',empty:false,restore:['alpha','beta'],remove:['extra']}):Response.json({status:'SUCCEEDED',results:[]}));
 vi.stubGlobal('fetch',fetcher);const done=vi.fn();const user=userEvent.setup();
 render(<EnvironmentRollback applicationId="boutique" environment="staging" disabled={false} onCompleted={done}/>);
 expect(fetcher).not.toHaveBeenCalled();await user.click(screen.getByRole('button',{name:'Xem trước Rollback'}));
 expect(await screen.findByRole('dialog',{name:'Xác nhận Rollback môi trường'})).toBeInTheDocument();
 expect(screen.getByText(/alpha, beta/)).toBeInTheDocument();expect(screen.getByText(/Xóa workload: extra/)).toBeInTheDocument();
 const confirm=screen.getByRole('button',{name:'Rollback môi trường'});expect(confirm).toBeDisabled();
 await user.type(screen.getByRole('textbox',{name:'Nhập boutique để xác nhận'}),'boutique');await user.click(confirm);
 await waitFor(()=>expect(done).toHaveBeenCalledTimes(1));
 expect(JSON.parse((fetcher.mock.calls[1] as unknown as [string,RequestInit])[1].body as string)).toEqual({token:'pin',confirmation:'boutique'});
 expect(await screen.findByRole('status')).toHaveTextContent('Đã khôi phục toàn bộ môi trường.');
});
it('keeps failure visible without echoing raw provider text or auto-retrying',async()=>{
 const fetcher=vi.fn(async(input:string|URL|Request)=>String(input).endsWith('/rollback-preview')?Response.json({token:'pin',empty:true,restore:[],remove:['alpha']}):Response.json({error:'secret-provider-output',code:'RECOVERY_INCOMPLETE'},{status:409}));
 vi.stubGlobal('fetch',fetcher);const done=vi.fn();const user=userEvent.setup();
 render(<EnvironmentRollback applicationId="app" environment="staging" disabled={false} onCompleted={done}/>);
 await user.click(screen.getByRole('button',{name:'Xem trước Rollback'}));
 expect(await screen.findByText(/Đưa staging về rỗng/)).toBeInTheDocument();
 await user.type(screen.getByRole('textbox',{name:'Nhập app để xác nhận'}),'app');await user.click(screen.getByRole('button',{name:'Rollback môi trường'}));
 expect(await screen.findByRole('alert')).not.toHaveTextContent('secret-provider-output');expect(done).not.toHaveBeenCalled();expect(fetcher).toHaveBeenCalledTimes(2);
});
