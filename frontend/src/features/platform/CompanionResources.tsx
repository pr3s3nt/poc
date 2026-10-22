import { Button } from '../../shared/ui/Button';
import type { TypeContract } from './moduleMapping';
import { SourceCard } from './ParameterSources';
import { newProvisionRow, type ProvisionRow } from './typedProvision';
import { entryFor, entryValue, parseObjectText, type JsonObject } from './typedVariables';
import { classChoices, defaultClass, descriptorOf, environmentScopeId, infraScopeId, parseDescriptor, referenceTypes, scopeFromId, type ResourceScope } from './valueSource';

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
  if (!type) return { reason: 'Chọn Loại tài nguyên có metadata để dựng params theo schema của Loại.' };
  const parsed = parseObjectText(row.params, 'Params');
  if (!parsed.value) return { reason: `${parsed.error ?? ''} Sửa ở Cấu hình kỹ thuật.` };
  const values: Record<string, string> = {};
  for (const [name, value] of Object.entries(parsed.value)) {
    const input = type.inputs.find((item) => item.name === name);
    if (!input) return { reason: `Param "${name}" không thuộc schema của Loại ${type.key}. Giữ nguyên trong Cấu hình kỹ thuật.` };
    const entry = entryFor({ ...variableOf(name, input.type) }, value);
    if (!entry) return { reason: `Giá trị của "${name}" không biểu diễn an toàn bằng control. Giữ nguyên trong Cấu hình kỹ thuật.` };
    values[name] = entry.raw;
  }
  return { values };
}

export function CompanionResources({ rows, onChange, parentType, types, disabled = false }: Props) {
  const patch = (id: number, change: Partial<ProvisionRow>) => onChange(rows.map((row) => (row.id === id ? { ...row, ...change } : row)));
  const parentName = parentType || 'Tài nguyên đang cấu hình';
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

  return <section className="catalog-fields companion-section" aria-label="Tài nguyên đi kèm">
    <div className="section-header"><div><h4>Tài nguyên đi kèm</h4><p className="feature-note">Tạo thêm tài nguyên khi cấu hình này được sử dụng.</p></div></div>
    <div className="companion-parent"><span className="companion-icon" aria-hidden="true">▣</span><div><small>TÀI NGUYÊN ĐANG CẤU HÌNH</small><strong>{parentName}</strong></div></div>
    {rows.length === 0 ? <p className="muted">Chưa có tài nguyên đi kèm.</p> : null}
    {rows.map((row, index) => {
      const number = index + 1;
      const descriptor = parseDescriptor(row.descriptor);
      const modelled = row.descriptor.trim() === '' || descriptor !== undefined;
      const type = descriptor ? types.find((item) => item.key === descriptor.type) : undefined;
      const params = descriptor ? paramsView(row, type) : { reason: '' };
      const companionName = descriptor?.type || `Tài nguyên đi kèm ${number}`;
      return <div key={row.id} className="companion-card provision-row" data-companion={number}>
        <div className="companion-head"><span className="companion-icon" aria-hidden="true">+</span><div><strong>{companionName}</strong><small className="muted">{row.descriptor || 'Chưa chọn loại tài nguyên'}</small></div>
          <Button type="button" tone="quiet" aria-label={`Xóa tài nguyên đi kèm ${number}`} disabled={disabled} onClick={() => onChange(rows.filter((item) => item.id !== row.id))}>Xóa</Button></div>
        {modelled ? <div className="field-grid">
          <label>Loại tài nguyên<select aria-label={`Loại tài nguyên đi kèm ${number}`} value={descriptor?.type ?? ''} disabled={disabled} onChange={(event) => setDescriptor(row, { type: event.target.value, class: descriptor?.class ?? defaultClass, scope: descriptor?.scope ?? { kind: 'environment' } })}>
            <option value="">Chọn loại tài nguyên</option>
            {descriptor && !types.some((item) => item.key === descriptor.type) ? <option value={descriptor.type}>{descriptor.type} (không còn trong metadata)</option> : null}
            {referenceTypes(types).map((item) => <option key={item.key} value={item.key}>{item.key}</option>)}</select></label>
          <label>Nhóm cấu hình<select aria-label={`Nhóm cấu hình tài nguyên đi kèm ${number}`} value={descriptor?.class ?? defaultClass} disabled={disabled || !descriptor} onChange={(event) => descriptor ? setDescriptor(row, { ...descriptor, class: event.target.value }) : undefined}>
            {classChoices(type, descriptor?.class ?? '').map((item) => <option key={item} value={item}>{item === defaultClass ? 'Mặc định · default' : item}</option>)}</select></label>
          <label>Phạm vi<select aria-label={`Phạm vi tài nguyên đi kèm ${number}`} value={descriptor ? (descriptor.scope.kind === 'resource' ? 'resource' : descriptor.scope.kind === 'infra' ? infraScopeId : environmentScopeId) : environmentScopeId} disabled={disabled || !descriptor} onChange={(event) => descriptor ? setDescriptor(row, { ...descriptor, scope: event.target.value === 'resource' ? { kind: 'resource', id: '' } : scopeFromId(event.target.value) }) : undefined}>
            <option value={environmentScopeId}>Cùng ứng dụng và môi trường</option><option value={infraScopeId}>Ngữ cảnh hạ tầng</option><option value="resource">ID tài nguyên cụ thể</option></select></label>
          {descriptor?.scope.kind === 'resource' ? <label>ID tài nguyên<input aria-label={`ID tài nguyên đi kèm ${number}`} value={descriptor.scope.id} spellCheck={false} disabled={disabled} onChange={(event) => setDescriptor(row, { ...descriptor, scope: { kind: 'resource', id: event.target.value } })} /></label> : null}
        </div> : <div className="form-warning" role="note">Descriptor này không biểu diễn bằng control (cần dạng loại.class#phạm-vi). Giữ nguyên và sửa ở Cấu hình kỹ thuật.</div>}
        {descriptor && type ? <div className="companion-params" aria-label={`Params của ${companionName}`}>
          {params.values ? (type.inputs.length === 0 ? <p className="feature-note">Loại tài nguyên này không khai báo params.</p> : type.inputs.map((input) => <SourceCard key={input.name} name={input.name} type={input.type} required={input.required === true} raw={params.values?.[input.name]}
            inherited={{ origin: 'Chưa cung cấp', value: input.required ? 'Không gửi giá trị; tham số do ứng dụng hoặc Loại tài nguyên cung cấp.' : 'Tùy chọn: bỏ trống thì không gửi giá trị.', choice: { label: 'Không cung cấp', description: 'Không gửi param này.' } }}
            types={types} disabled={disabled} onChange={(raw) => setParam(row, type, input.name, raw)} />)) : <div className="form-warning" role="note">{params.reason}</div>}
        </div> : null}
        <div className="companion-flags"><h5>Quan hệ phụ thuộc</h5>
          <label className="catalog-check companion-flag"><input type="checkbox" aria-label={`Tài nguyên đi kèm ${number}: ${companionName} chờ ${parentName} tạo xong (is_dependent)`} checked={row.isDependent} disabled={disabled} onChange={(event) => patch(row.id, { isDependent: event.target.checked })} /> <span><strong>{companionName}</strong> chờ <strong>{parentName}</strong> tạo xong</span></label>
          <label className="catalog-check companion-flag"><input type="checkbox" aria-label={`Tài nguyên đi kèm ${number}: các tài nguyên đang dùng ${parentName} cũng phải chờ ${companionName} (match_dependents)`} checked={row.matchDependents} disabled={disabled} onChange={(event) => patch(row.id, { matchDependents: event.target.checked })} /> <span>Các tài nguyên đang dùng <strong>{parentName}</strong> cũng phải chờ <strong>{companionName}</strong></span></label>
          <p className="feature-note">Cờ tắt không thêm cạnh phụ thuộc từ quy tắc này; reference riêng trong params vẫn tạo phụ thuộc.</p></div>
        <details className="companion-technical"><summary>Cấu hình kỹ thuật</summary>
          <label>Descriptor {number}<input aria-label={`Descriptor tài nguyên đi kèm ${number}`} value={row.descriptor} spellCheck={false} disabled={disabled} placeholder="loại.class#phạm-vi, ví dụ vpc.default#@infra" onChange={(event) => patch(row.id, { descriptor: event.target.value })} /></label>
          <label>Params {number} (JSON object)<textarea aria-label={`Params tài nguyên đi kèm ${number} (JSON object)`} rows={3} value={row.params} spellCheck={false} disabled={disabled} onChange={(event) => patch(row.id, { params: event.target.value })} /></label></details>
      </div>;
    })}
    <Button type="button" className="companion-add" disabled={disabled} onClick={() => onChange([...rows, newProvisionRow()])}>+ Thêm tài nguyên đi kèm</Button>
  </section>;
}
