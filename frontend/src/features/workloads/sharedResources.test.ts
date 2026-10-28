import { expect, it, vi } from 'vitest';
import { newSharedId, sharedResources, sharedSelectionProblem } from './sharedResources';
import type { ResourceType, Workload } from './api';

const types: ResourceType[] = [{ key: 'postgres', inputs: [{ name: 'database', type: 'string' }, { name: 'replicas', type: 'number' }, { name: 'tls', type: 'bool' }], outputs: [] }];
const workload = (id: string, resources: Record<string, unknown>, state?: Workload['state']): Workload => ({ id, ready: true, score: { resources }, state });

it('offers shared resources once, excludes private and pending deletes, preserving authored typed params', () => {
  const spec = { id: 'database', type: 'postgres', params: { database: 'app', replicas: 2, tls: false } };
  const choices = sharedResources([workload('api', { db: spec, private: { type: 'postgres' } }), workload('worker', { store: spec }), workload('gone', { old: { ...spec, id: 'removed' } }, 'PENDING_DELETE')], types);
  expect(choices).toHaveLength(1);
  const selected = choices[0]!.dependency!;
  expect(selected).toMatchObject({ sharedId: 'database', params: { database: 'app', replicas: '2', tls: 'false' } });
  expect(sharedSelectionProblem({ ...selected, alias: 'local', reuseId: 'database' }, choices)).toBeUndefined();
  expect(sharedSelectionProblem({ ...selected, reuseId: 'database', params: { database: 'changed' } }, choices)).toBeDefined();
  expect(sharedSelectionProblem({ ...selected, reuseId: 'database' }, [])).toBeDefined();
});

it('disables conflicting identities in either order and unsafe or workload-local parameter projections', () => {
  const specs = [
    { id: 'db', type: 'postgres', params: { database: 'a' } },
    { id: 'db', type: 'postgres', params: { database: 'b' } },
  ];
  for (const order of [specs, [...specs].reverse()]) expect(sharedResources(order.map((spec, i) => workload(String(i), { db: spec })), types)[0]?.dependency).toBeUndefined();
  for (const params of [{ database: '${resources.env.KEY}' }, { database: { nested: true } }, { extra: 'unknown' }]) {
    expect(sharedResources([workload('api', { db: { id: 'db', type: 'postgres', params } })], types)[0]?.dependency).toBeUndefined();
  }
});


it('generates distinct valid resource IDs on internal HTTP without randomUUID', () => {
  let draw = 0;
  vi.stubGlobal('crypto', { getRandomValues: (bytes: Uint8Array) => bytes.fill(++draw) });
  try {
    const first = newSharedId();
    expect(first).toMatch(/^resource-[a-f0-9]{32}$/);
    expect(newSharedId()).not.toBe(first);
  } finally { vi.unstubAllGlobals(); }
});
