import test from 'node:test';
import assert from 'node:assert/strict';
import {expandParameters,validateParameters} from './input-parameters.mjs';
test('nonsecret parameter expansion preserves typed scalars, literal product references and escaping',()=>{
 const params=validateParameters({name:'a"b\nnext',enabled:false,replicas:2});
 const value=expandParameters({name:'${bootstrap.name}',text:'prefix-${bootstrap.name}',enabled:'${bootstrap.enabled}',list:['${bootstrap.replicas}','${context.env.namespace}','${resources.db.host}','${templateSources.acceptance}']},params);
 assert.equal(value.name,params.name);assert.equal(value.enabled,false);assert.equal(value.list[0],2);assert.equal(value.list[1],'${context.env.namespace}');assert.equal(value.list[2],'${resources.db.host}');assert.equal(value.list[3],'${templateSources.acceptance}');assert.deepEqual(JSON.parse(JSON.stringify(value)),value);
 assert.throws(()=>expandParameters('${bootstrap.missing}',params),/Missing bootstrap parameter/);
 assert.throws(()=>validateParameters({name:'${bootstrap.other}'}),/Invalid/);
 assert.throws(()=>validateParameters({secret:{token:'not-allowed'}}),/Invalid/);
});
