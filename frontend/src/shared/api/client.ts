export class ApiError extends Error { constructor(public readonly status: number, message: string, public readonly field?: string) { super(message); } }

// Dispatched when a protected request returns 401 so the shell can end the session.
export const sessionExpiredEvent = 'orchestrator:session-expired';

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, { credentials: 'same-origin', headers: { 'Content-Type': 'application/json', ...init?.headers }, ...init });
  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { error?: string; field?: string };
    if (response.status === 401 && !path.startsWith('/auth/')) window.dispatchEvent(new Event(sessionExpiredEvent));
    throw new ApiError(response.status, body.error ?? 'Request failed.', body.field);
  }
  return response.status === 204 ? undefined as T : response.json() as Promise<T>;
}
