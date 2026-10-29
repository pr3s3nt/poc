// Strict readers of the Platform metadata the registration forms depend on. A response that is
// missing a required array, or has a malformed member or an incomplete module pin, is a metadata
// failure (retryable, nothing is registered from it); it never becomes empty or "ready" metadata.
// The error carries no part of the payload.
export class MetadataError extends Error {
  constructor() { super('metadata is unavailable'); }
}

type Obj = Record<string, unknown>;
export const isObject = (value: unknown): value is Obj => typeof value === 'object' && value !== null && !Array.isArray(value);
export const text = (value: unknown): value is string => typeof value === 'string';
export const filled = (value: unknown): value is string => typeof value === 'string' && value !== '';
export const flag = (value: unknown): value is boolean => typeof value === 'boolean';
export const fail = (): never => { throw new MetadataError(); };
// A required array must be present; `optional` arrays may be absent (the server omits them when empty).
function array(value: unknown, optional = false): unknown[] {
  if (value === undefined && optional) return [];
  return Array.isArray(value) ? value : fail();
}
export const members = <T>(value: unknown, read: (item: unknown) => T, optional = false): T[] => array(value, optional).map(read);

export type TypeField = { name: string; type: string; required?: boolean; secret?: boolean };
export type TypeMetadata = { key: string; inputs: TypeField[]; outputs: TypeField[]; classes: string[] };

function field(item: unknown): TypeField {
  if (!isObject(item) || !filled(item.name) || !text(item.type)) return fail();
  if ((item.required !== undefined && !flag(item.required)) || (item.secret !== undefined && !flag(item.secret))) return fail();
  return { name: item.name, type: item.type, ...(item.required !== undefined ? { required: item.required } : {}), ...(item.secret !== undefined ? { secret: item.secret } : {}) };
}

// GET /resource-types for the Definition form: the contract (null schema arrays are the server's empty
// contract) and the computed `classes` (a required array; empty is valid).
export function parseTypeMetadata(body: unknown): TypeMetadata[] {
  if (!isObject(body)) return fail();
  return members(body.resourceTypes, (item) => {
    if (!isObject(item) || !filled(item.key)) return fail();
    const schema = (value: unknown) => (value === null ? [] : members(value, field));
    const classes = members(item.classes, (entry) => (text(entry) ? entry : fail()));
    return { key: item.key, inputs: schema(item.inputs), outputs: schema(item.outputs), classes };
  });
}

// A Git render bundle (ADR-021) adds its verified source pin and input schema; an installed
// bundle keeps the legacy id/version/status shape. Both shapes are accepted.
export type RenderSource = { kind: 'git' | 'zip'; url: string; path: string; revision: string; contentSha256: string };
export type RenderBundle = { id: string; version: string; status: string; name?: string; source?: RenderSource; inputsSchema?: Record<string, unknown>; engineContract?: string; inputContract?: string; renderer?: { type: string; version: string } };

export function parseRenderBundle(item: unknown): RenderBundle {
  if (!isObject(item) || !filled(item.id) || !text(item.version) || !text(item.status)) return fail();
  const base: RenderBundle = { id: item.id, version: item.version, status: item.status };
  if (item.source === undefined) return base;
  const source = item.source;
  if (!isObject(source) || (source.kind !== 'git' && source.kind !== 'zip') || !filled(source.url) || !filled(source.path) || !filled(source.revision) || !filled(source.contentSha256)) return fail();
  if (!filled(item.name) || !isObject(item.inputsSchema) || !filled(item.engineContract) || !filled(item.inputContract)) return fail();
  const renderer = item.renderer;
  if (!isObject(renderer) || !filled(renderer.type) || !filled(renderer.version)) return fail();
  return { ...base, name: item.name, source: { kind: source.kind, url: source.url, path: source.path, revision: source.revision, contentSha256: source.contentSha256 }, inputsSchema: item.inputsSchema, engineContract: item.engineContract, inputContract: item.inputContract, renderer: { type: renderer.type, version: renderer.version } };
}

export function parseRenderBundles(body: unknown): RenderBundle[] {
  if (!isObject(body)) return fail();
  return members(body.renderBundles, parseRenderBundle);
}
