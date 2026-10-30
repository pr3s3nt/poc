import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { ensureGitAccess } from './git-access-ui.mjs';

// Unit harness only: models UI-produced payloads and route guards, no server.
function harness(current, desired, { testStatus = 200, pageVersion = current.version, fillError = false } = {}) {
  const baseURL = 'https://console.example';
  let stored = { ...current }, pending;
  const routes = new Map(), calls = [], draft = { authenticated: true, expectedVersion: pageVersion };
  const fields = { 'URL Git / phạm vi': 'url', Username: 'username', Token: 'token', 'Chứng chỉ CA (PEM, tùy chọn)': 'caPem', 'Repo kiểm tra': 'repositoryUrl', 'Nhánh, tag hoặc commit kiểm tra': 'ref' };
  const form = {
    waitFor: async () => {},
    getByLabel: label => ({
      fill: async value => { if (fillError && label === 'Token') throw Error('fill failed: secret-sentinel'); draft[fields[label]] = value; },
      selectOption: async () => {},
    }),
    locator: () => ({ click: async () => {} }),
    getByRole: (_role, { name }) => ({ click: async () => {
      const method = name === 'Lưu' ? 'PUT' : 'POST';
      const url = baseURL + '/api/v1/platform/settings/git' + (method === 'POST' ? '/test' : '');
      const body = { url: draft.url, username: draft.username, token: draft.token, authenticated: true, caPem: draft.caPem, expectedVersion: draft.expectedVersion, ...(method === 'POST' ? { repositoryUrl: draft.repositoryUrl, ref: draft.ref } : {}) };
      let status;
      const route = {
        request: () => ({ method: () => method, postDataJSON: () => body }),
        fulfill: async value => { status = value.status; },
        fallback: async () => {
          calls.push(method); status = method === 'POST' ? testStatus : 200;
          if (method === 'PUT') stored = { url: desired.url, username: desired.username, authenticated: true, caPem: desired.caPem, tokenConfigured: true, version: current.version + 1 };
        },
      };
      await routes.get(url)(route);
      pending({ ok: () => status === 200, status: () => status, json: async () => method === 'POST' ? { reachable: true } : { settings: stored } });
    } }),
  };
  const page = {
    goto: async () => {}, locator: () => ({ filter: () => form }), getByRole: () => ({}),
    route: async (url, guard) => routes.set(url, guard), unroute: async url => routes.delete(url),
    waitForResponse: () => new Promise(resolve => { pending = resolve; }), evaluate: async () => {},
  };
  return { page, baseURL, desired, get: async () => { calls.push('GET'); return { settings: stored }; }, calls, routes };
}
function desired(tokenFile) { return { url: 'https://git.example/team', username: 'reader', caPem: '', repositoryUrl: 'https://git.example/team/catalog.git', ref: 'main', tokenFile }; }
const currentFor = d => ({ url: d.url, username: d.username, caPem: '', authenticated: true, tokenConfigured: true, version: 3 });

test('reuse tests stored access without reading token file or sending PUT', async () => {
  const d = desired('/must-not-read'); const h = harness(currentFor(d), d);
  await ensureGitAccess(h);
  assert.deepEqual(h.calls, ['GET', 'POST', 'GET']); assert.equal(h.routes.size, 0);
});
test('create and explicit rotation test first, then save exactly once and read back', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'orch-git-ui-'));
  try {
    const file = join(dir, 'token'); writeFileSync(file, 'secret-sentinel', { mode: 0o600 });
    const d = desired(file);
    for (const current of [{ version: 0 }, currentFor(d)]) {
      const h = harness(current, d);
      await ensureGitAccess({ ...h, update: current.version > 0 });
      assert.deepEqual(h.calls, ['GET', 'POST', 'PUT', 'GET']); assert.equal(h.routes.size, 0);
    }
  } finally { rmSync(dir, { recursive: true }); }
});
test('failed connection never saves; concurrent UI version is blocked before forwarding', async () => {
  const d = desired('/must-not-read'); const current = currentFor(d);
  const failed = harness(current, d, { testStatus: 403 });
  await assert.rejects(() => ensureGitAccess(failed), /HTTP 403/);
  assert.deepEqual(failed.calls, ['GET', 'POST']); assert.equal(failed.routes.size, 0);
  const raced = harness(current, d, { pageVersion: 4 });
  await assert.rejects(() => ensureGitAccess(raced), /đã chặn request/);
  assert.deepEqual(raced.calls, ['GET']); assert.equal(raced.routes.size, 0);
});
test('unknown browser errors never expose filled credential', async () => {
  const d = desired('/must-not-read'); const h = harness(currentFor(d), d, { fillError: true });
  await assert.rejects(() => ensureGitAccess(h), error => !error.message.includes('secret-sentinel'));
  assert.deepEqual(h.calls, ['GET']);
});
