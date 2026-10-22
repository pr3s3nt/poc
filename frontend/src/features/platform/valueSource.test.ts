import { describe, expect, it } from 'vitest';
import { typeFixture } from '../../test/types';
import { classChoices, compatibleOutputs, contextAllowed, contextPlaceholder, describeSource, parseDescriptor, parseSource, referencePlaceholder } from './valueSource';

const type = (key: string) => typeFixture.find((item) => item.key === key);

describe('value sources (UC-03 UI11-02)', () => {
  it('generates and parses the three reference scopes without losing the placeholder', () => {
    for (const [scope, id] of [[{ kind: 'environment' }, 'environments.@app.@env'], [{ kind: 'infra' }, '@infra'], [{ kind: 'resource', id: 'shared.db' }, 'shared.db']] as const) {
      const text = referencePlaceholder({ type: 'postgres', class: 'in-cluster', scope, output: 'host' });
      expect(text).toBe(`\${resources['postgres.in-cluster#${id}'].outputs.host}`);
      expect(parseSource(text)).toEqual({ kind: 'resource', ref: { type: 'postgres', class: 'in-cluster', scope, output: 'host' } });
    }
    expect(parseSource(contextPlaceholder('env.namespace'))).toEqual({ kind: 'context', path: 'env.namespace' });
  });

  it('keeps anything the controls cannot model as an advanced or literal source', () => {
    for (const text of ['${context.env.id}', '${resources["vpc.default#@infra"].outputs.id}', "${resources['vpc#@infra'].outputs.id}", "${resources['vpc.default#@infra'].outputs.a.b}"]) expect(parseSource(text).kind).toBe('advanced');
    expect(parseSource('${x} ${y}').kind).toBe('literal');
    expect(parseSource('db-${context.env.id}')).toEqual({ kind: 'literal', text: 'db-${context.env.id}' });
    expect(parseSource('')).toEqual({ kind: 'literal', text: '' });
  });

  it('offers only public outputs compatible with the target type', () => {
    expect(compatibleOutputs(type('postgres'), 'string').map((output) => output.name)).toEqual(['host', 'database', 'username']);
    expect(compatibleOutputs(type('postgres'), 'number').map((output) => output.name)).toEqual(['port']);
    expect(compatibleOutputs(type('postgres'), 'list(string)')).toEqual([]);
    expect(compatibleOutputs(type('vpc'), 'list(string)').map((output) => output.name)).toEqual(['subnetIds', 'privateSubnetIds']);
    expect(compatibleOutputs(undefined, 'string')).toEqual([]);
    expect(contextAllowed('string')).toBe(true);
    expect(contextAllowed('number')).toBe(false);
  });

  it('lists the default class first and the published ones, keeping an unknown current class', () => {
    expect(classChoices(type('postgres'))).toEqual(['default', 'cloud-managed', 'in-cluster']);
    expect(classChoices(type('postgres'), 'legacy')).toEqual(['default', 'cloud-managed', 'in-cluster', 'legacy']);
    expect(classChoices(undefined)).toEqual(['default']);
  });

  it('parses only complete type.class#scope descriptors', () => {
    expect(parseDescriptor('vpc.default#@infra')).toEqual({ type: 'vpc', class: 'default', scope: { kind: 'infra' } });
    expect(parseDescriptor('redis.audit#shared.cache')).toEqual({ type: 'redis', class: 'audit', scope: { kind: 'resource', id: 'shared.cache' } });
    for (const text of ['redis', 'redis.audit', 'redis#shared', '#x', "a.b#c'd"]) expect(parseDescriptor(text)).toBeUndefined();
  });

  it('describes a reference that is no longer in the metadata without dropping it', () => {
    const source = parseSource("${resources['cache.default#@infra'].outputs.url}");
    expect(describeSource(source, typeFixture).origin).toContain('chưa có trong metadata');
    expect(describeSource(parseSource("${resources['vpc.default#@infra'].outputs.id}"), typeFixture)).toEqual({ origin: 'Từ tài nguyên khác', value: 'vpc → id · default · Ngữ cảnh hạ tầng' });
  });
});
