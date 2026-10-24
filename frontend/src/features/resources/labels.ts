import { cleanupReasonLabels, type CleanupReason, cleanupReasons } from '../../shared/api/errors';
import { t } from '../../shared/i18n';

// Own-key lookup with a generic fallback: a persisted value outside the table is never shown as text.
const pick = (map: Record<string, string>, key: string, generic: string) => (Object.hasOwn(map, key) ? map[key] ?? generic : generic);

export const resourceStatusLabel = (key: string) => pick({
  PENDING: t('resources.labels.pending'), PROVISIONING: t('resources.labels.creating'), READY: t('resources.labels.ready'), FAILED: t('resources.labels.failed'), UNREFERENCED: t('resources.labels.unreferenced'), INTERRUPTED: t('resources.labels.interrupted'),
  NEEDS_ATTENTION: t('resources.labels.needsAttention'), DESTROYING: t('resources.labels.deleting'), DESTROYED: t('resources.labels.deleted'),
}, key, t('resources.labels.unknownStatus'));

export const scopeLabel = (key: string) => pick({ APPLICATION: t('resources.labels.application'), ENVIRONMENT: t('resources.labels.environment'), WORKLOAD: 'Workload', SHARED: t('resources.labels.shared') }, key, t('resources.labels.unknownScope'));

export const stageLabel = (key: string) => pick({
  REQUESTED: t('resources.labels.requestReceived'), CHECKING: t('resources.labels.checking'), DESTROYING: t('resources.labels.deleting'), VERIFYING: t('resources.labels.verifyingGone'), INTENT: t('resources.labels.intentRecorded'), INIT: t('resources.labels.moduleInitialization'), PLANNING: t('resources.labels.planning'),
  PLANNED: t('resources.labels.planned'), APPLYING: t('resources.labels.applying'), APPLIED: t('resources.labels.applied'), OUTPUTS: t('resources.labels.readingOutputs'), READINESS: t('resources.labels.readinessCheck'), READY: t('resources.labels.ready'), NO_CHANGE: t('resources.labels.noChanges'), DESTROYED: t('resources.labels.deleted'), RECOVERY: t('resources.labels.recovering'),
}, key, t('resources.labels.unknownPhase'));

export const operationStatusLabel = (key: string) => pick({
  ACTIVE: t('resources.labels.running'), RECOVERING: t('resources.labels.recovering'), INTERRUPTED: t('resources.labels.interruptedNeedsAttention'), SUCCEEDED: t('resources.labels.completed'), FAILED: t('resources.labels.failed'), RECOVERED: t('resources.labels.recovered'),
}, key, t('resources.labels.unknownStatus'));

export const outcomeLabel = (key: string) => pick({
  DESTROYED: t('resources.labels.deletedAndVerifiedGone'), DESTROYING: t('resources.labels.deleting'), PENDING: t('resources.labels.pending'), REMAINING: t('resources.labels.notFullyDeletedRemaining'), RETAINED: t('resources.labels.keptNotDeleted'), NEEDS_ATTENTION: t('resources.labels.needsAttention'), UNKNOWN: t('resources.labels.unknown'),
}, key, t('resources.labels.unknown'));

export const reasonCodeLabel = (key: string) => pick({
  NOT_ATTEMPTED: t('resources.labels.notAttemptedBecauseAnEarlier'), RETAINED_CONSUMER: t('resources.labels.keptBecauseAResourceUsing'), NEEDS_ATTENTION: t('resources.labels.outcomeUnknownManualCheckNeeded'),
  INSTANCE_HELD: t('resources.labels.aPreviousAttemptIsUnresolved'), ORIGINAL_TARGET_UNAVAILABLE: t('resources.labels.theOriginalTargetIsUnusable'), DESTRUCTIVE_PLAN: t('resources.labels.theDeletionPlanIsInvalid'),
  NAMESPACE_NOT_OWNED: t('resources.labels.theNamespaceStillHasObjects'), DESTROY_FAILED: t('resources.labels.deletionIncomplete'), UNKNOWN_ERROR: t('resources.labels.unknownError'),
}, key, t('resources.labels.unknownError'));

export const failureLabel = (key: string) => pick({
  CLEANUP_INCOMPLETE: t('resources.labels.theSelectedResourcesWereNot'), CLEANUP_REJECTED: t('resources.labels.theRequestWasRejectedBefore'), CLEANUP_ERROR: t('resources.labels.theDeletionFailed'), CLEANUP_NEEDS_ATTENTION: t('resources.labels.outcomeUnknownTheEnvironmentIs'),
  CLEANUP_FAILED: t('resources.labels.theDeletionDidNotComplete'),
}, key, t('resources.labels.theDeletionDidNotComplete'));

export const consumerKindLabel = (key: string) => pick({
  RESOURCE: t('resources.labels.resource'), WORKLOAD: t('resources.labels.currentWorkload'), PENDING_WORKLOAD: t('resources.labels.workloadPendingDeployment'), DEPLOYMENT_SET: t('resources.labels.currentDeploymentSet'),
}, key, t('resources.labels.unknownConsumer'));

export const refusalLabel = (key: string) => (cleanupReasons as readonly string[]).includes(key) ? cleanupReasonLabels[key as CleanupReason] : t('resources.labels.notEligibleForDeletion');
