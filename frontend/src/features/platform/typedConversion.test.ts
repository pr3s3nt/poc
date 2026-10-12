import { describe, expect, it } from 'vitest';
import { capabilityFixture } from '../../test/capabilities';
import { capabilityFor } from './capabilities';
import { draftToVariables, entryValue, variablesToDraft, type JsonObject } from './typedVariables';
import { provisionToRows, rowsToProvision } from './typedProvision';

const vars = capabilityFor(capabilityFixture, 'postgres', 'terraform')?.variables ?? [];
const anyVar = { name: 'blob', type: 'any', required: false, hasDefault: false, resourceParam: false };
const roundTrip = (object: JsonObject, variables = vars) => {
  const converted = variablesToDraft(object, variables);
  expect(converted.reason).toBeUndefined();
  return draftToVariables(converted.draft ?? {}, variables).value;
};

describe('typed variable conversion is lossless', () => {
  it('keeps whitespace, interpolation and escaped placeholders exactly', () => {
    const object = { name: ' ${context.infra.resourceName} ', database: 'db-${context.app.id}-x', username: '$${literal}', vpc_id: '${resources[\'vpc.default#@infra\'].outputs.id}' };
    expect(roundTrip(object)).toEqual(object);
  });
  it('does not treat a padded placeholder as whole for number', () => {
    const number = vars.find((variable) => variable.name === 'min_capacity')!;
    expect(entryValue(number, { on: true, raw: ' ${context.x} ' }).error).toBeTruthy();
    expect(entryValue(number, { on: true, raw: '${context.x}' }).value).toBe('${context.x}');
    expect(roundTrip({ min_capacity: 0.5, max_capacity: '${context.x}', subnet_ids: ['a', '${context.y}'] })).toEqual({ min_capacity: 0.5, max_capacity: '${context.x}', subnet_ids: ['a', '${context.y}'] });
  });
  it('round-trips every valid any value, plain strings included', () => {
    for (const value of ['plain', ' padded ', '', 3, true, [1, 'a'], { a: { b: null } }, '${context.app.id}']) {
      if (value === null) continue;
      expect(roundTrip({ blob: value }, [anyVar])).toEqual({ blob: value });
    }
  });
  it('refuses to import null or mismatching values into typed controls', () => {
    expect(variablesToDraft({ min_capacity: 'many' }, vars).reason).toBeTruthy();
    expect(variablesToDraft({ name: null }, vars).reason).toBeTruthy();
    expect(variablesToDraft({ region: 'x' }, vars).reason).toBeTruthy();
    expect(variablesToDraft({ master_password: 'x' }, vars).reason).toBeTruthy();
  });
});

describe('typed provision conversion is lossless', () => {
  const roundTrip = (object: JsonObject) => {
    const converted = provisionToRows(JSON.stringify(object));
    expect(converted.reason).toBeUndefined();
    return rowsToProvision(converted.rows ?? []).value;
  };
  it('keeps explicit false flags and empty params', () => {
    const object = { 'vpc.default#@infra': { is_dependent: false, match_dependents: false, params: {} }, 'k8s-namespace.default': {} };
    expect(JSON.stringify(roundTrip(object))).toBe(JSON.stringify(object));
  });
  it('keeps nested params and references', () => {
    const object = { 'postgres.audit#@infra': { is_dependent: true, params: { database: 'a', opts: { n: [1, true, '${resources[\'vpc.default#@infra\'].outputs.id}'] } } }, 'vpc.default#@infra': { match_dependents: true } };
    expect(JSON.stringify(roundTrip(object))).toBe(JSON.stringify(object));
  });
  it('omits flags newly created as false and stays advanced for unknown fields', () => {
    expect(rowsToProvision([{ id: 1, descriptor: 'vpc.default', params: '{}', isDependent: false, matchDependents: false }]).value).toEqual({ 'vpc.default': {} });
    expect(provisionToRows('{"a":{"extra":1}}').reason).toBeTruthy();
  });
});

// SYNTHETIC metadata: the real editable modules have no bool/map variables, so
// these generic-type cases use made-up variables only inside this test. No
// executor-owned variable is exposed.
const synthetic = [
  { name: 'enabled', type: 'bool', required: false, hasDefault: true, default: true, resourceParam: false },
  { name: 'labels', type: 'map(string)', required: false, hasDefault: false, resourceParam: false },
];
describe('generic bool and map typed controls (synthetic metadata)', () => {
  it('keeps false, true and a whole placeholder for bool', () => {
    for (const value of [false, true, '${context.env.enabled}']) {
      expect(roundTrip({ enabled: value }, synthetic)).toEqual({ enabled: value });
    }
    const converted = variablesToDraft({ enabled: '${context.env.enabled}' }, synthetic);
    expect(converted.draft?.enabled).toEqual({ on: true, raw: '${context.env.enabled}', placeholder: true });
    const bool = synthetic[0]!;
    expect(entryValue(bool, { on: true, raw: 'false' }).value).toBe(false);
    expect(entryValue(bool, { on: true, raw: 'maybe' }).error).toBeTruthy();
    expect(entryValue(bool, { on: true, raw: ' ${context.x} ', placeholder: true }).error).toBeTruthy();
  });
  it('omits unselected variables and never drops an explicit false', () => {
    expect(draftToVariables({ enabled: { on: false, raw: 'false' } }, synthetic).value).toEqual({});
    expect(draftToVariables({}, synthetic).value).toEqual({});
    expect(draftToVariables({ enabled: { on: true, raw: 'false' } }, synthetic).value).toEqual({ enabled: false });
  });
  it('round-trips map(string) with nested placeholders and escaped literals', () => {
    const labels = { whole: '${context.app.id}', escaped: '$${literal}', padded: ' x ', mixed: 'a-${context.env.namespace}' };
    expect(roundTrip({ labels }, synthetic)).toEqual({ labels });
    expect(roundTrip({ labels: '${context.env.labels}' }, synthetic)).toEqual({ labels: '${context.env.labels}' });
    expect(variablesToDraft({ labels: ['not', 'a', 'map'] }, synthetic).reason).toBeTruthy();
    expect(entryValue(synthetic[1]!, { on: true, raw: '[1]' }).error).toBeTruthy();
    expect(entryValue(synthetic[1]!, { on: true, raw: 'null' }).error).toBeTruthy();
  });
  it('rejects an executor-owned name even when synthetic metadata is used', () => {
    expect(variablesToDraft({ region: 'x' }, synthetic).reason).toBeTruthy();
  });
});
