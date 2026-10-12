import type { CapabilityVariable } from './capabilities';
import { baseKind, entryValue, type VariableDraft, type VariableEntry } from './typedVariables';

const kindLabels = { string: 'chuỗi', number: 'số', bool: 'đúng/sai', list: 'danh sách', map: 'bản đồ khóa-giá trị', any: 'giá trị bất kỳ' } as const;

const defaultText = (variable: CapabilityVariable) => (typeof variable.default === 'string' ? variable.default : JSON.stringify(variable.default));

function sourceNote(variable: CapabilityVariable): string {
  const parts: string[] = [];
  if (variable.resourceParam) parts.push('Nguồn: tham số (param) của Loại tài nguyên; Cấu hình không điền lại. Giá trị bạn nhập ở đây chỉ là giá trị dự phòng: khi workload khai báo param thật, param của workload vẫn được ưu tiên.');
  if (variable.hasDefault) parts.push(`Mặc định: ${defaultText(variable)}. Bỏ trống để dùng mặc định, không gửi giá trị.`);
  if (variable.required && !variable.hasDefault && !variable.resourceParam) parts.push('Bắt buộc: phải ghi đè và nhập giá trị.');
  else if (!variable.hasDefault && !variable.resourceParam) parts.push('Tùy chọn: bỏ trống thì hệ thống không gửi giá trị.');
  return parts.join(' ');
}

type Props = {
  variables: CapabilityVariable[];
  draft: VariableDraft;
  onChange: (draft: VariableDraft) => void;
  disabled?: boolean;
};

// One opt-in control per editable Driver Input variable (UC-03 BR-24/BR-25).
// Executor-owned variables and credentials have no control; the JSON mode keeps
// anything this form cannot represent.
export function TypedVariablesEditor({ variables, draft, onChange, disabled = false }: Props) {
  const patch = (name: string, change: Partial<VariableEntry>) => onChange({ ...draft, [name]: { on: false, raw: '', ...draft[name], ...change } });
  const remove = (name: string) => { const next = { ...draft }; delete next[name]; onChange(next); };
  if (variables.length === 0) return <p className="feature-note">Cách tạo này không có tham số cấu hình để nhập.</p>;
  return <div className="typed-variables">{variables.map((variable) => {
    const entry = draft[variable.name];
    const on = entry?.on === true;
    const kind = baseKind(variable.type);
    const id = `typed-${variable.name}`;
    const check = on ? entryValue(variable, entry) : {};
    const setRaw = (raw: string) => patch(variable.name, { on: true, raw });
    return <div key={variable.name} className="typed-variable" data-variable={variable.name}>
      <label className="typed-variable-toggle"><input type="checkbox" aria-label={`Ghi đè ${variable.name}`} checked={on} disabled={disabled} onChange={(event) => (event.target.checked ? patch(variable.name, { on: true, raw: entry?.raw ?? '' }) : remove(variable.name))} /> <code>{variable.name}</code> <span className="status-pill">{variable.type}</span>{variable.required && !variable.hasDefault && !variable.resourceParam ? <span className="status-pill">Bắt buộc</span> : null}</label>
      <p id={`${id}-note`} className="feature-note">{sourceNote(variable)}</p>
      {on ? <div className="typed-variable-control">
        {kind === 'bool' ? <label>Giá trị {variable.name}<select aria-label={`Giá trị ${variable.name}`} aria-describedby={`${id}-note`} value={entry?.placeholder ? 'placeholder' : entry?.raw === 'true' || entry?.raw === 'false' ? entry.raw : ''} disabled={disabled} onChange={(event) => (event.target.value === 'placeholder' ? patch(variable.name, { raw: '', placeholder: true }) : patch(variable.name, { raw: event.target.value, placeholder: false }))}>
          <option value="">Chọn giá trị</option><option value="true">true</option><option value="false">false</option><option value="placeholder">Placeholder nguyên vẹn…</option></select></label> : null}
        {kind === 'bool' && entry?.placeholder ? <label>Placeholder {variable.name}<input aria-label={`Placeholder ${variable.name}`} value={entry.raw} spellCheck={false} disabled={disabled} onChange={(event) => patch(variable.name, { raw: event.target.value })} /></label> : null}
        {kind === 'string' || kind === 'number' ? <label>Giá trị {variable.name} ({kindLabels[kind]})<input aria-label={`Giá trị ${variable.name}`} aria-describedby={check.error ? `${id}-note ${id}-error` : `${id}-note`} aria-invalid={check.error ? true : undefined} inputMode={kind === 'number' ? 'decimal' : undefined} value={entry?.raw ?? ''} spellCheck={false} disabled={disabled} onChange={(event) => setRaw(event.target.value)} /></label> : null}
        {kind === 'list' || kind === 'map' || kind === 'any' ? <label>Giá trị {variable.name} ({kindLabels[kind]}, JSON hoặc placeholder nguyên vẹn)<textarea aria-label={`Giá trị ${variable.name}`} aria-describedby={check.error ? `${id}-note ${id}-error` : `${id}-note`} aria-invalid={check.error ? true : undefined} rows={kind === 'any' ? 2 : 3} value={entry?.raw ?? ''} spellCheck={false} disabled={disabled} onChange={(event) => setRaw(event.target.value)} /></label> : null}
        {variable.preset ? <button type="button" className="button button-quiet" aria-label={`Dùng preset cho ${variable.name}`} disabled={disabled} onClick={() => patch(variable.name, { on: true, raw: variable.preset ?? '', placeholder: kind === 'bool' ? true : undefined })}>Dùng preset <code>{variable.preset}</code></button> : null}
        {check.error ? <div id={`${id}-error`} className="form-error">{check.error}</div> : null}
      </div> : variable.preset ? <button type="button" className="button button-quiet" aria-label={`Dùng preset cho ${variable.name}`} disabled={disabled} onClick={() => patch(variable.name, { on: true, raw: variable.preset ?? '' })}>Dùng preset <code>{variable.preset}</code></button> : null}
    </div>;
  })}</div>;
}
