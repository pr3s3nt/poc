// Environment DTO fields that change what an exact matching preview means
// (T19): target binding, scope and version. Opaque and non-secret.
export type ApiEnvironment = {
  key: string; name?: string; environmentType?: string; executionProfile?: string;
  version?: number; connectionKey?: string; region?: string; runtimeStatus?: string;
  infrastructureScope?: string; targetGeneration?: number; currentDeploymentSetId?: string;
};

export const snapshotKey = (env: ApiEnvironment): string => JSON.stringify([env.version ?? 0, env.connectionKey ?? '', env.region ?? '', env.runtimeStatus ?? '', env.infrastructureScope ?? '', env.targetGeneration ?? 0, env.currentDeploymentSetId ?? '']);
