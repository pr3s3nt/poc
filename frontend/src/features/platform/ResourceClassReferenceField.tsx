import { offeredClasses } from '../../shared/classChoices';
import { t } from '../../shared/i18n';
import type { TypeContract } from './moduleMapping';
import { parseSource, referencePlaceholder } from './valueSource';

// Same class metadata as namespace provider bindings. The stored placeholder
// remains the source of truth; opening a form never changes its scope or pin.
export function ResourceClassReferenceField({ label, type, resourceType, output, value, onChange, disabled }: {
  label: string;
  type: TypeContract | undefined;
  resourceType: string;
  output: string;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}) {
  const source = parseSource(value);
  const modeled = source.kind === 'resource' && source.ref.type === resourceType && source.ref.output === output && source.ref.scope.kind === 'environment';
  const selected = modeled ? source.ref.class : value;
  const classes = type ? offeredClasses(type, 'default') : [];
  const missing = modeled && !classes.includes(selected);
  return <>
    <label>{label}<select value={selected} disabled={disabled} aria-required="true" onChange={(event) => onChange(event.target.value === '' ? '' : referencePlaceholder({ type: resourceType, class: event.target.value, scope: { kind: 'environment' }, output }))}>
      <option value="">{t('platform.infrastructureConnection.notSelected')}</option>
      {missing ? <option value={selected}>{t('platform.infrastructureConnection.clusterClassNoLongerInMetadata', { clusterClass: selected })}</option> : null}
      {!modeled && value ? <option value={value}>{t('platform.valueSource.advancedExpression')}</option> : null}
      {classes.map((className) => <option key={className} value={className}>{className === 'default' ? t('platform.valueSourceDialog.defaultDefault') : className}</option>)}
    </select></label>
    {value ? <details className="definition-help"><summary>{t('platform.valueSourceDialog.generatedExpressionTechnical')}</summary><code>{value}</code></details> : null}
  </>;
}
