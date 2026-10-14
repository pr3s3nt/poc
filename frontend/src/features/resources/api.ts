import { ApiError, api } from '../../shared/api/client';
import type { EnvironmentKey } from '../../shared/types/application';

// Scoped, safe resource detail and confirmed deletion (UC-09 RES-01..03, UC-07 DEL-*, OC-19).
// Every response is read through a strict parser that keeps only the allowlisted fields: a
// malformed body is an error (never an empty list), unknown stages, statuses, outcomes and
// reasons are replaced by a generic marker, and nothing else the server might send is kept.

export class MalformedResponseError extends Error {
  constructor() { super('Phản hồi từ máy chủ không đúng định dạng nên không được hiển thị.'); }
}

export type Consumer = { kind: string; name: string; resourceId?: string; status?: string };
export type Eligibility = { eligible: boolean; reasons: string[] };
export type CleanupSummary = { operationId: string; status: string; stage: string };
export type AttemptSummary = { number: number; action: string; stage: string; status: string; errorCode?: string; updatedAt: string };

export type Target = { known: boolean; connectionKey?: string; connectionName?: string; connectionKind?: string; profile?: string; accountId?: string; region?: string; generation: number };

export type ResourceDetail = {
  id: string; applicationId: string; environment: string; descriptor: string; type: string; class: string; resourceId: string;
  status: string; scope: { type: string; id: string }; owner: string; generation: number; module: { id: string; revision: string }; target: Target;
  version: number; outputs: Record<string, string>; consumers: Consumer[]; eligibility: Eligibility;
  lastAttempt?: AttemptSummary; cleanup?: CleanupSummary;
};

export type ResourceSummary = { id: string; descriptor: string; type: string; class: string; resourceId: string; status: string; scope: string; generation: number; module: string };

export type Candidate = {
  id: string; descriptor: string; type: string; class: string; status: string; version: number; selectable: boolean;
  reasons: string[]; requiredBy: string[]; consumers: Consumer[];
};

export type DeletionPreview = {
  token: string; expiresAt: string; applicationId: string; environment: string; generation: number; confirmationName: string;
  root: Candidate; candidates: Candidate[];
};

export type Admission = { operationId: string; status: string; stage: string; replayed: boolean };

export type CleanupResource = {
  // The resource's status right now (context only); outcome and verified are what THIS operation recorded.
  id: string; descriptor: string; currentStatus: string; outcome: string; verified: boolean; remaining: string[]; remainingUnknown: boolean; reasonCode?: string;
};

export type CleanupOperation = {
  operationId: string; status: string; stage: string; terminal?: string; allVerified: boolean; authorizationMode: string; actor: string; rootId: string;
  resources: CleanupResource[]; remainingIds: string[]; failureCode?: string; startedAt: string; updatedAt: string; finishedAt?: string;
};

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);
const str = (v: unknown): string => { if (typeof v !== 'string') throw new MalformedResponseError(); return v; };
const optStr = (v: unknown): string | undefined => (v === undefined || v === null || v === '' ? undefined : str(v));
const num = (v: unknown): number => { if (typeof v !== 'number' || !Number.isFinite(v)) throw new MalformedResponseError(); return v; };
const bool = (v: unknown): boolean => { if (typeof v !== 'boolean') throw new MalformedResponseError(); return v; };
const obj = (v: unknown): Record<string, unknown> => { if (!isObject(v)) throw new MalformedResponseError(); return v; };
const arr = (v: unknown): unknown[] => { if (!Array.isArray(v)) throw new MalformedResponseError(); return v; };
const strs = (v: unknown): string[] => arr(v).map(str);

function consumer(v: unknown): Consumer {
  const o = obj(v);
  return { kind: str(o.kind), name: str(o.name), resourceId: optStr(o.resourceId), status: optStr(o.status) };
}

function eligibility(v: unknown): Eligibility {
  const o = obj(v);
  return { eligible: bool(o.eligible), reasons: strs(o.reasons) };
}

function target(v: unknown): Target {
  const o = obj(v);
  return { known: bool(o.known), connectionKey: optStr(o.connectionKey), connectionName: optStr(o.connectionName), connectionKind: optStr(o.connectionKind), profile: optStr(o.profile), accountId: optStr(o.accountId), region: optStr(o.region), generation: num(o.generation) };
}

export function parseResource(raw: unknown): ResourceDetail {
  const o = obj(obj(raw).resource);
  const scope = obj(o.scope);
  const module = obj(o.module);
  const outputs: Record<string, string> = {};
  for (const [key, value] of Object.entries(obj(o.outputs))) {
    // Only plain scalars are shown; the backend marker and every nested value stay hidden.
    outputs[key] = typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean' ? String(value) : '***redacted***';
  }
  const attempt = o.lastAttempt === undefined ? undefined : obj(o.lastAttempt);
  const cleanup = o.cleanup === undefined ? undefined : obj(o.cleanup);
  return {
    id: str(o.id), applicationId: str(o.applicationId), environment: str(o.environment), descriptor: str(o.descriptor), type: str(o.type), class: str(o.class), resourceId: str(o.resourceId),
    status: str(o.status), scope: { type: str(scope.type), id: str(scope.id) }, owner: str(o.owner), generation: num(o.generation), module: { id: str(module.id), revision: str(module.revision) },
    target: target(o.target), version: num(o.version), outputs, consumers: arr(o.consumers).map(consumer), eligibility: eligibility(o.eligibility),
    lastAttempt: attempt ? { number: num(attempt.number), action: str(attempt.action), stage: str(attempt.stage), status: str(attempt.status), errorCode: optStr(attempt.errorCode), updatedAt: str(attempt.updatedAt) } : undefined,
    cleanup: cleanup ? { operationId: str(cleanup.operationId), status: str(cleanup.status), stage: str(cleanup.stage) } : undefined,
  };
}

export function parseResourceList(raw: unknown): ResourceSummary[] {
  return arr(obj(raw).resources).map((item) => {
    const o = obj(item);
    return { id: str(o.id), descriptor: str(o.descriptor), type: str(o.type), class: str(o.class), resourceId: str(o.resourceId), status: str(o.status), scope: str(o.scope), generation: num(o.generation), module: str(o.module) };
  });
}

function candidate(v: unknown): Candidate {
  const o = obj(v);
  return { id: str(o.id), descriptor: str(o.descriptor), type: str(o.type), class: str(o.class), status: str(o.status), version: num(o.version), selectable: bool(o.selectable), reasons: strs(o.reasons), requiredBy: strs(o.requiredBy), consumers: arr(o.consumers).map(consumer) };
}

export function parsePreview(raw: unknown): DeletionPreview {
  const o = obj(obj(raw).preview);
  return {
    token: str(o.token), expiresAt: str(o.expiresAt), applicationId: str(o.applicationId), environment: str(o.environment), generation: num(o.generation),
    confirmationName: str(o.confirmationName), root: candidate(o.root), candidates: arr(o.candidates).map(candidate),
  };
}

export function parseAdmission(raw: unknown): Admission {
  const o = obj(obj(raw).cleanup);
  return { operationId: str(o.operationId), status: str(o.status), stage: str(o.stage), replayed: bool(o.replayed) };
}

export function parseCleanup(raw: unknown): CleanupOperation {
  const o = obj(obj(raw).cleanup);
  return {
    operationId: str(o.operationId), status: str(o.status), stage: str(o.stage), terminal: optStr(o.terminal), allVerified: bool(o.allVerified), authorizationMode: str(o.authorizationMode),
    actor: str(o.actor), rootId: str(o.rootId), remainingIds: strs(o.remainingIds), failureCode: optStr(o.failureCode), startedAt: str(o.startedAt), updatedAt: str(o.updatedAt), finishedAt: optStr(o.finishedAt),
    resources: arr(o.resources).map((item) => {
      const r = obj(item);
      return { id: str(r.id), descriptor: str(r.descriptor), currentStatus: str(r.currentStatus), outcome: str(r.outcome), verified: bool(r.verified), remaining: strs(r.remaining), remainingUnknown: bool(r.remainingUnknown), reasonCode: optStr(r.reasonCode) };
    }),
  };
}

function scopePath(app: string, env: EnvironmentKey) {
  return `/applications/${encodeURIComponent(app)}/environments/${encodeURIComponent(env)}`;
}

export async function listResources(app: string, env: EnvironmentKey): Promise<ResourceSummary[]> {
  return parseResourceList(await api<unknown>(`${scopePath(app, env)}/resources`));
}

export async function getResource(app: string, env: EnvironmentKey, id: string): Promise<ResourceDetail> {
  return parseResource(await api<unknown>(`${scopePath(app, env)}/resources/${encodeURIComponent(id)}`));
}

export async function previewDeletion(app: string, env: EnvironmentKey, id: string): Promise<DeletionPreview> {
  return parsePreview(await api<unknown>(`${scopePath(app, env)}/resources/${encodeURIComponent(id)}/deletion-preview`, { method: 'POST', body: '{}' }));
}

export async function requestDeletion(app: string, env: EnvironmentKey, id: string, body: { token: string; confirmationName: string; selectedResourceIds: string[] }): Promise<Admission> {
  return parseAdmission(await api<unknown>(`${scopePath(app, env)}/resources/${encodeURIComponent(id)}/deletions`, { method: 'POST', body: JSON.stringify(body) }));
}

export async function getCleanup(app: string, env: EnvironmentKey, operationId: string): Promise<CleanupOperation> {
  return parseCleanup(await api<unknown>(`${scopePath(app, env)}/resource-cleanups/${encodeURIComponent(operationId)}`));
}

/** True when the request may have reached the server although no usable reply arrived. */
export function responseLost(error: unknown): boolean {
  return !(error instanceof ApiError);
}
