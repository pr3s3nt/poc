import { api } from '../../../shared/api/client';

// UC-18 Application deletion (ADR-025). Responses carry identities, stages and
// allowlisted codes only; the server never sends provider text or secret values.

export type InventoryItem = { id: string; descriptor: string; type: string; class: string; status: string; generation: number; blocker?: string };
export type Blocker = { code: string; resourceId?: string; descriptor?: string; operationId?: string };
export type EnvironmentScope = { key: string; workloads: string[]; pendingDrafts: string[]; resources: InventoryItem[]; secretValues: number; deployments: number; blockers: Blocker[] };
export type DeletionResource = { id: string; descriptor: string; outcome: string; remaining?: string[] };
export type DeletionEnvironment = {
  key: string; stage: string; status: string; failureCode?: string; removedWorkloads: string[]; deploymentId?: string;
  cleanupOperationIds: string[]; resources: DeletionResource[]; secretsPurged: number;
};
export type DeletionProgress = {
  operationId: string; applicationId: string; applicationStatus: string; status: string; failureCode?: string; actor: string; attempt: number;
  running: boolean; requestedAt: string; updatedAt: string; finishedAt?: string; environments: DeletionEnvironment[];
};
export type DeletionPreview = {
  token?: string; expiresAt?: string; applicationId: string; status: string; confirmationName: string; environments: EnvironmentScope[];
  kept: { secretStores: string[]; platformResources: number; definitions: string[] }; warnings: string[]; blocked: boolean; progress?: DeletionProgress;
};
export type Admission = { operationId: string; status: string; replayed: boolean };

const base = (app: string) => `/applications/${encodeURIComponent(app)}`;

export const previewApplicationDeletion = (app: string) =>
  api<{ preview: DeletionPreview }>(`${base(app)}/deletion-preview`, { method: 'POST', body: '{}' }).then((r) => r.preview);

export const deleteApplication = (app: string, token: string, confirmationName: string) =>
  api<{ deletion: Admission }>(`${base(app)}/deletions`, { method: 'POST', body: JSON.stringify({ token, confirmationName }) }).then((r) => r.deletion);

export const getApplicationDeletion = (app: string, operationId: string) =>
  api<{ deletion: DeletionProgress }>(`${base(app)}/deletions/${encodeURIComponent(operationId)}`).then((r) => r.deletion);
