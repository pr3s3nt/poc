import { api } from '../../shared/api/client';
import type { EnvironmentKey } from '../../shared/types/application';

export type PreviewAction = 'deploy' | 'update' | 'remove';

export type ScorePreviewRequest = {
  workloadId: string;
  action: PreviewAction;
  runId: string;
  scoreBefore?: Record<string, unknown>;
  scoreAfter?: Record<string, unknown>;
};

export type JSONPatchOperation = { op: string; path: string; value?: unknown };

export type PreviewDelta = {
  modules?: { add?: Record<string, unknown>; remove?: string[]; update?: Record<string, JSONPatchOperation[]> };
  shared?: JSONPatchOperation[];
};

export type PreviewNode = { descriptor: string; kind: 'workload' | 'resource'; resourceType: string; class: string; origins: string[]; workloadId?: string; paramKeys: string[]; bindings: Record<string, string> };
export type PreviewEdge = { consumer: string; provider: string; reason: string; path?: string };
export type PreviewMatch = { descriptor: string; definitionKey: string; driverType: string; specificity: number };

export type RenderingSelection = { definitionKey: string; driverType: string; bundle: { id: string; version: string; digest: string } };

export type ScorePreview = {
  rendering?: Record<string, RenderingSelection>;
  applicationKey: string;
  environmentKey: EnvironmentKey;
  baseSetId: string;
  baseVersion: number;
  runId: string;
  workloadId: string;
  action: PreviewAction;
  planHash: string;
  delta: PreviewDelta;
  candidateSet: { modules: Record<string, unknown>; shared: Record<string, unknown> };
  graph: { nodes: PreviewNode[]; edges: PreviewEdge[] };
  matches: PreviewMatch[];
  batches: string[][];
  classification: { existing: string[]; new: string[]; unreferenced: string[] };
};

export const previewScore = (app: string, env: EnvironmentKey, request: ScorePreviewRequest) =>
  api<ScorePreview>(`/applications/${encodeURIComponent(app)}/environments/${env}/score-preview`, { method: 'POST', body: JSON.stringify(request) });
