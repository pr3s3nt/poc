// Legacy paths stay covered with switches enabled; production defaults have separate coverage.
vi.mock('../../shared/authoringFeatures', () => ({ authoringFeatures: { workloadImport: true, nonGitDefinitionSources: true } }));
import {render,screen,waitFor,within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach,expect,it,vi} from 'vitest';
import {ResourceDefinitionsPage} from './ResourceDefinitionsPage';
import {SavedSourceSummary} from './SavedSourceSummary';
import {TemplateDriverFields,emptyTemplateDriver} from './TemplateDriverFields';
import {capabilityFixture} from '../../test/capabilities';
import {backend,field} from '../../test/definitionKit';

const base=capabilityFixture.find(c=>c.module==='vpc')!;
const git={...base,source:{...base.source!,sourceId:'git-owned',revision:'a'.repeat(40),repositoryUrl:'https://github.com/company/platform.git',subdirectory:'modules/vpc'}};
const pin={source_id:git.source.sourceId,module_id:git.source.moduleId,revision:git.source.revision,subdirectory:git.source.subdirectory,content_sha256:git.source.contentSha256};
const stored={key:'git-vpc',name:'Git VPC',version:1,resourceType:'vpc',driverType:'terraform',driverAccount:'aws-account',driverInputs:{values:{source:pin,variables:{name:'vpc',region:'ap-southeast-1'},input_mapping:{},output_mapping:{id:'id',cidr:'cidr',subnetIds:'subnetIds',privateSubnetIds:'privateSubnetIds'},provider_bindings:{aws:{target:'account'}}}},provision:{},criteria:[{}]};
afterEach(()=>vi.unstubAllGlobals());
it('reopens exact Git origin and preserves pin on parameter-only save even with a newer module',async()=>{
 const newer={...git,source:{...git.source,revision:'b'.repeat(40),contentSha256:'c'.repeat(64)}};
 const {puts,posted}=backend({definitions:[stored],capabilities:[...capabilityFixture.filter(c=>c.module!=='vpc'),git,newer]});
 const user=userEvent.setup();render(<ResourceDefinitionsPage/>);
 await user.click(await screen.findByRole('button',{name:'Sửa cấu hình git-vpc'}));
 const summary=screen.getByLabelText('Nguồn đã lưu');expect(summary).toHaveTextContent('Git');expect(summary).toHaveTextContent(git.source.revision);
 expect(within(summary).getByRole('link',{name:git.source.revision})).toHaveAttribute('href',`https://github.com/company/platform/tree/${git.source.revision}/modules/vpc`);
 expect(screen.queryByLabelText('Module đã kiểm chứng')).not.toBeInTheDocument();
 await user.clear(field('Tên hiển thị'));await user.type(field('Tên hiển thị'),'Renamed VPC');
 await user.click(screen.getByRole('button',{name:'Lưu thay đổi'}));await waitFor(()=>expect(puts).toHaveLength(1));
 expect((puts[0]!.body.driverInputs as {values:{source:unknown}}).values.source).toEqual(pin);expect(posted).toHaveLength(0);
});
it('never substitutes the sole newer module when the saved pin is unavailable',async()=>{
 backend({definitions:[stored],capabilities:[{...git,source:{...git.source,revision:'b'.repeat(40)}}]});
 const user=userEvent.setup();render(<ResourceDefinitionsPage/>);
 await user.click(await screen.findByRole('button',{name:'Sửa cấu hình git-vpc'}));
 expect(field('Module đã kiểm chứng')).toHaveValue('');expect(screen.getByRole('button',{name:'Lưu thay đổi'})).toBeDisabled();
});
it.each(['zip','inline'] as const)('labels %s provenance without inventing Git links',kind=>{
 render(<SavedSourceSummary source={{kind,url:'javascript:alert(1)',path:'.',revision:'a'.repeat(64)}}/>);
 expect(screen.getByLabelText('Nguồn đã lưu')).toHaveTextContent(kind==='zip'?'ZIP':'Inline');expect(screen.queryByRole('link')).not.toBeInTheDocument();
});
it('reopens a Git template without calling it Catalog or exposing source controls until requested',async()=>{
 const source={id:'11111111-1111-4111-8111-111111111111',version:'1',status:'AVAILABLE',name:'Git template',inputsSchema:{type:'object',properties:{}},engineContract:'template-v1',inputContract:'orchestrator.dev/template-input/v1',renderer:{type:'template',version:'1.0.0'},source:{kind:'git',url:git.source.repositoryUrl,path:'templates',revision:git.source.revision,contentSha256:'a'.repeat(64)}};
 vi.stubGlobal('fetch',vi.fn(async()=>Response.json({templateSources:[source]})));
 const onChange=vi.fn(),user=userEvent.setup();render(<TemplateDriverFields editing value={{...emptyTemplateDriver(),source:source.id}} disabled={false} onChange={onChange}/>);
 const summary=await screen.findByLabelText('Nguồn đã lưu');expect(summary).toHaveTextContent('Git');expect(summary).toHaveTextContent(source.source.revision);
 expect(screen.queryByRole('button',{name:'Catalog'})).not.toBeInTheDocument();expect(screen.queryByLabelText('Nguồn đã xác thực')).not.toBeInTheDocument();expect(onChange).not.toHaveBeenCalled();
 await user.click(screen.getByRole('button',{name:'Đổi nguồn'}));expect(screen.getByRole('button',{name:'Nguồn đã đăng ký'})).toBeVisible();expect(field('Nguồn đã xác thực')).toHaveValue(source.id);expect(onChange).not.toHaveBeenCalled();
});
