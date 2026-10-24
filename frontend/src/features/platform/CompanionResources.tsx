import { Button } from '../../shared/ui/Button';
import type { TypeContract } from './moduleMapping';
import { SourceCard } from './ParameterSources';
import { newProvisionRow, type ProvisionRow } from './typedProvision';
import { entryFor, entryValue, parseObjectText, type JsonObject } from './typedVariables';
import { classChoices, defaultClass, descriptorOf, environmentScopeId, infraScopeId, parseDescriptor, referenceTypes, scopeFromId, type ResourceScope } from './valueSource';
import { t } from '../../shared/i18n';

// Companion resources of a Definition (UC-03 UI11-04 / BR-26 / UC-06 BR-08). A card picks Resource Type, class and scope
// and builds the params from the Resource Type input schema; both are generated into the same descriptor/params text the
// row keeps, so the technical fields and the JSON mode stay lossless and authoritative for anything the card cannot show.

type Props = {
  rows: ProvisionRow[];
  onChange: (rows: ProvisionRow[]) => void;
  parentType: string;
  types: TypeContract[];
  disabled?: boolean;
};

const variableOf = (name: string, type: string) => ({ name, type, required: false, hasDefault: false, resourceParam: false });

// Params the typed controls can hold: a JSON object whose keys are inputs of the Type and whose values the chooser can show.
function paramsView(row: ProvisionRow, type: TypeContract | undefined): { values?: Record<string, string>; reason?: string } {
  if (!type) return { reason: t('platform.companionResources.chooseAResourceTypeWith') };
  const parsed = parseObjectText(row.params, 'Params');
  if (!parsed.value) return { reason: t('platform.companionResources.valueEditItInTechnical', { value: parsed.error ?? '' }) };
  const values: Record<string, string> = {};
  for (const [name, value] of Object.entries(parsed.value)) {
    const input = type.inputs.find((item) => item.name === name);
    if (!input) return { reason: t('platform.companionResources.paramNameIsNotPart', { name, key: type.key }) };
    const entry = entryFor({ ...variableOf(name, input.type) }, value);
    if (!entry) return { reason: t('platform.companionResources.theValueOfNameCannot', { name }) };
    values[name] = entry.raw;
  }
  return { values };
}

export function CompanionResources({ rows, onChange, parentType, types, disabled = false }: Props) {
  const patch = (id: number, change: Partial<ProvisionRow>) => onChange(rows.map((row) => (row.id === id ? { ...row, ...change } : row)));
  const parentName = parentType || t('platform.companionResources.resourceBeingConfigured');
  const setDescriptor = (row: ProvisionRow, next: { type: string; class: string; scope: ResourceScope }) => patch(row.id, { descriptor: descriptorOf(next) });
  const setParam = (row: ProvisionRow, type: TypeContract, name: string, raw: string | undefined) => {
    const parsed = parseObjectText(row.params, 'Params').value ?? {};
    const next: JsonObject = { ...parsed };
    if (raw === undefined) delete next[name];
    else {
      const input = type.inputs.find((item) => item.name === name);
      const value = entryValue(variableOf(name, input?.type ?? 'any'), { on: true, raw });
      if (value.error) return;
      next[name] = value.value;
    }
    patch(row.id, { params: JSON.stringify(next, null, 2), keepParams: raw === undefined ? row.keepParams : true });
  };

  return <section className="catalog-fields companion-section" aria-label={t('platform.companionResources.companionResources')}>
    <div className="section-header"><div><h4>{t('platform.companionResources.companionResources')}</h4><p className="feature-note">{t('platform.companionResources.createAdditionalResourcesWhenThis')}</p></div></div>
    <div className="companion-parent"><span className="companion-icon" aria-hidden="true">▣</span><div><small>{t('platform.companionResources.rESOURCEBEINGCONFIGURED')}</small><strong>{parentName}</strong></div></div>
    {rows.length === 0 ? <p className="muted">{t('platform.companionResources.noCompanionResourcesYet')}</p> : null}
    {rows.map((row, index) => {
      const number = index + 1;
      const descriptor = parseDescriptor(row.descriptor);
      const modelled = row.descriptor.trim() === '' || descriptor !== undefined;
      const type = descriptor ? types.find((item) => item.key === descriptor.type) : undefined;
      const params = descriptor ? paramsView(row, type) : { reason: '' };
      const companionName = descriptor?.type || t('platform.companionResources.companionResourceNumber', { number });
      return <div key={row.id} className="companion-card provision-row" data-companion={number}>
        <div className="companion-head"><span className="companion-icon" aria-hidden="true">+</span><div><strong>{companionName}</strong><small className="muted">{row.descriptor || t('platform.companionResources.noResourceTypeSelected')}</small></div>
          <Button type="button" tone="quiet" aria-label={t('platform.companionResources.removeCompanionResourceNumber', { number })} disabled={disabled} onClick={() => onChange(rows.filter((item) => item.id !== row.id))}>{t('platform.companionResources.remove')}</Button></div>
        {modelled ? <div className="field-grid">
          <label>{t('platform.companionResources.resourceType')}<select aria-label={t('platform.companionResources.companionResourceTypeNumber', { number })} value={descriptor?.type ?? ''} disabled={disabled} onChange={(event) => setDescriptor(row, { type: event.target.value, class: descriptor?.class ?? defaultClass, scope: descriptor?.scope ?? { kind: 'environment' } })}>
            <option value="">{t('platform.companionResources.chooseAResourceType')}</option>
            {descriptor && !types.some((item) => item.key === descriptor.type) ? <option value={descriptor.type}>{t('platform.companionResources.typeNoLongerInMetadata', { type: descriptor.type })}</option> : null}
            {referenceTypes(types).map((item) => <option key={item.key} value={item.key}>{item.key}</option>)}</select></label>
          <label>{t('platform.companionResources.class')}<select aria-label={t('platform.companionResources.companionResourceClassNumber', { number })} value={descriptor?.class ?? defaultClass} disabled={disabled || !descriptor} onChange={(event) => descriptor ? setDescriptor(row, { ...descriptor, class: event.target.value }) : undefined}>
            {classChoices(type, descriptor?.class ?? '').map((item) => <option key={item} value={item}>{item === defaultClass ? t('platform.companionResources.defaultDefault') : item}</option>)}</select></label>
          <label>{t('platform.companionResources.scope')}<select aria-label={t('platform.companionResources.companionResourceScopeNumber', { number })} value={descriptor ? (descriptor.scope.kind === 'resource' ? 'resource' : descriptor.scope.kind === 'infra' ? infraScopeId : environmentScopeId) : environmentScopeId} disabled={disabled || !descriptor} onChange={(event) => descriptor ? setDescriptor(row, { ...descriptor, scope: event.target.value === 'resource' ? { kind: 'resource', id: '' } : scopeFromId(event.target.value) }) : undefined}>
            <option value={environmentScopeId}>{t('platform.companionResources.sameApplicationAndEnvironment')}</option><option value={infraScopeId}>{t('platform.companionResources.infrastructureContext')}</option><option value="resource">{t('platform.companionResources.specificResourceID')}</option></select></label>
          {descriptor?.scope.kind === 'resource' ? <label>{t('platform.companionResources.resourceID')}<input aria-label={t('platform.companionResources.companionResourceIDNumber', { number })} value={descriptor.scope.id} spellCheck={false} disabled={disabled} onChange={(event) => setDescriptor(row, { ...descriptor, scope: { kind: 'resource', id: event.target.value } })} /></label> : null}
        </div> : <div className="form-warning" role="note">{t('platform.companionResources.thisDescriptorCannotBeRepresented')}</div>}
        {descriptor && type ? <div className="companion-params" aria-label={t('platform.companionResources.paramsOfCompanionName', { companionName })}>
          {params.values ? (type.inputs.length === 0 ? <p className="feature-note">{t('platform.companionResources.thisResourceTypeDeclaresNo')}</p> : type.inputs.map((input) => <SourceCard key={input.name} name={input.name} type={input.type} required={input.required === true} raw={params.values?.[input.name]}
            inherited={{ origin: t('platform.companionResources.notProvided'), value: input.required ? t('platform.companionResources.noValueSentTheParameter') : t('platform.companionResources.optionalLeaveEmptyToSend'), choice: { label: t('platform.companionResources.doNotProvide'), description: t('platform.companionResources.thisParamIsNotSent') } }}
            types={types} disabled={disabled} onChange={(raw) => setParam(row, type, input.name, raw)} />)) : <div className="form-warning" role="note">{params.reason}</div>}
        </div> : null}
        <div className="companion-flags"><h5>{t('platform.companionResources.dependencies')}</h5>
          <label className="catalog-check companion-flag"><input type="checkbox" aria-label={t('platform.companionResources.companionResourceNumberCompanionNameWaits', { number, companionName, parentName })} checked={row.isDependent} disabled={disabled} onChange={(event) => patch(row.id, { isDependent: event.target.checked })} /> <span><strong>{companionName}</strong> {' '}{t('platform.companionResources.waitsFor')}{' '} <strong>{parentName}</strong> {' '}{t('platform.companionResources.toFinish')}</span></label>
          <label className="catalog-check companion-flag"><input type="checkbox" aria-label={t('platform.companionResources.companionResourceNumberResourcesUsing', { number, parentName, companionName })} checked={row.matchDependents} disabled={disabled} onChange={(event) => patch(row.id, { matchDependents: event.target.checked })} /> <span>{t('platform.companionResources.resourcesUsing')}{' '} <strong>{parentName}</strong> {' '}{t('platform.companionResources.mustAlsoWaitFor')}{' '} <strong>{companionName}</strong></span></label>
          <p className="feature-note">{t('platform.companionResources.aDisabledFlagAddsNo')}</p></div>
        <details className="companion-technical"><summary>{t('platform.companionResources.technicalConfiguration')}</summary>
          <label>Descriptor {number}<input aria-label={t('platform.companionResources.companionResourceDescriptorNumber', { number })} value={row.descriptor} spellCheck={false} disabled={disabled} placeholder={t('platform.companionResources.typeClassScopeForExample')} onChange={(event) => patch(row.id, { descriptor: event.target.value })} /></label>
          <label>Params {number} (JSON object)<textarea aria-label={t('platform.companionResources.companionResourceParamsNumberJSON', { number })} rows={3} value={row.params} spellCheck={false} disabled={disabled} onChange={(event) => patch(row.id, { params: event.target.value })} /></label></details>
      </div>;
    })}
    <Button type="button" className="companion-add" disabled={disabled} onClick={() => onChange([...rows, newProvisionRow()])}>{t('platform.companionResources.addCompanionResource')}</Button>
  </section>;
}
