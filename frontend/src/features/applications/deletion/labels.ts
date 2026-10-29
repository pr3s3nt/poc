import { t } from '../../../shared/i18n';

const stages: Record<string, string> = {
  REQUESTED: 'applications.deletion.stageRequested',
  REMOVING_WORKLOADS: 'applications.deletion.stageRemovingWorkloads',
  DESTROYING_RESOURCES: 'applications.deletion.stageDestroying',
  VERIFYING: 'applications.deletion.stageVerifying',
  PURGING_SECRETS: 'applications.deletion.stagePurgingSecrets',
  DONE: 'applications.deletion.stageDone',
};

const codes: Record<string, string> = {
  ENVIRONMENT_BUSY: 'applications.deletion.codeEnvironmentBusy',
 SCHEDULE_NOT_SUSPENDED: 'applications.deletion.codeScheduleNotSuspended',
  DELETION_RUNNING: 'applications.deletion.codeDeletionRunning',
  RESOURCE_HELD: 'applications.deletion.codeResourceHeld',
  OWNERSHIP_UNVERIFIED: 'applications.deletion.codeOwnershipUnverified',
  WORKLOAD_REMOVAL_FAILED: 'applications.deletion.codeWorkloadRemovalFailed',
  CLEANUP_INCOMPLETE: 'applications.deletion.codeCleanupIncomplete',
  CLEANUP_REFUSED: 'applications.deletion.codeCleanupRefused',
  VERIFICATION_FAILED: 'applications.deletion.codeVerificationFailed',
  SECRET_PURGE_FAILED: 'applications.deletion.codeSecretPurgeFailed',
  INTERRUPTED: 'applications.deletion.codeInterrupted',
  MATCHING_CRITERIA_TARGET_APPLICATION: 'applications.deletion.codeMatchingCriteria',
};

const statuses: Record<string, string> = {
  RUNNING: 'applications.deletion.statusRunning',
  FAILED: 'applications.deletion.statusFailed',
  SUCCEEDED: 'applications.deletion.statusSucceeded',
};

export const stageLabel = (stage: string) => t(stages[stage] ?? 'applications.deletion.stageUnknown');
export const codeLabel = (code: string) => t(codes[code] ?? 'applications.deletion.codeUnknown');
export const statusLabel = (status: string) => t(statuses[status] ?? 'applications.deletion.statusUnknown');
