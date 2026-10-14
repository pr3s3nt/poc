import { api } from '../../shared/api/client';
import type { EnvironmentKey } from '../../shared/types/application';

export type Deployment = {
  id: string;
  applicationKey: string;
  environmentKey: EnvironmentKey;
  action: 'DEPLOY' | 'UPDATE' | 'REMOVE';
  workloadId: string;
  actorRef: string;
  status: 'PLANNING' | 'PROVISIONING' | 'DEPLOYING' | 'SUCCEEDED' | 'FAILED';
  failureReason?: string;
  startedAt: string;
  finishedAt?: string;
};

export type DeploymentStatus = Deployment['status'];

// One persisted Terraform attempt as the Console may see it: allowlisted codes and the public
// module pin only (ADR-014). The server never sends snapshots, state, physical IDs or recovery data.
export type ResourceAttempt = {
  // Opaque ID of the resource instance (the detail route key); absent when the server did not send a valid one.
  resourceInstanceId?: string;
  descriptor: string; scopeType: string; attempt: number; action: string; ownership: string; stage: string; status: string;
  safeErrorCode?: string; moduleId: string; moduleRevision: string; startedAt: string; updatedAt: string;
};

export type DeploymentDetail = {
  deployment: Deployment;
  deploymentSetId: string;
  planHash: string;
  graph?: { nodes?: { descriptor: string; kind: string }[]; edges?: { consumer: string; provider: string }[] };
  matches?: Record<string, { definitionKey: string; driverType: string }>;
  batches?: string[][];
  resources: { descriptor: string; resourceType: string; definitionKey: string; status: string; outputs: Record<string, unknown> }[];
  workloads: { workloadId: string; status: string; manifestDigest: string; lastDeploymentId: string; observedAt: string }[];
  // Undefined when the response has no valid progress projection: unavailable, never "no attempts".
  resourceProgress?: ResourceAttempt[];
};

function scopePath(app: string, env: EnvironmentKey) {
  return `/applications/${encodeURIComponent(app)}/environments/${encodeURIComponent(env)}/deployments`;
}

export function listDeployments(app: string, env: EnvironmentKey, status?: DeploymentStatus) {
  const query = status ? `?${new URLSearchParams({ status })}` : '';
  return api<{ deployments: Deployment[] }>(`${scopePath(app, env)}${query}`);
}

const text = (value: unknown): value is string => typeof value === 'string';

// Reads only the allowlisted fields. A missing field or any malformed member makes the whole
// projection unavailable instead of silently dropping attempts or showing an empty list.
export function parseResourceProgress(raw: unknown): ResourceAttempt[] | undefined {
  if (!Array.isArray(raw)) return undefined;
  const out: ResourceAttempt[] = [];
  for (const item of raw as unknown[]) {
    if (typeof item !== 'object' || item === null) return undefined;
    const r = item as Record<string, unknown>;
    if (!text(r.descriptor) || !text(r.scopeType) || typeof r.attempt !== 'number' || !Number.isInteger(r.attempt) || !text(r.action) || !text(r.ownership) || !text(r.stage) || !text(r.status) || !text(r.moduleId) || !text(r.moduleRevision) || !text(r.startedAt) || !text(r.updatedAt)) return undefined;
    if (r.safeErrorCode !== undefined && !text(r.safeErrorCode)) return undefined;
    const resourceInstanceId = typeof r.resourceInstanceId === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(r.resourceInstanceId) ? r.resourceInstanceId : undefined;
    out.push({ resourceInstanceId, descriptor: r.descriptor, scopeType: r.scopeType, attempt: r.attempt, action: r.action, ownership: r.ownership, stage: r.stage, status: r.status, safeErrorCode: r.safeErrorCode, moduleId: r.moduleId, moduleRevision: r.moduleRevision, startedAt: r.startedAt, updatedAt: r.updatedAt });
  }
  return out;
}

export async function getDeployment(app: string, env: EnvironmentKey, id: string): Promise<DeploymentDetail> {
  const raw = await api<DeploymentDetail & { resourceProgress?: unknown }>(`${scopePath(app, env)}/${encodeURIComponent(id)}`);
  return { ...raw, resourceProgress: parseResourceProgress(raw.resourceProgress) };
}
