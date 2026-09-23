export class ApiError extends Error { constructor(public readonly status: number, message: string) { super(message); } }

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, { credentials: 'same-origin', headers: { 'Content-Type': 'application/json', ...init?.headers }, ...init });
  if (!response.ok) { const body = await response.json().catch(() => ({})); throw new ApiError(response.status, body.error ?? 'Request failed.'); }
  return response.status === 204 ? undefined as T : response.json() as Promise<T>;
}
