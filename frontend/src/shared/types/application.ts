export type EnvironmentKey = 'staging' | 'production';

export interface Workload {
  id: string;
  name: string;
  status: 'Ready' | 'Draft';
}

/**
 * Safe, nonsecret execution binding of one Environment (ADR-011). It is set
 * once in Environment Settings and never changes afterwards.
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
  /** Authoritative Environment version; sent as expectedVersion when setting. */
  version: number;
}

export const unconfiguredTarget: EnvironmentTarget = { configured: false, connectionKey: '', profile: '', runtimeStatus: 'UNCONFIGURED', infrastructureScope: 'ENVIRONMENT', version: 1 };

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
