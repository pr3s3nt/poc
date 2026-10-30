import {render,screen,within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach,expect,it,vi} from 'vitest';
import {ReconciliationPanel} from './ReconciliationPanel';
afterEach(()=>vi.unstubAllGlobals());
const observation=(time=new Date().toISOString())=>({scope:{unitKey:'api/ROLLOUT'},intendedRevision:'admitted',observedRevision:'admitted',sync:'IN_SYNC',health:'HEALTHY',drift:'NONE',complete:true,observedAt:time});
it('shows persisted independent dimensions and exact revisions; refresh only reads',async()=>{
 const requests:string[]=[];
 vi.stubGlobal('fetch',vi.fn(async(_input:string|URL|Request,init?:RequestInit)=>{requests.push(init?.method??'GET');const lastKnown=observation();return Response.json({units:[{unitId:'unit',workloadId:'api',sequence:1,lastKnown,effective:lastKnown,fresh:true}],nextCursor:'',intervalSeconds:30});}));
 render(<ReconciliationPanel applicationId="app" environment="staging"/>);
 const panel=screen.getByRole('region',{name:'Đối soát workload'});
 expect(await within(panel).findByText('Đã đồng bộ')).toBeInTheDocument();expect(within(panel).getByText('Khỏe')).toBeInTheDocument();expect(within(panel).getByText('Không lệch')).toBeInTheDocument();
 await userEvent.click(within(panel).getByText('api · Ứng dụng'));expect(within(panel).getAllByText('admitted')).toHaveLength(2);
 await userEvent.click(within(panel).getByRole('button',{name:'Tải lại'}));expect(requests).toEqual(['GET','GET']);
});
it('shows stale observations as unknown instead of retaining a clean status',async()=>{
 const lastKnown=observation(new Date(Date.now()-100_000).toISOString());
 vi.stubGlobal('fetch',vi.fn(async()=>Response.json({units:[{unitId:'unit',workloadId:'api',sequence:1,lastKnown,effective:lastKnown,fresh:true}],nextCursor:'',intervalSeconds:30})));
 render(<ReconciliationPanel applicationId="app" environment="staging"/>);
 expect(await screen.findAllByText('Chưa xác định')).toHaveLength(3);expect(screen.queryByText('Không lệch')).not.toBeInTheDocument();expect(screen.getByText(/Dữ liệu cũ/)).toBeInTheDocument();
});
