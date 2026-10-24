import { Button } from '../../shared/ui/Button';
import { newProvisionRow, type ProvisionRow } from './typedProvision';
import { t } from '../../shared/i18n';

type Props = { rows: ProvisionRow[]; onChange: (rows: ProvisionRow[]) => void; disabled?: boolean };

// Provision editor (UC-03 BR-26): descriptor, params and the two flags of
// UC-06 BR-08. Editing never previews or provisions; reference edges stay
// consumer -> provider as the planner builds them.
export function ProvisionEditor({ rows, onChange, disabled = false }: Props) {
  const patch = (id: number, change: Partial<ProvisionRow>) => onChange(rows.map((row) => (row.id === id ? { ...row, ...change } : row)));
  return <section className="catalog-fields" aria-label={t('platform.provisionEditor.companionResources')}>
    <div className="section-header"><div><h4>{t('platform.provisionEditor.companionResources')}</h4>
      <details className="definition-help"><summary>{t('platform.provisionEditor.aboutDependencies')}</summary><p className="feature-note">{t('platform.provisionEditor.eachRuleDescribesAResource')}</p></details></div>
      <Button type="button" disabled={disabled} onClick={() => onChange([...rows, newProvisionRow()])}>{t('platform.provisionEditor.addCompanionResource')}</Button></div>
    {rows.length === 0 ? <p className="muted">{t('platform.provisionEditor.noCompanionResourcesYet')}</p> : null}
    {rows.map((row, index) => <div key={row.id} className="provision-row">
      <label>Descriptor {index + 1}<input aria-label={t('platform.provisionEditor.companionResourceDescriptorValue', { value: index + 1 })} value={row.descriptor} spellCheck={false} disabled={disabled} placeholder={t('platform.provisionEditor.typeClassIdForExample')} onChange={(event) => patch(row.id, { descriptor: event.target.value })} /></label>
      <label>Params {index + 1} (JSON object)<textarea aria-label={t('platform.provisionEditor.companionResourceParamsValueJSON', { value: index + 1 })} rows={3} value={row.params} spellCheck={false} disabled={disabled} onChange={(event) => patch(row.id, { params: event.target.value })} /></label>
      <label className="catalog-check"><input type="checkbox" aria-label={t('platform.provisionEditor.companionResourceValueDependsOn', { value: index + 1 })} checked={row.isDependent} disabled={disabled} onChange={(event) => patch(row.id, { isDependent: event.target.checked })} /> {' '}{t('platform.provisionEditor.dependsOnTheParentResource')}</label>
      <details><summary>{t('platform.provisionEditor.isDependentDetails')}</summary><p className="feature-note">{t('platform.provisionEditor.reversesRolesTheCompanionResource')}</p></details>
      <label className="catalog-check"><input type="checkbox" aria-label={t('platform.provisionEditor.connectConsumersOfTheParent', { value: index + 1 })} checked={row.matchDependents} disabled={disabled} onChange={(event) => patch(row.id, { matchDependents: event.target.checked })} /> {' '}{t('platform.provisionEditor.connectConsumersOfTheParent2')}</label>
      <details><summary>{t('platform.provisionEditor.matchDependentsDetails')}</summary><p className="feature-note">{t('platform.provisionEditor.connectsEveryConsumerUsingThe')}</p></details>
      <Button type="button" tone="quiet" aria-label={t('platform.provisionEditor.removeCompanionResourceValue', { value: index + 1 })} disabled={disabled} onClick={() => onChange(rows.filter((item) => item.id !== row.id))}>{t('platform.provisionEditor.remove')}</Button>
    </div>)}
  </section>;
}
