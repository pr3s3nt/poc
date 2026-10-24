import { Button } from '../../shared/ui/Button';
import { Advanced } from './Advanced';
import { ClassControls } from './ClassControls';
import type { ResourceType } from './api';
import { t } from '../../shared/i18n';

// sharedId is the Score resource `id` (UC-06 shared dependency `<type>.<class>#shared.<id>`); className is the
// provisioning class. Both are kept exactly as chosen or imported: never defaulted, mapped or switched.
export type Dependency = { alias: string; type: string; className: string; sharedId: string; params: Record<string, string> };
export type ClassMetadata = 'ready' | 'loading' | 'error';
export const emptyDependency = (): Dependency => ({ alias: '', type: '', className: '', sharedId: '', params: {} });

// Reads a resource input the user set; an unset name such as __proto__ is empty,
// not an inherited Object member.
export const paramValue = (params: Record<string, string>, name: string): string => (Object.hasOwn(params, name) ? params[name] ?? '' : '');

// Aliases the Score reserves for virtual resources (Application keys and Service references).
export const reservedAlias = (alias: string): boolean => alias === 'env' || alias.startsWith('svc_');

// The first problem of one dependency (type, required and numeric inputs), or undefined.
export function dependencyProblem(dependency: Dependency, types: ResourceType[]): string | undefined {
  const contract = types.find((item) => item.key === dependency.type);
  if (!contract) return t('workloads.workloadEditor.chooseAValidResourceType', { value: dependency.alias || t('workloads.workloadEditor.resource') });
  for (const input of contract.inputs ?? []) {
    const value = paramValue(dependency.params, input.name);
    if (input.required && !value.trim()) return t('workloads.workloadEditor.valueNameIsRequired', { value: dependency.alias || dependency.type, name: input.name });
    if (value && input.type === 'number' && !Number.isFinite(Number(value))) return t('workloads.workloadEditor.valueNameMustBeA', { value: dependency.alias || dependency.type, name: input.name });
  }
  return undefined;
}

type Props = {
  index: number;
  dependency: Dependency;
  types: ResourceType[];
  classMetadata: ClassMetadata;
  disabled?: boolean;
  onChange(patch: Partial<Dependency>): void;
  onReload(): void;
  onRemove?(): void;
};

// Alias, Type, provisioning class, shared ID and typed inputs of one dependency. The
// workload "Tài nguyên" section and the inline "Thêm tài nguyên" dialog share it, so
// both apply the same inputs and never offer a provider, module or credential.
export function DependencyFields({ index, dependency: dep, types, classMetadata, disabled = false, onChange, onReload, onRemove }: Props) {
  const type = types.find((item) => item.key === dep.type);
  return <div className="editor-grid">
    <div className="binding-row">
      <input aria-label={t('workloads.workloadEditor.resourceAlias')} value={dep.alias} disabled={disabled} placeholder="db" onChange={(event) => onChange({ alias: event.target.value })} />
      <select aria-label={t('workloads.workloadEditor.resourceType')} value={dep.type} disabled={disabled} onChange={(event) => onChange({ type: event.target.value, params: {} })}><option value="">{t('workloads.workloadEditor.chooseAType')}</option>{types.map((item) => <option key={item.key} value={item.key}>{item.key}</option>)}</select>
      {onRemove ? <Button tone="quiet" onClick={onRemove}>{t('workloads.workloadEditor.remove')}</Button> : null}
    </div>
    <ClassControls index={index} alias={dep.alias} type={type} className={dep.className} sharedId={dep.sharedId} metadata={classMetadata} disabled={disabled} onClass={(value) => onChange({ className: value })} onSharedId={(value) => onChange({ sharedId: value })} onReload={onReload} />
    <div className="field-grid">{type?.inputs?.map((input) => <label key={input.name}>{dep.alias || dep.type} · {input.name}{input.required ? ' *' : ''}{input.type === 'bool'
      ? <select aria-label={t('workloads.workloadEditor.parameterName', { name: input.name })} value={paramValue(dep.params, input.name)} disabled={disabled} onChange={(event) => onChange({ params: { ...dep.params, [input.name]: event.target.value } })}><option value="">{t('workloads.workloadEditor.choose')}</option><option value="true">{t('workloads.workloadEditor.true')}</option><option value="false">{t('workloads.workloadEditor.false')}</option></select>
      : <input aria-label={t('workloads.workloadEditor.parameterName', { name: input.name })} type={input.type === 'number' ? 'number' : 'text'} value={paramValue(dep.params, input.name)} disabled={disabled} onChange={(event) => onChange({ params: { ...dep.params, [input.name]: event.target.value } })} />}</label>)}</div>
    <Advanced label={t('workloads.workloadEditor.resourceValue', { value: dep.alias || index + 1 })} summary={t('workloads.workloadEditor.effectiveClassValueValue2', { value: dep.className || 'default', value2: dep.className ? t('workloads.workloadEditor.explicit') : t('workloads.workloadEditor.default') })}>
      <label>{t('workloads.workloadEditor.resourceClass')}<input aria-label={t('workloads.workloadEditor.resourceClass')} value={dep.className} disabled={disabled} placeholder="default" onChange={(event) => onChange({ className: event.target.value })} /></label>
      <p>{t('workloads.workloadEditor.emptyUsesTheDefaultClass')}</p>
    </Advanced>
  </div>;
}

// Validate all rows before projecting to a dictionary; never silently overwrite an alias.
export function dependencyAliasesProblem(dependencies: Dependency[], serviceAliases: string[] = []): string | undefined {
  const seen = new Set<string>();
  for (const { alias } of dependencies) {
    if (!/^[A-Za-z0-9_-]+$/.test(alias)) return t('workloads.addResource.aliasInvalid');
    if (alias === 'env' || serviceAliases.includes(alias)) return t('workloads.addResource.aliasReserved', { alias });
    if (seen.has(alias)) return t('workloads.addResource.aliasTaken', { alias });
    seen.add(alias);
  }
  return undefined;
}
