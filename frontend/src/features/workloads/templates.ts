import type { RuntimePolicy } from './RuntimePolicyFields';
import { api } from '../../shared/api/client';
import type { JsonObject } from '../platform/typedVariables';

export type TemplatePin = { id: string; version: string; sha256: string };
export type WorkloadTemplate = TemplatePin & { kind: 'Deployment' | 'StatefulSet' | 'CronJob'; labels: Record<string, string>; descriptions?: Record<string, string>; inputsSchema: JsonObject; defaults: JsonObject; runtimeDefaults?: { image?: string } };
export type Execution = RuntimePolicy & { version: 1; kind: WorkloadTemplate['kind']; template?: TemplatePin; inputs?: JsonObject; headless?: boolean; volumes?: PodVolume[]; volumeMounts?: LocalVolumeMount[]; secretFiles?: { ref: string; container: string; path: string; mode?: number }[]; mounts?: { resource: string; container: string; path: string; readOnly: boolean; subPath?: string }[]; schedule?: { expression: string; timeZone: string; concurrencyPolicy: string; suspend: boolean; startingDeadlineSeconds?: number; activeDeadlineSeconds?: number; backoffLimit?: number; successfulJobsHistoryLimit?: number; failedJobsHistoryLimit?: number } };
export const templateKey = (t: TemplatePin) => `${t.id}@${t.version}:${t.sha256}`;
export async function getWorkloadTemplates(app: string, env: string): Promise<WorkloadTemplate[]> {
  const result = await api<{ templates: WorkloadTemplate[] }>(`/applications/${encodeURIComponent(app)}/environments/${encodeURIComponent(env)}/workload-templates`);
  if (!Array.isArray(result.templates)) throw new Error('Template metadata unavailable');
  return result.templates;
}

export type PodVolume = { name: string; hostPath?: { path: string; type?: string }; emptyDir?: { medium?: string; sizeLimit?: string }; configMap?: { files: { name: string; value: string }[]; defaultMode?: number } };
export type LocalVolumeMount = { volume: string; container: string; path: string; readOnly: boolean; subPath?: string };
export const customExecution = (): Execution => ({version: 1, kind: 'Deployment'});
