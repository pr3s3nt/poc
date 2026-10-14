import { describe, expect, it } from 'vitest';
import { capabilityFixture } from '../../test/capabilities';
import { typeFixture } from '../../test/types';
import { clusterRef, emptyMapping, inputCompatible, mappingProblems, mappingToObject, needsAwsConnection, objectToMapping, outputCompatible, profileChoices, sameNameInputs, sameNameOutputs } from './moduleMapping';

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

  it('offers AWS-only modules the aws-eks profile alone, Kubernetes modules every scope', () => {
    expect(profileChoices(capability('aurora'))).toEqual(['aws-eks']);
    expect(needsAwsConnection(capability('vpc'))).toBe(true);
    expect(profileChoices(capability('postgres-k8s'))).toEqual(['internal-k8s', 'aws-eks', '']);
    expect(needsAwsConnection(capability('redis-k8s'))).toBe(false);
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
    // AWS always binds to the Environment; there is no account, region or Connection field to serialize.
    expect(mappingToObject(emptyMapping(), capability('aurora')).provider_bindings).toEqual({ aws: { target: 'environment' } });
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
      objectToMapping({ provider_bindings: { aws: { target: 'environment' } } }, pg, type('postgres'), []),
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
