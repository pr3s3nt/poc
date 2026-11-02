import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { readPrivateFile } from './console.mjs';
import { readGitToken } from './git-access-config.mjs';
test('Windows skips POSIX mode checks for every shared private file reader; Linux keeps enforcing them', () => {
  const dir = mkdtempSync(join(tmpdir(), 'orch-windows-'));
  try {
    const file = join(dir, 'credential'); writeFileSync(file, 'fixture-token', { mode: 0o644 });
    for (const what of ['mật khẩu', 'kubeconfig', 'Git token']) assert.equal(readPrivateFile(file, what, { platform: 'win32' }), 'fixture-token');
    assert.equal(readGitToken(file, { platform: 'win32' }), 'fixture-token');
    assert.throws(() => readPrivateFile(file, 'credential', { platform: 'linux' }), /chmod/);
    assert.throws(() => readGitToken(join(dir, 'missing'), { platform: 'win32' }), error => !/0600|chmod/u.test(error.message));
    writeFileSync(file, ''); assert.throws(() => readPrivateFile(file, 'credential', { platform: 'win32' }), /rỗng/);
  } finally { rmSync(dir, { recursive: true }); }
});
