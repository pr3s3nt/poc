import { getJson, postJson } from '../../../shared/api/http';
import type {
  ApplicationsResponse,
  DeploymentCreated,
  DeploymentView,
  DeploymentsResponse,
  ScoreSamplesResponse,
} from './types';

export function fetchApplications(): Promise<ApplicationsResponse> {
  return getJson<ApplicationsResponse>('/applications');
}

export function fetchScoreSamples(): Promise<ScoreSamplesResponse> {
  return getJson<ScoreSamplesResponse>('/score-samples');
}

export function fetchDeployments(applicationKey: string, environmentKey: string): Promise<DeploymentsResponse> {
  const query = new URLSearchParams({ application: applicationKey, environment: environmentKey });
  return getJson<DeploymentsResponse>(`/deployments?${query.toString()}`);
}

export function fetchDeployment(deploymentId: string): Promise<DeploymentView> {
  return getJson<DeploymentView>(`/deployments/${encodeURIComponent(deploymentId)}`);
}

export interface CreateDeploymentInput {
  readonly applicationKey: string;
  readonly environmentKey: string;
  readonly workloadId: string;
  readonly score: unknown;
  readonly actor: string;
}

export function createDeployment(input: CreateDeploymentInput): Promise<DeploymentCreated> {
  return postJson<DeploymentCreated>('/deployments', input);
}
