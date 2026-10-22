import { useEffect, useId, useRef, useState, type KeyboardEvent } from 'react';
import { Button } from '../../shared/ui/Button';
import type { TypeContract } from './moduleMapping';
import { baseKind, entryValue } from './typedVariables';
import { classChoices, compatibleOutputs, contextAllowed, contextChoices, contextPlaceholder, defaultClass, environmentScopeId, infraScopeId, parseSource, referencePlaceholder, referenceTypes, scopeFromId, type ResourceReference, type ValueSource } from './valueSource';

// The shared chooser of a value source (UC-03 UI11-02). It only offers choices that are valid for the target and writes
// nothing until “Áp dụng”: the result is one string (a literal, a generated placeholder) or `null` for “no override”.

export type SourceTarget = { name: string; type: string; label: string };
export type InheritChoice = { label: string; description: string };
type Mode = 'inherit' | 'literal' | 'context' | 'resource' | 'advanced';
type Props = {
  target: SourceTarget;
  raw: string | undefined;
  types: TypeContract[];
  inherit?: InheritChoice;
  onApply: (raw: string | null) => void;
  onCancel: () => void;
};

const kindLabels = { string: 'chuỗi', number: 'số', bool: 'đúng/sai', list: 'danh sách', map: 'bản đồ khóa-giá trị', any: 'giá trị bất kỳ' } as const;
const emptyRef = (): ResourceReference => ({ type: '', class: defaultClass, scope: { kind: 'environment' }, output: '' });

export function ValueSourceDialog({ target, raw, types, inherit, onApply, onCancel }: Props) {
  const initial: ValueSource | undefined = raw === undefined ? undefined : parseSource(raw);
  const [mode, setMode] = useState<Mode>(initial === undefined ? (inherit ? 'inherit' : 'literal') : initial.kind);
  const [literal, setLiteral] = useState(initial?.kind === 'literal' ? initial.text : '');
  const [contextPath, setContextPath] = useState(initial?.kind === 'context' ? initial.path : contextChoices[0].path);
  const [ref, setRef] = useState<ResourceReference>(initial?.kind === 'resource' ? initial.ref : emptyRef());
  const heading = useId();
  const dialog = useRef<HTMLDivElement>(null);
  useEffect(() => { dialog.current?.focus(); }, []);

  const kind = baseKind(target.type);
  const probe = { name: target.name, type: target.type, required: false, hasDefault: false, resourceParam: false };
  const literalCheck = mode === 'literal' ? entryValue(probe, { on: true, raw: literal }) : {};
  const refType = types.find((type) => type.key === ref.type);
  const outputs = compatibleOutputs(refType, target.type);
  const allowContext = contextAllowed(target.type);
  const scopeValue = ref.scope.kind === 'environment' ? environmentScopeId : ref.scope.kind === 'infra' ? infraScopeId : 'resource';

  let result: string | null | undefined;
  let blocker = '';
  if (mode === 'inherit') result = null;
  else if (mode === 'advanced') result = raw;
  else if (mode === 'context') result = contextPlaceholder(contextPath);
  else if (mode === 'literal') { if (literalCheck.error) blocker = literalCheck.error; else result = literal; }
  else if (ref.type === '') blocker = 'Chọn Loại tài nguyên.';
  else if (ref.class === '') blocker = 'Chọn Nhóm cấu hình.';
  else if (ref.scope.kind === 'resource' && ref.scope.id.trim() === '') blocker = 'Nhập ID tài nguyên.';
  else if (ref.output === '' || !outputs.some((output) => output.name === ref.output)) blocker = outputs.length === 0 ? 'Loại tài nguyên này không có output tương thích với kiểu của thông số.' : 'Chọn output cần lấy.';
  else result = referencePlaceholder(ref);

  const setRefPart = (patch: Partial<ResourceReference>) => setRef((current) => ({ ...current, ...patch }));
  const option = (value: Mode, title: string, hint: string) => <label className={`value-option${mode === value ? ' value-option-active' : ''}`}><input type="radio" name={`${heading}-mode`} aria-label={title} checked={mode === value} onChange={() => setMode(value)} /><span><strong>{title}</strong><small>{hint}</small></span></label>;
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => { if (event.key === 'Escape') { event.stopPropagation(); onCancel(); } };

  return <div className="dialog-backdrop">
    <div className="dialog value-dialog" role="dialog" aria-modal="true" aria-labelledby={heading} tabIndex={-1} ref={dialog} onKeyDown={onKeyDown}>
      <div className="value-dialog-head"><div><h2 id={heading}>Chọn nguồn giá trị</h2><p className="muted">{target.label}</p></div><button type="button" className="button button-quiet" aria-label="Đóng" onClick={onCancel}>×</button></div>
      <div className="value-options" role="radiogroup" aria-label="Nguồn giá trị">
        {inherit ? option('inherit', inherit.label, inherit.description) : null}
        {option('literal', 'Nhập giá trị', `Nhập theo kiểu ${kindLabels[kind]}.`)}
        {allowContext ? option('context', 'Lấy từ ngữ cảnh triển khai', 'Hệ thống cung cấp giá trị khi triển khai; bạn không cần nhập biểu thức.') : null}
        {option('resource', 'Lấy từ tài nguyên khác', 'Chọn Loại tài nguyên, class, phạm vi và output.')}
        {initial?.kind === 'advanced' ? option('advanced', 'Giữ biểu thức nâng cao', 'Biểu thức này không biểu diễn được bằng lựa chọn; giữ nguyên.') : null}
      </div>
      {mode === 'literal' ? <div className="value-body">
        {kind === 'bool' ? <label>Giá trị {target.name}<select aria-label={`Giá trị ${target.name}`} value={literal === 'true' || literal === 'false' ? literal : ''} onChange={(event) => setLiteral(event.target.value)}><option value="">Chọn giá trị</option><option value="true">true</option><option value="false">false</option></select></label>
          : kind === 'string' || kind === 'number' ? <label>Giá trị {target.name} ({kindLabels[kind]})<input aria-label={`Giá trị ${target.name}`} inputMode={kind === 'number' ? 'decimal' : undefined} value={literal} spellCheck={false} aria-invalid={literalCheck.error ? true : undefined} onChange={(event) => setLiteral(event.target.value)} /></label>
            : <label>Giá trị {target.name} ({kindLabels[kind]}, JSON)<textarea aria-label={`Giá trị ${target.name}`} rows={kind === 'any' ? 3 : 4} value={literal} spellCheck={false} aria-invalid={literalCheck.error ? true : undefined} onChange={(event) => setLiteral(event.target.value)} /></label>}
      </div> : null}
      {mode === 'context' ? <div className="value-body"><label>Giá trị ngữ cảnh<select aria-label="Giá trị ngữ cảnh" value={contextPath} onChange={(event) => setContextPath(event.target.value)}>{contextChoices.map((choice) => <option key={choice.path} value={choice.path}>{choice.label}</option>)}</select></label></div> : null}
      {mode === 'resource' ? <div className="value-body value-grid">
        <label>Loại tài nguyên<select aria-label="Loại tài nguyên nguồn" value={ref.type} onChange={(event) => setRef({ ...ref, type: event.target.value, output: '', class: defaultClass })}>
          <option value="">Chọn loại tài nguyên</option>
          {ref.type !== '' && !types.some((type) => type.key === ref.type) ? <option value={ref.type}>{ref.type} (không còn trong metadata)</option> : null}
          {referenceTypes(types).map((type) => <option key={type.key} value={type.key}>{type.key}</option>)}</select></label>
        <label>Nhóm cấu hình<select aria-label="Nhóm cấu hình nguồn" value={ref.class} onChange={(event) => setRefPart({ class: event.target.value })}>{classChoices(refType, ref.class).map((item) => <option key={item} value={item}>{item === defaultClass ? 'Mặc định · default' : item}</option>)}</select></label>
        <label className="value-wide">Phạm vi<select aria-label="Phạm vi nguồn" value={scopeValue} onChange={(event) => setRefPart({ scope: event.target.value === 'resource' ? { kind: 'resource', id: '' } : scopeFromId(event.target.value) })}>
          <option value={environmentScopeId}>Cùng ứng dụng và môi trường</option><option value={infraScopeId}>Ngữ cảnh hạ tầng</option><option value="resource">ID tài nguyên cụ thể</option></select></label>
        {ref.scope.kind === 'resource' ? <label className="value-wide">ID tài nguyên<input aria-label="ID tài nguyên nguồn" value={ref.scope.id} spellCheck={false} onChange={(event) => setRefPart({ scope: { kind: 'resource', id: event.target.value } })} /></label> : null}
        <label className="value-wide">Output cần lấy<select aria-label="Output cần lấy" value={ref.output} disabled={ref.type === ''} onChange={(event) => setRefPart({ output: event.target.value })}>
          <option value="">Chọn output</option>
          {outputs.map((output) => <option key={output.name} value={output.name}>{output.name}{output.type ? ` · ${output.type}` : ''}</option>)}</select></label>
        <p className="feature-note">Chỉ liệt kê output công khai của Loại tài nguyên, không bí mật và tương thích kiểu. Hệ thống chưa kiểm tra matching hoặc vòng phụ thuộc ở bước này.</p>
      </div> : null}
      {mode === 'advanced' && raw !== undefined ? <div className="value-body"><code>{raw}</code></div> : null}
      {mode === 'resource' && result ? <details className="value-technical"><summary>Biểu thức sinh ra (kỹ thuật)</summary><code>{result}</code></details> : null}
      {blocker ? <div className="form-error" role="note">{blocker}</div> : null}
      <div className="form-actions"><Button type="button" onClick={onCancel}>Hủy</Button><Button type="button" tone="primary" disabled={result === undefined} onClick={() => { if (result !== undefined) onApply(result); }}>Áp dụng</Button></div>
    </div>
  </div>;
}
