import { api } from '../../shared/api/client';
import { toTarget, type APIEnvironment } from '../../shared/api/environment';
import type { EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';

export type StoreChoice = { key: string; name: string; provider: string; status: string; legacy?: boolean };
export type Mapping = { sourceDescriptor: string; destinationDescriptor: string };
export type Mode = 'DEPLOY_NEW' | 'MIGRATE_POSTGRES';
export type Endpoint = { connectionKey: string; connectionName?: string; kind?: string; executionProfile: string; region?: string; generation: number; namespace?: string };
export type TransitionPreview = {
  token: string; mode: Mode; source: Endpoint; destination: Endpoint; workloads: { workloadId: string; action: 'UNCHANGED' | 'UPDATED' | 'ADDED' | 'REMOVED'; configChanged?: boolean }[];
  mappings: Mapping[]; downtimeRequired: boolean; notes: string[];
};
export type StageResult = { stage: string; status: string; message?: string; startedAt: string; finishedAt?: string };
export type TransitionDetail = {
  id: string; operationId: string; mode: Mode; stage: string; status: 'RUNNING' | 'SUCCEEDED' | 'FAILED' | 'INTERRUPTED';
  source: Endpoint; destination: Endpoint; sourceState: string; authority: 'SOURCE' | 'DESTINATION'; mappings?: Mapping[]; stages: StageResult[]; failure?: string;
  compensation?: string[]; compensationFailed?: string[]; backupRetained?: boolean; canCleanupSource: boolean; createdAt: string; updatedAt: string;
};
export type RecoveryResult = { operationId: string; kind: string; outcome: string; compensation?: string[]; compensationFailed?: string[] };

const env = (app: string, key: EnvironmentKey) => `/applications/${encodeURIComponent(app)}/environments/${key}`;
const post = (path: string, body: unknown) => api<never>(path, { method: 'POST', body: JSON.stringify(body) });

export const listStoreChoices = () => api<{ secretStores: StoreChoice[] }>('/secret-store-choices').then((r) => r.secretStores ?? []);
export const listConnectionChoices = () => api<{ connections: { key: string; name: string; kind: string; status: string }[] }>('/application-connections').then((r) => r.connections ?? []);
export const setSecretStore = (app: string, key: EnvironmentKey, secretStoreKey: string, expectedVersion: number, expectedConfigVersion: number) =>
  api<{ environment: APIEnvironment; changed: boolean; copiedSecrets: number }>(`${env(app, key)}/secret-store`, { method: 'PUT', body: JSON.stringify({ secretStoreKey, expectedVersion, expectedConfigVersion }) });
export const fetchEnvironment = async (app: string, key: EnvironmentKey): Promise<EnvironmentTarget | undefined> => {
  const response = await api<{ application: { environments?: APIEnvironment[] } }>(`/applications/${encodeURIComponent(app)}`);
  const item = response.application.environments?.find((e) => e.key === key);
  return item ? toTarget(item) : undefined;
};
export const previewTransition = (app: string, key: EnvironmentKey, destinationKey: string, mode: Mode, mappings?: Mapping[]) =>
  post(`${env(app, key)}/connection-transition/preview`, { destinationKey, mode, mappings }) as unknown as Promise<{ preview: TransitionPreview }>;
export const executeTransition = (app: string, key: EnvironmentKey, destinationKey: string, mode: Mode, token: string, acknowledgeDowntime: boolean, mappings: Mapping[]) =>
  post(`${env(app, key)}/connection-transitions`, { destinationKey, mode, token, acknowledgeDowntime, mappings }) as unknown as Promise<{ transition: TransitionDetail }>;
export const getTransition = (app: string, key: EnvironmentKey, id: string) => api<{ transition: TransitionDetail }>(`${env(app, key)}/connection-transitions/${encodeURIComponent(id)}`).then((r) => r.transition);
export const listTransitions = (app: string, key: EnvironmentKey) => api<{ transitions: TransitionDetail[] }>(`${env(app, key)}/connection-transitions`).then((r) => r.transitions ?? []);
export const cleanupSource = (app: string, key: EnvironmentKey, id: string) => post(`${env(app, key)}/connection-transitions/${encodeURIComponent(id)}/cleanup-source`, {}) as unknown as Promise<{ transition: TransitionDetail }>;
export const recoverOperation = (app: string, key: EnvironmentKey, operationId: string) => post(`${env(app, key)}/operations/${encodeURIComponent(operationId)}/recover`, { priorExecutionStopped: true }) as unknown as Promise<{ recovery: RecoveryResult }>;
