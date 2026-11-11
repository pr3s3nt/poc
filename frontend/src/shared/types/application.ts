import type { ExecutionRecord } from "../../features/deployments/ExecutionProgress";
export type EnvironmentKey = 'staging' | 'production';

export interface Workload {
  id: string;
  name: string;
  status: 'Ready' | 'Draft';
}

/** Safe projection of the operation currently owning an Environment. */
export interface EnvironmentOperation {
 progress?: ExecutionRecord;
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
	 reconciliationEnabled?: boolean;
	 infrastructureDriftEnabled?: boolean;
  version: number;
  secretStoreKey: string;
  secretStoreName?: string;
  targetGeneration: number;
  draftVersion: number;
  activeOperation?: EnvironmentOperation;
  /** Actual public URLs of ready endpoint resources (ADR-037). */
  endpoints?: readonly EnvironmentEndpoint[];
}

export type EnvironmentEndpoint = { workloadId: string; url: string };

export const emptyEnvironment: EnvironmentTarget = { version: 1, secretStoreKey: '', targetGeneration: 0, draftVersion: 0 };

export interface Application {
  id: string;
  name: string;
  environments: Record<EnvironmentKey, EnvironmentTarget>;
  /** Exact keys advertised by the API; absent only for older callers. */
  availableEnvironments?: readonly string[];
  workloads: Record<EnvironmentKey, readonly Workload[]>;
  /** UC-18 lifecycle; absent reads as ACTIVE. */
  status?: 'ACTIVE' | 'DELETING' | string;
  /** The deletion operation of a DELETING Application. */
  deletionOperationId?: string;
}

/** The first actual endpoint URL of an Environment, if any (ADR-037). */
export function endpointFor(application: Application, environment: EnvironmentKey): string | undefined {
  return application.environments[environment]?.endpoints?.[0]?.url;
}
