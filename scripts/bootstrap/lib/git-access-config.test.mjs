import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, chmodSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { parseGitAccess, gitAccessAction, inGitScope, readGitToken } from './git-access-config.mjs';
import { loadTarget } from './target-config.mjs';
import { parseBundleConfig } from './render-bundle-config.mjs';
const ref = 'a'.repeat(40);
const raw = () => ({ gitAccess: { username: 'reader', tokenFile: '~/private/token' }, bundles: [{ source: { url: 'https://git.example.com/team/catalog.git', ref } }] });
test('Git access is opt-in; presets and public source overrides preserve existing behavior', () => {
  assert.equal(parseGitAccess({}), null);
  for (const target of ['dev', 'staging']) {
    const config = loadTarget({ target }, {}, '/private');
    assert.equal(parseGitAccess(config.raw), null);
    assert.ok(parseBundleConfig(config.raw));
  }
});
test('derive repo/scope/ref and expand private token path without reading it', () => {
  const value = parseGitAccess(raw(), {}, '/private/home');
  assert.equal(value.repositoryUrl, raw().bundles[0].source.url);
  assert.equal(value.url, value.repositoryUrl);
  assert.equal(value.ref, ref);
  assert.equal(value.tokenFile, '/private/home/private/token');
  const override = parseGitAccess(raw(), { 'git-url': 'https://git.example.com/team/catalog.git', 'git-ref': 'b'.repeat(40) });
  assert.equal(override.ref, 'b'.repeat(40));
});
test('scope matching preserves exact origin, port and whole path boundary', () => {
  assert.ok(inGitScope('https://git.example.com/team', 'https://git.example.com/team/catalog.git'));
  for (const repo of ['https://git.example.com/teams/catalog.git', 'https://git.example.com:443/team/catalog.git', 'https://git.example.com.evil/team/catalog.git']) assert.equal(inGitScope('https://git.example.com/team', repo), false);
  for (const repo of ['https://git.example.com/team/../other', 'https://reader:token@git.example.com/team', 'https://git.example.com/team/%2e%2e/other']) assert.throws(() => inGitScope('https://git.example.com/team', repo));
});
test('requires explicit, valid source/CA configuration; errors never echo submitted credential', () => {
  for (const patch of [{ token: 'secret-sentinel' }, { username: '' }, { caPemFile: '/missing-ca' }, { url: 'https://git.example.com/other' }, { ref: '../main' }]) {
    const value = raw(); Object.assign(value.gitAccess, patch);
    assert.throws(() => parseGitAccess(value), error => !error.message.includes('secret-sentinel'));
  }
});
test('reuse needs matching metadata and configured token; rotation/update requires separate authorization', () => {
  const desired = parseGitAccess(raw());
  const current = { url: desired.url, username: desired.username, authenticated: true, tokenConfigured: true, caPem: '', version: 2 };
  assert.equal(gitAccessAction({ version: 0 }, desired, false), 'create');
  assert.equal(gitAccessAction(current, desired, false), 'reuse');
  assert.equal(gitAccessAction(current, desired, true), 'update');
  for (const change of [{ username: 'other' }, { tokenConfigured: false }, { caPem: 'different' }, { authenticated: false }]) assert.throws(() => gitAccessAction({ ...current, ...change }, desired, false), /--update-git-access/);
});
test('token file must be private, bounded and single-line; invalid errors do not include values', () => {
  const dir = mkdtempSync(join(tmpdir(), 'orch-git-access-'));
  try {
    const file = join(dir, 'token');
    writeFileSync(file, 'secret-sentinel\n', { mode: 0o600 });
    assert.equal(readGitToken(file), 'secret-sentinel');
    chmodSync(file, 0o644);
    if (process.platform !== 'win32') assert.throws(() => readGitToken(file), error => !error.message.includes('secret-sentinel'));
    else assert.equal(readGitToken(file), 'secret-sentinel');
    chmodSync(file, 0o600);
    for (const text of ['', 'secret-sentinel\nother', 'x'.repeat(4097)]) {
      writeFileSync(file, text); assert.throws(() => readGitToken(file));
    }
  } finally { rmSync(dir, { recursive: true }); }
});
