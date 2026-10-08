import type { EnvironmentOperation, EnvironmentTarget } from '../types/application';

/** Environment JSON as returned by the Applications API. */
export type APIEnvironment = {
  key: string; version: number; configured?: boolean; connectionKey?: string; connectionName?: string; connectionKind?: string;
  executionProfile?: string; region?: string; runtimeStatus?: string; infrastructureScope?: string;
  secretStoreKey?: string; secretStoreName?: string; targetGeneration?: number; draftVersion?: number; runtimeExists?: boolean;
  activeOperation?: EnvironmentOperation;
};

export function toTarget(item: APIEnvironment): EnvironmentTarget {
  return {
    configured: Boolean(item.configured), connectionKey: item.connectionKey ?? '', connectionName: item.connectionName, connectionKind: item.connectionKind,
    profile: item.executionProfile ?? '', region: item.region, runtimeStatus: item.runtimeStatus ?? 'UNCONFIGURED', infrastructureScope: item.infrastructureScope ?? 'ENVIRONMENT',
    version: item.version, secretStoreKey: item.secretStoreKey ?? '', secretStoreName: item.secretStoreName, targetGeneration: item.targetGeneration ?? 0,
    draftVersion: item.draftVersion ?? 0, runtimeExists: Boolean(item.runtimeExists), activeOperation: item.activeOperation,
  };
}
