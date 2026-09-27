import { api } from '../../shared/api/client';
import type { EnvironmentKey } from '../../shared/types/application';

export type KeyKind = 'VARIABLE' | 'SECRET';
export type ConfigKey = { name: string; kind: KeyKind; configured: boolean; value?: string; usedBy: string[] };
export type Configuration = { applicationKey: string; environmentKey: EnvironmentKey; revisionId?: string; version: number; keys: ConfigKey[] };

const base = (app: string, env: EnvironmentKey) => `/applications/${encodeURIComponent(app)}/environments/${env}/configuration`;
const keyPath = (app: string, env: EnvironmentKey, key: string) => `${base(app, env)}/keys/${encodeURIComponent(key)}`;

export const getConfiguration = (app: string, env: EnvironmentKey) => api<Configuration>(base(app, env));
export const putKey = (app: string, env: EnvironmentKey, key: string, kind: KeyKind, value: string, version: number) => api<Configuration>(keyPath(app, env, key), { method: 'PUT', body: JSON.stringify({ kind, value, version }) });
export const renameKey = (app: string, env: EnvironmentKey, key: string, newName: string, version: number) => api<Configuration>(keyPath(app, env, key), { method: 'PATCH', body: JSON.stringify({ newName, version }) });
export const deleteKey = (app: string, env: EnvironmentKey, key: string, version: number) => api<Configuration>(keyPath(app, env, key), { method: 'DELETE', body: JSON.stringify({ version }) });
