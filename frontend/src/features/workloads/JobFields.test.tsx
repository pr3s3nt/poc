import {render,screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {useState} from 'react';
import {vi} from 'vitest';
import {JobFields,WaitForJobsFields} from './JobFields';
import type {Execution} from './templates';

it('keeps explicit zero retries and optional Job deadline without saving or executing',async()=>{
 const change=vi.fn();function Host(){const[value,set]=useState<Execution>({version:1,kind:'Job'});return <JobFields value={value} onChange={v=>{change(v);set(v)}}/>}
 render(<Host/>);const user=userEvent.setup();await user.type(screen.getByLabelText('Số lần thử lại khi thất bại'),'0');await user.type(screen.getByLabelText('Thời gian chạy tối đa (giây)'),'300');expect(change).toHaveBeenLastCalledWith({version:1,kind:'Job',job:{backoffLimit:0,activeDeadlineSeconds:300}});
 await user.clear(screen.getByLabelText('Thời gian chạy tối đa (giây)'));expect(change).toHaveBeenLastCalledWith({version:1,kind:'Job',job:{backoffLimit:0}});
});
it('offers only other non-deleted Jobs, keeps missing selections repairable and never infers Service dependencies',async()=>{
 const change=vi.fn();render(<WaitForJobsFields name="web" value={{version:1,kind:'Deployment',waitForJobs:['missing']}} workloads={[{id:'migrate',runtime:'Job',ready:false},{id:'web',runtime:'Job',ready:false},{id:'api',runtime:'Deployment',ready:false},{id:'removed',runtime:'Job',state:'PENDING_DELETE',ready:false}]} onChange={change}/>);
 expect(screen.getAllByRole('checkbox')).toHaveLength(2);await userEvent.click(screen.getByLabelText('migrate'));expect(change).toHaveBeenLastCalledWith({version:1,kind:'Deployment',waitForJobs:['missing','migrate']});await userEvent.click(screen.getByLabelText(/missing/));expect(change).toHaveBeenLastCalledWith({version:1,kind:'Deployment'});
});
