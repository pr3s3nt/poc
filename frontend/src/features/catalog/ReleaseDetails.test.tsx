import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { ReleaseDetails } from './ReleaseDetails';
afterEach(()=>vi.unstubAllGlobals());
it('shows release configuration read-only and retries a failed detail read',async()=>{
 let fail=true;const methods:string[]=[];
 vi.stubGlobal('fetch',vi.fn(async (_url,init?:RequestInit)=>{methods.push(init?.method??'GET');return fail?Response.json({code:'SERVICE_UNAVAILABLE'},{status:503}):Response.json({release:{components:[{id:'web',kind:'definition',path:'definitions/web.json',sha256:'abc',snapshot:{criteria:[{class:'web'}],driverInputs:{values:{variables:{namespace:'example'}}},provision:{}}}],templates:[{id:'service',version:'1.0.0',kind:'service',inputs:{replicas:2}}]}});}));
 render(<ReleaseDetails source="platform" tag="v1.0.0"/>);
 const alert=await screen.findByRole('alert');expect(alert).toBeInTheDocument();fail=false;
 await userEvent.setup().click(screen.getByRole('button',{name:'Tải lại'}));
 expect(await screen.findByText('web',{selector:'strong'})).toBeInTheDocument();
 expect(screen.getByText('definitions/web.json')).toBeInTheDocument();
 expect(screen.getByText(/"namespace": "example"/)).toBeInTheDocument();
 expect(screen.queryByRole('textbox')).not.toBeInTheDocument();expect(methods).toEqual(['GET','GET']);
});
it('ignores a detail response arriving after changing release',async()=>{
 let complete:(r:Response)=>void=()=>{};
 vi.stubGlobal('fetch',vi.fn((url:string)=>url.endsWith('v1.0.0')?new Promise<Response>(resolve=>{complete=resolve;}):Promise.resolve(Response.json({release:{components:[{id:'new-release',kind:'definition',path:'definitions/new.json',sha256:'abc'}],templates:[]}}))));
 const {rerender}=render(<ReleaseDetails source="platform" tag="v1.0.0"/>);
 rerender(<ReleaseDetails source="platform" tag="v2.0.0"/>);
 await screen.findByText('new-release');complete(Response.json({release:{components:[{id:'old-release'}],templates:[]}}));
 expect(screen.queryByText('old-release')).not.toBeInTheDocument();
});
