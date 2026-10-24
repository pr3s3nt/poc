import { useState } from 'react';
import type { CapabilityVariable } from './capabilities';
import type { TypeContract } from './moduleMapping';
import { entryValue, type VariableDraft } from './typedVariables';
import { describeSource, parseSource } from './valueSource';
import { ValueSourceDialog, type InheritChoice } from './ValueSourceDialog';
import { t } from '../../shared/i18n';

// Parameter cards of the verified module (UC-03 UI11-02/03): each card shows where the value comes from and opens the shared
// source chooser. Only an explicit choice writes an override; omitted variables keep their default or mapped param.

export type SourceCardProps = {
  name: string; type: string; required: boolean; label?: string;
  raw: string | undefined;
  // What applies while no override exists (module default, mapped param or nothing).
  inherited: { origin: string; value: string; choice?: InheritChoice; note?: string; needsValue?: boolean; applies?: boolean };
  types: TypeContract[];
  disabled?: boolean;
  onChange: (raw: string | undefined) => void;
};

export function SourceCard({ name, type, required, label, raw, inherited, types, disabled = false, onChange }: SourceCardProps) {
  const [open, setOpen] = useState(false);
  const shown = raw !== undefined ? describeSource(parseSource(raw), types) : { origin: inherited.origin, value: inherited.value };
  const check = raw !== undefined ? entryValue({ name, type, required: false, hasDefault: false, resourceParam: false }, { on: true, raw }) : {};
  const parsed = raw !== undefined ? parseSource(raw) : undefined;
  const needsValue = raw === undefined && inherited.needsValue === true;
  // “Thay đổi” only when some value already applies (an override, a mapped param or a module default).
  const action = raw === undefined && inherited.applies !== true ? t('platform.parameterSources.provideValue') : t('platform.parameterSources.change');
  return <div className={`source-card${needsValue ? ' needs-value' : ''}`} data-variable={name}>
    <div className="source-card-title"><div><strong>{label ?? name}</strong>{label ? <code className="variable-code">{name}</code> : null} <span className="status-pill">{type || 'any'}</span></div>
      <button type="button" className="button button-quiet source-action" aria-label={`${action} ${name}`} disabled={disabled} onClick={() => setOpen(true)}>{action}</button></div>
    <span className="source-origin">{shown.origin}{required ? t('platform.parameterSources.required') : ''}</span>
    <div className="source-value">{shown.value}</div>
    {raw !== undefined && inherited.note ? <small className="muted">{inherited.note}</small> : null}
    {raw === undefined && inherited.note ? <small className="muted">{inherited.note}</small> : null}
    {check.error ? <div className="form-error" role="note">{check.error}</div> : null}
    {parsed && parsed.kind !== 'literal' ? <details className="source-technical"><summary>{t('platform.parameterSources.technicalExpression')}</summary><code>{raw}</code></details> : null}
    {open ? <ValueSourceDialog target={{ name, type, label: label ?? name }} raw={raw} types={types} inherit={inherited.choice}
      onCancel={() => setOpen(false)} onApply={(next) => { setOpen(false); onChange(next === null ? undefined : next); }} /> : null}
  </div>;
}

const defaultText = (variable: CapabilityVariable) => (typeof variable.default === 'string' ? variable.default : JSON.stringify(variable.default));

function inheritedFor(variable: CapabilityVariable, mappedParam: string | undefined, covered: boolean): SourceCardProps['inherited'] {
  if (mappedParam) {
    return { origin: t('platform.parameterSources.applicationProvidedParameter'), value: `${mappedParam} → ${variable.name}`, note: t('platform.parameterSources.theApplicationParameterTakesPrecedence'),
      choice: { label: t('platform.parameterSources.useTheApplicationParameterMappedParam', { mappedParam }), description: t('platform.parameterSources.noFallbackValueIsSent') }, applies: true, needsValue: !covered && variable.required && !variable.hasDefault };
  }
  if (variable.hasDefault) return { origin: t('platform.parameterSources.moduleDefault'), value: defaultText(variable) ?? '', choice: { label: t('platform.parameterSources.useTheModuleDefault'), description: t('platform.parameterSources.noValueSentTheModule') }, applies: true };
  if (!variable.required) return { origin: t('platform.parameterSources.notProvided'), value: t('platform.parameterSources.optionalLeaveEmptyToSend'), choice: { label: t('platform.parameterSources.doNotProvide'), description: t('platform.parameterSources.noValueIsSentFor') } };
  return { origin: t('platform.parameterSources.notProvided'), value: t('platform.parameterSources.chooseAValueOrSource'), needsValue: true };
}

type Props = {
  variables: CapabilityVariable[];
  draft: VariableDraft;
  onChange: (draft: VariableDraft) => void;
  executorOwned: string[];
  types: TypeContract[];
  // Explicit input mapping (variable -> Resource Type input) and the variables it feeds from a required input.
  mapped: Record<string, string>;
  covered: ReadonlySet<string>;
  disabled?: boolean;
};

export function ParameterSources({ variables, draft, onChange, executorOwned, types, mapped, covered, disabled = false }: Props) {
  const rawOf = (name: string): string | undefined => (draft[name]?.on ? draft[name].raw : undefined);
  const set = (variable: CapabilityVariable, raw: string | undefined) => {
    const next = { ...draft };
    if (raw === undefined) delete next[variable.name]; else next[variable.name] = { on: true, raw };
    onChange(next);
  };
  const card = (variable: CapabilityVariable) => <SourceCard key={variable.name} name={variable.name} type={variable.type} required={variable.required && !variable.hasDefault && !covered.has(variable.name)}
    raw={rawOf(variable.name)} inherited={inheritedFor(variable, mapped[variable.name], covered.has(variable.name))} types={types} disabled={disabled} onChange={(raw) => set(variable, raw)} />;
  if (variables.length === 0 && executorOwned.length === 0) return <p className="feature-note">{t('platform.parameterSources.thisCreationMethodHasNo')}</p>;
  // A required parameter without a default, a mapped one and every override are shown first; the rest stays collapsed.
  const primary = variables.filter((variable) => draft[variable.name]?.on === true || mapped[variable.name] !== undefined || (variable.required && !variable.hasDefault));
  const rest = variables.filter((variable) => !primary.includes(variable));
  const defaults = rest.filter((variable) => variable.hasDefault);
  const optional = rest.filter((variable) => !variable.hasDefault);
  const unresolved = primary.filter((variable) => rawOf(variable.name) === undefined && inheritedFor(variable, mapped[variable.name], covered.has(variable.name)).needsValue).length;
  return <div className="source-groups">
    {primary.length > 0 ? <div className="source-group"><p className="source-group-label">{t('platform.parameterSources.needsAValueOrIs')}{' '} <span className="status-pill">{primary.length}</span>{unresolved > 0 ? <span className="status-pill status-warning">{t('platform.parameterSources.unresolvedWithoutASource', { unresolved })}</span> : null}</p>{primary.map(card)}</div> : null}
    {defaults.length > 0 ? <details className="source-collapsed"><summary>{t('platform.parameterSources.settingsWithDefaults')}{' '} <span className="status-pill">{defaults.length}</span></summary><p className="feature-note">{t('platform.parameterSources.youCanKeepOrChange')}</p>{defaults.map(card)}</details> : null}
    {optional.length > 0 ? <details className="source-collapsed"><summary>{t('platform.parameterSources.optionalSettings')}{' '} <span className="status-pill">{optional.length}</span></summary><p className="feature-note">{t('platform.parameterSources.whenEmptyTheSystemSends')}</p>{optional.map(card)}</details> : null}
    {executorOwned.length > 0 ? <details className="source-collapsed"><summary>{t('platform.parameterSources.providedByTheSystem')}{' '} <span className="status-pill">{executorOwned.length}</span></summary><p className="feature-note">{t('platform.parameterSources.noInputField')}{' '} {executorOwned.map((name) => <code key={name}>{name} </code>)}</p></details> : null}
  </div>;
}
