import {test} from 'node:test';
import assert from 'node:assert/strict';
import {loadTarget} from './target-config.mjs';
import {parseCatalogConfig,parseResourceTypes,assertSameResourceType} from './catalog-config.mjs';
test('catalog override changes repository only, rejects retired input and unsafe URLs',()=>{
 const raw=loadTarget({target:'dev'},{},'/private').raw;
 const next=loadTarget({target:'dev','git-url':'https://git.company.example/catalog.git'},{},'/private').raw;
 assert.equal(next.catalog.repository,'https://git.company.example/catalog.git');
 assert.deepEqual(next.account,raw.account);assert.deepEqual(next.resourceTypes,raw.resourceTypes);
 for(const url of ['https://reader:secret@example.com/repo','http://example.com/repo'])assert.throws(()=>loadTarget({target:'dev','git-url':url},{},'/private'));
 assert.throws(()=>loadTarget({target:'dev','git-ref':'a'.repeat(40)},{},'/private'));
 assert.throws(()=>parseCatalogConfig({...next,resourceDefinitions:[]}));
 assert.throws(()=>parseCatalogConfig({...next,catalog:{...next.catalog,manifestPath:'../escape'}}));
});
test('schema prerequisites preserve flags and reject drift without printing values',()=>{
 const types=parseResourceTypes([{key:'storage',outputs:[{name:'claimName',type:'string',required:true}]}]);
 assert.doesNotThrow(()=>assertSameResourceType(types[0],types[0]));
 assert.throws(()=>assertSameResourceType({...types[0],outputs:[]},types[0]));
 assert.throws(()=>parseResourceTypes([{key:'storage',outputs:[{name:'claimName',type:'invalid'}]}]));
});
