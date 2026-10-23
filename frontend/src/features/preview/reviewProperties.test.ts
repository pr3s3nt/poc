import { expect, it } from 'vitest';
import { nodeFacts } from './reviewProperties';
import type { ReviewArtifact, ReviewNode } from './DeploymentReview';

const database: ReviewNode = { id: 'postgres.default#shared.db', kind: 'resource', resourceType: 'postgres', class: 'default', name: 'Database', properties: [
  { key: 'database', availability: 'AVAILABLE', value: 'catalog', source: 'configured' },
  { key: 'port', availability: 'AVAILABLE', value: '5432', source: 'module-output' },
  { key: 'namespace', availability: 'UNKNOWN', source: 'configured' },
] };
it('prioritizes known database facts and keeps deferred values explicit', () => {
  const facts = nodeFacts(database, []);
  expect(facts.slice(0, 2)).toEqual([['Tên database', 'catalog'], ['Port', '5432']]);
  expect(facts).toContainEqual(['Namespace', 'Chờ đầu ra tài nguyên']);
});
it('never turns raw sources or unknown property keys into card facts', () => {
  const secret = 'never-show-this-value';
  const node = { ...database, properties: [...database.properties!, { key: 'password', availability: 'AVAILABLE' as const, value: secret, source: 'configured' }] };
  const raw: ReviewArtifact = { id: 'raw', nodeId: node.id, kind: 'terraform-source', path: 'main.tf', language: 'hcl', before: { availability: 'ABSENT' }, after: { availability: 'PARTIAL', text: `password = "${secret}"` } };
  expect(JSON.stringify(nodeFacts(node, [raw]))).not.toContain(secret);
});
it('supports older nodes and derives all container ports from safe manifest fields only', () => {
  const node: ReviewNode = { id: 'api', kind: 'workload', name: 'API', resourceType: 'workload', class: 'default' };
  const manifest: ReviewArtifact = { id: 'api:deployment', nodeId: 'api', kind: 'kubernetes-manifest', path: 'Deployment/api', language: 'yaml', before: { availability: 'ABSENT' }, after: { availability: 'PARTIAL', text: JSON.stringify({ spec: { replicas: 2, template: { spec: { containers: [ { image: 'api:v2', ports: [{ containerPort: 8080 }], env: [{ name: 'TOKEN', value: '<redacted>' }] }, { image: 'sidecar:v1', ports: [{ containerPort: 9090 }] } ] } } } }) } };
  const facts = nodeFacts(node, [manifest]);
  expect(facts).toContainEqual(['Port', '8080, 9090']);
  expect(facts).toContainEqual(['Replicas', '2']);
  expect(JSON.stringify(facts)).not.toMatch(/TOKEN|redacted/);
});
