import { afterEach, describe, expect, it, vi } from 'vitest';
import { spawn } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { removeTechnicalPods, stopOwnedProcess, validateKubectlTarget } from './terraform-v2-aws-definitions.mjs';

let dirs = [];
afterEach(() => { for (const dir of dirs) rmSync(dir, { recursive: true, force: true }); dirs = []; });

function targetFixture() {
  const dir = mkdtempSync(join(tmpdir(), 'v04-eks-target-'));
  dirs.push(dir);
  const kubeconfig = join(dir, 'eks.kubeconfig');
  writeFileSync(kubeconfig, 'apiVersion: v1\nkind: Config\n', { mode: 0o600 });
  return { kubectl: '/usr/bin/kubectl', kubeconfig, context: 'eks-run', env: { AWS_REGION: 'us-east-1', AWS_PROFILE: 'owned-run' } };
}

describe('owned AWS Kubernetes proxy lifecycle', () => {
  it('recognizes a real child terminated with SIGTERM as stopped', async () => {
    const child = spawn(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], { stdio: 'ignore' });
    await new Promise((resolve, reject) => { child.once('spawn', resolve); child.once('error', reject); });
    await stopOwnedProcess(child, 'test proxy child');
    expect(child.exitCode).toBeNull();
    expect(child.signalCode).toBe('SIGTERM');
  });

  it('passes the resolved private EKS target to the cleanup proxy and skips an empty helper set', async () => {
    const target = targetFixture();
    validateKubectlTarget(target);
    const seen = [];
    const stop = vi.fn();
    const removed = await removeTechnicalPods({
      target, pods: [{ name: 'tech-owned', uid: 'uid-1' }], namespace: 'owned-ns', requiredLabels: { 'run-id': 'r1' },
      waitForAbsent: async () => true,
      startProxy: async (actual) => { seen.push(actual); return { request: vi.fn(async () => ({ status: 404 })), stop }; },
    });
    expect(seen).toEqual([target]);
    expect(removed).toEqual(['tech-owned']);
    expect(stop).toHaveBeenCalledOnce();
    const unexpectedProxy = vi.fn();
    expect(await removeTechnicalPods({ target, pods: [], namespace: 'owned-ns', requiredLabels: {}, waitForAbsent: async () => true, startProxy: unexpectedProxy })).toEqual([]);
    expect(unexpectedProxy).not.toHaveBeenCalled();
  });

  it('rejects host/default kubeconfig fallback', () => {
    expect(() => validateKubectlTarget({ kubectl: '/usr/bin/kubectl', kubeconfig: '', context: '', env: {} })).toThrow();
  });
});
