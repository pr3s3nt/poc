import { describe, expect, it } from 'vitest';
import { deletePodWithUIDPrecondition } from './terraform-v2-aws-definitions.mjs';

const pod = { metadata: { namespace: 'orch-v2-test-run-7', name: 'tech-one', uid: 'uid-current', labels: { 'run-id': 'run-7', 'managed-by': 'orchestrator-verification' } } };

describe('technical Pod Kubernetes DeleteOptions contract', () => {
  it('sends an atomic UID precondition to the exact namespaced Pod and waits for absence', async () => {
    const calls = [];
    let absent = false;
    await deletePodWithUIDPrecondition({
      namespace: 'orch-v2-test-run-7', name: 'tech-one', expectedUID: 'uid-current', requiredLabels: pod.metadata.labels,
      request: async (method, path, body) => {
        calls.push({ method, path, body });
        if (method === 'GET') return absent ? { status: 404 } : { status: 200, body: pod };
        absent = true;
        return { status: 202, body: { status: 'Success' } };
      },
      waitForAbsent: async (check) => check(),
    });
    expect(calls).toEqual([
      { method: 'GET', path: '/api/v1/namespaces/orch-v2-test-run-7/pods/tech-one', body: undefined },
      { method: 'DELETE', path: '/api/v1/namespaces/orch-v2-test-run-7/pods/tech-one', body: { apiVersion: 'v1', kind: 'DeleteOptions', preconditions: { uid: 'uid-current' } } },
      { method: 'GET', path: '/api/v1/namespaces/orch-v2-test-run-7/pods/tech-one', body: undefined },
    ]);
  });

  it('refuses a replacement UID or changed run labels before sending DELETE', async () => {
    for (const replacement of [
      { ...pod, metadata: { ...pod.metadata, uid: 'replacement' } },
      { ...pod, metadata: { ...pod.metadata, labels: { ...pod.metadata.labels, 'run-id': 'other-run' } } },
    ]) {
      const methods = [];
      await expect(deletePodWithUIDPrecondition({
        namespace: 'orch-v2-test-run-7', name: 'tech-one', expectedUID: 'uid-current', requiredLabels: pod.metadata.labels,
        request: async (method) => { methods.push(method); return { status: 200, body: replacement }; },
        waitForAbsent: async () => true,
      })).rejects.toThrow('identity changed');
      expect(methods).toEqual(['GET']);
    }
  });
});
