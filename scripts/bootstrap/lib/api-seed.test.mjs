import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, chmodSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { loadConfig, seed, expand, matches, definitionMatches } from './api-seed.mjs';

function fixture(t, patch = {}) {
 const dir = mkdtempSync(join(tmpdir(), 'orch-api-seed-'));
 t.after(() => rmSync(dir, { recursive: true }));
 writeFileSync(join(dir, 'password'), 'private-test-password', { mode: 0o600 });
 const config = { schemaVersion: 1, console: { url: 'http://127.0.0.1:9090', username: 'pe', passwordFile: 'password', expectedOrganization: 'acme' }, ...patch };
 const file = join(dir, 'input.yaml');writeFileSync(file, JSON.stringify(config), { mode: 0o600 });
 return { file, dir, config };
}
test('loads local credential files but refuses broad permissions and inline secrets', t => {
 const {file,dir}=fixture(t);
 assert.equal(loadConfig(file).console.password,'private-test-password');
 chmodSync(join(dir,'password'),0o644);assert.throws(()=>loadConfig(file),/owner-only/);
 chmodSync(join(dir,'password'),0o600);
 const other=fixture(t,{git:{url:'https://example.test/org',token:'secret-inline'}});
 assert.throws(()=>loadConfig(other.file),/file references/);
});
test('rejects duplicate YAML keys and insecure remote endpoint', t => {
 const {file}=fixture(t);writeFileSync(file,'schemaVersion: 1\nschemaVersion: 1\n');assert.throws(()=>loadConfig(file),/YAML/);
 const other=fixture(t,{console:{url:'http://untrusted.example',username:'pe',passwordFile:'password',expectedOrganization:'acme'}});
 assert.throws(()=>loadConfig(other.file),/HTTPS/);
});
test('preloads all required secrets before any network call', t => {
 const {file}=fixture(t,{accounts:[{kind:'aws',name:'aws-dev',accessKeyIdFile:'missing'}]});
 assert.throws(()=>loadConfig(file),/unavailable/);
});
test('expansion preserves product expressions and rejects missing seed references',()=>{
 assert.deepEqual(expand({account:'${accounts.dev}',value:'${context.env.namespace}'},{accounts:{dev:'key'}}),{account:'key',value:'${context.env.namespace}'});
 assert.throws(()=>expand('${accounts.absent}',{accounts:{}}),/Missing/);
 assert.equal(matches({key:'x',version:3,inputs:[]},{key:'x',inputs:[]}),true);
 assert.equal(matches({key:'x'},{key:'y'}),false);
});
function server(role='PLATFORM_ENGINEER') {
 const calls=[];const types=[];
 return {calls,types,fetcher:async(raw,options)=>{
  const path=new URL(raw).pathname.replace('/api/v1','');calls.push({path,method:options.method});assert.equal(options.redirect,'error');
  if(path==='/auth/sign-in')return new Response(JSON.stringify({user:{Role:role,OrganizationKey:'acme'}}),{headers:{'set-cookie':'orchestrator_session=private-session; HttpOnly'}});
  if(path==='/auth/sign-out')return new Response(null,{status:204});
  assert.equal(options.headers.Cookie,'orchestrator_session=private-session');
  if(path==='/resource-types'){
   if(options.method==='POST'){types.push(JSON.parse(options.body));return Response.json(types.at(-1));}
   return Response.json({resourceTypes:types});
  }
  throw Error('unexpected request');
 }};
}
test('first run creates, rerun reuses and conflict never overwrites', async t=>{
 const c=loadConfig(fixture(t,{resourceTypes:[{key:'test-type',inputs:[],outputs:[]}]}).file);
 const s=server();const report=[];
 await seed(c,{fetcher:s.fetcher,report:x=>report.push(x)});
 await seed(c,{fetcher:s.fetcher,report:x=>report.push(x)});
 assert.equal(s.calls.filter(c=>c.path==='/resource-types'&&c.method==='POST').length,1);
 assert.ok(report.includes('ResourceType CREATED'));assert.ok(report.includes('ResourceType REUSED'));
 s.types[0].inputs=[{name:'different'}];await assert.rejects(seed(c,{fetcher:s.fetcher,report:()=>{}}),/CONFLICT/);
 assert.equal(s.calls.filter(c=>c.path==='/resource-types'&&c.method==='POST').length,1);
 assert.equal(s.calls.at(-1).path,'/auth/sign-out');
 assert.ok(!report.join('\n').includes('private-'));
});
test('wrong role fails before writes and still signs out',async t=>{
 const c=loadConfig(fixture(t).file);const s=server('DEVELOPER');
 await assert.rejects(seed(c,{fetcher:s.fetcher}),/insufficient role/);
 assert.deepEqual(s.calls.map(c=>c.path),['/auth/sign-in','/auth/sign-out']);
});
test('transport failure is not retried and does not echo secret-bearing exception',async t=>{
 const c=loadConfig(fixture(t).file);let calls=0;
 await assert.rejects(seed(c,{fetcher:async()=>{calls++;throw Error('secret-value');}}),e=>!e.message.includes('secret-value')&&e.message.includes('transport failed'));
 assert.equal(calls,1);
});

test('product password output mappings remain ordinary documents and control text cannot become a token', t => {
 const f=fixture(t,{definitions:[{key:'db',name:'DB',driverInputs:{values:{output_mapping:{password:'password'}}}}]});
 assert.equal(loadConfig(f.file).definitions[0].driverInputs.values.output_mapping.password,'password');
 writeFileSync(join(f.dir,'git-token'),'WSL error\nnot a token',{mode:0o600});
 writeFileSync(f.file,JSON.stringify({...f.config,git:{url:'https://example.test/org',username:'bot',tokenFile:'git-token'}}));
 assert.throws(()=>loadConfig(f.file),/control characters/);
 assert.equal(matches({name:'password',type:'string'},{name:'password',type:'string',secret:false}),true);
});

test('Definition reuse detects newly added variables and provision rules', () => {
 const desired={key:'d',name:'D',resourceType:'db',driverType:'terraform',driverInputs:{values:{variables:{}}},criteria:[{}],provision:{}};
 assert.equal(definitionMatches({...desired,version:2},desired),true);
 assert.equal(definitionMatches({...desired,driverInputs:{values:{variables:{changed:'value'}}}},desired),false);
 assert.equal(definitionMatches({...desired,provision:{new:{isDependent:true}}},desired),false);
});

test('tailnet HTTP opt-in rejects public 100.x addresses and hostname tricks', t => {
 for (const url of ['http://100.1.2.3:3070','http://100.attacker.test:3070']) {
  const {file}=fixture(t,{console:{url,tailnet:true,username:'pe',passwordFile:'password',expectedOrganization:'acme'}});
  assert.throws(()=>loadConfig(file),/HTTPS/);
 }
 const {file}=fixture(t,{console:{url:'http://100.77.224.24:3070',tailnet:true,username:'pe',passwordFile:'password',expectedOrganization:'acme'}});
 assert.equal(loadConfig(file).console.origin,'http://100.77.224.24:3070');
});

test('prepares only Organization identity without credentials or overwriting output', async t => {
 const {prepareOrganization} = await import('./api-seed.mjs');
 const {readFileSync,statSync} = await import('node:fs');
 const f=fixture(t,{organization:{key:'my-company',name:'Công ty của tôi'},console:{expectedOrganization:'my-company',passwordFile:'missing'}});
 const output=join(f.dir,'organization.json');
 prepareOrganization(f.file,output);
 assert.deepEqual(JSON.parse(readFileSync(output,'utf8')),{key:'my-company',name:'Công ty của tôi'});
 assert.equal(statSync(output).mode&0o077,0);
 assert.throws(()=>prepareOrganization(f.file,output),/must be new/);
});
test('Organization configuration rejects mismatching keys and invalid identity before sign-in', t=>{
 const f=fixture(t,{organization:{key:'other-company',name:'Other'}});
 assert.throws(()=>loadConfig(f.file),/conflicts/);
 for(const org of [{key:'bad key',name:'Company'},{key:'company',name:' '},{key:'company',name:'line\nbreak'},{key:'company',name:'Company',token:'secret'}]) {
  const other=fixture(t,{organization:org});assert.throws(()=>loadConfig(other.file),/identity/);
 }
});
test('checks Organization name before platform writes and signs out on mismatch', async t=>{
 const f=fixture(t,{organization:{key:'acme',name:'My Company'},resourceTypes:[{key:'new-type',inputs:[],outputs:[]}]});
 const c=loadConfig(f.file),s=server();let name='Wrong Company';
 const fetcher=async(raw,options)=>new URL(raw).pathname==='/api/v1/organization'?(s.calls.push({path:'/organization',method:options.method}),Response.json({organization:{key:'acme',name}})):s.fetcher(raw,options);
 await assert.rejects(seed(c,{fetcher,report:()=>{}}),/Organization: CONFLICT/);
 assert.deepEqual(s.calls.map(c=>c.path),['/auth/sign-in','/organization','/auth/sign-out']);
 name='My Company';await seed(c,{fetcher,report:()=>{}});await seed(c,{fetcher,report:()=>{}});
 assert.equal(s.calls.filter(c=>c.path==='/resource-types'&&c.method==='POST').length,1);
});
test('allows Organization key/name without legacy expectedOrganization', t=>{
 const f=fixture(t,{organization:{key:'my-company',name:'My Company'},console:{url:'http://127.0.0.1:9090',username:'admin',passwordFile:'password'}});
 assert.equal(loadConfig(f.file).organization.key,'my-company');
});
