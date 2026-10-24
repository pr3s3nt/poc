import type { ConfigKey, KeyKind } from '../configuration/api';
import { t } from '../../shared/i18n';

// One row of the per-container "Biến môi trường" table (UC-16 BR-15). The source
// is frontend state only: it is projected onto Score on Save and never sent as a field.
//   LITERAL  ordinary string kept with the workload configuration
//   VARIABLE application key of kind VARIABLE (UC-12)
//   SECRET   application key of kind SECRET (UC-12); only the reference is kept
//   KEY      an imported `${resources.env.KEY}` whose kind is resolved from the catalog
//   RESOURCE output of a declared dependency
//   SERVICE  Service port of another workload of the same Environment
export type Source = 'LITERAL' | 'VARIABLE' | 'SECRET' | 'KEY' | 'RESOURCE' | 'SERVICE';
// id is a stable row identity: late replies and focus never depend on the row position.
// value is the literal as the user sees it (not escaped); alias is the Service resource alias kept from an import.
export type Binding = { id: string; name: string; source: Source; value: string; key: string; alias: string; output: string; workload: string; port: string };
export type KeyCatalog = { status: 'loading' } | { status: 'error'; message: string } | { status: 'ready'; keys: ConfigKey[]; version: number; secretStoreKey?: string };

let sequence = 0;
export const newRowId = (): string => `row-${++sequence}`;

export const emptyBinding = (source: Source = 'LITERAL'): Binding => ({ id: newRowId(), name: '', source, value: '', key: '', alias: '', output: '', workload: '', port: '' });

// Changing the source keeps the name and identity and drops every payload of the old source,
// so a Secret reference never turns into a literal and nothing stale is serialized.
export const changeSource = (binding: Binding, source: Source): Binding => ({ ...emptyBinding(source), id: binding.id, name: binding.name });

export const REFERENCE = /^\$\{resources\.([A-Za-z0-9_-]+)\.([A-Za-z0-9_-]+)\}$/;
// The Score expander resolves ${...} and turns $${...} into a literal ${...}.
const PLACEHOLDER = /\$\$?\{[^}]*\}/g;

export function hasUnescapedPlaceholder(value: string): boolean {
  return (value.match(PLACEHOLDER) ?? []).some((match) => !match.startsWith('$$'));
}

// Form text is never interpreted as a placeholder: every ${...} (and $${...}) gets one more $,
// which the Score expander removes again. escape/unescape are inverse for any text.
export const escapeLiteral = (text: string): string => text.replace(PLACEHOLDER, (match) => `$${match}`);
export const unescapeLiteral = (value: string): string => value.replace(PLACEHOLDER, (match) => (match.startsWith('$$') ? match.slice(1) : match));

export type ResourceSpecs = Record<string, { type?: string; params?: Record<string, unknown> } | undefined>;

// Reads one Score variable. It returns undefined for a value the table cannot hold
// without changing it (text mixed with a reference, a malformed reference): such a
// Score keeps the advanced Score path instead of being rewritten.
export function readBinding(name: string, raw: unknown, resources: ResourceSpecs): Binding | undefined {
  if (typeof raw !== 'string') return undefined;
  const base = { ...emptyBinding(), name };
  const match = raw.match(REFERENCE);
  if (match) {
    const alias = match[1] ?? '';
    const output = match[2] ?? '';
    if (alias === 'env') return { ...base, source: 'KEY', key: output };
    const spec = resources[alias];
    if (spec?.type === 'service') return { ...base, source: 'SERVICE', alias, workload: String(spec.params?.workload ?? ''), port: String(spec.params?.port ?? '') };
    return { ...base, source: 'RESOURCE', alias, output };
  }
  if (hasUnescapedPlaceholder(raw)) return undefined;
  return { ...base, source: 'LITERAL', value: unescapeLiteral(raw) };
}

export const serviceAlias = (workload: string, port: string): string => `svc_${workload}_${port}`.replace(/[^A-Za-z0-9_-]/g, '_');

// The effective source: an imported key takes the kind the catalog reports.
export function sourceOf(binding: Binding, catalog: KeyCatalog): Exclude<Source, 'KEY'> {
  if (binding.source !== 'KEY') return binding.source;
  if (catalog.status !== 'ready') return 'VARIABLE';
  return catalog.keys.find((key) => key.name === binding.key)?.kind === 'SECRET' ? 'SECRET' : 'VARIABLE';
}

export const isKeySource = (source: Source): boolean => source === 'VARIABLE' || source === 'SECRET' || source === 'KEY';
export const keyKind = (source: Exclude<Source, 'KEY'>): KeyKind | undefined => (source === 'VARIABLE' ? 'VARIABLE' : source === 'SECRET' ? 'SECRET' : undefined);

export type RowContext = {
  catalog: KeyCatalog;
  dependencies: { alias: string; type: string }[];
  outputs: (alias: string) => string[] | undefined;
  services: { id: string; ports: string[] }[];
};

// A row problem: incomplete means "not filled in yet", which the table only reports once Save was tried;
// every other problem (duplicate, missing or invalid target) is shown as soon as it exists.
export type RowProblem = { message: string; incomplete: boolean };
const incomplete = (message: string): RowProblem => ({ message, incomplete: true });
const invalid = (message: string): RowProblem => ({ message, incomplete: false });

// Problem of one row, or undefined. Names are kept exactly as typed (no trim, rename or suffix).
export function rowProblem(binding: Binding, row: number, container: string, bindings: Binding[], context: RowContext): RowProblem | undefined {
  const where = { container, row };
  if (!binding.name.trim()) return incomplete(t('workloads.envTable.problemName', where));
  if (bindings.some((other) => other.id !== binding.id && other.name === binding.name)) return invalid(t('workloads.envTable.problemDuplicate', { ...where, name: binding.name }));
  const source = sourceOf(binding, context.catalog);
  switch (source) {
    case 'LITERAL':
      return binding.value.includes('\u0000') ? invalid(t('workloads.envTable.problemNul', { ...where, name: binding.name })) : undefined;
    case 'VARIABLE':
    case 'SECRET': {
      if (!binding.key) return incomplete(t('workloads.envTable.problemKey', { ...where, name: binding.name }));
      if (context.catalog.status !== 'ready') return undefined;
      const found = context.catalog.keys.find((key) => key.name === binding.key);
      if (!found) return invalid(t('workloads.envTable.problemKeyMissing', { ...where, name: binding.name, key: binding.key }));
      if (found.kind !== keyKind(source)) return invalid(t('workloads.envTable.problemKeyKind', { ...where, name: binding.name, key: binding.key }));
      return undefined;
    }
    case 'RESOURCE': {
      if (!binding.alias || !binding.output) return incomplete(t('workloads.envTable.problemResource', { ...where, name: binding.name }));
      if (!context.dependencies.some((dependency) => dependency.alias === binding.alias)) return invalid(t('workloads.envTable.problemAliasMissing', { ...where, name: binding.name, alias: binding.alias }));
      const outputs = context.outputs(binding.alias);
      if (outputs && !outputs.includes(binding.output)) return invalid(t('workloads.envTable.problemOutputMissing', { ...where, name: binding.name, alias: binding.alias, output: binding.output }));
      return undefined;
    }
    case 'SERVICE': {
      if (!binding.workload || !binding.port) return incomplete(t('workloads.envTable.problemService', { ...where, name: binding.name }));
      const target = context.services.find((service) => service.id === binding.workload);
      if (!target) return invalid(t('workloads.envTable.problemWorkloadMissing', { ...where, name: binding.name, workload: binding.workload }));
      if (!target.ports.includes(binding.port)) return invalid(t('workloads.envTable.problemPortMissing', { ...where, name: binding.name, workload: binding.workload, port: binding.port }));
      return undefined;
    }
  }
}

export type ContainerBindings = { name: string; bindings: Binding[] };

// The first problem of any container; checked before serialization so nothing is overwritten or dropped.
export function bindingProblem(containers: ContainerBindings[], context: RowContext): string | undefined {
  for (const [ci, container] of containers.entries()) {
    const label = container.name || String(ci + 1);
    for (const [index, binding] of container.bindings.entries()) {
      const problem = rowProblem(binding, index + 1, label, container.bindings, context);
      if (problem) return problem.message;
    }
  }
  return undefined;
}
