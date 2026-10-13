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

// Safe projection of one resource node of a Preview (UC-05 BR-10): no params, resolved inputs, secret or state reference.
export type ResourceIntent = {
  descriptor: string; resourceType: string; class: string; scope: string; ownership: 'PLATFORM' | 'ORCHESTRATOR'; intent: 'CREATE' | 'RETAIN' | 'BIND';
  definitionKey: string; driverType: string;
  module?: { module: string; sourceId: string; revision: string; subdirectory: string; contentSha256: string };
  providers?: { name: string; kind: string; target: string; connectionKey?: string; region?: string; provider?: string }[];
};
export type PendingChange = { rendering?: { definitionKey: string; driverType: string; bundle: { id: string; version: string } }; workloadId: string; action: 'DEPLOY' | 'UPDATE' | 'REMOVE'; delta: Record<string, unknown>; resources: { existing: string[]; new: string[]; unreferenced: string[] }; resourceIntents?: ResourceIntent[]; planHash: string };
export type PendingPreview = { token: string; binding?: { connectionKey: string; executionProfile: string; region: string }; accountId?: string; applicationKey: string; environmentKey: EnvironmentKey; baseDeploymentSetId: string; baseVersion: number; draftVersion: number; routePending?: boolean; configRevisionId?: string; changes: PendingChange[] };
export const previewChanges = (app: string, env: EnvironmentKey) => api<PendingPreview>(`/applications/${encodeURIComponent(app)}/environments/${env}/preview`, { method: 'POST', body: '{}' });
export type DeployReport = { status: 'SUCCEEDED' | 'PARTIAL' | 'FAILED'; results: { workloadId: string; action: string; status: 'SUCCEEDED' | 'FAILED' | 'SKIPPED'; deploymentId?: string; error?: string }[] };
export const deployChanges = (app: string, env: EnvironmentKey, token: string) => api<DeployReport>(`/applications/${encodeURIComponent(app)}/environments/${env}/deploy`, { method: 'POST', body: JSON.stringify({ token }) });

// classes is the computed read projection of GET /resource-types (UC-16 API
// mapping): the provisioning classes the scoped catalog serves. It is never a
// registration field and never guessed when absent.
export type ResourceType = { key: string; inputs: { name: string; type: string; required?: boolean }[]; outputs: { name: string; secret?: boolean }[]; classes?: string[] };
export const getResourceTypes = async () => {
  const body = await api<{ resourceTypes: ResourceType[] }>('/resource-types');
  return { resourceTypes: (body.resourceTypes ?? []).map((type) => ({ ...type, inputs: type.inputs ?? [], outputs: type.outputs ?? [], classes: type.classes ?? [] })) };
};
