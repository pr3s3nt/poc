import { describe, expect, it } from 'vitest';
import { capabilityFixture } from '../../test/capabilities';
import { typeFixture } from '../../test/types';
import { clusterRef, emptyMapping, inputCompatible, mappingProblems, inheritsCluster, mappingToObject, needsAccount, objectToMapping, outputCompatible, sameNameInputs, sameNameOutputs } from './moduleMapping';

const capability = (module: string) => {
  const found = capabilityFixture.find((candidate) => candidate.driverType === 'terraform' && candidate.module === module);
  if (!found) throw new Error(module);
  return found;
};
const type = (key: string) => typeFixture.find((candidate) => candidate.key === key);

describe('module mapping rules mirror the backend contract', () => {
  it('checks input and output compatibility like tfmodule (unknown defers to binding time)', () => {
    expect(inputCompatible('string', 'string')).toBe(true);
    expect(inputCompatible('string', 'number')).toBe(false);
    expect(inputCompatible('string', 'list(string)')).toBe(false);
    expect(inputCompatible('', 'list(string)')).toBe(true); // any contract input
    expect(inputCompatible('number', 'any')).toBe(true);
    expect(outputCompatible('number', undefined)).toBe(true); // type known only after apply
    expect(outputCompatible('number', 'string')).toBe(false);
    expect(outputCompatible('any', 'list(any)')).toBe(true);
    expect(outputCompatible('bool', 'bool')).toBe(true);
  });

  it('takes the account requirement from the published metadata: AWS modules need an account, Kubernetes modules inherit their cluster', () => {
    for (const module of ['aurora', 'vpc', 'eks', 'elasticache']) {
      expect(needsAccount(capability(module))).toBe(true);
      expect(capability(module).account).toEqual({ required: true, type: 'AWS', regionVariable: 'region' });
      expect(inheritsCluster(capability(module))).toBe(false);
    }
    for (const module of ['postgres-k8s', 'redis-k8s', 'k8s-namespace']) {
      expect(needsAccount(capability(module))).toBe(false);
      expect(inheritsCluster(capability(module))).toBe(true);
      // The Environment target stays selectable but is labelled legacy until V03.
      expect(capability(module).bindings[0]?.targets).toEqual([{ target: 'resource' }, { target: 'environment', legacy: true }]);
    }
  });

  it('serializes to the strict Driver Inputs shape and never invents a binding', () => {
    const pg = capability('postgres-k8s');
    const draft = emptyMapping();
    expect(mappingToObject(draft, pg)).toEqual({ input_mapping: {}, output_mapping: {}, provider_bindings: {} });
    draft.inputs = { database: 'database', username: 'username' };
    draft.outputs = { host: 'host', password: 'password' };
    draft.bindings = { kubernetes: { target: 'resource', clusterClass: 'eks' } };
    expect(mappingToObject(draft, pg)).toEqual({
      input_mapping: { database: { param: 'database' }, username: { param: 'username' } },
      output_mapping: { host: 'host', password: 'password' },
      provider_bindings: { kubernetes: { target: 'resource', resource_ref: clusterRef('eks') } },
    });
    // AWS always binds to the Definition's Driver Account; the account and region are separate fields, never serialized here.
    expect(mappingToObject(emptyMapping(), capability('aurora')).provider_bindings).toEqual({ aws: { target: 'account' } });
  });

  it('reads valid objects back and refuses what the controls cannot hold', () => {
    const pg = capability('postgres-k8s');
    const ok = objectToMapping({ input_mapping: { database: { param: 'database' } }, output_mapping: { host: 'host' }, provider_bindings: { kubernetes: { target: 'environment' } } }, pg, type('postgres'), ['eks']);
    expect(ok.draft).toEqual({ inputs: { database: 'database' }, outputs: { host: 'host' }, bindings: { kubernetes: { target: 'environment', clusterClass: '' } } });
    const reasons = [
      objectToMapping({ extra: 1 }, pg, type('postgres'), []),
      objectToMapping({ input_mapping: { database: { param: 'database', extra: 1 } } }, pg, type('postgres'), []),
      objectToMapping({ input_mapping: { password: { param: 'database' } } }, pg, type('postgres'), []), // executor-owned
      objectToMapping({ input_mapping: { database: { param: 'nope' } } }, pg, type('postgres'), []),
      objectToMapping({ input_mapping: { database: { param: 'database' }, username: { param: 'database' } } }, pg, type('postgres'), []),
      objectToMapping({ output_mapping: { nope: 'host' } }, pg, type('postgres'), []),
      objectToMapping({ output_mapping: { host: 'nope' } }, pg, type('postgres'), []),
      objectToMapping({ provider_bindings: { aws: { target: 'account' } } }, pg, type('postgres'), []), // not a provider of this module
      objectToMapping({ provider_bindings: { aws: { target: 'environment' } } }, capability('aurora'), type('postgres'), []), // an AWS provider never binds to the Environment
      objectToMapping({ provider_bindings: { kubernetes: { target: 'resource', resource_ref: clusterRef('other') } } }, pg, type('postgres'), ['eks']),
      objectToMapping({ provider_bindings: { kubernetes: { target: 'environment', region: 'x' } } }, pg, type('postgres'), []),
    ];
    for (const result of reasons) {
      expect(result.draft).toBeUndefined();
      expect(result.reason).toMatch(/JSON nâng cao/);
    }
  });

  it('guides the author through unmapped inputs, required outputs and unchosen targets', () => {
    const pg = capability('postgres-k8s');
    const problems = mappingProblems(emptyMapping(), pg, type('postgres')).map((problem) => problem.field);
    expect(problems).toEqual(['input:database', 'input:username', 'output:host', 'output:port', 'output:database', 'output:username', 'output:password', 'binding:kubernetes']);
    expect(mappingProblems(emptyMapping(), pg, undefined)).toHaveLength(1);
    const full = { inputs: { database: 'database', username: 'username' }, outputs: sameNameOutputs(pg, type('postgres')!), bindings: { kubernetes: { target: 'environment' as const, clusterClass: '' } } };
    expect(mappingProblems(full, pg, type('postgres'))).toEqual([]);
  });

  it('suggests same-name pairs only when the contract types allow it', () => {
    const aurora = capability('aurora');
    expect(sameNameInputs(aurora, type('postgres')!)).toEqual({ database: 'database', username: 'username' });
    expect(sameNameOutputs(aurora, type('postgres')!)).toEqual({ host: 'host', port: 'port', database: 'database', username: 'username', password: 'password' });
    expect(sameNameOutputs(capability('vpc'), type('vpc')!)).toEqual({ id: 'id', cidr: 'cidr', subnetIds: 'subnetIds', privateSubnetIds: 'privateSubnetIds' });
  });
});
