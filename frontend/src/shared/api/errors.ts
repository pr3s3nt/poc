// Stable public error codes of the backend contract (operation-contracts.md,
// "Public HTTP error contract — T20"). The Console never reads or translates the
// backend `error` sentence: it chooses Vietnamese text from the code.
import { t } from '../i18n';

const messageTable = {
  INVALID_REQUEST: t('shared.errors.theSubmittedDataIsInvalid'),
  INVALID_ID: t('shared.errors.theIDDoesNotFollow'),
  INVALID_VARIABLE: t('shared.errors.invalidVariableNameValueOr'),
  INVALID_SCORE: t('shared.errors.invalidScoreOrResourceParameters'),
  PLANNING_REJECTED: t('shared.errors.thisChangeCannotBePlanned'),
  DUPLICATE: t('shared.errors.thisIdentifierAlreadyExistsIn'),
  UNAUTHORIZED: t('shared.errors.yourSessionHasExpiredOr'),
  INVALID_CREDENTIALS: t('shared.errors.incorrectUsernameOrPassword'),
  FORBIDDEN: t('shared.errors.youAreNotAllowedTo'),
  NOT_FOUND: t('shared.errors.thisItemWasNotFound'),
  REQUEST_TOO_LARGE: t('shared.errors.theSubmittedContentIsToo'),
  STALE_VERSION: t('shared.errors.theDataChangedSinceIt'),
  STALE_PREVIEW: t('shared.errors.thePreviewIsStalePreview'),
  ENVIRONMENT_BUSY: t('shared.errors.theEnvironmentIsBusyWith'),
  CONNECTION_UNAVAILABLE: t('shared.errors.theConnectionIsInvalidOr'),
  COMMIT_UNCERTAIN: t('shared.errors.theServerCouldNotConfirm'),
  ACCOUNT_TARGET_MISMATCH: t('shared.errors.theNewCredentialDoesNot'),
  VERIFICATION_FAILED: t('shared.errors.verificationFailedCheckTheInformation'),
  VERIFICATION_CONTEXT_MISSING: t('shared.errors.verificationFailedTheContextIs'),
  VERIFICATION_UNREACHABLE: t('shared.errors.verificationFailedCouldNotReach'),
  VERIFICATION_PERMISSION_DENIED: t('shared.errors.verificationFailedMissingPermissionsRequired'),
  VERIFICATION_CREDENTIAL_REJECTED: t('shared.errors.verificationFailedTheCredentialsWere'),
  VERIFICATION_REGION_UNAVAILABLE: t('shared.errors.verificationFailedAnAllowedRegion'),
  VERIFICATION_CAPABILITY_UNVERIFIABLE: t('shared.errors.verificationFailedProvisioningPermissionsCould'),
  VERIFICATION_PRINCIPAL_UNSUPPORTED: t('shared.errors.verificationFailedThisCredentialType'),
  KUBECONFIG_INVALID: t('shared.errors.theKubeconfigFileIsInvalid'),
  KUBECONFIG_NO_VALID_CONTEXT: t('shared.errors.theKubeconfigHasNoUsable'),
  KUBECONFIG_CONTEXT_REQUIRED: t('shared.errors.chooseAContextInThe'),
  KUBECONFIG_CONTEXT_NOT_FOUND: t('shared.errors.theSelectedContextIsNot'),
  KUBECONFIG_UNSUPPORTED_AUTH: t('shared.errors.theContextUsesAnUnsupported'),
  KUBECONFIG_UNSUPPORTED_CLUSTER: t('shared.errors.theClusterUsesAnUnsupported'),
  KUBECONFIG_INVALID_SERVER: t('shared.errors.theClusterServerAddressMust'),
  KUBECONFIG_INVALID_TLS: t('shared.errors.theTLSDataEmbeddedIn'),
  SECRET_STORE_REQUIRED: t('shared.errors.chooseASecretStoreIn'),
  SECRET_STORE_UNAVAILABLE: t('shared.errors.theSelectedSecretStoreIs'),
  REFERENCE_REWRITE_UNSAFE: t('shared.errors.referencesCannotBeUpdatedAutomatically'),
  SECRET_COPY_FAILED: t('shared.errors.couldNotCopyTheSecrets'),
  RECOVERY_CONFIRMATION_REQUIRED: t('shared.errors.confirmThatTheInterruptedOperation'),
  CLEANUP_REFUSED: t('shared.errors.theResourceCannotBeDeleted'),
  RESOURCE_CONFIRMATION_MISMATCH: t('shared.errors.theReEnteredNameDoes'),
  NOT_RECOVERABLE: t('shared.errors.thisOperationCannotBeRecovered'),
  RECOVERY_INCOMPLETE: t('shared.errors.recoveryIsIncompleteTheEnvironment'),
  OPERATION_LOST: t('shared.errors.theOperationWasInterruptedCheck'),
  ROUTE_RECONCILE_FAILED: t('shared.errors.theWorkloadWasAppliedBut'),
  SERVICE_UNAVAILABLE: t('shared.errors.aRequiredServiceIsNot'),
  INTERNAL_ERROR: t('shared.errors.theServerHitAnUnknown'),
  RENDER_SOURCE_INVALID: t('shared.errors.renderSourceInvalid'),
  RENDER_BUNDLE_INVALID: t('shared.errors.renderBundleInvalid'),
  RENDER_INPUT_INVALID: t('shared.errors.renderInputInvalid'),
  RENDER_SOURCE_UNAVAILABLE: t('shared.errors.renderSourceUnavailable'),
  RENDER_BUNDLE_UNAVAILABLE: t('shared.errors.renderBundleUnavailable'),
  RENDER_BUNDLE_DRIFT: t('shared.errors.renderBundleDrift'),
  RENDER_CONTRACT_UNSUPPORTED: t('shared.errors.renderContractUnsupported'),
  RENDER_FAILED: t('shared.errors.renderFailed'),
} as const;
export const errorMessages: Readonly<Record<string, string>> & typeof messageTable = Object.freeze(messageTable);

const fallbackByStatus = (status: number): string => {
  if (status === 401) return messageTable.UNAUTHORIZED;
  if (status === 403) return messageTable.FORBIDDEN;
  if (status === 404) return messageTable.NOT_FOUND;
  if (status === 409) return t('shared.errors.theDataConflictsWithThe');
  if (status === 413) return messageTable.REQUEST_TOO_LARGE;
  if (status >= 500) return messageTable.INTERNAL_ERROR;
  if (status >= 400) return t('shared.errors.theServerDidNotAccept');
  return t('shared.errors.theRequestFailedPleaseTry');
};

export const plannerStages = ['score', 'before-state', 'candidate', 'public-routes', 'service-references', 'catalog', 'graph', 'reference', 'contract', 'schedule'] as const;
export const plannerReasons = ['workload-exists', 'workload-not-current', 'before-mismatch', 'legacy-infrastructure-reference', 'environment-unconfigured', 'renderer-no-match', 'renderer-ambiguous', 'renderer-unavailable'] as const;
export type PlannerStage = (typeof plannerStages)[number];
export type PlannerReason = (typeof plannerReasons)[number];
// Allowlisted refusal reasons of a resource deletion (OC-19); the server sends them under `reason`.
export const cleanupReasons = ['NOT_UNREFERENCED', 'HAS_CONSUMERS', 'PLATFORM_RESOURCE', 'OWNERSHIP_UNVERIFIED', 'ORIGINAL_TARGET_UNAVAILABLE', 'ENVIRONMENT_BUSY', 'INFRASTRUCTURE_UNAVAILABLE', 'METADATA_SYNC_PENDING'] as const;
export type CleanupReason = (typeof cleanupReasons)[number];
export const cleanupReasonLabels: Readonly<Record<CleanupReason, string>> = Object.freeze({
  NOT_UNREFERENCED: t('shared.errors.theResourceIsNotYet'),
  HAS_CONSUMERS: t('shared.errors.workloadsOrOtherResourcesOutside'),
  PLATFORM_RESOURCE: t('shared.errors.thisIsAPlatformResource'),
  OWNERSHIP_UNVERIFIED: t('shared.errors.thereIsNoDurableEvidence'),
  ORIGINAL_TARGET_UNAVAILABLE: t('shared.errors.theOriginalTargetModuleOr'),
  ENVIRONMENT_BUSY: t('shared.errors.theEnvironmentIsBusyWith2'),
  METADATA_SYNC_PENDING: t('shared.errors.synchronizationOfTheResourcesState'),
  INFRASTRUCTURE_UNAVAILABLE: t('shared.errors.theStateServiceModulesCredentials'),
});
export interface ErrorParams { stage?: PlannerStage; reason?: PlannerReason; cleanupReason?: CleanupReason; resourceType?: string; file?: string }

// A bundle-relative file name of a RENDER_BUNDLE_INVALID error.
const bundleFile = /^[A-Za-z0-9_.-]+(\/[A-Za-z0-9_.-]+){0,7}$/;

// A registered resource Type key: lowercase letters, digits and hyphens, at most 63 characters.
const technicalID = /^[a-z0-9][a-z0-9-]{0,62}$/;

export const stageLabels: Readonly<Record<PlannerStage, string>> = Object.freeze({
  score: t('shared.errors.theScoreDoesNotMatch'),
  'before-state': t('shared.errors.theBeforeScoreDoesNot'),
  candidate: t('shared.errors.couldNotBuildACandidate'),
  'public-routes': t('shared.errors.invalidPublicRouteDuplicatePath'),
  'service-references': t('shared.errors.aWorkloadReferencesAService'),
  catalog: t('shared.errors.aRegisteredResourceDefinitionIs'),
  graph: t('shared.errors.couldNotBuildOrMatch'),
  reference: t('shared.errors.couldNotResolveAResource'),
  contract: t('shared.errors.aMatchedResourceDefinitionHas'),
  schedule: t('shared.errors.theResourceGraphHasA'),
});

export const reasonLabels: Readonly<Record<PlannerReason, string>> = Object.freeze({
  'workload-exists': t('shared.errors.theWorkloadIsAlreadyIn'),
  'workload-not-current': t('shared.errors.theWorkloadIsNotIn'),
  'before-mismatch': t('shared.errors.theBeforeScoreDoesNot2'),
  'legacy-infrastructure-reference': t('shared.errors.aResourceDefinitionStillReferences'),
  'environment-unconfigured': t('shared.errors.theEnvironmentHasNoDeployment'),
  'renderer-no-match': t('shared.errors.noRenderingTemplateScoreK8s'),
  'renderer-ambiguous': t('shared.errors.severalRenderingTemplatesMatchThis'),
  'renderer-unavailable': t('shared.errors.theRenderingTemplateNeedsA'),
});

const has = (table: object, key: unknown): key is string => typeof key === 'string' && Object.prototype.hasOwnProperty.call(table, key);

// Field names the Console recognises; everything else is dropped.
const knownFields: ReadonlySet<string> = new Set([
  'name', 'subdomain', 'connectionKey', 'expectedVersion', 'expectedConfigVersion', 'secretStoreKey', 'destinationKey',
  'acknowledgeDowntime', 'priorExecutionStopped', 'resources', 'status', 'scoreBefore', 'scoreAfter', 'version', 'token', 'kubeconfig',
  'type.key', 'type.schema', 'definition.schema', 'definition.driver', 'definition.references', 'definition.key',
  'definition.resourceType', 'definition.driverType', 'definition.criteria',
  'definition.connectionKey', 'definition.driverInputs.values', 'definition.driverInputs.values.source', 'definition.driverInputs.values.provider_bindings', 'driverInputs.values.variables', 'context.applicationId', 'context.environmentId', 'context.resourceId', 'context.class', 'service.publicPort',
  'accessKeyId', 'secretAccessKey', 'allowedRegions', 'verificationRegions', 'confirmationName', 'selectedResourceIds',
  'from', 'to', 'workload', 'metric', 'valueSeconds', 'source', 'effectiveFrom', 'sampleSize', 'measuredFrom', 'measuredTo',
  'definition.driverInputs.values.variables.render_bundle', 'definition.driverInputs.values.variables.render_inputs',
]);

export function knownCode(code: unknown): code is string { return has(errorMessages, code); }
export function knownField(field: unknown): field is string { return typeof field === 'string' && knownFields.has(field); }

export function safeParams(params: unknown): ErrorParams | undefined {
  if (typeof params !== 'object' || params === null || Array.isArray(params)) return undefined;
  const source = params as Record<string, unknown>;
  const out: ErrorParams = {};
  if (typeof source.stage === 'string' && (plannerStages as readonly string[]).includes(source.stage)) out.stage = source.stage as PlannerStage;
  if (typeof source.reason === 'string' && (plannerReasons as readonly string[]).includes(source.reason)) out.reason = source.reason as PlannerReason;
  else if (typeof source.reason === 'string' && (cleanupReasons as readonly string[]).includes(source.reason)) out.cleanupReason = source.reason as CleanupReason;
  if (typeof source.resourceType === 'string' && technicalID.test(source.resourceType)) out.resourceType = source.resourceType;
  if (typeof source.file === 'string' && source.file.length <= 256 && bundleFile.test(source.file)) out.file = source.file;
  return out.stage || out.reason || out.cleanupReason || out.resourceType || out.file ? out : undefined;
}

/** Vietnamese text for a code/status; never uses backend sentences. */
export function messageFor(status: number, code?: string, params?: ErrorParams, field?: string): string {
  const base = (knownCode(code) ? errorMessages[code] : undefined) ?? fallbackByStatus(status);
  if (code === 'INVALID_SCORE' && params?.resourceType) {
    return t('shared.errors.baseParametersOfTheResourceType', { base, resourceType: params.resourceType });
  }
  if (code === 'INVALID_SCORE' && field === 'service.publicPort') {
    return t('shared.errors.baseThePublicPortFieldIs', { base });
  }
  if (code === 'INVALID_SCORE' && field === 'resources') {
    return t('shared.errors.baseAResourceInThe', { base });
  }
  if (code === 'RENDER_BUNDLE_INVALID' && params?.file) {
    return t('shared.errors.renderBundleInvalidFile', { base, file: params.file });
  }
  if (code === 'CLEANUP_REFUSED' && params?.cleanupReason) {
    return t('shared.errors.cannotDeleteValueNothingHas', { value: cleanupReasonLabels[params.cleanupReason] });
  }
  if (code === 'PLANNING_REJECTED' || code === 'INVALID_SCORE') {
    const detail = (params?.reason && reasonLabels[params.reason]) || (params?.stage && stageLabels[params.stage]);
    if (detail) return `${base} ${detail}.`;
  }
  return base;
}

const fieldLabels: Readonly<Record<string, string>> = Object.freeze({
  name: t('shared.errors.name'), subdomain: 'Subdomain', connectionKey: t('shared.errors.connection'), expectedVersion: t('shared.errors.version'), expectedConfigVersion: t('shared.errors.configurationVersion'),
  secretStoreKey: t('shared.errors.secretStore'), destinationKey: t('shared.errors.targetConnection'), acknowledgeDowntime: t('shared.errors.stopTimeConfirmation'), priorExecutionStopped: t('shared.errors.confirmTheOperationHasStopped'),
  status: t('shared.errors.status'), resources: t('shared.errors.resourcesInTheScore'), scoreBefore: t('shared.errors.beforeScore'), scoreAfter: 'Score sau', version: t('shared.errors.version'), token: 'Token', kubeconfig: 'Kubeconfig',
  'type.key': t('shared.errors.resourceTypeID'), 'type.schema': t('shared.errors.inputOutputStructureOfThe'), 'definition.schema': t('shared.errors.resourceDefinition'),
  'definition.driver': t('shared.errors.driverAndParametersOfThe'), 'definition.references': t('shared.errors.referencesInParametersRelatedResource'),
  'definition.key': t('shared.errors.definitionID'), 'definition.resourceType': t('shared.errors.resourceType'),
  'definition.driverType': t('shared.errors.driverType'), 'definition.criteria': t('shared.errors.matchingCriteria'), 'definition.connectionKey': t('shared.errors.connectionOfTheDefinition'),
  'definition.driverInputs.values': t('shared.errors.moduleInputOutputMapping'), 'definition.driverInputs.values.source': t('shared.errors.verifiedModuleSourceSourceRevision'),
  'definition.driverInputs.values.provider_bindings': t('shared.errors.moduleProviderBinding'), 'driverInputs.values.variables': t('shared.errors.definitionParameters'),
  'context.applicationId': t('shared.errors.sampleApplication'), 'context.environmentId': t('shared.errors.sampleEnvironment'), 'context.resourceId': t('shared.errors.sampleResourceID'), 'context.class': t('shared.errors.sampleClass'),
  accessKeyId: 'Access key ID', secretAccessKey: 'Secret access key', allowedRegions: t('shared.errors.allowedRegions'), verificationRegions: t('shared.errors.checkRegions'),
  confirmationName: t('shared.errors.reEnteredName'), selectedResourceIds: t('shared.errors.selectedResources'),
  from: t('shared.errors.fromDate'), to: t('shared.errors.toDate'), workload: 'Workload', metric: t('shared.errors.metric'), valueSeconds: t('shared.errors.leadTime'), source: t('shared.errors.measurementSource'),
  'definition.driverInputs.values.variables.render_bundle': t('shared.errors.renderBundleField'), 'definition.driverInputs.values.variables.render_inputs': t('shared.errors.renderInputsField'),
  effectiveFrom: t('shared.errors.effectiveDate'), sampleSize: t('shared.errors.sampleSize'), measuredFrom: t('shared.errors.measurementPeriod'), measuredTo: t('shared.errors.measurementPeriod'),
});

/** " Trường liên quan: <nhãn>." for a trusted field, otherwise nothing. */
export function fieldNote(field?: string): string {
  return field && Object.prototype.hasOwnProperty.call(fieldLabels, field) ? t('shared.errors.relatedFieldValue', { value: fieldLabels[field] }) : '';
}

// Codes whose page-level text stays status-specific (session, permission, size, duplicate, server).
const statusOwned: ReadonlySet<string> = new Set(['UNAUTHORIZED', 'FORBIDDEN', 'REQUEST_TOO_LARGE', 'DUPLICATE', 'INTERNAL_ERROR', 'SERVICE_UNAVAILABLE', 'NOT_FOUND']);

/** Code-specific guidance for a known code, or undefined so the page falls back to its status text. */
export function specificGuidance(code: string | undefined, message: string, field?: string): string | undefined {
  return code && !statusOwned.has(code) ? `${message}${fieldNote(field)}` : undefined;
}
