import { inputCompatible, type TypeContract } from './moduleMapping';
import { baseKind, isWholePlaceholder } from './typedVariables';

// Value sources of a Driver Input variable or a companion param (UC-03 UI11-02).
// A source is a view of one placeholder string: the form generates the string
// from explicit choices and parses it back, so the draft keeps a single
// representation and anything the controls cannot model stays verbatim.

export type ResourceScope = { kind: 'environment' } | { kind: 'infra' } | { kind: 'resource'; id: string };
export type ResourceReference = { type: string; class: string; scope: ResourceScope; output: string };
export type ValueSource =
  | { kind: 'literal'; text: string }
  | { kind: 'context'; path: string }
  | { kind: 'resource'; ref: ResourceReference }
  | { kind: 'advanced'; text: string };

// Context values the author may pick (BR-25). Any other `${context.*}` placeholder stays an advanced source.
export const contextChoices = [
  { path: 'env.namespace', label: 'Namespace của môi trường' },
  { path: 'infra.resourceName', label: 'Tên tài nguyên hạ tầng' },
] as const;

export const defaultClass = 'default';
export const environmentScopeId = 'environments.@app.@env';
export const infraScopeId = '@infra';

const contextPattern = /^\$\{context\.([A-Za-z0-9_.]+)\}$/;
const resourcePattern = /^\$\{resources\['([^.#'@]+)\.([^#']+)#([^']+)'\]\.outputs\.([A-Za-z0-9_]+)\}$/;

export const contextPlaceholder = (path: string) => `\${context.${path}}`;
export const scopeId = (scope: ResourceScope): string => (scope.kind === 'environment' ? environmentScopeId : scope.kind === 'infra' ? infraScopeId : scope.id);
export const descriptorOf = (ref: Pick<ResourceReference, 'type' | 'class' | 'scope'>) => `${ref.type}.${ref.class}#${scopeId(ref.scope)}`;
export const referencePlaceholder = (ref: ResourceReference) => `\${resources['${descriptorOf(ref)}'].outputs.${ref.output}}`;

export function scopeFromId(id: string): ResourceScope {
  if (id === environmentScopeId) return { kind: 'environment' };
  if (id === infraScopeId) return { kind: 'infra' };
  return { kind: 'resource', id };
}

// Descriptor `type.class#scope` as written by the form; anything else (missing class/ID, selectors) is not modelled.
export function parseDescriptor(text: string): { type: string; class: string; scope: ResourceScope } | undefined {
  const [, type, className, id] = /^([^.#'@\s]+)\.([^#'\s]+)#(\S+)$/.exec(text) ?? [];
  if (type === undefined || className === undefined || id === undefined || id.includes("'")) return undefined;
  return { type, class: className, scope: scopeFromId(id) };
}

export function parseSource(raw: string): ValueSource {
  const [, path] = contextPattern.exec(raw) ?? [];
  if (path !== undefined && contextChoices.some((choice) => choice.path === path)) return { kind: 'context', path };
  const [, type, className, id, output] = resourcePattern.exec(raw) ?? [];
  if (type !== undefined && className !== undefined && id !== undefined && output !== undefined) return { kind: 'resource', ref: { type, class: className, scope: scopeFromId(id), output } };
  return isWholePlaceholder(raw) ? { kind: 'advanced', text: raw } : { kind: 'literal', text: raw };
}

// Resource Types an author can reference: workloads are not a value provider.
export const referenceTypes = (types: TypeContract[]): TypeContract[] => types.filter((type) => type.key !== 'workload');

// Public, non-secret outputs whose declared type can fill the target (Resource Type contract types are flat).
export function compatibleOutputs(type: TypeContract | undefined, targetType: string): TypeContract['outputs'] {
  return (type?.outputs ?? []).filter((output) => !output.secret && inputCompatible(output.type, targetType));
}

export const classChoices = (type: TypeContract | undefined, current = ''): string[] => {
  const classes = new Set([defaultClass, ...(type?.classes ?? [])]);
  if (current !== '') classes.add(current);
  return [...classes];
};

export const contextAllowed = (targetType: string) => baseKind(targetType) === 'string' || baseKind(targetType) === 'any';

export const scopeLabel = (scope: ResourceScope): string => (scope.kind === 'environment' ? 'Cùng ứng dụng và môi trường' : scope.kind === 'infra' ? 'Ngữ cảnh hạ tầng' : `Tài nguyên ${scope.id}`);
export const outputLabel = (output: string) => output;

// Human summary of a source for a parameter card.
export function describeSource(source: ValueSource, types: TypeContract[]): { origin: string; value: string } {
  switch (source.kind) {
    case 'literal': return { origin: 'Nhập trực tiếp', value: source.text === '' ? 'Chuỗi rỗng' : source.text };
    case 'context': return { origin: 'Từ ngữ cảnh triển khai', value: contextChoices.find((choice) => choice.path === source.path)?.label ?? source.path };
    case 'resource': {
      const known = types.some((type) => type.key === source.ref.type && type.outputs.some((output) => output.name === source.ref.output));
      return { origin: known ? 'Từ tài nguyên khác' : 'Từ tài nguyên khác · chưa có trong metadata', value: `${source.ref.type} → ${outputLabel(source.ref.output)} · ${source.ref.class} · ${scopeLabel(source.ref.scope)}` };
    }
    default: return { origin: 'Biểu thức nâng cao', value: source.text };
  }
}
