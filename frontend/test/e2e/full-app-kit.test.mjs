import { describe, expect, it } from 'vitest';
import { namespaceLookupResult, requiredJSON, responseFromAction } from './full-app-kit.mjs';

const response = (status, body) => ({ ok: () => status >= 200 && status < 300, status: () => status, json: async () => body });

describe('full-app acceptance required reads', () => {
  it.each([401, 403, 500])('fails closed for HTTP %i', async (status) => {
    await expect(requiredJSON(response(status, { accounts: [] }), '/driver-accounts')).rejects.toThrow(`HTTP ${status}`);
  });

  it('requires a well-formed JSON object and accepts a genuine empty catalog', async () => {
    await expect(requiredJSON(response(200, null), '/driver-accounts')).rejects.toThrow('malformed object');
    await expect(requiredJSON(response(200, []), '/driver-accounts')).rejects.toThrow('malformed object');
    await expect(requiredJSON(response(200, { accounts: [] }), '/driver-accounts')).resolves.toEqual({ accounts: [] });
  });

  it('distinguishes Kubernetes NotFound from forbidden, transport and malformed results', () => {
    expect(namespaceLookupResult({ code: 0, stdout: 'uid-1', stderr: '' })).toBe('uid-1');
    expect(namespaceLookupResult({ code: 1, stdout: '', stderr: 'Error from server (NotFound): namespaces "run" not found' })).toBe('');
    for (const result of [
      { code: 1, stdout: '', stderr: 'Forbidden' },
      { code: 1, stdout: '', stderr: 'connection refused' },
      { code: 0, stdout: '', stderr: '' },
    ]) expect(() => namespaceLookupResult(result)).toThrow();
  });
});


describe('human action/response lifetime', () => {
  it('observes a response rejection while the click is still pending', async () => {
    let release;
    const action = new Promise((resolve) => { release = resolve; });
    const failure = new Error('response timeout');
    const page = { waitForResponse: () => Promise.reject(failure) };
    const observed = responseFromAction(page, () => true, () => action);
    await expect(observed).rejects.toBe(failure);
    release();
  });
  it('observes an action rejection while the response is still pending', async () => {
    let release;
    const response = new Promise((resolve) => { release = resolve; });
    const failure = new Error('button cannot be clicked');
    await expect(responseFromAction({ waitForResponse: () => response }, () => true, async () => { throw failure; })).rejects.toBe(failure);
    release({ ok: true });
  });
  it('returns the observed response only after the action completed', async () => {
    const response = { ok: true };
    let clicked = false;
    const found = await responseFromAction({ waitForResponse: () => Promise.resolve(response) }, () => true, async () => { clicked = true; });
    expect(clicked).toBe(true);
    expect(found).toBe(response);
  });
});
