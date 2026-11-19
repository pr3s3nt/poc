import { authoringDocument, parseCatalogDocument } from './catalogDocument';

export type DefinitionDocument = Record<string, unknown>;

export function editDocument(value: object): DefinitionDocument {
  const doc = authoringDocument('resource-definitions', value);
  return { ...doc, driverInputs: doc.driverInputs ?? { values: { variables: {} } }, provision: doc.provision ?? {}, criteria: doc.criteria ?? [] };
}

export function sameDocument(a: unknown, b: unknown): boolean {
  if (Object.is(a, b)) return true;
  if (Array.isArray(a) && Array.isArray(b)) return a.length === b.length && a.every((item, i) => sameDocument(item, b[i]));
  if (!record(a) || !record(b)) return false;
  const keys = Object.keys(a);
  return keys.length === Object.keys(b).length && keys.every(key => Object.hasOwn(b, key) && sameDocument(a[key], b[key]));
}
function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

// Apply only changes made by controls. Fields outside the form's projection
// stay in the common draft rather than disappearing on serialization or Save.
export function applyFormChanges(original: unknown, before: unknown, after: unknown): unknown {
  if (sameDocument(before, after)) return original;
  if (!record(before) || !record(after)) return after;
  const result: Record<string, unknown> = Object.assign(Object.create(null) as Record<string, unknown>, record(original) ? original : {});
  for (const key of new Set([...Object.keys(before), ...Object.keys(after)])) {
    if (!Object.hasOwn(after, key)) delete result[key];
    else if (!Object.hasOwn(before, key)) result[key] = after[key];
    else result[key] = applyFormChanges(result[key], before[key], after[key]);
  }
  return result;
}

export function parseEditDocument(text: string, key: string, resourceType: string): DefinitionDocument {
  const doc = parseCatalogDocument(text);
  if (doc.key !== key || doc.resourceType !== resourceType) throw new Error('identity');
  // Do not project/filter code here: the strict server DTO must reject unknown
  // and system fields instead of silently stripping them.
  return doc;
}
