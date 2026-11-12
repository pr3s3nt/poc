import { parseAllDocuments, stringify } from 'yaml';

export const documentLimit = 1024 * 1024;
export type DocumentKind = 'resource-types' | 'resource-definitions';
export class DocumentProblem extends Error {
  constructor(public code: string, public line = 0) { super(code); }
}
export function parseCatalogDocument(text: string): Record<string, unknown> {
  if (new TextEncoder().encode(text).length > documentLimit) throw new DocumentProblem('size');
  const documents = parseAllDocuments(text.replace(/^\uFEFF/, ''), { uniqueKeys: true, strict: true, prettyErrors: false });
  if (documents.length !== 1) throw new DocumentProblem('single');
  const doc = documents[0]!;
  if (doc.errors.length || doc.warnings.length) throw new DocumentProblem('syntax', doc.errors[0]?.linePos?.[0]?.line ?? 0);
  let value: unknown;
  try { value = doc.toJS({ maxAliasCount: 0 }); } catch { throw new DocumentProblem('syntax'); }
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new DocumentProblem('object');
  return value as Record<string, unknown>;
}
const typeFields = ['key', 'inputs', 'outputs'];
const definitionFields = ['key', 'name', 'resourceType', 'driverType', 'driverAccount', 'driverInputs', 'criteria', 'provision'];
export function authoringDocument(kind: DocumentKind, source: object) {
  const values = source as Record<string, unknown>;
  return Object.fromEntries((kind === 'resource-types' ? typeFields : definitionFields).filter(key => values[key] !== undefined).map(key => [key, values[key]]));
}
export function downloadDocument(kind: DocumentKind, source: object, revision?: string) {
  const doc = authoringDocument(kind, source);
  const blob = new Blob([stringify(doc)], { type: 'application/yaml;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url; link.download = `${String(doc.key).replace(/[^a-z0-9-]/g, '-')}${revision ? `-${revision}` : ''}.yaml`;
  link.click(); URL.revokeObjectURL(url);
}
