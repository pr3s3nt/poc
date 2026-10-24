import { Button } from '../../shared/ui/Button';
import type { Criterion, CriteriaDraft, CriterionField } from './CriteriaEditor';
import { t } from '../../shared/i18n';

export const criterionFields: { key: CriterionField; label: string }[] = [
  { key: 'env_type', label: t('platform.criteriaRules.environmentType') }, { key: 'app_id', label: t('platform.criteriaRules.applicationID') },
  { key: 'env_id', label: t('platform.criteriaRules.environmentID') }, { key: 'res_id', label: t('platform.criteriaRules.resourceID') }, { key: 'class', label: 'Class' },
];
// Keep selected-but-empty conditions in the owner's draft. They are incomplete,
// whereas an explicitly empty case is a wildcard; neither tab changes nor copy
// confirmations may silently turn one into the other.
export const selectedCriterionFields = (draft: CriteriaDraft): CriterionField[][] => draft.fields ?? draft.rows.map((row) => criterionFields.filter(({ key }) => row[key] !== '').map(({ key }) => key));

export function CriteriaRules({ draft, onChange }: { draft: CriteriaDraft; onChange(next: CriteriaDraft): void }) {
  const selected = selectedCriterionFields(draft);
  const update = (rows: CriteriaDraft['rows'], fields: CriterionField[][]) => onChange({ mode: 'advanced', rows, fields });
  return <div className="criteria-cases">
    <p className="feature-note criteria-logic">{t('platform.criteriaRules.allConditionsInACase')}</p>
    {draft.rows.map((row, index) => <section className="criteria-case" key={index} aria-label={t('platform.criteriaRules.caseValue', { value: index + 1 })}>
      <div className="section-header"><h4>{t('platform.criteriaRules.caseValue', { value: index + 1 })}</h4><Button type="button" tone="quiet" aria-label={t('platform.criteriaRules.removeCaseValue', { value: index + 1 })} onClick={() => update(draft.rows.filter((_, i) => i !== index), selected.filter((_, i) => i !== index))}>{t('platform.criteriaRules.removeCase')}</Button></div>
      <p className="criteria-case-hint">{t('platform.criteriaRules.appliesWhenAllOfThe')}</p>
      {(selected[index] ?? []).length === 0 ? <p className="feature-note">{t('platform.criteriaRules.everywhereNoLimitWildcardScore')}</p> : null}
      {(selected[index] ?? []).map((field, position) => <div className="criteria-rule" key={position}>
        <label><span className="criteria-control-label">{t('platform.criteriaRules.attribute')}</span><select aria-label={t('platform.criteriaRules.caseValueAttributeValue2', { value: index + 1, value2: position + 1 })} value={field} onChange={(event) => {
          const target = event.target.value as CriterionField;
          update(draft.rows.map((item, i) => i === index ? { ...item, [field]: '', [target]: item[field] } : item), selected.map((keys, i) => i === index ? keys.map((key, j) => j === position ? target : key) : keys));
        }}>{criterionFields.map(({ key, label }) => <option key={key} value={key} disabled={key !== field && selected[index]?.includes(key)}>{label}</option>)}</select></label>
        <span className="muted criteria-is">{t('platform.criteriaRules.is')}</span>
        <label><span className="criteria-control-label">{t('platform.criteriaRules.value')}</span><input aria-label={t('platform.criteriaRules.conditionValueLabel', { value: index + 1, label: criterionFields.find(({ key }) => key === field)?.label })} value={row[field]} aria-required="true" aria-invalid={row[field] === '' ? true : undefined} onChange={(event) => update(draft.rows.map((item, i) => i === index ? { ...item, [field]: event.target.value } : item), selected)} /></label>
        <Button type="button" tone="quiet" aria-label={t('platform.criteriaRules.removeConditionValueField', { value: index + 1, field })} onClick={() => update(draft.rows.map((item, i) => i === index ? { ...item, [field]: '' } : item), selected.map((keys, i) => i === index ? keys.filter((key) => key !== field) : keys))}>×</Button>
      </div>)}
      <Button type="button" disabled={selected[index]?.length === criterionFields.length} onClick={() => {
        const next = criterionFields.find(({ key }) => !selected[index]?.includes(key));
        if (next) update(draft.rows, selected.map((keys, i) => i === index ? [...keys, next.key] : keys));
      }}>{t('platform.criteriaRules.addCondition')}</Button>
    </section>)}
    <Button type="button" onClick={() => update([...draft.rows, Object.fromEntries(criterionFields.map(({ key }) => [key, ''])) as Criterion], [...selected, []])}>{t('platform.criteriaRules.addMatchingCase')}</Button>
  </div>;
}
