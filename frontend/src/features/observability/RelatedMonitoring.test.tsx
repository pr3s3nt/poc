import { render, screen, act } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { ResourceMonitoring } from './RelatedMonitoring';
afterEach(() => vi.unstubAllGlobals());
const observation = (id:string,name:string) => ({id,name,kind:'resource',state:'PARTIAL',configuration:'APPLIED',collectorWorkload:'exporter',profileVersion:'db-v1',metrics:{connections:{unit:'count',state:'MISSING',series:[]}},alerts:[]});
it('keeps an external resource failure explicit and does not show native history', async () => {
 vi.stubGlobal('fetch',vi.fn(async()=>Response.json({code:'SERVICE_UNAVAILABLE'},{status:503})));
 render(<ResourceMonitoring applicationId="app" environment="staging" resourceId="db"/>);
 expect(await screen.findByRole('alert')).toBeInTheDocument();
 expect(screen.queryByText('native-history')).not.toBeInTheDocument();
});
it('does not display a late observation from a different resource', async () => {
 let finish:(r:Response)=>void=()=>{};
 vi.stubGlobal('fetch',vi.fn(async(input:unknown)=>String(input).includes('/old?')?new Promise<Response>(r=>{finish=r}):Response.json(observation('new','Current database'))));
 const view=render(<ResourceMonitoring applicationId="app" environment="staging" resourceId="old"/>);
 view.rerender(<ResourceMonitoring applicationId="app" environment="staging" resourceId="new"/>);
 await screen.findByRole('region',{name:'Current database'});
 await act(async()=>finish(Response.json(observation('old','Old database'))));
 expect(screen.queryByText('Old database')).not.toBeInTheDocument();
 expect(screen.getByText(/connections \(count\)/)).toBeInTheDocument();
 expect(screen.getByText('Thiếu dữ liệu')).toBeInTheDocument();
});
