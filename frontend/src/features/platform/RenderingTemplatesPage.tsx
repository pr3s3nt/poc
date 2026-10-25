import { useEffect, useRef, useState, type FormEvent } from 'react';
import { specificGuidance } from '../../shared/api/errors';
import { parseRenderBundle, parseRenderBundles, type RenderBundle } from './metadata';
import { api, ApiError, isStaleConflict } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { snapshotKey, type ApiEnvironment } from './environmentSnapshot';
import { CriteriaEditor, draftFromCriteria, draftToCriteria, emptyDraft, type ApplicationOption, type Criterion, type CriteriaDraft } from './CriteriaEditor';
import type { MatchingCandidateInput } from './MatchingPreviewPanel';
import { useCatalogList } from './useCatalogList';
import { loadCapabilities } from './capabilities';
import { ProvisionEditor } from './ProvisionEditor';
import type { ProvisionRow } from './typedProvision';
import type { JsonObject } from './typedVariables';
import { bodyToTemplate, emptyTemplateForm, jsonToBody, templateToBody, type TemplateForm } from './workloadTemplate';
import { GitBundleSource, emptyGitSource, type GitSourceDraft } from './GitBundleSource';
import { RenderInputsEditor, renderInputsBlocked } from './RenderInputsEditor';
import { t } from '../../shared/i18n';

type APIApplication = { key: string; name?: string; environments?: ApiEnvironment[] };
// The same profile-free Definition contract as the Resource Definition page (UC-03 V02-01): Name, version, five-field criteria.
type Definition = { key: string; name?: string; version?: number; resourceType: string; driverType: string; driverInputs?: { values?: { variables?: Record<string, unknown> } }; provision?: Record<string, unknown>; criteria?: Partial<Criterion>[] };
type Editing = { key: string; version: number };
// New drafts default to Git (UC-03 RB-01); verified covers installed and previously verified Git bundles.
type SourceMode = 'git' | 'verified';

const idShape = /^[a-z0-9-]+$/;
const uuidShape = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const maxNameLength = 253;
const listFailure = () => t('platform.renderingTemplates.couldNotLoadDataFrom');
const loadDefinitions = async () => (await api<{ resourceDefinitions: Definition[] }>('/resource-definitions')).resourceDefinitions ?? [];
// Installed bundles are listed only when available; Git records stay listed when unavailable so a saved pin can be repaired.
const loadBundles = async (): Promise<RenderBundle[]> => parseRenderBundles(await api<unknown>('/render-bundles')).filter((item) => item.status === 'AVAILABLE' || item.source !== undefined);
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
const shortCommit = (bundle: RenderBundle) => bundle.source?.revision.slice(0, 12) ?? bundle.version;
const bundleLabel = (bundle: RenderBundle) => bundle.source ? t('platform.renderingTemplates.gitBundleLabel', { name: bundle.name ?? '', version: shortCommit(bundle) }) : t('platform.renderingTemplates.idVersionVersion', { id: bundle.id, version: bundle.version });

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
  const [sourceMode, setSourceMode] = useState<SourceMode>('git');
  const [gitDraft, setGitDraft] = useState<GitSourceDraft>(emptyGitSource);
  const [gitDraftVersion, setGitDraftVersion] = useState(0);
  const [gitSourceToLoad, setGitSourceToLoad] = useState('');
  const [inspected, setInspected] = useState<RenderBundle | null>(null);
  const [bundle, setBundle] = useState('');
  // A saved Git pin reopened by ID when the list does not (yet) hold it; never re-inspected.
  const [savedDetail, setSavedDetail] = useState<RenderBundle | null>(null);
  const [inputs, setInputs] = useState<JsonObject>({});
  const [inputDrafts, setInputDrafts] = useState<Record<string, JsonObject>>({});
  const [rawProblems, setRawProblems] = useState(0);
  const [repairing, setRepairing] = useState(false);
  const [repairError, setRepairError] = useState('');
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
  const detailRequest = useRef(0);

  const verifiedItems = bundleList.items;
  const selectedId = sourceMode === 'git' ? inspected?.id ?? '' : bundle;
  const savedBundle = verifiedItems.find((item) => item.id === bundle) ?? (savedDetail?.id === bundle ? savedDetail : undefined);
  const selected: RenderBundle | undefined = sourceMode === 'git' ? inspected ?? undefined : savedBundle;
  const git = selected?.source !== undefined;

  // A user may switch to Git before either catalog or detail has arrived.
  // Resolve the saved source independently of the active mode; source edits,
  // cancellation and a different template invalidate this initialization.
  useEffect(() => {
    if (sourceMode !== 'git' || gitSourceToLoad !== bundle || !savedBundle?.source) return;
    const source = savedBundle.source;
    setGitSourceToLoad('');
    setGitDraft((draft) => ({ ...draft, url: source.url, path: source.path === '.' ? '' : source.path }));
  }, [sourceMode, gitSourceToLoad, bundle, savedBundle]);

  // Inputs are drafted per pin: leaving a pin keeps its draft, returning restores it, and a
  // first visit carries the current inputs over so incompatible fields are shown, never dropped.
  const pinRef = useRef('');
  const inputsRef = useRef(inputs);
  inputsRef.current = inputs;
  const draftsRef = useRef(inputDrafts);
  draftsRef.current = inputDrafts;
  useEffect(() => {
    const previous = pinRef.current;
    if (previous === selectedId) return;
    if (previous) setInputDrafts((drafts) => ({ ...drafts, [previous]: inputsRef.current }));
    const stored = selectedId ? draftsRef.current[selectedId] : undefined;
    if (stored) setInputs(stored);
    pinRef.current = selectedId;
  }, [selectedId]);

  // One installed bundle is chosen for an unselected verified draft; several need an explicit choice.
  // The chosen ID is never replaced by a later or late list response.
  useEffect(() => {
    const [only] = verifiedItems;
    if (sourceMode === 'verified' && !bundleList.loading && !bundleList.loadError && bundle === '' && verifiedItems.length === 1 && only && editing === null) setBundle(only.id);
  }, [sourceMode, bundleList.loading, bundleList.loadError, verifiedItems, bundle, editing]);
  const listBlocked = bundleList.loading || bundleList.loadError !== '' || verifiedItems.length === 0;
  // The chosen ID stays visible even when the (stale, loading or failed) list does not hold it.
  const selectedMissing = sourceMode === 'verified' && bundle !== '' && !selected;
  const bundleStale = selectedMissing && !listBlocked;
  const unavailable = sourceMode === 'verified' && selected !== undefined && selected.status !== 'AVAILABLE';

  const formState = (): TemplateForm => ({ ...emptyTemplateForm(), bundle: selectedId, namespaceRef, clusterRef, rows, ...(git ? { inputs } : {}) });
  function applyForm(form: TemplateForm) { setNamespaceRef(form.namespaceRef); setClusterRef(form.clusterRef); setRows(form.rows); setInputs(form.inputs ?? {}); }
  function jsonFromForm(form: TemplateForm): string {
    const body = templateToBody(form);
    // An incomplete form still round-trips: raw values are shown exactly as typed.
    const variables = body.variables ?? { render_bundle: form.bundle, namespace: form.namespaceRef, cluster: form.clusterRef, ...(form.inputs !== undefined ? { render_inputs: form.inputs } : {}) };
    return JSON.stringify({ variables, ...(body.provision ? { provision: body.provision } : {}) }, null, 2);
  }
  function switchMode(next: 'form' | 'json') {
    setModeNote('');
    if (next === 'json') {
      if (rawProblems > 0) { setModeNote(t('platform.renderingTemplates.fixInputsBeforeJSON')); return; }
      setJsonText(jsonFromForm(formState())); setMode('json'); return;
    }
    const parsed = jsonToBody(jsonText);
    if (parsed.error) { setModeNote(parsed.error); return; }
    const converted = bodyToTemplate(parsed.variables, parsed.provision);
    if (!converted.form) { setModeNote(converted.reason ?? t('platform.renderingTemplates.theAdvancedJSONCannotBe')); return; }
    if (converted.form.bundle !== selectedId) { setModeNote(t('platform.renderingTemplates.changeBundleInTheSource')); return; }
    if (converted.form.inputs !== undefined && !git) { setModeNote(t('platform.renderingTemplates.installedBundlesTakeNoInputs')); return; }
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

  function chooseSource(next: SourceMode) {
    if (next === sourceMode) return;
    setBundleError('');
    // The ref stays an explicit user edit; late metadata supplies only URL/path.
    setGitSourceToLoad(next === 'git' && !gitDraft.url && !gitDraft.path ? bundle : '');
    if (next === 'verified' && inspected) setBundle(inspected.id);
    setSourceMode(next);
  }

  function resetForm() {
    detailRequest.current += 1; setGitSourceToLoad('');
    applyForm(emptyTemplateForm()); setMode('form'); setModeNote(''); setJsonText('{}');
    setKey(''); setName(''); setBundle(''); setSourceMode('git'); setGitDraft(emptyGitSource()); setGitDraftVersion((value) => value + 1); setInspected(null); setSavedDetail(null);
    setInputs({}); setInputDrafts({}); setRawProblems(0); setRepairError(''); pinRef.current = '';
    setCriteriaDraft(emptyDraft()); setEditing(null); setReview(null);
    setError(''); setIdError(''); setNameError(''); setBundleError('');
  }
  async function loadSavedDetail(id: string) {
    const current = ++detailRequest.current;
    try {
      const response = await api<{ renderBundle: unknown }>(`/render-bundles/${encodeURIComponent(id)}`);
      const detail = parseRenderBundle(response.renderBundle);
      if (current === detailRequest.current) setSavedDetail(detail);
    } catch { /* The list remains the source; a missing pin shows as no longer available. */ }
  }
  function startEdit(template: Definition) {
    detailRequest.current += 1; setGitSourceToLoad('');
    setNotice(''); setError(''); setIdError(''); setNameError(''); setBundleError(''); setReview(null); setRepairError('');
    setKey(template.key); setName(template.name ?? ''); setCriteriaDraft(draftFromCriteria(template.criteria));
    // Edit reopens the saved pin and inputs without re-inspection (UC-03 RB-04).
    const saved = bundleOf(template);
    setSourceMode('verified'); setBundle(saved); setInspected(null); setGitDraft(emptyGitSource()); setGitDraftVersion((value) => value + 1);
    setInputDrafts({}); setRawProblems(0); pinRef.current = saved; setSavedDetail(null);
    if (uuidShape.test(saved)) void loadSavedDetail(saved);
    const converted = bodyToTemplate(template.driverInputs?.values?.variables ?? {}, template.provision);
    if (converted.form) { applyForm(converted.form); setMode('form'); setModeNote(''); }
    else {
      applyForm(emptyTemplateForm());
      setJsonText(JSON.stringify({ variables: template.driverInputs?.values?.variables ?? {}, ...(template.provision ? { provision: template.provision } : {}) }, null, 2));
      setMode('json'); setModeNote(t('platform.renderingTemplates.valueTheTemplateWasOpened', { value: converted.reason ?? '' }));
    }
    setEditing({ key: template.key, version: template.version ?? 0 });
  }
  // Re-inspection of an unavailable saved pin uses its exact full commit; a changed branch is a new version.
  async function repairSaved() {
    if (!selected?.source) return;
    setRepairing(true); setRepairError('');
    try {
      await api('/render-bundles/inspect', { method: 'POST', body: JSON.stringify({ url: selected.source.url, path: selected.source.path, ref: selected.source.revision }) });
      await bundleList.reload();
    } catch (reason) {
      setRepairError(reason instanceof ApiError && reason.code?.startsWith('RENDER_') ? reason.message : t('platform.gitBundleSource.couldNotCheckTheBundle'));
    } finally { setRepairing(false); }
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

  const inputsInvalid = git && mode === 'form' && renderInputsBlocked(selected?.inputsSchema, inputs, rawProblems);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving || review) return;
    setError(''); setIdError(''); setNameError(''); setBundleError(''); setNotice('');
    // The backend owns the ID rule; the ID is checked, never changed.
    if (key === '') { setIdError(t('platform.renderingTemplates.enterATemplateID')); return; }
    if (!idShape.test(key)) { setIdError(t('platform.renderingTemplates.theIDMayContainOnly')); return; }
    if (name.trim() === '') { setNameError(t('platform.renderingTemplates.enterTheDisplayNameOf')); return; }
    if ([...name].length > maxNameLength) { setNameError(t('platform.renderingTemplates.theNameCanHaveAt', { maxNameLength })); return; }
    if (sourceMode === 'git' && !inspected) { setBundleError(t('platform.renderingTemplates.checkTheBundleBeforeSaving')); return; }
    if (sourceMode === 'verified') {
      if (listBlocked && !selected) { setBundleError(t('platform.renderingTemplates.noBundleListIsAvailable')); return; }
      if (bundle === '') { setBundleError(t('platform.renderingTemplates.chooseARenderingBundle')); return; }
      if (bundleStale) { setBundleError(t('platform.renderingTemplates.theSelectedBundleIsNo')); return; }
      if (unavailable) { setBundleError(t('platform.renderingTemplates.theSavedBundleIsUnavailable')); return; }
    }
    if (inputsInvalid) { setError(t('platform.renderingTemplates.fixTheRenderInputs')); return; }
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
      if (sourceMode === 'git' && inspected) { setSourceMode('verified'); setBundle(inspected.id); setSavedDetail(inspected); }
      setNotice(t('platform.renderingTemplates.savedRenderingTemplateKeyVersion', { key: saved.key, value: saved.version ?? editing.version + 1 }));
      setSaving(false);
      await Promise.all([definitionList.reload(), bundleList.reload()]);
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
    // Committed: report it and reset the submitted form (back to the Git default) before reloading.
    resetForm();
    setNotice(t('platform.renderingTemplates.registeredRenderingTemplateSubmitted', { submitted })); setSaving(false);
    await Promise.all([definitionList.reload(), bundleList.reload()]);
  }

  const matching = ((): MatchingCandidateInput => {
    if (key === '' || !idShape.test(key)) return { definition: null, problem: t('platform.renderingTemplates.enterAValidTemplateID') };
    if (selectedId === '') return { definition: null, problem: t('platform.renderingTemplates.chooseARenderingBundleTo') };
    const { criteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) return { definition: null, problem };
    return { definition: { key, name, resourceType: 'workload', driverType: 'score-k8s', driverInputs: { values: { variables: { render_bundle: selectedId, ...(git ? { render_inputs: inputs } : {}) } } }, criteria }, problem: '' };
  })();

  const templateBundle = (template: Definition) => {
    const id = bundleOf(template);
    const known = verifiedItems.find((item) => item.id === id);
    if (!id) return t('platform.renderingTemplates.unknown');
    if (!known?.source) return id;
    return `${bundleLabel(known)}${known.status === 'AVAILABLE' ? '' : ` (${t('platform.renderingTemplates.unavailableStatus')})`}`;
  };
  const technical = (item: RenderBundle) => item.source ? <details><summary>{t('platform.renderingTemplates.technicalDetails')}</summary><dl className="pin-panel" aria-label={t('platform.renderingTemplates.bundlePin')}>
    <div><dt>Git URL</dt><dd><code>{item.source.url}</code></dd></div><div><dt>{t('platform.renderingTemplates.bundleDirectory')}</dt><dd><code>{item.source.path}</code></dd></div>
    <div><dt>Commit</dt><dd><code>{item.source.revision}</code></dd></div><div><dt>Content SHA-256</dt><dd><code>{item.source.contentSha256}</code></dd></div>
    <div><dt>Engine</dt><dd><code>{item.engineContract}</code> · <code>{item.renderer?.type} {item.renderer?.version}</code></dd></div></dl></details> : null;

  const listError = definitionList.loadError;
  const saveBlocked = saving || review !== null || (sourceMode === 'git' ? !inspected : (listBlocked && !selected) || selectedMissing || unavailable) || inputsInvalid;
  return <section className="page"><header className="page-header"><div><p className="eyebrow">{t('platform.renderingTemplates.platform')}</p><h1>{t('platform.renderingTemplates.renderingTemplates')}</h1><p>{t('platform.renderingTemplates.everyWorkloadNeedsExactlyOne')}</p></div></header>
    <section className="content-panel" aria-label={t('platform.renderingTemplates.registeredRenderingTemplates')}><div className="section-header"><h2>{t('platform.renderingTemplates.registeredTemplates')}</h2></div>{listError ? <div className="form-error" role="alert" aria-label={t('platform.renderingTemplates.listLoadError')}>{listError} <Button type="button" onClick={() => { void definitionList.reload(); }}>{t('platform.renderingTemplates.retry')}</Button></div> : definitionList.loading && definitionList.items.length === 0 ? <p role="status">{t('platform.renderingTemplates.loadingRenderingTemplates')}</p> : templates.length === 0 ? <p>{t('platform.renderingTemplates.noRenderingTemplatesYet')}</p> : <div className="catalog-list">{templates.map((template) => <div key={template.key} className="catalog-entry" data-definition={template.key}><strong>{template.name || template.key}</strong><span>ID <code>{template.key}</code> · Bundle: {templateBundle(template)} {' '}{t('platform.renderingTemplates.version')}{' '} {template.version ?? '?'} · {template.criteria?.length ?? 0} {' '}{t('platform.renderingTemplates.matchingCriteria')}</span><Button type="button" aria-label={t('platform.renderingTemplates.editTemplateKey', { key: template.key })} disabled={saving} onClick={() => startEdit(template)}>{t('platform.renderingTemplates.edit')}</Button></div>)}</div>}</section>
    <section className="content-panel"><div className="section-header"><div><h2>{editing ? t('platform.renderingTemplates.editRenderingTemplateKey', { key: editing.key }) : t('platform.renderingTemplates.registerRenderingTemplate')}</h2><p>{t('platform.renderingTemplates.theResourceTypeAndDriver')}</p></div>{editing ? <Button type="button" onClick={resetForm} disabled={saving}>{t('platform.renderingTemplates.cancelEditing')}</Button> : null}</div>
      {review ? <div className="form-warning" role="alert" aria-label={t('platform.renderingTemplates.templateNeedsReloading')}><p>{review.kind === 'stale' ? t('platform.renderingTemplates.theTemplateWasChangedBy') : review.created ? t('platform.renderingTemplates.itIsUnknownWhetherThe') : t('platform.renderingTemplates.itIsUnknownWhetherThe2')} {review.created ? t('platform.renderingTemplates.checkOnTheServerIf') : t('platform.renderingTemplates.reloadTheLatestVersionReview')}</p>
        <div className="form-actions"><Button type="button" onClick={() => { void definitionList.reload(); }}>{t('platform.renderingTemplates.reloadList')}</Button><Button type="button" tone="primary" onClick={() => { void reloadLatest(); }}>{review.created ? t('platform.renderingTemplates.checkOnServer') : t('platform.renderingTemplates.reloadLatestVersion')}</Button></div></div> : null}
      <form onSubmit={submit} noValidate aria-label={editing ? t('platform.renderingTemplates.editRenderingTemplate') : t('platform.renderingTemplates.registerRenderingTemplate')}><fieldset className="editor-grid form-fieldset" disabled={saving} aria-busy={saving}>
        <div className="field-grid"><label>{t('platform.renderingTemplates.templateID')}<input value={key} readOnly={editing !== null} aria-readonly={editing !== null ? true : undefined} aria-required="true" aria-invalid={idError ? true : undefined} aria-describedby={idError ? 'template-id-hint template-id-error' : 'template-id-hint'} onChange={(event) => { setKey(event.target.value); setIdError(''); }} /></label>
          <label>{t('platform.renderingTemplates.displayName')}<input value={name} maxLength={maxNameLength} aria-required="true" aria-invalid={nameError ? true : undefined} onChange={(event) => { setName(event.target.value); setNameError(''); }} /></label></div>
        <p id="template-id-hint" className="feature-note">{t('platform.renderingTemplates.theTemplateIDUsesLowercase')}{' '} <code>res_id</code>{t('platform.renderingTemplates.theDisplayNameCanBe')}</p>
        {idError ? <div id="template-id-error" className="form-error" role="alert">{idError}</div> : null}
        {nameError ? <div className="form-error" role="alert">{nameError}</div> : null}
        <section className="catalog-fields" aria-label={t('platform.renderingTemplates.bundleSection')}>
          <div className="definition-card-title"><h4>{t('platform.renderingTemplates.renderingBundle')}</h4>{selected?.source && selected.status === 'AVAILABLE' ? <span className="definition-verified">{t('platform.renderingTemplates.verified')}</span> : null}</div>
          <div className="definition-segmented" role="group" aria-label={t('platform.renderingTemplates.bundleSource')}><Button type="button" aria-pressed={sourceMode === 'git'} onClick={() => chooseSource('git')}>{t('platform.renderingTemplates.addFromGit')}</Button><Button type="button" aria-pressed={sourceMode === 'verified'} onClick={() => chooseSource('verified')}>{t('platform.renderingTemplates.verifiedBundles')}</Button></div>
          {sourceMode === 'git' ? <>
            <GitBundleSource key={gitDraftVersion} draft={gitDraft} onDraftChange={(draft) => {
              if (draft.url !== gitDraft.url || draft.path !== gitDraft.path) setGitSourceToLoad('');
              setGitDraft(draft);
            }} disabled={saving} onResult={(result) => { setInspected(result); setBundleError(''); }} />
            {!inspected ? <p className="feature-note">{t('platform.renderingTemplates.checkTheBundleBeforeSaving')}</p> : null}
            {inspected ? <p className="form-success" role="status">{t('platform.renderingTemplates.verifiedBundle', { name: inspected.name ?? '', version: shortCommit(inspected) })}</p> : null}
            {inspected ? technical(inspected) : null}
          </> : <>
            <label>{t('platform.renderingTemplates.renderingBundle')}<select value={bundle} disabled={listBlocked && !selected} aria-required="true" aria-invalid={bundleError || selectedMissing ? true : undefined} aria-describedby="template-bundle-hint" onChange={(event) => { setBundle(event.target.value); setBundleError(''); setRepairError(''); }}>
              {verifiedItems.length !== 1 || bundle === '' ? <option value="">{bundleList.loading ? t('platform.renderingTemplates.loadingBundles') : t('platform.renderingTemplates.chooseARenderingBundle2')}</option> : null}
              {selectedMissing ? <option value={bundle}>{bundle} ({listBlocked ? t('platform.renderingTemplates.availabilityNotConfirmed') : t('platform.renderingTemplates.noLongerAvailable')})</option> : null}
              {savedDetail && savedDetail.id === bundle && !verifiedItems.some((item) => item.id === bundle) ? <option value={savedDetail.id}>{bundleLabel(savedDetail)}</option> : null}
              {verifiedItems.map((item) => <option key={item.id} value={item.id}>{bundleLabel(item)}{item.status === 'AVAILABLE' ? '' : ` (${t('platform.renderingTemplates.unavailableStatus')})`}</option>)}
            </select></label>
            <details className="feature-note" id="template-bundle-hint"><summary>{t('platform.renderingTemplates.help')}</summary><p>{t('platform.renderingTemplates.chooseABundleInstalledOn')}</p></details>
            {bundleList.loadError ? <div className="form-error" role="alert" aria-label={t('platform.renderingTemplates.bundleLoadError')}>{bundleList.loadError} {' '}{t('platform.renderingTemplates.theEnteredDataIsKept')}{' '} <Button type="button" onClick={() => { void bundleList.reload(); }}>{t('platform.renderingTemplates.retryLoadingBundles')}</Button></div> : bundleList.loading ? <p role="status">{t('platform.renderingTemplates.loadingInstalledBundles')}</p> : verifiedItems.length === 0 ? <div className="form-warning" role="status" aria-label={t('platform.renderingTemplates.noBundles')}>{t('platform.renderingTemplates.noVerifiedBundleYet')}{' '} <Button type="button" onClick={() => { void bundleList.reload(); }}>{t('platform.renderingTemplates.refreshBundleList')}</Button></div> : <p><Button type="button" onClick={() => { void bundleList.reload(); }}>{t('platform.renderingTemplates.refreshBundleList')}</Button></p>}
            {bundleStale ? <div className="form-error" role="alert" aria-label={t('platform.renderingTemplates.bundleNoLongerAvailable')}>{t('platform.renderingTemplates.theSelectedBundleBundleIs', { bundle })}</div> : null}
            {unavailable && selected?.source ? <div className="form-warning" role="alert" aria-label={t('platform.renderingTemplates.bundleUnavailable')}><p>{t('platform.renderingTemplates.theSavedBundleIsUnavailable')}</p><Button type="button" disabled={repairing} onClick={() => { void repairSaved(); }}>{repairing ? t('platform.gitBundleSource.checking') : t('platform.renderingTemplates.reinspect')}</Button>{repairError ? <p className="form-error">{repairError}</p> : null}</div> : null}
            {selected ? technical(selected) : null}
          </>}
          {bundleError ? <div id="template-bundle-error" className="form-error" role="alert">{bundleError}</div> : null}
          {git && mode === 'form' ? <RenderInputsEditor key={selectedId} schema={selected?.inputsSchema} value={inputs} onChange={setInputs} onRawProblems={setRawProblems} disabled={saving} /> : null}
        </section>
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
        <CriteriaEditor draft={criteriaDraft} onChange={setCriteriaDraft} applications={{ items: applicationList.items, loading: applicationList.loading, error: applicationList.loadError, onRetry: () => { void applicationList.reload(); } }} sources={{ items: templates, loading: definitionList.loading, error: definitionList.loadError, onRetry: () => { void definitionList.reload(); } }} resourceType="workload" matching={matching} scopeKey={`bundle:${selectedId}|inputs:${JSON.stringify(git ? inputs : null)}|bundles:${bundleList.loading ? 'loading' : bundleList.loadError ? 'error' : JSON.stringify(verifiedItems.map((item) => [item.id, item.version, item.status]))}|editing:${editing ? `${editing.key}@${editing.version}` : ''}`} excludeKey={editing?.key ?? ''} />
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saveBlocked}>{saving ? (editing ? t('platform.renderingTemplates.saving') : t('platform.renderingTemplates.registering')) : editing ? t('platform.renderingTemplates.saveChanges') : t('platform.renderingTemplates.registerRenderingTemplate')}</Button></div>
      </fieldset></form>
    </section>
  </section>;
}
