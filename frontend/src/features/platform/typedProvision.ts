import { parseObjectText, type JsonObject } from './typedVariables';

// Typed rows of the provision object (UC-03 BR-26 / UC-06 BR-08). Each row is
// one co-provisioned resource rule keyed by its descriptor. Rows serialize to
// exactly {descriptor: {params, is_dependent, match_dependents}}; false flags and
// empty params are omitted like the backend does.
export type ProvisionRow = { id: number; descriptor: string; params: string; isDependent: boolean; matchDependents: boolean; keepIsDependent?: boolean; keepMatchDependents?: boolean; keepParams?: boolean };

let nextRowId = 1;
export const newProvisionRow = (): ProvisionRow => ({ id: nextRowId++, descriptor: '', params: '{}', isDependent: false, matchDependents: false });

export function provisionToRows(text: string): { rows?: ProvisionRow[]; reason?: string } {
  const parsed = parseObjectText(text, 'Quy tắc tạo tài nguyên liên quan');
  if (!parsed.value) return { reason: `${parsed.error ?? ''} Sửa JSON nâng cao trước khi chuyển sang biểu mẫu.` };
  const rows: ProvisionRow[] = [];
  for (const [descriptor, rule] of Object.entries(parsed.value)) {
    if (rule === null || typeof rule !== 'object' || Array.isArray(rule)) return { reason: `Quy tắc "${descriptor}" không phải object. Giữ trong JSON nâng cao.` };
    const body = rule as JsonObject;
    for (const key of Object.keys(body)) {
      if (key !== 'params' && key !== 'is_dependent' && key !== 'match_dependents') return { reason: `Quy tắc "${descriptor}" có trường "${key}" mà biểu mẫu không biểu diễn. Giữ trong JSON nâng cao.` };
    }
    if ('is_dependent' in body && typeof body.is_dependent !== 'boolean') return { reason: `"is_dependent" của "${descriptor}" phải là true/false. Giữ trong JSON nâng cao.` };
    if ('match_dependents' in body && typeof body.match_dependents !== 'boolean') return { reason: `"match_dependents" của "${descriptor}" phải là true/false. Giữ trong JSON nâng cao.` };
    if ('params' in body && (body.params === null || typeof body.params !== 'object' || Array.isArray(body.params))) return { reason: `"params" của "${descriptor}" phải là JSON object. Giữ trong JSON nâng cao.` };
    rows.push({ id: nextRowId++, descriptor, params: JSON.stringify(body.params ?? {}, null, 2), isDependent: body.is_dependent === true, matchDependents: body.match_dependents === true, keepIsDependent: 'is_dependent' in body, keepMatchDependents: 'match_dependents' in body, keepParams: 'params' in body });
  }
  return { rows };
}

export function rowsToProvision(rows: ProvisionRow[]): { value?: JsonObject; problem?: string } {
  const out: JsonObject = {};
  for (const [index, row] of rows.entries()) {
    const descriptor = row.descriptor;
    if (descriptor.trim() === '') return { problem: `Quy tắc ${index + 1}: nhập descriptor của tài nguyên đi kèm (ví dụ vpc.default#@infra).` };
    if (Object.hasOwn(out, descriptor)) return { problem: `Descriptor "${descriptor}" bị trùng. Mỗi tài nguyên đi kèm chỉ có một quy tắc.` };
    const params = parseObjectText(row.params, `Params của "${descriptor}"`);
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
