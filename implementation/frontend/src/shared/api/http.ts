// Typed same-origin transport for the orchestrator JSON API. The console never
// talks to any other origin and never sends credentials.
const API_BASE = '/api/v1';

export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function parseBody(response: Response): Promise<unknown> {
  const text = await response.text();
  if (text.length === 0) {
    return null;
  }
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return text;
  }
}

function messageOf(status: number, body: unknown): string {
  if (body && typeof body === 'object' && 'error' in body) {
    const value = (body as { error?: unknown }).error;
    if (typeof value === 'string' && value.length > 0) {
      return value;
    }
  }
  if (typeof body === 'string' && body.length > 0) {
    return body;
  }
  return `request failed with status ${status}`;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      ...init,
      headers: { Accept: 'application/json', ...(init?.headers ?? {}) },
    });
  } catch (cause) {
    throw new ApiError(0, cause instanceof Error ? cause.message : 'network error');
  }
  const body = await parseBody(response);
  if (!response.ok) {
    throw new ApiError(response.status, messageOf(response.status, body));
  }
  return body as T;
}

export function getJson<T>(path: string): Promise<T> {
  return request<T>(path);
}

export function postJson<T>(path: string, payload: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}
