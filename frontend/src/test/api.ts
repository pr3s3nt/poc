import { vi } from 'vitest';

export type Call = { method: string; url: string; body?: unknown };
export type Handler = (call: Call) => Response | Promise<Response> | undefined;

/** Stubs fetch with the first handler that answers; any other request is a 404 so a missed route is visible. */
export function stubApi(...handlers: Handler[]) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const call: Call = { method: init?.method ?? 'GET', url: String(input), body: init?.body ? JSON.parse(String(init.body)) as unknown : undefined };
    calls.push(call);
    for (const handler of handlers) {
      const response = handler(call);
      if (response) return response;
    }
    return Response.json({ code: 'NOT_FOUND' }, { status: 404 });
  }));
  return calls;
}

export const apiError = (status: number, code: string, params?: Record<string, string>, field?: string) => Response.json({ error: 'raw server sentence must never be shown', code, ...(params ? { params } : {}), ...(field ? { field } : {}) }, { status });
