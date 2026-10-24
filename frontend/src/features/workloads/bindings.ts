import type { ConfigKey } from '../configuration/api';
import { t } from '../../shared/i18n';

// KEY bindings reference an Application variable/secret by name; the key's kind
// comes from the loaded catalog, so a binding never stores or copies a value.
export type Source = 'KEY' | 'RESOURCE' | 'SERVICE';
export type Binding = { name: string; source: Source; key: string; custom: boolean; alias: string; output: string; workload: string; port: string };
export type KeyCatalog = { status: 'loading' } | { status: 'error'; message: string } | { status: 'ready'; keys: ConfigKey[] };

export const emptyBinding = (): Binding => ({ name: '', source: 'RESOURCE', key: '', custom: false, alias: '', output: '', workload: '', port: '' });
export const keyBinding = (key: string, name = key): Binding => ({ ...emptyBinding(), source: 'KEY', key, name, custom: name !== key });

export const selectKey = (bindings: Binding[], key: string): Binding[] => [...bindings, keyBinding(key)];
// Removes every mapping of the key in this container only; other sources stay.
export const unselectKey = (bindings: Binding[], key: string): Binding[] => bindings.filter((binding) => binding.source !== 'KEY' || binding.key !== key);

// Returns the first problem that would make the container variables ambiguous or
// incomplete. Checked before serialization so no binding is overwritten or dropped.
export function bindingProblem(containers: { name: string; bindings: Binding[] }[], available: ReadonlySet<string>): string | undefined {
  for (const [index, container] of containers.entries()) {
    const label = `Container ${container.name || index + 1}`;
    const seen = new Set<string>();
    for (const binding of container.bindings) {
      if (binding.source === 'KEY') {
        if (!available.has(binding.key)) return t('workloads.bindings.labelApplicationKeyKeyNo', { label, key: binding.key });
        if (!binding.name.trim()) return t('workloads.bindings.labelEnterTheNameIn', { label, key: binding.key });
      } else {
        if (!binding.name.trim()) return t('workloads.bindings.labelEnterTheNameIn2', { label });
        if (binding.source === 'RESOURCE' && (!binding.alias || !binding.output)) return t('workloads.bindings.labelChooseTheResourceAnd', { label, name: binding.name });
        if (binding.source === 'SERVICE' && (!binding.workload || !binding.port)) return t('workloads.bindings.labelChooseTheWorkloadAnd', { label, name: binding.name });
      }
      if (seen.has(binding.name)) return t('workloads.bindings.labelNameNameIsUsed', { label, name: binding.name });
      seen.add(binding.name);
    }
  }
  return undefined;
}
