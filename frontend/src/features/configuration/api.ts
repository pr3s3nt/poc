import { api } from '../../shared/api/client';
import type { EnvironmentKey } from '../../shared/types/application';

export type KeyKind = 'VARIABLE' | 'SECRET';
export type ConfigKey = { name: string; kind: KeyKind; configured: boolean; value?: string; usedBy: string[] };
// environmentVersion and draftVersion come from the same snapshot as version; they are optional on the
// type so a reply without them is never read as zero.
export type Configuration = { applicationKey: string; environmentKey: EnvironmentKey; revisionId?: string; version: number; environmentVersion?: number; draftVersion?: number; keys: ConfigKey[] };
export type ReferenceGuards = { environmentVersion: number; draftVersion: number };

const base = (app: string, env: EnvironmentKey) => `/applications/${encodeURIComponent(app)}/environments/${env}/configuration`;
const keyPath = (app: string, env: EnvironmentKey, key: string) => `${base(app, env)}/keys/${encodeURIComponent(key)}`;

export const getConfiguration = (app: string, env: EnvironmentKey) => api<Configuration>(base(app, env));
export const putKey = (app: string, env: EnvironmentKey, key: string, kind: KeyKind, value: string, version: number) => api<Configuration>(keyPath(app, env, key), { method: 'PUT', body: JSON.stringify({ kind, value, version }) });
export const renameKey = (app: string, env: EnvironmentKey, key: string, newName: string, version: number, references?: ReferenceGuards) => api<Configuration>(keyPath(app, env, key), { method: 'PATCH', body: JSON.stringify(references ? { newName, version, updateReferences: true, environmentVersion: references.environmentVersion, draftVersion: references.draftVersion } : { newName, version }) });
export const deleteKey = (app: string, env: EnvironmentKey, key: string, version: number) => api<Configuration>(keyPath(app, env, key), { method: 'DELETE', body: JSON.stringify({ version }) });
