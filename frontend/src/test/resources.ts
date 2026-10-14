import type { CleanupOperation, Candidate, DeletionPreview, ResourceDetail } from '../features/resources/api';

// Fixtures of the resource detail / deletion tests (UC-07 DEL-*, UC-09 RES-*). They mirror the
// server's safe projection; no private field exists in them.

export const rootId = '00000000-0000-4000-8000-000000000001';
export const depA = '00000000-0000-4000-8000-0000000000a1';
export const depB = '00000000-0000-4000-8000-0000000000b2';
export const depC = '00000000-0000-4000-8000-0000000000c3';
export const rootName = 'postgres.default#shared.db';

export const candidate = (over: Partial<Candidate> & { id: string; descriptor: string }): Candidate => ({
  type: 'vpc', class: 'default', status: 'UNREFERENCED', version: 3, selectable: true, reasons: [], requiredBy: [rootId], consumers: [], ...over,
});

export const rootCandidate = candidate({ id: rootId, descriptor: rootName, type: 'postgres', requiredBy: [] });
export const dependencyA = candidate({ id: depA, descriptor: 'vpc.default#environments.shop.vpc' });
export const dependencyB = candidate({ id: depB, descriptor: 'k8s-namespace.default#environments.shop.ns', selectable: false, reasons: ['HAS_CONSUMERS'], consumers: [{ kind: 'RESOURCE', name: 'redis.default#shared.cache', resourceId: 'x', status: 'READY' }] });
export const dependencyC = candidate({ id: depC, descriptor: 'k8s-secret.default#environments.shop.cfg' });

export function preview(over: Partial<DeletionPreview> = {}): DeletionPreview {
  return { token: 'rd1.4102444800.' + 'a'.repeat(64), expiresAt: '2100-01-01T00:00:00Z', applicationId: 'shop', environment: 'staging', generation: 0, confirmationName: rootName,
    root: rootCandidate, candidates: [dependencyA, dependencyB, dependencyC], ...over };
}

export function detail(over: Partial<ResourceDetail> = {}): ResourceDetail {
  return { id: rootId, applicationId: 'shop', environment: 'staging', descriptor: rootName, type: 'postgres', class: 'default', resourceId: 'shared.db', status: 'UNREFERENCED', scope: { type: 'SHARED', id: 'shop.staging' },
    owner: 'SHARED', generation: 0, module: { id: 'aurora', revision: 'rev1' }, version: 4, outputs: { host: 'db.internal', password: '***redacted***' }, consumers: [], eligibility: { eligible: true, reasons: [] }, ...over };
}

export function operation(over: Partial<CleanupOperation> = {}): CleanupOperation {
  return { operationId: 'op-1', status: 'ACTIVE', stage: 'DESTROYING', allVerified: false, authorizationMode: 'USER_CONFIRMED', actor: 'platform-engineer', rootId,
    resources: [{ id: rootId, descriptor: rootName, status: 'DESTROYING', outcome: 'DESTROYING', verified: false, remaining: [], remainingUnknown: false }], remainingIds: [], startedAt: '2026-10-14T00:00:00Z', updatedAt: '2026-10-14T00:00:01Z', ...over };
}

/** Server wire form of a ResourceDetail response. */
export const wire = {
  resource: (d: ResourceDetail) => ({ resource: d }),
  preview: (p: DeletionPreview) => ({ preview: p }),
  admission: (operationId = 'op-1', replayed = false) => ({ cleanup: { operationId, status: 'ACTIVE', stage: 'REQUESTED', replayed } }),
  cleanup: (o: CleanupOperation) => ({ cleanup: o }),
};
