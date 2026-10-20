import { api } from '../../shared/api/client';
import { toTarget, type APIEnvironment } from '../../shared/api/environment';
import type { EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';

export type RecoveryResult = { operationId: string; kind: string; outcome: string; compensation?: string[]; compensationFailed?: string[] };
export type StoreChoice = { key: string; name: string; provider: string; status: string };
const env = (app: string, key: EnvironmentKey) => `/applications/${encodeURIComponent(app)}/environments/${key}`;
const post = (path: string, body: unknown) => api<never>(path, { method: 'POST', body: JSON.stringify(body) });
export const listStoreChoices = async (): Promise<StoreChoice[]> => (await api<{ secretStores?: StoreChoice[] }>('/secret-store-choices')).secretStores ?? [];
export const setSecretStore = (app: string, key: EnvironmentKey, secretStoreKey: string, expectedVersion: number, expectedConfigVersion: number) =>
  api<{ environment: APIEnvironment; changed: boolean; copiedSecrets: number }>(`${env(app, key)}/secret-store`, { method: 'PUT', body: JSON.stringify({ secretStoreKey, expectedVersion, expectedConfigVersion }) });
export const fetchEnvironment = async (app: string, key: EnvironmentKey): Promise<EnvironmentTarget | undefined> => {
  const response = await api<{ application: { environments?: APIEnvironment[] } }>(`/applications/${encodeURIComponent(app)}`);
  const item = response.application.environments?.find((e) => e.key === key);
  return item ? toTarget(item) : undefined;
};
export const recoverOperation = (app: string, key: EnvironmentKey, operationId: string) => post(`${env(app, key)}/operations/${encodeURIComponent(operationId)}/recover`, { priorExecutionStopped: true }) as unknown as Promise<{ recovery: RecoveryResult }>;
