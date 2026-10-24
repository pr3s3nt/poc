import { useId, useState } from 'react';
import { CriteriaRules, criterionFields as fields } from './CriteriaRules';
import { Button } from '../../shared/ui/Button';
import { MatchingPreviewPanel, type MatchingCandidateInput } from './MatchingPreviewPanel';
import { t } from '../../shared/i18n';

// Shared "Điều kiện áp dụng" editor (UC-03 T02B, V02-02). It edits the five-field
// Matching Criteria contract and serializes it unchanged; modes are only a
// friendlier projection. There is no profile, account or region predicate and
// the browser never ranks Definitions: only the server matching preview names a
// winner. The draft is controlled by the owner so the editor can serve Resource
// Definition and renderer forms alike.

export type CriterionField = 'env_type' | 'app_id' | 'env_id' | 'res_id' | 'class';
export type Criterion = Record<CriterionField, string>;
export type CriteriaMode = 'all' | 'env_type' | 'app_env' | 'advanced';
export type CriteriaDraft = { mode: CriteriaMode; rows: Criterion[]; fields?: CriterionField[][] };
export type EnvironmentOption = { key: string; name: string; environmentType: string; /** Opaque non-secret target/version fingerprint of the Environment DTO; a change invalidates exact previews. */ snapshotKey?: string };
export type ApplicationOption = { key: string; name: string; environments: EnvironmentOption[] };
// Public Definition metadata that decides a matching result (never secret material); the whole record is part of the preview signature.
export type CriteriaSource = { key: string; name?: string; version?: number; resourceType: string; driverType?: string; driverAccount?: string; driverInputs?: unknown; criteria?: Partial<Criterion>[] };
export type ListState<T> = { items: readonly T[]; loading: boolean; error: string; onRetry(): void };

const modes: { key: CriteriaMode; label: string; hint: string }[] = [
  { key: 'all', label: t('platform.criteriaEditor.everywhere'), hint: t('platform.criteriaEditor.appliesToEveryApplicationAnd') },
  { key: 'env_type', label: t('platform.criteriaEditor.byEnvironmentType'), hint: t('platform.criteriaEditor.appliesOnlyToEnvironmentsOf') },
  { key: 'app_env', label: t('platform.criteriaEditor.byApplicationEnvironment'), hint: t('platform.criteriaEditor.appliesToOneApplicationOptionally') },
  { key: 'advanced', label: t('platform.criteriaEditor.advanced'), hint: t('platform.criteriaEditor.theFiveStandardFieldsMultiple') },
];
const modeLabel = (mode: CriteriaMode) => modes.find((item) => item.key === mode)?.label ?? mode;

export const emptyCriterion = (): Criterion => ({ env_type: '', app_id: '', env_id: '', res_id: '', class: '' });
export const emptyDraft = (): CriteriaDraft => ({ mode: 'all', rows: [emptyCriterion()] });
const isBlank = (row: Criterion) => fields.every(({ key }) => row[key] === '');
const hasData = (rows: Criterion[]) => rows.some((row) => !isBlank(row));
const only = (row: Criterion, allowed: CriterionField[]) => fields.every(({ key }) => allowed.includes(key) || row[key] === '');
const cloneRows = (rows: Criterion[]) => rows.map((row) => ({ ...row }));

// Missing fields of an API criterion are empty; the result never aliases the source.
// Only the five known own fields are read: no prototype keys, no unknown strings.
function normalize(rows: Partial<Criterion>[]): Criterion[] {
  return rows.map((row) => {
    const next = emptyCriterion();
    for (const { key } of fields) { const value = Object.hasOwn(row, key) ? row[key] : undefined; if (typeof value === 'string') next[key] = value; }
    return next;
  });
}

// The simplest mode that shows these rows without losing a field or a row:
// rows are alternatives (OR), so only exactly one row can be simple.
function classify(rows: Criterion[]): CriteriaMode {
  const [row] = rows;
  if (rows.length !== 1 || !row) return 'advanced';
  if (isBlank(row)) return 'all';
  if (row.env_type !== '' && only(row, ['env_type'])) return 'env_type';
  if (row.app_id !== '' && only(row, ['app_id', 'env_id'])) return 'app_env';
  return 'advanced';
}

// A missing list is a wildcard draft; an empty list stays zero advanced rows, never rewritten to a wildcard.
export function draftFromCriteria(criteria: Partial<Criterion>[] | undefined): CriteriaDraft {
  if (criteria === undefined) return emptyDraft();
  const rows = normalize(criteria);
  const mode = classify(rows);
  return { mode, rows: mode === 'all' ? [emptyCriterion()] : rows };
}

// Rows for a target mode. Simple modes keep only the fields they can show.
function convert(rows: Criterion[], mode: CriteriaMode): Criterion[] {
  const first = rows[0] ?? emptyCriterion();
  if (mode === 'advanced') return cloneRows(rows);
  if (mode === 'env_type') return [{ ...emptyCriterion(), env_type: first.env_type }];
  if (mode === 'app_env') return [{ ...emptyCriterion(), app_id: first.app_id, env_id: first.env_id }];
  return [emptyCriterion()];
}

// Leaving advanced is free only when exactly one row maps onto the target without dropping a field.
function lossless(rows: Criterion[], mode: CriteriaMode): boolean {
  const [row] = rows;
  if (rows.length !== 1 || !row) return false;
  return isBlank(row) || classify(rows) === mode;
}

// Wire payload: wildcard is [{}], empty fields are dropped; an incomplete simple mode is a problem, never a silent wildcard.
export function draftToCriteria(draft: CriteriaDraft): { criteria: Record<string, string>[]; problem: string } {
  if (draft.fields) {
    const incomplete = draft.fields.some((keys, index) => keys.some((key) => draft.rows[index]?.[key] === ''));
    if (incomplete) return { criteria: [], problem: t('platform.criteriaEditor.enterAValueForEach') };
  }
  const clean = (rows: Criterion[]) => rows.map((row) => Object.fromEntries(Object.entries(row).filter(([, value]) => value !== '')) as Record<string, string>);
  const [row = emptyCriterion()] = draft.rows;
  if (draft.mode === 'all') return { criteria: [{}], problem: '' };
  if (draft.mode === 'env_type') return row.env_type === '' ? { criteria: [], problem: t('platform.criteriaEditor.enterTheEnvironmentTypeFor') } : { criteria: clean([{ ...emptyCriterion(), env_type: row.env_type }]), problem: '' };
  if (draft.mode === 'app_env') return row.app_id === '' ? { criteria: [], problem: t('platform.criteriaEditor.chooseTheApplicationForThe') } : { criteria: clean([{ ...emptyCriterion(), app_id: row.app_id, env_id: row.env_id }]), problem: '' };
  return draft.rows.length === 0 ? { criteria: [], problem: t('platform.criteriaEditor.addAtLeastOneCondition') } : { criteria: clean(draft.rows), problem: '' };
}

const canOverlap = (a: Record<string, string>, b: Record<string, string>) => fields.every(({ key }) => !a[key] || !b[key] || a[key] === b[key]);

function describeRow(row: Record<string, string>, applications: readonly ApplicationOption[]): string {
  const parts: string[] = [];
  if (row.env_type) parts.push(t('platform.criteriaEditor.environmentTypeEnvTypeEnv', { env_type: row.env_type }));
  if (row.app_id) { const app = applications.find((item) => item.key === row.app_id); parts.push(t('platform.criteriaEditor.applicationAppIdValue', { value: app && app.name !== app.key ? `${app.name} (${app.key})` : row.app_id })); }
  if (row.env_id) parts.push(t('platform.criteriaEditor.environmentEnvIdEnvId', { env_id: row.env_id }));
  if (row.res_id) parts.push(t('platform.criteriaEditor.resourceResIdResId', { res_id: row.res_id }));
  if (row.class) parts.push(`Class: ${row.class}`);
  return parts.length === 0 ? t('platform.criteriaEditor.everywhereNoLimit') : parts.join(' · ');
}

type Pending = { kind: 'mode'; target: CriteriaMode } | { kind: 'copy'; source: string; draft: CriteriaDraft };

export function CriteriaEditor({ draft, onChange, applications, sources, resourceType = '', matching, scopeKey = '', excludeKey = '', rules = false }: {
  rules?: boolean;
  draft: CriteriaDraft;
  onChange(next: CriteriaDraft): void;
  applications: ListState<ApplicationOption>;
  /** Other definitions: copy source and overlap candidates. */
  sources: ListState<CriteriaSource>;
  /** Resource Type of the form being edited. */
  resourceType?: string;
  /** Full unsaved candidate for the exact server-side matching preview (T19); omitted = panel hidden. */
  matching?: MatchingCandidateInput;
  /** Session/scope identity: a change invalidates the exact preview result. */
  scopeKey?: string;
  /** The Definition being edited: never its own copy or overlap source. */
  excludeKey?: string;
}) {
  const id = useId();
  const [pending, setPending] = useState<Pending | null>(null);
  const [copyFrom, setCopyFrom] = useState('');
  const [sample, setSample] = useState({ app: '', env: '', resId: '', cls: '' });
  const rows = draft.rows;
  const [row = emptyCriterion()] = rows;
  const { criteria, problem } = draftToCriteria(draft);
  const selectedApp = applications.items.find((app) => app.key === row.app_id);
  const knownTypes = [...new Set(applications.items.flatMap((app) => app.environments.map((env) => env.environmentType)).filter(Boolean))].sort();

  const setRows = (next: Criterion[]) => onChange({ mode: draft.mode, rows: next });
  const setRow = (index: number, field: CriterionField, value: string) => setRows(rows.map((item, position) => (position === index ? { ...item, [field]: value } : item)));

  function chooseMode(target: CriteriaMode) {
    if (target === draft.mode) { setPending(null); return; }
    if (draft.mode === 'advanced' && !lossless(rows, target)) { setPending({ kind: 'mode', target }); return; }
    setPending(null);
    onChange({ mode: target, rows: convert(rows, target) });
  }
  function copy() {
    const source = sources.items.find((item) => item.key === copyFrom);
    if (!source) return;
    const next = draftFromCriteria(source.criteria); // normalize() builds new objects: no live link
    if (hasData(rows) || rows.length > 1 || draft.fields?.some((keys) => keys.length > 0)) { setPending({ kind: 'copy', source: source.key, draft: next }); return; }
    setPending(null);
    onChange(next);
  }
  function confirm() {
    if (pending?.kind === 'mode') onChange({ mode: pending.target, rows: convert(rows, pending.target) });
    if (pending?.kind === 'copy') onChange({ mode: pending.draft.mode, rows: cloneRows(pending.draft.rows) });
    setPending(null);
  }

  const overlaps = resourceType === '' || problem !== '' ? [] : sources.items.filter((source) => source.key !== excludeKey && source.resourceType === resourceType
    && (source.criteria ?? []).some((other) => criteria.some((mine) => canOverlap(mine, other as Record<string, string>))));

  const sampleApp = applications.items.find((app) => app.key === sample.app);
  const sampleEnv = sampleApp?.environments.find((env) => env.key === sample.env);
  const missing = [sampleApp && sampleEnv ? '' : t('platform.criteriaEditor.applicationAndEnvironment'), sample.resId.trim() ? '' : t('platform.criteriaEditor.resourceIDResId'), sample.cls.trim() ? '' : 'Class'].filter(Boolean);

  return <section className={`catalog-fields criteria-editor${rules ? ' criteria-editor-rules' : ''}`} aria-labelledby={`${id}-title`}>
    <div className="section-header"><div><h3 id={`${id}-title`}>{rules ? t('platform.criteriaEditor.whereIsThisDefinitionUsed') : t('platform.criteriaEditor.matchingCriteria')}</h3><p>{rules ? t('platform.criteriaEditor.addTheConditionsToLimit') : t('platform.criteriaEditor.chooseWhereThisDefinitionIs')}</p></div></div>

    {rules ? <CriteriaRules draft={draft} onChange={onChange} /> : <div role="radiogroup" aria-label={t('platform.criteriaEditor.matchingCriteriaMode')} className="criteria-modes">{modes.map((mode) => <label key={mode.key} className="criteria-mode"><input type="radio" name={`${id}-mode`} checked={draft.mode === mode.key} onChange={() => chooseMode(mode.key)} /><span><strong>{mode.label}</strong><small>{mode.hint}</small></span></label>)}</div>}

    {pending ? <div role="group" aria-label={t('platform.criteriaEditor.confirmCriteriaChange')} className="form-warning criteria-confirm">
      <p>{pending.kind === 'mode' ? t('platform.criteriaEditor.theModeLabelModeCannotFully', { modeLabel: modeLabel(pending.target) }) : t('platform.criteriaEditor.copyingFromSourceWillReplace', { source: pending.source })}</p>
      <div className="form-actions"><Button type="button" onClick={() => setPending(null)}>{pending.kind === 'mode' ? t('platform.criteriaEditor.keepAdvancedCriteria') : t('platform.criteriaEditor.keepCurrentCriteria')}</Button><Button type="button" tone="danger" onClick={confirm}>{pending.kind === 'mode' ? t('platform.criteriaEditor.dropCriteriaAndSwitchMode') : t('platform.criteriaEditor.replaceWithTheCopy')}</Button></div>
    </div> : null}

    {!rules && draft.mode === 'env_type' ? <label>{t('platform.criteriaEditor.environmentType')}<input list={`${id}-types`} value={row.env_type} aria-invalid={problem ? true : undefined} onChange={(event) => setRows([{ ...emptyCriterion(), env_type: event.target.value }])} /><datalist id={`${id}-types`}>{knownTypes.map((type) => <option key={type} value={type} />)}</datalist></label> : null}
    {!rules && draft.mode === 'app_env' ? <div className="field-grid">
      <label>{t('platform.criteriaEditor.application')}<select value={row.app_id} aria-invalid={problem ? true : undefined} onChange={(event) => setRows([{ ...emptyCriterion(), app_id: event.target.value }])}>
        <option value="">{applications.loading && applications.items.length === 0 ? t('platform.criteriaEditor.loadingApplications') : t('platform.criteriaEditor.chooseAnApplication')}</option>
        {row.app_id !== '' && !selectedApp ? <option value={row.app_id}>{t('platform.criteriaEditor.appIdNotInThe', { app_id: row.app_id })}</option> : null}
        {applications.items.map((app) => <option key={app.key} value={app.key}>{app.name !== app.key ? `${app.name} (${app.key})` : app.key}</option>)}</select></label>
      <label>{t('platform.criteriaEditor.environmentOptional')}<select value={row.env_id} disabled={row.app_id === ''} onChange={(event) => setRows([{ ...row, env_id: event.target.value }])}>
        <option value="">{t('platform.criteriaEditor.everyEnvironmentOfTheApplication')}</option>
        {row.env_id !== '' && !selectedApp?.environments.some((env) => env.key === row.env_id) ? <option value={row.env_id}>{t('platform.criteriaEditor.envIdNotInThe', { env_id: row.env_id })}</option> : null}
        {(selectedApp?.environments ?? []).map((env) => <option key={env.key} value={env.key}>{t('platform.criteriaEditor.valueTypeEnvironmentType', { value: env.name !== env.key ? `${env.name} (${env.key})` : env.key, environmentType: env.environmentType })}</option>)}</select></label>
    </div> : null}
    {!rules && draft.mode === 'advanced' ? <>{rows.map((item, index) => <div className="catalog-criterion" key={index}>{fields.map(({ key, label }) => <label key={key}>{label}<input aria-label={t('platform.criteriaEditor.conditionValueLabel', { value: index + 1, label })} value={item[key]} onChange={(event) => setRow(index, key, event.target.value)} /></label>)}<Button type="button" tone="quiet" aria-label={t('platform.criteriaEditor.removeConditionValue', { value: index + 1 })} disabled={rows.length <= 1} onClick={() => setRows(rows.filter((_, position) => position !== index))}>{t('platform.criteriaEditor.remove')}</Button></div>)}
      <div><Button type="button" onClick={() => setRows([...rows, emptyCriterion()])}>{t('platform.criteriaEditor.addCondition')}</Button></div>
      <p className="feature-note">{t('platform.criteriaEditor.anEmptyRowIsAn')}</p></> : null}

    {applications.error ? <div className="form-error" role="alert" aria-label={t('platform.criteriaEditor.applicationLoadError')}>{t('platform.criteriaEditor.couldNotLoadTheApplication')}{' '} <Button type="button" onClick={applications.onRetry}>{t('platform.criteriaEditor.retry')}</Button></div> : null}
    {sources.error ? <div className="form-error" role="alert" aria-label={t('platform.criteriaEditor.errorLoadingDefinitionsToCopy')}>{t('platform.criteriaEditor.couldNotLoadTheDefinitions')}{' '} <Button type="button" onClick={sources.onRetry}>{t('platform.criteriaEditor.retry')}</Button></div> : null}
    {problem ? <p className="feature-note" id={`${id}-problem`}>{problem}</p> : null}

    <details open={rules ? undefined : true}><summary>{t('platform.criteriaEditor.copyFromAnotherDefinition')}</summary><div className="criteria-copy field-grid"><label>{t('platform.criteriaEditor.copyCriteriaFromAnotherDefinition')}<select aria-label={t('platform.criteriaEditor.copyCriteriaFromAnotherDefinition')} value={copyFrom} onChange={(event) => setCopyFrom(event.target.value)}><option value="">{sources.loading && sources.items.length === 0 ? t('platform.criteriaEditor.loadingDefinitions') : sources.items.length === 0 ? t('platform.criteriaEditor.noDefinitionsToCopy') : t('platform.criteriaEditor.chooseADefinition')}</option>{sources.items.filter((source) => source.key !== excludeKey).map((source) => <option key={source.key} value={source.key}>{source.name && source.name !== source.key ? `${source.name} (${source.key})` : source.key} ({source.resourceType})</option>)}</select></label>
      <div className="form-actions"><Button type="button" disabled={copyFrom === ''} onClick={copy}>{t('platform.criteriaEditor.copyCriteria')}</Button></div></div>
    <p className="feature-note">{t('platform.criteriaEditor.copyingTakesAnIndependentCopy')}</p></details>

    <div className="criteria-summary" role="group" aria-label={t('platform.criteriaEditor.scopeSummary')}>
      <h4>{t('platform.criteriaEditor.scope')}</h4>
      {problem ? <p>{t('platform.criteriaEditor.notEnoughInformationToSummarize')}</p> : rules ? <>{criteria.map((item, index) => <div className="criteria-summary-case" key={index}><small>{t('platform.criteriaEditor.caseValue', { value: index + 1 })}</small>{Object.keys(item).length === 0 ? <p>{t('platform.criteriaEditor.everywhereNoLimit')}</p> : fields.filter(({ key }) => item[key] !== undefined).map(({ key }) => <p key={key}>{describeRow({ [key]: item[key] ?? '' }, applications.items)}</p>)}</div>)}<p className="criteria-summary-note">{t('platform.criteriaEditor.criteriaOnlyDecideWhenThe')}</p></> : <ul>{criteria.map((item, index) => <li key={index}>{describeRow(item, applications.items)}</li>)}</ul>}
    </div>

    {overlaps.length > 0 ? <div role="group" aria-label={t('platform.criteriaEditor.overlapRiskWarning')} className="form-warning">{t('platform.criteriaEditor.theseCriteriaMayOverlapWith', { value: overlaps.map((item) => item.key).join(', ') })}</div> : null}

    <details open={rules ? undefined : true}><summary>{t('platform.criteriaEditor.checkTheSelectedDefinition')}</summary><div className="criteria-preview" role="group" aria-label={t('platform.criteriaEditor.matchingCheckContext')}>
      <h4>{t('platform.criteriaEditor.matchingCheckContext')}</h4>
      <p className="feature-note">{t('platform.criteriaEditor.chooseARealApplicationAnd')}</p>
      <div className="field-grid">
        <label>{t('platform.criteriaEditor.checkApplication')}<select aria-label={t('platform.criteriaEditor.checkApplication')} value={sample.app} onChange={(event) => setSample({ ...sample, app: event.target.value, env: '' })}><option value="">{t('platform.criteriaEditor.chooseAnApplication')}</option>{applications.items.map((app) => <option key={app.key} value={app.key}>{app.name !== app.key ? `${app.name} (${app.key})` : app.key}</option>)}</select></label>
        <label>{t('platform.criteriaEditor.checkEnvironment')}<select aria-label={t('platform.criteriaEditor.checkEnvironment')} value={sample.env} disabled={!sampleApp} onChange={(event) => setSample({ ...sample, env: event.target.value })}><option value="">{t('platform.criteriaEditor.chooseAnEnvironment')}</option>{(sampleApp?.environments ?? []).map((env) => <option key={env.key} value={env.key}>{env.name !== env.key ? `${env.name} (${env.key})` : env.key}</option>)}</select></label>
        <label>{t('platform.criteriaEditor.checkResourceIDResId')}<input value={sample.resId} onChange={(event) => setSample({ ...sample, resId: event.target.value })} /></label>
        <label>{t('platform.criteriaEditor.checkClass')}<input value={sample.cls} onChange={(event) => setSample({ ...sample, cls: event.target.value })} /></label>
      </div>
      {missing.length > 0 ? <p>{t('platform.criteriaEditor.moreContextNeededMissingMatching', { missing: missing.join(', ') })}</p> : problem ? <p>{t('platform.criteriaEditor.completeTheMatchingCriteriaBefore')}</p> : null}
    </div>

    {matching ? <MatchingPreviewPanel candidate={matching} scopeKey={`${scopeKey}|${sampleEnv?.environmentType ?? ''}|${sampleEnv?.snapshotKey ?? ''}|apps:${applications.loading ? 'loading' : 'idle'}:${applications.error}|defs:${sources.loading ? 'loading' : 'idle'}:${sources.error}|${JSON.stringify(sources.items)}`} sample={{ applicationId: sample.app, environmentId: sampleApp && sampleEnv ? sample.env : '', resourceId: sample.resId, class: sample.cls }} /> : null}
    </details>
    <details open={rules ? undefined : true}><summary>{t('platform.criteriaEditor.howTheSystemSelectsA')}</summary><div className="feature-note criteria-rules" role="note" aria-label={t('platform.criteriaEditor.matchingRules')}><p><strong>{t('platform.criteriaEditor.aNDWithinARowOR')}</strong> {' '}{t('platform.criteriaEditor.everyFieldEnteredInThe')}</p>
      <p>{t('platform.criteriaEditor.fixedWeightsEnvType1')}</p>
      <p><strong>{t('platform.criteriaEditor.noCriteria')}</strong> {' '}{t('platform.criteriaEditor.emptyListIsRejected')}{' '} <strong>{t('platform.criteriaEditor.emptyRow')}{' '} <code>{'{}'}</code></strong> {' '}{t('platform.criteriaEditor.isAnExplicitWildcardMatching')}</p>
      <p>{t('platform.criteriaEditor.noteTheDefinitionIDDefinition')}{' '} <code>res_id</code>{t('platform.criteriaEditor.only')}{' '} <code>res_id</code> {' '}{t('platform.criteriaEditor.isAMatchingCondition')}</p></div></details>
  </section>;
}
