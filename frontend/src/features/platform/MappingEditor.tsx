import { Button } from '../../shared/ui/Button';
import type { Capability } from './capabilities';
import { platformOutput, inputCompatible, outputCompatible, sameNameInputs, sameNameOutputs, type MappingDraft, type MappingProblem, type TypeContract } from './moduleMapping';
import { t } from '../../shared/i18n';

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
  if (!type) return <p className="feature-note" role="status">{t('platform.mappingEditor.waitingForTheResourceType')}</p>;
  const usedVariables = (except: string) => new Set(Object.entries(draft.inputs).filter(([name, variable]) => name !== except && variable !== '').map(([, variable]) => variable));
  const usedOutputs = (except: string) => new Set(Object.entries(draft.outputs).filter(([name, output]) => name !== except && output !== '').map(([, output]) => output));
  const setInput = (name: string, variable: string) => { const inputs = { ...draft.inputs }; if (variable === '') delete inputs[name]; else inputs[name] = variable; onChange({ ...draft, inputs }); };
  const setOutput = (name: string, output: string) => { const outputs = { ...draft.outputs }; if (output === '') delete outputs[name]; else outputs[name] = output; onChange({ ...draft, outputs }); };
  return <div className="mapping-editor">
    <div className="mapping-group" aria-label={t('platform.mappingEditor.inputMapping')}>
      <div className="section-header"><div><h4>{t('platform.mappingEditor.inputMapping')}</h4><details className="definition-help"><summary>{t('platform.mappingEditor.howInputMappingWorks')}</summary><p className="feature-note">{t('platform.mappingEditor.eachParameterOfTheResource')}</p></details></div>
        {type.inputs.length > 0 ? <Button type="button" tone="quiet" disabled={disabled} onClick={() => onChange({ ...draft, inputs: sameNameInputs(capability, type) })}>{t('platform.mappingEditor.matchInputsByIdenticalName')}</Button> : null}</div>
      {type.inputs.length === 0 ? <p className="feature-note">{t('platform.mappingEditor.thisResourceTypeDeclaresNo')}</p> : type.inputs.map((input) => {
        const taken = usedVariables(input.name);
        const options = capability.variables.filter((variable) => inputCompatible(input.type, variable.type) && !taken.has(variable.name));
        const problem = problemFor(`input:${input.name}`);
        return <label key={input.name} className="mapping-row mapping-input-row"><span className="mapping-key"><strong>{input.name}</strong><small>{input.type || 'any'}{input.required ? t('platform.mappingEditor.required') : t('platform.mappingEditor.optional')}</small></span><span className="mapping-direction" aria-hidden="true">→</span>
          <select aria-label={t('platform.mappingEditor.moduleVariableReceivingParameterName', { name: input.name })} aria-invalid={problem ? true : undefined} value={draft.inputs[input.name] ?? ''} disabled={disabled} onChange={(event) => setInput(input.name, event.target.value)}>
            <option value="">{t('platform.mappingEditor.notSelected')}</option>
            {draft.inputs[input.name] && !options.some((variable) => variable.name === draft.inputs[input.name]) ? <option value={draft.inputs[input.name]}>{t('platform.mappingEditor.valueNoLongerValid', { value: draft.inputs[input.name] })}</option> : null}
            {options.map((variable) => <option key={variable.name} value={variable.name}>{variable.name} ({variable.type})</option>)}
          </select>{problem ? <span className="form-error" role="note">{problem}</span> : null}</label>;
      })}
    </div>
    <div className="mapping-group" aria-label={t('platform.mappingEditor.outputMapping')}>
      <div className="section-header"><div><h4>{t('platform.mappingEditor.outputMapping')}</h4><details className="definition-help"><summary>{t('platform.mappingEditor.howOutputMappingWorks')}</summary><p className="feature-note">{t('platform.mappingEditor.allRequiredOutputsMustBe')}</p></details></div>
        <Button type="button" tone="quiet" disabled={disabled} onClick={() => onChange({ ...draft, outputs: sameNameOutputs(capability, type) })}>{t('platform.mappingEditor.matchOutputsByIdenticalName')}</Button></div>
      {type.outputs.map((output) => {
        if (platformOutput(capability, output.name)) return <p key={output.name} className="feature-note">Output <code>{output.name}</code> {' '}{t('platform.mappingEditor.isProvidedByThePlatform')}</p>;
        const taken = usedOutputs(output.name);
        const options = capability.outputs.filter((candidate) => outputCompatible(output.type, candidate.type) && !taken.has(candidate.name));
        const problem = problemFor(`output:${output.name}`);
        return <label key={output.name} className="mapping-row mapping-output-row"><span className="mapping-key"><strong>{output.name}</strong><small>{output.type || 'any'}{output.required ? t('platform.mappingEditor.required') : t('platform.mappingEditor.optional')}{output.secret ? t('platform.mappingEditor.secretValue') : ''}</small></span><span className="mapping-direction" aria-hidden="true">←</span>
          <select aria-label={t('platform.mappingEditor.moduleOutputProvidingName', { name: output.name })} aria-invalid={problem ? true : undefined} value={draft.outputs[output.name] ?? ''} disabled={disabled} onChange={(event) => setOutput(output.name, event.target.value)}>
            <option value="">{t('platform.mappingEditor.notSelected')}</option>
            {draft.outputs[output.name] && !options.some((candidate) => candidate.name === draft.outputs[output.name]) ? <option value={draft.outputs[output.name]}>{t('platform.mappingEditor.valueNoLongerValid', { value: draft.outputs[output.name] })}</option> : null}
            {options.map((candidate) => <option key={candidate.name} value={candidate.name}>{candidate.name}{candidate.type ? ` (${candidate.type})` : ''}{candidate.sensitive ? t('platform.mappingEditor.secret') : ''}</option>)}
          </select>{problem ? <span className="form-error" role="note">{problem}</span> : null}</label>;
      })}
    </div>
  </div>;
}
