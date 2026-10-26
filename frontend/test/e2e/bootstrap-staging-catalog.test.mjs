import { describe, expect, it } from 'vitest';
import { assertMatches, bootstrapCatalog, expectedDefinition, specs, BUNDLE } from './bootstrap-staging-catalog.mjs';

const pin = { source: { sourceId:'platform',moduleId:'k8s-namespace',revision:'v1',subdirectory:'.',contentSha256:'abc' }, outputs:[{name:'name'}] };
describe('operator staging catalog', () => {
 it('retains an exact saved configuration but rejects account, matching, pin and companion changes', () => {
   for (const spec of specs) {
     const expected = expectedDefinition(spec, pin, 'k8s-4f');
     expect(() => assertMatches({ ...structuredClone(expected), key:spec.id, version:4 },expected,spec.id)).not.toThrow();
     expect(() => assertMatches({ ...structuredClone(expected), criteria:[{}] },expected,spec.id)).toThrow(/Conflicting/);
     expect(() => assertMatches({ ...structuredClone(expected), provision:{extra:{}} },expected,spec.id)).toThrow(/Conflicting/);
   }
   const expected=expectedDefinition(specs[0],null,'k8s-4f');
   expect(() => assertMatches({...expected,driverAccount:'other'},expected,specs[0].id)).toThrow(/Conflicting/);
   const module=expectedDefinition(specs[1],pin,'k8s-4f');
   const changed=structuredClone(module);changed.driverInputs.values.source.revision='v2';
   expect(() => assertMatches(changed,module,specs[1].id)).toThrow(/Conflicting/);
 });
 it('completes prerequisite and conflict reads before allowing any UI write', async () => {
   const data={
     '/driver-accounts':{accounts:[{key:'k8s-4f',type:'INTERNAL',status:'VERIFYING'}]},
   };
   const paths=[];
   const page={request:{get:async url => {const path=new URL(url).pathname.replace('/api/v1','');paths.push(path);return {ok:()=>true,json:async()=>data[path]};}}};
   await expect(bootstrapCatalog(page,'http://localhost')).rejects.toThrow(/READY/);
   expect(paths).toEqual(['/driver-accounts']);
   data['/driver-accounts'].accounts[0].status='READY';
   data['/resource-types']={resourceTypes:specs.map(s=>({key:s.type,outputs:[{name:'password',secret:true}]}))};
   data['/render-bundles']={renderBundles:[]};
   await expect(bootstrapCatalog(page,'http://localhost')).rejects.toThrow(/unavailable/);
   data['/render-bundles']={renderBundles:[{id:BUNDLE,status:'AVAILABLE'}]};
   data['/resource-definition-capabilities']={capabilities:specs.filter(s=>s.module).map(s=>({...pin,resourceType:s.type,driverType:'terraform',module:s.module}))};
   data['/resource-definitions']={resourceDefinitions:[{key:specs[0].id,resourceType:'k8s-cluster',driverType:'existing-cluster',driverAccount:'other'}]};
   await expect(bootstrapCatalog(page,'http://localhost')).rejects.toThrow(/Conflicting/);
 });
});
