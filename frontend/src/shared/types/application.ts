export type EnvironmentKey = 'staging' | 'production';

export interface Workload {
  id: string;
  name: string;
  status: 'Ready' | 'Draft';
}

/** Safe projection of the operation currently owning an Environment (ADR-012). */
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

/**
 * Safe, nonsecret selections of one Environment (ADR-012): the execution
 * Connection and the workload Secret Store are both editable, versioned and
 * independent; neither is locked.
 */
export interface EnvironmentTarget {
  configured: boolean;
  connectionKey: string;
  connectionName?: string;
  connectionKind?: string;
  profile: string;
  region?: string;
  runtimeStatus: string;
  infrastructureScope: string;
  /** Authoritative Environment version; sent as expectedVersion when saving. */
  version: number;
  secretStoreKey: string;
  secretStoreName?: string;
  targetGeneration: number;
  draftVersion: number;
  /** Runtime resources exist, so a destination change is an explicit transition. */
  runtimeExists: boolean;
  activeOperation?: EnvironmentOperation;
}

export const unconfiguredTarget: EnvironmentTarget = { configured: false, connectionKey: '', profile: '', runtimeStatus: 'UNCONFIGURED', infrastructureScope: 'ENVIRONMENT', version: 1, secretStoreKey: '', targetGeneration: 0, draftVersion: 0, runtimeExists: false };

export interface Application {
  id: string;
  name: string;
  subdomain: string;
  /** Execution target per Environment; the Application itself has none. */
  environments: Record<EnvironmentKey, EnvironmentTarget>;
  workloads: Record<EnvironmentKey, readonly Workload[]>;
}

export const platformDomain = 'example.com';

export function endpointFor(application: Application, environment: EnvironmentKey): string {
  return environment === 'production'
    ? `${application.subdomain}.${platformDomain}`
    : `staging.${application.subdomain}.${platformDomain}`;
}
