import { api } from '../../shared/api/client';
import type { EnvironmentKey } from '../../shared/types/application';

export type Workload = { id: string; state?: 'PENDING_UPSERT' | 'PENDING_DELETE'; score?: Record<string, unknown>; ready: boolean; servicePorts?: string[] };
export type WorkloadList = { applicationKey: string; environmentKey: EnvironmentKey; draftVersion: number; workloads: Workload[] };

const base = (app: string, env: EnvironmentKey) => `/applications/${encodeURIComponent(app)}/environments/${env}/workloads`;
const item = (app: string, env: EnvironmentKey, id: string) => `${base(app, env)}/${encodeURIComponent(id)}`;

export const getWorkloads = (app: string, env: EnvironmentKey) => api<WorkloadList>(base(app, env));
export const saveWorkload = (app: string, env: EnvironmentKey, id: string, score: Record<string, unknown>, version: number) => api<WorkloadList>(item(app, env, id), { method: 'PUT', body: JSON.stringify({ score, version }) });
export const deleteWorkload = (app: string, env: EnvironmentKey, id: string, version: number) => api<WorkloadList>(item(app, env, id), { method: 'DELETE', body: JSON.stringify({ version }) });
export const undoWorkloadDelete = (app: string, env: EnvironmentKey, id: string, version: number) => api<WorkloadList>(`${item(app, env, id)}/undo`, { method: 'POST', body: JSON.stringify({ version }) });
export const parseScoreImport = (app: string, env: EnvironmentKey, content: string) => api<{ score: Record<string, unknown> }>(`${base(app, env)}/parse`, { method: 'POST', body: JSON.stringify({ content }) });

export type PendingChange = { rendering?: { definitionKey: string; driverType: string; bundle: { id: string; version: string } }; workloadId: string; action: 'DEPLOY' | 'UPDATE' | 'REMOVE'; delta: Record<string, unknown>; resources: { existing: string[]; new: string[]; unreferenced: string[] }; planHash: string };
export type PendingPreview = { token: string; applicationKey: string; environmentKey: EnvironmentKey; baseDeploymentSetId: string; baseVersion: number; draftVersion: number; routePending?: boolean; configRevisionId?: string; changes: PendingChange[] };
export const previewChanges = (app: string, env: EnvironmentKey) => api<PendingPreview>(`/applications/${encodeURIComponent(app)}/environments/${env}/preview`, { method: 'POST', body: '{}' });
export type DeployReport = { status: 'SUCCEEDED' | 'PARTIAL' | 'FAILED'; results: { workloadId: string; action: string; status: 'SUCCEEDED' | 'FAILED' | 'SKIPPED'; deploymentId?: string; error?: string }[] };
export const deployChanges = (app: string, env: EnvironmentKey, token: string) => api<DeployReport>(`/applications/${encodeURIComponent(app)}/environments/${env}/deploy`, { method: 'POST', body: JSON.stringify({ token }) });

export type ResourceType = { key: string; inputs: { name: string; type: string; required?: boolean }[]; outputs: { name: string; secret?: boolean }[] };
export const getResourceTypes = () => api<{ resourceTypes: ResourceType[] }>('/resource-types');
