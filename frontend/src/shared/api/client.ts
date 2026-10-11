import { knownCode, knownField, messageFor, safeParams, type ErrorParams } from './errors';

/**
 * API failure. `code`, `field` and `params` are validated against the Console's
 * own tables; the backend `error` sentence is never read, so `message` is always
 * a safe Vietnamese text for the code or status.
 */
export class ApiError extends Error {
  readonly status: number;
  readonly code?: string;
  readonly field?: string;
  readonly params?: ErrorParams;
  // Every field is validated here, so a manually built or mocked instance can
  // never carry unknown codes, fields or params past the public contract.
  constructor(status: number, code?: unknown, field?: unknown, params?: unknown) {
    const safeStatus = Number.isInteger(status) ? status : 0;
    const safeCode = knownCode(code) ? code : undefined;
    const clean = safeParams(params);
    const safeField = knownField(field) ? field : undefined;
    super(messageFor(safeStatus, safeCode, clean, safeField));
    this.status = safeStatus;
    this.code = safeCode;
    this.field = safeField;
    this.params = clean;
  }
}

// Dispatched when a protected request returns 401 so the shell can end the session.
export const sessionExpiredEvent = 'orchestrator:session-expired';

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, { credentials: 'same-origin', headers: { 'Content-Type': 'application/json', ...init?.headers }, ...init });
  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { code?: unknown; field?: unknown; params?: unknown } | null;
    if (response.status === 401 && !path.startsWith('/auth/')) window.dispatchEvent(new Event(sessionExpiredEvent));
    const source = typeof body === 'object' && body !== null ? body : {};
    throw new ApiError(response.status, source.code, source.field, source.params);
  }
  return response.status === 204 ? undefined as T : response.json() as Promise<T>;
}

/** Safe Vietnamese text for any thrown value; non-ApiError values use `fallback`. */
export function describeError(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

/** Code-first: stale data needs a reload; a code-less 409 is the only status fallback. */
export function isStaleConflict(error: unknown): boolean {
  return error instanceof ApiError && (error.code === 'STALE_VERSION' || error.code === 'STALE_PREVIEW' || (error.code === undefined && error.status === 409));
}

export function isBusy(error: unknown): boolean {
  return error instanceof ApiError && error.code === 'ENVIRONMENT_BUSY';
}
