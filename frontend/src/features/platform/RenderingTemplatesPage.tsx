import { useEffect, useState, type FormEvent } from 'react';
import { specificGuidance } from '../../shared/api/errors';
import { parseRenderBundles } from './metadata';
import { api, ApiError, isStaleConflict } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { snapshotKey, type ApiEnvironment } from './environmentSnapshot';
import { CriteriaEditor, draftFromCriteria, draftToCriteria, emptyDraft, type ApplicationOption, type Criterion, type CriteriaDraft } from './CriteriaEditor';
import type { MatchingCandidateInput } from './MatchingPreviewPanel';
import { useCatalogList } from './useCatalogList';
import { loadCapabilities } from './capabilities';
import { ProvisionEditor } from './ProvisionEditor';
import type { ProvisionRow } from './typedProvision';
import { bodyToTemplate, emptyTemplateForm, jsonToBody, templateToBody, type TemplateForm } from './workloadTemplate';
import { t } from '../../shared/i18n';

type APIApplication = { key: string; name?: string; environments?: ApiEnvironment[] };
// The same profile-free Definition contract as the Resource Definition page (UC-03 V02-01): Name, version, five-field criteria.
type Definition = { key: string; name?: string; version?: number; resourceType: string; driverType: string; driverInputs?: { values?: { variables?: Record<string, unknown> } }; provision?: Record<string, unknown>; criteria?: Partial<Criterion>[] };
type Editing = { key: string; version: number };

type InstalledBundle = { id: string; version: string; status: string };
const idShape = /^[a-z0-9-]+$/;
const maxNameLength = 253;
const listFailure = () => t('platform.renderingTemplates.couldNotLoadDataFrom');
const loadDefinitions = async () => (await api<{ resourceDefinitions: Definition[] }>('/resource-definitions')).resourceDefinitions ?? [];
const loadBundles = async (): Promise<InstalledBundle[]> => parseRenderBundles(await api<unknown>('/render-bundles')).filter((item) => item.status === 'AVAILABLE');
// Real application/environment IDs (keys) for the criteria editor; never derived from names.
const loadApplications = async (): Promise<ApplicationOption[]> => ((await api<{ applications: APIApplication[] }>('/applications')).applications ?? []).map((app) => ({ key: app.key, name: app.name ?? app.key, environments: (app.environments ?? []).map((env) => ({ key: env.key, name: env.name ?? env.key, environmentType: env.environmentType ?? '', snapshotKey: snapshotKey(env) })) }));

// Specific guidance comes from the stable error code; raw backend text is never shown.
function describeFailure(reason: unknown, editing: boolean): string {
  if (!(reason instanceof ApiError)) return editing ? t('platform.renderingTemplates.noResponseFromTheServer') : t('platform.renderingTemplates.noResponseFromTheServer2');
  const specific = specificGuidance(reason.code, reason.message, reason.field);
  if (specific) return t('platform.renderingTemplates.theServerRejectedTheTemplate', { specific });
  switch (reason.status) {
    case 400: return t('platform.renderingTemplates.theServerRejectedThisTemplate');
    case 401: return t('platform.renderingTemplates.yourSessionHasExpiredPlease');
    case 403: return t('platform.renderingTemplates.youAreNotAllowedTo');
    case 409: return t('platform.renderingTemplates.theTemplateIDAlreadyExists');
    case 413: return t('platform.renderingTemplates.theTemplateContentIsToo');
    default: return t('platform.renderingTemplates.theServerFailedWhileRegistering');
  }
}

const bundleOf = (definition: Definition) => {
  const bundle = definition.driverInputs?.values?.variables?.render_bundle;
  return typeof bundle === 'string' && bundle !== '' ? bundle : '';
};

export function RenderingTemplatesPage() {
  const definitionList = useCatalogList(loadDefinitions, listFailure);
  const applicationList = useCatalogList(loadApplications, listFailure);
  const bundleList = useCatalogList(loadBundles, () => t('platform.renderingTemplates.couldNotLoadTheInstalled'));
  const capabilityList = useCatalogList(loadCapabilities, () => t('platform.renderingTemplates.couldNotLoadTheWorkload'));
  const presets = capabilityList.items.find((capability) => capability.driverType === 'score-k8s')?.variables ?? [];
  const presetOf = (name: string) => presets.find((variable) => variable.name === name)?.preset ?? '';
  const templates = definitionList.items.filter((definition) => definition.resourceType === 'workload');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [idError, setIdError] = useState('');
  const [nameError, setNameError] = useState('');
  const [bundleError, setBundleError] = useState('');
  const [notice, setNotice] = useState('');
  const [key, setKey] = useState('');
  const [name, setName] = useState('');
  const [bundle, setBundle] = useState('');
  const [criteriaDraft, setCriteriaDraft] = useState<CriteriaDraft>(emptyDraft);
  // Typed placement and companions (ADR-016). Form and advanced JSON serialize to the same two objects.
  const [namespaceRef, setNamespaceRef] = useState('');
  const [clusterRef, setClusterRef] = useState('');
  const [rows, setRows] = useState<ProvisionRow[]>([]);
  const [mode, setMode] = useState<'form' | 'json'>('form');
  const [jsonText, setJsonText] = useState('{}');
  const [modeNote, setModeNote] = useState('');
  const [editing, setEditing] = useState<Editing | null>(null);
  // A stale (409) or unknown-outcome save never resubmits by itself: the draft stays and Save is off until an explicit reload.
  // A create with an unknown outcome is held the same way (`created` is the submitted ID): no automatic POST replay before the stored key is read back.
  const [review, setReview] = useState<{ kind: 'stale' | 'unknown'; created?: string } | null>(null);

  // One installed bundle is chosen for an unselected draft; several need an explicit choice.
  // The chosen ID is never replaced by a later or late list response.
  useEffect(() => {
    const [only] = bundleList.items;
    if (!bundleList.loading && !bundleList.loadError && bundle === '' && bundleList.items.length === 1 && only && editing === null) setBundle(only.id);
  }, [bundleList.loading, bundleList.loadError, bundleList.items, bundle, editing]);
  const bundleBlocked = bundleList.loading || bundleList.loadError !== '' || bundleList.items.length === 0;
  // The chosen ID stays visible even when the (stale, loading or failed) list does not hold it.
  const selectedMissing = bundle !== '' && !bundleList.items.some((item) => item.id === bundle);
  const bundleStale = selectedMissing && !bundleBlocked;

  const formState = (): TemplateForm => ({ ...emptyTemplateForm(), bundle, namespaceRef, clusterRef, rows });
  function applyForm(form: TemplateForm) { setBundle(form.bundle); setNamespaceRef(form.namespaceRef); setClusterRef(form.clusterRef); setRows(form.rows); }
  function jsonFromForm(form: TemplateForm): string {
    const body = templateToBody(form);
    // An incomplete form still round-trips: raw values are shown exactly as typed.
    const variables = body.variables ?? { render_bundle: form.bundle, namespace: form.namespaceRef, cluster: form.clusterRef };
    return JSON.stringify({ variables, ...(body.provision ? { provision: body.provision } : {}) }, null, 2);
  }
  function switchMode(next: 'form' | 'json') {
    setModeNote('');
    if (next === 'json') { setJsonText(jsonFromForm(formState())); setMode('json'); return; }
    const parsed = jsonToBody(jsonText);
    if (parsed.error) { setModeNote(parsed.error); return; }
    const converted = bodyToTemplate(parsed.variables, parsed.provision);
    if (!converted.form) { setModeNote(converted.reason ?? t('platform.renderingTemplates.theAdvancedJSONCannotBe')); return; }
    applyForm(converted.form); setMode('form');
  }
  // The body both modes send; a problem blocks Save without changing the draft.
  const currentBody = (): { variables?: unknown; provision?: unknown; problem?: string } => {
    if (mode === 'form') return templateToBody(formState());
    const parsed = jsonToBody(jsonText);
    if (parsed.error) return { problem: parsed.error };
    if (parsed.variables === null || typeof parsed.variables !== 'object' || Array.isArray(parsed.variables)) return { problem: t('platform.renderingTemplates.variablesMustBeAJSON') };
    return { variables: parsed.variables, ...(parsed.provision !== undefined ? { provision: parsed.provision } : {}) };
  };

  function resetForm() {
    applyForm(emptyTemplateForm()); setMode('form'); setModeNote(''); setJsonText('{}');
    setKey(''); setName(''); setBundle(''); setCriteriaDraft(emptyDraft()); setEditing(null); setReview(null);
    setError(''); setIdError(''); setNameError(''); setBundleError('');
  }
  function startEdit(template: Definition) {
    setNotice(''); setError(''); setIdError(''); setNameError(''); setBundleError(''); setReview(null);
    setKey(template.key); setName(template.name ?? ''); setBundle(bundleOf(template)); setCriteriaDraft(draftFromCriteria(template.criteria));
    const converted = bodyToTemplate(template.driverInputs?.values?.variables ?? {}, template.provision);
    if (converted.form) { applyForm(converted.form); setMode('form'); setModeNote(''); }
    else {
      applyForm({ ...emptyTemplateForm(), bundle: bundleOf(template) });
      setJsonText(JSON.stringify({ variables: template.driverInputs?.values?.variables ?? {}, ...(template.provision ? { provision: template.provision } : {}) }, null, 2));
      setMode('json'); setModeNote(t('platform.renderingTemplates.valueTheTemplateWasOpened', { value: converted.reason ?? '' }));
    }
    setEditing({ key: template.key, version: template.version ?? 0 });
  }
  // Explicit reload after a stale or unknown-outcome save: one GET of the stored template replaces the draft.
  // After an unknown CREATE the GET decides: found means the earlier POST committed (the stored version opens, nothing is re-sent);
  // not found means it rolled back (the draft stays and the hold is released); a failed GET keeps both draft and hold.
  async function reloadLatest() {
    const target = review?.created ?? editing?.key;
    if (!target) return;
    let latest: Definition | undefined;
    try { latest = (await loadDefinitions()).find((definition) => definition.key === target); } catch { setError(t('platform.renderingTemplates.couldNotReloadTheTemplate')); return; }
    void definitionList.reload();
    if (review?.created) {
      if (!latest) { setReview(null); setError(''); setNotice(t('platform.renderingTemplates.theServerHasNoTemplate', { target })); return; }
      startEdit(latest);
      setNotice(t('platform.renderingTemplates.templateTargetExistsOnThe', { target }));
      return;
    }
    if (!latest) { setError(t('platform.renderingTemplates.thisTemplateIsNoLonger')); return; }
    startEdit(latest);
    setNotice(t('platform.renderingTemplates.theLatestVersionWasReloaded'));
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving || review) return;
    setError(''); setIdError(''); setNameError(''); setBundleError(''); setNotice('');
    // The backend owns the ID rule; the ID is checked, never changed.
    if (key === '') { setIdError(t('platform.renderingTemplates.enterATemplateID')); return; }
    if (!idShape.test(key)) { setIdError(t('platform.renderingTemplates.theIDMayContainOnly')); return; }
    if (name.trim() === '') { setNameError(t('platform.renderingTemplates.enterTheDisplayNameOf')); return; }
    if ([...name].length > maxNameLength) { setNameError(t('platform.renderingTemplates.theNameCanHaveAt', { maxNameLength })); return; }
    if (bundleBlocked) { setBundleError(t('platform.renderingTemplates.noBundleListIsAvailable')); return; }
    if (bundle === '') { setBundleError(t('platform.renderingTemplates.chooseARenderingBundle')); return; }
    if (bundleStale) { setBundleError(t('platform.renderingTemplates.theSelectedBundleIsNo')); return; }
    const { criteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) { setError(problem); return; }
    const submitted = key;
    const parts = currentBody();
    if (parts.problem) { setError(parts.problem); return; }
    const body = { key: submitted, name, resourceType: 'workload', driverType: 'score-k8s', driverInputs: { values: { variables: parts.variables } }, ...(parts.provision !== undefined ? { provision: parts.provision } : {}), criteria };
    setSaving(true);
    if (editing) {
      let saved: Definition;
      try {
        saved = await api<Definition>(`/resource-definitions/${encodeURIComponent(editing.key)}`, { method: 'PUT', body: JSON.stringify({ ...body, version: editing.version }) });
      } catch (reason) {
        setSaving(false);
        if (reason instanceof ApiError && isStaleConflict(reason)) { setReview({ kind: 'stale' }); return; }
        if (!(reason instanceof ApiError) || reason.code === 'COMMIT_UNCERTAIN') { setReview({ kind: 'unknown' }); void definitionList.reload(); }
        setError(describeFailure(reason, true));
        return;
      }
      setEditing({ key: saved.key, version: saved.version ?? editing.version + 1 });
      setNotice(t('platform.renderingTemplates.savedRenderingTemplateKeyVersion', { key: saved.key, value: saved.version ?? editing.version + 1 }));
      setSaving(false);
      await definitionList.reload();
      return;
    }
    try {
      await api('/resource-definitions', { method: 'POST', body: JSON.stringify(body) });
    } catch (reason) {
      setSaving(false);
      if (!(reason instanceof ApiError) || reason.code === 'COMMIT_UNCERTAIN') { setReview({ kind: 'unknown', created: submitted }); void definitionList.reload(); }
      setError(describeFailure(reason, false));
      return;
    }
    // Committed: report it and reset the submitted form before reloading.
    setNotice(t('platform.renderingTemplates.registeredRenderingTemplateSubmitted', { submitted })); setKey(''); setName(''); applyForm(emptyTemplateForm()); setMode('form'); setJsonText('{}'); setCriteriaDraft(emptyDraft()); setSaving(false);
    await definitionList.reload();
  }

  const matching = ((): MatchingCandidateInput => {
    if (key === '' || !idShape.test(key)) return { definition: null, problem: t('platform.renderingTemplates.enterAValidTemplateID') };
    if (bundle === '') return { definition: null, problem: t('platform.renderingTemplates.chooseARenderingBundleTo') };
    const { criteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) return { definition: null, problem };
    return { definition: { key, name, resourceType: 'workload', driverType: 'score-k8s', driverInputs: { values: { variables: { render_bundle: bundle } } }, criteria }, problem: '' };
  })();

  const listError = definitionList.loadError;
  return <section className="page"><header className="page-header"><div><p className="eyebrow">{t('platform.renderingTemplates.platform')}</p><h1>{t('platform.renderingTemplates.renderingTemplates')}</h1><p>{t('platform.renderingTemplates.everyWorkloadNeedsExactlyOne')}</p></div></header>
    <section className="content-panel" aria-label={t('platform.renderingTemplates.registeredRenderingTemplates')}><div className="section-header"><h2>{t('platform.renderingTemplates.registeredTemplates')}</h2></div>{listError ? <div className="form-error" role="alert" aria-label={t('platform.renderingTemplates.listLoadError')}>{listError} <Button type="button" onClick={() => { void definitionList.reload(); }}>{t('platform.renderingTemplates.retry')}</Button></div> : definitionList.loading && definitionList.items.length === 0 ? <p role="status">{t('platform.renderingTemplates.loadingRenderingTemplates')}</p> : templates.length === 0 ? <p>{t('platform.renderingTemplates.noRenderingTemplatesYet')}</p> : <div className="catalog-list">{templates.map((template) => <div key={template.key} className="catalog-entry" data-definition={template.key}><strong>{template.name || template.key}</strong><span>ID <code>{template.key}</code> · Bundle: {bundleOf(template) || t('platform.renderingTemplates.unknown')} {' '}{t('platform.renderingTemplates.version')}{' '} {template.version ?? '?'} · {template.criteria?.length ?? 0} {' '}{t('platform.renderingTemplates.matchingCriteria')}</span><Button type="button" aria-label={t('platform.renderingTemplates.editTemplateKey', { key: template.key })} disabled={saving} onClick={() => startEdit(template)}>{t('platform.renderingTemplates.edit')}</Button></div>)}</div>}</section>
    <section className="content-panel"><div className="section-header"><div><h2>{editing ? t('platform.renderingTemplates.editRenderingTemplateKey', { key: editing.key }) : t('platform.renderingTemplates.registerRenderingTemplate')}</h2><p>{t('platform.renderingTemplates.theResourceTypeAndDriver')}</p></div>{editing ? <Button type="button" onClick={resetForm} disabled={saving}>{t('platform.renderingTemplates.cancelEditing')}</Button> : null}</div>
      {review ? <div className="form-warning" role="alert" aria-label={t('platform.renderingTemplates.templateNeedsReloading')}><p>{review.kind === 'stale' ? t('platform.renderingTemplates.theTemplateWasChangedBy') : review.created ? t('platform.renderingTemplates.itIsUnknownWhetherThe') : t('platform.renderingTemplates.itIsUnknownWhetherThe2')} {review.created ? t('platform.renderingTemplates.checkOnTheServerIf') : t('platform.renderingTemplates.reloadTheLatestVersionReview')}</p>
        <div className="form-actions"><Button type="button" onClick={() => { void definitionList.reload(); }}>{t('platform.renderingTemplates.reloadList')}</Button><Button type="button" tone="primary" onClick={() => { void reloadLatest(); }}>{review.created ? t('platform.renderingTemplates.checkOnServer') : t('platform.renderingTemplates.reloadLatestVersion')}</Button></div></div> : null}
      <form onSubmit={submit} noValidate aria-label={editing ? t('platform.renderingTemplates.editRenderingTemplate') : t('platform.renderingTemplates.registerRenderingTemplate')}><fieldset className="editor-grid form-fieldset" disabled={saving} aria-busy={saving}>
        <div className="field-grid"><label>{t('platform.renderingTemplates.templateID')}<input value={key} readOnly={editing !== null} aria-readonly={editing !== null ? true : undefined} aria-required="true" aria-invalid={idError ? true : undefined} aria-describedby={idError ? 'template-id-hint template-id-error' : 'template-id-hint'} onChange={(event) => { setKey(event.target.value); setIdError(''); }} /></label>
          <label>{t('platform.renderingTemplates.displayName')}<input value={name} maxLength={maxNameLength} aria-required="true" aria-invalid={nameError ? true : undefined} onChange={(event) => { setName(event.target.value); setNameError(''); }} /></label>
          <label>{t('platform.renderingTemplates.renderingBundle')}<select value={bundle} disabled={bundleBlocked} aria-required="true" aria-invalid={bundleError || selectedMissing ? true : undefined} aria-describedby={bundleError ? 'template-bundle-hint template-bundle-error' : 'template-bundle-hint'} onChange={(event) => { setBundle(event.target.value); setBundleError(''); }}>
            {bundleList.items.length !== 1 ? <option value="">{bundleList.loading ? t('platform.renderingTemplates.loadingBundles') : t('platform.renderingTemplates.chooseARenderingBundle2')}</option> : null}
            {selectedMissing ? <option value={bundle}>{bundle} ({bundleBlocked ? t('platform.renderingTemplates.availabilityNotConfirmed') : t('platform.renderingTemplates.noLongerAvailable')})</option> : null}
            {bundleList.items.map((item) => <option key={item.id} value={item.id}>{t('platform.renderingTemplates.idVersionVersion', { id: item.id, version: item.version })}</option>)}
          </select></label></div>
        <p id="template-id-hint" className="feature-note">{t('platform.renderingTemplates.theTemplateIDUsesLowercase')}{' '} <code>res_id</code>{t('platform.renderingTemplates.theDisplayNameCanBe')}</p>
        {idError ? <div id="template-id-error" className="form-error" role="alert">{idError}</div> : null}
        {nameError ? <div className="form-error" role="alert">{nameError}</div> : null}
        <p id="template-bundle-hint" className="feature-note">{t('platform.renderingTemplates.chooseABundleInstalledOn')}</p>
        {bundleList.loadError ? <div className="form-error" role="alert" aria-label={t('platform.renderingTemplates.bundleLoadError')}>{bundleList.loadError} {' '}{t('platform.renderingTemplates.theEnteredDataIsKept')}{' '} <Button type="button" onClick={() => { void bundleList.reload(); }}>{t('platform.renderingTemplates.retryLoadingBundles')}</Button></div> : bundleList.loading ? <p role="status">{t('platform.renderingTemplates.loadingInstalledBundles')}</p> : bundleList.items.length === 0 ? <div className="form-error" role="alert" aria-label={t('platform.renderingTemplates.noBundles')}>{t('platform.renderingTemplates.noRenderingBundleIsInstalled')}{' '} <Button type="button" onClick={() => { void bundleList.reload(); }}>{t('platform.renderingTemplates.refreshBundleList')}</Button></div> : <p><Button type="button" onClick={() => { void bundleList.reload(); }}>{t('platform.renderingTemplates.refreshBundleList')}</Button></p>}
        {bundleStale ? <div className="form-error" role="alert" aria-label={t('platform.renderingTemplates.bundleNoLongerAvailable')}>{t('platform.renderingTemplates.theSelectedBundleBundleIs', { bundle })}</div> : null}
        {bundleError ? <div id="template-bundle-error" className="form-error" role="alert">{bundleError}</div> : null}
        <section className="catalog-fields" aria-label={t('platform.renderingTemplates.deploymentTargetAndWorkloadConfiguration')}>
          <div className="section-header"><div><h4>{t('platform.renderingTemplates.deploymentTargetAndInputs')}</h4><p className="feature-note">{t('platform.renderingTemplates.theConfigurationDecidesWhereThe')}{' '} <code>name</code> {' '}{t('platform.renderingTemplates.ofTheNamespaceAndOutput')}{' '} <code>cluster</code> {' '}{t('platform.renderingTemplates.ofTheClusterTheCluster')}</p></div>
            <Button type="button" onClick={() => switchMode(mode === 'form' ? 'json' : 'form')}>{mode === 'form' ? t('platform.renderingTemplates.advancedJSON') : t('platform.renderingTemplates.form')}</Button></div>
          {modeNote ? <div className="form-warning" role="status">{modeNote}</div> : null}
          {mode === 'json' ? <label>{t('platform.renderingTemplates.advancedJSONVariablesProvision')}<textarea aria-label={t('platform.renderingTemplates.advancedJSONOfTheRendering')} rows={14} spellCheck={false} value={jsonText} onChange={(event) => setJsonText(event.target.value)} /></label> : <>
            <label>{t('platform.renderingTemplates.namespaceReference')}<input aria-label={t('platform.renderingTemplates.namespaceReference')} value={namespaceRef} spellCheck={false} placeholder="${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}" onChange={(event) => setNamespaceRef(event.target.value)} /></label>
            {presetOf('namespace') ? <Button type="button" tone="quiet" onClick={() => setNamespaceRef(presetOf('namespace'))}>{t('platform.renderingTemplates.useNamespaceSuggestion')}</Button> : null}
            <label>{t('platform.renderingTemplates.clusterReference')}<input aria-label={t('platform.renderingTemplates.clusterReference')} value={clusterRef} spellCheck={false} placeholder="${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}" onChange={(event) => setClusterRef(event.target.value)} /></label>
            {presetOf('cluster') ? <Button type="button" tone="quiet" onClick={() => setClusterRef(presetOf('cluster'))}>{t('platform.renderingTemplates.useClusterSuggestion')}</Button> : null}
            <ProvisionEditor rows={rows} onChange={setRows} disabled={saving} />
          </>}
        </section>
        <CriteriaEditor draft={criteriaDraft} onChange={setCriteriaDraft} applications={{ items: applicationList.items, loading: applicationList.loading, error: applicationList.loadError, onRetry: () => { void applicationList.reload(); } }} sources={{ items: templates, loading: definitionList.loading, error: definitionList.loadError, onRetry: () => { void definitionList.reload(); } }} resourceType="workload" matching={matching} scopeKey={`bundles:${bundleList.loading ? 'loading' : bundleList.loadError ? 'error' : JSON.stringify(bundleList.items.map((item) => [item.id, item.version, item.status]))}|editing:${editing ? `${editing.key}@${editing.version}` : ''}`} excludeKey={editing?.key ?? ''} />
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saving || bundleBlocked || selectedMissing || review !== null}>{saving ? (editing ? t('platform.renderingTemplates.saving') : t('platform.renderingTemplates.registering')) : editing ? t('platform.renderingTemplates.saveChanges') : t('platform.renderingTemplates.registerRenderingTemplate')}</Button></div>
      </fieldset></form>
    </section>
  </section>;
}
