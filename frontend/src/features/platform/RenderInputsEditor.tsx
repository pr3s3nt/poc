import { useState } from 'react';
import { Button } from '../../shared/ui/Button';
import { checkValue, displayValue, inputFields, inputProblems, unknownInputs, type InputField, type InputProblem } from './renderInputs';
import type { JsonObject } from './typedVariables';
import { t } from '../../shared/i18n';

// Schema-driven render_inputs form (UC-03 RB-03). An untouched field is
// omitted and shows the bundle default as a hint; "Dùng mặc định" removes an
// explicit value again. false, 0 and "" are explicit values. Numbers and JSON
// fields keep their raw text until it parses, so nothing is coerced or lost.
type Props = { schema: JsonObject | undefined; value: JsonObject; onChange: (next: JsonObject) => void; onRawProblems: (count: number) => void; disabled: boolean };

const problemText = (problem: InputProblem) => t(`platform.renderInputs.problem.${problem}`);
const has = (object: JsonObject, key: string) => Object.prototype.hasOwnProperty.call(object, key);

export function RenderInputsEditor({ schema, value, onChange, onRawProblems, disabled }: Props) {
  const fields = inputFields(schema);
  const [raw, setRaw] = useState<Record<string, string>>({});
  const [rawError, setRawError] = useState<Record<string, boolean>>({});
  const problems = inputProblems(schema, value);
  const unknown = unknownInputs(schema, value);

  const set = (name: string, next: unknown) => onChange({ ...value, [name]: next });
  const reset = (name: string) => {
    const next = { ...value };
    delete next[name];
    const nextRaw = { ...raw }; delete nextRaw[name]; setRaw(nextRaw);
    const nextErrors = { ...rawError }; delete nextErrors[name]; setRawError(nextErrors); onRawProblems(Object.keys(nextErrors).length);
    onChange(next);
  };
  const setRawText = (field: InputField, text: string) => {
    setRaw({ ...raw, [field.name]: text });
    let parsed: unknown;
    let ok = true;
    if (field.type === 'integer' || field.type === 'number') {
      parsed = Number(text);
      ok = text.trim() !== '' && Number.isFinite(parsed);
    } else {
      try { parsed = JSON.parse(text); } catch { ok = false; }
    }
    const nextErrors = { ...rawError };
    if (ok) { delete nextErrors[field.name]; set(field.name, parsed); } else nextErrors[field.name] = true;
    setRawError(nextErrors); onRawProblems(Object.keys(nextErrors).length);
  };
  const dropUnknown = () => {
    const next = { ...value };
    for (const key of unknown) delete next[key];
    onChange(next);
  };

  if (fields.length === 0 && unknown.length === 0) return <p className="feature-note">{t('platform.renderInputs.noInputs')}</p>;
  return <fieldset className="catalog-fields" disabled={disabled} aria-label={t('platform.renderInputs.inputs')}>
    <legend>{t('platform.renderInputs.inputs')}</legend>
    {fields.map((field) => {
      const explicit = has(value, field.name);
      const current = explicit ? value[field.name] : undefined;
      const problem = rawError[field.name] ? 'type' : problems[field.name];
      const hintId = `render-input-${field.name}-hint`;
      const hint = field.hasDefault ? t('platform.renderInputs.defaultHint', { value: displayValue(field.defaultValue) }) : field.required ? t('platform.renderInputs.requiredHint') : t('platform.renderInputs.optionalHint');
      let control;
      if (field.type === 'boolean') {
        const checked = explicit ? current === true : field.defaultValue === true;
        control = <label className="checkbox-row"><input type="checkbox" checked={checked} aria-describedby={hintId} onChange={(event) => set(field.name, event.target.checked)} />{field.title}</label>;
      } else if (field.enumValues) {
        const options = field.enumValues.map((option) => JSON.stringify(option));
        control = <label>{field.title}<select value={explicit ? JSON.stringify(current) : ''} aria-describedby={hintId} aria-invalid={problem ? true : undefined} onChange={(event) => { if (event.target.value === '') reset(field.name); else set(field.name, JSON.parse(event.target.value) as unknown); }}>
          <option value="">{field.hasDefault ? t('platform.renderInputs.useDefaultOption', { value: displayValue(field.defaultValue) }) : t('platform.renderInputs.notSetOption')}</option>
          {options.map((option, index) => <option key={option} value={option}>{displayValue(field.enumValues?.[index])}</option>)}
        </select></label>;
      } else if (field.type === 'string') {
        control = <label>{field.title}<input value={typeof current === 'string' ? current : ''} spellCheck={false} placeholder={field.hasDefault ? displayValue(field.defaultValue) : ''} aria-describedby={hintId} aria-invalid={problem ? true : undefined} onChange={(event) => set(field.name, event.target.value)} /></label>;
      } else if (field.type === 'integer' || field.type === 'number') {
        const text = raw[field.name] ?? (explicit ? displayValue(current) : '');
        control = <label>{field.title}<input inputMode={field.type === 'integer' ? 'numeric' : 'decimal'} value={text} spellCheck={false} placeholder={field.hasDefault ? displayValue(field.defaultValue) : ''} aria-describedby={hintId} aria-invalid={problem ? true : undefined} onChange={(event) => setRawText(field, event.target.value)} /></label>;
      } else {
        const text = raw[field.name] ?? (explicit ? JSON.stringify(current, null, 2) : '');
        control = <label>{field.title}<textarea rows={4} spellCheck={false} value={text} placeholder={field.hasDefault ? JSON.stringify(field.defaultValue) : ''} aria-describedby={hintId} aria-invalid={problem ? true : undefined} onChange={(event) => setRawText(field, event.target.value)} /></label>;
      }
      return <div key={field.name} className="render-input" data-input={field.name}>
        {control}
        <p id={hintId} className="feature-note">{explicit ? t('platform.renderInputs.explicitHint') : hint}{field.description ? ` ${field.description}` : ''}</p>
        {explicit ? <Button type="button" tone="quiet" onClick={() => reset(field.name)}>{t('platform.renderInputs.useDefault', { name: field.title })}</Button> : null}
        {problem ? <p className="form-error" role="alert">{problemText(problem)}</p> : null}
      </div>;
    })}
    {unknown.length > 0 ? <div className="form-warning" role="alert" aria-label={t('platform.renderInputs.incompatible')}>
      <p>{t('platform.renderInputs.incompatibleFields', { names: unknown.join(', ') })}</p>
      <Button type="button" onClick={dropUnknown}>{t('platform.renderInputs.dropIncompatible')}</Button>
    </div> : null}
  </fieldset>;
}

/** Problems that block Save: field problems plus unparsed raw text. */
export function renderInputsBlocked(schema: JsonObject | undefined, value: JsonObject, rawProblems: number): boolean {
  if (rawProblems > 0) return true;
  if (Object.keys(inputProblems(schema, value)).length > 0) return true;
  return inputFields(schema).some((field) => has(value, field.name) && checkValue(field.schema, value[field.name]) !== undefined);
}
