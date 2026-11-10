// Class dropdown rules shared by the Workload editor and the Platform Engineer's
// Definition form (UC-16 BR-33, issue #61). GET /resource-types publishes the
// named `classes` of a Type and whether the class `default` can be served
// (`defaultAvailable`). A dropdown offers only what can be served.

export type ClassMeta = { classes?: string[]; defaultAvailable?: boolean };

// `defaultValue` is how the field writes the default class: '' where an omitted
// class means default (Workload metadata.class, a dependency class) and
// 'default' inside a descriptor `type.default#scope`.
export function offeredClasses(meta: ClassMeta | undefined, defaultValue: string): string[] {
  const published = meta?.classes ?? [];
  const named = [...new Set(published)].filter((name) => name !== '' && name !== 'default').sort();
  const servesDefault = meta?.defaultAvailable === true || published.includes('default');
  // A Type with nothing published keeps the default so existing drafts stay as they were.
  const nothingPublished = named.length === 0 && !servesDefault;
  return [...(servesDefault || nothingPublished ? [defaultValue] : []), ...named];
}

// The choice a new value takes when exactly one is offered.
export function autoClass(meta: ClassMeta | undefined, defaultValue: string): string | undefined {
  const offered = offeredClasses(meta, defaultValue);
  return offered.length === 1 ? offered[0] : undefined;
}

// A new value still left at the default although the default is not offered:
// the user must pick one of the offered classes.
export function classChoiceMissing(meta: ClassMeta | undefined, defaultValue: string, value: string): boolean {
  return value === defaultValue && !offeredClasses(meta, defaultValue).includes(defaultValue);
}
