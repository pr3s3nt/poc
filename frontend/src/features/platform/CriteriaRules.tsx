import { Button } from '../../shared/ui/Button';
import type { Criterion, CriteriaDraft, CriterionField } from './CriteriaEditor';

export const criterionFields: { key: CriterionField; label: string }[] = [
  { key: 'env_type', label: 'Loại môi trường' }, { key: 'app_id', label: 'ID ứng dụng' },
  { key: 'env_id', label: 'ID môi trường' }, { key: 'res_id', label: 'ID tài nguyên' }, { key: 'class', label: 'Class' },
];
// Keep selected-but-empty conditions in the owner's draft. They are incomplete,
// whereas an explicitly empty case is a wildcard; neither tab changes nor copy
// confirmations may silently turn one into the other.
export const selectedCriterionFields = (draft: CriteriaDraft): CriterionField[][] => draft.fields ?? draft.rows.map((row) => criterionFields.filter(({ key }) => row[key] !== '').map(({ key }) => key));

export function CriteriaRules({ draft, onChange }: { draft: CriteriaDraft; onChange(next: CriteriaDraft): void }) {
  const selected = selectedCriterionFields(draft);
  const update = (rows: CriteriaDraft['rows'], fields: CriterionField[][]) => onChange({ mode: 'advanced', rows, fields });
  return <div className="criteria-cases">
    <p className="feature-note criteria-logic">Tất cả điều kiện trong một trường hợp phải khớp (AND). Chỉ cần một trường hợp khớp (OR).</p>
    {draft.rows.map((row, index) => <section className="criteria-case" key={index} aria-label={`Trường hợp ${index + 1}`}>
      <div className="section-header"><h4>Trường hợp {index + 1}</h4><Button type="button" tone="quiet" aria-label={`Xóa trường hợp ${index + 1}`} onClick={() => update(draft.rows.filter((_, i) => i !== index), selected.filter((_, i) => i !== index))}>Xóa trường hợp</Button></div>
      <p className="criteria-case-hint">Áp dụng khi tất cả điều kiện sau đều khớp</p>
      {(selected[index] ?? []).length === 0 ? <p className="feature-note">Mọi nơi (không giới hạn) · wildcard {'{}'}, điểm 0.</p> : null}
      {(selected[index] ?? []).map((field, position) => <div className="criteria-rule" key={position}>
        <label><span className="criteria-control-label">Thuộc tính</span><select aria-label={`Trường hợp ${index + 1} thuộc tính ${position + 1}`} value={field} onChange={(event) => {
          const target = event.target.value as CriterionField;
          update(draft.rows.map((item, i) => i === index ? { ...item, [field]: '', [target]: item[field] } : item), selected.map((keys, i) => i === index ? keys.map((key, j) => j === position ? target : key) : keys));
        }}>{criterionFields.map(({ key, label }) => <option key={key} value={key} disabled={key !== field && selected[index]?.includes(key)}>{label}</option>)}</select></label>
        <span className="muted criteria-is">là</span>
        <label><span className="criteria-control-label">Giá trị</span><input aria-label={`Điều kiện ${index + 1} ${criterionFields.find(({ key }) => key === field)?.label}`} value={row[field]} aria-required="true" aria-invalid={row[field] === '' ? true : undefined} onChange={(event) => update(draft.rows.map((item, i) => i === index ? { ...item, [field]: event.target.value } : item), selected)} /></label>
        <Button type="button" tone="quiet" aria-label={`Xóa điều kiện ${index + 1} ${field}`} onClick={() => update(draft.rows.map((item, i) => i === index ? { ...item, [field]: '' } : item), selected.map((keys, i) => i === index ? keys.filter((key) => key !== field) : keys))}>×</Button>
      </div>)}
      <Button type="button" disabled={selected[index]?.length === criterionFields.length} onClick={() => {
        const next = criterionFields.find(({ key }) => !selected[index]?.includes(key));
        if (next) update(draft.rows, selected.map((keys, i) => i === index ? [...keys, next.key] : keys));
      }}>+ Thêm điều kiện</Button>
    </section>)}
    <Button type="button" onClick={() => update([...draft.rows, Object.fromEntries(criterionFields.map(({ key }) => [key, ''])) as Criterion], [...selected, []])}>+ Thêm trường hợp áp dụng</Button>
  </div>;
}
