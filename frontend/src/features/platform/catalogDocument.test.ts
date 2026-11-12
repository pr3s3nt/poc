import { describe, expect, it } from 'vitest';
import { authoringDocument, parseCatalogDocument } from './catalogDocument';
describe('catalog document', () => {
  it('parses equivalent YAML and JSON without interpreting resource placeholders', () => {
    const yaml = 'key: demo\ninputs: []\noutputs: []\n';
    expect(parseCatalogDocument(yaml)).toEqual(parseCatalogDocument('{"outputs":[],"key":"demo","inputs":[]}'));
    expect(parseCatalogDocument('value: "${resources.test.outputs.host}"').value).toBe('${resources.test.outputs.host}');
  });
  it.each(['key: a\nkey: b', 'key: a\n---\nkey: b', '[a,b]', 'key: [', 'key: &a [*a]', 'a'.repeat(1024 * 1024 + 1)])('rejects malformed or unbounded documents', value => {
    expect(() => parseCatalogDocument(value)).toThrow();
  });
  it('exports only authoring fields and preserves source and secret flags', () => {
    const source = { key: 'db', version: 4, revisionId: 'r4', renderPin: { credential: 'hidden' }, driverInputs: { values: { source: { revision: 'pin' } } }, criteria: [{}] };
    expect(authoringDocument('resource-definitions', source)).toEqual({ key: 'db', driverInputs: source.driverInputs, criteria: [{}] });
    expect(authoringDocument('resource-types', { key: 'db', classes: ['x'], outputs: [{ name: 'password', secret: true }] })).toEqual({ key: 'db', outputs: [{ name: 'password', secret: true }] });
  });
});
