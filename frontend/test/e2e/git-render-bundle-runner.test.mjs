import { describe, expect, it } from 'vitest';
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';

// Unit-level fault injection only. The end-user kind acceptance uses real CLIs.
// Exercise the production exit trap without creating/deleting external resources.
const runner = readFileSync(resolve(import.meta.dirname, '../../../backend/test/integration/git-render-bundle-kind.sh'), 'utf8');
const cleanup = runner.match(/^cleanup\(\) \{[\s\S]*?^\}\n(?=trap cleanup EXIT)/m)?.[0];
if (!cleanup) throw new Error('Acceptance runner cleanup entrypoint missing');

describe('real acceptance runner cleanup gate', () => {
  for (const failure of ['none', 'containers', 'volumes', 'git-unavailable', 'images', 'docker-unavailable']) {
    it(`returns the correct process status when cleanup result is ${failure}`, () => {
      const directory = mkdtempSync(join(tmpdir(), 'manifest-cleanup-test-'));
      try {
        mkdirSync(join(directory, 'private'));
        writeFileSync(join(directory, 'private/compose-up'), '');
        const shell = `
log() { :; }
docker() {
 case "$1 $2" in
  'info ') [[ "$CASE" != docker-unavailable ]] ;;
  'ps -aq') if [[ "$CASE" == containers ]]; then echo owned-container; fi ;;
  'volume ls') if [[ "$CASE" == volumes ]]; then echo owned-volume; fi ;;
  'image inspect') [[ "$CASE" == images ]] ;;
  'rmi '*) return 0 ;;
  *) return 1 ;;
 esac
}
git() {
 if [[ "$*" == *ls-remote* && "$CASE" == git-unavailable ]]; then return 1; fi
 return 0
}
COMPOSE=(true)
PROJECT=unit-owned CLUSTER=unit-owned VAULT_NAME=unit-owned TAG=unit-owned RUN_ID=unit-owned
CLUSTER_CREATED=0 FIXTURE_PUSHED=0
if [[ "$CASE" == git-unavailable ]]; then FIXTURE_PUSHED=1; fi
REPO=/nonexistent ORIGIN_URL=https://example.invalid/repo.git FIXTURE_REF=test/owned
${cleanup}
true
cleanup
`;
        const result = spawnSync('bash', ['-c', shell], { encoding: 'utf8', env: { ...process.env, CASE: failure, PRIVATE: join(directory, 'private'), STATUS_FILE: join(directory, 'status.txt') } });
        const expected = failure === 'none' ? 0 : 1;
        expect(result.status, result.stdout + result.stderr).toBe(expected);
        expect(readFileSync(join(directory, 'status.txt'), 'utf8')).toContain(`final_status=${expected}`);
      } finally { rmSync(directory, { recursive: true, force: true }); }
    });
  }
});
