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
  descriptor: string; resourceType: string; class: string; scope: string; ownership: 'PLATFORM' | 'ORCHESTRATOR' | 'PREEXISTING'; intent: 'CREATE' | 'UPDATE' | 'RETAIN' | 'BIND' | 'UNVERIFIED';
  definitionKey: string; driverType: string; driverAccount?: string; accountVersion?: number;
  module?: { module: string; sourceId: string; revision: string; subdirectory: string; contentSha256: string };
  providers?: { name: string; kind: string; target: string; connectionKey?: string; region?: string; provider?: string }[];
};
// Safe resolved target of one workload or Kubernetes-bound resource (ADR-016): identities and versions only.
export type Placement = { node: string; cluster: string; clusterDefinition: string; clusterDefinitionVersion: number; account: string; accountVersion: number; namespace?: string; source: string; provider?: string; region?: string };
export type PendingChange = { placements?: Placement[]; rendering?: { definitionKey: string; driverType: string; bundle: { id: string; version: string } }; workloadId: string; action: 'DEPLOY' | 'UPDATE' | 'REMOVE'; delta: Record<string, unknown>; resources: { existing: string[]; new: string[]; unreferenced: string[] }; resourceIntents?: ResourceIntent[]; planHash: string };
export type PendingPreview = { token: string; applicationKey: string; environmentKey: EnvironmentKey; baseDeploymentSetId: string; baseVersion: number; draftVersion: number; routePending?: boolean; configRevisionId?: string; changes: PendingChange[] };
export const previewChanges = (app: string, env: EnvironmentKey) => api<PendingPreview>(`/applications/${encodeURIComponent(app)}/environments/${env}/preview`, { method: 'POST', body: '{}' });
export type DeployReport = { status: 'SUCCEEDED' | 'PARTIAL' | 'FAILED'; results: { workloadId: string; action: string; status: 'SUCCEEDED' | 'FAILED' | 'SKIPPED'; deploymentId?: string; error?: string }[] };
export const deployChanges = (app: string, env: EnvironmentKey, token: string) => api<DeployReport>(`/applications/${encodeURIComponent(app)}/environments/${env}/deploy`, { method: 'POST', body: JSON.stringify({ token }) });

// classes is the computed read projection of GET /resource-types (UC-16 API
// mapping): the provisioning classes the scoped catalog serves. It is never a
// registration field. A valid `classes: []` means "none published"; an absent or
// malformed list, or a malformed body, is unavailable metadata and throws, so
// nothing becomes a fake-empty or guessed choice.
export type ResourceType = { key: string; inputs: { name: string; type: string; required?: boolean }[]; outputs: { name: string; secret?: boolean }[]; classes?: string[] };
export class MetadataError extends Error {}
const isArray = (value: unknown): value is unknown[] => Array.isArray(value);
export function parseResourceTypes(body: unknown): { resourceTypes: ResourceType[] } {
  const list = (body as { resourceTypes?: unknown } | null)?.resourceTypes;
  if (!isArray(list)) throw new MetadataError('resource types are missing');
  return { resourceTypes: list.map((item) => {
    const type = item as { key?: unknown; inputs?: unknown; outputs?: unknown; classes?: unknown } | null;
    if (typeof type !== 'object' || type === null || typeof type.key !== 'string' || type.key === '') throw new MetadataError('resource type is malformed');
    if (!isArray(type.classes) || !type.classes.every((entry) => typeof entry === 'string')) throw new MetadataError('classes are missing or malformed');
    if ((type.inputs != null && !isArray(type.inputs)) || (type.outputs != null && !isArray(type.outputs))) throw new MetadataError('schema is malformed');
    return { key: type.key, inputs: (type.inputs ?? []) as ResourceType['inputs'], outputs: (type.outputs ?? []) as ResourceType['outputs'], classes: type.classes as string[] };
  }) };
}
export const getResourceTypes = async () => parseResourceTypes(await api<unknown>('/resource-types'));
