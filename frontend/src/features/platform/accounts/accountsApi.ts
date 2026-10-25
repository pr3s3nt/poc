import { api, ApiError, isStaleConflict } from '../../../shared/api/client';
import { t } from '../../../shared/i18n';
import type { Summary } from './diagnostics/diagnosticsApi';

// V01 infrastructure account (UC-04 AC-01..12). Public DTO only: the server
// never returns a secret reference, key or kubeconfig, and neither does this file.
export type AccountType = 'AWS' | 'INTERNAL';
export type Account = {
  id?: string; key: string; name: string; type: AccountType; authenticationType: string;
  status: string; version: number;
  config?: { cluster?: string; kubeContext?: string; endpoint?: string; tls?: { mode?: string; serverName?: string }; accountId?: string; principalArn?: string; roleArn?: string; roleId?: string; verificationRegions?: string[] };
  /** Newest diagnostic report; never part of the connection status or version. */
  latestDiagnostic?: Summary | null;
  /** True when the diagnostic history could not be read: not "never checked". */
  diagnosticsUnavailable?: boolean;
  verification?: { verified?: boolean; endpoint?: string; serverVersion?: string; accountId?: string; principalArn?: string; identity?: string; regionCheck?: string; capabilityCheck?: string; verifiedAt?: string; regions?: string[]; capabilities?: string[] };
};
export type KubeconfigContext = { name: string; cluster: string; endpoint: string };

export const maxNameLength = 100;
export const maxRegions = 10;
export const maxKubeconfigBytes = 1 << 20;
const accessKeyShape = /^AKIA[A-Z0-9]{16}$/;
const secretKeyShape = /^[A-Za-z0-9/+]{40}$/;
const regionShape = /^[a-z]{2}(-[a-z]+)+-[0-9]$/;

export const listAccounts = async () => (await api<{ accounts: Account[] }>('/driver-accounts')).accounts ?? [];
export const getAccount = (key: string) => api<Account>(`/driver-accounts/${encodeURIComponent(key)}`);
export const inspectKubeconfig = (kubeconfig: string) => api<{ contexts: KubeconfigContext[] }>('/driver-accounts/internal/inspect', { method: 'POST', body: JSON.stringify({ kubeconfig }) });
export const createEc2Role = (body: { name: string; roleArn: string; verificationRegions: string[] }) => api<Account>('/driver-accounts/aws/ec2-role', { method: 'POST', body: JSON.stringify(body) });
export const createInternal = (body: { name: string; kubeconfig: string; context: string }) => api<Account>('/driver-accounts/internal', { method: 'POST', body: JSON.stringify(body) });
export const createAws = (body: { name: string; accessKeyId: string; secretAccessKey: string; verificationRegions: string[] }) => api<Account>('/driver-accounts/aws', { method: 'POST', body: JSON.stringify(body) });
export const renameAccount = (key: string, body: { name: string; version: number }) => api<Account>(`/driver-accounts/${encodeURIComponent(key)}`, { method: 'PUT', body: JSON.stringify(body) });
export const verifyAccount = (key: string, body: { version: number; verificationRegions?: string[] }) => api<Account>(`/driver-accounts/${encodeURIComponent(key)}/verify`, { method: 'POST', body: JSON.stringify(body) });
export const replaceCredentials = (key: string, body: Record<string, unknown>) => api<Account>(`/driver-accounts/${encodeURIComponent(key)}/credentials`, { method: 'PUT', body: JSON.stringify(body) });

/** Splits on commas and white space, keeping typed order. */
export function parseRegions(text: string): string[] { return text.split(/[\s,;]+/).filter(Boolean); }

export function validateName(raw: string): string {
  const name = raw.trim();
  if (!name) return t('platform.accounts.accountsApi.enterAnAccountName');
  if ([...name].length > maxNameLength) return t('platform.accounts.accountsApi.theAccountNameCanHave', { maxNameLength });
  return '';
}

export function validateRegions(text: string): string {
  const regions = parseRegions(text);
  if (regions.length === 0) return t('platform.accounts.accountsApi.enterAtLeastOneCheck');
  if (regions.length > maxRegions) return t('platform.accounts.accountsApi.atMostMaxRegionsRegionsCan', { maxRegions });
  if (regions.some((region) => !regionShape.test(region))) return t('platform.accounts.accountsApi.invalidRegionNameUseA');
  if (new Set(regions).size !== regions.length) return t('platform.accounts.accountsApi.eachRegionMayBeEntered');
  return '';
}

export function validateRoleArn(value: string): string {
  return /^arn:aws:iam::[0-9]{12}:role\/(?:[A-Za-z0-9+=,.@_-]+\/)*[A-Za-z0-9+=,.@_-]{1,64}$/.test(value) && value.length <= 2048 ? '' : t('platform.accounts.accountsApi.enterAValidIAMRole');
}

export function validateAwsKeys(accessKeyId: string, secretAccessKey: string): string {
  if (!accessKeyShape.test(accessKeyId)) return t('platform.accounts.accountsApi.invalidAccessKeyID20');
  if (!secretKeyShape.test(secretAccessKey)) return t('platform.accounts.accountsApi.invalidSecretAccessKeyExactly');
  return '';
}

export type Operation = 'create' | 'rename' | 'verify' | 'replace' | 'inspect';

/** True when the mutation may or may not have been committed: no HTTP status, or the server itself reports an uncertain commit. */
export const outcomeUnknown = (reason: unknown) => !(reason instanceof ApiError) || reason.code === 'COMMIT_UNCERTAIN';

/**
 * Fixed Vietnamese guidance by operation, status and stable code. Raw server
 * or provider text never reaches the page; ApiError.message is already the
 * safe text for a known code.
 */
export function accountGuidance(operation: Operation, reason: unknown, ec2Role = false): string {
  if (ec2Role && reason instanceof ApiError) {
    if (reason.code === 'ACCOUNT_TARGET_MISMATCH') return t('platform.accounts.accountsApi.theActualIAMRoleDoes');
    if (reason.code === 'VERIFICATION_CREDENTIAL_REJECTED') return t('platform.accounts.accountsApi.couldNotObtainValidTemporary');
    if (reason.code === 'VERIFICATION_UNREACHABLE') return t('platform.accounts.accountsApi.couldNotReachIMDSv2Or');
  }
  if (outcomeUnknown(reason) && operation !== 'inspect') {
    return t('platform.accounts.accountsApi.theConnectionWasInterruptedOr');
  }
  if (!(reason instanceof ApiError)) {
    return operation === 'inspect'
      ? t('platform.accounts.accountsApi.theKubeconfigWasNotChecked')
      : t('platform.accounts.accountsApi.noResponseFromTheServer');
  }
  if (isStaleConflict(reason)) return t('platform.accounts.accountsApi.theAccountChangedSinceIt');
  switch (reason.status) {
    case 401: return t('platform.accounts.accountsApi.yourSessionHasExpiredSign');
    case 403: return t('platform.accounts.accountsApi.theSignedInAccountIs');
    case 404: return t('platform.accounts.accountsApi.thisAccountWasNotFound');
    case 413: return t('platform.accounts.accountsApi.theSubmittedDataIsToo');
    case 503: return t('platform.accounts.accountsApi.thePlatformCredentialStoreIs');
  }
  if (reason.status === 422) {
    const effect = operation === 'verify'
      ? t('platform.accounts.accountsApi.theLastCheckFailedThe')
      : operation === 'replace' ? t('platform.accounts.accountsApi.theOldCredentialIsStill') : t('platform.accounts.accountsApi.theAccountWasNotSaved');
    return `${reason.message} ${effect}`;
  }
  if (reason.status === 400) return t('platform.accounts.accountsApi.messageTheEnteredDataIs', { message: reason.message });
  return reason.status >= 500
    ? t('platform.accounts.accountsApi.theServerFailedWhileProcessing')
    : t('platform.accounts.accountsApi.theOperationCouldNotBe');
}

export function typeLabel(type: AccountType | string): string { return type === 'AWS' ? 'AWS' : type === 'INTERNAL' ? t('platform.accounts.accountsApi.internalExistingCluster') : type; }
export function authLabel(auth: string): string { return auth === 'AWS_EC2_ROLE' ? t('platform.accounts.accountsApi.eC2IAMRole') : auth === 'AWS_ACCESS_KEY' ? 'Access key' : auth === 'KUBECONFIG' ? 'Kubeconfig' : auth; }
export function statusLabel(status: string): string {
  if (status === 'READY') return t('platform.accounts.accountsApi.ready');
  if (status === 'VERIFYING') return t('platform.accounts.accountsApi.notVerified');
  if (status === 'REJECTED') return t('platform.accounts.accountsApi.rejected');
  return status;
}
