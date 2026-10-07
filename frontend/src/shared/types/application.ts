export type EnvironmentKey = 'staging' | 'production';

export interface Workload {
  id: string;
  name: string;
  status: 'Ready' | 'Draft';
}

export interface Application {
  id: string;
  name: string;
  subdomain: string;
  /** Saved Connection binding shared by both Environments (UC-01 BR-08). */
  connectionKey: string;
  profile: string;
  region?: string;
  workloads: Record<EnvironmentKey, readonly Workload[]>;
}

export const platformDomain = 'example.com';

export function endpointFor(application: Application, environment: EnvironmentKey): string {
  return environment === 'production'
    ? `${application.subdomain}.${platformDomain}`
    : `staging.${application.subdomain}.${platformDomain}`;
}
