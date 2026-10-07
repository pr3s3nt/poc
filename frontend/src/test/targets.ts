import type { EnvironmentTarget } from '../shared/types/application';

/** A configured Environment target for tests (Kubernetes unless overridden). */
export function configuredTarget(connectionKey = 'internal-cluster', overrides: Partial<EnvironmentTarget> = {}): EnvironmentTarget {
  return { configured: true, connectionKey, connectionName: connectionKey, connectionKind: 'KUBERNETES', profile: 'internal-k8s', runtimeStatus: 'READY', infrastructureScope: 'ENVIRONMENT', version: 2, ...overrides };
}

export function bothConfigured(connectionKey = 'internal-cluster') {
  return { staging: configuredTarget(connectionKey), production: configuredTarget(connectionKey) };
}
