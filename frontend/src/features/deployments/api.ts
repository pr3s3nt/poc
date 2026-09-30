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

export type DeploymentDetail = {
  deployment: Deployment;
  deploymentSetId: string;
  planHash: string;
  graph?: { nodes?: { descriptor: string; kind: string }[]; edges?: { consumer: string; provider: string }[] };
  matches?: Record<string, { definitionKey: string; driverType: string }>;
  batches?: string[][];
  resources: { descriptor: string; resourceType: string; definitionKey: string; status: string; outputs: Record<string, unknown> }[];
  workloads: { workloadId: string; status: string; manifestDigest: string; lastDeploymentId: string; observedAt: string }[];
};

function scopePath(app: string, env: EnvironmentKey) {
  return `/applications/${encodeURIComponent(app)}/environments/${encodeURIComponent(env)}/deployments`;
}

export function listDeployments(app: string, env: EnvironmentKey, status?: DeploymentStatus) {
  const query = status ? `?${new URLSearchParams({ status })}` : '';
  return api<{ deployments: Deployment[] }>(`${scopePath(app, env)}${query}`);
}

export function getDeployment(app: string, env: EnvironmentKey, id: string) {
  return api<DeploymentDetail>(`${scopePath(app, env)}/${encodeURIComponent(id)}`);
}
