import { afterEach, expect, it, vi } from 'vitest';
import { ApiError, api, describeError, isBusy, isStaleConflict, sessionExpiredEvent } from './client';
import { errorMessages, messageFor } from './errors';

afterEach(() => { vi.unstubAllGlobals(); });

const respond = (body: unknown, status: number) => vi.stubGlobal('fetch', vi.fn(async () => Response.json(body, { status })));
const failure = async (path = '/x') => api(path).then(() => { throw new Error('expected a failure'); }, (error: unknown) => error as ApiError);

it('reads code, field and allow-listed params, never the backend sentence', async () => {
  respond({ error: 'raw English SECRET', code: 'PLANNING_REJECTED', field: 'scoreAfter', params: { stage: 'graph', reason: 'before-mismatch', extra: 'SECRET' } }, 400);
  const error = await failure();
  expect(error).toBeInstanceOf(ApiError);
  expect([error.status, error.code, error.field, error.params]).toEqual([400, 'PLANNING_REJECTED', 'scoreAfter', { stage: 'graph', reason: 'before-mismatch' }]);
  expect(error.message).toContain(errorMessages.PLANNING_REJECTED);
  expect(error.message).not.toMatch(/SECRET|raw English/);
});

it.each([
  ['unknown code', { code: 'SOMETHING_NEW', error: 'raw SECRET' }],
  ['prototype code', { code: '__proto__', error: 'raw SECRET' }],
  ['constructor code', { code: 'constructor', error: 'raw SECRET' }],
  ['non-string code', { code: { toString: 'x' }, error: 'raw SECRET' }],
  ['array body', ['raw SECRET']],
  ['null body', null],
  ['string body', 'raw SECRET'],
])('falls back to a safe Vietnamese status message for %s', async (_name, body) => {
  respond(body, 502);
  const error = await failure();
  expect(error.code).toBeUndefined();
  expect(error.message).toBe(messageFor(502));
  expect(error.message).not.toMatch(/SECRET|raw/);
});

it('drops unknown or malformed field and params', async () => {
  respond({ code: 'INVALID_REQUEST', field: 'user.alias.SECRET', params: { stage: '__proto__', reason: 'SECRET' } }, 400);
  const error = await failure();
  expect([error.code, error.field, error.params]).toEqual(['INVALID_REQUEST', undefined, undefined]);
  respond({ code: 'INVALID_REQUEST', field: { a: 1 }, params: ['graph'] }, 400);
  const second = await failure();
  expect([second.field, second.params]).toEqual([undefined, undefined]);
});

it('sanitizes manually built ApiError instances too', () => {
  const error = new ApiError(409, 'NOT_A_CODE', 'raw.SECRET', { stage: 'SECRET', reason: 'SECRET' });
  expect([error.code, error.field, error.params]).toEqual([undefined, undefined, undefined]);
  expect(error.message).not.toMatch(/SECRET|NOT_A_CODE/);
  expect(new ApiError(Number.NaN, 'DUPLICATE').status).toBe(0);
});

it('handles a non-JSON error body and a network failure without raw text', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response('<html>raw SECRET</html>', { status: 500 })));
  expect((await failure()).message).toBe(messageFor(500));
  vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('Failed to fetch SECRET'); }));
  const network = await api('/x').catch((error: unknown) => error);
  expect(network).not.toBeInstanceOf(ApiError);
  expect(describeError(network, 'Chưa rõ kết quả.')).toBe('Chưa rõ kết quả.');
});

it.each(['PUT', 'POST', 'DELETE'])('does not replay a failed %s mutation by itself', async (method) => {
  const fetcher = vi.fn(async (_url: string, _init?: RequestInit) => Response.json({ code: 'SERVICE_UNAVAILABLE', error: 'raw SECRET' }, { status: 503 }));
  vi.stubGlobal('fetch', fetcher);
  const error = await api('/y', { method, body: JSON.stringify({ value: 'one' }) }).then(() => { throw new Error('expected a failure'); }, (reason: unknown) => reason as ApiError);
  expect(error.code).toBe('SERVICE_UNAVAILABLE');
  await new Promise((resolve) => setTimeout(resolve, 50));
  expect(fetcher).toHaveBeenCalledTimes(1);
  const [url, init] = fetcher.mock.calls[0] ?? [];
  expect(url).toBe('/api/v1/y');
  expect(init?.method).toBe(method);
  expect(init?.body).toBe('{"value":"one"}');
});

it('separates BUSY, stale and unknown 409 by code first', () => {
  expect(isBusy(new ApiError(409, 'ENVIRONMENT_BUSY'))).toBe(true);
  expect(isStaleConflict(new ApiError(409, 'ENVIRONMENT_BUSY'))).toBe(false);
  expect(isStaleConflict(new ApiError(409, 'STALE_VERSION'))).toBe(true);
  expect(isStaleConflict(new ApiError(409, 'STALE_PREVIEW'))).toBe(true);
  expect(isStaleConflict(new ApiError(409, 'DUPLICATE'))).toBe(false);
  expect(isStaleConflict(new ApiError(409))).toBe(true);
  expect(isStaleConflict(new Error('409'))).toBe(false);
});

it('announces session expiry only for protected 401', async () => {
  const listener = vi.fn();
  window.addEventListener(sessionExpiredEvent, listener);
  respond({ code: 'UNAUTHORIZED' }, 401);
  await failure('/applications');
  await failure('/auth/sign-in');
  window.removeEventListener(sessionExpiredEvent, listener);
  expect(listener).toHaveBeenCalledTimes(1);
});

it('names only a bounded registered resource Type for INVALID_SCORE and drops malformed ones', async () => {
  respond({ code: 'INVALID_SCORE', error: 'raw SECRET', params: { resourceType: 'postgres', other: 'SECRET' } }, 400);
  const known = await failure();
  expect(known.params).toEqual({ resourceType: 'postgres' });
  expect(known.message).toContain('loại postgres');
  expect(known.message).not.toMatch(/SECRET|alias|db/);
  for (const resourceType of ['Bad_SECRET', 'a'.repeat(64), '', 7, { a: 1 }, '__proto__', 'x y']) {
    respond({ code: 'INVALID_SCORE', params: { resourceType } }, 400);
    const bad = await failure();
    expect(bad.params).toBeUndefined();
    expect(bad.message).not.toContain('loại ');
  }
  respond({ code: 'INVALID_SCORE', field: 'resources' }, 400);
  const unknown = await failure();
  expect(unknown.field).toBe('resources');
  expect(unknown.message).toContain('loại chưa được đăng ký');
  expect(new ApiError(400, 'INVALID_SCORE', 'resources', { resourceType: 'Bad_SECRET' }).message).not.toContain('SECRET');
});

it('maps the resource deletion codes to specific Vietnamese text and keeps only allowlisted cleanup reasons', async () => {
  respond({ code: 'RESOURCE_CONFIRMATION_MISMATCH', field: 'confirmationName', error: 'expected-name SECRET' }, 400);
  const mismatch = await failure();
  expect([mismatch.code, mismatch.field]).toEqual(['RESOURCE_CONFIRMATION_MISMATCH', 'confirmationName']);
  expect(mismatch.message).toBe(errorMessages.RESOURCE_CONFIRMATION_MISMATCH);
  expect(mismatch.message).not.toMatch(/SECRET|expected-name/);

  respond({ code: 'CLEANUP_REFUSED', params: { reason: 'HAS_CONSUMERS', extra: 'SECRET' } }, 409);
  const refused = await failure();
  expect(refused.params).toEqual({ cleanupReason: 'HAS_CONSUMERS' });
  expect(refused.message).toContain('Không thể xóa:');
  expect(refused.message).toContain('Chưa có gì bị xóa.');

  // An unknown reason keeps the generic CLEANUP_REFUSED text and exposes nothing.
  respond({ code: 'CLEANUP_REFUSED', params: { reason: 'password=hunter2' } }, 409);
  const unknown = await failure();
  expect(unknown.params).toBeUndefined();
  expect(unknown.message).toBe(errorMessages.CLEANUP_REFUSED);
  expect(unknown.message).not.toContain('hunter2');
});
