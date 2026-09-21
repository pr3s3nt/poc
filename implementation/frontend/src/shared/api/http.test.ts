import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, getJson, postJson } from './http';

function mockFetch(response: Response) {
  const spy = vi.fn().mockResolvedValue(response);
  vi.stubGlobal('fetch', spy);
  return spy;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('http transport', () => {
  it('calls the same-origin API base path', async () => {
    const spy = mockFetch(new Response(JSON.stringify({ ok: true }), { status: 200 }));
    await getJson<{ ok: boolean }>('/applications');
    expect(spy).toHaveBeenCalledWith('/api/v1/applications', expect.objectContaining({ headers: expect.any(Object) }));
  });

  it('sends JSON payloads on POST', async () => {
    const spy = mockFetch(new Response(JSON.stringify({ deploymentId: 'dep-1' }), { status: 201 }));
    await postJson('/deployments', { workloadId: 'backend' });
    const init = spy.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe('POST');
    expect(init.body).toBe(JSON.stringify({ workloadId: 'backend' }));
  });

  it('turns API errors into ApiError with the server message', async () => {
    mockFetch(new Response(JSON.stringify({ error: 'score: invalid document' }), { status: 400 }));
    await expect(getJson('/deployments')).rejects.toMatchObject({
      name: 'ApiError',
      status: 400,
      message: 'score: invalid document',
    });
  });

  it('reports network failures as status 0', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
    const error = await getJson('/applications').catch((cause: unknown) => cause);
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(0);
  });
});
