import { Button } from '../../shared/ui/Button';
import type { Capability } from './capabilities';
import { platformOutput, inputCompatible, outputCompatible, sameNameInputs, sameNameOutputs, type MappingDraft, type MappingProblem, type TypeContract } from './moduleMapping';

type Props = {
  capability: Capability;
  type: TypeContract | undefined;
  draft: MappingDraft;
  onChange: (draft: MappingDraft) => void;
  problems: MappingProblem[];
  disabled?: boolean;
};

// Typed controls of input_mapping / output_mapping (UC-03 BR-11); provider
// bindings are edited with the infrastructure connection of section 2. Choices come only from the verified module metadata and the Resource
// Type contract; "Ghép theo tên trùng" is an explicit action, never a default.
export function MappingEditor({ capability, type, draft, onChange, problems, disabled = false }: Props) {
  const problemFor = (field: string) => problems.find((problem) => problem.field === field)?.message;
  if (!type) return <p className="feature-note" role="status">Đang chờ hợp đồng của Loại tài nguyên để ánh xạ.</p>;
  const usedVariables = (except: string) => new Set(Object.entries(draft.inputs).filter(([name, variable]) => name !== except && variable !== '').map(([, variable]) => variable));
  const usedOutputs = (except: string) => new Set(Object.entries(draft.outputs).filter(([name, output]) => name !== except && output !== '').map(([, output]) => output));
  const setInput = (name: string, variable: string) => { const inputs = { ...draft.inputs }; if (variable === '') delete inputs[name]; else inputs[name] = variable; onChange({ ...draft, inputs }); };
  const setOutput = (name: string, output: string) => { const outputs = { ...draft.outputs }; if (output === '') delete outputs[name]; else outputs[name] = output; onChange({ ...draft, outputs }); };
  return <div className="mapping-editor">
    <div className="mapping-group" aria-label="Ánh xạ đầu vào">
      <div className="section-header"><div><h4>Ánh xạ đầu vào</h4><details className="definition-help"><summary>Cách ánh xạ đầu vào</summary><p className="feature-note">Mỗi tham số của Loại tài nguyên phải được nhận bởi một biến module. Tham số thật của workload luôn được ưu tiên hơn giá trị dự phòng của cấu hình và mặc định của module.</p></details></div>
        {type.inputs.length > 0 ? <Button type="button" tone="quiet" disabled={disabled} onClick={() => onChange({ ...draft, inputs: sameNameInputs(capability, type) })}>Ghép đầu vào theo tên trùng</Button> : null}</div>
      {type.inputs.length === 0 ? <p className="feature-note">Loại tài nguyên này không khai báo tham số đầu vào.</p> : type.inputs.map((input) => {
        const taken = usedVariables(input.name);
        const options = capability.variables.filter((variable) => inputCompatible(input.type, variable.type) && !taken.has(variable.name));
        const problem = problemFor(`input:${input.name}`);
        return <label key={input.name} className="mapping-row mapping-input-row"><span className="mapping-key"><strong>{input.name}</strong><small>{input.type || 'any'}{input.required ? ' · Bắt buộc' : ' · Tùy chọn'}</small></span><span className="mapping-direction" aria-hidden="true">→</span>
          <select aria-label={`Biến module nhận tham số ${input.name}`} aria-invalid={problem ? true : undefined} value={draft.inputs[input.name] ?? ''} disabled={disabled} onChange={(event) => setInput(input.name, event.target.value)}>
            <option value="">Chưa chọn</option>
            {draft.inputs[input.name] && !options.some((variable) => variable.name === draft.inputs[input.name]) ? <option value={draft.inputs[input.name]}>{draft.inputs[input.name]} (không còn hợp lệ)</option> : null}
            {options.map((variable) => <option key={variable.name} value={variable.name}>{variable.name} ({variable.type})</option>)}
          </select>{problem ? <span className="form-error" role="note">{problem}</span> : null}</label>;
      })}
    </div>
    <div className="mapping-group" aria-label="Ánh xạ đầu ra">
      <div className="section-header"><div><h4>Ánh xạ đầu ra</h4><details className="definition-help"><summary>Cách ánh xạ đầu ra</summary><p className="feature-note">Phải đủ các output bắt buộc, đúng kiểu. Cờ bí mật của Loại hoặc module được giữ; không thể hạ cờ bí mật khi ánh xạ.</p></details></div>
        <Button type="button" tone="quiet" disabled={disabled} onClick={() => onChange({ ...draft, outputs: sameNameOutputs(capability, type) })}>Ghép đầu ra theo tên trùng</Button></div>
      {type.outputs.map((output) => {
        if (platformOutput(capability, output.name)) return <p key={output.name} className="feature-note">Output <code>{output.name}</code> do nền tảng cung cấp từ danh tính cluster đã tạo; không ánh xạ từ output module.</p>;
        const taken = usedOutputs(output.name);
        const options = capability.outputs.filter((candidate) => outputCompatible(output.type, candidate.type) && !taken.has(candidate.name));
        const problem = problemFor(`output:${output.name}`);
        return <label key={output.name} className="mapping-row mapping-output-row"><span className="mapping-key"><strong>{output.name}</strong><small>{output.type || 'any'}{output.required ? ' · Bắt buộc' : ' · Tùy chọn'}{output.secret ? ' · Giá trị bí mật' : ''}</small></span><span className="mapping-direction" aria-hidden="true">←</span>
          <select aria-label={`Output module cung cấp ${output.name}`} aria-invalid={problem ? true : undefined} value={draft.outputs[output.name] ?? ''} disabled={disabled} onChange={(event) => setOutput(output.name, event.target.value)}>
            <option value="">Chưa chọn</option>
            {draft.outputs[output.name] && !options.some((candidate) => candidate.name === draft.outputs[output.name]) ? <option value={draft.outputs[output.name]}>{draft.outputs[output.name]} (không còn hợp lệ)</option> : null}
            {options.map((candidate) => <option key={candidate.name} value={candidate.name}>{candidate.name}{candidate.type ? ` (${candidate.type})` : ''}{candidate.sensitive ? ' · bí mật' : ''}</option>)}
          </select>{problem ? <span className="form-error" role="note">{problem}</span> : null}</label>;
      })}
    </div>
  </div>;
}
