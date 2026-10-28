import { Button } from '../../shared/ui/Button';
import { Advanced } from './Advanced';
import { ClassControls } from './ClassControls';
import type { ResourceType } from './api';
import { t } from '../../shared/i18n';
import { newSharedId, type SharedResource } from './sharedResources';

// sharedId is the Score resource `id` (UC-06 shared dependency `<type>.<class>#shared.<id>`); className is the
// provisioning class. Existing/imported identities are preserved. New form resources are shared.
export type Dependency = { alias: string; type: string; className: string; sharedId: string; params: Record<string, string>; reuseId?: string; existing?: boolean };
export type ClassMetadata = 'ready' | 'loading' | 'error';
export const emptyDependency = (): Dependency => ({ alias: '', type: '', className: '', sharedId: newSharedId(), params: {} });

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
  choices?: SharedResource[];
  classMetadata: ClassMetadata;
  disabled?: boolean;
  onChange(patch: Partial<Dependency>): void;
  onReload(): void;
  onRemove?(): void;
};

// Resource selection, local alias, provisioning class and typed inputs. The
// workload "Tài nguyên" section and the inline "Thêm tài nguyên" dialog share it, so
// both apply the same inputs and never offer a provider, module or credential.
export function DependencyFields({ index, dependency: dep, types, choices = [], classMetadata, disabled = false, onChange, onReload, onRemove }: Props) {
  const locked = disabled || !!dep.reuseId;
  const type = types.find((item) => item.key === dep.type);
  return <div className="editor-grid">
    <label>{t('workloads.resources.source')}
      <select aria-label={t('workloads.resources.source')} value={dep.reuseId ? `shared:${dep.reuseId}` : dep.existing ? 'current' : ''} disabled={disabled || classMetadata !== 'ready'} onChange={(event) => {
        const selected = choices.find((item) => `shared:${item.id}` === event.target.value)?.dependency;
        if (selected) onChange({ ...selected, alias: dep.alias || selected.alias, reuseId: selected.sharedId, existing: false });
        else onChange({ ...emptyDependency(), alias: dep.alias, reuseId: undefined, existing: false });
      }}>
        {dep.existing && !dep.reuseId ? <option value="current" disabled>{t('workloads.resources.current')}</option> : null}
        <option value="">{t('workloads.resources.createNew')}</option>
        <optgroup label={t('workloads.resources.useExisting')}>
          {dep.reuseId && !choices.some((item) => item.id === dep.reuseId) ? <option value={`shared:${dep.reuseId}`} disabled>{t('workloads.resources.unavailable')}</option> : null}
          {choices.map((item) => <option key={item.id} value={`shared:${item.id}`} disabled={!item.dependency}>{item.label}{!item.dependency ? ` · ${t('workloads.resources.unavailable')}` : ''}</option>)}
        </optgroup>
      </select>
    </label>
    {choices.length === 0 ? <small className="muted">{t('workloads.resources.noExisting')}</small> : null}
    <div className="binding-row">
      <input aria-label={t('workloads.workloadEditor.resourceAlias')} value={dep.alias} disabled={disabled} placeholder="db" onChange={(event) => onChange({ alias: event.target.value })} />
      <select aria-label={t('workloads.workloadEditor.resourceType')} value={dep.type} disabled={locked} onChange={(event) => onChange({ type: event.target.value, params: {} })}><option value="">{t('workloads.workloadEditor.chooseAType')}</option>{types.map((item) => <option key={item.key} value={item.key}>{item.key}</option>)}</select>
      {onRemove ? <Button tone="quiet" onClick={onRemove}>{t('workloads.workloadEditor.remove')}</Button> : null}
    </div>
    <ClassControls index={index} alias={dep.alias} type={type} className={dep.className} metadata={classMetadata} disabled={locked} onClass={(value) => onChange({ className: value })} onReload={onReload} />
    <div className="field-grid">{type?.inputs?.map((input) => <label key={input.name}>{dep.alias || dep.type} · {input.name}{input.required ? ' *' : ''}{input.type === 'bool'
      ? <select aria-label={t('workloads.workloadEditor.parameterName', { name: input.name })} value={paramValue(dep.params, input.name)} disabled={locked} onChange={(event) => onChange({ params: { ...dep.params, [input.name]: event.target.value } })}><option value="">{t('workloads.workloadEditor.choose')}</option><option value="true">{t('workloads.workloadEditor.true')}</option><option value="false">{t('workloads.workloadEditor.false')}</option></select>
      : <input aria-label={t('workloads.workloadEditor.parameterName', { name: input.name })} type={input.type === 'number' ? 'number' : 'text'} value={paramValue(dep.params, input.name)} disabled={locked} onChange={(event) => onChange({ params: { ...dep.params, [input.name]: event.target.value } })} />}</label>)}</div>
    <Advanced label={t('workloads.workloadEditor.resourceValue', { value: dep.alias || index + 1 })} summary={<>{t(dep.sharedId ? 'workloads.resources.shared' : 'workloads.resources.private')} · {t('workloads.workloadEditor.effectiveClassValueValue2', { value: dep.className || 'default', value2: dep.className ? t('workloads.workloadEditor.explicit') : t('workloads.workloadEditor.default') })}</>}>
      {!dep.reuseId ? <label><input type="checkbox" checked={!dep.sharedId} disabled={disabled} onChange={(event) => onChange({ sharedId: event.target.checked ? '' : newSharedId() })} />{t('workloads.resources.privateOnly')}</label> : null}
      <label>{t('workloads.workloadEditor.resourceClass')}<input aria-label={t('workloads.workloadEditor.resourceClass')} value={dep.className} disabled={locked} placeholder="default" onChange={(event) => onChange({ className: event.target.value })} /></label>
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
