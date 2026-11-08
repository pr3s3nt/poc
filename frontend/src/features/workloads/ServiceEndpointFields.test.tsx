import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { ServiceEndpointFields } from './ServiceEndpointFields';
import type { Dependency } from './DependencyFields';
const props={dependencies:[] as Dependency[],ports:[{name:'http',port:'8080'}],available:true,serviceEnabled:true,workloadName:'web',disabled:false};
it('creates one private endpoint only after an explicit port action and avoids alias collisions',async()=>{
 const user=userEvent.setup(),change=vi.fn(),deps=[{alias:'public',type:'postgres',className:'',sharedId:'db',params:{}}];
 render(<ServiceEndpointFields {...props} dependencies={deps} onChange={change}/>);
 expect(change).not.toHaveBeenCalled();await user.click(screen.getByRole('button',{name:'Tạo endpoint từ cổng http'}));
 expect(change).toHaveBeenCalledWith([...deps,{alias:'public-2',type:'endpoint',className:'',sharedId:'',params:{name:'web',port:'http',path:'/'}}]);
});
it('edits the original endpoint without changing its alias, class or unknown params',async()=>{
 const endpoint={alias:'edge',type:'endpoint',className:'external',sharedId:'',params:{name:'shop',port:'http',path:'/api',extra:'keep'}};
 const change=vi.fn(),user=userEvent.setup();render(<ServiceEndpointFields {...props} dependencies={[endpoint]} onChange={change}/>);
 expect(screen.queryByRole('button',{name:/Tạo endpoint/})).not.toBeInTheDocument();
 await user.type(screen.getByLabelText('Đường dẫn endpoint'),'v2');
 expect(change.mock.lastCall?.[0][0]).toEqual({...endpoint,params:{...endpoint.params,path:'/api2'}});
});
it('does not offer exposure for UDP or a disabled Service',()=>{
 const change=vi.fn(),{rerender}=render(<ServiceEndpointFields {...props} ports={[{name:'dns',port:'53',protocol:'UDP'}]} onChange={change}/>);
 expect(screen.queryByRole('button')).not.toBeInTheDocument();
 rerender(<ServiceEndpointFields {...props} serviceEnabled={false} onChange={change}/>);
 expect(screen.getByRole('button',{name:/Tạo endpoint/})).toBeDisabled();expect(change).not.toHaveBeenCalled();
});
