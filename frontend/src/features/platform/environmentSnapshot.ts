// Environment values that change matching Preview meaning: identity version,
// generation and current deployment-set pointer.
export type ApiEnvironment = {
  key: string; name?: string; environmentType?: string;
  version?: number; targetGeneration?: number; currentDeploymentSetId?: string;
};

export const snapshotKey = (env: ApiEnvironment): string => JSON.stringify([env.version ?? 0, env.targetGeneration ?? 0, env.currentDeploymentSetId ?? '']);
