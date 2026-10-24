import { useEffect, useId, useRef, useState, type KeyboardEvent } from 'react';
import { Button } from '../../shared/ui/Button';
import type { TypeContract } from './moduleMapping';
import { baseKind, entryValue } from './typedVariables';
import { classChoices, compatibleOutputs, contextAllowed, contextChoices, contextPlaceholder, defaultClass, environmentScopeId, infraScopeId, parseSource, referencePlaceholder, referenceTypes, scopeFromId, type ResourceReference, type ValueSource } from './valueSource';
import { t } from '../../shared/i18n';

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

const kindLabels = { string: t('platform.valueSourceDialog.string'), number: t('platform.valueSourceDialog.number'), bool: t('platform.valueSourceDialog.trueFalse'), list: t('platform.valueSourceDialog.list'), map: t('platform.valueSourceDialog.keyValueMap'), any: t('platform.valueSourceDialog.anyValue') } as const;
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
  else if (ref.type === '') blocker = t('platform.valueSourceDialog.chooseAResourceType');
  else if (ref.class === '') blocker = t('platform.valueSourceDialog.chooseAClass');
  else if (ref.scope.kind === 'resource' && ref.scope.id.trim() === '') blocker = t('platform.valueSourceDialog.enterAResourceID');
  else if (ref.output === '' || !outputs.some((output) => output.name === ref.output)) blocker = outputs.length === 0 ? t('platform.valueSourceDialog.thisResourceTypeHasNo') : t('platform.valueSourceDialog.chooseTheOutputToUse');
  else result = referencePlaceholder(ref);

  const setRefPart = (patch: Partial<ResourceReference>) => setRef((current) => ({ ...current, ...patch }));
  const option = (value: Mode, title: string, hint: string) => <label className={`value-option${mode === value ? ' value-option-active' : ''}`}><input type="radio" name={`${heading}-mode`} aria-label={title} checked={mode === value} onChange={() => setMode(value)} /><span><strong>{title}</strong><small>{hint}</small></span></label>;
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => { if (event.key === 'Escape') { event.stopPropagation(); onCancel(); } };

  return <div className="dialog-backdrop">
    <div className="dialog value-dialog" role="dialog" aria-modal="true" aria-labelledby={heading} tabIndex={-1} ref={dialog} onKeyDown={onKeyDown}>
      <div className="value-dialog-head"><div><h2 id={heading}>{t('platform.valueSourceDialog.chooseValueSource')}</h2><p className="muted">{target.label}</p></div><button type="button" className="button button-quiet" aria-label={t('platform.valueSourceDialog.close')} onClick={onCancel}>×</button></div>
      <div className="value-options" role="radiogroup" aria-label={t('platform.valueSourceDialog.valueSource')}>
        {inherit ? option('inherit', inherit.label, inherit.description) : null}
        {option('literal', t('platform.valueSourceDialog.enterAValue'), t('platform.valueSourceDialog.enterAsValue', { value: kindLabels[kind] }))}
        {allowContext ? option('context', t('platform.valueSourceDialog.fromTheDeploymentContext'), t('platform.valueSourceDialog.theSystemProvidesTheValue')) : null}
        {option('resource', t('platform.valueSourceDialog.fromAnotherResource'), t('platform.valueSourceDialog.chooseTheResourceTypeClass'))}
        {initial?.kind === 'advanced' ? option('advanced', t('platform.valueSourceDialog.keepAdvancedExpression'), t('platform.valueSourceDialog.thisExpressionCannotBeRepresented')) : null}
      </div>
      {mode === 'literal' ? <div className="value-body">
        {kind === 'bool' ? <label>{t('platform.valueSourceDialog.value')}{' '} {target.name}<select aria-label={t('platform.valueSourceDialog.valueOfName', { name: target.name })} value={literal === 'true' || literal === 'false' ? literal : ''} onChange={(event) => setLiteral(event.target.value)}><option value="">{t('platform.valueSourceDialog.chooseAValue')}</option><option value="true">true</option><option value="false">false</option></select></label>
          : kind === 'string' || kind === 'number' ? <label>{t('platform.valueSourceDialog.value')}{' '} {target.name} ({kindLabels[kind]})<input aria-label={t('platform.valueSourceDialog.valueOfName', { name: target.name })} inputMode={kind === 'number' ? 'decimal' : undefined} value={literal} spellCheck={false} aria-invalid={literalCheck.error ? true : undefined} onChange={(event) => setLiteral(event.target.value)} /></label>
            : <label>{t('platform.valueSourceDialog.value')}{' '} {target.name} ({kindLabels[kind]}, JSON)<textarea aria-label={t('platform.valueSourceDialog.valueOfName', { name: target.name })} rows={kind === 'any' ? 3 : 4} value={literal} spellCheck={false} aria-invalid={literalCheck.error ? true : undefined} onChange={(event) => setLiteral(event.target.value)} /></label>}
      </div> : null}
      {mode === 'context' ? <div className="value-body"><label>{t('platform.valueSourceDialog.contextValue')}<select aria-label={t('platform.valueSourceDialog.contextValue')} value={contextPath} onChange={(event) => setContextPath(event.target.value)}>{contextChoices.map((choice) => <option key={choice.path} value={choice.path}>{choice.label}</option>)}</select></label></div> : null}
      {mode === 'resource' ? <div className="value-body value-grid">
        <label>{t('platform.valueSourceDialog.resourceType')}<select aria-label={t('platform.valueSourceDialog.sourceResourceType')} value={ref.type} onChange={(event) => setRef({ ...ref, type: event.target.value, output: '', class: defaultClass })}>
          <option value="">{t('platform.valueSourceDialog.chooseAResourceType2')}</option>
          {ref.type !== '' && !types.some((type) => type.key === ref.type) ? <option value={ref.type}>{t('platform.valueSourceDialog.typeNoLongerInMetadata', { type: ref.type })}</option> : null}
          {referenceTypes(types).map((type) => <option key={type.key} value={type.key}>{type.key}</option>)}</select></label>
        <label>{t('platform.valueSourceDialog.class')}<select aria-label={t('platform.valueSourceDialog.sourceClass')} value={ref.class} onChange={(event) => setRefPart({ class: event.target.value })}>{classChoices(refType, ref.class).map((item) => <option key={item} value={item}>{item === defaultClass ? t('platform.valueSourceDialog.defaultDefault') : item}</option>)}</select></label>
        <label className="value-wide">{t('platform.valueSourceDialog.scope')}<select aria-label={t('platform.valueSourceDialog.sourceScope')} value={scopeValue} onChange={(event) => setRefPart({ scope: event.target.value === 'resource' ? { kind: 'resource', id: '' } : scopeFromId(event.target.value) })}>
          <option value={environmentScopeId}>{t('platform.valueSourceDialog.sameApplicationAndEnvironment')}</option><option value={infraScopeId}>{t('platform.valueSourceDialog.infrastructureContext')}</option><option value="resource">{t('platform.valueSourceDialog.specificResourceID')}</option></select></label>
        {ref.scope.kind === 'resource' ? <label className="value-wide">{t('platform.valueSourceDialog.resourceID')}<input aria-label={t('platform.valueSourceDialog.sourceResourceID')} value={ref.scope.id} spellCheck={false} onChange={(event) => setRefPart({ scope: { kind: 'resource', id: event.target.value } })} /></label> : null}
        <label className="value-wide">{t('platform.valueSourceDialog.outputToUse')}<select aria-label={t('platform.valueSourceDialog.outputToUse')} value={ref.output} disabled={ref.type === ''} onChange={(event) => setRefPart({ output: event.target.value })}>
          <option value="">{t('platform.valueSourceDialog.chooseAnOutput')}</option>
          {outputs.map((output) => <option key={output.name} value={output.name}>{output.name}{output.type ? ` · ${output.type}` : ''}</option>)}</select></label>
        <p className="feature-note">{t('platform.valueSourceDialog.listsOnlyPublicNonSecret')}</p>
      </div> : null}
      {mode === 'advanced' && raw !== undefined ? <div className="value-body"><code>{raw}</code></div> : null}
      {mode === 'resource' && result ? <details className="value-technical"><summary>{t('platform.valueSourceDialog.generatedExpressionTechnical')}</summary><code>{result}</code></details> : null}
      {blocker ? <div className="form-error" role="note">{blocker}</div> : null}
      <div className="form-actions"><Button type="button" onClick={onCancel}>{t('platform.valueSourceDialog.cancel')}</Button><Button type="button" tone="primary" disabled={result === undefined} onClick={() => { if (result !== undefined) onApply(result); }}>{t('platform.valueSourceDialog.apply')}</Button></div>
    </div>
  </div>;
}
