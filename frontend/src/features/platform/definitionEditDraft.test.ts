import { expect, it } from 'vitest';
import { applyFormChanges, editDocument, parseEditDocument } from './definitionEditDraft';

it('preserves unmodelled nested fields while applying form edits and explicit removals', () => {
  const original = { driverInputs: { values: { variables: { enabled: false, extra: { keep: 0 } }, source: { revision: 'exact', checksum: 'fixed' } } }, provision: { old: { params: { x: 1 } } } };
  const before = { driverInputs: { values: { variables: { enabled: false }, source: { revision: 'exact' } } }, provision: { old: { params: { x: 1 } } } };
  const after = { driverInputs: { values: { variables: { enabled: true }, source: { revision: 'exact' } } }, provision: {} };
  expect(applyFormChanges(original, before, after)).toEqual({ driverInputs: { values: { variables: { enabled: true, extra: { keep: 0 } }, source: { revision: 'exact', checksum: 'fixed' } } }, provision: {} });
  expect(original.driverInputs.values.variables.enabled).toBe(false);
});
it('keeps data keys safe without changing Object prototypes', () => {
  const original = JSON.parse('{"variables":{"__proto__":{"old":true},"constructor":false}}');
  const before = JSON.parse('{"variables":{"__proto__":{"old":true},"constructor":false}}');
  const after = JSON.parse('{"variables":{"__proto__":{"old":false},"constructor":false}}');
  expect(JSON.parse(JSON.stringify(applyFormChanges(original, before, after)))).toEqual(after);
  expect(({} as Record<string, unknown>).old).toBeUndefined();
});
it('never accepts context identity changes or strips unknown/system fields before validation', () => {
  expect(() => parseEditDocument('key: other\nresourceType: endpoint', 'mine', 'endpoint')).toThrow('identity');
  expect(parseEditDocument('key: mine\nresourceType: endpoint\nversion: 999', 'mine', 'endpoint')).toMatchObject({ version: 999 });
  expect(editDocument({ key: 'mine', version: 2, revisionId: 'hidden' })).not.toHaveProperty('version');
});
