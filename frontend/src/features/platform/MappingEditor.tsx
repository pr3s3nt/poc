import { Button } from '../../shared/ui/Button';
import type { Capability } from './capabilities';
import { bindingProviders, inputCompatible, outputCompatible, sameNameInputs, sameNameOutputs, type BindingDraft, type MappingDraft, type MappingProblem, type TypeContract } from './moduleMapping';

type Props = {
  capability: Capability;
  type: TypeContract | undefined;
  clusterClasses: string[];
  draft: MappingDraft;
  onChange: (draft: MappingDraft) => void;
  problems: MappingProblem[];
  disabled?: boolean;
};

const bindingLabel = { aws: 'Kết nối AWS của Môi trường', kubernetes: 'Cluster Kubernetes' } as const;

// Typed controls of input_mapping / output_mapping / provider_bindings (UC-03
// BR-11). Choices come only from the verified module metadata and the Resource
// Type contract; "Ghép theo tên trùng" is an explicit action, never a default.
export function MappingEditor({ capability, type, clusterClasses, draft, onChange, problems, disabled = false }: Props) {
  const problemFor = (field: string) => problems.find((problem) => problem.field === field)?.message;
  if (!type) return <p className="feature-note" role="status">Đang chờ hợp đồng của Loại tài nguyên để ánh xạ.</p>;
  const usedVariables = (except: string) => new Set(Object.entries(draft.inputs).filter(([name, variable]) => name !== except && variable !== '').map(([, variable]) => variable));
  const usedOutputs = (except: string) => new Set(Object.entries(draft.outputs).filter(([name, output]) => name !== except && output !== '').map(([, output]) => output));
  const setInput = (name: string, variable: string) => { const inputs = { ...draft.inputs }; if (variable === '') delete inputs[name]; else inputs[name] = variable; onChange({ ...draft, inputs }); };
  const setOutput = (name: string, output: string) => { const outputs = { ...draft.outputs }; if (output === '') delete outputs[name]; else outputs[name] = output; onChange({ ...draft, outputs }); };
  const setBinding = (name: string, patch: Partial<BindingDraft>) => onChange({ ...draft, bindings: { ...draft.bindings, [name]: { target: '', clusterClass: '', ...draft.bindings[name], ...patch } } });
  const providers = bindingProviders(capability);
  return <div className="mapping-editor">
    <div className="mapping-group" aria-label="Ánh xạ đầu vào">
      <div className="section-header"><div><h4>Đầu vào: tham số của Loại → biến module</h4><p className="feature-note">Mỗi tham số của Loại tài nguyên phải được nhận bởi một biến module. Tham số thật của workload luôn được ưu tiên hơn giá trị dự phòng của cấu hình và mặc định của module.</p></div>
        {type.inputs.length > 0 ? <Button type="button" tone="quiet" disabled={disabled} onClick={() => onChange({ ...draft, inputs: sameNameInputs(capability, type) })}>Ghép đầu vào theo tên trùng</Button> : null}</div>
      {type.inputs.length === 0 ? <p className="feature-note">Loại tài nguyên này không khai báo tham số đầu vào.</p> : type.inputs.map((input) => {
        const taken = usedVariables(input.name);
        const options = capability.variables.filter((variable) => inputCompatible(input.type, variable.type) && !taken.has(variable.name));
        const problem = problemFor(`input:${input.name}`);
        return <label key={input.name} className="mapping-row">Tham số {input.name} ({input.type || 'any'}{input.required ? ', bắt buộc' : ''}) → biến module
          <select aria-label={`Biến module nhận tham số ${input.name}`} aria-invalid={problem ? true : undefined} value={draft.inputs[input.name] ?? ''} disabled={disabled} onChange={(event) => setInput(input.name, event.target.value)}>
            <option value="">Chưa chọn</option>
            {draft.inputs[input.name] && !options.some((variable) => variable.name === draft.inputs[input.name]) ? <option value={draft.inputs[input.name]}>{draft.inputs[input.name]} (không còn hợp lệ)</option> : null}
            {options.map((variable) => <option key={variable.name} value={variable.name}>{variable.name} ({variable.type})</option>)}
          </select>{problem ? <span className="form-error" role="note">{problem}</span> : null}</label>;
      })}
    </div>
    <div className="mapping-group" aria-label="Ánh xạ đầu ra">
      <div className="section-header"><div><h4>Đầu ra: output module → output của Loại</h4><p className="feature-note">Phải đủ các output bắt buộc, đúng kiểu. Cờ bí mật của Loại hoặc module được giữ; không thể hạ cờ bí mật khi ánh xạ.</p></div>
        <Button type="button" tone="quiet" disabled={disabled} onClick={() => onChange({ ...draft, outputs: sameNameOutputs(capability, type) })}>Ghép đầu ra theo tên trùng</Button></div>
      {type.outputs.map((output) => {
        const taken = usedOutputs(output.name);
        const options = capability.outputs.filter((candidate) => outputCompatible(output.type, candidate.type) && !taken.has(candidate.name));
        const problem = problemFor(`output:${output.name}`);
        return <label key={output.name} className="mapping-row">Output {output.name} ({output.type || 'any'}{output.required ? ', bắt buộc' : ''}{output.secret ? ', bí mật' : ''}) ← output module
          <select aria-label={`Output module cung cấp ${output.name}`} aria-invalid={problem ? true : undefined} value={draft.outputs[output.name] ?? ''} disabled={disabled} onChange={(event) => setOutput(output.name, event.target.value)}>
            <option value="">Chưa chọn</option>
            {draft.outputs[output.name] && !options.some((candidate) => candidate.name === draft.outputs[output.name]) ? <option value={draft.outputs[output.name]}>{draft.outputs[output.name]} (không còn hợp lệ)</option> : null}
            {options.map((candidate) => <option key={candidate.name} value={candidate.name}>{candidate.name}{candidate.type ? ` (${candidate.type})` : ''}{candidate.sensitive ? ' · bí mật' : ''}</option>)}
          </select>{problem ? <span className="form-error" role="note">{problem}</span> : null}</label>;
      })}
    </div>
    <div className="mapping-group" aria-label="Provider binding">
      <div className="section-header"><div><h4>Provider binding</h4><p className="feature-note">Không có ô nhập Kết nối, tài khoản, vùng, endpoint hay credential: mọi provider dùng đích của Môi trường khi Xem trước hoặc Triển khai.</p></div></div>
      {providers.length === 0 ? <p className="feature-note">Module này không cần provider binding.</p> : providers.map((provider) => {
        const binding = draft.bindings[provider.name] ?? { target: '', clusterClass: '' };
        const problem = problemFor(`binding:${provider.name}`);
        return <div key={provider.name} className="mapping-row" data-provider={provider.name}>
          <strong>{provider.name}</strong> <span className="status-pill">{provider.source} {provider.version}</span>
          {provider.binding === 'aws' ? <p className="feature-note" role="note">{bindingLabel.aws}. Tài khoản và vùng lấy từ Môi trường khi Xem trước/Triển khai; cấu hình không chọn tài khoản riêng, kể cả Aurora.</p> : <>
            <label>Cluster đích của {provider.name}
              <select aria-label={`Cluster đích của provider ${provider.name}`} aria-invalid={problem ? true : undefined} value={binding.target} disabled={disabled} onChange={(event) => setBinding(provider.name, { target: event.target.value as BindingDraft['target'] })}>
                <option value="">Chưa chọn</option>
                <option value="environment">Cluster của Môi trường (Kết nối Kubernetes của Môi trường)</option>
                <option value="resource">Cluster do node EKS cung cấp</option>
              </select></label>
            {binding.target === 'resource' ? <label>Class cluster EKS
              <select aria-label={`Class cluster EKS của provider ${provider.name}`} value={binding.clusterClass} disabled={disabled} onChange={(event) => setBinding(provider.name, { clusterClass: event.target.value })}>
                <option value="">Chưa chọn</option>
                {binding.clusterClass && !clusterClasses.includes(binding.clusterClass) ? <option value={binding.clusterClass}>{binding.clusterClass} (không còn trong metadata)</option> : null}
                {clusterClasses.map((item) => <option key={item} value={item}>{item}</option>)}
              </select></label> : null}
            {binding.target === 'resource' && clusterClasses.length === 0 ? <p className="feature-note" role="note">Danh mục chưa công bố class nào cho Loại k8s-cluster; chưa chọn được cluster EKS.</p> : null}
            {problem ? <span className="form-error" role="note">{problem}</span> : null}</>}
        </div>;
      })}
    </div>
  </div>;
}
