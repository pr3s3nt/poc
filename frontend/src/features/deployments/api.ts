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

export type DeploymentDetail = {
  deployment: Deployment;
  deploymentSetId: string;
  planHash: string;
  graph?: { nodes?: { descriptor: string; kind: string }[]; edges?: { consumer: string; provider: string }[] };
  matches?: Record<string, { definitionKey: string; driverType: string }>;
  batches?: string[][];
  resources: { descriptor: string; resourceType: string; definitionKey: string; status: string; outputs: Record<string, unknown> }[];
  workloads: { workloadId: string; status: string; lastDeploymentId: string; observedAt: string }[];
};

export function listDeployments(app: string, env: EnvironmentKey) {
  const query = new URLSearchParams({ application: app, environment: env });
  return api<{ deployments: Deployment[] }>(`/deployments?${query}`);
}

export function getDeployment(id: string) {
  return api<DeploymentDetail>(`/deployments/${encodeURIComponent(id)}`);
}
