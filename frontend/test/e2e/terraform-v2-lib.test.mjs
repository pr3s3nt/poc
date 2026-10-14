import { mkdtempSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { createRecorder, nonexistentDigest, parseImageRef, pickByImage, registryDigestStatus, safeError, waitFor } from './terraform-v2-lib.mjs';

describe('terraform-v2-lib', () => {
  it('records assertions and phases to files, fails closed on fake sources and duplicates, and keeps a failed assertion', () => {
    const dir = mkdtempSync(join(tmpdir(), 'v2lib-'));
    const rec = createRecorder({ dir, now: () => 5000, startedAt: () => 1000 });
    rec.phase('PREPARE', 'Chuẩn bị');
    rec.record('ui.signin', 'PREPARE', 'ui', true, 'ok');
    expect(() => rec.record('x.fake', 'READY', 'fake', true)).toThrow(/cannot count/);
    expect(() => rec.record('ui.signin', 'PREPARE', 'ui', true)).toThrow(/duplicate/);
    expect(() => rec.record('bad.phase', 'NOPE', 'ui', true)).toThrow(/unknown phase/);
    expect(() => rec.record('job.failed', 'JOB', 'private-read', false, 'row missing')).toThrow(/failed/);
    const saved = JSON.parse(readFileSync(join(dir, 'assertions.json'), 'utf8'));
    expect(saved.map((a) => [a.id, a.pass])).toEqual([['ui.signin', true], ['job.failed', false]]);
    expect(JSON.parse(readFileSync(join(dir, 'phases.json'), 'utf8'))).toEqual([{ phase: 'PREPARE', atSec: 4, caption: 'Chuẩn bị' }]);
  });

  it('never leaks arguments or secrets through error text', () => {
    expect(safeError(new Error('Command failed: kubectl --kubeconfig /priv/kc exec --pass s3cr3t\nstderr…'))).not.toMatch(/s3cr3t|\/priv/);
    expect(safeError({ safe: 'fixed' })).toBe('fixed');
  });

  it('parses image references with registry defaults', () => {
    const digest = `sha256:${'a'.repeat(64)}`;
    expect(parseImageRef(`redis:7.4.7-alpine@${digest}`)).toEqual({ host: 'registry-1.docker.io', repo: 'library/redis', tag: '7.4.7-alpine', digest });
    expect(parseImageRef(`ghcr.io/acme/app:1@${digest}`)).toMatchObject({ host: 'ghcr.io', repo: 'acme/app' });
    expect(parseImageRef('not valid!')).toBeUndefined();
    expect(nonexistentDigest('r1')).toMatch(/^sha256:[0-9a-f]{64}$/);
    expect(nonexistentDigest('r1')).toBe(nonexistentDigest('r1'));
    expect(nonexistentDigest('r1')).not.toBe(nonexistentDigest('r2'));
  });

  it('proves a digest missing only on a definitive 404', async () => {
    const ref = `redis:7-alpine@${nonexistentDigest('x')}`;
    const respond = (status) => async (url) => (String(url).includes('auth.docker.io') ? { ok: true, json: async () => ({ token: 't' }) } : { status });
    expect(await registryDigestStatus(ref, respond(404))).toBe('missing');
    expect(await registryDigestStatus(ref, respond(200))).toBe('present');
    expect(await registryDigestStatus(ref, respond(500))).toBe('unknown');
    expect(await registryDigestStatus(ref, async () => { throw new Error('offline'); })).toBe('unknown');
    expect(await registryDigestStatus('redis:7', respond(404))).toBe('unknown');
  });

  it('waits with a safe timeout', async () => {
    let n = 0;
    expect(await waitFor(async () => ++n >= 3, { timeoutMs: 2000, intervalMs: 10, what: 'x' })).toBe(true);
    await expect(waitFor(async () => false, { timeoutMs: 30, intervalMs: 10, what: 'the thing' })).rejects.toThrow('timed out waiting for the thing');
  });

  it('picks workloads by image', () => {
    const list = { items: [{ spec: { template: { spec: { containers: [{ image: 'postgres:16' }] } } } }, { spec: { template: { spec: { containers: [{ image: 'redis:7' }] } } } }] };
    expect(pickByImage(list, 'redis')).toHaveLength(1);
  });
});
