import { useEffect, useId, useRef, useState, type FormEvent } from 'react';
import { specificGuidance, fieldNote } from '../../shared/api/errors';
import { api, ApiError, isStaleConflict } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { snapshotKey, type ApiEnvironment } from './environmentSnapshot';
import { CriteriaEditor, draftFromCriteria, draftToCriteria, emptyDraft, type ApplicationOption, type Criterion, type CriteriaDraft } from './CriteriaEditor';
import type { MatchingCandidateInput } from './MatchingPreviewPanel';
import { useCatalogList } from './useCatalogList';
import { own } from './resourceSupport';
import { capabilitiesForType, capabilityKey, isVerifiedPin, loadCapabilities, type Capability } from './capabilities';
import { ParameterSources } from './ParameterSources';
import { InfrastructureConnection } from './InfrastructureConnection';
import { CompanionResources } from './CompanionResources';
import { parseTypeMetadata } from './metadata';
import { MappingEditor } from './MappingEditor';
import { clusterType, emptyMapping, mappingProblems, mappingToObject, needsAccount, objectToMapping, type BindingDraft, type MappingDraft, type TypeContract } from './moduleMapping';
import { draftToVariables, emptyVariableDraft, missingRequired, parseObjectText, variablesToDraft, type JsonObject, type VariableDraft } from './typedVariables';
import { provisionToRows, rowsToProvision, type ProvisionRow } from './typedProvision';
import { GitModuleSource } from './GitModuleSource';
import { listAccounts, typeLabel, type Account } from './accounts/accountsApi';
import { t } from '../../shared/i18n';

type APIApplication = { key: string; name?: string; environments?: ApiEnvironment[] };
// The public Definition DTO (UC-03 V02-01): Name, version and Driver Account; no profile or Connection field.
type Definition = {
  key: string; name?: string; version?: number; resourceType: string; driverType: string; driverAccount?: string;
  driverInputs?: { values?: Record<string, unknown> }; provision?: Record<string, unknown>; criteria?: Partial<Criterion>[];
};
const driverLabels: Record<string, string> = { terraform: t('platform.resourceDefinitions.terraformVerifiedModule'), kubernetes: t('platform.resourceDefinitions.createInClusterKubernetes'), 'existing-cluster': t('platform.resourceDefinitions.existingClusterExistingCluster') };
const idShape = /^[a-z0-9-]+$/;
const regionShape = /^[a-z]{2}(-[a-z]+)+-[0-9]{1,2}$/;
const maxNameLength = 253;

// Draft of one verified module pin: switching pin keeps each pin's own edits
// (UC-03 BR-24); only the submitted pin is reset after a commit.
type Mode = 'typed' | 'json';
type PairDraft = {
  // Driver Account key and literal AWS region of a pair that needs an account (UC-03 V02-03).
  account: string; region: string;
  varMode: Mode; vars: VariableDraft; varsJSON: string; varsNote: string;
  mapMode: Mode; map: MappingDraft; mapJSON: string; mapNote: string;
  provMode: Mode; rows: ProvisionRow[]; provJSON: string; provNote: string;
};
const initialDraft = (): PairDraft => ({ account: '', region: '', varMode: 'typed', vars: emptyVariableDraft(), varsJSON: '{}', varsNote: '', mapMode: 'typed', map: emptyMapping(), mapJSON: '{}', mapNote: '', provMode: 'typed', rows: [], provJSON: '{}', provNote: '' });
type Editing = { key: string; version: number };
// A stale (409) or unknown-outcome save never resubmits by itself: the form keeps the draft and Save stays off until the author reloads explicitly.
// A create whose outcome is unknown (`created` names the submitted ID) is held the same way: no automatic POST replay until the stored key is read back.
type Review = { kind: 'stale' | 'unknown'; created?: string } | null;

// Specific guidance comes from the stable error code; raw backend text is never shown.
function describeFailure(reason: unknown, editing: boolean): string {
  if (!(reason instanceof ApiError)) return editing
    ? t('platform.resourceDefinitions.noResponseFromTheServer')
    : t('platform.resourceDefinitions.noResponseFromTheServer2');
  const field = fieldNote(reason.field);
  const specific = specificGuidance(reason.code, reason.message, reason.field);
  if (specific) return t('platform.resourceDefinitions.theServerRejectedTheDefinition', { specific });
  switch (reason.status) {
    case 400: return t('platform.resourceDefinitions.theServerRejectedThisDefinition', { field });
    case 401: return t('platform.resourceDefinitions.yourSessionHasExpiredPlease');
    case 403: return t('platform.resourceDefinitions.youAreNotAllowedTo');
    case 404: return t('platform.resourceDefinitions.thisDefinitionWasNotFound');
    case 409: return t('platform.resourceDefinitions.theDefinitionIDAlreadyExists');
    case 413: return t('platform.resourceDefinitions.theDefinitionContentIsToo');
    default: return t('platform.resourceDefinitions.theServerFailedWhileSaving');
  }
}
const listFailure = () => t('platform.resourceDefinitions.couldNotLoadDataFrom');
const accountFailure = () => t('platform.resourceDefinitions.couldNotLoadTheInfrastructure');
const capabilityFailure = () => t('platform.resourceDefinitions.couldNotLoadTheVerified');

const loadTypes = async (): Promise<TypeContract[]> => parseTypeMetadata(await api<unknown>('/resource-types'));
const loadDefinitions = async () => (await api<{ resourceDefinitions: Definition[] }>('/resource-definitions')).resourceDefinitions ?? [];
// Real application/environment IDs (keys) for the criteria editor; never derived from names.
const loadApplications = async (): Promise<ApplicationOption[]> => ((await api<{ applications: APIApplication[] }>('/applications')).applications ?? []).map((app) => ({ key: app.key, name: app.name ?? app.key, environments: (app.environments ?? []).map((env) => ({ key: env.key, name: env.name ?? env.key, environmentType: env.environmentType ?? '', snapshotKey: snapshotKey(env) })) }));
const pinLabel = (capability: Capability) => `${capability.module} · ${capability.source?.revision.slice(0, 12) ?? ''}`;
const accountLabel = (account: Account) => (account.name && account.name !== account.key ? `${account.name} (${account.key})` : account.key);
const isObject = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value);

// initialResourceType comes from the Resource Types CTA query (UC-03 UI): it is
// applied once after catalog metadata loads, including Types awaiting their
// first Git module, and never over a choice.
export function ResourceDefinitionsPage({ initialResourceType = '' }: { initialResourceType?: string } = {}) {
  const typeList = useCatalogList(loadTypes, listFailure);
  const capabilityList = useCatalogList(loadCapabilities, capabilityFailure);
  const definitionList = useCatalogList(loadDefinitions, listFailure);
  const applicationList = useCatalogList(loadApplications, listFailure);
  // The account list names the account of every listed Definition and fills the selector, so it always loads.
  const accountList = useCatalogList(listAccounts, accountFailure);
  const types = typeList.items.filter((type) => type.key !== 'workload');
  const definitions = definitionList.items.filter((definition) => definition.resourceType !== 'workload' && definition.driverType !== 'score-k8s');
  const tabId = useId();
  const [view, setView] = useState<'catalog' | 'editor'>(initialResourceType ? 'editor' : 'catalog');
  const [tab, setTab] = useState<'configuration' | 'criteria'>('configuration');
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [idError, setIdError] = useState('');
  const [nameError, setNameError] = useState('');
  const [notice, setNotice] = useState('');
  const [key, setKey] = useState('');
  const [name, setName] = useState('');
  const [resourceType, setResourceType] = useState('');
  const [clusterMethod, setClusterMethod] = useState<'terraform' | 'existing-cluster'>('terraform');
  const [pinChoice, setPinChoice] = useState('');
  const [sourceMode, setSourceMode] = useState<'installed' | 'git'>('git');
  const [gitPin, setGitPin] = useState<Capability | null>(null);
  const [gitDraftVersion, setGitDraftVersion] = useState(0);
  const [drafts, setDrafts] = useState<Record<string, PairDraft>>({});
  const [criteriaDraft, setCriteriaDraft] = useState<CriteriaDraft>(emptyDraft);
  const [editing, setEditing] = useState<Editing | null>(null);
  const [review, setReview] = useState<Review>(null);
  // The stored module of an edited Definition is no longer verified: the replacement pin must then be chosen explicitly, never inferred.
  const [sourceLost, setSourceLost] = useState(false);
  const formRef = useRef<HTMLElement | null>(null);
  useEffect(() => { if (view === 'editor') formRef.current?.closest('.page')?.scrollIntoView?.({ block: 'start' }); }, [view, editing?.key]);

  // The registry metadata is authoritative: while it loads or fails nothing is "supported" and nothing is guessed.
  const metadataReady = !capabilityList.loading && !capabilityList.loadError;
  const typesReady = !typeList.loading && !typeList.loadError;
  const verifiedPins = (typeKey: string) => (metadataReady ? capabilitiesForType(capabilityList.items, typeKey).filter(isVerifiedPin) : []);
  const existingCapability = metadataReady ? capabilityList.items.find((item) => item.resourceType === resourceType && item.resourceType === clusterType && item.driverType === 'existing-cluster') : undefined;
  const existingCluster = resourceType === clusterType && clusterMethod === 'existing-cluster';
  const supportedType = (typeKey: string) => verifiedPins(typeKey).length > 0 || (metadataReady && capabilityList.items.some((item) => item.resourceType === typeKey && typeKey === clusterType && item.driverType === 'existing-cluster'));
  const typePins = verifiedPins(resourceType);
  const typeContract: TypeContract | undefined = types.find((type) => type.key === resourceType);
  const clusterClasses = typeList.items.find((type) => type.key === clusterType)?.classes ?? [];

  const prefilled = useRef(false);
  useEffect(() => {
    if (prefilled.current || typeList.loading || typeList.loadError || capabilityList.loading || capabilityList.loadError) return;
    prefilled.current = true;
    if (initialResourceType !== '' && initialResourceType !== 'workload' && typeList.items.some((type) => type.key === initialResourceType)) setResourceType((current) => (current === '' ? initialResourceType : current));
  }, [typeList.loading, typeList.loadError, typeList.items, capabilityList.loading, capabilityList.loadError, capabilityList.items, initialResourceType]);

  // One verified pin is chosen for the user; several need an explicit choice.
  const pin: Capability | undefined = existingCluster ? existingCapability : sourceMode === 'git' ? (gitPin?.resourceType === resourceType ? gitPin : undefined) : typePins.length === 1 && !sourceLost ? typePins[0] : typePins.find((candidate) => capabilityKey(candidate) === pinChoice);
  const draftKey = pin ? capabilityKey(pin) : '';
  const draft = drafts[draftKey] ?? initialDraft();
  const patchDraft = (change: Partial<PairDraft>) => { if (draftKey) setDrafts((current) => ({ ...current, [draftKey]: { ...initialDraft(), ...current[draftKey], ...change } })); };
  // The cluster source is part of the mapping draft the advanced editor and the JSON mode serialize: one state, one serializer.
  const setBinding = (provider: string, binding: BindingDraft) => patchDraft({ map: { ...draft.map, bindings: { ...draft.map.bindings, [provider]: binding } } });
  // The account requirement is published by the server per pair; the form never infers a driver's needs.
  const accountRequired = pin !== undefined && needsAccount(pin);
  const regionRequired = accountRequired && pin?.account.type === 'AWS';
  const regionName = pin?.account.regionVariable ?? 'region';
  // The region has its own control, so it is not a generic variable of the typed form.
  const variableList = pin ? pin.variables.filter((variable) => !(regionRequired && variable.name === regionName)) : [];
  const chosenAccount = accountRequired ? draft.account : '';
  const chosenRegion = accountRequired ? draft.region.trim() : '';

  // The mapping as the server will receive it, from whichever mode is active.
  const mappingObject = (): { value?: { input_mapping: JsonObject; output_mapping: JsonObject; provider_bindings: JsonObject }; problem?: string } => {
    if (!pin) return { problem: t('platform.resourceDefinitions.chooseAVerifiedModule') };
    if (draft.mapMode === 'typed') return { value: mappingToObject(draft.map, pin) };
    const parsed = parseObjectText(draft.mapJSON, t('platform.resourceDefinitions.moduleMapping'));
    if (!parsed.value) return { problem: parsed.error };
    const extra = Object.keys(parsed.value).find((name) => !['input_mapping', 'output_mapping', 'provider_bindings'].includes(name));
    if (extra) return { problem: t('platform.resourceDefinitions.theModuleMappingAcceptsOnly', { extra }) };
    if (typeof parsed.value.provider_bindings !== 'object' || parsed.value.provider_bindings === null || Array.isArray(parsed.value.provider_bindings)) return { problem: t('platform.resourceDefinitions.theModuleMappingNeedsProvider') };
    return { value: { input_mapping: (parsed.value.input_mapping ?? {}) as JsonObject, output_mapping: (parsed.value.output_mapping ?? {}) as JsonObject, provider_bindings: parsed.value.provider_bindings as JsonObject } };
  };
  const currentMapping = pin ? mappingObject() : {};
  // Variables a required Resource Type input feeds (the param then outranks the fallback and the module default).
  const mapped: Record<string, string> = {};
  const covered = new Set<string>();
  for (const [variable, entry] of Object.entries(currentMapping.value?.input_mapping ?? {})) {
    const param = typeof entry === 'object' && entry !== null ? (entry as { param?: unknown }).param : undefined;
    if (typeof param !== 'string') continue;
    mapped[variable] = param;
    if (typeContract?.inputs.some((input) => input.name === param && input.required)) covered.add(variable);
  }

  function switchVariables(target: Mode) {
    if (!pin || target === draft.varMode) return;
    if (target === 'json') {
      const built = draftToVariables(draft.vars, variableList);
      if (!built.value) { patchDraft({ varsNote: built.problem ?? '' }); return; }
      patchDraft({ varMode: 'json', varsJSON: JSON.stringify(built.value, null, 2), varsNote: '' });
      return;
    }
    const parsed = parseObjectText(draft.varsJSON, t('platform.resourceDefinitions.definitionParameters'));
    if (!parsed.value) { patchDraft({ varsNote: t('platform.resourceDefinitions.valueFixTheJSONBefore', { value: parsed.error ?? '' }) }); return; }
    const converted = variablesToDraft(parsed.value, variableList);
    if (!converted.draft) { patchDraft({ varsNote: converted.reason ?? '' }); return; }
    patchDraft({ varMode: 'typed', vars: converted.draft, varsNote: '' });
  }
  // One advanced toolbar, preserving the existing separate mapping/provision
  // contracts. Never switch just half of the editor on a failed conversion.
  function switchAdvanced(target: Mode) {
    if (!pin || (target === draft.mapMode && target === draft.provMode)) return;
    if (target === 'json') {
      const built = rowsToProvision(draft.rows);
      if (!built.value) { patchDraft({ provNote: built.problem ?? '' }); return; }
      patchDraft({ mapMode: 'json', mapJSON: JSON.stringify(mappingToObject(draft.map, pin), null, 2), mapNote: '', provMode: 'json', provJSON: JSON.stringify(built.value, null, 2), provNote: '' });
      return;
    }
    const parsed = parseObjectText(draft.mapJSON, t('platform.resourceDefinitions.moduleMapping'));
    const mapping = parsed.value ? objectToMapping(parsed.value, pin, typeContract, clusterClasses) : undefined;
    const provision = provisionToRows(draft.provJSON);
    if (!mapping?.draft || !provision.rows) {
      patchDraft({ mapNote: !parsed.value ? t('platform.resourceDefinitions.valueFixTheJSONBefore', { value: parsed.error ?? '' }) : mapping?.reason ?? '', provNote: provision.reason ?? '' });
      return;
    }
    patchDraft({ mapMode: 'typed', map: mapping.draft, mapNote: '', provMode: 'typed', rows: provision.rows, provNote: '' });
  }

  // Every mode serializes to the same driverInputs.values / provision objects.
  function buildInputs(): { values?: Record<string, unknown>; provision?: Record<string, unknown>; problem?: string } {
    if (existingCluster && pin) {
      const provision = draft.provMode === 'typed' ? rowsToProvision(draft.rows) : parseObjectText(draft.provJSON, t('platform.resourceDefinitions.relatedResourceRules'));
      return provision.value ? { values: { variables: {} }, provision: provision.value } : { problem: t('platform.resourceDefinitions.checkTheRelatedResourceRules') };
    }
    if (!pin?.source) return { problem: t('platform.resourceDefinitions.chooseAResourceTypeWith') };
    let variables: Record<string, unknown>, provision: Record<string, unknown>;
    if (draft.varMode === 'typed') {
      const built = draftToVariables(draft.vars, variableList);
      if (!built.value) return { problem: built.problem };
      variables = built.value;
    } else {
      const parsed = parseObjectText(draft.varsJSON, t('platform.resourceDefinitions.definitionParameters'));
      if (!parsed.value) return { problem: parsed.error };
      variables = parsed.value;
    }
    if (accountRequired) {
      if (Object.hasOwn(variables, regionName)) return { problem: t('platform.resourceDefinitions.enterTheAWSRegionIn', { regionName }) };
      if (chosenRegion !== '') variables = { ...variables, [regionName]: chosenRegion };
    }
    const mapping = mappingObject();
    if (!mapping.value) return { problem: mapping.problem };
    if (draft.provMode === 'typed') {
      const built = rowsToProvision(draft.rows);
      if (!built.value) return { problem: built.problem };
      provision = built.value;
    } else {
      const parsed = parseObjectText(draft.provJSON, t('platform.resourceDefinitions.relatedResourceRules'));
      if (!parsed.value) return { problem: parsed.error };
      provision = parsed.value;
    }
    const { sourceId, moduleId, revision, subdirectory, contentSha256 } = pin.source;
    return { values: { source: { source_id: sourceId, module_id: moduleId, revision, subdirectory, content_sha256: contentSha256 }, variables, ...mapping.value }, provision };
  }

  // Guidance before the request; the server checks the same rules again.
  function guidance(): string {
    if (existingCluster && !pin) return t('platform.resourceDefinitions.existingClusterSupportCouldNot');
    if (!pin) return metadataReady ? t('platform.resourceDefinitions.chooseAResourceTypeAnd') : t('platform.resourceDefinitions.theVerifiedModuleCouldNot');
    if (!typesReady || !typeContract) return t('platform.resourceDefinitions.couldNotLoadTheResource');
    if (accountRequired && chosenAccount === '') return t('platform.resourceDefinitions.chooseAReadyTypeLabelAccount', { typeLabel: typeLabel(pin.account.type ?? '') });
    if (regionRequired && !regionShape.test(chosenRegion)) return t('platform.resourceDefinitions.enterTheAWSRegionAs');
    if (existingCluster) return '';
    if (draft.mapMode === 'typed') {
      const problem = mappingProblems(draft.map, pin, typeContract)[0];
      if (problem) return problem.message;
    }
    if (draft.mapMode === 'json') {
      const parsed = parseObjectText(draft.mapJSON, t('platform.resourceDefinitions.moduleMapping'));
      const bindings = parsed.value && isObject(parsed.value.provider_bindings) ? parsed.value.provider_bindings : {};
      for (const binding of Object.values(bindings)) {
        if (isObject(binding) && binding.target === 'environment') return t('platform.resourceDefinitions.kubernetesBindingsToAnEnvironment');
      }
    }
    const uncovered = missingRequired(draft.vars, variableList, covered).map((variable) => variable.name);
    if (draft.varMode === 'typed' && uncovered.length > 0) return t('platform.resourceDefinitions.requiredBeforeSavingUncoveredRequired', { uncovered: uncovered.join(', ') });
    return '';
  }

  function resetForm() {
    setTab('configuration'); setAdvancedOpen(false);
    setSourceMode('git'); setGitPin(null); setGitDraftVersion((version) => version + 1); setClusterMethod('terraform');
    setKey(''); setName(''); setIdError(''); setNameError(''); setError(''); setReview(null); setEditing(null); setSourceLost(false);
    setDrafts({}); setCriteriaDraft(emptyDraft()); setPinChoice('');
  }

  // Loads a stored Definition into the form for editing (UC-03 V02-06): the ID and Resource Type stay fixed.
  function startEdit(definition: Definition) {
    setView('editor');
    setTab('configuration'); setAdvancedOpen(false);
    setSourceMode('installed'); setGitPin(null);
    setClusterMethod(definition.driverType === 'existing-cluster' ? 'existing-cluster' : 'terraform');
    if (definition.driverType === 'existing-cluster') {
      const capability = capabilityList.items.find((item) => item.resourceType === definition.resourceType && item.driverType === 'existing-cluster');
      setKey(definition.key); setName(definition.name ?? ''); setResourceType(definition.resourceType);
      setPinChoice(''); setSourceLost(false); setError(''); setNotice(''); setIdError(''); setNameError(''); setReview(null);
      const next = { ...initialDraft(), account: definition.driverAccount ?? '', provMode: 'json' as const, provJSON: JSON.stringify(definition.provision ?? {}, null, 2) };
      setDrafts(capability ? { [capabilityKey(capability)]: next } : {});
      setCriteriaDraft(draftFromCriteria(definition.criteria)); setEditing({ key: definition.key, version: definition.version ?? 0 });
      return;
    }
    const values = isObject(definition.driverInputs?.values) ? definition.driverInputs.values : {};
    const source = isObject(values.source) ? values.source : {};
    const capability = capabilitiesForType(capabilityList.items, definition.resourceType).filter(isVerifiedPin).find((candidate) =>
      candidate.source?.sourceId === source.source_id && candidate.source?.moduleId === source.module_id && candidate.source?.revision === source.revision
      && candidate.source?.subdirectory === source.subdirectory && candidate.source?.contentSha256 === source.content_sha256);
    setNotice(''); setError(''); setIdError(''); setNameError(''); setReview(null);
    if (!capability) {
      // Nothing of the old module configuration is guessed or carried over: the author must pick a verified module explicitly before Save.
      setKey(definition.key); setName(definition.name ?? ''); setResourceType(definition.resourceType); setPinChoice(''); setDrafts({}); setCriteriaDraft(draftFromCriteria(definition.criteria));
      setEditing({ key: definition.key, version: definition.version ?? 0 }); setSourceLost(true);
      setError(t('platform.resourceDefinitions.theTerraformModuleOfThis'));

      return;
    }
    setSourceLost(false);
    const contract = types.find((type) => type.key === definition.resourceType);
    const accountBound = needsAccount(capability);
    const variables = isObject(values.variables) ? { ...values.variables } : {};
    const regionVariable = capability.account.regionVariable ?? 'region';
    const region = accountBound && typeof variables[regionVariable] === 'string' ? (variables[regionVariable] as string) : '';
    if (accountBound) delete variables[regionVariable];
    const editable = capability.variables.filter((variable) => !(accountBound && variable.name === regionVariable));
    const next = initialDraft();
    next.account = accountBound ? (definition.driverAccount ?? '') : '';
    next.region = region;
    const typedVars = variablesToDraft(variables, editable);
    if (typedVars.draft) next.vars = typedVars.draft; else { next.varMode = 'json'; next.varsJSON = JSON.stringify(variables, null, 2); next.varsNote = typedVars.reason ?? ''; }
    const mappingValue: JsonObject = { input_mapping: isObject(values.input_mapping) ? values.input_mapping : {}, output_mapping: isObject(values.output_mapping) ? values.output_mapping : {}, provider_bindings: isObject(values.provider_bindings) ? values.provider_bindings : {} };
    const typedMap = objectToMapping(mappingValue, capability, contract, clusterClasses);
    if (typedMap.draft) next.map = typedMap.draft; else { next.mapMode = 'json'; next.mapJSON = JSON.stringify(mappingValue, null, 2); next.mapNote = typedMap.reason ?? ''; }
    const provisionText = JSON.stringify(definition.provision ?? {}, null, 2);
    const typedProvision = provisionToRows(provisionText);
    if (typedProvision.rows) next.rows = typedProvision.rows; else { next.provMode = 'json'; next.provJSON = provisionText; next.provNote = typedProvision.reason ?? ''; }
    // A stored section that cannot be represented safely keeps the entire
    // advanced editor in JSON; never serialize its empty fallback form rows.
    if (next.mapMode === 'json' || next.provMode === 'json') {
      next.mapMode = 'json'; next.mapJSON = JSON.stringify(mappingValue, null, 2);
      next.provMode = 'json'; next.provJSON = provisionText;
    }
    setKey(definition.key); setName(definition.name ?? ''); setResourceType(definition.resourceType); setPinChoice(capabilityKey(capability));
    setDrafts({ [capabilityKey(capability)]: next }); setCriteriaDraft(draftFromCriteria(definition.criteria));
    setEditing({ key: definition.key, version: definition.version ?? 0 });

  }

  // Explicit reload after a stale or unknown-outcome save: one GET of the stored Definition replaces the draft.
  // After an unknown CREATE the same GET decides: found means the earlier POST committed (the stored version opens for editing, nothing is
  // re-sent); not found means it rolled back (the draft stays and the hold is released); a failed GET keeps both draft and hold.
  async function reloadLatest() {
    const target = review?.created ?? editing?.key;
    if (!target) return;
    let latest: Definition | undefined;
    try { latest = (await loadDefinitions()).find((definition) => definition.key === target); } catch { setError(t('platform.resourceDefinitions.couldNotReloadTheDefinition')); return; }
    void definitionList.reload();
    if (review?.created) {
      if (!latest) { setReview(null); setError(''); setNotice(t('platform.resourceDefinitions.theServerHasNoDefinition', { target })); return; }
      startEdit(latest);
      setNotice(t('platform.resourceDefinitions.definitionTargetExistsOnThe', { target }));
      return;
    }
    if (!latest) { setError(t('platform.resourceDefinitions.thisDefinitionIsNoLonger')); return; }
    startEdit(latest);
    setNotice(t('platform.resourceDefinitions.theLatestVersionWasReloaded'));
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving || review) return;
    setError(''); setIdError(''); setNameError(''); setNotice('');
    // The backend owns the ID rule (UC-02 BR-05); the ID is checked, never changed.
    if (key === '') { setTab('configuration'); setIdError(t('platform.resourceDefinitions.enterADefinitionID')); return; }
    if (!idShape.test(key)) { setTab('configuration'); setIdError(t('platform.resourceDefinitions.theIDMayContainOnly')); return; }
    if (name.trim() === '') { setTab('configuration'); setNameError(t('platform.resourceDefinitions.enterTheDisplayNameOf')); return; }
    if ([...name].length > maxNameLength) { setTab('configuration'); setNameError(t('platform.resourceDefinitions.theNameCanHaveAt', { maxNameLength })); return; }
    const blocked = guidance();
    if (blocked) { setTab('configuration'); setAdvancedOpen(true); setError(blocked); return; }
    const inputs = buildInputs();
    if (!inputs.values || !inputs.provision) { setTab('configuration'); setAdvancedOpen(true); setError(inputs.problem ?? ''); return; }
    const { criteria: cleanCriteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) { setTab('criteria'); setError(problem); return; }
    const submitted = key;
    const submittedDraft = draftKey;
    const body = { key: submitted, name, resourceType, driverType: pin?.driverType ?? 'terraform', ...(accountRequired ? { driverAccount: chosenAccount } : {}), driverInputs: { values: inputs.values }, provision: inputs.provision, criteria: cleanCriteria };
    setSaving(true);
    if (editing) {
      let saved: Definition;
      try {
        saved = await api<Definition>(`/resource-definitions/${encodeURIComponent(editing.key)}`, { method: 'PUT', body: JSON.stringify({ ...body, version: editing.version }) });
      } catch (reason) {
        setSaving(false);
        if (reason instanceof ApiError && isStaleConflict(reason)) { setReview({ kind: 'stale' }); setError(''); return; }
        const unknown = !(reason instanceof ApiError) || reason.code === 'COMMIT_UNCERTAIN';
        if (unknown) { setReview({ kind: 'unknown' }); void definitionList.reload(); }
        setError(describeFailure(reason, true));
        return;
      }
      // Committed: stay on the edited Definition at its new version; the list reload is a separate, retryable GET.
      setEditing({ key: saved.key, version: saved.version ?? editing.version + 1 });
      setNotice(t('platform.resourceDefinitions.savedDefinitionKeyVersionValue', { key: saved.key, value: saved.version ?? editing.version + 1 }));
      setSaving(false);
      // Definition criteria change the server-computed Resource Type classes.
      await Promise.all([definitionList.reload(), typeList.reload(), capabilityList.reload()]);
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
    // Committed: report it and reset the submitted pin's draft before reloading.
    setNotice(t('platform.resourceDefinitions.registeredResourceDefinitionSubmitted', { submitted })); setSourceMode('git'); setGitPin(null); setGitDraftVersion((version) => version + 1); setClusterMethod('terraform');
    setKey(''); setName(''); setDrafts((current) => { const next = { ...current }; delete next[submittedDraft]; return next; }); setCriteriaDraft(emptyDraft()); setSaving(false);
    await Promise.all([definitionList.reload(), typeList.reload(), capabilityList.reload()]);
  }

  // Full unsaved candidate for the exact matching preview: same shape as the save request, built without side effects.
  const matching = ((): MatchingCandidateInput => {
    if (key === '' || !idShape.test(key)) return { definition: null, problem: t('platform.resourceDefinitions.enterAValidDefinitionID') };
    if (!pin) return { definition: null, problem: t('platform.resourceDefinitions.chooseAResourceTypeAnd2') };
    if (existingCluster && !chosenAccount) return { definition: null, problem: t('platform.resourceDefinitions.chooseAnInternalAccountTo') };
    const inputs = buildInputs();
    if (!inputs.values || !inputs.provision) return { definition: null, problem: inputs.problem ?? '' };
    const { criteria, problem } = draftToCriteria(criteriaDraft);
    if (problem) return { definition: null, problem };
    return { definition: { key, name, resourceType, driverType: pin.driverType, ...(accountRequired && chosenAccount !== '' ? { driverAccount: chosenAccount } : {}), driverInputs: { values: inputs.values }, provision: inputs.provision, criteria }, problem: '' };
  })();
  // Account metadata and the stored catalog version invalidate an earlier preview result.
  const previewScope = `accounts:${accountList.loading ? 'loading' : accountList.loadError ? 'error' : JSON.stringify(accountList.items.map((account) => [account.key, account.status, account.version]))}|editing:${editing ? `${editing.key}@${editing.version}` : ''}`;

  const listError = definitionList.loadError;
  const typeOptionLabel = (typeKey: string) => (capabilityList.loading ? t('platform.resourceDefinitions.typeKeyCheckingSupport', { typeKey }) : capabilityList.loadError ? t('platform.resourceDefinitions.typeKeySupportNotConfirmed', { typeKey }) : supportedType(typeKey) ? typeKey : t('platform.resourceDefinitions.typeKeyNoVerifiedModule', { typeKey }));
  const problems = pin && draft.mapMode === 'typed' ? mappingProblems(draft.map, pin, typeContract) : [];
  const accountName = (accountKey: string) => { const account = accountList.items.find((item) => item.key === accountKey); return account ? accountLabel(account) : accountKey; };
  const reviewInputs = buildInputs();
  const reviewCriteria = draftToCriteria(criteriaDraft);
  const reviewProblem = reviewInputs.problem || reviewCriteria.problem;
  const reviewDocument = reviewProblem ? null : { key, name, resourceType, driverType: pin?.driverType ?? 'terraform', ...(accountRequired ? { driverAccount: chosenAccount } : {}), driverInputs: { values: reviewInputs.values }, provision: reviewInputs.provision, criteria: reviewCriteria.criteria, ...(editing ? { version: editing.version } : {}) };
  const saveDisabled = saving || types.length === 0 || !metadataReady || review !== null || (!existingCluster && sourceMode === 'git' && !gitPin);

  const identityFields = <>
<div className={`field-grid definition-identity-fields${editing ? ' definition-edit-identity' : ''}`}><label>{t('platform.resourceDefinitions.definitionID')}<input value={key} tabIndex={editing ? -1 : undefined} readOnly={editing !== null} aria-readonly={editing !== null ? true : undefined} aria-required="true" aria-invalid={idError ? true : undefined} aria-describedby={idError ? 'definition-id-hint definition-id-error' : 'definition-id-hint'} onChange={(event) => { setKey(event.target.value); setIdError(''); }} /></label>
          <label>{t('platform.resourceDefinitions.displayName')}<input value={name} maxLength={maxNameLength} aria-required="true" aria-invalid={nameError ? true : undefined} aria-describedby={nameError ? 'definition-name-error' : undefined} onChange={(event) => { setName(event.target.value); setNameError(''); }} /></label>
          <label>{t('platform.resourceDefinitions.resourceType')}<select value={resourceType} aria-required="true" disabled={editing !== null} onChange={(event) => { setResourceType(event.target.value); setPinChoice(''); setClusterMethod('terraform'); }}><option value="">{typeList.loading && types.length === 0 ? t('platform.resourceDefinitions.loadingResourceTypes') : t('platform.resourceDefinitions.chooseAResourceType')}</option>{types.map((type) => <option key={type.key} value={type.key} disabled={!typesReady || !metadataReady}>{typeOptionLabel(type.key)}</option>)}</select></label></div>
        <details className="definition-help"><summary>{t('platform.resourceDefinitions.aboutTheIDAndDisplay')}</summary><p id="definition-id-hint" className="feature-note">{t('platform.resourceDefinitions.theDefinitionIDIsThe')}<code>res_id</code>{t('platform.resourceDefinitions.usedInTheMatchingCriteria')}{' '} {maxNameLength} {' '}{t('platform.resourceDefinitions.charactersAndCanBeEdited')}</p></details>
        {idError ? <div id="definition-id-error" className="form-error" role="alert">{idError}</div> : null}
        {nameError ? <div id="definition-name-error" className="form-error" role="alert">{nameError}</div> : null}
  </>;

  return <section className={`page definition-page definition-view-${view}`}>
    <div hidden={view !== 'catalog'}>
    <header className="page-header"><div><p className="eyebrow">{t('platform.resourceDefinitions.platform')}</p><h1>{t('platform.resourceDefinitions.resourceDefinitions')}</h1><p>{t('platform.resourceDefinitions.manageHowResourcesAreCreated')}</p></div><div className="form-actions">{editing || key || name ? <Button type="button" onClick={() => setView('editor')}>{t('platform.resourceDefinitions.continueDraft')}</Button> : null}<Button type="button" tone="primary" onClick={() => { if (editing) resetForm(); setNotice(''); setView('editor'); }}>{t('platform.resourceDefinitions.createDefinition')}</Button></div></header>
    <section className="content-panel"><div className="section-header"><h2>{t('platform.resourceDefinitions.registeredDefinitions')}</h2></div>{listError ? <div className="form-error" role="alert" aria-label={t('platform.resourceDefinitions.listLoadError')}>{listError} <Button type="button" onClick={() => { void definitionList.reload(); }}>{t('platform.resourceDefinitions.retry')}</Button></div> : definitionList.loading && definitionList.items.length === 0 ? <p>{t('platform.resourceDefinitions.loadingResourceDefinitions')}</p> : definitions.length === 0 ? <p>{t('platform.resourceDefinitions.noResourceDefinitionsYet')}</p> : <div className="catalog-list">{definitions.map((definition) => <div key={definition.key} className="catalog-entry" data-definition={definition.key}><strong>{definition.name || definition.key}</strong><span>ID <code>{definition.key}</code> · {definition.resourceType} · {own(driverLabels, definition.driverType) ?? definition.driverType}{definition.driverAccount ? t('platform.resourceDefinitions.accountAccountName', { accountName: accountName(definition.driverAccount) }) : ''} {' '}{t('platform.resourceDefinitions.version')}{' '} {definition.version ?? '?'} · {definition.criteria?.length ?? 0} {' '}{t('platform.resourceDefinitions.matchingCriteria')}</span>
      {definition.key !== 'builtin-existing-cluster' ? <Button type="button" aria-label={t('platform.resourceDefinitions.editDefinitionKey', { key: definition.key })} disabled={saving || !metadataReady || !typesReady} onClick={() => startEdit(definition)}>{t('platform.resourceDefinitions.edit')}</Button> : null}</div>)}</div>}</section>
    </div>
    <section className="definition-detail" hidden={view !== 'editor'} ref={formRef} aria-label={editing ? t('platform.resourceDefinitions.editResourceDefinition') : t('platform.resourceDefinitions.registerResourceDefinition')}>
      <nav className="definition-breadcrumb" aria-label={t('platform.resourceDefinitions.definitionPath')}><span>{t('platform.resourceDefinitions.platform')}</span><span>/</span><button type="button" disabled={saving} onClick={() => { setView('catalog'); }}>{t('platform.resourceDefinitions.resourceDefinitions')}</button><span>/</span><span>{editing?.key || t('platform.resourceDefinitions.create')}</span></nav>
      <header className="definition-heading"><div><h1>{name || (editing ? editing.key : t('platform.resourceDefinitions.newResourceDefinition'))} {pin ? <span className="definition-driver">{existingCluster ? t('platform.resourceDefinitions.existingCluster') : 'Terraform'}</span> : null}</h1><p>{key || t('platform.resourceDefinitions.newDraft')} <span>/</span> {' '}{t('platform.resourceDefinitions.resourceType2')}{' '} {resourceType || t('platform.resourceDefinitions.notSelected')}</p></div>{editing ? <span className="definition-version">{t('platform.resourceDefinitions.versionVersion', { version: editing.version })}</span> : null}</header>
      <h2 className="definition-sr-only">{editing ? t('platform.resourceDefinitions.editDefinitionKey', { key: editing.key }) : t('platform.resourceDefinitions.registerResourceDefinition')}</h2>
      {view === 'editor' && listError ? <div className="form-error" role="alert" aria-label={t('platform.resourceDefinitions.listLoadError')}>{listError} <Button type="button" onClick={() => { void definitionList.reload(); }}>{t('platform.resourceDefinitions.retry')}</Button></div> : null}
      {review ? <div className="form-warning" role="alert" aria-label={t('platform.resourceDefinitions.definitionNeedsReloading')}>
        <p>{review.kind === 'stale' ? t('platform.resourceDefinitions.theDefinitionWasChangedBy') : review.created ? t('platform.resourceDefinitions.itIsUnknownWhetherThe') : t('platform.resourceDefinitions.itIsUnknownWhetherThe2')}</p>
        <p>{review.created ? t('platform.resourceDefinitions.checkOnTheServerIf') : t('platform.resourceDefinitions.reloadTheLatestVersionReview')}</p>
        <div className="form-actions"><Button type="button" onClick={() => { void definitionList.reload(); }}>{t('platform.resourceDefinitions.reloadList')}</Button><Button type="button" tone="primary" onClick={() => { void reloadLatest(); }}>{review.created ? t('platform.resourceDefinitions.checkOnServer') : t('platform.resourceDefinitions.reloadLatestVersion')}</Button></div></div> : null}
      <form onSubmit={submit} noValidate><fieldset className="editor-grid form-fieldset" disabled={saving} aria-busy={saving}>
        {typeList.loadError ? <div className="form-error" role="alert" aria-label={t('platform.resourceDefinitions.resourceTypeLoadError')}>{t('platform.resourceDefinitions.couldNotLoadResourceTypes')}{' '} <Button type="button" onClick={() => { void typeList.reload(); }}>{t('platform.resourceDefinitions.retry')}</Button></div> : null}
        {capabilityList.loading ? <p role="status" className="feature-note">{t('platform.resourceDefinitions.loadingVerifiedModuleMetadata')}</p> : null}
        {capabilityList.loadError ? <div className="form-error" role="alert" aria-label={t('platform.resourceDefinitions.moduleMetadataLoadError')}>{capabilityList.loadError} {' '}{t('platform.resourceDefinitions.noModuleCanBeChosen')}{' '} <Button type="button" onClick={() => { void capabilityList.reload(); }}>{t('platform.resourceDefinitions.retry')}</Button></div> : null}
        <div className="tabs definition-tabs" role="tablist" aria-label={t('platform.resourceDefinitions.resourceDefinitionTasks')}>
          {(['configuration', 'criteria'] as const).map((value) => <button key={value} type="button" role="tab" id={`${tabId}-${value}-tab`} aria-controls={`${tabId}-${value}-panel`} aria-selected={tab === value} tabIndex={tab === value ? 0 : -1} className={`tab ${tab === value ? 'tab-active' : ''}`} onClick={() => setTab(value)} onKeyDown={(event) => {
            if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
            event.preventDefault();
            const next = event.key === 'Home' ? 'configuration' : event.key === 'End' ? 'criteria' : value === 'configuration' ? 'criteria' : 'configuration';
            setTab(next); document.getElementById(`${tabId}-${next}-tab`)?.focus();
          }}>{value === 'configuration' ? t('platform.resourceDefinitions.definition') : t('platform.resourceDefinitions.matchingCriteriaCount', { count: criteriaDraft.rows.length })}</button>)}
        </div>
        <div role="tabpanel" id={`${tabId}-configuration-panel`} aria-labelledby={`${tabId}-configuration-tab`} hidden={tab !== 'configuration'} className="definition-configuration">
        <div className="definition-tab-intro"><h2>{t('platform.resourceDefinitions.provisioningMethod')}</h2><p>{t('platform.resourceDefinitions.chooseHowTheResourceIs')}</p></div>
        <div className="definition-layout"><div className="definition-main">
        {!editing ? <section className="definition-card definition-create-identity" aria-label={t('platform.resourceDefinitions.definitionDetails')}>{identityFields}</section> : null}
        {resourceType === clusterType ? <section className="definition-card"><label>{t('platform.resourceDefinitions.clusterProvisioning')}<select value={clusterMethod} onChange={(event) => setClusterMethod(event.target.value as 'terraform' | 'existing-cluster')}><option value="terraform">{t('platform.resourceDefinitions.createAClusterWithTerraform')}</option><option value="existing-cluster" disabled={!existingCapability}>{t('platform.resourceDefinitions.useAnExistingCluster')}</option></select></label></section> : null}
        {existingCluster ? <section className="definition-card"><h3>{t('platform.resourceDefinitions.existingCluster')}</h3><p>{t('platform.resourceDefinitions.chooseAnInternalAccountRegistered')}</p></section> : null}
        <section hidden={existingCluster} className="definition-card definition-module" aria-label={t('platform.resourceDefinitions.terraformModule')}><div className="definition-card-title"><span className="definition-step">1</span><h3>{t('platform.resourceDefinitions.terraformModule')}</h3>{pin ? <span className="definition-verified">{t('platform.resourceDefinitions.verified')}</span> : null}</div>
        <div className="definition-segmented" role="group" aria-label={t('platform.resourceDefinitions.terraformModuleSource')}><Button type="button" aria-pressed={sourceMode === 'git'} onClick={() => setSourceMode('git')}>{t('platform.resourceDefinitions.addFromGit')}</Button><Button type="button" aria-pressed={sourceMode === 'installed'} onClick={() => setSourceMode('installed')}>{t('platform.resourceDefinitions.registeredModules')}</Button></div>
        <GitModuleSource key={gitDraftVersion} resourceType={resourceType} active={sourceMode === 'git'} disabled={saving || !typesReady || !metadataReady} onResult={setGitPin} />
        <div className="field-grid"><label hidden={sourceMode === 'git'}>{t('platform.resourceDefinitions.verifiedModules')}<select value={pin ? capabilityKey(pin) : ''} aria-required="true" disabled={typePins.length <= 1 && !sourceLost} onChange={(event) => setPinChoice(event.target.value)}>
            {typePins.length === 0 ? <option value="">{t('platform.resourceDefinitions.notSelectedChooseAType')}</option> : typePins.length > 1 || sourceLost ? <option value="">{t('platform.resourceDefinitions.chooseAModuleAndVersion')}</option> : null}
            {typePins.map((candidate) => <option key={capabilityKey(candidate)} value={capabilityKey(candidate)}>{pinLabel(candidate)}</option>)}</select></label>
          <label className="definition-sr-only">Driver Type<input aria-label="Driver Type" tabIndex={-1} readOnly value={pin ? own(driverLabels, pin.driverType) ?? pin.driverType : ''} placeholder={t('platform.resourceDefinitions.followsTheSelectedModule')} aria-describedby="driver-type-hint" /></label></div>
        <p id="driver-type-hint" className="definition-sr-only">{t('platform.resourceDefinitions.theDriverTypeComesFrom')}</p>
        {pin?.source ? <details><summary>{t('platform.resourceDefinitions.moduleTechnicalDetails')}</summary><dl className="pin-panel" aria-label={t('platform.resourceDefinitions.verifiedModulePin')}>
          <div><dt>Module</dt><dd><code>{pin.source.moduleId}</code></dd></div><div><dt>{t('platform.resourceDefinitions.source')}</dt><dd><code>{pin.source.sourceId}</code></dd></div>
          <div><dt>Git URL</dt><dd>{pin.source.repositoryUrl ?? t('platform.resourceDefinitions.platformModule')}</dd></div><div><dt>Revision</dt><dd><code>{pin.source.revision}</code></dd></div><div><dt>{t('platform.resourceDefinitions.subdirectory')}</dt><dd><code>{pin.source.subdirectory}</code></dd></div>
          <div><dt>{t('platform.resourceDefinitions.contentSHA256')}</dt><dd><code>{pin.source.contentSha256}</code></dd></div>
          <div><dt>Terraform</dt><dd>{pin.terraformVersion ?? t('platform.resourceDefinitions.unknown')}</dd></div>
          <div><dt>Provider</dt><dd>{pin.providerRequirements.map((provider) => `${provider.name} ${provider.version}`).join(', ') || t('platform.resourceDefinitions.none')}</dd></div></dl></details> : null}
        </section>
        <section className="definition-card definition-parameters"><div className="definition-card-title"><span className="definition-step">2</span><h3>{t('platform.resourceDefinitions.accountSettings')}</h3></div>
        {editing ? identityFields : null}
        {pin ? <section className="catalog-fields connection-fields" aria-label={t('platform.resourceDefinitions.infrastructureConnection')}>
          <div className="section-header"><div><h3>{t('platform.resourceDefinitions.infrastructureConnection')}</h3><p className="feature-note">{t('platform.resourceDefinitions.chooseTheAccountOrAccess')}</p></div></div>
          <InfrastructureConnection capability={pin} accountRequired={accountRequired} accounts={{ items: accountList.items, loading: accountList.loading, error: accountList.loadError, reload: () => { void accountList.reload(); } }}
            account={chosenAccount} region={draft.region} regionRequired={regionRequired} onAccount={(value) => patchDraft({ account: value })} onRegion={(value) => patchDraft({ region: value })}
            mapping={draft.map} mappingEditable={draft.mapMode === 'typed'} clusterClasses={clusterClasses} onBinding={setBinding} problems={problems} disabled={saving} />
        </section> : null}
        {pin && !existingCluster ? <section className="catalog-fields" aria-label={t('platform.resourceDefinitions.definitionParameters')}>
          <div className="section-header"><div><h3>{t('platform.resourceDefinitions.resourceSettings')}</h3><details className="definition-help"><summary>{t('platform.resourceDefinitions.howValuesAreProvided')}</summary><p>{t('platform.resourceDefinitions.eachSettingIsSentOnly')}</p></details></div>
            <div className="definition-segmented" role="group" aria-label={t('platform.resourceDefinitions.parameterInputMode')}><Button type="button" aria-label={t('platform.resourceDefinitions.parametersForm')} aria-pressed={draft.varMode === 'typed'} onClick={() => switchVariables('typed')}>{t('platform.resourceDefinitions.form')}</Button><Button type="button" aria-label={t('platform.resourceDefinitions.parametersAdvancedJSON')} aria-pressed={draft.varMode === 'json'} onClick={() => switchVariables('json')}>{t('platform.resourceDefinitions.advancedJSON')}</Button></div></div>
          {draft.varsNote ? <div className="form-warning" role="note">{draft.varsNote}</div> : null}
          {draft.varMode === 'typed' && missingRequired(draft.vars, variableList, covered).length > 0 ? <p className="feature-note">{t('platform.resourceDefinitions.requiredBeforeSavingNoDefault', { value: missingRequired(draft.vars, variableList, covered).map((variable) => variable.name).join(', ') })}</p> : null}
          {draft.varMode === 'typed' ? <ParameterSources variables={variableList} draft={draft.vars} onChange={(vars) => patchDraft({ vars })} executorOwned={pin.executorOwned} types={types} disabled={saving} mapped={mapped} covered={covered} />
            : <>{pin.executorOwned.length > 0 ? <p className="feature-note">{t('platform.resourceDefinitions.providedByTheSystemNo')}{' '} {pin.executorOwned.map((name) => <code key={name}>{name} </code>)}</p> : null}<label>{t('platform.resourceDefinitions.definitionParametersJSONObject')}<textarea aria-label={t('platform.resourceDefinitions.definitionParametersJSONObject')} value={draft.varsJSON} rows={6} spellCheck={false} aria-describedby="driver-variables-hint" onChange={(event) => patchDraft({ varsJSON: event.target.value, varsNote: '' })} /></label>
              <p id="driver-variables-hint" className="feature-note">{t('platform.resourceDefinitions.onlyParametersDeclaredByThe', { value: accountRequired ? t('platform.resourceDefinitions.theAWSRegionRegionNameIs', { regionName }) : '' })}</p></>}
        </section> : null}
        </section>
        {pin && !existingCluster ? <details className="definition-advanced" open={advancedOpen} onToggle={(event) => setAdvancedOpen(event.currentTarget.open)}>
          <summary>{t('platform.resourceDefinitions.advancedSettingsMappingRelatedResources')}</summary>
          <div className="definition-advanced-toolbar"><p className="feature-note">{t('platform.resourceDefinitions.editTheMappingOrDeclare')}</p><div className="definition-segmented" role="group" aria-label={t('platform.resourceDefinitions.advancedSettingsInputMode')}><Button type="button" aria-label={t('platform.resourceDefinitions.advancedForm')} aria-pressed={draft.mapMode === 'typed'} onClick={() => switchAdvanced('typed')}>{t('platform.resourceDefinitions.form')}</Button><Button type="button" aria-label={t('platform.resourceDefinitions.advancedAdvancedJSON')} aria-pressed={draft.mapMode === 'json'} onClick={() => switchAdvanced('json')}>{t('platform.resourceDefinitions.advancedJSON')}</Button></div></div>
        <section className="catalog-fields" aria-label={t('platform.resourceDefinitions.moduleMapping')}>
          {draft.mapNote ? <div className="form-warning" role="note">{draft.mapNote}</div> : null}
          {!typeContract ? <p className="feature-note" role="status">{t('platform.resourceDefinitions.waitingForTheResourceType')}</p> : draft.mapMode === 'typed'
            ? <MappingEditor capability={pin} type={typeContract} draft={draft.map} onChange={(map) => patchDraft({ map })} problems={problems} disabled={saving} />
            : <><label>{t('platform.resourceDefinitions.moduleMappingJSONObjectInput')}<textarea aria-label={t('platform.resourceDefinitions.moduleMappingJSON')} value={draft.mapJSON} rows={8} spellCheck={false} onChange={(event) => patchDraft({ mapJSON: event.target.value, mapNote: '' })} /></label>
              <p className="feature-note">{t('platform.resourceDefinitions.validJSONIsStillChecked')}{' '} <code>{'{"target": "account"}'}</code>{t('platform.resourceDefinitions.endpointsAndCredentialsAreNot')}</p></>}
        </section>
        <section className="catalog-fields" aria-label={t('platform.resourceDefinitions.relatedResourceRules')}>
          {draft.provMode === 'json' ? <div className="section-header"><div><h3>{t('platform.resourceDefinitions.companionResources')}</h3><p className="feature-note">{t('platform.resourceDefinitions.declareAdditionalResourcesToCreate')}</p></div></div> : null}
          {draft.provNote ? <div className="form-warning" role="note">{draft.provNote}</div> : null}
          {draft.provMode === 'typed' ? <CompanionResources rows={draft.rows} onChange={(rows) => patchDraft({ rows })} parentType={resourceType} types={types} disabled={saving} />
            : <label>{t('platform.resourceDefinitions.relatedResourceRulesJSONObject')}<textarea aria-label={t('platform.resourceDefinitions.relatedResourceRulesJSONObject')} value={draft.provJSON} rows={4} spellCheck={false} onChange={(event) => patchDraft({ provJSON: event.target.value, provNote: '' })} /></label>}
        </section>
        </details> : null}
        </div><aside className="definition-overview" aria-label={t('platform.resourceDefinitions.definitionOverview')}><h3>{t('platform.resourceDefinitions.oVERVIEW')}</h3><dl><div><dt>{t('platform.resourceDefinitions.resourceType')}</dt><dd>{resourceType || t('platform.resourceDefinitions.notSelected')}</dd></div><div><dt>Driver</dt><dd>{pin ? (existingCluster ? t('platform.resourceDefinitions.existingCluster') : 'Terraform') : t('platform.resourceDefinitions.notSelected')}</dd></div><div><dt>Module</dt><dd>{existingCluster ? t('platform.resourceDefinitions.noModuleNeeded') : pin?.module || t('platform.resourceDefinitions.notSelected')}</dd></div><div><dt>{t('platform.resourceDefinitions.version2')}</dt><dd>{existingCluster ? t('platform.resourceDefinitions.notApplicable') : pin?.source?.revision.slice(0, 12) || t('platform.resourceDefinitions.notSelected')}</dd></div></dl><p>{t('platform.resourceDefinitions.chooseWhereThisDefinitionIs')}</p><Button type="button" tone="quiet" onClick={() => setTab('criteria')}>{t('platform.resourceDefinitions.matchingCriteria2')}</Button></aside></div>
        </div>
        <div role="tabpanel" id={`${tabId}-criteria-panel`} aria-labelledby={`${tabId}-criteria-tab`} hidden={tab !== 'criteria'}>
        <CriteriaEditor rules draft={criteriaDraft} onChange={setCriteriaDraft} applications={{ items: applicationList.items, loading: applicationList.loading, error: applicationList.loadError, onRetry: () => { void applicationList.reload(); } }} sources={{ items: definitions, loading: definitionList.loading, error: definitionList.loadError, onRetry: () => { void definitionList.reload(); } }} resourceType={resourceType} matching={matching} scopeKey={previewScope} excludeKey={editing?.key ?? ''} />
        </div>
        <details className="definition-review"><summary>{t('platform.resourceDefinitions.reviewTheEnteredVersionBefore')}</summary>
          {editing ? <p className="feature-note">{t('platform.resourceDefinitions.theIDAndResourceType')}</p> : null}
          <p className="feature-note">{t('platform.resourceDefinitions.currentContentTheServerStill')}</p>
          {reviewProblem ? <p role="status">{t('platform.resourceDefinitions.couldNotBuildTheContent', { reviewProblem })}</p> : <pre aria-label={t('platform.resourceDefinitions.definitionBeingEntered')}>{JSON.stringify(reviewDocument, null, 2)}</pre>}
        </details>
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="definition-savebar"><span>{editing ? t('platform.resourceDefinitions.editingVersionVersion', { version: editing.version }) : t('platform.resourceDefinitions.draftUnsaved')}</span><div className="form-actions">{editing ? <Button type="button" onClick={resetForm}>{t('platform.resourceDefinitions.cancelEditing')}</Button> : null}<Button type="submit" tone="primary" disabled={saveDisabled}>{saving ? (editing ? t('platform.resourceDefinitions.saving') : t('platform.resourceDefinitions.registering')) : editing ? t('platform.resourceDefinitions.saveChanges') : t('platform.resourceDefinitions.registerResourceDefinition')}</Button></div></div>
      </fieldset></form>
    </section>
  </section>;
}
