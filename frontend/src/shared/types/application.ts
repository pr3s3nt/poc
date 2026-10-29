export type EnvironmentKey = 'staging' | 'production';

export interface Workload {
  id: string;
  name: string;
  status: 'Ready' | 'Draft';
}

/** Safe projection of the operation currently owning an Environment. */
export interface EnvironmentOperation {
  id: string;
  kind: string;
  status: 'ACTIVE' | 'INTERRUPTED' | 'RECOVERING' | string;
  stage?: string;
  startedAt: string;
  updatedAt: string;
  heartbeatAt: string;
  failure?: string;
  recoverable: boolean;
}

/** Identity, secret-store selection, draft version and operation only. */
export interface EnvironmentTarget {
  version: number;
  secretStoreKey: string;
  secretStoreName?: string;
  targetGeneration: number;
  draftVersion: number;
  activeOperation?: EnvironmentOperation;
}

export const emptyEnvironment: EnvironmentTarget = { version: 1, secretStoreKey: '', targetGeneration: 0, draftVersion: 0 };

export interface Application {
  id: string;
  name: string;
  subdomain: string;
  environments: Record<EnvironmentKey, EnvironmentTarget>;
  /** Exact keys advertised by the API; absent only for older callers. */
  availableEnvironments?: readonly string[];
  workloads: Record<EnvironmentKey, readonly Workload[]>;
  /** UC-18 lifecycle; absent reads as ACTIVE. */
  status?: 'ACTIVE' | 'DELETING' | string;
  /** The deletion operation of a DELETING Application. */
  deletionOperationId?: string;
}

export const platformDomain = 'example.com';

export function endpointFor(application: Application, environment: EnvironmentKey): string {
  return environment === 'production'
    ? `${application.subdomain}.${platformDomain}`
    : `staging.${application.subdomain}.${platformDomain}`;
}
