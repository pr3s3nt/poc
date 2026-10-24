import { parseObjectText, type JsonObject } from './typedVariables';
import { t } from '../../shared/i18n';

// Typed rows of the provision object (UC-03 BR-26 / UC-06 BR-08). Each row is
// one co-provisioned resource rule keyed by its descriptor. Rows serialize to
// exactly {descriptor: {params, is_dependent, match_dependents}}; false flags and
// empty params are omitted like the backend does.
export type ProvisionRow = { id: number; descriptor: string; params: string; isDependent: boolean; matchDependents: boolean; keepIsDependent?: boolean; keepMatchDependents?: boolean; keepParams?: boolean };

let nextRowId = 1;
export const newProvisionRow = (): ProvisionRow => ({ id: nextRowId++, descriptor: '', params: '{}', isDependent: false, matchDependents: false });

export function provisionToRows(text: string): { rows?: ProvisionRow[]; reason?: string } {
  const parsed = parseObjectText(text, t('platform.typedProvision.relatedResourceRules'));
  if (!parsed.value) return { reason: t('platform.typedProvision.valueFixTheAdvancedJSON', { value: parsed.error ?? '' }) };
  const rows: ProvisionRow[] = [];
  for (const [descriptor, rule] of Object.entries(parsed.value)) {
    if (rule === null || typeof rule !== 'object' || Array.isArray(rule)) return { reason: t('platform.typedProvision.ruleDescriptorIsNotAn', { descriptor }) };
    const body = rule as JsonObject;
    for (const key of Object.keys(body)) {
      if (key !== 'params' && key !== 'is_dependent' && key !== 'match_dependents') return { reason: t('platform.typedProvision.ruleDescriptorHasFieldKey', { descriptor, key }) };
    }
    if ('is_dependent' in body && typeof body.is_dependent !== 'boolean') return { reason: t('platform.typedProvision.isDependentOfDescriptorMust', { descriptor }) };
    if ('match_dependents' in body && typeof body.match_dependents !== 'boolean') return { reason: t('platform.typedProvision.matchDependentsOfDescriptorMust', { descriptor }) };
    if ('params' in body && (body.params === null || typeof body.params !== 'object' || Array.isArray(body.params))) return { reason: t('platform.typedProvision.paramsOfDescriptorMustBe', { descriptor }) };
    rows.push({ id: nextRowId++, descriptor, params: JSON.stringify(body.params ?? {}, null, 2), isDependent: body.is_dependent === true, matchDependents: body.match_dependents === true, keepIsDependent: 'is_dependent' in body, keepMatchDependents: 'match_dependents' in body, keepParams: 'params' in body });
  }
  return { rows };
}

export function rowsToProvision(rows: ProvisionRow[]): { value?: JsonObject; problem?: string } {
  const out: JsonObject = {};
  for (const [index, row] of rows.entries()) {
    const descriptor = row.descriptor;
    if (descriptor.trim() === '') return { problem: t('platform.typedProvision.ruleValueEnterTheDescriptor', { value: index + 1 }) };
    if (Object.hasOwn(out, descriptor)) return { problem: t('platform.typedProvision.descriptorDescriptorIsDuplicatedEach', { descriptor }) };
    const params = parseObjectText(row.params, t('platform.typedProvision.paramsOfDescriptor', { descriptor }));
    if (!params.value) return { problem: params.error };
    const rule: JsonObject = {};
    // Fields present in the imported JSON keep their presence, even when false/empty.
    if (row.isDependent || row.keepIsDependent) rule.is_dependent = row.isDependent;
    if (row.matchDependents || row.keepMatchDependents) rule.match_dependents = row.matchDependents;
    if (Object.keys(params.value).length > 0 || row.keepParams) rule.params = params.value;
    out[descriptor] = rule;
  }
  return { value: out };
}
