import { test } from 'node:test';
import assert from 'node:assert/strict';
import { chmodSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { ConsoleError, readConsoleConfig, readSecondAccount } from './console.mjs';

function passwordFile(content = 'secret\n', mode = 0o600) {
  const file = join(mkdtempSync(join(tmpdir(), 'orch-e2e-')), 'password');
  writeFileSync(file, content);
  chmodSync(file, mode);
  return file;
}

const env = (overrides = {}) => ({
  ORCH_PLATFORM_URL: 'http://localhost:3001',
  ORCH_PLATFORM_USERNAME: 'developer',
  ORCH_PLATFORM_PASSWORD_FILE: passwordFile(),
  ...overrides,
});

test('accepts the origin with or without /ui', () => {
  for (const url of ['http://localhost:3001', 'http://localhost:3001/', 'http://localhost:3001/ui', 'http://localhost:3001/ui/']) {
    assert.equal(readConsoleConfig(env({ ORCH_PLATFORM_URL: url })).baseURL, 'http://localhost:3001');
  }
  assert.equal(readConsoleConfig(env({ ORCH_PLATFORM_URL: 'https://devportal.example.com' })).baseURL, 'https://devportal.example.com');
});

test('reads the password without its trailing newline and defaults to headed', () => {
  const config = readConsoleConfig(env());
  assert.equal(config.password, 'secret');
  assert.equal(config.username, 'developer');
  assert.equal(config.headless, false);
  assert.equal(readConsoleConfig(env({ ORCH_HEADLESS: '1' })).headless, true);
});

test('rejects missing or malformed settings', () => {
  for (const overrides of [
    { ORCH_PLATFORM_URL: '' },
    { ORCH_PLATFORM_URL: 'localhost:3001' },
    { ORCH_PLATFORM_URL: 'ftp://localhost:3001' },
    { ORCH_PLATFORM_URL: 'http://user:pass@localhost:3001' },
    { ORCH_PLATFORM_URL: 'http://localhost:3001/applications' },
    { ORCH_PLATFORM_URL: 'http://localhost:3001/?a=1' },
    { ORCH_PLATFORM_USERNAME: ' ' },
    { ORCH_PLATFORM_PASSWORD_FILE: '' },
    { ORCH_PLATFORM_PASSWORD_FILE: '/nonexistent/password' },
  ]) {
    assert.throws(() => readConsoleConfig(env(overrides)), ConsoleError, JSON.stringify(overrides));
  }
});

test('rejects shared or empty password files', () => {
  assert.throws(() => readConsoleConfig(env({ ORCH_PLATFORM_PASSWORD_FILE: passwordFile('secret', 0o644) })), /chmod 600/);
  assert.throws(() => readConsoleConfig(env({ ORCH_PLATFORM_PASSWORD_FILE: passwordFile('\n') })), /rỗng/);
});

test('error messages never contain the password', () => {
  const file = passwordFile('top-secret-value', 0o644);
  assert.throws(() => readConsoleConfig(env({ ORCH_PLATFORM_PASSWORD_FILE: file })), (error) => !error.message.includes('top-secret-value'));
});

test('second account reuses the first password file unless its own is given', () => {
  const first = readConsoleConfig(env());
  const shared = readSecondAccount(first, {}, { usernameVar: 'DEV_USER', passwordFileVar: 'DEV_PW', defaultUsername: 'developer' });
  assert.equal(shared.username, 'developer');
  assert.equal(shared.password, 'secret');
  assert.equal(shared.baseURL, first.baseURL);
  const own = readSecondAccount(first, { DEV_USER: 'dev2', DEV_PW: passwordFile('other\n') }, { usernameVar: 'DEV_USER', passwordFileVar: 'DEV_PW' });
  assert.deepEqual([own.username, own.password], ['dev2', 'other']);
  assert.throws(() => readSecondAccount(first, {}, { usernameVar: 'DEV_USER', passwordFileVar: 'DEV_PW' }), ConsoleError);
});
