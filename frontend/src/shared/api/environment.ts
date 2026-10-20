import type { EnvironmentOperation, EnvironmentTarget } from '../types/application';

/** Identity-only Environment projection returned by the Applications API. */
export type APIEnvironment = {
  key: string;
  version: number;
  secretStoreKey?: string;
  secretStoreName?: string;
  targetGeneration?: number;
  draftVersion?: number;
  activeOperation?: EnvironmentOperation;
};

export function toTarget(item: APIEnvironment): EnvironmentTarget {
  return {
    version: item.version,
    secretStoreKey: item.secretStoreKey ?? '',
    secretStoreName: item.secretStoreName,
    targetGeneration: item.targetGeneration ?? 0,
    draftVersion: item.draftVersion ?? 0,
    activeOperation: item.activeOperation,
  };
}
